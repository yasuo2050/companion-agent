package agent

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type stubToolCallingModel struct {
	generateCalls  int
	streamCalls    int
	withToolsCalls int
}

func (m *stubToolCallingModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	m.generateCalls++
	return schema.AssistantMessage("ok", nil), nil
}

func (m *stubToolCallingModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	m.streamCalls++
	return schema.StreamReaderFromArray([]*schema.Message{schema.AssistantMessage("ok", nil)}), nil
}

func (m *stubToolCallingModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	m.withToolsCalls++
	return m, nil
}
func (m *stubToolCallingModel) IsCallbacksEnabled() bool { return true }
func (m *stubToolCallingModel) GetType() string          { return "Stub" }

func TestTracedModelCountsRealModelCallsAndPreservesBinding(t *testing.T) {
	delegate := &stubToolCallingModel{}
	var iteration atomic.Int64
	wrapped := newTracedModel(delegate, "r-test", "c-test", "u-test", "model-test", &iteration)
	if _, err := wrapped.Generate(context.Background(), []*schema.Message{schema.UserMessage("hello")}); err != nil {
		t.Fatal(err)
	}
	stream, err := wrapped.Stream(context.Background(), []*schema.Message{schema.UserMessage("hello")})
	if err != nil {
		t.Fatal(err)
	}
	stream.Close()
	bound, err := wrapped.WithTools(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bound.Generate(context.Background(), []*schema.Message{schema.UserMessage("hello")}); err != nil {
		t.Fatal(err)
	}
	if iteration.Load() != 3 || delegate.generateCalls != 2 || delegate.streamCalls != 1 || delegate.withToolsCalls != 1 {
		t.Fatalf("调用计数不符合预期: iteration=%d generate=%d stream=%d with_tools=%d", iteration.Load(), delegate.generateCalls, delegate.streamCalls, delegate.withToolsCalls)
	}
	for name, candidate := range map[string]model.ToolCallingChatModel{"未绑定": wrapped, "绑定工具后": bound} {
		if !components.IsCallbacksEnabled(candidate) {
			t.Fatalf("%s未透传 Callback 能力", name)
		}
		if got, ok := components.GetType(candidate); !ok || got != "Stub" {
			t.Fatalf("%s模型类型=%q, %v", name, got, ok)
		}
	}
}
