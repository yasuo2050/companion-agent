package config

import "testing"

func TestLoadRequiresLLMCredentials(t *testing.T) {
	t.Setenv("ARK_API_KEY", "")
	t.Setenv("ARK_MODEL_ID", "")
	if _, err := Load(true); err == nil {
		t.Fatal("缺少 Ark 凭证时必须启动失败")
	}
}
func TestLoadDoesNotRequireLLMForDeterministicComponents(t *testing.T) {
	t.Setenv("ARK_API_KEY", "")
	t.Setenv("ARK_MODEL_ID", "")
	cfg, err := Load(false)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxIterations != 6 || cfg.ConversationTTL.Hours() != 24 || cfg.AgentFlowLogPath != "logs/agent-flow.log" {
		t.Fatalf("默认配置错误: %#v", cfg)
	}
}

func TestLoadUsesArkModelIDAsModelIdentifier(t *testing.T) {
	t.Setenv("ARK_API_KEY", "test-key")
	t.Setenv("ARK_MODEL_ID", "doubao-seed-2-0-mini-260428")

	cfg, err := Load(true)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ArkModelID != "doubao-seed-2-0-mini-260428" {
		t.Fatalf("ARK_MODEL_ID 未正确加载: %q", cfg.ArkModelID)
	}
}

func TestLoadLangfuseDefaultsToDisabled(t *testing.T) {
	for _, key := range []string{"LANGFUSE_ENABLED", "LANGFUSE_HOST", "LANGFUSE_PUBLIC_KEY", "LANGFUSE_SECRET_KEY", "LANGFUSE_SERVICE_NAME", "LANGFUSE_ENVIRONMENT", "LANGFUSE_RELEASE", "LANGFUSE_COLLAPSE_AGENT_INTERNAL_SPANS"} {
		t.Setenv(key, "")
	}

	cfg, err := Load(false)
	if err != nil {
		t.Fatal(err)
	}
	got := cfg.Langfuse
	if got.Enabled || got.Host != "http://127.0.0.1:3000" || got.ServiceName != "companion-agent-service" || got.Environment != "local" || got.Release != "local-dev" || !got.CollapseAgentInternalSpans {
		t.Fatalf("Langfuse 默认配置错误: %#v", got)
	}
}

func TestLoadLangfuseEnabled(t *testing.T) {
	t.Setenv("LANGFUSE_ENABLED", "true")
	t.Setenv("LANGFUSE_PUBLIC_KEY", "public-test")
	t.Setenv("LANGFUSE_SECRET_KEY", "secret-test")
	t.Setenv("LANGFUSE_COLLAPSE_AGENT_INTERNAL_SPANS", "false")

	cfg, err := Load(false)
	if err != nil {
		t.Fatal(err)
	}
	got := cfg.Langfuse
	if !got.Enabled || got.PublicKey != "public-test" || got.SecretKey != "secret-test" || got.CollapseAgentInternalSpans {
		t.Fatalf("Langfuse 启用配置错误: %#v", got)
	}
}

func TestLoadLangfuseInvalidBooleanFallsBackToDisabled(t *testing.T) {
	for _, tc := range []struct {
		name, enabled, collapse string
	}{
		{name: "无效开关", enabled: "sometimes", collapse: "true"},
		{name: "无效折叠配置", enabled: "true", collapse: "sometimes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LANGFUSE_ENABLED", tc.enabled)
			t.Setenv("LANGFUSE_COLLAPSE_AGENT_INTERNAL_SPANS", tc.collapse)
			cfg, err := Load(false)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Langfuse.Enabled {
				t.Fatalf("无效旁路配置不应启用: %#v", cfg.Langfuse)
			}
		})
	}
}
