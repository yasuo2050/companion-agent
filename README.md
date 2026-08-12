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

当前目录只是开发骨架；实现顺序和验收标准见外层 `方案文档/陪玩下单Agent_MVP_具体实施方案_含MongoDB_Redis_Ark.md`。
