// Package config 负责读取 Companion Agent Service 的运行配置。
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config 保存 Agent Service 的全部运行配置。
type Config struct {
	ArkAPIKey, ArkModelID, ArkBaseURL, LLMProvider, LLMModelAlias        string
	LLMMaxTokens, LLMRetryTimes, MaxIterations                           int
	LLMTimeout                                                           time.Duration
	HTTPAddr, BusinessGRPCAddr, RedisAddr, RedisPassword, RedisKeyPrefix string
	RedisDB                                                              int
	ConversationTTL                                                      time.Duration
}

// Load 从环境变量读取并校验配置。
func Load(requireLLM bool) (Config, error) {
	cfg := Config{ArkAPIKey: os.Getenv("ARK_API_KEY"), ArkModelID: os.Getenv("ARK_MODEL_ID"), ArkBaseURL: env("ARK_BASE_URL", "https://ark.cn-beijing.volces.com/api/v3"), LLMProvider: env("LLM_PROVIDER", "ark"), LLMModelAlias: env("LLM_MODEL_ALIAS", "doubao-seed-2.1-turbo"), HTTPAddr: env("AGENT_HTTP_ADDR", ":8080"), BusinessGRPCAddr: env("BUSINESS_GRPC_ADDR", "127.0.0.1:9090"), RedisAddr: env("REDIS_ADDR", "127.0.0.1:6379"), RedisPassword: os.Getenv("REDIS_PASSWORD"), RedisKeyPrefix: env("REDIS_KEY_PREFIX", "companion-agent:mvp:")}
	var err error
	if cfg.LLMMaxTokens, err = intValue("LLM_MAX_TOKENS", 2048); err != nil {
		return Config{}, err
	}
	if cfg.LLMRetryTimes, err = intValue("LLM_RETRY_TIMES", 1); err != nil {
		return Config{}, err
	}
	if cfg.MaxIterations, err = intValue("AGENT_MAX_ITERATIONS", 6); err != nil {
		return Config{}, err
	}
	if cfg.RedisDB, err = intValue("REDIS_DB", 0); err != nil {
		return Config{}, err
	}
	if cfg.LLMTimeout, err = duration("LLM_TIMEOUT", 30*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.ConversationTTL, err = duration("CONVERSATION_TTL", 24*time.Hour); err != nil {
		return Config{}, err
	}
	if cfg.LLMProvider != "ark" {
		return Config{}, fmt.Errorf("LLM_PROVIDER 仅支持 ark")
	}
	if requireLLM && (cfg.ArkAPIKey == "" || cfg.ArkModelID == "") {
		return Config{}, fmt.Errorf("ARK_API_KEY 和 ARK_MODEL_ID 必须通过环境变量提供")
	}
	return cfg, nil
}
func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
func intValue(key string, fallback int) (int, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s 配置无效: %w", key, err)
	}
	return parsed, nil
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
