package agent

import (
	"strings"
	"testing"
)

func TestRenderDraftTemplateUsesFixedYuanPresentation(t *testing.T) {
	value, ok := renderOrderToolResult("create_order_draft", map[string]any{"draft": map[string]any{
		"draftId": "d001", "companionId": "p006", "companionName": "糖糖", "gameId": "lol",
		"startTime": "2026-08-13T12:00:00Z", "endTime": "2026-08-13T14:00:00Z", "durationMinutes": 120,
		"pricePerHour": "8000", "totalAmount": "16000",
	}})
	if !ok {
		t.Fatal("应生成订单预览")
	}
	for _, expected := range []string{"订单预览：", "糖糖（p006）", "LOL", "2026-08-13 20:00 - 2026-08-13 22:00", "80元/小时", "160元", "d001"} {
		if !strings.Contains(value.Content, expected) {
			t.Fatalf("缺少字段 %q: %s", expected, value.Content)
		}
	}
	if strings.Contains(value.Content, "8000") || strings.Contains(value.Content, "16000") {
		t.Fatalf("不得展示内部分值: %s", value.Content)
	}
}

func TestRenderConfirmedOrderTemplate(t *testing.T) {
	value, ok := renderOrderToolResult("confirm_order", map[string]any{"order": map[string]any{
		"orderId": "o001", "companionId": "p010", "companionName": "星野", "gameId": "hok",
		"startTime": "2026-08-13T12:00:00Z", "endTime": "2026-08-13T14:00:00Z", "durationMinutes": 120,
		"pricePerHour": 8550, "totalAmount": 17100,
	}})
	if !ok || value.Kind != "order_confirmed" {
		t.Fatal("应生成已确认订单")
	}
	for _, expected := range []string{"订单已确认：", "o001", "星野（p010）", "王者荣耀", "85.50元/小时", "171元"} {
		if !strings.Contains(value.Content, expected) {
			t.Fatalf("缺少字段 %q: %s", expected, value.Content)
		}
	}
}

func TestRenderOrderToolResultRejectsErrorOrIncompletePayload(t *testing.T) {
	if _, ok := renderOrderToolResult("create_order_draft", map[string]any{"error": map[string]any{"code": "SLOT_UNAVAILABLE"}}); ok {
		t.Fatal("工具失败时不得生成成功模板")
	}
	if _, ok := renderOrderToolResult("search_companions", map[string]any{}); ok {
		t.Fatal("非订单工具不得生成订单模板")
	}
}
