//go:build integration

package test

import (
	"context"
	"testing"
	"time"

	businessv1 "companion-agent/contracts/companion-business/gen"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestRealBusinessFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	conn, err := grpc.Dial("127.0.0.1:9090", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	health, err := grpc_health_v1.NewHealthClient(conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil || health.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("gRPC Health 失败: %v %#v", err, health)
	}
	client := businessv1.NewCompanionBusinessServiceClient(conn)
	profile, err := client.GetUserProfile(ctx, &businessv1.GetUserProfileRequest{UserId: "10001"})
	if err != nil || profile.GetError() != nil {
		t.Fatalf("用户画像查询失败: %v %#v", err, profile.GetError())
	}
	location, _ := time.LoadLocation("Asia/Shanghai")
	tomorrow := time.Now().In(location).AddDate(0, 0, 1)
	start := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 20, 0, 0, 0, location)
	search, err := client.SearchCompanions(ctx, &businessv1.SearchCompanionsRequest{CompanionId: "p001", GameId: "naraka", StartTime: timestamppb.New(start), DurationMinutes: 120, PreferredTags: profile.GetProfile().GetPreferredTags(), Limit: 5})
	if err != nil || search.GetError() != nil || len(search.GetCandidates()) != 1 {
		t.Fatalf("陪玩查询失败: %v %#v", err, search)
	}
	conversation := "integration-" + time.Now().Format("150405.000000")
	draft, err := client.CreateOrderDraft(ctx, &businessv1.CreateOrderDraftRequest{UserId: "10001", ConversationId: conversation, CompanionId: "p001", GameId: "naraka", StartTime: timestamppb.New(start), DurationMinutes: 120})
	if err != nil || draft.GetError() != nil {
		t.Fatalf("创建 Draft 失败: %v %#v", err, draft.GetError())
	}
	confirmed, err := client.ConfirmOrder(ctx, &businessv1.ConfirmOrderRequest{UserId: "10001", ConversationId: conversation, DraftId: draft.GetDraft().GetDraftId()})
	if err != nil || confirmed.GetError() != nil || confirmed.GetOrder().GetOrderId() == "" {
		t.Fatalf("Confirm 失败: %v %#v", err, confirmed.GetError())
	}
	again, err := client.ConfirmOrder(ctx, &businessv1.ConfirmOrderRequest{UserId: "10001", ConversationId: conversation, DraftId: draft.GetDraft().GetDraftId()})
	if err != nil || again.GetOrder().GetOrderId() != confirmed.GetOrder().GetOrderId() {
		t.Fatalf("重复 Confirm 不幂等: %v %#v", err, again)
	}
}
