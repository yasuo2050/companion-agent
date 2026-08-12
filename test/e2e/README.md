# 端到端测试

先完成 Seed 并启动两个服务，再执行：

```bash
E2E_BASE_URL=http://127.0.0.1:8080 go test -tags=integration -v
```

测试通过 HTTP + SSE 黑盒验证三个画像的差异化推荐、历史复购、Draft Preview、明确确认、必要补问、无候选不虚构和 SSE 首尾事件顺序。默认单元测试不访问 Ark；集成测试会创建 Mock Draft/Order，执行前后可用 `go run ./cmd/seed --reset` 恢复固定状态。
