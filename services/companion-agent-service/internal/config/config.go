// Package config 负责读取 Companion Agent Service 的运行配置。
package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config 保存 Agent Service 的全部运行配置。
type Config struct {
	Langfuse                                                             LangfuseConfig
	ArkAPIKey, ArkModelID, ArkBaseURL, LLMProvider                       string
	LLMMaxTokens, LLMRetryTimes, MaxIterations                           int
	LLMTimeout                                                           time.Duration
	HTTPAddr, BusinessGRPCAddr, RedisAddr, RedisPassword, RedisKeyPrefix string
	AgentFlowLogPath                                                     string
	RedisDB                                                              int
	ConversationTTL                                                      time.Duration
	KnowledgeURL, KnowledgeJWTSecret                                     string
}

// Load 先从当前目录 .env 补充缺失值，再读取并校验配置。
func Load(requireLLM bool) (Config, error) {
	if err := loadDotEnv(".env"); err != nil {
		return Config{}, fmt.Errorf("加载 .env 失败: %w", err)
	}
	cfg := Config{ArkAPIKey: os.Getenv("ARK_API_KEY"), ArkModelID: os.Getenv("ARK_MODEL_ID"), ArkBaseURL: env("ARK_BASE_URL", "https://ark.cn-beijing.volces.com/api/v3"), LLMProvider: env("LLM_PROVIDER", "ark"), HTTPAddr: env("AGENT_HTTP_ADDR", ":8080"), BusinessGRPCAddr: env("BUSINESS_GRPC_ADDR", "127.0.0.1:9090"), RedisAddr: env("REDIS_ADDR", "127.0.0.1:6379"), RedisPassword: os.Getenv("REDIS_PASSWORD"), RedisKeyPrefix: env("REDIS_KEY_PREFIX", "companion-agent:mvp:"), AgentFlowLogPath: env("AGENT_FLOW_LOG_PATH", "logs/agent-flow.log")}
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
		return Config{}, fmt.Errorf("ARK_API_KEY 和 ARK_MODEL_ID 必须通过 .env 或环境变量提供")
	}
	cfg.Langfuse = loadLangfuse()
	cfg.KnowledgeURL = env("KNOWLEDGE_API_URL", "http://127.0.0.1:8082")
	cfg.KnowledgeJWTSecret = os.Getenv("KNOWLEDGE_JWT_SECRET")
	return cfg, nil
}

func loadDotEnv(path string) error {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, value, found := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !found || !validEnvKey(key) {
			return fmt.Errorf("第 %d 行格式无效", lineNumber)
		}
		value, err = parseDotEnvValue(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("第 %d 行格式无效: %w", lineNumber, err)
		}
		if _, exists := os.LookupEnv(key); !exists {
			if err := os.Setenv(key, value); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}

func validEnvKey(key string) bool {
	if key == "" {
		return false
	}
	for index, character := range key {
		if (character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z') || character == '_' || (index > 0 && character >= '0' && character <= '9') {
			continue
		}
		return false
	}
	return true
}

func parseDotEnvValue(value string) (string, error) {
	if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
		return value[1 : len(value)-1], nil
	}
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		return strconv.Unquote(value)
	}
	if index := strings.Index(value, " #"); index >= 0 {
		value = strings.TrimSpace(value[:index])
	}
	return value, nil
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
