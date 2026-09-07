// Package observability 提供可关闭、失败不阻断业务的 Agent 观测。
package observability

import (
	"context"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"companion-agent/services/companion-agent-service/internal/config"
	lf "github.com/cloudwego/eino-ext/callbacks/langfuse/v2"
	"github.com/cloudwego/eino/adk"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type TurnInfo struct{ RequestID, UserID, ConversationID, Input, ModelID string }
type TurnResult struct{ Output, Status, ErrorCode string }
type Turn interface{ End(TurnResult) }
type TurnTracer interface {
	StartAgentTurn(context.Context, TurnInfo) (context.Context, Turn)
	Shutdown(context.Context) error
}

type Langfuse struct {
	handler *lf.CallbackHandler
	mask    func(string) string
}
type turnKey struct{}
type turn struct {
	once    sync.Once
	owner   *Langfuse
	ctx     context.Context
	adapter *callbackAdapter
}
type noop struct{}

func (noop) End(TurnResult) {}
func (noop) StartAgentTurn(ctx context.Context, _ TurnInfo) (context.Context, Turn) {
	return ctx, noop{}
}
func (noop) Shutdown(context.Context) error { return nil }
func Noop() TurnTracer                      { return noop{} }

// New 在观测初始化失败时降级；日志不包含原始配置或密钥。
func New(ctx context.Context, cfg config.LangfuseConfig, secrets ...string) TurnTracer {
	if !cfg.Enabled {
		return Noop()
	}
	u, err := url.Parse(cfg.Host)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || cfg.PublicKey == "" || cfg.SecretKey == "" {
		slog.Warn("Langfuse 配置无效，已降级为 Noop")
		return Noop()
	}
	mask := masker(append(secrets, cfg.PublicKey, cfg.SecretKey)...)
	h, err := lf.NewHandler(ctx, &lf.Config{Host: cfg.Host, PublicKey: cfg.PublicKey, SecretKey: cfg.SecretKey, ServiceName: cfg.ServiceName, Environment: cfg.Environment, Release: cfg.Release, CollapseAgentInternalSpans: cfg.CollapseAgentInternalSpans, SampleRate: 1, MaxQueueSize: 2048, MaxExportBatchSize: 128, BatchTimeout: time.Second, Timeout: 3 * time.Second, RetryConfig: &lf.RetryConfig{Enabled: true, InitialInterval: time.Second, MaxInterval: 3 * time.Second, MaxElapsedTime: 10 * time.Second}, MaskFunc: mask})
	if err != nil {
		slog.Warn("Langfuse 初始化失败，已降级为 Noop")
		return Noop()
	}
	return &Langfuse{handler: h, mask: mask}
}

// requestContext 保留 HTTP 取消语义，但由 Turn.End 负责根节点收尾，避免取消先于最终文本写入。
type requestContext struct {
	context.Context
	request context.Context
}

func (c requestContext) Deadline() (time.Time, bool) { return c.request.Deadline() }
func (c requestContext) Done() <-chan struct{}       { return c.request.Done() }
func (c requestContext) Err() error                  { return c.request.Err() }

func (l *Langfuse) StartAgentTurn(ctx context.Context, info TurnInfo) (context.Context, Turn) {
	root := l.handler.StartTrace(context.WithoutCancel(ctx), lf.WithName("companion-agent.turn"), lf.WithUserID(info.UserID), lf.WithSessionID(info.ConversationID), lf.WithInput(info.Input), lf.WithMetadata(map[string]string{"request_id": info.RequestID, "conversation_id": info.ConversationID, "model": info.ModelID}))
	t := &turn{owner: l, ctx: root, adapter: &callbackAdapter{CallbackHandler: l.handler, mask: l.mask}}
	return context.WithValue(requestContext{Context: root, request: ctx}, turnKey{}, t), t
}

func (t *turn) End(result TurnResult) {
	t.once.Do(func() {
		span := trace.SpanFromContext(t.ctx)
		span.SetAttributes(attribute.String("langfuse.observation.metadata.turn_status", result.Status))
		if result.ErrorCode != "" {
			span.SetAttributes(attribute.String("langfuse.observation.metadata.error_code", t.owner.mask(result.ErrorCode)))
		}
		if result.Status == "error" {
			span.SetStatus(codes.Error, t.owner.mask(result.ErrorCode))
			span.SetAttributes(attribute.String("langfuse.observation.level", "ERROR"), attribute.String("langfuse.observation.status_message", t.owner.mask(result.ErrorCode)))
		}
		if result.Status == "cancelled" {
			span.SetAttributes(attribute.String("langfuse.observation.status_message", "cancelled"))
		}
		t.owner.handler.EndTrace(t.ctx, result.Output)
	})
}

func AgentOptions(ctx context.Context) []adk.AgentRunOption {
	if t, ok := ctx.Value(turnKey{}).(*turn); ok {
		return []adk.AgentRunOption{adk.WithCallbacks(t.adapter)}
	}
	return nil
}
func TraceID(ctx context.Context) string {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return ""
	}
	return sc.TraceID().String()
}
func (l *Langfuse) Flush(ctx context.Context) error    { return l.handler.Flush(ctx) }
func (l *Langfuse) Shutdown(ctx context.Context) error { return l.handler.Shutdown(ctx) }
