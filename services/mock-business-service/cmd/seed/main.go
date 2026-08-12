package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"companion-agent/services/mock-business-service/internal/config"
	mongorepo "companion-agent/services/mock-business-service/internal/repository/mongo"
	"companion-agent/services/mock-business-service/internal/seed"
	"github.com/redis/go-redis/v9"
)

func main() {
	reset := flag.Bool("reset", false, "只重置 companion_agent_mvp 数据与指定 Redis 前缀")
	flag.Parse()
	cfg, err := config.Load()
	if err != nil {
		slog.Error("配置加载失败", "error", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	repo, err := mongorepo.New(ctx, cfg.MongoURI, cfg.MongoDatabase)
	if err != nil {
		slog.Error("MongoDB 连接失败", "error", err)
		os.Exit(1)
	}
	defer repo.Close(context.Background())
	if *reset {
		if err := repo.Reset(ctx); err != nil {
			slog.Error("MongoDB 重置失败", "error", err)
			os.Exit(1)
		}
		if err := clearRedisPrefix(ctx, cfg); err != nil {
			slog.Error("Redis 会话重置失败", "error", err)
			os.Exit(1)
		}
	}
	profiles, companions, orders := seed.Data(time.Now())
	if err := repo.Seed(ctx, profiles, companions, orders); err != nil {
		slog.Error("Seed 写入失败", "error", err)
		os.Exit(1)
	}
	fmt.Printf("Seed 完成：用户 %d，陪玩 %d，历史订单 %d\n", len(profiles), len(companions), len(orders))
}
func clearRedisPrefix(ctx context.Context, cfg config.Config) error {
	client := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPassword, DB: cfg.RedisDB})
	defer client.Close()
	var cursor uint64
	for {
		keys, next, err := client.Scan(ctx, cursor, cfg.RedisKeyPrefix+"*", 100).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			if err := client.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}
		cursor = next
		if cursor == 0 {
			return nil
		}
	}
}
