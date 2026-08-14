package agent

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type directReturnModel struct {
	generateCalls int
}

func (m *directReturnModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	m.generateCalls++
	if m.generateCalls == 1 {
		return schema.AssistantMessage("", []schema.ToolCall{{
			ID: "call_create_draft",
			Function: schema.FunctionCall{
				Name:      "create_order_draft",
				Arguments: `{}`,
			},
		}}), nil
	}
	return schema.AssistantMessage("这段大模型回复不应被生成", nil), nil
}

func (m *directReturnModel) Stream(ctx context.Context, messages []*schema.Message, options ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	message, err := m.Generate(ctx, messages, options...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{message}), nil
}

func (m *directReturnModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}

func TestOrderPresentationStopsBeforeNextModelCall(t *testing.T) {
	draftTool, err := utils.InferTool("create_order_draft", "test", func(context.Context, struct{}) (map[string]any, error) {
		return map[string]any{"draft": map[string]any{"draftId": "d001"}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	chatModel := &directReturnModel{}
	agent, err := adk.NewChatModelAgent(context.Background(), &adk.ChatModelAgentConfig{
		Name:        "direct-return-test",
		Description: "test",
		Model:       chatModel,
		ToolsConfig: adk.ToolsConfig{ToolsNodeConfig: compose.ToolsNodeConfig{Tools: []tool.BaseTool{draftTool}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var presentation atomic.Pointer[orderPresentation]
	presentation.Store(&orderPresentation{Kind: "order_preview", Content: "订单预览"})
	iterator := agent.Run(context.Background(), &adk.AgentInput{Messages: []*schema.Message{schema.UserMessage("创建订单")}}, adk.WithAfterToolCallsHook(stopAfterOrderPresentation(&presentation)))
	var runErr error
	toolResultSeen := false
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			runErr = event.Err
		}
		if event.Output != nil && event.Output.MessageOutput != nil && event.Output.MessageOutput.Role == schema.Tool {
			toolResultSeen = true
		}
	}
	if !errors.Is(runErr, errOrderPresentationReady) {
		t.Fatalf("应该在工具完成后命中条件中止，实际错误: %v", runErr)
	}
	if chatModel.generateCalls != 1 {
		t.Fatalf("订单工具成功后不应再调用大模型，实际调用 %d 次", chatModel.generateCalls)
	}
	if !toolResultSeen {
		t.Fatal("直接返回前应该先保留 Tool Result 事件")
	}
}

func TestNoOrderPresentationContinuesAgentLoop(t *testing.T) {
	var presentation atomic.Pointer[orderPresentation]
	if err := stopAfterOrderPresentation(&presentation)(context.Background()); err != nil {
		t.Fatalf("没有成功订单展示时应继续 Agent Loop: %v", err)
	}
}
