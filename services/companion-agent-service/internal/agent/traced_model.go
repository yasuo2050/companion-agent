package agent

import (
	"context"
	"sync/atomic"

	"companion-agent/services/companion-agent-service/internal/flowlog"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type tracedModel struct {
	delegate       model.ToolCallingChatModel
	requestID      string
	conversationID string
	userID         string
	modelAlias     string
	iteration      *atomic.Int64
}

func newTracedModel(delegate model.ToolCallingChatModel, requestID, conversationID, userID, modelAlias string, iteration *atomic.Int64) model.ToolCallingChatModel {
	return &tracedModel{delegate: delegate, requestID: requestID, conversationID: conversationID, userID: userID, modelAlias: modelAlias, iteration: iteration}
}

func (m *tracedModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	iteration := m.logStart(input, "generate")
	output, err := m.delegate.Generate(ctx, input, opts...)
	if err != nil {
		m.logError(iteration, err)
	}
	return output, err
}

func (m *tracedModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	iteration := m.logStart(input, "stream")
	output, err := m.delegate.Stream(ctx, input, opts...)
	if err != nil {
		m.logError(iteration, err)
	}
	return output, err
}

func (m *tracedModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	bound, err := m.delegate.WithTools(tools)
	if err != nil {
		return nil, err
	}
	return &tracedModel{delegate: bound, requestID: m.requestID, conversationID: m.conversationID, userID: m.userID, modelAlias: m.modelAlias, iteration: m.iteration}, nil
}

func (m *tracedModel) logStart(input []*schema.Message, mode string) int64 {
	iteration := m.iteration.Add(1)
	triggerRole := "unknown"
	if len(input) > 0 && input[len(input)-1] != nil {
		triggerRole = string(input[len(input)-1].Role)
	}
	flowlog.Info("调用大模型", "flow_step", "llm_started", "request_id", m.requestID, "conversation_id", m.conversationID, "user_id", m.userID, "provider", "volcengine-ark", "model", m.modelAlias, "llm_iteration", iteration, "mode", mode, "message_count", len(input), "trigger_role", triggerRole)
	return iteration
}

func (m *tracedModel) logError(iteration int64, err error) {
	flowlog.Error("大模型调用失败", "flow_step", "llm_failed", "request_id", m.requestID, "conversation_id", m.conversationID, "user_id", m.userID, "llm_iteration", iteration, "error", err)
}
