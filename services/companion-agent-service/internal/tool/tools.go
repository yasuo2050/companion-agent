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
	GameID          string   `json:"game_id,omitempty" jsonschema_description:"游戏 ID：naraka、lol 或 hok"`
	Gender          string   `json:"gender,omitempty" jsonschema_description:"只有用户明确要求时传 female 或 male"`
	MaxPricePerHour int64    `json:"max_price_per_hour,omitempty" jsonschema_description:"只有用户明确说以内时传，单位分"`
	StartTime       string   `json:"start_time,omitempty" jsonschema_description:"RFC3339 开始时间，Asia/Shanghai"`
	DurationMinutes int32    `json:"duration_minutes,omitempty" jsonschema_description:"时长分钟"`
	PreferredTags   []string `json:"preferred_tags,omitempty" jsonschema_description:"只能使用受控标签，属于软偏好"`
	Limit           int32    `json:"limit,omitempty" jsonschema_description:"默认 5，最大 10"`
}
type draftInput struct {
	CompanionID     string `json:"companion_id" jsonschema:"required" jsonschema_description:"真实候选陪玩 ID"`
	GameID          string `json:"game_id" jsonschema:"required" jsonschema_description:"游戏 ID"`
	StartTime       string `json:"start_time" jsonschema:"required" jsonschema_description:"RFC3339 开始时间"`
	DurationMinutes int32  `json:"duration_minutes" jsonschema:"required" jsonschema_description:"60 到 240 分钟，步长 30"`
}
type confirmInput struct {
	DraftID string `json:"draft_id" jsonschema:"required" jsonschema_description:"当前 Preview 对应的 active Draft ID"`
}

// Build 为单次请求创建带服务端身份上下文的工具集合。
func Build(client *businessclient.Client, userID, conversationID string) ([]tool.BaseTool, error) {
	recent, err := utils.InferTool("get_recent_orders", "查询当前用户真实历史订单；出现上次、之前、还是那个等指代时使用", func(ctx context.Context, in recentInput) (map[string]any, error) {
		response, callErr := client.GetRecentOrders(ctx, &businessv1.GetRecentOrdersRequest{UserId: userID, Limit: in.Limit})
		return encode(response, callErr)
	})
	if err != nil {
		return nil, err
	}
	search, err := utils.InferTool("search_companions", "查询满足当前硬条件的真实陪玩候选；preferred_tags 只计算匹配，不过滤或重排", func(ctx context.Context, in searchInput) (map[string]any, error) {
		request := &businessv1.SearchCompanionsRequest{CompanionId: in.CompanionID, GameId: in.GameID, Gender: in.Gender, MaxPricePerHour: in.MaxPricePerHour, DurationMinutes: in.DurationMinutes, PreferredTags: in.PreferredTags, Limit: in.Limit}
		if in.StartTime != "" {
			parsed, parseErr := time.Parse(time.RFC3339, in.StartTime)
			if parseErr != nil {
				return toolError("INVALID_ARGUMENT", "start_time 必须为 RFC3339"), nil
			}
			request.StartTime = timestamppb.New(parsed)
		}
		response, callErr := client.SearchCompanions(ctx, request)
		return encode(response, callErr)
	})
	if err != nil {
		return nil, err
	}
	draft, err := utils.InferTool("create_order_draft", "信息完整后创建真实订单草稿并返回 Preview 事实；不得自行提供价格或金额", func(ctx context.Context, in draftInput) (map[string]any, error) {
		parsed, parseErr := time.Parse(time.RFC3339, in.StartTime)
		if parseErr != nil {
			return toolError("INVALID_ARGUMENT", "start_time 必须为 RFC3339"), nil
		}
		response, callErr := client.CreateOrderDraft(ctx, &businessv1.CreateOrderDraftRequest{UserId: userID, ConversationId: conversationID, CompanionId: in.CompanionID, GameId: in.GameID, StartTime: timestamppb.New(parsed), DurationMinutes: in.DurationMinutes})
		return encode(response, callErr)
	})
	if err != nil {
		return nil, err
	}
	confirm, err := utils.InferTool("confirm_order", "仅当用户对当前 Preview 明确确认且未修改条件时确认订单", func(ctx context.Context, in confirmInput) (map[string]any, error) {
		response, callErr := client.ConfirmOrder(ctx, &businessv1.ConfirmOrderRequest{UserId: userID, ConversationId: conversationID, DraftId: in.DraftID})
		return encode(response, callErr)
	})
	if err != nil {
		return nil, err
	}
	return []tool.BaseTool{recent, search, draft, confirm}, nil
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
func ValidateControlledTags(tags []string) error {
	allowed := map[string]bool{"上分": true, "教学": true, "高段位": true, "指挥": true, "会聊天": true, "话多": true, "搞笑": true, "安静": true, "情绪价值": true, "不压力": true, "耐心": true, "声音甜": true, "御姐音": true, "少年音": true}
	for _, tag := range tags {
		if !allowed[tag] {
			return fmt.Errorf("未受控标签: %s", tag)
		}
	}
	return nil
}
