// Package conversation 保存可完整恢复 ToolCall/ToolResult 的 Redis 对话。
package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/redis/go-redis/v9"
)

var ErrOwnership = errors.New("conversation_id 已属于其他用户")

type ToolCallDTO struct {
	ToolCallID string `json:"tool_call_id"`
	ToolName   string `json:"tool_name"`
	Arguments  string `json:"arguments"`
}
type MessageDTO struct {
	Role       string        `json:"role"`
	Content    string        `json:"content,omitempty"`
	ToolCalls  []ToolCallDTO `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
	ToolName   string        `json:"tool_name,omitempty"`
	CreatedAt  time.Time     `json:"created_at"`
}
type Conversation struct {
	ConversationID string       `json:"conversation_id"`
	UserID         string       `json:"user_id"`
	Messages       []MessageDTO `json:"messages"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}
type Repository struct {
	client *redis.Client
	prefix string
	ttl    time.Duration
}

func New(addr, password string, db int, prefix string, ttl time.Duration) *Repository {
	return &Repository{client: redis.NewClient(&redis.Options{Addr: addr, Password: password, DB: db}), prefix: prefix, ttl: ttl}
}
func (r *Repository) Close() error                   { return r.client.Close() }
func (r *Repository) Ping(ctx context.Context) error { return r.client.Ping(ctx).Err() }
func (r *Repository) Load(ctx context.Context, userID, conversationID string) (Conversation, error) {
	raw, err := r.client.Get(ctx, r.key(conversationID)).Bytes()
	if errors.Is(err, redis.Nil) {
		now := time.Now()
		return Conversation{ConversationID: conversationID, UserID: userID, Messages: []MessageDTO{}, CreatedAt: now, UpdatedAt: now}, nil
	}
	if err != nil {
		return Conversation{}, err
	}
	var value Conversation
	if err := json.Unmarshal(raw, &value); err != nil {
		return Conversation{}, err
	}
	if value.UserID != userID {
		return Conversation{}, ErrOwnership
	}
	return value, nil
}
func (r *Repository) Save(ctx context.Context, value Conversation) error {
	value.UpdatedAt = time.Now()
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, r.key(value.ConversationID), raw, r.ttl).Err()
}
func (r *Repository) key(id string) string { return r.prefix + "conversation:" + id }
func FromMessage(message *schema.Message) MessageDTO {
	calls := make([]ToolCallDTO, 0, len(message.ToolCalls))
	for _, call := range message.ToolCalls {
		calls = append(calls, ToolCallDTO{ToolCallID: call.ID, ToolName: call.Function.Name, Arguments: call.Function.Arguments})
	}
	return MessageDTO{Role: string(message.Role), Content: message.Content, ToolCalls: calls, ToolCallID: message.ToolCallID, ToolName: message.ToolName, CreatedAt: time.Now()}
}
func ToMessages(values []MessageDTO) []*schema.Message {
	out := make([]*schema.Message, 0, len(values))
	for _, value := range values {
		calls := make([]schema.ToolCall, 0, len(value.ToolCalls))
		for _, call := range value.ToolCalls {
			calls = append(calls, schema.ToolCall{ID: call.ToolCallID, Type: "function", Function: schema.FunctionCall{Name: call.ToolName, Arguments: call.Arguments}})
		}
		out = append(out, &schema.Message{Role: schema.RoleType(value.Role), Content: value.Content, ToolCalls: calls, ToolCallID: value.ToolCallID, ToolName: value.ToolName})
	}
	return out
}
