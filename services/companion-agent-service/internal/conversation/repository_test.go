package conversation

import (
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestMessageRoundTripPreservesToolCallID(t *testing.T) {
	source := []*schema.Message{{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{ID: "call-1", Type: "function", Function: schema.FunctionCall{Name: "search_companions", Arguments: `{"game_id":"naraka"}`}}}}, {Role: schema.Tool, Content: `{"candidates":[]}`, ToolCallID: "call-1", ToolName: "search_companions"}}
	dtos := []MessageDTO{FromMessage(source[0]), FromMessage(source[1])}
	restored := ToMessages(dtos)
	if restored[0].ToolCalls[0].ID != "call-1" || restored[1].ToolCallID != "call-1" || restored[1].ToolName != "search_companions" {
		t.Fatalf("ToolCall 对应关系恢复失败: %#v", restored)
	}
}
