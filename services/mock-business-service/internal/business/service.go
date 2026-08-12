package business

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Service 集中实现所有确定性业务规则。
type Service struct {
	store                                  Store
	draftTTL                               time.Duration
	minDuration, maxDuration, durationStep int32
	now                                    func() time.Time
	mu                                     sync.Mutex
}

// NewService 创建业务规则服务。
func NewService(store Store, draftTTL time.Duration, minDuration, maxDuration, durationStep int32) *Service {
	return &Service{store: store, draftTTL: draftTTL, minDuration: minDuration, maxDuration: maxDuration, durationStep: durationStep, now: time.Now}
}
func (s *Service) GetUserProfile(ctx context.Context, userID string) (UserProfile, error) {
	p, err := s.store.GetProfile(ctx, userID)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return UserProfile{}, biz("USER_NOT_FOUND", "用户画像不存在", false, nil)
	}
	return p, err
}
func (s *Service) GetRecentOrders(ctx context.Context, userID string, limit int32) ([]Order, error) {
	if limit <= 0 {
		limit = 1
	}
	if limit > 10 {
		limit = 10
	}
	return s.store.RecentOrders(ctx, userID, limit)
}

// SearchCompanions 查询满足明确硬条件的稳定候选集合。
func (s *Service) SearchCompanions(ctx context.Context, in SearchInput) ([]Candidate, error) {
	for _, tag := range in.PreferredTags {
		if !contains(ControlledTags, tag) {
			return nil, biz("INVALID_ARGUMENT", "preferred_tags 包含未受控标签", false, map[string]any{"tag": tag})
		}
	}
	if in.Limit <= 0 {
		in.Limit = 5
	}
	if in.Limit > 10 {
		in.Limit = 10
	}
	items, err := s.store.ListCompanions(ctx)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].CompanionID < items[j].CompanionID })
	result := make([]Candidate, 0, in.Limit)
	for _, item := range items {
		if item.Status != "active" || (in.CompanionID != "" && item.CompanionID != in.CompanionID) || (in.GameID != "" && !contains(item.GameIDs, in.GameID)) || (in.Gender != "" && item.Gender != in.Gender) || (in.MaxPricePerHour > 0 && item.PricePerHour > in.MaxPricePerHour) {
			continue
		}
		if !in.StartTime.IsZero() && in.DurationMinutes > 0 {
			ok, checkErr := s.slotAvailable(ctx, item, in.StartTime, in.StartTime.Add(time.Duration(in.DurationMinutes)*time.Minute))
			if checkErr != nil {
				return nil, checkErr
			}
			if !ok {
				continue
			}
		}
		matched := intersection(item.Tags, in.PreferredTags)
		result = append(result, Candidate{Companion: item, MatchedTags: matched, TagMatchCount: int32(len(matched))})
		if int32(len(result)) == in.Limit {
			break
		}
	}
	if len(result) == 0 {
		return nil, biz("NO_COMPANION_FOUND", "没有找到符合当前硬条件的陪玩", false, nil)
	}
	return result, nil
}

// CreateOrderDraft 创建真实订单草稿并淘汰同一会话的旧 active Draft。
func (s *Service) CreateOrderDraft(ctx context.Context, in CreateDraftInput) (Draft, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.validateDuration(in.DurationMinutes); err != nil {
		return Draft{}, err
	}
	if in.UserID == "" || in.ConversationID == "" || in.CompanionID == "" || in.GameID == "" || in.StartTime.IsZero() {
		return Draft{}, biz("INVALID_ARGUMENT", "创建订单草稿所需信息不完整", false, nil)
	}
	companion, err := s.store.GetCompanion(ctx, in.CompanionID)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return Draft{}, biz("NO_COMPANION_FOUND", "陪玩不存在", false, nil)
	}
	if err != nil {
		return Draft{}, err
	}
	if !contains(companion.GameIDs, in.GameID) {
		return Draft{}, biz("NO_COMPANION_FOUND", "该陪玩不支持所选游戏", false, nil)
	}
	end := in.StartTime.Add(time.Duration(in.DurationMinutes) * time.Minute)
	ok, err := s.slotAvailable(ctx, companion, in.StartTime, end)
	if err != nil {
		return Draft{}, err
	}
	if !ok {
		return Draft{}, biz("COMPANION_SLOT_UNAVAILABLE", "该陪玩当前时间段不可约", false, nil)
	}
	now := s.now()
	draft := Draft{DraftID: newID("d", now), UserID: in.UserID, ConversationID: in.ConversationID, CompanionID: companion.CompanionID, CompanionName: companion.Name, GameID: in.GameID, StartTime: in.StartTime, EndTime: end, DurationMinutes: in.DurationMinutes, PricePerHour: companion.PricePerHour, TotalAmount: companion.PricePerHour * int64(in.DurationMinutes) / 60, Status: DraftActive, ExpiresAt: now.Add(s.draftTTL), CreatedAt: now, UpdatedAt: now}
	if err := s.store.SupersedeActiveDrafts(ctx, in.UserID, in.ConversationID, now); err != nil {
		return Draft{}, err
	}
	if err := s.store.InsertDraft(ctx, draft); err != nil {
		return Draft{}, err
	}
	return draft, nil
}

