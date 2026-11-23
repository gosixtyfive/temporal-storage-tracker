# Architecture

How temporal-storage-tracker is built and how its parts connect. For what the
application does, see [`../brain/overview.md`](../brain/overview.md).

## Stack

| Layer | Technology |
|-------|------------|
| Workflow engine | Temporal, run locally with `temporal server start-dev` |
| Backend | Go: Temporal Go SDK and the standard `net/http` server |
| Frontend | Nuxt (Vue 3) |
| Persistence | Item store behind a Go interface; in memory for now |

## Components

```
┌──────────────┐  HTTP/JSON  ┌──────────────────────────────┐  gRPC  ┌─────────────────┐
│ Nuxt frontend│ ──────────▶ │  Go server (one process)     │ ─────▶ │ Temporal server │
│ (forms,      │   /api/*    │                              │        │ (dev server)    │
│  status,     │ ◀────────── │  HTTP API ──reads──▶ Store   │ ◀───── │                 │
│  dashboard)  │             │     │                  ▲     │  task  └─────────────────┘
└──────────────┘             │     │ start/Update     │     │  queue
                             │     ▼                writes  │
                             │  Temporal client   Worker    │
                             │                (workflow +   │
                             │                 activities)  │
                             └──────────────────────────────┘
```

- **Nuxt frontend.** Pages for storing an item, viewing an item's status, and
  the admin dashboard. It talks only to the Go API. During development, Nuxt
  proxies `/api/*` to the Go API, so the browser needs no CORS setup.
- **Go server** (`cmd/server`). One process that hosts three parts sharing
  one store:
  - **HTTP API.** Reads items from the store. Every change goes through
    Temporal: it starts workflows and sends Updates.
  - **Worker.** Hosts `StorageItemWorkflow` and its activities on the
    `storage-tracker` task queue.
  - **Store** (`internal/store`). The application's record of items and
    notifications.
- **Temporal server.** The local dev server. It runs the process: timers,
  retries, and durable workflow state.

## Data model

**The store is the system of record for reads; the workflow is the only
writer.** Each stored item has one row in the store and one
`StorageItemWorkflow` execution. The workflow ID is the item ID
(`item-<uuid>`).

- The workflow writes to the store only through **activities**: its first
  step creates the item, later steps change its status and deliver
  notifications. Because the workflow is the single writer, the store never
  disagrees with the workflow's decisions.
- The API **reads** the store for the item view and the dashboard, and
  **changes** items only through Temporal (start a workflow, send an Update).
