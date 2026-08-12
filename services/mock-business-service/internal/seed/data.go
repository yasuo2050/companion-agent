// Package seed 提供可重复的本地演示数据。
package seed

import (
	"time"

	"companion-agent/services/mock-business-service/internal/business"
)

// Data 返回按当前日期生成未来档期的固定 Demo 数据。
func Data(now time.Time) ([]business.UserProfile, []business.Companion, []business.Order) {
	location, _ := time.LoadLocation("Asia/Shanghai")
	now = now.In(location)
	updated := now.Truncate(time.Second)
	profiles := []business.UserProfile{
		{UserID: "10001", FavoriteGameIDs: []string{"naraka"}, PreferredGender: "female", PreferredStyles: []string{"聊天娱乐"}, PreferredTags: []string{"会聊天", "不压力", "声音甜"}, PriceMin: 5000, PriceMax: 8000, UsualDurationMinutes: 120, UpdatedAt: updated},
		{UserID: "10002", FavoriteGameIDs: []string{"lol"}, PreferredStyles: []string{"认真上分"}, PreferredTags: []string{"上分", "高段位", "指挥"}, PriceMax: 10000, UsualDurationMinutes: 120, UpdatedAt: updated},
		{UserID: "10003", FavoriteGameIDs: []string{"hok"}, PreferredStyles: []string{"轻松娱乐"}, PreferredTags: []string{"搞笑", "会聊天", "情绪价值"}, PriceMax: 7000, UsualDurationMinutes: 90, UpdatedAt: updated},
	}
	definitions := []struct {
		id, name, gender string
		games, tags      []string
		price            int64
		level            string
	}{
		{"p001", "小雨", "female", []string{"naraka", "lol"}, []string{"会聊天", "不压力", "声音甜"}, 6800, "diamond"},
		{"p002", "阿杰", "male", []string{"naraka", "lol"}, []string{"上分", "高段位", "指挥"}, 7500, "master"},
		{"p003", "可可", "female", []string{"naraka", "hok"}, []string{"搞笑", "会聊天", "情绪价值"}, 6500, "diamond"},
		{"p004", "安然", "female", []string{"lol", "hok"}, []string{"安静", "耐心", "教学"}, 6000, "diamond"},
		{"p005", "北辰", "male", []string{"naraka", "hok"}, []string{"上分", "教学", "耐心"}, 7200, "master"},
		{"p006", "糖糖", "female", []string{"naraka", "lol", "hok"}, []string{"话多", "声音甜", "情绪价值"}, 8000, "diamond"},
		{"p007", "林深", "male", []string{"lol"}, []string{"高段位", "安静", "指挥"}, 9000, "grandmaster"},
		{"p008", "阿梨", "female", []string{"hok"}, []string{"搞笑", "不压力", "御姐音"}, 6200, "master"},
		{"p009", "十一", "male", []string{"naraka"}, []string{"少年音", "会聊天", "教学"}, 5800, "diamond"},
		{"p010", "星野", "female", []string{"lol", "hok"}, []string{"上分", "高段位", "耐心"}, 8500, "master"},
	}
	companions := make([]business.Companion, 0, len(definitions))
	for _, d := range definitions {
		companions = append(companions, business.Companion{CompanionID: d.id, Name: d.name, Gender: d.gender, GameIDs: d.games, Styles: d.tags, SkillLevel: d.level, PricePerHour: d.price, Tags: d.tags, AvailableSlots: futureSlots(now, location), Status: "active"})
	}
	start := time.Date(now.Year(), now.Month(), now.Day()-2, 20, 0, 0, 0, location)
	orders := []business.Order{
		{OrderID: "history-10001-1", UserID: "10001", CompanionID: "p001", CompanionName: "小雨", GameID: "naraka", StartTime: start, EndTime: start.Add(2 * time.Hour), DurationMinutes: 120, PricePerHour: 6800, TotalAmount: 13600, Status: business.OrderConfirmed, Source: "seed_history", CreatedAt: start.Add(-24 * time.Hour)},
		{OrderID: "history-10002-1", UserID: "10002", CompanionID: "p002", CompanionName: "阿杰", GameID: "lol", StartTime: start, EndTime: start.Add(2 * time.Hour), DurationMinutes: 120, PricePerHour: 7500, TotalAmount: 15000, Status: business.OrderConfirmed, Source: "seed_history", CreatedAt: start.Add(-48 * time.Hour)},
		{OrderID: "history-10003-1", UserID: "10003", CompanionID: "p003", CompanionName: "可可", GameID: "hok", StartTime: start, EndTime: start.Add(90 * time.Minute), DurationMinutes: 90, PricePerHour: 6500, TotalAmount: 9750, Status: business.OrderConfirmed, Source: "seed_history", CreatedAt: start.Add(-72 * time.Hour)},
	}
	return profiles, companions, orders
}
func futureSlots(now time.Time, location *time.Location) []business.AvailableSlot {
	slots := make([]business.AvailableSlot, 0, 8)
	for i := 0; i < 8; i++ {
		day := now.AddDate(0, 0, i)
		slots = append(slots, business.AvailableSlot{StartAt: time.Date(day.Year(), day.Month(), day.Day(), 8, 0, 0, 0, location), EndAt: time.Date(day.Year(), day.Month(), day.Day(), 23, 59, 59, 0, location)})
	}
	return slots
}
