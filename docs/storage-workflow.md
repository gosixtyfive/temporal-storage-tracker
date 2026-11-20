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

`ItemStatus` values. The search attribute is updated on every transition.

| Status | Meaning | Terminal |
|--------|---------|----------|
| `Stored` | Within its time | no |
| `ExpiringSoon` | Reminder sent | no |
| `Expired` | Expiry notice sent | no |
| `Collected` | Collected by the member | yes |
| `RemovedByAdmin` | Removed by an admin | yes |

## Flow

```
start
  │ status = Stored
  ▼
wait for: timer(ReminderAt) │ collect │ remove
  │ timer
  ▼
SendNotification(ExpiringSoon); status = ExpiringSoon
  ▼
wait for: timer(ExpiresAt) │ collect │ remove
  │ timer
  ▼
SendNotification(Expired); status = Expired
  ▼
wait for: collect │ remove   (no further timers or notifications)

on collect: status = Collected; workflow completes
on remove:  SendNotification(RemovedByAdmin); status = RemovedByAdmin;
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

Returns `ItemState`: the input fields, `StoredAt`, `ReminderAt`, `ExpiresAt`,
`Status`, `EndedAt` (once terminal), and `Notifications`. That last field is
the list of notifications sent so far, each with a kind, message and
timestamp. The status view and dashboard render this list.

## Activity: `SendNotification`

```go
type Notification struct {
    ItemID     string
    MemberName string
    Kind       string // ExpiringSoon | Expired | RemovedByAdmin
    Message    string
}
```

Responsibilities are split along the side-effect boundary:

- **Workflow:** composes the `Notification`, including the message text,
  which comes only from the input and the derived timings, so building it is
  deterministic. After the activity succeeds, the workflow appends the
  notification to its `Notifications` list, timestamped with `workflow.Now()`.
  The list is workflow state, which only the workflow can change; that is how
  notifications reach the UI.
- **Activity:** delivers the notification. In the demo that means logging it.
  Because delivery is in an activity, real email or SMS could be added later
  without changing the workflow.

The timestamp comes from the workflow clock rather than the activity's wall
clock, so time-skipping tests can assert when notifications happen.

Retry policy: Temporal's default (unlimited retries with backoff), with a
`StartToCloseTimeout` of 10 seconds.

## Testing

Use the Go SDK's `testsuite.WorkflowTestSuite`. Its test environment skips
time, so a full 7-day lifecycle runs in milliseconds. Cover:

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
