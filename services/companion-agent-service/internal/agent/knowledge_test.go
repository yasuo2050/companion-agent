package agent

import (
	"companion-agent/services/companion-agent-service/internal/identity"
	"context"
	"encoding/json"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type readonlyModel struct {
	steps           int
	input           []*schema.Message
	tools           []*schema.ToolInfo
	answer          string
	skipSearch      bool
	duplicateSearch bool
}

func (m *readonlyModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	m.tools = tools
	return m, nil
}
func (m *readonlyModel) Generate(_ context.Context, messages []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	m.steps++
	if m.steps == 1 {
		m.input = messages
		if !m.skipSearch {
			calls := []schema.ToolCall{{ID: "search", Function: schema.FunctionCall{Name: "search_knowledge", Arguments: `{"question":"退款比例"}`}}}
			if m.duplicateSearch {
				calls = append(calls, schema.ToolCall{ID: "duplicate", Function: schema.FunctionCall{Name: "search_knowledge", Arguments: `{"question":"退款比例"}`}})
			}
			return schema.AssistantMessage("", calls), nil
		}
	}
	return schema.AssistantMessage(m.answer, nil), nil
}
func (m *readonlyModel) Stream(ctx context.Context, messages []*schema.Message, options ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	message, err := m.Generate(ctx, messages, options...)
	return schema.StreamReaderFromArray([]*schema.Message{message}), err
}

func TestReadOnlyKnowledgeLoopNoWritesNoStaleEvidence(t *testing.T) {
	for _, tt := range []struct {
		name, status, answer, want string
		skip                       bool
	}{{"有来源", "ready", "模拟测试规则：退款90%[1]", "退款90%", false}, {"无引用", "ready", "退款100%", "未能提供可核验", false}, {"撤权", "no_accessible_sources", "退款100%", "没有可访问", false}, {"未检索", "ready", "退款100%[1]", "没有取得", true}, {"生成期间撤权", "ready", "退款90%[1]", "旧证据已失效", false}, {"复核不可用", "ready", "退款90%[1]", "无法复核来源", false}, {"并行重复检索", "ready", "退款90%[1]", "退款90%", false}} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path == "/api/v1/knowledge/validate" {
					if r.Header.Get("Authorization") != "Bearer unit-token" {
						t.Error("复核缺少令牌")
					}
					if tt.name == "复核不可用" {
						w.WriteHeader(503)
						return
					}
					json.NewEncoder(w).Encode(map[string]bool{"valid": tt.name != "生成期间撤权"})
					return
				}
				if r.URL.Path != "/api/v1/knowledge/retrieve" || r.Header.Get("Authorization") != "Bearer unit-token" {
					t.Error("路径或令牌错误")
				}
				response := map[string]any{"status": tt.status, "backend": "milvus", "query_id": "unit", "evidence": []map[string]any{}, "citations": []map[string]any{}}
				if tt.status == "ready" {
					response["evidence"] = []map[string]any{{"text": "模拟规则退款90%", "chunk_id": "c1"}}
					response["citations"] = []map[string]any{{"title": "规则", "segment_id": "c1", "version_id": "v1", "document_id": "d1", "index_revision": "r1", "url": "/api/v1/sources/v1/segments/c1?index_revision=r1"}}
				}
				_ = json.NewEncoder(w).Encode(response)
			}))
			defer srv.Close()
			m := &readonlyModel{answer: tt.answer, skipSearch: tt.skip, duplicateSearch: tt.name == "并行重复检索"}
			runner := NewRunner(m, nil, 4, "unit").WithKnowledgeReadOnly(true).WithKnowledgeURL(srv.URL)
			history := []*schema.Message{schema.UserMessage("以前退款多少"), schema.AssistantMessage("旧政策退款100%", nil), schema.ToolMessage("过期内部SOP", "stale"), schema.UserMessage("现在退款多少")}
			var deltas strings.Builder
			citations := 0
			_, err := runner.Run(identity.WithToken(context.Background(), "unit-token"), "request", "10001", "conversation", history, func(e Event) {
				if e.Type == "citations" {
					citations += len(e.Citations)
				}
				if e.Type == "delta" {
					deltas.WriteString(e.Content)
				}
			})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(deltas.String(), tt.want) || strings.Contains(deltas.String(), "100%") {
				t.Fatalf("输出门禁失效: %s", deltas.String())
			}
			for _, info := range m.tools {
				if info.Name != "search_knowledge" {
					t.Fatalf("只读注册了写工具: %s", info.Name)
				}
			}
			for _, msg := range m.input {
				if strings.Contains(msg.Content, "旧政策") || strings.Contains(msg.Content, "过期内部") {
					t.Fatal("旧证据进入本轮模型")
				}
			}
			wantCalls := 1
			if tt.status == "ready" && tt.name != "无引用" {
				wantCalls = 2
			}
			if tt.name == "有来源" || tt.name == "并行重复检索" {
				if citations != 1 {
					t.Fatal("缺少最终引用")
				}
			} else if citations != 0 {
				t.Fatal("无效答案暴露了引用")
			}
			if !tt.skip && calls != wantCalls {
				t.Fatalf("检索次数 %d", calls)
			}
		})
	}
}

func TestKnowledgeHistoryHasOneCurrentQuestion(t *testing.T) {
	history := []*schema.Message{schema.UserMessage("前三小时问题"), schema.AssistantMessage("旧助手答案", nil), schema.ToolMessage("旧私有证据", "old"), schema.UserMessage("恰好24小时退款多少")}
	input := knowledgeHistory(history)
	if len(input) != 1 || input[0].Role != schema.User || !strings.HasSuffix(input[0].Content, "本轮用户问题（只回答这一项）：\n恰好24小时退款多少") {
		t.Fatal("多个历史问题被当作未回答用户消息")
	}
	if strings.Contains(input[0].Content, "旧助手答案") || strings.Contains(input[0].Content, "旧私有证据") || history[3].Content != "恰好24小时退款多少" {
		t.Fatal("旧证据进入上下文或原会话被修改")
	}
}
