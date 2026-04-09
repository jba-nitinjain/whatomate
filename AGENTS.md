# AGENTS

Read this file before starting any complex task in this repository.

## Repo Map

- Backend entrypoint: [cmd/whatomate/main.go](/e:/xampp/htdocs/bu-so/whatomate/cmd/whatomate/main.go)
- Backend handlers: [internal/handlers](/e:/xampp/htdocs/bu-so/whatomate/internal/handlers)
- Data models: [internal/models/models.go](/e:/xampp/htdocs/bu-so/whatomate/internal/models/models.go)
- Database setup and migrations: [internal/database](/e:/xampp/htdocs/bu-so/whatomate/internal/database)
- Frontend app: [frontend/src](/e:/xampp/htdocs/bu-so/whatomate/frontend/src)
- Settings views: [frontend/src/views/settings](/e:/xampp/htdocs/bu-so/whatomate/frontend/src/views/settings)
- Shared frontend components: [frontend/src/components](/e:/xampp/htdocs/bu-so/whatomate/frontend/src/components)
- API client layer: [frontend/src/services/api.ts](/e:/xampp/htdocs/bu-so/whatomate/frontend/src/services/api.ts)
- Docker assets: [docker](/e:/xampp/htdocs/bu-so/whatomate/docker)
- External API docs: [EXTERNAL_MESSAGE_API.md](/e:/xampp/htdocs/bu-so/whatomate/EXTERNAL_MESSAGE_API.md), [EXTERNAL_TEMPLATE_SEND_API.md](/e:/xampp/htdocs/bu-so/whatomate/EXTERNAL_TEMPLATE_SEND_API.md)

## Working Rules

- Keep frontend and backend changes aligned when an API contract changes.
- Prefer small, focused files. If a file grows beyond roughly 150-200 lines because of a new feature, split it.
- Update this file or [README.md](/e:/xampp/htdocs/bu-so/whatomate/README.md) when adding a new workflow, root-level doc, or architectural pattern that future agents need to know.
- Do not assume Docker publishing conventions from memory; use [DOCKER_HUB_PUBLISHING.md](/e:/xampp/htdocs/bu-so/whatomate/DOCKER_HUB_PUBLISHING.md).

## Verification

- Frontend typecheck: `cd frontend && npm run typecheck`
- Frontend build: `cd frontend && npm run build`
- Backend tests: `go test ./...`
- Handler-focused tests: `go test ./internal/handlers -count=1`

If the local shell does not have Go installed, note that explicitly instead of claiming backend verification.

## Current Important Conventions

- External message ingestion supports super-admin routing by `phone_number_id` to the correct organization/account.
- Legacy misrouted chats can be reviewed from the super-admin-only Settings UI using all message history, with safe moves plus explicit manual approval for merge-required cases.
- The chat repair backend endpoints are preview/apply flows under `/api/admin/chat-repair`.

## Docker Publishing Rule

Always publish the application image as:

- `nikyjain/whatomate:latest`

Always publish as multi-arch for:

- `linux/amd64`
- `linux/arm64`

Use [DOCKER_HUB_PUBLISHING.md](/e:/xampp/htdocs/bu-so/whatomate/DOCKER_HUB_PUBLISHING.md) as the source of truth.

Preferred command from repo root:

- `make docker-push`

The Docker publish path also maintains a remote Buildx cache at:

- `nikyjain/whatomate:buildcache`
