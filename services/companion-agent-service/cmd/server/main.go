package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"companion-agent/services/companion-agent-service/internal/agent"
	businessclient "companion-agent/services/companion-agent-service/internal/client/business"
	"companion-agent/services/companion-agent-service/internal/config"
	"companion-agent/services/companion-agent-service/internal/conversation"
	"companion-agent/services/companion-agent-service/internal/flowlog"
	"companion-agent/services/companion-agent-service/internal/handler"
	llmfactory "companion-agent/services/companion-agent-service/internal/llm"
	webassets "companion-agent/services/companion-agent-service/web"
)

func main() {
	cfg, err := config.Load(true)
	if err != nil {
		slog.Error("配置加载失败", "error", err)
		os.Exit(1)
	}
	flowFile, flowPath, err := flowlog.Configure(cfg.AgentFlowLogPath)
	if err != nil {
		slog.Error("对话流水日志初始化失败", "path", cfg.AgentFlowLogPath, "error", err)
		os.Exit(1)
	}
	defer flowFile.Close()
	slog.Info("对话流水日志已启用", "path", flowPath)
	ctx, cancel := context.WithTimeout(context.Background(), cfg.LLMTimeout)
	defer cancel()
	chatModel, err := llmfactory.NewChatModel(ctx, cfg)
	if err != nil {
		slog.Error("Ark 模型初始化失败", "error", err)
		os.Exit(1)
	}
	business, err := businessclient.New(cfg.BusinessGRPCAddr, 3*time.Second)
	if err != nil {
		slog.Error("业务客户端初始化失败", "error", err)
		os.Exit(1)
	}
	defer business.Close()
	conversations := conversation.New(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, cfg.RedisKeyPrefix, cfg.ConversationTTL)
	defer conversations.Close()
	if err := conversations.Ping(ctx); err != nil {
		slog.Error("Redis 连接失败", "error", err)
		os.Exit(1)
	}
	runner := agent.NewRunner(chatModel, business, cfg.MaxIterations, cfg.ArkModelID)
	api := handler.NewHTTP(runner, conversations, business)
	mux := http.NewServeMux()
	api.Register(mux)
	mux.Handle("GET /", http.FileServer(http.FS(webassets.Files)))
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		slog.Info("Companion Agent Service 已启动", "addr", cfg.HTTPAddr, "ark_model_id", cfg.ArkModelID)
		if serveErr := server.ListenAndServe(); serveErr != nil && serveErr != http.ErrServerClosed {
			slog.Error("HTTP 服务退出", "error", serveErr)
		}
	}()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	<-signals
	shutdown, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	_ = server.Shutdown(shutdown)
}
