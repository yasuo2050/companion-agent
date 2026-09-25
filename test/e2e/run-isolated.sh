#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
env_path="${E2E_KNOWLEDGE_ENV:-../../../companion-ai-knowledge-platform/.env}"
if [[ ! -f "$env_path" ]]; then
  echo "缺少知识平台本机配置：$env_path" >&2
  exit 1
fi
env_path="$(cd "$(dirname "$env_path")" && pwd)/$(basename "$env_path")"
compose=(docker compose --env-file "$env_path" -p companion-rag-e2e -f compose.yaml)
cleanup() { "${compose[@]}" stop >/dev/null 2>&1 || true; }
trap cleanup EXIT
"${compose[@]}" up -d --build mongo redis business agent
"${compose[@]}" --profile seed run --rm seed --reset
ready=false
for _ in {1..30}; do
  if curl -fsS http://127.0.0.1:18080/readyz >/dev/null 2>&1; then ready=true; break; fi
  sleep 1
done
if [[ "$ready" != true ]]; then
  echo '隔离 Agent 未就绪，请检查 companion-rag-e2e 容器日志。' >&2
  exit 1
fi
E2E_BASE_URL=http://127.0.0.1:18080 E2E_KNOWLEDGE_ENV="$env_path" go test -tags=integration -count=1 -v ./...
