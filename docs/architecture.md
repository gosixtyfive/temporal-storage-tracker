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

- **Nuxt frontend.** Two pages: Add item and the Dashboard. It talks only to
  the Go API. During development, Nuxt
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
| `POST` | `/api/items` | Start `StorageItemWorkflow`; returns `201` with the new ID | Add item |
| `GET` | `/api/items` | List items from the store; optional `?status=` | Dashboard |
| `GET` | `/api/items/{id}` | Read the item from the store | (not used by the UI) |
| `POST` | `/api/items/{id}/collect` | Update `collect`, then return the item | Dashboard |
| `POST` | `/api/items/{id}/remove` | Update `remove`, then return the item | Dashboard |
| `GET` | `/api/config` | Current timing | Dashboard header |

- **New items appear shortly after creation.** `POST /api/items` returns as
  soon as the workflow has started, before its first activity has written
  the row. Until then the item is missing from the store (`GET
  /api/items/{id}` returns `404`). The dashboard simply shows it on its next
  poll, so the UI has no "pending" state.
- Updates return the item as read from the store after the Update completes,
  or an error, so the UI can react without waiting for the next poll. The
  handler checks the store first: unknown items get `404` and ended items
  `409` without calling Temporal.

**JSON.** camelCase keys, RFC 3339 timestamps in UTC, durations in seconds.
`endedAt` is `null` until the item ends; `notifications` is always a list.
Lists are wrapped as `{"items": [...]}`; errors are `{"error": "message"}`.

**Errors.**

| Status | When |
|--------|------|
| `400` | Invalid JSON, missing or too-long fields, unknown `?status=` |
| `404` | ID not of the form `item-<uuid>`, or no such item in the store (yet) |
| `409` | Item already ended: found in the store, rejected by the workflow's validator (`ItemEnded`), or the workflow had already completed (`NotFound` from Temporal) |
| `503` | Temporal reports itself unavailable |
| `504` | A Temporal call did not finish within 10 seconds |

Every Temporal call has a 10-second timeout, because an Update blocks until
a worker runs it. If the Temporal server is down, the SDK keeps retrying the
connection until that timeout, so in practice an outage shows up as `504`
rather than `503`.

The handlers (`internal/api`) depend on a small `Workflows` interface rather
than the Temporal client, so tests use the in-memory store with a fake in
place of Temporal.

## Frontend

| Route | View |
|-------|------|
| `/` | Redirects to `/dashboard` |
| `/add` | Add item form |
| `/dashboard` | All items with state, time left, notifications, and Collect / Remove actions; the current storage timing in its header |

- **No roles.** A single switch in the header moves between Add item and
  Dashboard. Anyone can do anything (see `brain/overview.md`).
- **Demo layout.** Meant to be used in two windows side by side: one adding
  items, one watching the dashboard.
- **Add item.** Asks for the member name in plain text (there are no
  accounts), the item name, and an optional description. After a successful
  add it stays on the form, clears it except for the member name, and shows a
  toast, so several items can be entered in a row.
- **Dashboard.** A responsive grid of item cards rather than a table, so it
  still works in a half-width window next to Add item. Each card shows the
  item, a status badge, time left (a countdown with a progress bar; "expired
  N ago" after expiry; the end time once ended), its notifications, a link to
  the item's workflow in the Temporal Web UI, and Collect / Remove while the
  item is active. Ended cards are dimmed. Above the cards, one tile per
  status shows its count and filters the list (in the browser; the API's
  `?status=` is not needed). Actions take one click, with no confirmation;
  a `409` shows the server's message and refreshes.
- **Live updates.** The dashboard polls `GET /api/items` every 2 seconds
  (no overlapping requests, paused while the tab is hidden, refreshed when
  it becomes visible). A shared 1-second clock drives the countdowns, using
  the browser's clock against the API's timestamps. When a poll brings a new
  notification, the dashboard shows a toast and briefly highlights that card.
  If the server cannot be reached, it keeps the last data and shows a
  "retrying" banner.
- **Client-side only.** Nuxt runs with `ssr: false`. Every view is live data
  from the API, so server rendering would add nothing; nothing needs SEO.
- **UI library.** Nuxt UI (built on Tailwind) for forms, badges, tables and
  toasts.
- **API access.** A Nuxt route rule proxies `/api/**` to the Go server
  (`http://localhost:8080` by default, overridable by environment variable).
  A `useApi()` composable holds typed versions of the API's JSON and turns
  `{"error": ...}` responses into errors that carry the status code. When the
  Go server is down, the proxy answers `502` without an API error body; the
  client reports that, like a failed request, as "could not reach the server".
- **Add item focus.** Nuxt UI's form disables its inputs while the submit
  handler runs, so the page moves focus back to Item name only after the
  form has finished loading.

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
make temporal           # Temporal dev server (Web UI on :8233)
make server             # API + worker + store on :8080; STORAGE_DURATION defaults to 7m here
make frontend-install   # once: frontend dependencies from the lockfile
make frontend           # Nuxt dev server on http://localhost:3000
```

`make test`, `make vet` and `make build` cover the backend;
`make frontend-check` typechecks and unit-tests the frontend.

Environment variables:

| Variable | Used by | Default |
|----------|---------|---------|
| `TEMPORAL_ADDRESS` | server | `localhost:7233` |
| `TEMPORAL_NAMESPACE` | server | `default` |
| `STORAGE_DURATION` | server | `168h` (`make server` overrides it to `7m`) |
| `PORT` | server | `8080` |
| `NUXT_API_TARGET` | frontend (read at startup) | `http://localhost:8080` |
| `NUXT_PUBLIC_TEMPORAL_UI_URL` | frontend | `http://localhost:8233` |
| `NUXT_PUBLIC_TEMPORAL_NAMESPACE` | frontend | `default` |

**Frontend dependencies and npm versions.** `npm ci` installs from the
lockfile and works with npm 10. Adding or upgrading a dependency changes the
lockfile, and npm 10.9 crashes while resolving this dependency tree
(`Cannot read properties of null (reading 'edgesOut')`), so use npm 11 for
that: `npx -y npm@11 install <pkg>`. Some packages also ask for Node 22.21 or
later; 22.18 works but prints a warning.

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
frontend/                    Nuxt app
  app/pages/                 Routes: / (redirect), /add, /dashboard
  app/components/            Header with the view switch, dashboard parts
  app/composables/           useApi(), usePolledItems(), useNow()
  app/utils/                 API types and client, dashboard logic, form checks, formatting
  test/                      Vitest unit tests for app/utils
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
- **Two views, no roles.** Add item and Dashboard, with collect and remove
  on the dashboard. A demo runs them in two windows side by side, which shows
  the workflows better than one page per item. (Replaced a Member/Admin role
  switch with a separate item status view.)
- **No custom search attributes.** The dashboard filters in the store, so
  Temporal visibility is not needed.
- **Updates rather than Signals** for collect and remove. Updates can reject
  invalid transitions and return the new state synchronously, which a form
  submission needs.
- **Duration passed as workflow input** rather than read from config inside
  the workflow. This keeps the workflow deterministic.
