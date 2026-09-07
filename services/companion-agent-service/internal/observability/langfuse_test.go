package observability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"companion-agent/services/companion-agent-service/internal/config"
	lf "github.com/cloudwego/eino-ext/callbacks/langfuse/v2"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func fixture(t *testing.T) (*Langfuse, *tracetest.InMemoryExporter) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	mask := masker("fake-secret-123")
	h, err := lf.NewHandler(context.Background(), &lf.Config{SpanExporter: exporter, ServiceName: "test", Environment: "local", Release: "test-sha", CollapseAgentInternalSpans: true, MaskFunc: mask})
	if err != nil {
		t.Fatal(err)
	}
	l := &Langfuse{handler: h, mask: mask}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = l.Shutdown(ctx)
	})
	return l, exporter
}
func flush(t *testing.T, l *Langfuse) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := l.Flush(ctx); err != nil {
		t.Fatal(err)
	}
}
func attrs(span tracetest.SpanStub) map[string]string {
	result := map[string]string{}
	for _, a := range span.Attributes {
		result[string(a.Key)] = a.Value.Emit()
	}
	return result
}

func TestTurnsIsolationCancellationAndIdempotence(t *testing.T) {
	l, exporter := fixture(t)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ctx, cancel := context.WithCancel(context.Background())
			ctx, tr := l.StartAgentTurn(ctx, TurnInfo{RequestID: fmt.Sprint(i), UserID: "user", ConversationID: "same-session", Input: "fake-secret-123"})
			if TraceID(ctx) == "" {
				t.Error("缺少 trace ID")
			}
			cancel()
			if ctx.Err() != context.Canceled {
				t.Error("丢失请求取消语义")
			}
			tr.End(TurnResult{Output: fmt.Sprint(i), Status: "cancelled"})
			tr.End(TurnResult{Output: "错误覆盖", Status: "error"})
		}(i)
	}
	wg.Wait()
	flush(t, l)
	spans := exporter.GetSpans()
	if len(spans) != 20 {
		t.Fatalf("根数量 %d", len(spans))
	}
	ids := map[string]bool{}
	for _, span := range spans {
		a := attrs(span)
		if a["langfuse.session.id"] != "same-session" || a["langfuse.observation.metadata.turn_status"] != "cancelled" || a["langfuse.observation.output"] != a["langfuse.trace.metadata.request_id"] {
			t.Fatalf("关联或结果不正确: %v", a)
		}
		ids[span.SpanContext.TraceID().String()] = true
		if span.Status.Code == codes.Error {
			t.Fatal("用户取消被标记失败")
		}
	}
	if len(ids) != 20 {
		t.Fatal("并发请求串链")
	}
	raw, _ := json.Marshal(spans)
	if strings.Contains(string(raw), "fake-secret-123") {
		t.Fatal("根输入未脱敏")
	}
}

func TestRootFailure(t *testing.T) {
	for _, code := range []string{"CONVERSATION_LOAD_FAILED", "CONVERSATION_SAVE_FAILED", "AGENT_RUN_FAILED", "REQUEST_TIMEOUT"} {
		t.Run(code, func(t *testing.T) {
			l, e := fixture(t)
			_, tr := l.StartAgentTurn(context.Background(), TurnInfo{})
			tr.End(TurnResult{Status: "error", ErrorCode: code, Output: "已经发送"})
			flush(t, l)
			s := e.GetSpans()[0]
			if s.Status.Code != codes.Error || attrs(s)["langfuse.observation.output"] != "已经发送" {
				t.Fatal("失败结果未保留")
			}
		})
	}
}

