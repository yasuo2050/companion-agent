// Package handler 实现 Agent HTTP、SSE 与健康检查边界。
package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"companion-agent/services/companion-agent-service/internal/agent"
	businessclient "companion-agent/services/companion-agent-service/internal/client/business"
	"companion-agent/services/companion-agent-service/internal/conversation"
	"companion-agent/services/companion-agent-service/internal/flowlog"
	"companion-agent/services/companion-agent-service/internal/observability"
	"github.com/cloudwego/eino/schema"
)

type ChatRequest struct {
	UserID         string `json:"user_id"`
	ConversationID string `json:"conversation_id"`
	Message        string `json:"message"`
}
type HistoryMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type HistoryResponse struct {
	UserID         string           `json:"user_id"`
	ConversationID string           `json:"conversation_id"`
	Messages       []HistoryMessage `json:"messages"`
}
type HTTP struct {
	tracer        observability.TurnTracer
	modelID       string
	runner        *agent.Runner
	conversations *conversation.Repository
	business      *businessclient.Client
	locks         sync.Map
}

func NewHTTP(runner *agent.Runner, conversations *conversation.Repository, business *businessclient.Client) *HTTP {
	return &HTTP{runner: runner, conversations: conversations, business: business, tracer: observability.Noop()}
}

// WithTracer 在服务启动时注入旁路观测，不改变已有 HTTP 接口。
func (h *HTTP) WithTracer(tracer observability.TurnTracer, modelID string) *HTTP {
	h.tracer = tracer
	h.modelID = modelID
	return h
}

