// Package business 封装 Agent Service 到 Mock Business Service 的 gRPC 调用。
package business

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	businessv1 "companion-agent/contracts/companion-business/gen"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	grpc_health_v1 "google.golang.org/grpc/health/grpc_health_v1"
)

// Client 提供带统一短超时的业务 RPC。
type Client struct {
	conn    *grpc.ClientConn
	api     businessv1.CompanionBusinessServiceClient
	health  grpc_health_v1.HealthClient
	timeout time.Duration
}

// New 创建业务 gRPC 客户端。
func New(addr string, timeout time.Duration) (*Client, error) {
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, api: businessv1.NewCompanionBusinessServiceClient(conn), health: grpc_health_v1.NewHealthClient(conn), timeout: timeout}, nil
}

// Close 关闭 gRPC 连接。
func (c *Client) Close() error { return c.conn.Close() }
func (c *Client) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, c.timeout)
}

// Ready 检查业务服务 Health。
func (c *Client) Ready(ctx context.Context) error {
	call, cancel := c.withTimeout(ctx)
	defer cancel()
	response, err := c.health.Check(call, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		return err
	}
	if response.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		return fmt.Errorf("业务服务未就绪")
	}
	return nil
}
func (c *Client) GetUserProfile(ctx context.Context, user string) (*businessv1.GetUserProfileResponse, error) {
	started := time.Now()
	defer func() {
		slog.Info("业务 RPC 完成", "rpc", "GetUserProfile", "rpc_duration_ms", time.Since(started).Milliseconds())
	}()
	call, cancel := c.withTimeout(ctx)
	defer cancel()
	return c.api.GetUserProfile(call, &businessv1.GetUserProfileRequest{UserId: user})
}
func (c *Client) GetRecentOrders(ctx context.Context, request *businessv1.GetRecentOrdersRequest) (*businessv1.GetRecentOrdersResponse, error) {
	started := time.Now()
	defer func() {
		slog.Info("业务 RPC 完成", "rpc", "GetRecentOrders", "rpc_duration_ms", time.Since(started).Milliseconds())
	}()
	call, cancel := c.withTimeout(ctx)
	defer cancel()
	return c.api.GetRecentOrders(call, request)
}
func (c *Client) SearchCompanions(ctx context.Context, request *businessv1.SearchCompanionsRequest) (*businessv1.SearchCompanionsResponse, error) {
	started := time.Now()
	defer func() {
		slog.Info("业务 RPC 完成", "rpc", "SearchCompanions", "rpc_duration_ms", time.Since(started).Milliseconds())
	}()
	call, cancel := c.withTimeout(ctx)
	defer cancel()
	return c.api.SearchCompanions(call, request)
}
func (c *Client) CreateOrderDraft(ctx context.Context, request *businessv1.CreateOrderDraftRequest) (*businessv1.CreateOrderDraftResponse, error) {
	started := time.Now()
	defer func() {
		slog.Info("业务 RPC 完成", "rpc", "CreateOrderDraft", "rpc_duration_ms", time.Since(started).Milliseconds())
	}()
	call, cancel := c.withTimeout(ctx)
	defer cancel()
	return c.api.CreateOrderDraft(call, request)
}
func (c *Client) ConfirmOrder(ctx context.Context, request *businessv1.ConfirmOrderRequest) (*businessv1.ConfirmOrderResponse, error) {
	started := time.Now()
	defer func() {
		slog.Info("业务 RPC 完成", "rpc", "ConfirmOrder", "rpc_duration_ms", time.Since(started).Milliseconds())
	}()
	call, cancel := c.withTimeout(ctx)
	defer cancel()
	return c.api.ConfirmOrder(call, request)
}
