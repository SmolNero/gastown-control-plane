# Gas Town Control Plane

[![CI](https://github.com/SmolNero/gastown-control-plane/actions/workflows/ci.yml/badge.svg)](https://github.com/SmolNero/gastown-control-plane/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/SmolNero/gastown-control-plane)](https://github.com/SmolNero/gastown-control-plane/releases/latest)
[![License](https://img.shields.io/github/license/SmolNero/gastown-control-plane)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/SmolNero/gastown-control-plane)](go.mod)

Hosted control plane for Gas Town workspaces. This service ingests events and snapshots from local rigs, stores them in Postgres, and serves a clean dashboard for operators who want a single view of multi-agent work.

This is an independent project and is not affiliated with the official Gas Town project.

## Diagrams

![Architecture diagram](docs/images/architecture.svg)

![Data flow diagram](docs/images/dataflow.svg)

## What you get

- API server in Go for events, snapshots, and queries
- Web UI for rigs, agents, convoys, hooks, and activity
- `gt-agent` binary that runs locally and ships telemetry
- Postgres schema + migrations for multi-tenant workspaces

## Quick start (local)

1) Start Postgres

```bash
docker compose up -d
```

2) Run migrations and create a workspace

```bash
export GTCP_DATABASE_URL="postgres://gtcp:gtcp@localhost:5432/gtcp?sslmode=disable"
go run ./cmd/admin migrate
go run ./cmd/admin create-all --org "Local" --workspace "dev" --key-name "local"
```

3) Start the API server

```bash
export GTCP_AUTO_MIGRATE=true
go run ./cmd/api
```

4) Open the UI

```
http://localhost:8080
```

Paste the API key from step 2.

5) Run the local agent

```bash
export GTCP_API_KEY="<api_key_from_step_2>"
export GTCP_WORKSPACE="$HOME/gt"   # your Gas Town workspace
go run ./cmd/agent run
```

You should see rigs and agents populate within a minute.

## Components

### API server (`cmd/api`)
Serves the dashboard and a JSON API.

Endpoints:

- `POST /v1/events` ingest events
- `POST /v1/snapshots` ingest a full snapshot
- `GET /v1/rigs`
- `GET /v1/agents`
- `GET /v1/convoys`
- `GET /v1/hooks`
- `GET /v1/activity?limit=100`
- `GET /v1/alerts`
- `GET /v1/info`

Auth is a simple API key via `Authorization: Bearer <key>` or `X-API-Key`.

### Local agent (`cmd/agent`)
Runs inside your workspace and ships data to the control plane.

Behavior:

- Heartbeat events every `GTCP_POLL_INTERVAL` (default 15s)
- Full snapshot every `GTCP_SNAPSHOT_INTERVAL` (default 60s)
- Optional spool directory for custom JSON events

Spool directory:

- Default: `<workspace>/.gtcp/spool`
- Drop `.json` (single event or array) or `.jsonl` (one event per line)
- Files are renamed to `.sent` after successful upload

Manual event example:

```bash
go run ./cmd/agent emit \
  --type convoy_started \
  --rig myproject \
  --convoy "Auth System" \
  --status running \
  --message "Convoy launched"
```

## Configuration

API server:

- `GTCP_HTTP_ADDR` (default `:8080`)
- `GTCP_DATABASE_URL` (default `postgres://gtcp:gtcp@localhost:5432/gtcp?sslmode=disable`)
- `GTCP_AUTO_MIGRATE` (`true`/`false`)
- `GTCP_MAX_EVENT_BYTES` (default `1048576`)
- `GTCP_MAX_SNAPSHOT_BYTES` (default `4194304`)
- `GTCP_RATE_LIMIT_PER_MINUTE` (default `600`)
- `GTCP_EVENT_RETENTION_DAYS` (default `30`)
- `GTCP_SNAPSHOT_RETENTION_DAYS` (default `30`)
- `GTCP_PRUNE_INTERVAL` (default `1h`)
- `GTCP_VERSION` (default `dev`)
- `GTCP_AGENT_DOWNLOAD_BASE_URL` (optional, used for update hints)

