package observability

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	lf "github.com/cloudwego/eino-ext/callbacks/langfuse/v2"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// ErrOrderPresentationReady 仅供 Runner 的成功条件中止识别，不代表业务失败。
var ErrOrderPresentationReady = errors.New("订单展示已就绪")

type DirectReturn struct{ Kind, Content string }

func (e *DirectReturn) Error() string { return ErrOrderPresentationReady.Error() }
func (e *DirectReturn) Unwrap() error { return ErrOrderPresentationReady }

// directReturn 只接受单一包装链；含其他失败的 errors.Join 不得被当作成功。
func directReturn(err error) *DirectReturn {
	for err != nil {
		if value, ok := err.(*DirectReturn); ok {
			return value
		}
		if _, joined := err.(interface{ Unwrap() []error }); joined {
			return nil
		}
		err = errors.Unwrap(err)
	}
	return nil
}

type callbackAdapter struct {
	*lf.CallbackHandler
	mask func(string) string
}
type safeError struct {
	text     string
	original error
}

func (e safeError) Error() string { return e.text }
func (e safeError) Unwrap() error { return e.original }
func (a *callbackAdapter) safe(err error) error {
	if err == nil {
		return nil
	}
	return safeError{a.mask(err.Error()), err}
}

func (a *callbackAdapter) OnError(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
	if stopped := directReturn(err); stopped != nil {
		trace.SpanFromContext(ctx).SetAttributes(attribute.String("langfuse.observation.metadata.direct_return", stopped.Kind))
		return a.CallbackHandler.OnEnd(ctx, info, stopped.Content)
	}
	return a.CallbackHandler.OnError(ctx, info, a.safe(err))
}

func (a *callbackAdapter) OnEnd(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
	if info != nil && info.Component == components.ComponentOfTool {
		a.markTool(ctx, output)
	}
	if events := adk.ConvAgentCallbackOutput(output); info != nil && info.Component == adk.ComponentOfAgent && events != nil && events.Events != nil {
		iter, generator := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
		go func() {
			defer generator.Close()
			for {
				event, ok := events.Events.Next()
				if !ok {
					return
				}
				if event == nil {
					continue
				}
				copied := *event
				if stopped := directReturn(event.Err); stopped != nil {
					copied.Err = nil
					copied.Output = &adk.AgentOutput{MessageOutput: &adk.MessageVariant{Role: schema.Assistant, Message: schema.AssistantMessage(stopped.Content, nil)}}
					trace.SpanFromContext(ctx).SetAttributes(attribute.String("langfuse.observation.metadata.direct_return", stopped.Kind))
				} else {
					copied.Err = a.safe(event.Err)
					if event.Output != nil && event.Output.MessageOutput != nil && event.Output.MessageOutput.IsStreaming {
						out := *event.Output
						message := *out.MessageOutput
						message.MessageStream = safeStream(message.MessageStream, a.safe, observationMessage)
						out.MessageOutput = &message
						copied.Output = &out
					}
				}
				generator.Send(&copied)
			}
		}()
		output = &adk.AgentCallbackOutput{Events: iter}
	}
	return a.CallbackHandler.OnEnd(ctx, info, observationOutput(info, output))
}

func (a *callbackAdapter) markTool(ctx context.Context, output callbacks.CallbackOutput) {
	value := tool.ConvCallbackOutput(output)
	if value == nil {
		return
	}
	var result struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(value.Response), &result) != nil || result.Error == nil || result.Error.Code == "" {
		return
	}
	code := a.mask(result.Error.Code)
	span := trace.SpanFromContext(ctx)
	level := "WARNING"
	if code == "BUSINESS_RPC_TIMEOUT" {
		level = "ERROR"
		span.SetStatus(codes.Error, code)
	}
	span.SetAttributes(attribute.String("langfuse.observation.level", level), attribute.String("langfuse.observation.status_message", code), attribute.String("langfuse.observation.metadata.error_code", code))
}

// safeStream 只消费框架分给观测的副本，保留业务流；下游关闭时及时释放源流。
func safeStream[T any](source *schema.StreamReader[T], sanitize func(error) error, transform func(T) T) *schema.StreamReader[T] {
	if source == nil {
		return nil
	}
	reader, writer := schema.Pipe[T](1)
	go func() {
		defer source.Close()
		defer writer.Close()
		for {
			value, err := source.Recv()
			if err == io.EOF {
				return
			}
			if directReturn(err) != nil {
				return
			}
			if err == nil && transform != nil {
				value = transform(value)
			}
			if writer.Send(value, sanitize(err)) || err != nil {
				return
			}
		}
	}()
	return reader
}
func (a *callbackAdapter) OnStartWithStreamInput(ctx context.Context, info *callbacks.RunInfo, input *schema.StreamReader[callbacks.CallbackInput]) context.Context {
	return a.CallbackHandler.OnStartWithStreamInput(ctx, info, safeStream(input, a.safe, nil))
}
func (a *callbackAdapter) OnEndWithStreamOutput(ctx context.Context, info *callbacks.RunInfo, output *schema.StreamReader[callbacks.CallbackOutput]) context.Context {
	inspect := func(value callbacks.CallbackOutput) callbacks.CallbackOutput {
		if info != nil && info.Component == components.ComponentOfTool {
			a.markTool(ctx, value)
		}
		return observationOutput(info, value)
	}
	return a.CallbackHandler.OnEndWithStreamOutput(ctx, info, safeStream(output, a.safe, inspect))
}

// observationMessage 只采集模型事实；不读取 ADK 并发写入的内部 Extra/消息 ID。
// 输入输出业务字段完整保留，避免对共享消息做整体浅拷贝。
func observationMessage(m *schema.Message) *schema.Message {
	if m == nil {
		return nil
	}
	return &schema.Message{Role: m.Role, Content: m.Content, Name: m.Name, ToolCalls: m.ToolCalls, ToolCallID: m.ToolCallID, ToolName: m.ToolName, ResponseMeta: m.ResponseMeta, ReasoningContent: m.ReasoningContent, MultiContent: m.MultiContent, UserInputMultiContent: m.UserInputMultiContent, AssistantGenMultiContent: m.AssistantGenMultiContent}
}
func observationOutput(info *callbacks.RunInfo, value callbacks.CallbackOutput) callbacks.CallbackOutput {
	if info != nil && info.Component == components.ComponentOfChatModel {
		if out := model.ConvCallbackOutput(value); out != nil {
			return &model.CallbackOutput{Message: observationMessage(out.Message), Config: out.Config, TokenUsage: out.TokenUsage}
		}
	}
	return observationValue(value)
}

// observationValue 为框架内部节点复制消息并丢弃内部 Extra。
// Eino 会在 Tool 流进入后续节点时补写消息 ID，观测端不能并发序列化同一个 map。
func observationValue(value callbacks.CallbackOutput) callbacks.CallbackOutput {
	switch typed := value.(type) {
	case *schema.Message:
		return observationMessage(typed)
	case []*schema.Message:
		copied := make([]*schema.Message, len(typed))
		for index, message := range typed {
			copied[index] = observationMessage(message)
		}
		return copied
	case []schema.Message:
		copied := make([]schema.Message, len(typed))
		for index := range typed {
			message := observationMessage(&typed[index])
			if message != nil {
				copied[index] = *message
			}
		}
		return copied
	default:
		return value
	}
}
