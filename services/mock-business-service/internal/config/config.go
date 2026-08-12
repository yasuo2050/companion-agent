// Package config 负责读取 Mock Business Service 的运行配置。
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config 保存业务服务的全部运行配置。
type Config struct {
	GRPCAddr       string
	MongoURI       string
	MongoDatabase  string
	DraftTTL       time.Duration
	MinDuration    int32
	MaxDuration    int32
	DurationStep   int32
	RedisAddr      string
	RedisPassword  string
	RedisDB        int
	RedisKeyPrefix string
}

// Load 从环境变量读取配置并应用安全的本地默认值。
func Load() (Config, error) {
	cfg := Config{
		GRPCAddr: env("BUSINESS_GRPC_ADDR", ":9090"), MongoURI: env("MONGO_URI", "mongodb://127.0.0.1:27017"),
		MongoDatabase: env("MONGO_DATABASE", "companion_agent_mvp"), RedisAddr: env("REDIS_ADDR", "127.0.0.1:6379"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"), RedisKeyPrefix: env("REDIS_KEY_PREFIX", "companion-agent:mvp:"),
	}
	var err error
	if cfg.DraftTTL, err = duration("DRAFT_TTL", 15*time.Minute); err != nil {
		return Config{}, err
	}
	if cfg.MinDuration, err = int32Value("MIN_DURATION_MINUTES", 60); err != nil {
		return Config{}, err
	}
	if cfg.MaxDuration, err = int32Value("MAX_DURATION_MINUTES", 240); err != nil {
		return Config{}, err
	}
	if cfg.DurationStep, err = int32Value("DURATION_STEP_MINUTES", 30); err != nil {
		return Config{}, err
	}
	db, err := strconv.Atoi(env("REDIS_DB", "0"))
	if err != nil {
		return Config{}, fmt.Errorf("REDIS_DB 配置无效: %w", err)
	}
	cfg.RedisDB = db
	if cfg.MongoDatabase == "" || cfg.MongoDatabase != "companion_agent_mvp" {
		return Config{}, fmt.Errorf("MONGO_DATABASE 必须为 companion_agent_mvp")
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
func duration(key string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s 配置无效: %w", key, err)
	}
	return parsed, nil
}
func int32Value(key string, fallback int32) (int32, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%s 配置无效: %w", key, err)
	}
	return int32(parsed), nil
}
