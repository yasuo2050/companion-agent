# Companion Agent Service

本服务负责 Ark/Eino Agent、4 个 Go Tool、gRPC Business Client、Redis Conversation、HTTP/SSE、Trace 与最小 Chat UI，不直接访问 MongoDB。

Ark 凭证只能通过当前进程环境变量提供：

```bash
export ARK_API_KEY="<从火山方舟控制台复制后仅注入当前终端>"
export ARK_MODEL_ID="doubao-seed-2-1-turbo-260628"
go run ./cmd/ark-smoke
```

Smoke 命令依次验证真实 Generate、Stream、Tool Calling 和 Tool Result 回传，不会打印 API Key。

启动服务：

```bash
go run ./cmd/server
```

页面地址为 `http://127.0.0.1:8080/`，就绪检查为 `GET /readyz`。SSE 事件固定为 `turn_started → status* → delta* → done`，失败时以 `error` 代替 `done`。

AI Agent Runtime，负责 HTTP Chat API、SSE、Eino、Ark、Context Builder、4 个 Agent Tool、Business gRPC Client、Redis Conversation 和 Agent Trace。

边界：

- 只通过 `contracts/companion-business` 调用 Mock Business Service。
- 不直接访问 MongoDB。
- 不引用 `mock-business-service/internal`。
- `cmd/ark-smoke` 是运行后退出的集成验证命令，不是常驻服务。
