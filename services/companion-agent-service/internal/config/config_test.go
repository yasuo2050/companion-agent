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
