package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"companion-agent/services/companion-agent-service/internal/config"
	llmfactory "companion-agent/services/companion-agent-service/internal/llm"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

type addInput struct {
	A int `json:"a" jsonschema:"required" jsonschema_description:"第一个整数"`
	B int `json:"b" jsonschema:"required" jsonschema_description:"第二个整数"`
}

func main() {
	cfg, err := config.Load(true)
	if err != nil {
		fail("配置加载失败", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.LLMTimeout*3)
	defer cancel()
	chatModel, err := llmfactory.NewChatModel(ctx, cfg)
	if err != nil {
		fail("Ark 模型初始化失败", err)
	}
	slog.Info("开始 Ark Smoke", "ark_model_id", cfg.ArkModelID)
	started := time.Now()
	generated, err := chatModel.Generate(ctx, []*schema.Message{schema.UserMessage("只回复：ok")})
	if err != nil {
		fail("Generate Smoke 失败", err)
	}
	fmt.Printf("Generate: PASS (%dms, content=%q)\n", time.Since(started).Milliseconds(), generated.Content)
	started = time.Now()
	stream, err := chatModel.Stream(ctx, []*schema.Message{schema.UserMessage("用一句简短中文回复：流式测试成功")})
	if err != nil {
		fail("Stream Smoke 失败", err)
	}
	chunks := 0
	content := ""
	for {
		message, recvErr := stream.Recv()
		if recvErr == io.EOF {
			break
		}
		if recvErr != nil {
			fail("Stream 接收失败", recvErr)
		}
		chunks++
		content += message.Content
	}
	stream.Close()
	if chunks == 0 || content == "" {
		fail("Stream Smoke 失败", fmt.Errorf("未收到有效内容"))
	}
	fmt.Printf("Stream: PASS (%dms, chunks=%d)\n", time.Since(started).Milliseconds(), chunks)
	toolInfo, err := utils.GoStruct2ToolInfo[addInput]("add_numbers", "把两个整数相加；用户要求计算时必须调用此工具")
	if err != nil {
		fail("Tool Schema 创建失败", err)
	}
	toolModel, err := chatModel.WithTools([]*schema.ToolInfo{toolInfo})
	if err != nil {
		fail("Tool 绑定失败", err)
	}
	started = time.Now()
	callMessage, err := toolModel.Generate(ctx, []*schema.Message{schema.UserMessage("请务必调用 add_numbers 工具计算 2 加 3，不要自己口算")})
	if err != nil {
		fail("Tool Calling Smoke 失败", err)
	}
	if len(callMessage.ToolCalls) != 1 || callMessage.ToolCalls[0].Function.Name != "add_numbers" {
		fail("Tool Calling Smoke 失败", fmt.Errorf("模型未返回预期 ToolCall"))
	}
	call := callMessage.ToolCalls[0]
	final, err := toolModel.Generate(ctx, []*schema.Message{schema.UserMessage("请调用工具计算 2 加 3"), callMessage, schema.ToolMessage("5", call.ID)})
	if err != nil {
		fail("Tool Result 回传失败", err)
	}
	if final.Content == "" {
		fail("Tool Result 回传失败", fmt.Errorf("模型未生成最终回答"))
	}
	fmt.Printf("ToolCalling: PASS (%dms, tool=%s)\n", time.Since(started).Milliseconds(), call.Function.Name)
	fmt.Println("Ark Smoke 全部通过")
}
func fail(message string, err error) { slog.Error(message, "error", err); os.Exit(1) }