- Activities may be retried, so every store write is idempotent (see
  [Store](#store)).

| Need | Mechanism |
|------|-----------|
| Item details, state, notifications | Store read |
| Dashboard list and filter by state | Store read |
| Store an item | Start `StorageItemWorkflow`; its first activity creates the row |
| Member collects / admin removes | Workflow **Update** `collect` / `remove` |
| Reminder timing | Durable workflow **timers** |
| Status changes and notifications | **Activities** writing to the store |

Workflow internals are described in
[`storage-workflow.md`](storage-workflow.md).

## Store

A small Go interface in `internal/store`, so a persistent implementation can
replace the in-memory one without touching the workflow or the API.

```go
type Store interface {
    CreateItem(ctx context.Context, item Item) error
    SetStatus(ctx context.Context, id string, status Status, at time.Time) error
    AddNotification(ctx context.Context, id string, n Notification) error
    GetItem(ctx context.Context, id string) (Item, error) // ErrNotFound
    ListItems(ctx context.Context, f ListFilter) ([]Item, error)
}
```

- `Item` holds the input fields, `StoredAt`, `ReminderAt`, `ExpiresAt`,
  `Status`, `EndedAt` and its `Notifications` (kind, message, sent at).
- The domain types `Status` and `NotificationKind` live in `internal/store`.
  The workflow package imports the store (its activities write to it), so the
  store must not import the workflow package.
- **Idempotent writes.** `CreateItem` with an existing ID is a no-op.
  `AddNotification` is a no-op if the item already has a notification of
  that kind.
- **Terminal guard.** `SetStatus` never changes an item that is already
  `Collected` or `RemovedByAdmin`, so a late write cannot overwrite a
  terminal status. Setting a terminal status also sets `EndedAt`.
- **Timestamps come from the caller** (the workflow clock), not the store.
- `ListItems` returns items newest first, optionally filtered by status.
- The in-memory implementation is a map guarded by a mutex, and it returns
  copies so callers cannot change stored data.

The in-memory store is lost when the server process restarts. Workflows that
are still running on the Temporal server then refer to items the store no
longer has; their activities fail with a non-retryable `ItemNotFound` error
and the workflow fails. Restart the Temporal dev server too, or use a
persistent store.

## HTTP API

| Method | Path | Does | Used by |
|--------|------|------|---------|
| `POST` | `/api/items` | Start `StorageItemWorkflow`; returns `201` with the new ID | Store item form |
| `GET` | `/api/items` | List items from the store; optional `?status=` | Dashboard |
| `GET` | `/api/items/{id}` | Read the item from the store | Item status view |
| `POST` | `/api/items/{id}/collect` | Update `collect`, then return the item | Item status view (member) |
| `POST` | `/api/items/{id}/remove` | Update `remove`, then return the item | Dashboard (admin) |
| `GET` | `/api/config` | Current timing | Frontend |

- **Pending items.** `POST /api/items` returns as soon as the workflow has
  started, before its first activity has written the row. Until then,
  `GET /api/items/{id}` returns `404`. The status view, which the form
  navigates to, shows "pending" and keeps polling until the item appears.
- Updates return the item as read from the store after the Update completes,
  or an error, so the UI can react without waiting for the next poll.

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

The storage duration is configured on the server with `STORAGE_DURATION` (a Go
duration). The default is `168h` (7 days); a demo might use `7m`.

- The "expiring soon" reminder fires at **1/7 of the duration before expiry**
  (1 day out of 7; 1 minute out of 7 minutes).
- The API passes the duration into each workflow as **input** when it starts
  the workflow. Each item keeps the timing it was created with, and changing
  the setting never affects running workflows. That avoids non-determinism.

## Local development

Run each in its own terminal from the repo root (see the `Makefile`):

```bash
make temporal   # Temporal dev server
make server     # API + worker on :8080; STORAGE_DURATION defaults to 7m here
cd frontend && npm run dev   # frontend
```

`make test`, `make vet` and `make build` cover the backend.

Environment variables:

| Variable | Default |
|----------|---------|
| `TEMPORAL_ADDRESS` | `localhost:7233` |
| `TEMPORAL_NAMESPACE` | `default` |
| `STORAGE_DURATION` | `168h` (`make server` overrides it to `7m`) |
| `PORT` | `8080` |

The Temporal Web UI (http://localhost:8233) shows workflow histories next to
the app. That is a large part of the demo.

## Repository layout

```
Makefile                     Local dev targets
backend/                     Go module github.com/gosixtyfive/temporal-storage-tracker/backend
  cmd/server/                One process: HTTP API + Temporal worker + store
  internal/storage/          Workflow, activities, shared types, task queue name
  internal/store/            Store interface and in-memory implementation
  internal/temporalclient/   Temporal client setup from the environment
  internal/api/              HTTP handlers
frontend/                    Nuxt app (planned)
brain/                       What the project does
docs/                        How it is built
```

## Key decisions

- **Application store, written only by the workflow.** The workflow
  orchestrates; the store holds the data the UI reads. This is how a real
  system would be built, and it keeps the API to simple reads. (It replaced
  an earlier design where Temporal held all state and the dashboard ran one
  Query per item.)
- **In-memory store behind an interface.** It keeps the data layer trivial
  for the demo; a persistent store can be added later without changing the
  workflow or the API.
- **API and worker in one process.** The in-memory store can only be shared
  inside one process. The cost is that the demo cannot restart the worker on
  its own; with a persistent store they can be split again.
- **No custom search attributes.** The dashboard filters in the store, so
  Temporal visibility is not needed.
- **Updates rather than Signals** for collect and remove. Updates can reject
  invalid transitions and return the new state synchronously, which a form
  submission needs.
- **Duration passed as workflow input** rather than read from config inside
  the workflow. This keeps the workflow deterministic.
