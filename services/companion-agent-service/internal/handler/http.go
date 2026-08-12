// Package handler 实现 Agent HTTP、SSE 与健康检查边界。
package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"companion-agent/services/companion-agent-service/internal/agent"
	businessclient "companion-agent/services/companion-agent-service/internal/client/business"
	"companion-agent/services/companion-agent-service/internal/conversation"
	"github.com/cloudwego/eino/schema"
)

type ChatRequest struct {
	UserID         string `json:"user_id"`
	ConversationID string `json:"conversation_id"`
	Message        string `json:"message"`
}
type HTTP struct {
	runner        *agent.Runner
	conversations *conversation.Repository
	business      *businessclient.Client
	locks         sync.Map
}

func NewHTTP(runner *agent.Runner, conversations *conversation.Repository, business *businessclient.Client) *HTTP {
	return &HTTP{runner: runner, conversations: conversations, business: business}
}
func (h *HTTP) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("GET /readyz", h.ready)
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
func (h *HTTP) chat(w http.ResponseWriter, r *http.Request) {
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
	sendSSE(w, flusher, "turn_started", map[string]any{"request_id": requestID, "conversation_id": request.ConversationID})
	value, err := h.conversations.Load(r.Context(), request.UserID, request.ConversationID)
	if err != nil {
		code := "CONVERSATION_LOAD_FAILED"
		message := "对话加载失败"
		if errors.Is(err, conversation.ErrOwnership) {
			code = "CONVERSATION_FORBIDDEN"
			message = "该对话不属于当前用户"
		}
		sendSSE(w, flusher, "error", map[string]any{"request_id": requestID, "code": code, "message": message})
		return
	}
	history := conversation.ToMessages(value.Messages)
	userMessage := schema.UserMessage(request.Message)
	history = append(history, userMessage)
	firstDelta := false
	outputs, runErr := h.runner.Run(r.Context(), request.UserID, request.ConversationID, history, func(event agent.Event) {
		switch event.Type {
		case "status":
			sendSSE(w, flusher, "status", map[string]any{"request_id": requestID, "tool_name": event.ToolName, "message": toolStatus(event.ToolName)})
		case "delta":
			if event.Content != "" {
				firstDelta = true
				sendSSE(w, flusher, "delta", map[string]any{"request_id": requestID, "content": event.Content})
			}
		}
	})
	if runErr != nil {
		sendSSE(w, flusher, "error", map[string]any{"request_id": requestID, "code": agentErrorCode(runErr), "message": "Agent 暂时无法完成本次请求，请稍后重试"})
		return
	}
	if !firstDelta {
		for i := len(outputs) - 1; i >= 0; i-- {
			if outputs[i].Role == schema.Assistant && outputs[i].Content != "" {
				sendSSE(w, flusher, "delta", map[string]any{"request_id": requestID, "content": outputs[i].Content})
				break
			}
		}
	}
	value.Messages = append(value.Messages, conversation.FromMessage(userMessage))
	for _, message := range outputs {
		value.Messages = append(value.Messages, conversation.FromMessage(message))
	}
	if err := h.conversations.Save(r.Context(), value); err != nil {
		sendSSE(w, flusher, "error", map[string]any{"request_id": requestID, "code": "CONVERSATION_SAVE_FAILED", "message": "对话保存失败，请重试"})
		return
	}
	sendSSE(w, flusher, "done", map[string]any{"request_id": requestID})
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
	values := map[string]string{"get_recent_orders": "正在查看你之前的订单…", "search_companions": "正在确认陪玩档期…", "create_order_draft": "正在生成订单信息…", "confirm_order": "正在确认订单…"}
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
