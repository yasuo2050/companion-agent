// Package business 实现陪玩查询、订单草稿和确认下单规则。
package business

import (
	"context"
	"time"
)

const (
	DraftActive     = "active"
	DraftSuperseded = "superseded"
	DraftConfirmed  = "confirmed"
	DraftExpired    = "expired"
	OrderConfirmed  = "confirmed"
	OrderSource     = "agent_mvp"
)

// ControlledTags 是业务允许使用的完整陪玩标签字典。
var ControlledTags = []string{"上分", "教学", "高段位", "指挥", "会聊天", "话多", "搞笑", "安静", "情绪价值", "不压力", "耐心", "声音甜", "御姐音", "少年音"}

// BusinessError 是可安全返回给 Agent 的结构化业务错误。
type BusinessError struct {
	Code      string
	Message   string
	Retryable bool
	Details   map[string]any
}

func (e *BusinessError) Error() string { return e.Code + ": " + e.Message }

type UserProfile struct {
	UserID               string    `bson:"_id"`
	FavoriteGameIDs      []string  `bson:"favorite_game_ids"`
	PreferredGender      string    `bson:"preferred_gender"`
	PreferredStyles      []string  `bson:"preferred_styles"`
	PreferredTags        []string  `bson:"preferred_tags"`
	PriceMin             int64     `bson:"price_min"`
	PriceMax             int64     `bson:"price_max"`
	UsualDurationMinutes int32     `bson:"usual_duration_minutes"`
	UpdatedAt            time.Time `bson:"updated_at"`
}
type AvailableSlot struct {
	StartAt time.Time `bson:"start_at"`
	EndAt   time.Time `bson:"end_at"`
}
type Companion struct {
	CompanionID    string          `bson:"_id"`
	Name           string          `bson:"name"`
	Gender         string          `bson:"gender"`
	GameIDs        []string        `bson:"game_ids"`
	Styles         []string        `bson:"styles"`
	SkillLevel     string          `bson:"skill_level"`
	PricePerHour   int64           `bson:"price_per_hour"`
	Tags           []string        `bson:"tags"`
	AvailableSlots []AvailableSlot `bson:"available_slots"`
	Status         string          `bson:"status"`
}
type Draft struct {
	DraftID         string    `bson:"_id"`
	UserID          string    `bson:"user_id"`
	ConversationID  string    `bson:"conversation_id"`
	CompanionID     string    `bson:"companion_id"`
	CompanionName   string    `bson:"companion_name"`
	GameID          string    `bson:"game_id"`
	StartTime       time.Time `bson:"start_time"`
	EndTime         time.Time `bson:"end_time"`
	DurationMinutes int32     `bson:"duration_minutes"`
	PricePerHour    int64     `bson:"price_per_hour"`
	TotalAmount     int64     `bson:"total_amount"`
	Status          string    `bson:"status"`
	ExpiresAt       time.Time `bson:"expires_at"`
	CreatedAt       time.Time `bson:"created_at"`
	UpdatedAt       time.Time `bson:"updated_at"`
}
type Order struct {
	OrderID         string    `bson:"_id"`
	DraftID         string    `bson:"draft_id,omitempty"`
	UserID          string    `bson:"user_id"`
	CompanionID     string    `bson:"companion_id"`
	CompanionName   string    `bson:"companion_name_snapshot"`
	GameID          string    `bson:"game_id"`
	StartTime       time.Time `bson:"start_time"`
	EndTime         time.Time `bson:"end_time"`
	DurationMinutes int32     `bson:"duration_minutes"`
	PricePerHour    int64     `bson:"price_per_hour"`
	TotalAmount     int64     `bson:"total_amount"`
	Status          string    `bson:"status"`
	Source          string    `bson:"source"`
	CreatedAt       time.Time `bson:"created_at"`
}
type Candidate struct {
	Companion     Companion
	MatchedTags   []string
	TagMatchCount int32
}
type SearchInput struct {
	CompanionID, GameID, Gender string
	MaxPricePerHour             int64
	StartTime                   time.Time
	DurationMinutes             int32
	PreferredTags               []string
	Limit                       int32
}
type CreateDraftInput struct {
	UserID, ConversationID, CompanionID, GameID string
	StartTime                                   time.Time
	DurationMinutes                             int32
}

// Store 描述业务规则依赖的持久化操作。
type Store interface {
	GetProfile(context.Context, string) (UserProfile, error)
	RecentOrders(context.Context, string, int32) ([]Order, error)
	GetOrder(context.Context, string, string) (Order, error)
	ListCompanions(context.Context) ([]Companion, error)
	GetCompanion(context.Context, string) (Companion, error)
	HasConfirmedOverlap(context.Context, string, time.Time, time.Time) (bool, error)
	SupersedeActiveDrafts(context.Context, string, string, time.Time) error
	InsertDraft(context.Context, Draft) error
	GetDraft(context.Context, string) (Draft, error)
	SetDraftStatus(context.Context, string, string, time.Time) error
	GetOrderByDraft(context.Context, string) (Order, error)
	InsertOrder(context.Context, Order) error
}
