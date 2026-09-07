# Langfuse 本地部署

本目录在 `companion-agent` 仓库内独立管理 Langfuse v4 的本地部署配置。它不包含 Langfuse 源码，也不创建嵌套 Git。Postgres、Redis、ClickHouse 和 MinIO 使用独立 named volumes，不接触 Companion Agent 的业务数据。

## 初始化

要求本机已经运行 OrbStack 或兼容的 Docker Engine，并可使用 `docker compose`。本项目只发布两个本机端口：

- Langfuse Web：`127.0.0.1:3000`
- MinIO S3 API：`127.0.0.1:19090`

先确认端口未被其他进程占用。命令返回非零代表当前没有监听者，可以继续：

```bash
nc -zv 127.0.0.1 3000
nc -zv 127.0.0.1 19090
```

首次创建只保存在本机的随机 Secret：

```bash
cd deploy/langfuse
umask 077
{
  printf 'POSTGRES_PASSWORD=%s\n' "$(openssl rand -hex 32)"
  printf 'SALT=%s\n' "$(openssl rand -hex 32)"
  printf 'ENCRYPTION_KEY=%s\n' "$(openssl rand -hex 32)"
  printf 'NEXTAUTH_SECRET=%s\n' "$(openssl rand -hex 32)"
  printf 'CLICKHOUSE_PASSWORD=%s\n' "$(openssl rand -hex 32)"
  printf 'REDIS_AUTH=%s\n' "$(openssl rand -hex 32)"
  printf 'MINIO_ROOT_PASSWORD=%s\n' "$(openssl rand -hex 32)"
} > .env
```

`.env` 已被仓库根 `.gitignore` 忽略。不要把文件内容复制到提交、日志或工单中。

## 启动和首次配置

在仓库根目录执行：

```bash
docker compose --project-name companion-langfuse \
  --env-file deploy/langfuse/.env \
  -f deploy/langfuse/compose.yaml config --quiet

docker compose --project-name companion-langfuse \
  --env-file deploy/langfuse/.env \
  -f deploy/langfuse/compose.yaml up -d
```

查看状态，除没有声明 healthcheck 的 worker 外，其余服务应显示 `healthy`：

```bash
docker compose --project-name companion-langfuse \
  --env-file deploy/langfuse/.env \
  -f deploy/langfuse/compose.yaml ps

curl --fail http://127.0.0.1:3000/api/public/health
```

打开 `http://127.0.0.1:3000`，首次创建本地账号、Organization 和 `companion-agent-local` Project，然后在 Project Settings 中生成 API Keys。账号凭证和部署 Secret 只属于本地 Langfuse；Agent 只需要项目 Public Key 和 Secret Key。

## 日常停止和恢复

所有服务均设置 `restart: "no"`。保留容器与数据的正常停止方式是：

```bash
docker compose --project-name companion-langfuse \
  --env-file deploy/langfuse/.env \
  -f deploy/langfuse/compose.yaml stop
```

恢复已有环境：

```bash
docker compose --project-name companion-langfuse \
  --env-file deploy/langfuse/.env \
  -f deploy/langfuse/compose.yaml start
```

不要在日常启停时使用 `down -v`，它会删除本地 Trace 数据卷。

## Agent 接入

在 `services/companion-agent-service/.env` 追加以下本地配置：

```dotenv
LANGFUSE_ENABLED=true
LANGFUSE_HOST=http://127.0.0.1:3000
LANGFUSE_PUBLIC_KEY=<项目 Public Key>
LANGFUSE_SECRET_KEY=<项目 Secret Key>
LANGFUSE_SERVICE_NAME=companion-agent-service
LANGFUSE_ENVIRONMENT=local
LANGFUSE_RELEASE=<当前 Git SHA>
LANGFUSE_COLLAPSE_AGENT_INTERNAL_SPANS=true
```

用 `LANGFUSE_ENABLED=false` 可完全关闭 exporter。关闭时无需提供 Key，`/readyz` 也不会检查 Langfuse。启用但 Host 或 Key 无效时，服务会安全降级为 Noop，并继续处理业务请求。

## 排障

查看某个服务的最近日志：

```bash
docker compose --project-name companion-langfuse \
  --env-file deploy/langfuse/.env \
  -f deploy/langfuse/compose.yaml logs --tail=200 langfuse-web
```

- Web 未健康：先检查 Postgres、Redis、ClickHouse 和 MinIO 是否健康，再看 web/worker 日志。
- 3000 或 19090 被占用：识别监听进程并决定如何处理，不要让 Compose 自动改端口，也不要停止无关服务。
- Trace 未出现：确认 Agent 使用项目级 Key、Host 可访问，并等待最长约 10 秒的有限重试窗口；再通过 `request_id` 对照 `logs/agent-flow.log`。
- 图片或多模态上传失败：确认浏览器可访问 `127.0.0.1:19090`，容器内部仍应使用 `http://minio:9000`。

镜像与上游版本来源见 [versions.md](versions.md)。
