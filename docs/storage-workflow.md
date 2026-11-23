# StorageItemWorkflow

The Temporal workflow that tracks one stored item from storage to its end
state. For the business rules behind it, see the lifecycle and timing sections
of [`../brain/overview.md`](../brain/overview.md).

## Input

```go
type StoreItemInput struct {
    ItemID          string
    MemberName      string
    ItemName        string
    Description     string
    StorageDuration time.Duration // fixed per run; set by the API
}
```

Derived inside the workflow from `workflow.Now()` when it starts:

- `ExpiresAt = StoredAt + StorageDuration`
- `ReminderAt = ExpiresAt - StorageDuration/7`

## States

| Status | Meaning | Terminal |
|--------|---------|----------|
| `Stored` | Within its time | no |
| `ExpiringSoon` | Reminder sent | no |
| `Expired` | Expiry notice sent | no |
| `Collected` | Collected by the member | yes |
| `RemovedByAdmin` | Removed by an admin | yes |

## Flow

Every step that touches the store is an activity (see [Activities](#activities)).

```
start
  ▼
CreateItem                (row with status Stored and the derived timings)
  ▼
wait for: timer(ReminderAt) │ collect │ remove
  │ timer
  ▼
SendNotification(ExpiringSoon); SetItemStatus(ExpiringSoon)
  ▼
wait for: timer(ExpiresAt) │ collect │ remove
  │ timer
  ▼
SendNotification(Expired); SetItemStatus(Expired)
  ▼
wait for: collect │ remove   (no further timers or notifications)

on collect: SetItemStatus(Collected); workflow completes
on remove:  SendNotification(RemovedByAdmin); SetItemStatus(RemovedByAdmin);
            workflow completes
```

Each timed wait is a `workflow.AwaitWithOptions` with a timeout (a durable
timer named `reminder` or `expiry` in the Temporal UI) on the state that the
Update handlers change. When a terminal Update is accepted, the wait returns
early and its timer is cancelled, so a late timer never fires.

### Concurrency rules

Updates can arrive at any moment, including while a notification activity is
running. The workflow keeps its state consistent with these rules:

- **Ending flag.** When a `collect` or `remove` Update is accepted, the
  workflow marks the item as *ending* before doing anything else. Both
  validators reject calls once the item is ending, so only one terminal
  Update can ever be accepted, even while the removal notification is still
  being sent.
- **No status rollback.** After a reminder or expiry notification completes,
  the workflow re-checks whether the item is ending before it sets
  `ExpiringSoon` or `Expired`. If a terminal Update arrived during the
  activity, the timer-driven transition is skipped.
- **Created before ended.** Update handlers wait until `CreateItem` has
  completed before they write anything, so a terminal status is never written
  for an item the store does not have yet.
- **Terminal guard in the store.** A timer-driven `SetItemStatus` can still be
  in flight when a terminal Update writes its status. The store never changes
  a terminal status (see `docs/architecture.md#store`), so the late write is
  a no-op.
- **Drain handlers before completing.** Before returning, the workflow waits
  for `workflow.AllHandlersFinished`, so a `remove` handler that is still
  sending its notification finishes and returns its result.

### Input validation

The workflow fails immediately if `StorageDuration` is not positive. The API
validates this as well; the check guards against bad direct starts.

## Updates

| Update | Allowed from | Rejected when |
|--------|--------------|---------------|
| `collect` | `Stored`, `ExpiringSoon`, `Expired` | Item is already in a terminal state |
| `remove` | `Stored`, `ExpiringSoon`, `Expired` | Item is already in a terminal state |

The validators reject invalid calls before anything is written to history.
A call is also rejected once another terminal Update has been accepted (see
[Concurrency rules](#concurrency-rules)). Rejections are `ApplicationError`s
of type `ItemEnded`. Once the workflow has completed, Temporal itself rejects
any further Update ("workflow execution already completed"), so callers must
treat that error the same way. Both Updates take no arguments and
return the resulting `ItemState`; for `remove`, that is after the removal
notification has been sent.

## Query: `getStatus`

Returns the workflow's own `ItemState`: the input fields, `StoredAt`,
`ReminderAt`, `ExpiresAt`, `Status`, `EndedAt` (once terminal), and the
notifications sent so far. It mirrors what the workflow has written to the
store and is there for inspection in the Temporal UI and CLI. The application
reads the store, not this Query.

## Activities

The activities are methods on an `Activities` struct that holds the
`store.Store`, so tests can pass in an in-memory store and inspect it.

| Activity | Store call | Notes |
|----------|------------|-------|
| `CreateItem` | `CreateItem` | First step. Writes the input, timings and `Stored` |
| `SetItemStatus` | `SetStatus` | Sets `EndedAt` for terminal statuses |
| `SendNotification` | `AddNotification` | Logs the notification, then records it for the UI |

```go
type Notification struct {
    ItemID     string
    MemberName string
    Kind       string // ExpiringSoon | Expired | RemovedByAdmin
    Message    string
    SentAt     time.Time
}
```

Responsibilities are split along the side-effect boundary:

- **Workflow:** decides what happens and when. It composes each notification,
  including the message text, which comes only from the input and the derived
  timings, so building it is deterministic. It also supplies every timestamp
  (`StoredAt`, `SentAt`, `EndedAt`) from `workflow.Now()`, so time-skipping
  tests can assert when things happen.
- **Activities:** perform the writes. "Delivering" a notification means
  recording it in the store, where the UI shows it; email or SMS could be
  added in the same activity later without changing the workflow.

Every store write is idempotent, so retries are safe. If an activity finds no
item (for example, the in-memory store was lost on a restart), it fails with a
non-retryable `ItemNotFound` error, and the workflow fails.

Retry policy: Temporal's default (unlimited retries with backoff), with a
`StartToCloseTimeout` of 10 seconds.

Activity options are applied on every call (the `execute` helper), not once
on the workflow's main context: Update handlers receive their own context,
which does not inherit options set on the main one.

## Testing

Use the Go SDK's `testsuite.WorkflowTestSuite`. Its test environment skips
time, so a full 7-day lifecycle runs in milliseconds. The activities run
against a real in-memory store, so tests assert on the store's contents as
well as the workflow result. Cover:

- `CreateItem` as the first step, with the derived timings

- the full path to `Expired` with both notifications, at the right times
- `collect` from each non-terminal state
- `remove` from each non-terminal state, including the removal notification
- Update validators rejecting calls in terminal states
- no notification after `Expired` other than an admin removal
- a terminal Update arriving while a reminder or expiry notification is
  running: the final status stays terminal
- two terminal Updates competing: the second is rejected while the first is
  still in progress
- `remove` returns the final `RemovedByAdmin` state with its notification
- a non-positive `StorageDuration` fails the workflow
- an activity finding no item fails the workflow without retrying
