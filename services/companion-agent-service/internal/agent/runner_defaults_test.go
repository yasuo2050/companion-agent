package agent

import (
	"strings"
	"testing"
	"time"

	businessv1 "companion-agent/contracts/companion-business/gen"
)

func TestDeriveOrderDefaultsPrefersRecentOrder(t *testing.T) {
	profile := &businessv1.UserProfile{
		FavoriteGameIds:      []string{"lol"},
		UsualDurationMinutes: 90,
	}
	recent := []*businessv1.Order{{GameId: "naraka", DurationMinutes: 120}}

	defaults := deriveOrderDefaults(profile, recent)
	if defaults.GameID != "naraka" || defaults.DurationMinutes != 120 || defaults.Source != "recent_order" {
		t.Fatalf("最近订单没有优先于画像: %#v", defaults)
	}
}

func TestDeriveOrderDefaultsFallsBackToProfile(t *testing.T) {
	profile := &businessv1.UserProfile{
		FavoriteGameIds:      []string{"hok"},
		UsualDurationMinutes: 90,
	}

	defaults := deriveOrderDefaults(profile, nil)
	if defaults.GameID != "hok" || defaults.DurationMinutes != 90 || defaults.Source != "profile_fallback" {
		t.Fatalf("没有历史订单时未使用画像兜底: %#v", defaults)
	}
}

func TestSystemInstructionIncludesDeterministicDefaults(t *testing.T) {
	instruction := systemInstruction(
		&businessv1.UserProfile{},
		deriveOrderDefaults(&businessv1.UserProfile{FavoriteGameIds: []string{"naraka"}, UsualDurationMinutes: 120}, nil),
		time.Date(2026, 8, 13, 11, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60)),
	)
	for _, expected := range []string{`"game_id":"naraka"`, `"duration_minutes":120`, "不得再追问已有默认值的字段"} {
		if !strings.Contains(instruction, expected) {
			t.Fatalf("系统指令缺少 %q: %s", expected, instruction)
		}
	}
}
