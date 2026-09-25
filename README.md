# 陪玩下单 Agent 与知识问答

RAG 知识平台、完整本机 Compose、演示身份和跨服务评测位于并列仓库 [`../companion-ai-knowledge-platform`](../companion-ai-knowledge-platform/README.md)。本仓库保留原有 Draft/Confirm 下单链路，并新增签名身份、知识检索工具、只读订单查询和 SSE 引用事件。

本仓库采用 Service-first Go Monorepo。仓库内有两个常驻后端服务、一个共享协议模块和一个 P1 实验模块：

```text
services/companion-agent-service   Agent Runtime，持有 Redis Conversation
services/mock-business-service    Mock 业务后台，持有 MongoDB 业务事实
contracts/companion-business      两个服务唯一允许共享的 gRPC Contract
experiments/profile-extraction    P1 只读画像提取实验
test/e2e                          跨服务端到端测试
deploy/langfuse                   同仓库、独立数据卷的本地观测部署
```

两个服务不得共享 `internal` 实现，也不得绕过 gRPC 直接访问对方数据。仓库不使用 `go.work` 合并依赖图；每个 Module 独立解析依赖、构建和测试，避免旁路观测依赖改变其他服务的版本选择。

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

                        ↘ search_knowledge → Knowledge Service → Dify 已授权片段
                        ↘ get_order → Business Service 只读校验订单归属
```

Redis 保存完整 User / Assistant ToolCall / ToolResult / Assistant Final 消息并刷新 24 小时 TTL；MongoDB 保存画像、陪玩、历史订单、Draft 和 Order。

## 前置要求

- Go 1.25
- MongoDB：`127.0.0.1:27017`
- Redis：`127.0.0.1:6379`
- 已开通的火山方舟模型与 API Key
- 并列的 Knowledge Service 与同一 `KNOWLEDGE_JWT_SECRET`；聊天请求必须携带演示登录签发的 Bearer 令牌

默认数据库固定为 `companion_agent_mvp`，Redis Key 前缀固定为 `companion-agent:mvp:`。`seed --reset` 只删除这两个范围，不执行 `FLUSHDB`。

## 从干净环境启动

完整演示请先按 [知识平台启动说明](../companion-ai-knowledge-platform/README.md)运行 `../companion-ai-knowledge-platform/scripts/start.sh`。该脚本启动 MongoDB、Redis、业务服务、Agent、知识服务、管理台与 Dify，并装载演示订单。聊天页使用知识平台 `.env` 中的演示登录密码；Agent 只使用令牌身份，请求体中的 `user_id` 不参与授权。

若单独启动本仓库，先确保知识服务已运行，再按下列步骤启动业务服务与 Agent；Agent 模块的 `.env` 还需填写与知识平台一致的 `KNOWLEDGE_JWT_SECRET` 和 `KNOWLEDGE_API_URL=http://127.0.0.1:8082`。

终端一，在 Mock Business Module 中恢复 Demo 数据并启动 gRPC：

```bash
cd services/mock-business-service
go run ./cmd/seed --reset
go run ./cmd/server
```

终端二，首次在 Agent 模块目录创建本地 `.env`：

```bash
cd services/companion-agent-service
# .env 内填写：
# ARK_API_KEY=<本地 Ark Key>
# ARK_MODEL_ID=doubao-seed-2-1-turbo-260628
# KNOWLEDGE_JWT_SECRET=<与知识平台相同的签名密钥>
# KNOWLEDGE_API_URL=http://127.0.0.1:8082
```

之后可直接启动，无需重复 `export`：

```bash
go run ./cmd/ark-smoke
go run ./cmd/server
```

打开 [http://127.0.0.1:8080/](http://127.0.0.1:8080/)，以租户 A 的 `10001`、`10002`、`10003` 等演示身份登录。

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
| `KNOWLEDGE_JWT_SECRET` | 无，必填且至少 32 字节 | Agent 身份验签 |
| `KNOWLEDGE_API_URL` | `http://127.0.0.1:8082` | Agent 知识工具 |
| `ARK_BASE_URL` | `https://ark.cn-beijing.volces.com/api/v3` | Agent |
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
| `LANGFUSE_ENABLED` | `false` | Agent 旁路观测 |
| `LANGFUSE_HOST` | `http://127.0.0.1:3000` | Agent 旁路观测 |
| `LANGFUSE_PUBLIC_KEY` / `LANGFUSE_SECRET_KEY` | 无，启用时必填 | Agent 旁路观测 |
| `LANGFUSE_RELEASE` | `local-dev` | Agent 旁路观测 |

Langfuse 的本地初始化、启停和排障见 [`deploy/langfuse/README.md`](deploy/langfuse/README.md)，接入边界与验收记录见 [`docs/langfuse接入与验收.md`](docs/langfuse接入与验收.md)。

## 测试

进入每个 Module 的目录独立验证：

```bash
cd contracts/companion-business && go test ./...
cd services/mock-business-service && go test ./... && go test -race ./...
cd services/companion-agent-service && go test ./... && go test -race ./...
```

先运行知识平台，再用隔离容器运行真实黑盒 E2E：

```bash
cd test/e2e
./run-isolated.sh
```

E2E 会从并列知识平台的本机 `.env` 获取演示登录密码并换取令牌，真实访问 Ark，只在隔离的 MongoDB 和 Redis 中创建可重置的 Draft/Order。若配置文件路径不同，设置 `E2E_KNOWLEDGE_ENV`；也可设置 `E2E_DEMO_PASSWORD`。默认单元测试不访问外部模型。

## 安全说明

项目启动时会读取 Agent 模块当前目录的 `.env`，同名系统环境变量优先。`.env` 已被 Git 忽略，不得提交或对外分发。API Key 不写入代码、README、MongoDB、Redis、Trace、测试快照或前端。本地对话流水会记录用户原文、Tool 参数和业务结果，日志目录也已忽略提交。
