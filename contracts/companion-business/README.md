# Companion Business Contract

本模块是 `companion-agent-service` 与 `mock-business-service` 唯一共享的代码边界，定义 5 个一元 gRPC RPC、业务实体和结构化业务错误，不放置业务规则、Repository 或 Agent 逻辑。

重新生成代码：

```bash
protoc -I proto \
  --go_out=. --go_opt=module=companion-agent/contracts/companion-business \
  --go-grpc_out=. --go-grpc_opt=module=companion-agent/contracts/companion-business \
  proto/companion_business.proto
```
