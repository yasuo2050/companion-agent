// Package tool 定义 LLM 可自主选择的 4 个陪玩下单工具。
package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	businessv1 "companion-agent/contracts/companion-business/gen"
	businessclient "companion-agent/services/companion-agent-service/internal/client/business"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type recentInput struct {
	Limit int32 `json:"limit" jsonschema_description:"返回最近订单数量，默认 1，最大 10"`
}
type searchInput struct {
	CompanionID     string   `json:"companion_id,omitempty" jsonschema_description:"明确指定的陪玩 ID"`
	GameID          string   `json:"game_id,omitempty" jsonschema_description:"用户明确指定或对话上下文已确定的游戏 ID：naraka、lol 或 hok；缺失时由服务端使用最近订单默认值"`
	Gender          string   `json:"gender,omitempty" jsonschema_description:"只有用户明确要求时传 female 或 male"`
	MaxPricePerHour int64    `json:"max_price_per_hour,omitempty" jsonschema_description:"只有用户明确说以内时传，单位分"`
	StartTime       string   `json:"start_time,omitempty" jsonschema_description:"RFC3339 开始时间，Asia/Shanghai"`
	DurationMinutes int32    `json:"duration_minutes,omitempty" jsonschema_description:"用户明确指定或对话上下文已确定的时长分钟；缺失时由服务端使用最近订单默认值"`
	PreferredTags   []string `json:"preferred_tags,omitempty" jsonschema_description:"只能使用受控标签，属于软偏好"`
	Limit           int32    `json:"limit,omitempty" jsonschema_description:"默认 5，最大 10"`
}
type draftInput struct {
	CompanionID     string `json:"companion_id" jsonschema:"required" jsonschema_description:"真实候选陪玩 ID"`
	GameID          string `json:"game_id,omitempty" jsonschema_description:"用户明确指定或对话上下文已确定的游戏 ID；缺失时由服务端使用最近订单默认值"`
	StartTime       string `json:"start_time" jsonschema:"required" jsonschema_description:"RFC3339 开始时间"`
	DurationMinutes int32  `json:"duration_minutes,omitempty" jsonschema_description:"用户明确指定或对话上下文已确定的时长；缺失时由服务端使用最近订单默认值"`
}
type confirmInput struct {
	DraftID string `json:"draft_id" jsonschema:"required" jsonschema_description:"当前 Preview 对应的 active Draft ID"`
}

// EventObserver 接收真实 Tool 执行阶段，用于 SSE 状态和 Trace。
type EventObserver func(toolName, phase string, startedAt time.Time, input any)

// OrderDefaults 是当用户未明确提供时，由服务端补齐的最近订单参数。
type OrderDefaults struct {
	GameID          string `json:"game_id,omitempty"`
	DurationMinutes int32  `json:"duration_minutes,omitempty"`
	Source          string `json:"source,omitempty"`
}

// Build 为单次请求创建带服务端身份上下文的工具集合。
func Build(client *businessclient.Client, userID, conversationID string, observers ...EventObserver) ([]tool.BaseTool, error) {
	return BuildWithDefaults(client, userID, conversationID, OrderDefaults{}, observers...)
}

