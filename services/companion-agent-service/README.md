# Companion Agent Service

本服务负责 Ark/Eino Agent、4 个 Go Tool、gRPC Business Client、Redis Conversation、HTTP/SSE、Trace 与最小 Chat UI，不直接访问 MongoDB。

Agent 启动时会自动读取当前目录的 `.env`，首次使用：

```bash
# 创建 .env，填写 ARK_API_KEY 和 ARK_MODEL_ID
go run ./cmd/ark-smoke
```

`.env` 不会被 Git 提交，同名系统环境变量的优先级更高。Smoke 命令依次验证真实 Generate、Stream、Tool Calling 和 Tool Result 回传，不会打印 API Key。

启动服务：

```bash
go run ./cmd/server
```

启动后可在当前模块目录实时查看对话执行流水：

```bash
tail -f logs/agent-flow.log
```

流水包含用户原文、大模型调用轮次、ToolCall/ToolResult、最终回复与耗时，并用 `request_id` 串联。通过 `AGENT_FLOW_LOG_PATH` 可修改默认的 `logs/agent-flow.log`。日志不包含 Ark API Key，但含本地对话与业务数据，不得对外分发。

页面地址为 `http://127.0.0.1:8080/`，就绪检查为 `GET /readyz`。SSE 事件固定为 `turn_started → status* → delta* → done`，失败时以 `error` 代替 `done`。

AI Agent Runtime，负责 HTTP Chat API、SSE、Eino、Ark、Context Builder、4 个 Agent Tool、Business gRPC Client、Redis Conversation 和 Agent Trace。

订单 Preview 和确认结果不使用大模型文案：Go 代码直接解析 ToolResult，完成分转元、Asia/Shanghai 时区转换和固定模板渲染后，再写入 SSE、Redis 与 Trace。带 ToolCall 的中间模型文字不会发给前端。

边界：

- 只通过 `contracts/companion-business` 调用 Mock Business Service。
- 不直接访问 MongoDB。
- 不引用 `mock-business-service/internal`。
- `cmd/ark-smoke` 是运行后退出的集成验证命令，不是常驻服务。