Agent:

- `GTCP_API_URL` (default `http://localhost:8080`)
- `GTCP_API_KEY` (required)
- `GTCP_WORKSPACE` (default current directory)
- `GTCP_SPOOL_DIR` (default `<workspace>/.gtcp/spool`)
- `GTCP_POLL_INTERVAL` (default `15s`)
- `GTCP_SNAPSHOT_INTERVAL` (default `60s`)
- `GTCP_AGENT_NAME` (default hostname)
- `GTCP_SOURCE` (default `gt-agent`)
- `GTCP_CHECK_UPDATES` (default `true`)
- `GTCP_AGENT_VERSION` (default `dev`)

## Docker

Build the image:

```bash
docker build -t gtcp .
```

Run the API server:

```bash
docker run --rm -p 8080:8080 \
  -e GTCP_DATABASE_URL="postgres://gtcp:gtcp@host.docker.internal:5432/gtcp?sslmode=disable" \
  gtcp
```

Run the agent (mount your workspace):

```bash
docker run --rm \
  -e GTCP_API_URL="http://host.docker.internal:8080" \
  -e GTCP_API_KEY="<api_key>" \
  -e GTCP_WORKSPACE="/workspace" \
  -v "$HOME/gt:/workspace" \
  gtcp /usr/local/bin/gt-agent run
```

### Docker Compose

Run Postgres + API:

```bash
docker compose up -d
```

One-command dev setup (runs Postgres, migrates, creates an API key, starts API + agent):

```bash
./scripts/dev-up.sh
```

Optional: copy `.env.example` to `.env` and fill in `GTCP_API_KEY` for the agent profile.

Run the agent (profiled):

```bash
export GTCP_API_KEY="<api_key>"
export GTCP_WORKSPACE_PATH="$HOME/gt"
docker compose --profile agent up -d agent
```

## Event format

```json
{
  "schema_version": 1,
  "type": "heartbeat",
  "source": "gt-agent",
  "rig": "myproject",
  "agent": "mayor",
  "status": "ok",
  "message": "alive",
  "payload": {"details": "optional"},
  "occurred_at": "2026-02-26T12:34:56Z"
}
```

## Admin commands

```bash
go run ./cmd/admin migrate
go run ./cmd/admin create-all --org "Local" --workspace "dev" --key-name "local"
go run ./cmd/admin list-api-keys --workspace-id <uuid>
go run ./cmd/admin revoke-api-key --key-id <uuid>
```

## Notes

- The agent discovers rigs by scanning for `crew/` or `hooks/` directories inside the workspace.
- Convoy discovery is currently event-driven, so emit events or extend the snapshot scanner to suit your setup.
- For production, place the API server behind TLS and use a managed Postgres instance.

## Security note

The API uses a simple API key for authentication and is intended for trusted networks. If you expose this publicly, add TLS termination and a stronger auth layer (OAuth/JWT) first.

## Data handling note

The control plane stores events and snapshots in Postgres. Review and configure retention settings before sending sensitive data.

## Release

Tag and push to publish binaries and Docker images via GitHub Actions:

```bash
git tag v0.1.0
git push origin v0.1.0
```

To enable agent update hints, set:

```
GTCP_AGENT_DOWNLOAD_BASE_URL=https://github.com/SmolNero/gastown-control-plane/releases/download
```

## Recommended labels

These labels are used by the release notes template in `.github/release.yml`:

- `feature`
- `enhancement`
- `bug`
- `docs`
- `skip-changelog`

Create them quickly with GitHub CLI:

```bash
gh label create feature --description "New feature" --color 0E8A16
gh label create enhancement --description "Enhancement" --color 1D76DB
gh label create bug --description "Bug fix" --color D93F0B
gh label create docs --description "Documentation" --color 5319E7
gh label create skip-changelog --description "Skip release notes" --color C5DEF5
```

## Testing

```bash
go test ./...
```

The integration test uses `embedded-postgres` and will download a local Postgres binary on first run. It is skipped on Windows.

## License

MIT