// BuildWithDefaults 创建工具，并在工具执行前用最近订单参数补齐缺失的游戏和时长。
func BuildWithDefaults(client *businessclient.Client, userID, conversationID string, defaults OrderDefaults, observers ...EventObserver) ([]tool.BaseTool, error) {
	observe := func(name, phase string, started time.Time, input any) {
		for _, observer := range observers {
			if observer != nil {
				observer(name, phase, started, input)
			}
		}
	}
	recent, err := utils.InferTool("get_recent_orders", "查询当前用户真实历史订单；出现上次、之前、还是那个等指代时使用", func(ctx context.Context, in recentInput) (map[string]any, error) {
		started := time.Now()
		observe("get_recent_orders", "started", started, in)
		response, callErr := client.GetRecentOrders(ctx, &businessv1.GetRecentOrdersRequest{UserId: userID, Limit: in.Limit})
		output, runErr := encode(response, callErr)
		observe("get_recent_orders", "completed", started, output)
		return output, runErr
	})
	if err != nil {
		return nil, err
	}
	search, err := utils.InferTool("search_companions", "查询满足当前硬条件的真实陪玩候选；preferred_tags 只计算匹配，不过滤或重排", func(ctx context.Context, in searchInput) (map[string]any, error) {
		in = applySearchDefaults(in, defaults)
		started := time.Now()
		observe("search_companions", "started", started, in)
		request := &businessv1.SearchCompanionsRequest{CompanionId: in.CompanionID, GameId: in.GameID, Gender: in.Gender, MaxPricePerHour: in.MaxPricePerHour, DurationMinutes: in.DurationMinutes, PreferredTags: in.PreferredTags, Limit: in.Limit}
		if in.StartTime != "" {
			parsed, parseErr := time.Parse(time.RFC3339, in.StartTime)
			if parseErr != nil {
				output := toolError("INVALID_ARGUMENT", "start_time 必须为 RFC3339")
				observe("search_companions", "completed", started, output)
				return output, nil
			}
			request.StartTime = timestamppb.New(parsed)
		}
		response, callErr := client.SearchCompanions(ctx, request)
		output, runErr := encodeSearch(response, callErr, request.StartTime != nil && request.DurationMinutes > 0)
		observe("search_companions", "completed", started, output)
		return output, runErr
	})
	if err != nil {
		return nil, err
	}
	draft, err := utils.InferTool("create_order_draft", "信息完整后创建真实订单草稿并返回 Preview 事实；不得自行提供价格或金额", func(ctx context.Context, in draftInput) (map[string]any, error) {
		in = applyDraftDefaults(in, defaults)
		started := time.Now()
		observe("create_order_draft", "started", started, in)
		parsed, parseErr := time.Parse(time.RFC3339, in.StartTime)
		if parseErr != nil {
			output := toolError("INVALID_ARGUMENT", "start_time 必须为 RFC3339")
			observe("create_order_draft", "completed", started, output)
			return output, nil
		}
		response, callErr := client.CreateOrderDraft(ctx, &businessv1.CreateOrderDraftRequest{UserId: userID, ConversationId: conversationID, CompanionId: in.CompanionID, GameId: in.GameID, StartTime: timestamppb.New(parsed), DurationMinutes: in.DurationMinutes})
		output, runErr := encode(response, callErr)
		observe("create_order_draft", "completed", started, output)
		return output, runErr
	})
	if err != nil {
		return nil, err
	}
	confirm, err := utils.InferTool("confirm_order", "仅当用户对当前 Preview 明确确认且未修改条件时确认订单", func(ctx context.Context, in confirmInput) (map[string]any, error) {
		started := time.Now()
		observe("confirm_order", "started", started, in)
		response, callErr := client.ConfirmOrder(ctx, &businessv1.ConfirmOrderRequest{UserId: userID, ConversationId: conversationID, DraftId: in.DraftID})
		output, runErr := encode(response, callErr)
		observe("confirm_order", "completed", started, output)
		return output, runErr
	})
	if err != nil {
		return nil, err
	}
	return []tool.BaseTool{recent, search, draft, confirm}, nil
}

func applySearchDefaults(in searchInput, defaults OrderDefaults) searchInput {
	if in.GameID == "" {
		in.GameID = defaults.GameID
	}
	if in.DurationMinutes == 0 {
		in.DurationMinutes = defaults.DurationMinutes
	}
	return in
}

func applyDraftDefaults(in draftInput, defaults OrderDefaults) draftInput {
	if in.GameID == "" {
		in.GameID = defaults.GameID
	}
	if in.DurationMinutes == 0 {
		in.DurationMinutes = defaults.DurationMinutes
	}
	return in
}
func encode(message proto.Message, err error) (map[string]any, error) {
	if err != nil {
		return toolError("BUSINESS_RPC_TIMEOUT", "业务服务暂时繁忙，请稍后重试"), nil
	}
	raw, err := protojson.Marshal(message)
	if err != nil {
		return nil, err
	}
	var output map[string]any
	if err := json.Unmarshal(raw, &output); err != nil {
		return nil, err
	}
	return output, nil
}
func toolError(code, message string) map[string]any {
	return map[string]any{"error": map[string]any{"code": code, "message": message, "retryable": code == "BUSINESS_RPC_TIMEOUT"}}
}

func encodeSearch(response *businessv1.SearchCompanionsResponse, err error, slotVerified bool) (map[string]any, error) {
	if err != nil {
		return toolError("BUSINESS_RPC_TIMEOUT", "业务服务暂时繁忙，请稍后重试"), nil
	}
	if value := response.GetError(); value != nil {
		return map[string]any{"error": map[string]any{"code": value.GetCode(), "message": value.GetMessage(), "retryable": value.GetRetryable()}}, nil
	}
	candidates := make([]map[string]any, 0, len(response.GetCandidates()))
	for _, value := range response.GetCandidates() {
		item := value.GetCompanion()
		candidates = append(candidates, map[string]any{
			"companion_id": item.GetCompanionId(), "name": item.GetName(), "gender": item.GetGender(),
			"game_ids": item.GetGameIds(), "styles": item.GetStyles(), "skill_level": item.GetSkillLevel(),
			"price_per_hour": item.GetPricePerHour(), "tags": item.GetTags(), "matched_tags": value.GetMatchedTags(),
			"tag_match_count": value.GetTagMatchCount(), "slot_verified": slotVerified,
		})
	}
	return map[string]any{"candidates": candidates}, nil
}
func ValidateControlledTags(tags []string) error {
	allowed := map[string]bool{"上分": true, "教学": true, "高段位": true, "指挥": true, "会聊天": true, "话多": true, "搞笑": true, "安静": true, "情绪价值": true, "不压力": true, "耐心": true, "声音甜": true, "御姐音": true, "少年音": true}
	for _, tag := range tags {
		if !allowed[tag] {
			return fmt.Errorf("未受控标签: %s", tag)
		}
	}
	return nil
}
