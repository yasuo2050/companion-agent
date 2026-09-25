package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"companion-agent/services/companion-agent-service/internal/identity"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"go.opentelemetry.io/otel/trace"
)

type knowledgeInput struct {
	Question string `json:"question" jsonschema:"required" jsonschema_description:"需要查询已发布平台规则的用户问题"`
}
type KnowledgeCitation struct {
	DocumentID string `json:"document_id"`
	VersionID  string `json:"version_id"`
	Version    int    `json:"version"`
	SegmentID  string `json:"segment_id"`
	Title      string `json:"title"`
	URL        string `json:"url"`
}
type knowledgeResponse struct {
	QueryID   string              `json:"query_id"`
	Status    string              `json:"status"`
	Answer    string              `json:"answer"`
	Citations []KnowledgeCitation `json:"citations"`
}

// NewKnowledgeTool 只透传服务端验证的令牌；模型只能选择问题文本。
func NewKnowledgeTool(baseURL, conversationID string, observer EventObserver) (tool.BaseTool, error) {
	client := &http.Client{Timeout: 35 * time.Second}
	return utils.InferTool("search_knowledge", "查询当前身份可访问的已发布平台政策、FAQ 和 SOP，返回答案与服务端核验的引用", func(ctx context.Context, in knowledgeInput) (map[string]any, error) {
		started := time.Now()
		observer("search_knowledge", "started", started, map[string]any{"question_length": len(in.Question)})
		token := identity.Token(ctx)
		if token == "" {
			return toolError("UNAUTHORIZED", "缺少服务端身份令牌"), nil
		}
		payload, _ := json.Marshal(map[string]string{"question": in.Question, "conversation_id": conversationID})
		req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(baseURL, "/")+"/api/v1/knowledge/query", bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		if span := trace.SpanContextFromContext(ctx); span.IsValid() {
			req.Header.Set("traceparent", fmt.Sprintf("00-%s-%s-01", span.TraceID(), span.SpanID()))
		}
		res, err := client.Do(req)
		if err != nil {
			return toolError("KNOWLEDGE_UNAVAILABLE", "知识服务暂不可用"), nil
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			return toolError("KNOWLEDGE_UNAVAILABLE", "知识服务查询失败"), nil
		}
		var value knowledgeResponse
		if err := json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&value); err != nil {
			return nil, err
		}
		if value.Status == "" {
			return nil, errors.New("知识服务响应缺少状态")
		}
		output := map[string]any{"status": value.Status, "answer": value.Answer, "query_id": value.QueryID, "citations": value.Citations}
		observer("search_knowledge", "completed", started, map[string]any{"query_id": value.QueryID, "status": value.Status, "citations": value.Citations})
		return output, nil
	})
}