func TestToolOutcomesDoNotFailRecoveredTurn(t *testing.T) {
	for _, tc := range []struct{ output, level string }{{`{"candidates":[]}`, ""}, {`{"error":{"code":"PRICE_CHANGED"}}`, "WARNING"}, {`{"error":{"code":"BUSINESS_RPC_TIMEOUT"}}`, "ERROR"}} {
		t.Run(tc.level+tc.output, func(t *testing.T) {
			l, e := fixture(t)
			ctx, tr := l.StartAgentTurn(context.Background(), TurnInfo{})
			a := ctx.Value(turnKey{}).(*turn).adapter
			info := &callbacks.RunInfo{Name: "confirm_order", Component: components.ComponentOfTool}
			toolCtx := a.OnStart(ctx, info, `{}`)
			a.OnEnd(toolCtx, info, tc.output)
			tr.End(TurnResult{Status: "success", Output: "已恢复"})
			flush(t, l)
			for _, s := range e.GetSpans() {
				v := attrs(s)
				if s.Name == "companion-agent.turn" {
					if s.Status.Code == codes.Error {
						t.Fatal("已恢复的 Turn 被误报")
					}
				} else if v["langfuse.observation.level"] != tc.level {
					t.Fatalf("工具级别 %v", v)
				}
			}
		})
	}
}

type scriptedModel struct {
	calls  int
	result string
}

func (m *scriptedModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	m.calls++
	if m.calls == 1 {
		return schema.AssistantMessage("", []schema.ToolCall{{ID: "call-1", Type: "function", Function: schema.FunctionCall{Name: "create_order_draft", Arguments: `{}`}}}), nil
	}
	return schema.AssistantMessage("已恢复", nil), nil
}
func (m *scriptedModel) Stream(ctx context.Context, in []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	out, err := m.Generate(ctx, in, opts...)
	if err != nil {
		return nil, err
	}
	out.ResponseMeta = &schema.ResponseMeta{Usage: &schema.TokenUsage{PromptTokens: 10, CompletionTokens: 3, TotalTokens: 13}}
	return schema.StreamReaderFromArray([]*schema.Message{out}), nil
}
func (m *scriptedModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}

