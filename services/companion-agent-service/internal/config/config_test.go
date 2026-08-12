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
