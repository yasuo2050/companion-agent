# 隔离端到端测试

先启动并列知识平台中的 Knowledge Service，确保 `http://127.0.0.1:8082/health` 可访问，然后在本目录运行：

```bash
./run-isolated.sh
```

脚本启动独立的 MongoDB、Redis、Business 和 Agent 容器，读取知识平台本机 `.env` 中的签名密钥与 Ark 凭据，在**隔离数据库和 Redis 前缀**执行 `seed --reset` 后运行真实 Ark 黑盒回归。Agent 测试入口为 `127.0.0.1:18080`；结束时停止测试容器并保留测试数据卷，不删除主项目数据。成功标志为 Go E2E 测试 `PASS` 和脚本退出码 0；停止运行中的脚本可按 Ctrl+C，退出钩子会停止测试容器。

若本机知识平台配置不在并列目录，设置 `E2E_KNOWLEDGE_ENV` 为其 `.env` 的绝对路径。测试覆盖画像推荐、最近订单复购、Draft Preview、确认订单、必要补问、无候选说明和 SSE 首尾事件。模型调用和订单创建均为真实调用，默认单元测试不访问 Ark。
