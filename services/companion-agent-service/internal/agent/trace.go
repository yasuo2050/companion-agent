package agent

import (
	"encoding/json"
	"strings"
	"time"

	"companion-agent/services/companion-agent-service/internal/flowlog"
	arkmodel "github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/schema"
)

type tracedCandidate struct {
	ID   string `json:"companion_id"`
	Name string `json:"name"`
}

type toolTracePayload struct {
	Candidates []tracedCandidate `json:"candidates"`
	Draft      *struct {
		DraftID     string `json:"draftId"`
		CompanionID string `json:"companionId"`
	} `json:"draft"`
	Order *struct {
		OrderID     string `json:"orderId"`
		CompanionID string `json:"companionId"`
	} `json:"order"`
	Error *struct {
		Code string `json:"code"`
	} `json:"error"`
}

// logTurnTrace 从同一轮真实模型与 Tool 输出汇总可审计字段，不记录用户原文和密钥。
func logTurnTrace(requestID, conversationID, userID, modelAlias string, duration time.Duration, outputs []*schema.Message) {
	var inputTokens, outputTokens, totalTokens, llmIterations int
	var candidateIDs, arkRequestIDs, toolErrorCodes []string
	candidates := make([]tracedCandidate, 0)
	recommendedID, draftID, orderID := "", "", ""
	finalText := ""

	for _, message := range outputs {
		if message == nil {
			continue
		}
		if message.Role == schema.Assistant {
			llmIterations++
			if message.Content != "" {
				finalText = message.Content
			}
			if value := arkmodel.GetArkRequestID(message); value != "" {
				arkRequestIDs = appendUnique(arkRequestIDs, value)
			}
			if message.ResponseMeta != nil && message.ResponseMeta.Usage != nil {
				usage := message.ResponseMeta.Usage
				inputTokens += usage.PromptTokens
				outputTokens += usage.CompletionTokens
				totalTokens += usage.TotalTokens
			}
		}
		if message.Role != schema.Tool || message.Content == "" {
			continue
		}
		var payload toolTracePayload
		if json.Unmarshal([]byte(message.Content), &payload) != nil {
			continue
		}
		for _, candidate := range payload.Candidates {
			candidateIDs = appendUnique(candidateIDs, candidate.ID)
			candidates = append(candidates, candidate)
		}
		if payload.Draft != nil {
			draftID = payload.Draft.DraftID
			recommendedID = payload.Draft.CompanionID
		}
		if payload.Order != nil {
			orderID = payload.Order.OrderID
			recommendedID = payload.Order.CompanionID
		}
		if payload.Error != nil && payload.Error.Code != "" {
			toolErrorCodes = appendUnique(toolErrorCodes, payload.Error.Code)
		}
	}
	if recommendedID == "" {
		recommendedID = inferRecommendedID(finalText, candidates)
	}
	flowlog.Info("对话轮次汇总", "flow_step", "turn_summary",
		"request_id", requestID, "conversation_id", conversationID, "user_id", userID,
		"provider", "volcengine-ark", "model", modelAlias, "ark_request_ids", arkRequestIDs,
		"llm_iterations", llmIterations, "llm_duration_ms", duration.Milliseconds(),
		"candidate_ids", candidateIDs, "recommended_companion_id", recommendedID,
		"draft_id", draftID, "order_id", orderID, "tool_error_codes", toolErrorCodes,
		"input_tokens", inputTokens, "output_tokens", outputTokens, "total_tokens", totalTokens,
		"total_duration_ms", duration.Milliseconds())
}

func inferRecommendedID(text string, candidates []tracedCandidate) string {
	bestID, bestIndex := "", len(text)+1
	for _, candidate := range candidates {
		for _, marker := range []string{candidate.ID, candidate.Name} {
			if marker == "" {
				continue
			}
			if index := strings.Index(text, marker); index >= 0 && index < bestIndex {
				bestID, bestIndex = candidate.ID, index
			}
		}
	}
	return bestID
}

func appendUnique(values []string, value string) []string {
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
