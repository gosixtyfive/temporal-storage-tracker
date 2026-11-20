# Architecture

How temporal-storage-tracker is built and how its parts connect. For what the
application does, see [`../brain/overview.md`](../brain/overview.md).

## Stack

| Layer | Technology |
|-------|------------|
| Workflow engine | Temporal, run locally with `temporal server start-dev` |
| Backend | Go: Temporal Go SDK and the standard `net/http` server |
| Frontend | Nuxt (Vue 3) |
| Persistence | Temporal itself; there is no application database |

## Components

```
┌──────────────┐  HTTP/JSON  ┌──────────────┐   gRPC   ┌──────────────────┐
│ Nuxt frontend│ ──────────▶ │   Go API     │ ───────▶ │ Temporal server  │
│ (forms,      │   /api/*    │ (Temporal    │          │ (dev server)     │
│  status,     │ ◀────────── │  client)     │ ◀─────── │                  │
│  dashboard)  │             └──────────────┘          └────────┬─────────┘
└──────────────┘                                                │ task queue
                                                                │ storage-tracker
                                                       ┌────────▼─────────┐
                                                       │    Go worker     │
                                                       │ (workflow +      │
                                                       │  activities)     │
                                                       └──────────────────┘
```

- **Nuxt frontend.** Pages for storing an item, viewing an item's status, and
  the admin dashboard. It talks only to the Go API. During development, Nuxt
  proxies `/api/*` to the Go API, so the browser needs no CORS setup.
- **Go API** (`cmd/api`). A thin HTTP layer over the Temporal client. It
  starts workflows, sends Updates, runs Queries, and lists workflows through
  Temporal visibility. It holds no state.
- **Go worker** (`cmd/worker`). Hosts the `StorageItemWorkflow` and its
  activities on the `storage-tracker` task queue.
- **Temporal server.** The local dev server. It is the system of record:
  each stored item is one workflow execution.

The API and worker are separate processes on purpose. That is how Temporal
apps are normally deployed, and the demo can show a workflow surviving a
worker restart.

## Data model: one workflow per item

Each stored item is one `StorageItemWorkflow` execution. The workflow ID is
the item ID (`item-<uuid>`), so the API addresses an item directly by its ID.

| Need | Temporal feature |
|------|------------------|
| Item details and current state | Workflow **Query** `getStatus` |
| Member collects / admin removes | Workflow **Update** `collect` / `remove` |
| Dashboard list of all items | **Visibility** list filtered by workflow type |
| Dashboard filtering by state | Custom **search attribute** `ItemStatus` |
| Reminder timing | Durable workflow **timers** |
| Sending notifications | **Activity** `SendNotification` |

Workflow internals are described in
[`storage-workflow.md`](storage-workflow.md).

## HTTP API

| Method | Path | Temporal call | Used by |
|--------|------|---------------|---------|
| `POST` | `/api/items` | Start `StorageItemWorkflow` | Store item form |
| `GET` | `/api/items` | List workflows with visibility, plus a status Query per item | Dashboard |
| `GET` | `/api/items/{id}` | Query `getStatus` | Item status view |
| `POST` | `/api/items/{id}/collect` | Update `collect` | Item status view (member) |
| `POST` | `/api/items/{id}/remove` | Update `remove` | Dashboard (admin) |
| `GET` | `/api/config` | none | Frontend (shows the current timing) |

Updates return the item's new status, or a validation error (for example,
collecting an item that was already removed). The UI can then react
immediately, without polling.

## Frontend

| Route | View | Role |
|-------|------|------|
| `/` | Landing page with role switcher | Both |
| `/store` | Store item form | Member |
| `/items/[id]` | Item status view: details, time left, notifications, Collect button | Member |
| `/dashboard` | All items with state, time left, notifications, Remove button | Admin |

- **Role switching.** A toggle in the header switches between Member and
  Admin. The choice is kept in browser storage. It only changes navigation
  and which actions are shown; nothing enforces it (see the non-goals in
  `brain/`).
- **Member identity.** The store form asks for a member name in plain text.
  There are no accounts.
- **Live updates.** The status view and dashboard poll the API every few
  seconds, which is enough for compressed demo timings.

## Time compression

The storage duration is configured on the API with `STORAGE_DURATION` (a Go
duration). The default is `168h` (7 days); a demo might use `7m`.

- The "expiring soon" reminder fires at **1/7 of the duration before expiry**
  (1 day out of 7; 1 minute out of 7 minutes).
- The API passes the duration into each workflow as **input** when it starts
  the workflow. Each item keeps the timing it was created with, and changing
  the setting never affects running workflows. That avoids non-determinism.

## Local development

Run each in its own terminal from the repo root (see the `Makefile`):

```bash
make temporal   # Temporal dev server with the ItemStatus search attribute
make worker     # Temporal worker
make api        # HTTP API on :8080; STORAGE_DURATION defaults to 7m here
cd frontend && npm run dev   # frontend
```

`make test`, `make vet` and `make build` cover the backend.

Environment variables:

| Variable | Used by | Default |
|----------|---------|---------|
| `TEMPORAL_ADDRESS` | API, worker | `localhost:7233` |
| `TEMPORAL_NAMESPACE` | API, worker | `default` |
| `STORAGE_DURATION` | API | `168h` (`make api` overrides it to `7m`) |
| `PORT` | API | `8080` |

The Temporal Web UI (http://localhost:8233) shows workflow histories next to
the app. That is a large part of the demo.

## Repository layout

```
Makefile                     Local dev targets
backend/                     Go module github.com/gosixtyfive/temporal-storage-tracker/backend
  cmd/api/                   HTTP server (Temporal client)
  cmd/worker/                Temporal worker
  internal/storage/          Workflow, activities, shared types, task queue name
  internal/temporalclient/   Temporal client setup from the environment
  internal/api/              HTTP handlers (planned)
frontend/                    Nuxt app (planned)
brain/                       What the project does
docs/                        How it is built
```

## Key decisions

- **No application database.** Temporal holds all state, through workflow
  state, Queries and visibility. This keeps the demo focused on Temporal. The
  cost is that the dashboard queries every item individually, which is fine
  at demo scale.
- **Updates rather than Signals** for collect and remove. Updates can reject
  invalid transitions and return the new state synchronously, which a form
  submission needs.
- **Duration passed as workflow input** rather than read from config inside
  the workflow. This keeps the workflow deterministic.
