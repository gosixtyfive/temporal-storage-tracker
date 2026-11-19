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

The workflow uses `workflow.NewTimer` for the timers and `workflow.Await` on
the state that the Update handlers change. A timer that fires after a
terminal Update has been accepted is ignored.

## Updates

| Update | Allowed from | Rejected when |
|--------|--------------|---------------|
| `collect` | `Stored`, `ExpiringSoon`, `Expired` | Item is already in a terminal state |
| `remove` | `Stored`, `ExpiringSoon`, `Expired` | Item is already in a terminal state |

The validators reject invalid calls before anything is written to history.
Both Updates return the resulting `ItemState`.

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

In the demo, "delivering" a notification means logging it. The workflow then
appends the notification to its own `Notifications` list once the activity
succeeds, which is how it reaches the UI. Keeping delivery in an activity marks
the side-effect boundary, so real email or SMS could be added later without
changing the workflow.

Retry policy: Temporal's default (unlimited retries with backoff), with a
short `StartToCloseTimeout`.

## Testing

Use the Go SDK's `testsuite.WorkflowTestSuite`. Its test environment skips
time, so a full 7-day lifecycle runs in milliseconds. Cover:

- the full path to `Expired` with both notifications, at the right times
- `collect` from each non-terminal state
- `remove` from each non-terminal state, including the removal notification
- Update validators rejecting calls in terminal states
- no notification after `Expired` other than an admin removal
