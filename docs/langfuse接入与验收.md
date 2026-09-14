# companion-agent Langfuse 一期接入与验收

验收日期：2026-09-08。

## 接入边界

一期只接入请求级 Trace、Eino Agent/模型/Tool 子节点、Token/耗时、业务错误语义和本地 Langfuse 部署。保留现有 HTTP/SSE、gRPC、MongoDB、Redis、flowlog、订单固定模板与历史恢复行为。

未接入 Dataset、Eval Runner、远程 Prompt 管理、完整 gRPC/Redis 埋点和自建 Trace Viewer。模型货币成本不作为一期必过项；没有上游定价时不填零值。

## Trace 生命周期

Root 名称固定为 `companion-agent.turn`。Handler 在请求解析校验、Conversation 锁和 request ID 取得后创建 Root，随后使用同一上下文执行 Conversation Load、Agent Run 和 Conversation Save。锁等待时间与无效 HTTP 请求不进入 Root。

```text
companion-agent.turn
├── Conversation Load（当前只体现在 Root 生命周期内）
├── companion-order-agent
│   ├── Ark Generation
│   ├── search / recent-orders / draft / confirm Tool
│   └── 必要时的后续 Ark Generation
└── Conversation Save（当前只体现在 Root 生命周期内）
```

Root 使用 `user_id` 关联 User、`conversation_id` 关联 Session，并记录 request ID、conversation ID、模型 ID、Release、最终实际发送文本和最终状态。flowlog 的 `turn_observed` 记录 request ID 与 trace ID，便于双向查找。

## 观测语义

- `tracedModel` 透传底层模型的 Callback 能力与类型；工具绑定后的包装器也保持相同能力，避免 Eino 再包一层导致 Generation/Token 重复。
- Tool 返回结构化 `error.code` 时保留原始 JSON 给模型。`PRICE_CHANGED` 等业务拒绝标记为 `WARNING`；`BUSINESS_RPC_TIMEOUT` 标记 Tool 失败；Agent 恢复成功后 Root 仍按本轮最终成功结束。
- Draft/Confirm 得到完整成功结果后，原有固定模板继续直接返回，并避免额外模型调用。观测副本把内部停止信号归一化为成功，同时保留 ToolCall、ToolResult、最终模板和 `direct_return` 原因。
- Root、模型、Tool 输入输出和观测错误统一脱敏。配置对象、环境变量集合和 HTTP Authorization 不进入 Trace。
- exporter 使用 2048 有界队列、128 批次、1 秒批处理间隔、3 秒请求超时和最多 10 秒有限重试。队列满或 Langfuse 不可用不会触发业务重试。
- 服务先排空 HTTP 请求，再使用独立 5 秒上下文关闭观测组件。Langfuse 不属于 `/readyz` 依赖。

## 自动化验收

| 检查 | 结果 | 证据 |
|---|---|---|
| Eino `v0.9.14` 升级 | 已通过 | 独立提交 `79ff923`，升级前后单测、race 与现有 E2E 通过 |
| 默认关闭与无效配置降级 | 已通过 | `internal/config`、`internal/observability` 单测 |
| Root 成功/失败/取消/超时收尾 | 已通过 | 内存 exporter 检查状态、输出和幂等 End |
| 并发隔离与同 Session 多 Root | 已通过 | 20 路并发 Trace ID 唯一，Session 相同 |
| Generation 去重与 Token | 已通过 | 假模型执行次数和内存 Span 数量一致 |
| Tool 业务拒绝/RPC 失败/恢复 | 已通过 | Tool level、status 与 Root 最终状态断言 |
| Draft/Confirm 固定回复 | 已通过 | 成功路径只有一次模型调用，失败路径继续第二次模型调用 |
| exporter 慢响应/鉴权失败/队列满 | 已通过 | 本地假 OTLP HTTP 服务，100 个 Turn 不阻塞业务路径 |
| 共享消息并发读取 | 已通过 | 观测副本丢弃 Eino 内部 `Extra`，相关包 race 通过 |
| 密钥和 Authorization 脱敏 | 已通过 | 假 Key 导出快照不含原值且 JSON 保持有效 |

完整自动化命令：

```bash
cd services/companion-agent-service
GOCACHE=/tmp/companion-agent-gocache go test ./...
GOCACHE=/tmp/companion-agent-gocache go test -race ./...
```

仓库内各服务是独立 Go Module，不再保留根 `go.work`。这是本次依赖检查后的明确调整：Langfuse/OTEL 使用拆分后的 genproto，若用 workspace 合并所有模块依赖图，会与历史间接依赖中的整体 genproto 形成重叠包。Contract、Business、Agent 分别进入自身目录验证，部署边界与运行时依赖保持一致。

## 本地平台验收

| 检查 | 当前结果 | 说明 |
|---|---|---|
| Compose 配置与镜像 digest | 已通过 | 六个服务使用固定 digest，所有服务 `restart: "no"` |
| Web/依赖健康 | 已通过 | Web、Postgres、Redis、ClickHouse、MinIO healthy；worker 正常运行 |
| 账号、Organization、Project | 已通过 | 已创建本地 `companion-agent-local` Project |
| 真实业务五类场景 | 待最终验收 | 需要新项目 API Key 后运行隔离 E2E |
| 真实 Trace 入库与页面检查 | 待最终验收 | 需要确认 User、Session、Release、父子关系、Token 与延迟 |
| Langfuse 停机旁路 | 待最终验收 | 停止平台后再次执行真实业务请求并记录结果 |
| 代表性 Trace 截图 | 待最终验收 | 与 flowlog 的 request ID/trace ID 对照保存 |

真实回归必须使用隔离 MongoDB、Redis Key 前缀和测试端口。Seed/reset 只能清理该范围。验收完成后把上表更新为实际结果；任何环境阻塞应保留原始错误和未覆盖边界，不能用 exporter 请求成功代替 Trace 已入库。

部署、启停和排障命令见 [deploy/langfuse/README.md](../deploy/langfuse/README.md)，固定版本见 [deploy/langfuse/versions.md](../deploy/langfuse/versions.md)。
