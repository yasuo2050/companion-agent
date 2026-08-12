// Package llm 封装模型配置与 Ark Adapter 初始化。
package llm

import (
	"context"
	"fmt"

	"companion-agent/services/companion-agent-service/internal/config"
	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/components/model"
	arkmodel "github.com/volcengine/volcengine-go-sdk/service/arkruntime/model"
)

// NewChatModel 创建可安全复用的基础 Tool Calling ChatModel。
func NewChatModel(ctx context.Context, cfg config.Config) (model.ToolCallingChatModel, error) {
	if cfg.LLMProvider != "ark" {
		return nil, fmt.Errorf("不支持的模型提供方: %s", cfg.LLMProvider)
	}
	thinking := &arkmodel.Thinking{Type: arkmodel.ThinkingTypeDisabled}
	return ark.NewChatModel(ctx, &ark.ChatModelConfig{APIKey: cfg.ArkAPIKey, BaseURL: cfg.ArkBaseURL, Model: cfg.ArkModelID, MaxTokens: &cfg.LLMMaxTokens, Timeout: &cfg.LLMTimeout, RetryTimes: &cfg.LLMRetryTimes, Thinking: thinking})
}
