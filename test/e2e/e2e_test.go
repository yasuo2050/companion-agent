//go:build integration

package e2e

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

type response struct {
	Events, Tools []string
	Text          string
}

func chat(t *testing.T, user, conversation, message string) response {
	t.Helper()
	base := os.Getenv("E2E_BASE_URL")
	if base == "" {
		base = "http://127.0.0.1:8080"
	}
	body, _ := json.Marshal(map[string]string{"user_id": user, "conversation_id": conversation, "message": message})
	client := &http.Client{Timeout: 60 * time.Second}
	request, err := http.NewRequest(http.MethodPost, base+"/api/v1/agent/chat", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	raw, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Body.Close()
	if raw.StatusCode != http.StatusOK {
		t.Fatalf("HTTP 状态错误: %s", raw.Status)
	}
	result := response{}
	scanner := bufio.NewScanner(raw.Body)
	event := ""
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			event = strings.TrimPrefix(line, "event: ")
			result.Events = append(result.Events, event)
		}
		if strings.HasPrefix(line, "data: ") {
			var data map[string]any
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &data); err != nil {
				t.Fatal(err)
			}
			if event == "delta" {
				result.Text += fmt.Sprint(data["content"])
			}
			if event == "status" {
				result.Tools = append(result.Tools, fmt.Sprint(data["tool_name"]))
			}
			if event == "error" {
				t.Fatalf("SSE error: %#v", data)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(result.Events) < 2 || result.Events[0] != "turn_started" || result.Events[len(result.Events)-1] != "done" {
		t.Fatalf("SSE 顺序错误: %#v", result.Events)
	}
	return result
}
func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestPersonalizedRecommendations(t *testing.T) {
	cases := []struct{ user, name string }{{"10001", "小雨"}, {"10002", "阿杰"}, {"10003", "可可"}}
	for _, item := range cases {
		t.Run(item.user, func(t *testing.T) {
			result := chat(t, item.user, "e2e-personal-"+item.user, "明天晚上8点帮我找个永劫陪玩，玩两小时。只推荐一位，先不要创建订单预览。")
			if !contains(result.Tools, "search_companions") {
				t.Fatalf("未调用 search_companions: %#v", result.Tools)
			}
			if !strings.Contains(result.Text, item.name) {
				t.Fatalf("画像 %s 期望推荐 %s，实际 %s", item.user, item.name, result.Text)
			}
		})
	}
}
func TestHistoryDraftAndConfirm(t *testing.T) {
	conversation := "e2e-history-flow"
	preview := chat(t, "10001", conversation, "上次那个明天晚上8点再来两小时，直接创建订单预览")
	if !contains(preview.Tools, "get_recent_orders") || !contains(preview.Tools, "create_order_draft") {
		t.Fatalf("历史复购工具链不完整: %#v; 回复: %s", preview.Tools, preview.Text)
	}
	if !strings.Contains(preview.Text, "小雨") || !strings.Contains(preview.Text, "136") {
		t.Fatalf("Preview 不完整: %s", preview.Text)
	}
	confirmed := chat(t, "10001", conversation, "确认下单")
	if !contains(confirmed.Tools, "confirm_order") || !strings.Contains(confirmed.Text, "订单") {
		t.Fatalf("Confirm 失败: %#v %s", confirmed.Tools, confirmed.Text)
	}
}
func TestMissingStartTimeOnlyAsksNecessarySlot(t *testing.T) {
	result := chat(t, "10001", "e2e-missing-slot", "按我平时习惯找个永劫陪玩，时长也按平时来")
	if !strings.Contains(result.Text, "时间") && !strings.Contains(result.Text, "几点") {
		t.Fatalf("未补问开始时间: %s", result.Text)
	}
	if strings.Count(result.Text, "？") > 2 {
		t.Fatalf("补问过多: %s", result.Text)
	}
}
func TestNoCandidateDoesNotInvent(t *testing.T) {
	result := chat(t, "10001", "e2e-no-candidate", "明天晚上8点只要女性永劫陪玩，必须每小时1元以内，玩两小时")
	if !contains(result.Tools, "search_companions") {
		t.Fatalf("未真实查询候选")
	}
	if !strings.Contains(result.Text, "没有") && !strings.Contains(result.Text, "找不到") {
		t.Fatalf("无候选时未明确说明: %s", result.Text)
	}
}
