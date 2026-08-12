# mock-business-service

用于聚合模拟正式环境的 User/Profile、Companion、Recommendation/Search 和 Order 服务，负责 MongoDB、业务规则、档期、计费、Draft 与 Mock Order。

边界：

- 对 Agent 只暴露 gRPC Contract。
- 不负责 Redis Conversation。
- 不引用 `companion-agent-service/internal`。
- `cmd/seed` 是运行后退出的数据初始化命令，不是常驻服务。
