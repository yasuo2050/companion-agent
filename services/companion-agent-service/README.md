# companion-agent-service

AI Agent Runtime，负责 HTTP Chat API、SSE、Eino、Ark、Context Builder、4 个 Agent Tool、Business gRPC Client、Redis Conversation 和 Agent Trace。

边界：

- 只通过 `contracts/companion-business` 调用 Mock Business Service。
- 不直接访问 MongoDB。
- 不引用 `mock-business-service/internal`。
- `cmd/ark-smoke` 是运行后退出的集成验证命令，不是常驻服务。
