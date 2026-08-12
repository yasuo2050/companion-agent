package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	businessv1 "companion-agent/contracts/companion-business/gen"
	"companion-agent/services/mock-business-service/internal/business"
	"companion-agent/services/mock-business-service/internal/config"
	"companion-agent/services/mock-business-service/internal/handler"
	mongorepo "companion-agent/services/mock-business-service/internal/repository/mongo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	grpc_health_v1 "google.golang.org/grpc/health/grpc_health_v1"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("配置加载失败", "error", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	repo, err := mongorepo.New(ctx, cfg.MongoURI, cfg.MongoDatabase)
	cancel()
	if err != nil {
		slog.Error("MongoDB 连接失败", "error", err)
		os.Exit(1)
	}
	defer repo.Close(context.Background())
	if err := repo.EnsureIndexes(context.Background()); err != nil {
		slog.Error("MongoDB 索引初始化失败", "error", err)
		os.Exit(1)
	}
	listener, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		slog.Error("gRPC 监听失败", "error", err)
		os.Exit(1)
	}
	server := grpc.NewServer()
	businessv1.RegisterCompanionBusinessServiceServer(server, handler.NewGRPC(business.NewService(repo, cfg.DraftTTL, cfg.MinDuration, cfg.MaxDuration, cfg.DurationStep)))
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus(businessv1.CompanionBusinessService_ServiceDesc.ServiceName, grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(server, healthServer)
	go func() {
		slog.Info("Mock Business Service 已启动", "addr", cfg.GRPCAddr)
		if serveErr := server.Serve(listener); serveErr != nil {
			slog.Error("gRPC 服务退出", "error", serveErr)
		}
	}()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	<-signals
	healthServer.Shutdown()
	server.GracefulStop()
}
