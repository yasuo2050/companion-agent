package config

import (
	"log/slog"
	"os"
	"strconv"
)

// LangfuseConfig 只控制旁路观测，配置错误不阻止业务启动。
type LangfuseConfig struct {
	Enabled                                                       bool
	Host, PublicKey, SecretKey, ServiceName, Environment, Release string
	CollapseAgentInternalSpans                                    bool
}

func loadLangfuse() LangfuseConfig {
	c := LangfuseConfig{Host: env("LANGFUSE_HOST", "http://127.0.0.1:3000"), ServiceName: env("LANGFUSE_SERVICE_NAME", "companion-agent-service"), Environment: env("LANGFUSE_ENVIRONMENT", "local"), Release: env("LANGFUSE_RELEASE", "local-dev"), CollapseAgentInternalSpans: true}
	enabled, err := strconv.ParseBool(env("LANGFUSE_ENABLED", "false"))
	if err != nil {
		slog.Warn("Langfuse 开关配置无效，已关闭观测")
		return c
	}
	if !enabled {
		return c
	}
	collapse, err := strconv.ParseBool(env("LANGFUSE_COLLAPSE_AGENT_INTERNAL_SPANS", "true"))
	if err != nil {
		slog.Warn("Langfuse 折叠配置无效，已关闭观测")
		return c
	}
	c.Enabled, c.CollapseAgentInternalSpans = true, collapse
	c.PublicKey, c.SecretKey = os.Getenv("LANGFUSE_PUBLIC_KEY"), os.Getenv("LANGFUSE_SECRET_KEY")
	return c
}
