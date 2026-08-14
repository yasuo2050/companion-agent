package agent

import (
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestReceiveMessageStreamEmitsEveryTextChunkAndKeepsFullMessage(t *testing.T) {
	stream := schema.StreamReaderFromArray([]*schema.Message{
		schema.AssistantMessage("为你", nil),
		schema.AssistantMessage("推荐小雨", nil),
	})
	var deltas []string
	message, streamed, err := receiveMessageStream(stream, true, func(event Event) {
		if event.Type == "delta" {
			deltas = append(deltas, event.Content)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if !streamed || strings.Join(deltas, "") != "为你推荐小雨" {
		t.Fatalf("文本没有逐块输出: streamed=%v deltas=%#v", streamed, deltas)
	}
	if message == nil || message.Content != "为你推荐小雨" {
		t.Fatalf("完整消息没有正确合并: %#v", message)
	}
}

func TestReceiveMessageStreamDoesNotExposeToolCallArguments(t *testing.T) {
	stream := schema.StreamReaderFromArray([]*schema.Message{{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{{
			ID: "call-1", Type: "function",
			Function: schema.FunctionCall{Name: "search_companions", Arguments: `{"user_id":"10001"}`},
		}},
	}})
	var deltas []string
	message, streamed, err := receiveMessageStream(stream, true, func(event Event) {
		deltas = append(deltas, event.Content)
	})
	if err != nil {
		t.Fatal(err)
	}
	if streamed || len(deltas) != 0 {
		t.Fatalf("ToolCall 参数泄漏到前端: %#v", deltas)
	}
	if message == nil || len(message.ToolCalls) != 1 || message.ToolCalls[0].Function.Name != "search_companions" {
		t.Fatalf("ToolCall 没有保留在完整消息中: %#v", message)
	}
}

func TestReceiveMessageStreamCanSuppressModelTextForOrderTemplate(t *testing.T) {
	stream := schema.StreamReaderFromArray([]*schema.Message{schema.AssistantMessage("模型生成的订单文案", nil)})
	emitted := false
	message, streamed, err := receiveMessageStream(stream, false, func(Event) { emitted = true })
	if err != nil {
		t.Fatal(err)
	}
	if emitted || streamed {
		t.Fatal("订单模板阶段不应透传模型文本")
	}
	if message == nil || message.Content != "模型生成的订单文案" {
		t.Fatalf("被抑制的模型消息仍需完整合并: %#v", message)
	}
}

func TestReceiveMessageStreamReturnsRecvError(t *testing.T) {
	want := errors.New("stream interrupted")
	stream, writer := schema.Pipe[*schema.Message](1)
	go func() {
		writer.Send(nil, want)
		writer.Close()
	}()
	_, _, err := receiveMessageStream(stream, true, func(Event) {})
	if !errors.Is(err, want) {
		t.Fatalf("流错误未向上传递: %v", err)
	}
}
