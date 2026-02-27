#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

export GTCP_DATABASE_URL="${GTCP_DATABASE_URL:-postgres://gtcp:gtcp@localhost:5432/gtcp?sslmode=disable}"
export GTCP_WORKSPACE_PATH="${GTCP_WORKSPACE_PATH:-$HOME/gt}"

echo "Starting Postgres..."
docker compose up -d postgres

echo "Running migrations..."
go run ./cmd/admin migrate

if [[ -z "${GTCP_API_KEY:-}" ]]; then
  echo "Creating org/workspace/api key..."
  output=$(go run ./cmd/admin create-all --org "Local" --workspace "dev" --key-name "local")
  echo "$output"
  api_key=$(echo "$output" | awk -F= '/^api_key=/{print $2}')
  if [[ -z "$api_key" ]]; then
    echo "Failed to parse api key. Set GTCP_API_KEY and re-run." >&2
    exit 1
  fi
  export GTCP_API_KEY="$api_key"
fi

echo "Starting API..."
docker compose up -d api

echo "Starting agent..."
GTCP_API_KEY="$GTCP_API_KEY" GTCP_WORKSPACE_PATH="$GTCP_WORKSPACE_PATH" docker compose --profile agent up -d agent

echo "Done. Open http://localhost:8080"
