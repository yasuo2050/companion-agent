package tool

import (
	"context"
	"testing"
)

func TestFourToolSchemas(t *testing.T) {
	tools, err := Build(nil, "10001", "c1")
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 4 {
		t.Fatalf("期望 4 个 Tool，实际 %d", len(tools))
	}
	expected := []string{"get_recent_orders", "search_companions", "create_order_draft", "confirm_order"}
	for i, item := range tools {
		info, err := item.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if info.Name != expected[i] {
			t.Fatalf("Tool 名称错误: %s", info.Name)
		}
		if info.ParamsOneOf == nil {
			t.Fatalf("%s 缺少参数 Schema", info.Name)
		}
	}
}
func TestControlledTags(t *testing.T) {
	if err := ValidateControlledTags([]string{"搞笑", "会聊天", "不压力"}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateControlledTags([]string{"有意思"}); err == nil {
		t.Fatal("自然语言别名不得直接进入业务标签")
	}
}
