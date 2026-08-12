package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type orderPresentation struct {
	Kind    string
	Content string
}

type orderToolPayload struct {
	Draft *orderTemplateData `json:"draft"`
	Order *orderTemplateData `json:"order"`
}

type orderTemplateData struct {
	DraftID         string `json:"draftId"`
	OrderID         string `json:"orderId"`
	CompanionID     string `json:"companionId"`
	CompanionName   string `json:"companionName"`
	GameID          string `json:"gameId"`
	StartTime       string `json:"startTime"`
	EndTime         string `json:"endTime"`
	DurationMinutes int32  `json:"durationMinutes"`
	PricePerHour    cents  `json:"pricePerHour"`
	TotalAmount     cents  `json:"totalAmount"`
}

type cents int64

func (value *cents) UnmarshalJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		raw = raw[1 : len(raw)-1]
	}
	parsed, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil {
		return err
	}
	*value = cents(parsed)
	return nil
}

func renderOrderToolResult(toolName string, value any) (orderPresentation, bool) {
	if toolName != "create_order_draft" && toolName != "confirm_order" {
		return orderPresentation{}, false
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return orderPresentation{}, false
	}
	var payload orderToolPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return orderPresentation{}, false
	}
	if toolName == "create_order_draft" && payload.Draft != nil {
		content, ok := renderDraftTemplate(payload.Draft)
		return orderPresentation{Kind: "order_preview", Content: content}, ok
	}
	if toolName == "confirm_order" && payload.Order != nil {
		content, ok := renderConfirmedOrderTemplate(payload.Order)
		return orderPresentation{Kind: "order_confirmed", Content: content}, ok
	}
	return orderPresentation{}, false
}

func renderDraftTemplate(value *orderTemplateData) (string, bool) {
	start, end, ok := templateTimes(value)
	if !ok || value.DraftID == "" || value.CompanionID == "" || value.CompanionName == "" || value.GameID == "" || value.DurationMinutes <= 0 || value.PricePerHour <= 0 || value.TotalAmount < 0 {
		return "", false
	}
	return fmt.Sprintf(`订单预览：
- 陪玩：%s（%s）
- 游戏：%s
- 时间：%s - %s（%d分钟）
- 单价：%s/小时
- 总价：%s
- 草稿ID：%s

确认下单请回复“确认”。`, value.CompanionName, value.CompanionID, gameName(value.GameID), start, end, value.DurationMinutes, formatYuan(int64(value.PricePerHour)), formatYuan(int64(value.TotalAmount)), value.DraftID), true
}

func renderConfirmedOrderTemplate(value *orderTemplateData) (string, bool) {
	start, end, ok := templateTimes(value)
	if !ok || value.OrderID == "" || value.CompanionID == "" || value.CompanionName == "" || value.GameID == "" || value.DurationMinutes <= 0 || value.PricePerHour <= 0 || value.TotalAmount < 0 {
		return "", false
	}
	return fmt.Sprintf(`订单已确认：
- 订单号：%s
- 陪玩：%s（%s）
- 游戏：%s
- 时间：%s - %s（%d分钟）
- 单价：%s/小时
- 总价：%s`, value.OrderID, value.CompanionName, value.CompanionID, gameName(value.GameID), start, end, value.DurationMinutes, formatYuan(int64(value.PricePerHour)), formatYuan(int64(value.TotalAmount))), true
}

func templateTimes(value *orderTemplateData) (string, string, bool) {
	start, err := time.Parse(time.RFC3339Nano, value.StartTime)
	if err != nil {
		return "", "", false
	}
	end, err := time.Parse(time.RFC3339Nano, value.EndTime)
	if err != nil {
		return "", "", false
	}
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	return start.In(location).Format("2006-01-02 15:04"), end.In(location).Format("2006-01-02 15:04"), true
}

func formatYuan(value int64) string {
	sign := ""
	if value < 0 {
		sign = "-"
		value = -value
	}
	yuan, fraction := value/100, value%100
	if fraction == 0 {
		return fmt.Sprintf("%s%d元", sign, yuan)
	}
	return fmt.Sprintf("%s%d.%02d元", sign, yuan, fraction)
}

func gameName(gameID string) string {
	values := map[string]string{"lol": "LOL", "naraka": "永劫无间", "hok": "王者荣耀"}
	if value := values[strings.ToLower(gameID)]; value != "" {
		return value
	}
	return gameID
}
