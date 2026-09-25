// Package handler 实现 Mock Business Service 的 gRPC 边界。
package handler

import (
	"context"
	"errors"
	"time"

	businessv1 "companion-agent/contracts/companion-business/gen"
	"companion-agent/services/mock-business-service/internal/business"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// GRPC 将 Proto 请求转换为业务输入，并隐藏内部技术错误。
type GRPC struct {
	businessv1.UnimplementedCompanionBusinessServiceServer
	service *business.Service
}

// NewGRPC 创建 gRPC Handler。
func NewGRPC(service *business.Service) *GRPC { return &GRPC{service: service} }

func (h *GRPC) GetUserProfile(ctx context.Context, req *businessv1.GetUserProfileRequest) (*businessv1.GetUserProfileResponse, error) {
	value, err := h.service.GetUserProfile(ctx, req.GetUserId())
	if err != nil {
		return &businessv1.GetUserProfileResponse{Error: toError(err)}, nil
	}
	return &businessv1.GetUserProfileResponse{Profile: toProfile(value)}, nil
}
func (h *GRPC) GetRecentOrders(ctx context.Context, req *businessv1.GetRecentOrdersRequest) (*businessv1.GetRecentOrdersResponse, error) {
	values, err := h.service.GetRecentOrders(ctx, req.GetUserId(), req.GetLimit())
	if err != nil {
		return &businessv1.GetRecentOrdersResponse{Error: toError(err)}, nil
	}
	out := make([]*businessv1.Order, 0, len(values))
	for _, v := range values {
		out = append(out, toOrder(v))
	}
	return &businessv1.GetRecentOrdersResponse{Orders: out}, nil
}
func (h *GRPC) GetOrder(ctx context.Context, req *businessv1.GetOrderRequest) (*businessv1.GetOrderResponse, error) {
	v, err := h.service.GetOrder(ctx, req.GetUserId(), req.GetOrderId())
	if err != nil {
		return &businessv1.GetOrderResponse{Error: toError(err)}, nil
	}
	return &businessv1.GetOrderResponse{Order: toOrder(v)}, nil
}
func (h *GRPC) SearchCompanions(ctx context.Context, req *businessv1.SearchCompanionsRequest) (*businessv1.SearchCompanionsResponse, error) {
	var startTime time.Time
	if req.GetStartTime() != nil {
		startTime = req.GetStartTime().AsTime()
	}
	values, err := h.service.SearchCompanions(ctx, business.SearchInput{CompanionID: req.GetCompanionId(), GameID: req.GetGameId(), Gender: req.GetGender(), MaxPricePerHour: req.GetMaxPricePerHour(), StartTime: startTime, DurationMinutes: req.GetDurationMinutes(), PreferredTags: req.GetPreferredTags(), Limit: req.GetLimit()})
	if err != nil {
		return &businessv1.SearchCompanionsResponse{Error: toError(err)}, nil
	}
	out := make([]*businessv1.CompanionCandidate, 0, len(values))
	for _, v := range values {
		out = append(out, &businessv1.CompanionCandidate{Companion: toCompanion(v.Companion), MatchedTags: v.MatchedTags, TagMatchCount: v.TagMatchCount})
	}
	return &businessv1.SearchCompanionsResponse{Candidates: out}, nil
}
func (h *GRPC) CreateOrderDraft(ctx context.Context, req *businessv1.CreateOrderDraftRequest) (*businessv1.CreateOrderDraftResponse, error) {
	if req.GetStartTime() == nil {
		return &businessv1.CreateOrderDraftResponse{Error: toError(&business.BusinessError{Code: "INVALID_ARGUMENT", Message: "开始时间不能为空"})}, nil
	}
	value, err := h.service.CreateOrderDraft(ctx, business.CreateDraftInput{UserID: req.GetUserId(), ConversationID: req.GetConversationId(), CompanionID: req.GetCompanionId(), GameID: req.GetGameId(), StartTime: req.GetStartTime().AsTime(), DurationMinutes: req.GetDurationMinutes()})
	if err != nil {
		return &businessv1.CreateOrderDraftResponse{Error: toError(err)}, nil
	}
	return &businessv1.CreateOrderDraftResponse{Draft: toDraft(value)}, nil
}
func (h *GRPC) ConfirmOrder(ctx context.Context, req *businessv1.ConfirmOrderRequest) (*businessv1.ConfirmOrderResponse, error) {
	value, err := h.service.ConfirmOrder(ctx, req.GetUserId(), req.GetConversationId(), req.GetDraftId())
	if err != nil {
		return &businessv1.ConfirmOrderResponse{Error: toError(err)}, nil
	}
	return &businessv1.ConfirmOrderResponse{Order: toOrder(value)}, nil
}

func toError(err error) *businessv1.BusinessError {
	var value *business.BusinessError
	if !errors.As(err, &value) {
		return &businessv1.BusinessError{Code: "INTERNAL", Message: "业务服务暂时不可用", Retryable: true}
	}
	details, _ := structpb.NewStruct(value.Details)
	return &businessv1.BusinessError{Code: value.Code, Message: value.Message, Retryable: value.Retryable, Details: details}
}
func toProfile(v business.UserProfile) *businessv1.UserProfile {
	return &businessv1.UserProfile{UserId: v.UserID, FavoriteGameIds: v.FavoriteGameIDs, PreferredGender: v.PreferredGender, PreferredStyles: v.PreferredStyles, PreferredTags: v.PreferredTags, PriceMin: v.PriceMin, PriceMax: v.PriceMax, UsualDurationMinutes: v.UsualDurationMinutes, UpdatedAt: timestamppb.New(v.UpdatedAt)}
}
func toCompanion(v business.Companion) *businessv1.Companion {
	slots := make([]*businessv1.AvailableSlot, 0, len(v.AvailableSlots))
	for _, s := range v.AvailableSlots {
		slots = append(slots, &businessv1.AvailableSlot{StartAt: timestamppb.New(s.StartAt), EndAt: timestamppb.New(s.EndAt)})
	}
	return &businessv1.Companion{CompanionId: v.CompanionID, Name: v.Name, Gender: v.Gender, GameIds: v.GameIDs, Styles: v.Styles, SkillLevel: v.SkillLevel, PricePerHour: v.PricePerHour, Tags: v.Tags, AvailableSlots: slots, Status: v.Status}
}
func toDraft(v business.Draft) *businessv1.OrderDraft {
	return &businessv1.OrderDraft{DraftId: v.DraftID, UserId: v.UserID, ConversationId: v.ConversationID, CompanionId: v.CompanionID, CompanionName: v.CompanionName, GameId: v.GameID, StartTime: timestamppb.New(v.StartTime), EndTime: timestamppb.New(v.EndTime), DurationMinutes: v.DurationMinutes, PricePerHour: v.PricePerHour, TotalAmount: v.TotalAmount, Status: v.Status, ExpiresAt: timestamppb.New(v.ExpiresAt), CreatedAt: timestamppb.New(v.CreatedAt), UpdatedAt: timestamppb.New(v.UpdatedAt)}
}
func toOrder(v business.Order) *businessv1.Order {
	return &businessv1.Order{OrderId: v.OrderID, DraftId: v.DraftID, UserId: v.UserID, CompanionId: v.CompanionID, CompanionName: v.CompanionName, GameId: v.GameID, StartTime: timestamppb.New(v.StartTime), EndTime: timestamppb.New(v.EndTime), DurationMinutes: v.DurationMinutes, PricePerHour: v.PricePerHour, TotalAmount: v.TotalAmount, Status: v.Status, Source: v.Source, CreatedAt: timestamppb.New(v.CreatedAt)}
}
