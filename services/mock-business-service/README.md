# Mock Business Service

本服务通过 gRPC 提供用户画像、历史订单、陪玩查询、订单草稿和确认下单能力，所有业务事实持久化到 MongoDB `companion_agent_mvp`。active Draft 不占档期，confirmed Order 才占档期。

```bash
# 幂等写入 Demo 数据
go run ./cmd/seed

# 仅重置 companion_agent_mvp 与 companion-agent:mvp: 前缀后重新 Seed
go run ./cmd/seed --reset

# 启动 gRPC 服务
go run ./cmd/server
```

确定性测试：

```bash
go test ./...
go test -race ./...
```

服务已启动且完成 Seed 后，可执行真实 MongoDB + gRPC 联调：

```bash
go test -tags=integration ./test
```

用于聚合模拟正式环境的 User/Profile、Companion、Recommendation/Search 和 Order 服务，负责 MongoDB、业务规则、档期、计费、Draft 与 Mock Order。

边界：

- 对 Agent 只暴露 gRPC Contract。
- 不负责 Redis Conversation。
- 不引用 `companion-agent-service/internal`。
- `cmd/seed` 是运行后退出的数据初始化命令，不是常驻服务。
