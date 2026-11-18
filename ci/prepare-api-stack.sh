#!/usr/bin/env bash
set -euo pipefail
# Inputs: trusted pinned baseline, untrusted candidate checkout, output runtime dir.
baseline=$1
candidate=$2
runtime=$3
mkdir -p "$runtime"
cp -a "$baseline/docker/." "$runtime/"
# Pin all published service ports to loopback; preserve already-bound ports.
sed -i -E '/^      - ("?)\$\{VB_BIND_ADDR/! s/^(      - "?)\$\{([^}]+)\}:([0-9]+)("?)$/\1127.0.0.1:${\2}:\3\4/' "$runtime/docker-compose.yml" "$runtime/docker-compose-base.yml"
cat >> "$runtime/.env" <<'ENV'
DOC_ENGINE=seekdb
DB_TYPE=mysql
COMPOSE_PROFILES=seekdb,cpu,ragflow-go,tei-cpu,deepdoc,metadata-mysql
TEI_MODEL=BAAI/bge-small-en-v1.5
RAGFLOW_IMAGE=seekdb-api-candidate:local
RAGFLOW_DEV_MODE=true
SVR_WEB_HTTP_PORT=8080
SVR_WEB_HTTPS_PORT=8443
ENV
# Fixed trusted Dockerfile/compose/entrypoint, never PR-controlled host instructions.
cp "$baseline/Dockerfile" "$candidate/Dockerfile.api-ci"
# No frontend build needed for direct Go API calls. Keep server native CGO build.
sed -i '/^COPY --from=web-builder \/ragflow\/web\/dist /c\RUN mkdir -p /ragflow/web/dist' "$candidate/Dockerfile.api-ci"
sed -i '/^COPY --from=web-builder \/ragflow\/VERSION /c\RUN echo api-ci > /ragflow/VERSION' "$candidate/Dockerfile.api-ci"
# Base Dockerfile COPY statements also use these files; replace candidate config.
cp "$baseline/Dockerfile" "$candidate/Dockerfile"
cp -a "$baseline/docker/." "$candidate/docker/"
cat > "$runtime/api.override.yml" <<'YAML'
services:
  seekdb:
    image: oceanbase/seekdb:1.4.0-100000172026082615@sha256:4db4961869d554313520d6081fca29265ba6da6db7313311bd8acbed771a956b
  tei-cpu:
    # Stock upstream command expects a baked /data/model directory. Download the
    # public small model directly instead; no token or proprietary model.
    command: ["--model-id", "BAAI/bge-small-en-v1.5", "--auto-truncate"]
    environment:
      HF_ENDPOINT: https://huggingface.co
YAML
