# 陪玩下单 Agent MVP

本仓库采用 Service-first Go Monorepo。仓库内有两个常驻后端服务、一个共享协议模块和一个 P1 实验模块：

```text
services/companion-agent-service   Agent Runtime，持有 Redis Conversation
services/mock-business-service    Mock 业务后台，持有 MongoDB 业务事实
contracts/companion-business      两个服务唯一允许共享的 gRPC Contract
experiments/profile-extraction    P1 只读画像提取实验
test/e2e                          跨服务端到端测试
```

两个服务不得共享 `internal` 实现，也不得绕过 gRPC 直接访问对方数据。根目录 `go.work` 只用于统一本地开发，不合并服务的构建和部署边界。

## 开发语言规范

- Git 提交标题和正文使用中文，可保留 `feat:`、`fix:`、`test:`、`docs:`、`chore:` 等类型前缀。
- 手写代码注释、README、配置说明和测试说明使用中文。
- Go 标识符、Proto/JSON 字段、环境变量和错误码保持规范英文命名。
- 生成代码和第三方代码不手工修改注释。

## 已实现链路

```text
Chat UI → HTTP POST + SSE → Eino ChatModelAgent → Ark LLM 自主 Tool Calling
→ Go Tool → gRPC → Mock Business Service → MongoDB
→ Draft Preview → 用户明确确认 → Mock Order
```

Redis 保存完整 User / Assistant ToolCall / ToolResult / Assistant Final 消息并刷新 24 小时 TTL；MongoDB 保存画像、陪玩、历史订单、Draft 和 Order。

## 前置要求

- Go 1.25
- MongoDB：`127.0.0.1:27017`
- Redis：`127.0.0.1:6379`
- 已开通的火山方舟模型与 API Key

默认数据库固定为 `companion_agent_mvp`，Redis Key 前缀固定为 `companion-agent:mvp:`。`seed --reset` 只删除这两个范围，不执行 `FLUSHDB`。

## 从干净环境启动

终端一，在 Mock Business Module 中恢复 Demo 数据并启动 gRPC：

```bash
cd services/mock-business-service
go run ./cmd/seed --reset
go run ./cmd/server
```

终端二，只在当前进程环境注入 Ark 凭证并启动 Agent：

```bash
cd services/companion-agent-service
export ARK_API_KEY="<从火山方舟控制台复制后仅注入当前终端>"
export ARK_MODEL_ID="doubao-seed-2-1-turbo-260628"
go run ./cmd/ark-smoke
go run ./cmd/server
```

打开 [http://127.0.0.1:8080/](http://127.0.0.1:8080/)，使用 `10001`、`10002`、`10003` 可演示不同画像推荐。

健康检查：

```bash
curl http://127.0.0.1:8080/healthz
curl http://127.0.0.1:8080/readyz
```

## 查看对话调用流程

Agent 会把每轮的用户输入、画像加载、每次大模型调用、Tool 参数与结果、最终回复及耗时写入流水文件。在项目根目录执行：

```bash
tail -f services/companion-agent-service/logs/agent-flow.log
```

只看某个对话：

```bash
tail -f services/companion-agent-service/logs/agent-flow.log | grep 'conversation_id=demo-10002-04'
```

每条记录都有 `flow_step`，典型顺序为 `turn_started → profile_load_* → llm_started → llm_tool_call → tool_started → tool_result → llm_started → llm_final_response → turn_completed`。日志默认追加写入，可用 `AGENT_FLOW_LOG_PATH` 修改位置。

## 环境变量

| 变量 | 默认值 | 归属 |
|---|---|---|
| `ARK_API_KEY` | 无，必填 | Agent |
| `ARK_MODEL_ID` | 无，必填 | Agent |
| `ARK_BASE_URL` | `https://ark.cn-beijing.volces.com/api/v3` | Agent |
| `LLM_MODEL_ALIAS` | `doubao-seed-2.1-turbo` | Agent Trace |
| `LLM_MAX_TOKENS` | `2048` | Agent |
| `LLM_TIMEOUT` | `30s` | Agent |
| `LLM_RETRY_TIMES` | `1` | Agent |
| `AGENT_MAX_ITERATIONS` | `6` | Agent |
| `AGENT_HTTP_ADDR` | `:8080` | Agent |
| `AGENT_FLOW_LOG_PATH` | `logs/agent-flow.log` | Agent 对话流水 |
| `BUSINESS_GRPC_ADDR` | Agent 侧 `127.0.0.1:9090`，Business 侧 `:9090` | 两服务 |
| `MONGO_URI` | `mongodb://127.0.0.1:27017` | Business |
| `MONGO_DATABASE` | `companion_agent_mvp` | Business |
| `DRAFT_TTL` | `15m` | Business |
| `REDIS_ADDR` | `127.0.0.1:6379` | Agent / Seed Reset |
| `REDIS_DB` | `0` | Agent / Seed Reset |
| `REDIS_KEY_PREFIX` | `companion-agent:mvp:` | Agent / Seed Reset |
| `CONVERSATION_TTL` | `24h` | Agent |

## 测试

每个 Module 可脱离 `go.work` 独立验证：

```bash
cd contracts/companion-business && GOWORK=off go test ./...
cd services/mock-business-service && GOWORK=off go test ./... && GOWORK=off go test -race ./...
cd services/companion-agent-service && GOWORK=off go test ./... && GOWORK=off go test -race ./...
```

两个服务启动且执行干净 Seed 后，运行真实黑盒 E2E：

```bash
cd test/e2e
E2E_BASE_URL=http://127.0.0.1:8080 go test -tags=integration -count=1 -v
```

E2E 会真实访问 Ark，并在 MongoDB 创建可重置的 Draft/Order。默认单元测试不访问外部模型。

## 安全说明

项目不读取 `.env`，也不包含 API Key 默认值。API Key 不写入代码、配置、README、MongoDB、Redis、Trace、测试快照或前端。本地对话流水会记录用户原文、Tool 参数和业务结果，便于 MVP 调试；日志目录已忽略提交，不得将该文件对外分发。
