package tool

import (
	"companion-agent/services/companion-agent-service/internal/identity"
	"context"
	"encoding/json"
	einotool "github.com/cloudwego/eino/components/tool"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestKnowledgeValidationStrictRequestContract(t *testing.T) {
	// 与 Knowledge Service 的严格 DTO 契约一致；标题、URL、version 是显示字段。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Citations []struct {
				DocumentID    string `json:"document_id"`
				VersionID     string `json:"version_id"`
				Revision      string `json:"index_revision"`
				ScopeRevision int64  `json:"scope_revision"`
				ChunkID       string `json:"segment_id"`
			} `json:"citations"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&in); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if r.URL.Path != "/api/v1/knowledge/validate" || r.Header.Get("Authorization") != "Bearer local-token" || len(in.Citations) != 1 || in.Citations[0].ScopeRevision != 7 || in.Citations[0].ChunkID != "chunk" {
			t.Error("复核契约或身份错误")
		}
		json.NewEncoder(w).Encode(map[string]bool{"valid": true})
	}))
	defer srv.Close()
	valid, err := ValidateKnowledgeEvidence(identity.WithToken(context.Background(), "local-token"), srv.URL, []KnowledgeCitation{{DocumentID: "doc", VersionID: "ver", Revision: "rev", ScopeRevision: 7, SegmentID: "chunk", Title: "显示标题", URL: "/source", Version: 2}})
	if err != nil || !valid {
		t.Fatal("复核失败", err)
	}
}

func TestReadOnlyKnowledgeConcurrentCallsShareOneRetrievalBudget(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		json.NewEncoder(w).Encode(map[string]any{"status": "no_accessible_sources", "backend": "milvus"})
	}))
	defer srv.Close()
	ctx := identity.WithToken(context.Background(), "local-token")
	build := func() einotool.InvokableTool {
		base, err := NewKnowledgeToolMode(srv.URL, "same-turn", true, func(string, string, time.Time, any) {})
		if err != nil {
			t.Fatal(err)
		}
		return base.(einotool.InvokableTool)
	}
	tool := build()
	var wg sync.WaitGroup
	results := make(chan string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := tool.InvokableRun(ctx, `{"question":"退款比例"}`)
			if err != nil {
				t.Error(err)
			}
			results <- out
		}()
	}
	wg.Wait()
	close(results)
	rejected := 0
	for out := range results {
		if strings.Contains(out, "KNOWLEDGE_QUERY_BUDGET_EXCEEDED") {
			rejected++
		}
	}
	if requests.Load() != 1 || rejected != 7 {
		t.Fatal("并行检索未受预算限制", requests.Load(), rejected)
	}
	if _, err := build().InvokableRun(ctx, `{"question":"退款比例"}`); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 {
		t.Fatal("下一轮预算没有重置")
	}
}
