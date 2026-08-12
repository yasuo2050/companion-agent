package business

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

type memoryStore struct {
	profiles   map[string]UserProfile
	companions map[string]Companion
	drafts     map[string]Draft
	orders     map[string]Order
}

func newMemoryStore() *memoryStore {
	return &memoryStore{profiles: map[string]UserProfile{}, companions: map[string]Companion{}, drafts: map[string]Draft{}, orders: map[string]Order{}}
}
func (m *memoryStore) GetProfile(_ context.Context, id string) (UserProfile, error) {
	v, ok := m.profiles[id]
	if !ok {
		return UserProfile{}, mongo.ErrNoDocuments
	}
	return v, nil
}
func (m *memoryStore) RecentOrders(_ context.Context, user string, limit int32) ([]Order, error) {
	out := []Order{}
	for _, v := range m.orders {
		if v.UserID == user {
			out = append(out, v)
		}
	}
	if len(out) > int(limit) {
		out = out[:limit]
	}
	return out, nil
}
func (m *memoryStore) ListCompanions(context.Context) ([]Companion, error) {
	out := []Companion{}
	for _, v := range m.companions {
		out = append(out, v)
	}
	return out, nil
}
func (m *memoryStore) GetCompanion(_ context.Context, id string) (Companion, error) {
	v, ok := m.companions[id]
	if !ok {
		return Companion{}, mongo.ErrNoDocuments
	}
	return v, nil
}
func (m *memoryStore) HasConfirmedOverlap(_ context.Context, id string, start, end time.Time) (bool, error) {
	for _, v := range m.orders {
		if v.CompanionID == id && v.Status == OrderConfirmed && v.StartTime.Before(end) && v.EndTime.After(start) {
			return true, nil
		}
	}
	return false, nil
}
func (m *memoryStore) SupersedeActiveDrafts(_ context.Context, user, conversation string, now time.Time) error {
	for id, v := range m.drafts {
		if v.UserID == user && v.ConversationID == conversation && v.Status == DraftActive {
			v.Status = DraftSuperseded
			v.UpdatedAt = now
			m.drafts[id] = v
		}
	}
	return nil
}
func (m *memoryStore) InsertDraft(_ context.Context, v Draft) error {
	m.drafts[v.DraftID] = v
	return nil
}
func (m *memoryStore) GetDraft(_ context.Context, id string) (Draft, error) {
	v, ok := m.drafts[id]
	if !ok {
		return Draft{}, mongo.ErrNoDocuments
	}
	return v, nil
}
func (m *memoryStore) SetDraftStatus(_ context.Context, id, status string, now time.Time) error {
	v, ok := m.drafts[id]
	if !ok {
		return mongo.ErrNoDocuments
	}
	v.Status = status
	v.UpdatedAt = now
	m.drafts[id] = v
	return nil
}
func (m *memoryStore) GetOrderByDraft(_ context.Context, id string) (Order, error) {
	for _, v := range m.orders {
		if v.DraftID == id {
			return v, nil
		}
	}
	return Order{}, mongo.ErrNoDocuments
}
func (m *memoryStore) InsertOrder(_ context.Context, v Order) error {
	m.orders[v.OrderID] = v
	return nil
}

func fixture(t *testing.T) (*Service, *memoryStore, time.Time) {
	t.Helper()
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, location)
	start := time.Date(2026, 8, 13, 20, 0, 0, 0, location)
	store := newMemoryStore()
	store.companions["p001"] = Companion{CompanionID: "p001", Name: "小雨", Gender: "female", GameIDs: []string{"naraka"}, PricePerHour: 6800, Tags: []string{"会聊天", "不压力"}, Status: "active", AvailableSlots: []AvailableSlot{{StartAt: start.Add(-12 * time.Hour), EndAt: start.Add(4 * time.Hour)}}}
	service := NewService(store, 15*time.Minute, 60, 240, 30)
	service.now = func() time.Time { return now }
	return service, store, start
}
func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	value, ok := err.(*BusinessError)
	if !ok || value.Code != code {
		t.Fatalf("期望错误 %s，实际 %#v", code, err)
	}
}

