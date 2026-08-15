package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"companion-agent/services/companion-agent-service/internal/conversation"
	"github.com/cloudwego/eino/schema"
)

func TestHistoryRequiresUserID(t *testing.T) {
	handler := NewHTTP(nil, nil, nil)
	mux := http.NewServeMux()
	handler.Register(mux)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/conversations/demo-001/messages", nil)
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d，期望 %d", response.Code, http.StatusBadRequest)
	}
}

func TestDisplayMessagesOnlyReturnsVisibleConversation(t *testing.T) {
	values := []conversation.MessageDTO{
		{Role: string(schema.User), Content: "帮我找个陪玩"},
		{
			Role:    string(schema.Assistant),
			Content: "这段工具调用前的中间文本不应展示",
			ToolCalls: []conversation.ToolCallDTO{{
				ToolCallID: "call-1",
				ToolName:   "search_companions",
			}},
		},
		{Role: string(schema.Tool), Content: `{"candidates":[]}`, ToolCallID: "call-1", ToolName: "search_companions"},
		{Role: string(schema.Assistant), Content: "为你找到了小雨"},
		{Role: string(schema.Assistant), Content: "   "},
	}

	got := displayMessages(values)
	if len(got) != 2 {
		t.Fatalf("可见历史数量 = %d，期望 2: %#v", len(got), got)
	}
	if got[0].Role != "user" || got[0].Content != "帮我找个陪玩" {
		t.Fatalf("用户消息不符合预期: %#v", got[0])
	}
	if got[1].Role != "assistant" || got[1].Content != "为你找到了小雨" {
		t.Fatalf("最终回复不符合预期: %#v", got[1])
	}
}

func TestDisplayMessagesReturnsEmptyArray(t *testing.T) {
	got := displayMessages(nil)
	if got == nil || len(got) != 0 {
		t.Fatalf("空历史应返回空数组: %#v", got)
	}
}