func (h *HTTP) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("GET /readyz", h.ready)
	mux.HandleFunc("GET /api/v1/conversations/{conversation_id}/messages", h.history)
	mux.HandleFunc("POST /api/v1/agent/chat", h.chat)
}
func (h *HTTP) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
func (h *HTTP) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := h.conversations.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "dependency": "redis"})
		return
	}
	if err := h.business.Ready(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "dependency": "business"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
func (h *HTTP) history(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.URL.Query().Get("user_id"))
	conversationID := strings.TrimSpace(r.PathValue("conversation_id"))
	if userID == "" || conversationID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "INVALID_REQUEST", "message": "user_id 和 conversation_id 均不能为空"})
		return
	}

	lockValue, _ := h.locks.LoadOrStore(conversationID, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	value, err := h.conversations.Load(r.Context(), userID, conversationID)
	if err != nil {
		if errors.Is(err, conversation.ErrOwnership) {
			writeJSON(w, http.StatusForbidden, map[string]string{"code": "CONVERSATION_FORBIDDEN", "message": "该对话不属于当前用户"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"code": "CONVERSATION_LOAD_FAILED", "message": "对话加载失败"})
		return
	}

	writeJSON(w, http.StatusOK, HistoryResponse{
		UserID:         userID,
		ConversationID: conversationID,
		Messages:       displayMessages(value.Messages),
	})
}
func (h *HTTP) chat(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	var request ChatRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "INVALID_REQUEST", "message": "请求格式无效"})
		return
	}
	request.UserID = strings.TrimSpace(request.UserID)
	request.ConversationID = strings.TrimSpace(request.ConversationID)
	request.Message = strings.TrimSpace(request.Message)
	if request.UserID == "" || request.ConversationID == "" || request.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "INVALID_REQUEST", "message": "user_id、conversation_id 和 message 均不能为空"})
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"code": "STREAM_UNSUPPORTED"})
		return
	}
	lockValue, _ := h.locks.LoadOrStore(request.ConversationID, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	requestID := newRequestID()
	traceCtx, turn := h.tracer.StartAgentTurn(r.Context(), observability.TurnInfo{RequestID: requestID, UserID: request.UserID, ConversationID: request.ConversationID, Input: request.Message, ModelID: h.modelID})
	result := observability.TurnResult{Status: "error", ErrorCode: "TURN_INCOMPLETE"}
	var finalText strings.Builder
	defer func() {
		result.Output = finalText.String()
		if errors.Is(traceCtx.Err(), context.Canceled) {
			result.Status = "cancelled"
			result.ErrorCode = "REQUEST_CANCELLED"
		}
		if errors.Is(traceCtx.Err(), context.DeadlineExceeded) {
			result.Status = "error"
			result.ErrorCode = "REQUEST_TIMEOUT"
		}
		turn.End(result)
		flowlog.Info("观测请求关联", "flow_step", "turn_observed", "request_id", requestID, "trace_id", observability.TraceID(traceCtx), "status", result.Status, "code", result.ErrorCode)
	}()
	flowlog.Info("对话开始", "flow_step", "turn_started", "request_id", requestID, "conversation_id", request.ConversationID, "user_id", request.UserID, "user_message", request.Message)
	sendSSE(w, flusher, "turn_started", map[string]any{"request_id": requestID, "conversation_id": request.ConversationID})
	ttfe := time.Since(started)
	var ttts, ttft time.Duration
	value, err := h.conversations.Load(traceCtx, request.UserID, request.ConversationID)
	if err != nil {
		code := "CONVERSATION_LOAD_FAILED"
		message := "对话加载失败"
		if errors.Is(err, conversation.ErrOwnership) {
			code = "CONVERSATION_FORBIDDEN"
			message = "该对话不属于当前用户"
		}
		result.ErrorCode = code
		sendSSE(w, flusher, "error", map[string]any{"request_id": requestID, "code": code, "message": message})
		flowlog.Error("对话加载失败", "flow_step", "turn_failed", "request_id", requestID, "conversation_id", request.ConversationID, "user_id", request.UserID, "code", code, "error", err)
		return
	}
	history := conversation.ToMessages(value.Messages)
	userMessage := schema.UserMessage(request.Message)
	history = append(history, userMessage)
	firstDelta := false
	outputs, runErr := h.runner.Run(traceCtx, requestID, request.UserID, request.ConversationID, history, func(event agent.Event) {
		switch event.Type {
		case "status":
			if ttts == 0 {
				ttts = time.Since(started)
			}
			sendSSE(w, flusher, "status", map[string]any{"request_id": requestID, "tool_name": event.ToolName, "message": toolStatus(event.ToolName)})
		case "delta":
			if event.Content != "" {
				if ttft == 0 {
					ttft = time.Since(started)
				}
				firstDelta = true
				finalText.WriteString(event.Content)
				sendSSE(w, flusher, "delta", map[string]any{"request_id": requestID, "content": event.Content})
			}
		}
	})
	if runErr != nil {
		result.ErrorCode = agentErrorCode(runErr)
		slog.Error("Agent 运行失败", "request_id", requestID, "conversation_id", request.ConversationID, "user_id", request.UserID, "code", agentErrorCode(runErr), "error", runErr)
		flowlog.Error("Agent 运行失败", "flow_step", "turn_failed", "request_id", requestID, "conversation_id", request.ConversationID, "user_id", request.UserID, "code", agentErrorCode(runErr), "error", runErr)
		sendSSE(w, flusher, "error", map[string]any{"request_id": requestID, "code": agentErrorCode(runErr), "message": "Agent 暂时无法完成本次请求，请稍后重试"})
		return
	}
	if !firstDelta {
		for i := len(outputs) - 1; i >= 0; i-- {
			if outputs[i].Role == schema.Assistant && outputs[i].Content != "" {
				finalText.WriteString(outputs[i].Content)
				sendSSE(w, flusher, "delta", map[string]any{"request_id": requestID, "content": outputs[i].Content})
				break
			}
		}
	}
	value.Messages = append(value.Messages, conversation.FromMessage(userMessage))
	for _, message := range outputs {
		value.Messages = append(value.Messages, conversation.FromMessage(message))
	}
	if err := h.conversations.Save(traceCtx, value); err != nil {
		result.ErrorCode = "CONVERSATION_SAVE_FAILED"
		sendSSE(w, flusher, "error", map[string]any{"request_id": requestID, "code": "CONVERSATION_SAVE_FAILED", "message": "对话保存失败，请重试"})
		flowlog.Error("对话保存失败", "flow_step", "turn_failed", "request_id", requestID, "conversation_id", request.ConversationID, "user_id", request.UserID, "code", "CONVERSATION_SAVE_FAILED", "error", err)
		return
	}
	result.Status, result.ErrorCode = "success", ""
	sendSSE(w, flusher, "done", map[string]any{"request_id": requestID})
	slog.Info("Agent 延迟指标", "request_id", requestID, "conversation_id", request.ConversationID, "ttfe_ms", ttfe.Milliseconds(), "ttts_ms", durationMillis(ttts), "ttft_ms", durationMillis(ttft), "total_ms", time.Since(started).Milliseconds())
	flowlog.Info("对话结束", "flow_step", "turn_completed", "request_id", requestID, "conversation_id", request.ConversationID, "user_id", request.UserID, "total_ms", time.Since(started).Milliseconds())
}
func displayMessages(values []conversation.MessageDTO) []HistoryMessage {
	messages := make([]HistoryMessage, 0, len(values))
	for _, value := range values {
		content := strings.TrimSpace(value.Content)
		if content == "" {
			continue
		}
		if value.Role == string(schema.User) {
			messages = append(messages, HistoryMessage{Role: "user", Content: value.Content})
			continue
		}
		if value.Role == string(schema.Assistant) && len(value.ToolCalls) == 0 {
			messages = append(messages, HistoryMessage{Role: "assistant", Content: value.Content})
		}
	}
	return messages
}
func sendSSE(w http.ResponseWriter, flusher http.Flusher, event string, data any) {
	raw, _ := json.Marshal(data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, raw)
	flusher.Flush()
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func toolStatus(name string) string {
	values := map[string]string{"get_recent_orders": "正在查看你之前的订单…", "search_companions": "正在确认陪玩档期…", "create_order_draft": "正在为你创建订单预览…", "confirm_order": "正在确认订单…"}
	if value := values[name]; value != "" {
		return value
	}
	return "正在处理…"
}
func agentErrorCode(err error) string {
	if strings.Contains(err.Error(), "max iterations") {
		return "AGENT_MAX_ITERATIONS"
	}
	return "AGENT_RUN_FAILED"
}
func newRequestID() string {
	value := make([]byte, 8)
	_, _ = rand.Read(value)
	return "r" + hex.EncodeToString(value)
}
func durationMillis(value time.Duration) int64 {
	if value == 0 {
		return -1
	}
	return value.Milliseconds()
}
