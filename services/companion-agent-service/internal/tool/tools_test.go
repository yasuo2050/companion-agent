package tool

import (
	"context"
	"testing"
)

func TestBusinessToolSchemas(t *testing.T) {
	tools, err := Build(nil, "10001", "c1")
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 5 {
		t.Fatalf("期望 5 个 Tool，实际 %d", len(tools))
	}
	expected := []string{"get_recent_orders", "get_order", "search_companions", "create_order_draft", "confirm_order"}
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

func TestRecentOrderDefaultsFillOnlyMissingSearchParameters(t *testing.T) {
	defaults := OrderDefaults{CompanionID: "p003", GameID: "naraka", DurationMinutes: 120, Source: "recent_order"}
	filled := applySearchDefaults(searchInput{}, defaults)
	if filled.GameID != "naraka" || filled.DurationMinutes != 120 {
		t.Fatalf("未补齐最近订单默认值: %#v", filled)
	}
	if filled.CompanionID != "" {
		t.Fatalf("普通搜索不得沿用最近陪玩: %#v", filled)
	}
	repeat := applySearchDefaults(searchInput{ReuseRecentOrder: true}, defaults)
	if repeat.CompanionID != "p003" {
		t.Fatalf("复购搜索未补齐最近陪玩: %#v", repeat)
	}
	explicit := applySearchDefaults(searchInput{CompanionID: "p001", ReuseRecentOrder: true, GameID: "lol", DurationMinutes: 90}, defaults)
	if explicit.CompanionID != "p001" || explicit.GameID != "lol" || explicit.DurationMinutes != 90 {
		t.Fatalf("用户明确参数不得被覆盖: %#v", explicit)
	}
}

func TestRecentOrderDefaultsFillOnlyMissingDraftParameters(t *testing.T) {
	defaults := OrderDefaults{GameID: "hok", DurationMinutes: 90, Source: "recent_order"}
	filled := applyDraftDefaults(draftInput{CompanionID: "p003", StartTime: "2026-08-13T14:00:00+08:00"}, defaults)
	if filled.GameID != "hok" || filled.DurationMinutes != 90 {
		t.Fatalf("草稿未补齐最近订单默认值: %#v", filled)
	}
}

func TestRecentOrderReuseRequiresHistoryToolAndCompanion(t *testing.T) {
	if output := validateRecentOrderReuse(searchInput{ReuseRecentOrder: true, CompanionID: "p003"}, false); output == nil {
		t.Fatal("未调用历史订单工具时不得执行复购搜索")
	}
	if output := validateRecentOrderReuse(searchInput{ReuseRecentOrder: true}, true); output == nil {
		t.Fatal("没有最近陪玩时不得退化为普通搜索")
	}
	if output := validateRecentOrderReuse(searchInput{ReuseRecentOrder: true, CompanionID: "p003"}, true); output != nil {
		t.Fatalf("历史订单已查询且陪玩完整时不应报错: %#v", output)
	}
	if output := validateRecentOrderReuse(searchInput{}, false); output != nil {
		t.Fatalf("普通搜索不应依赖历史订单工具: %#v", output)
	}
}
