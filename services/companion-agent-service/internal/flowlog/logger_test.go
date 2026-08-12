package flowlog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigureWritesTailFriendlyLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "agent-flow.log")
	file, absolutePath, err := Configure(path)
	if err != nil {
		t.Fatal(err)
	}
	Info("调用大模型", "flow_step", "llm_started", "request_id", "r-test")
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(absolutePath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "调用大模型") || !strings.Contains(text, "flow_step=llm_started") || !strings.Contains(text, "request_id=r-test") {
		t.Fatalf("流水日志字段不完整: %s", text)
	}
}