// ConfirmOrder 重新校验 Draft、归属、TTL、档期、价格和金额，并保证幂等。
func (s *Service) ConfirmOrder(ctx context.Context, userID, conversationID, draftID string) (Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, err := s.store.GetOrderByDraft(ctx, draftID); err == nil {
		_ = s.store.SetDraftStatus(ctx, draftID, DraftConfirmed, s.now())
		return existing, nil
	} else if !errors.Is(err, mongo.ErrNoDocuments) {
		return Order{}, err
	}
	draft, err := s.store.GetDraft(ctx, draftID)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return Order{}, biz("DRAFT_NOT_FOUND", "订单草稿不存在", false, nil)
	}
	if err != nil {
		return Order{}, err
	}
	if draft.UserID != userID || draft.ConversationID != conversationID {
		return Order{}, biz("DRAFT_NOT_FOUND", "订单草稿不存在", false, nil)
	}
	if draft.Status != DraftActive {
		return Order{}, biz("DRAFT_NOT_ACTIVE", "订单草稿已失效", false, map[string]any{"status": draft.Status})
	}
	now := s.now()
	if !now.Before(draft.ExpiresAt) {
		_ = s.store.SetDraftStatus(ctx, draftID, DraftExpired, now)
		return Order{}, biz("DRAFT_EXPIRED", "订单草稿已过期，请重新确认", false, nil)
	}
	companion, err := s.store.GetCompanion(ctx, draft.CompanionID)
	if err != nil {
		return Order{}, err
	}
	ok, err := s.slotAvailable(ctx, companion, draft.StartTime, draft.EndTime)
	if err != nil {
		return Order{}, err
	}
	if !ok {
		return Order{}, biz("COMPANION_SLOT_UNAVAILABLE", "确认时该陪玩档期已不可用", false, nil)
	}
	currentTotal := companion.PricePerHour * int64(draft.DurationMinutes) / 60
	if companion.PricePerHour != draft.PricePerHour || currentTotal != draft.TotalAmount {
		_ = s.store.SetDraftStatus(ctx, draftID, DraftSuperseded, now)
		return Order{}, biz("PRICE_CHANGED", "陪玩价格已变化，需要按新价格重新确认", false, map[string]any{"current_price_per_hour": companion.PricePerHour, "current_total_amount": currentTotal})
	}
	order := Order{OrderID: newID("o", now), DraftID: draft.DraftID, UserID: draft.UserID, CompanionID: draft.CompanionID, CompanionName: companion.Name, GameID: draft.GameID, StartTime: draft.StartTime, EndTime: draft.EndTime, DurationMinutes: draft.DurationMinutes, PricePerHour: draft.PricePerHour, TotalAmount: draft.TotalAmount, Status: OrderConfirmed, Source: OrderSource, CreatedAt: now}
	if err := s.store.InsertOrder(ctx, order); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return s.store.GetOrderByDraft(ctx, draftID)
		}
		return Order{}, err
	}
	if err := s.store.SetDraftStatus(ctx, draftID, DraftConfirmed, now); err != nil {
		return order, nil
	}
	return order, nil
}

func (s *Service) validateDuration(value int32) error {
	if value < s.minDuration || value > s.maxDuration || value%s.durationStep != 0 {
		return biz("INVALID_DURATION", fmt.Sprintf("服务时长必须为 %d 到 %d 分钟，且按 %d 分钟递增", s.minDuration, s.maxDuration, s.durationStep), false, nil)
	}
	return nil
}
func (s *Service) slotAvailable(ctx context.Context, companion Companion, start, end time.Time) (bool, error) {
	covered := false
	for _, slot := range companion.AvailableSlots {
		if !start.Before(slot.StartAt) && !end.After(slot.EndAt) {
			covered = true
			break
		}
	}
	if !covered {
		return false, nil
	}
	overlap, err := s.store.HasConfirmedOverlap(ctx, companion.CompanionID, start, end)
	return !overlap, err
}
func biz(code, message string, retryable bool, details map[string]any) *BusinessError {
	return &BusinessError{Code: code, Message: message, Retryable: retryable, Details: details}
}
func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
func intersection(values, desired []string) []string {
	set := map[string]struct{}{}
	for _, value := range desired {
		set[value] = struct{}{}
	}
	out := []string{}
	for _, value := range values {
		if _, ok := set[value]; ok {
			out = append(out, value)
		}
	}
	return out
}
func newID(prefix string, now time.Time) string {
	random := make([]byte, 6)
	if _, err := rand.Read(random); err != nil {
		return fmt.Sprintf("%s%x", prefix, now.UnixNano())
	}
	return fmt.Sprintf("%s%x%s", prefix, now.UnixNano(), hex.EncodeToString(random))
}
