# temporal-storage-tracker

A demo of [Temporal](https://temporal.io) workflows driving a long-running
business process. Members of a makerspace leave items in a shared storage
area for a limited time; each stored item is a workflow that reminds the
member before time runs out, notifies them when it has expired, and ends when
the item is collected or removed by an admin.

- **Backend:** Go: one process with the HTTP API, the Temporal worker and an
  in-memory item store
- **Frontend:** Nuxt with Nuxt UI: an Add item page and a live Dashboard
- **Workflow engine:** the Temporal dev server

## Prerequisites

- [Go](https://go.dev/dl/) (see `backend/go.mod` for the version)
- [Temporal CLI](https://docs.temporal.io/cli) (`temporal server start-dev`)
- Node.js 22.21 or later and npm (22.18 works but prints warnings)

## Quickstart

Run each in its own terminal, from the repo root:

```bash
make temporal           # Temporal dev server; Web UI on http://localhost:8233
make server             # Go API + worker + store on :8080 (7-minute items)
make frontend-install   # first time only
make frontend           # app on http://localhost:3000
```

For quicker items: `make server STORAGE_DURATION=70s` (the reminder goes out
at 1/7 of the duration before expiry).

## Running a demo

Open http://localhost:3000 in two browser windows side by side: one on
**Add item** to enter items, one on the **Dashboard** to watch each item go
from Stored to Expiring soon to Expired, and to collect or remove items. Each
dashboard card links to its workflow in the Temporal Web UI.

The item store is in memory: restarting `make server` clears it, and
workflows still running on the Temporal server then fail. Restart
`make temporal` too for a clean slate.

## Checks

```bash
make test             # backend tests
make vet              # go vet
make frontend-check   # frontend typecheck and unit tests
```

## Documentation

- [`brain/`](brain/README.md): what the app does and why: scenario,
  features, item lifecycle and storage timing
- [`docs/`](docs/README.md): how it is built: architecture, HTTP API,
  workflow, local development (including environment variables and npm
  notes)
