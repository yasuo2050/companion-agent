// Package agent 创建并运行单请求隔离的 Eino ChatModelAgent。
package agent

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	businessv1 "companion-agent/contracts/companion-business/gen"
	businessclient "companion-agent/services/companion-agent-service/internal/client/business"
	toolbuilder "companion-agent/services/companion-agent-service/internal/tool"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"google.golang.org/protobuf/encoding/protojson"
)

type Event struct {
	Type     string
	ToolName string
	Content  string
	Message  *schema.Message
}
type Runner struct {
	model         model.ToolCallingChatModel
	business      *businessclient.Client
	maxIterations int
	modelAlias    string
}

func NewRunner(chatModel model.ToolCallingChatModel, business *businessclient.Client, maxIterations int, modelAlias string) *Runner {
	return &Runner{model: chatModel, business: business, maxIterations: maxIterations, modelAlias: modelAlias}
}

// Run 加载 Profile 后运行真实 Eino Agent Loop，并逐事件回调。
func (r *Runner) Run(ctx context.Context, userID, conversationID string, history []*schema.Message, emit func(Event)) ([]*schema.Message, error) {
	profileResponse, err := r.business.GetUserProfile(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("加载用户画像失败: %w", err)
	}
	if profileResponse.GetError() != nil {
		return nil, fmt.Errorf("加载用户画像失败: %s", profileResponse.GetError().GetMessage())
	}
	tools, err := toolbuilder.Build(r.business, userID, conversationID)
	if err != nil {
		return nil, err
	}
	instruction := systemInstruction(profileResponse.GetProfile(), time.Now())
	instance, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{Name: "companion-order-agent", Description: "游戏陪玩下单助手", Instruction: instruction, Model: r.model, ToolsConfig: adk.ToolsConfig{ToolsNodeConfig: compose.ToolsNodeConfig{Tools: tools}}, MaxIterations: r.maxIterations})
	if err != nil {
		return nil, err
	}
	started := time.Now()
	iterator := instance.Run(ctx, &adk.AgentInput{Messages: history, EnableStreaming: true})
	outputs := []*schema.Message{}
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			return outputs, event.Err
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}
		variant := event.Output.MessageOutput
		var message *schema.Message
		if variant.IsStreaming {
			chunks := []*schema.Message{}
			for {
				chunk, recvErr := variant.MessageStream.Recv()
				if recvErr == io.EOF {
					break
				}
				if recvErr != nil {
					return outputs, recvErr
				}
				chunks = append(chunks, chunk)
				if variant.Role == schema.Assistant && chunk.Content != "" {
					emit(Event{Type: "delta", Content: chunk.Content})
				}
			}
			variant.MessageStream.Close()
			if len(chunks) == 0 {
				continue
			}
			message, err = schema.ConcatMessages(chunks)
			if err != nil {
				return outputs, err
			}
		} else {
			message = variant.Message
		}
		if message == nil {
			continue
		}
		outputs = append(outputs, message)
		if variant.Role == schema.Tool {
			emit(Event{Type: "status", ToolName: variant.ToolName, Message: message})
		} else {
			emit(Event{Type: "message", Message: message})
		}
	}
	slog.Info("Agent Turn 完成", "conversation_id", conversationID, "user_id", userID, "model", r.modelAlias, "total_duration_ms", time.Since(started).Milliseconds())
	return outputs, nil
}
func systemInstruction(profile *businessv1.UserProfile, now time.Time) string {
	profileJSON, _ := protojson.Marshal(profile)
	return fmt.Sprintf(`你是游戏陪玩下单助手。当前时间：%s，时区：Asia/Shanghai。
用户画像：%s
规则：用户当前明确硬约束 > 当前软偏好 > 历史画像。只有“只要、必须、不要、以内”等才形成硬过滤。相对时间只基于当前时间解析。
所有陪玩、价格、档期、Draft、Order 必须来自 Tool，不得虚构。提到上次或之前且需要历史事实时调用 get_recent_orders。查询真实候选必须调用 search_companions。
受控标签只有：上分、教学、高段位、指挥、会聊天、话多、搞笑、安静、情绪价值、不压力、耐心、声音甜、御姐音、少年音。不得创造标签。“有意思”映射搞笑和会聊天；“别太压力”映射不压力；“带我赢”映射上分和高段位；“教教我”映射教学和耐心。
优先明确推荐一人，不把大量候选丢给用户。唯一 favorite_game_ids 和 usual_duration_minutes 可作为默认，但 Preview 必须展示；具体开始时间不得从画像猜测。
下单信息完整后先调用 create_order_draft，并用返回事实展示陪玩、游戏、时间、时长、单价、总额、draft_id 的 Preview。只有用户对当前 Preview 明确确认且没有修改条件，才调用 confirm_order。
PRICE_CHANGED 时说明变价，重新调用 create_order_draft 展示新 Preview，并等待再次确认。Tool 失败不得假装成功。回复使用简洁中文。`, now.Format(time.RFC3339), string(profileJSON))
}