func TestAgentSuccessStopAndRecovery(t *testing.T) {
	for _, success := range []bool{true, false} {
		t.Run(fmt.Sprint(success), func(t *testing.T) {
			l, e := fixture(t)
			ctx, tr := l.StartAgentTurn(context.Background(), TurnInfo{UserID: "u", ConversationID: "c"})
			m := &scriptedModel{}
			businessTool, err := utils.InferTool("create_order_draft", "创建草稿", func(context.Context, struct{}) (string, error) {
				if success {
					return `{"draft":{"draftId":"d1"}}`, nil
				}
				return `{"error":{"code":"PRICE_CHANGED"}}`, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			instance, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{Name: "companion-order-agent", Description: "测试", Model: m, ToolsConfig: adk.ToolsConfig{ToolsNodeConfig: compose.ToolsNodeConfig{Tools: []tool.BaseTool{businessTool}}}})
			if err != nil {
				t.Fatal(err)
			}
			opts := append(AgentOptions(ctx), adk.WithAfterToolCallsHook(func(context.Context) error {
				if success {
					return &DirectReturn{Kind: "order_preview", Content: "固定预览"}
				}
				return nil
			}))
			execution := adk.NewRunner(ctx, adk.RunnerConfig{Agent: instance, EnableStreaming: true})
			iter := execution.Run(ctx, []*schema.Message{schema.UserMessage("下单")}, opts...)
			final := "已恢复"
			toolSeen := false
			for {
				event, ok := iter.Next()
				if !ok {
					break
				}
				if event.Err != nil {
					if directReturn(event.Err) == nil {
						t.Fatal(event.Err)
					}
					final = "固定预览"
				}
				if event.Output != nil && event.Output.MessageOutput != nil {
					v := event.Output.MessageOutput
					if v.Role == schema.Tool {
						toolSeen = true
					}
					if _, err := v.GetMessage(); err != nil {
						t.Fatal(err)
					}
				}
			}
			tr.End(TurnResult{Output: final, Status: "success"})
			flush(t, l)
			want := 2
			if success {
				want = 1
			}
			if m.calls != want || !toolSeen {
				t.Fatalf("模型次数 %d 工具结果 %v", m.calls, toolSeen)
			}
			generations, agents, roots := 0, 0, 0
			for _, s := range e.GetSpans() {
				a := attrs(s)
				if s.Status.Code == codes.Error {
					t.Fatalf("成功路径出现错误: %s %v", s.Name, a)
				}
				switch a["langfuse.observation.type"] {
				case "generation":
					generations++
				case "agent":
					agents++
				}
				if s.Name == "companion-agent.turn" {
					roots++
					if a["langfuse.observation.output"] != final {
						t.Fatal("根回复不一致")
					}
				}
			}
			if generations != want || agents != 1 || roots != 1 {
				t.Fatalf("Generation=%d Agent=%d Root=%d", generations, agents, roots)
			}
		})
	}
}

func TestMixedErrorNotSuppressedAndMasked(t *testing.T) {
	l, e := fixture(t)
	ctx, tr := l.StartAgentTurn(context.Background(), TurnInfo{})
	a := ctx.Value(turnKey{}).(*turn).adapter
	info := &callbacks.RunInfo{Name: "broken", Component: components.ComponentOfTool}
	child := a.OnStart(ctx, info, `{}`)
	mixed := errors.Join(&DirectReturn{Content: "成功"}, errors.New("API_KEY=fake-secret-123"))
	if directReturn(mixed) != nil {
		t.Fatal("混合错误被掩盖")
	}
	a.OnError(child, info, mixed)
	tr.End(TurnResult{Status: "error", ErrorCode: "AGENT_RUN_FAILED"})
	flush(t, l)
	raw, _ := json.Marshal(e.GetSpans())
	if strings.Contains(string(raw), "fake-secret-123") {
		t.Fatal("错误信息泄露密钥")
	}
	found := false
	for _, s := range e.GetSpans() {
		if s.Name == "broken" && s.Status.Code == codes.Error {
			found = true
		}
	}
	if !found {
		t.Fatal("真实失败没有标记")
	}
}

func TestNoopInvalidConfig(t *testing.T) {
	for _, cfg := range []config.LangfuseConfig{{}, {Enabled: true}, {Enabled: true, Host: "ftp://host", PublicKey: "p", SecretKey: "s"}} {
		l := New(context.Background(), cfg)
		ctx, tr := l.StartAgentTurn(context.Background(), TurnInfo{})
		tr.End(TurnResult{})
		if TraceID(ctx) != "" {
			t.Fatal("无效配置未降级")
		}
	}
}
func TestMasking(t *testing.T) {
	mask := masker("exact-secret")
	got := mask(`{"api_key":"exact-secret","Authorization":"Bearer abc.def","password":"other"}`)
	if strings.Contains(got, "exact-secret") || strings.Contains(got, "abc.def") || strings.Contains(got, "other") {
		t.Fatal(got)
	}
	if !json.Valid([]byte(got)) {
		t.Fatal("脱敏损坏 JSON")
	}
}

func TestExportFailureDoesNotBlockTurn(t *testing.T) {
	for _, slow := range []bool{false, true} {
		t.Run(fmt.Sprint(slow), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				if slow {
					time.Sleep(100 * time.Millisecond)
				}
				w.WriteHeader(401)
			}))
			defer server.Close()
			h, err := lf.NewHandler(context.Background(), &lf.Config{Host: server.URL, PublicKey: "fake", SecretKey: "fake", Timeout: 20 * time.Millisecond, RetryConfig: &lf.RetryConfig{Enabled: false}, MaxQueueSize: 2, MaxExportBatchSize: 1, BatchTimeout: time.Millisecond})
			if err != nil {
				t.Fatal(err)
			}
			l := &Langfuse{handler: h, mask: masker()}
			start := time.Now()
			for i := 0; i < 100; i++ {
				_, tr := l.StartAgentTurn(context.Background(), TurnInfo{})
				tr.End(TurnResult{Status: "success", Output: "ok"})
			}
			if time.Since(start) > time.Second {
				t.Fatal("导出阻塞请求")
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_ = l.Shutdown(ctx)
		})
	}
}