func TestSearchCompanionsKeepsStableCandidates(t *testing.T) {
	service, store, start := fixture(t)
	store.companions["p002"] = Companion{CompanionID: "p002", Name: "阿杰", Gender: "male", GameIDs: []string{"naraka"}, PricePerHour: 7500, Tags: []string{"上分"}, Status: "active", AvailableSlots: store.companions["p001"].AvailableSlots}
	first, err := service.SearchCompanions(context.Background(), SearchInput{GameID: "naraka", StartTime: start, DurationMinutes: 120, PreferredTags: []string{"会聊天"}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.SearchCompanions(context.Background(), SearchInput{GameID: "naraka", StartTime: start, DurationMinutes: 120, PreferredTags: []string{"上分"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 || len(second) != 2 || first[0].Companion.CompanionID != second[0].Companion.CompanionID || first[1].Companion.CompanionID != second[1].Companion.CompanionID {
		t.Fatalf("软标签改变了候选集合或顺序")
	}
	if first[0].TagMatchCount != 1 || second[1].TagMatchCount != 1 {
		t.Fatalf("标签匹配统计错误")
	}
}

func TestCreateDraftSupersedesPrevious(t *testing.T) {
	service, store, start := fixture(t)
	first, err := service.CreateOrderDraft(context.Background(), CreateDraftInput{UserID: "10001", ConversationID: "c1", CompanionID: "p001", GameID: "naraka", StartTime: start, DurationMinutes: 120})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.CreateOrderDraft(context.Background(), CreateDraftInput{UserID: "10001", ConversationID: "c1", CompanionID: "p001", GameID: "naraka", StartTime: start.Add(2 * time.Hour), DurationMinutes: 120})
	if err != nil {
		t.Fatal(err)
	}
	if store.drafts[first.DraftID].Status != DraftSuperseded || store.drafts[second.DraftID].Status != DraftActive {
		t.Fatalf("Draft 状态迁移错误")
	}
	if second.TotalAmount != 13600 {
		t.Fatalf("金额错误: %d", second.TotalAmount)
	}
}

func TestConfirmOrderIdempotentAndOccupiesSlot(t *testing.T) {
	service, store, start := fixture(t)
	one, err := service.CreateOrderDraft(context.Background(), CreateDraftInput{UserID: "10001", ConversationID: "c1", CompanionID: "p001", GameID: "naraka", StartTime: start, DurationMinutes: 120})
	if err != nil {
		t.Fatal(err)
	}
	two, err := service.CreateOrderDraft(context.Background(), CreateDraftInput{UserID: "10002", ConversationID: "c2", CompanionID: "p001", GameID: "naraka", StartTime: start, DurationMinutes: 120})
	if err != nil {
		t.Fatal(err)
	}
	order, err := service.ConfirmOrder(context.Background(), "10001", "c1", one.DraftID)
	if err != nil {
		t.Fatal(err)
	}
	again, err := service.ConfirmOrder(context.Background(), "10001", "c1", one.DraftID)
	if err != nil || again.OrderID != order.OrderID {
		t.Fatalf("重复确认不幂等: %#v %v", again, err)
	}
	_, err = service.ConfirmOrder(context.Background(), "10002", "c2", two.DraftID)
	requireCode(t, err, "COMPANION_SLOT_UNAVAILABLE")
	if len(store.orders) != 1 {
		t.Fatalf("应只有一笔订单，实际 %d", len(store.orders))
	}
}

func TestConfirmRejectsExpiredAndPriceChanged(t *testing.T) {
	service, store, start := fixture(t)
	draft, err := service.CreateOrderDraft(context.Background(), CreateDraftInput{UserID: "10001", ConversationID: "c1", CompanionID: "p001", GameID: "naraka", StartTime: start, DurationMinutes: 120})
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return draft.ExpiresAt }
	_, err = service.ConfirmOrder(context.Background(), "10001", "c1", draft.DraftID)
	requireCode(t, err, "DRAFT_EXPIRED")
	service, store, start = fixture(t)
	draft, err = service.CreateOrderDraft(context.Background(), CreateDraftInput{UserID: "10001", ConversationID: "c1", CompanionID: "p001", GameID: "naraka", StartTime: start, DurationMinutes: 120})
	if err != nil {
		t.Fatal(err)
	}
	item := store.companions["p001"]
	item.PricePerHour = 7500
	store.companions["p001"] = item
	_, err = service.ConfirmOrder(context.Background(), "10001", "c1", draft.DraftID)
	requireCode(t, err, "PRICE_CHANGED")
	if store.drafts[draft.DraftID].Status != DraftSuperseded {
		t.Fatal("变价后旧 Draft 未废弃")
	}
}

func TestInvalidDurationAndTag(t *testing.T) {
	service, _, start := fixture(t)
	_, err := service.CreateOrderDraft(context.Background(), CreateDraftInput{UserID: "10001", ConversationID: "c1", CompanionID: "p001", GameID: "naraka", StartTime: start, DurationMinutes: 45})
	requireCode(t, err, "INVALID_DURATION")
	_, err = service.SearchCompanions(context.Background(), SearchInput{PreferredTags: []string{"自创标签"}})
	requireCode(t, err, "INVALID_ARGUMENT")
}
