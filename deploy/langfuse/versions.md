# Langfuse 本地部署版本

验证日期：2026-09-08。

本目录根据 Langfuse 官方 v4 Docker Compose 拆取本地单机所需服务，并收紧为仅本机端口、独立 named volumes、`restart: "no"`。上游参考：

- 官方部署说明：<https://langfuse.com/self-hosting/deployment/docker-compose>
- 官方 Compose：<https://github.com/langfuse/langfuse/blob/2f00fd4b504ae9de37dcd0a02b01ee02ee67d3dd/docker-compose.yml>
- Langfuse 镜像源码提交：`2f00fd4b504ae9de37dcd0a02b01ee02ee67d3dd`

| 服务 | 验证版本 | 镜像 digest |
|---|---|---|
| Langfuse Web | `4.30.0` | `sha256:376b96bdd8725e74171a3d0cdd9115c8265d4209df95cb26c47370cb3b32329a` |
| Langfuse Worker | `4.30.0` | `sha256:00bc13c8da68fb77c55bcba0c1c16377f1f360ffb17206dbc708912ce4e3afb5` |
| PostgreSQL | `17` 系列镜像 | `sha256:67f41722b7a8cbdb868a44a4995c846eddfdc2973bccb291ce937dce88ad5675` |
| Redis | `7-alpine` 系列镜像 | `sha256:e7723ff73d963f5cc6d9c4643ea3d989527a402a319239054e9472a7fb9219a2` |
| ClickHouse | `25.12` 系列镜像 | `sha256:8a790dd3468db22b1d4e7b18a176f378ff5ff6053b9c48dd4ea1fa71a24c5ba6` |
| MinIO | Chainguard MinIO 镜像 | `sha256:2a30e1a6d9e024631e9fed795773c40de5f819db80875f23dc00f7a4ca4f2246` |

Go 侧版本：

| 依赖 | 固定版本 |
|---|---|
| Eino | `v0.9.14` |
| Eino Langfuse Callback v2 | `v2.0.2-0.20260904094655-8d8ff4e79992` |
| OpenTelemetry Go | `v1.38.0` |

升级时先在独立分支更新 digest，执行 Compose 健康检查、Agent 单测/race 和真实 Trace 回归，再修改本文件。不要使用浮动 `latest` 作为已验收版本。
