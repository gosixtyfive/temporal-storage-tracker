package storage

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/gosixtyfive/temporal-storage-tracker/backend/internal/store"
)

// activityTimeout bounds each activity attempt. Retries use Temporal's
// default policy (unlimited, with backoff).
const activityTimeout = 10 * time.Second

// acts is used only to name activities in ExecuteActivity calls; the worker
// registers a real *Activities.
var acts *Activities

// execute runs an activity and waits for it. It sets the activity options on
// every call because Update handlers receive their own context, which does
// not inherit options set on the workflow's main context.
func execute(ctx workflow.Context, activity any, arg any) error {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: activityTimeout,
	})
	return workflow.ExecuteActivity(ctx, activity, arg).Get(ctx, nil)
}

// StorageItemWorkflow tracks one stored item from storage to Collected or
// RemovedByAdmin, writing every change to the store through activities. It
// returns the final ItemState.
func StorageItemWorkflow(ctx workflow.Context, in StoreItemInput) (ItemState, error) {
	if in.StorageDuration <= 0 {
		return ItemState{}, temporal.NewNonRetryableApplicationError(
			fmt.Sprintf("StorageDuration must be positive, got %s", in.StorageDuration),
			ErrTypeInvalidInput, nil)
	}

	now := workflow.Now(ctx)
	expiresAt := now.Add(in.StorageDuration)
	w := &itemWorkflow{
		state: ItemState{
			StoreItemInput: in,
			StoredAt:       now,
			ReminderAt:     expiresAt.Add(-in.StorageDuration / 7),
			ExpiresAt:      expiresAt,
			Status:         store.StatusStored,
			Notifications:  []store.Notification{},
		},
	}
	if err := w.register(ctx); err != nil {
		return ItemState{}, err
	}

	if err := w.create(ctx); err != nil {
		return ItemState{}, err
	}
	if err := w.run(ctx); err != nil {
		return ItemState{}, err
	}

	// Let a remove handler that is still sending its notification finish and
	// return its result before the workflow completes.
	if err := workflow.Await(ctx, func() bool { return workflow.AllHandlersFinished(ctx) }); err != nil {
		return ItemState{}, err
	}
	return w.state, nil
}

type itemWorkflow struct {
	state ItemState
	// created is set once CreateItem has written the item to the store.
	// Update handlers wait for it before writing anything.
	created bool
	// ending is set to the terminal status as soon as a collect or remove
	// Update is accepted. From then on, validators reject further terminal
	// Updates and timer-driven transitions are skipped.
	ending store.Status
}

func (w *itemWorkflow) register(ctx workflow.Context) error {
	if err := workflow.SetQueryHandler(ctx, QueryGetStatus, func() (ItemState, error) {
		return w.state, nil
	}); err != nil {
		return err
	}
	if err := workflow.SetUpdateHandlerWithOptions(ctx, UpdateCollect, w.collect,
		workflow.UpdateHandlerOptions{Validator: w.validateTerminal}); err != nil {
		return err
	}
	return workflow.SetUpdateHandlerWithOptions(ctx, UpdateRemove, w.remove,
		workflow.UpdateHandlerOptions{Validator: w.validateTerminal})
}

// create writes the item to the store: the workflow's first step.
func (w *itemWorkflow) create(ctx workflow.Context) error {
	s := w.state
	item := store.Item{
		ID:              s.ItemID,
		MemberName:      s.MemberName,
		ItemName:        s.ItemName,
		Description:     s.Description,
		StorageDuration: s.StorageDuration,
		StoredAt:        s.StoredAt,
		ReminderAt:      s.ReminderAt,
		ExpiresAt:       s.ExpiresAt,
		Status:          store.StatusStored,
	}
	if err := execute(ctx, acts.CreateItem, item); err != nil {
		return err
	}
	w.created = true
	return nil
}

// run drives the timer-based part of the lifecycle and returns once a
// terminal Update has been accepted.
func (w *itemWorkflow) run(ctx workflow.Context) error {
	steps := []struct {
		at     time.Time
		timer  string
		kind   store.NotificationKind
		status store.Status
		msg    string
	}{
		{w.state.ReminderAt, "reminder", store.KindExpiringSoon, store.StatusExpiringSoon,
			fmt.Sprintf("Your item %q expires in %s. Please collect it before then.",
				w.state.ItemName, humanDuration(w.state.StorageDuration/7))},
		{w.state.ExpiresAt, "expiry", store.KindExpired, store.StatusExpired,
			fmt.Sprintf("Your item %q has expired. Please collect it as soon as possible.",
				w.state.ItemName)},
	}

	for _, s := range steps {
		ended, err := w.awaitEndingUntil(ctx, s.at, s.timer)
		if err != nil || ended {
			return err
		}
		if err := w.notify(ctx, s.kind, s.msg); err != nil {
			return err
		}
		// A terminal Update may have arrived while the notification was
		// being sent; never roll its status back.
		if w.ending != "" {
			return nil
		}
		if err := w.setStatus(ctx, s.status); err != nil {
			return err
		}
	}

	// Expired: no further timers or notifications. Wait for collect or remove.
	return workflow.Await(ctx, func() bool { return w.ending != "" })
}

// awaitEndingUntil waits until t or until a terminal Update is accepted,
// whichever comes first. It reports whether the item is ending.
func (w *itemWorkflow) awaitEndingUntil(ctx workflow.Context, t time.Time, timer string) (bool, error) {
	d := t.Sub(workflow.Now(ctx))
	if d <= 0 {
		return w.ending != "", nil
	}
	return workflow.AwaitWithOptions(ctx, workflow.AwaitOptions{
		Timeout:      d,
		TimerOptions: workflow.TimerOptions{Summary: timer},
	}, func() bool { return w.ending != "" })
}

func (w *itemWorkflow) validateTerminal(ctx workflow.Context) error {
	switch {
	case w.state.Status.Terminal():
		return temporal.NewApplicationError(
			fmt.Sprintf("item is already %s", w.state.Status), ErrTypeItemEnded)
	case w.ending != "":
		return temporal.NewApplicationError(
			fmt.Sprintf("item is already being marked %s", w.ending), ErrTypeItemEnded)
	}
	return nil
}

func (w *itemWorkflow) collect(ctx workflow.Context) (ItemState, error) {
	w.ending = store.StatusCollected
	if err := w.awaitCreated(ctx); err != nil {
		return ItemState{}, err
	}
	if err := w.setStatus(ctx, store.StatusCollected); err != nil {
		return ItemState{}, err
	}
	return w.state, nil
}

func (w *itemWorkflow) remove(ctx workflow.Context) (ItemState, error) {
	w.ending = store.StatusRemovedByAdmin
	if err := w.awaitCreated(ctx); err != nil {
		return ItemState{}, err
	}
	msg := fmt.Sprintf("Your item %q was removed from the storage area by an admin.", w.state.ItemName)
	if err := w.notify(ctx, store.KindRemovedByAdmin, msg); err != nil {
		return ItemState{}, err
	}
	if err := w.setStatus(ctx, store.StatusRemovedByAdmin); err != nil {
		return ItemState{}, err
	}
	return w.state, nil
}

func (w *itemWorkflow) awaitCreated(ctx workflow.Context) error {
	return workflow.Await(ctx, func() bool { return w.created })
}

// notify composes a notification, has the SendNotification activity deliver
// it, and mirrors it in the workflow state.
func (w *itemWorkflow) notify(ctx workflow.Context, kind store.NotificationKind, msg string) error {
	n := Notification{
		ItemID:     w.state.ItemID,
		MemberName: w.state.MemberName,
		Kind:       kind,
		Message:    msg,
		SentAt:     workflow.Now(ctx),
	}
	if err := execute(ctx, acts.SendNotification, n); err != nil {
		return err
	}
	w.state.Notifications = append(w.state.Notifications, store.Notification{
		Kind:    n.Kind,
		Message: n.Message,
		SentAt:  n.SentAt,
	})
	return nil
}

// setStatus writes a status change to the store and mirrors it in the
// workflow state, with the same terminal guard as the store: once the item
// has ended, a late timer-driven change is ignored.
func (w *itemWorkflow) setStatus(ctx workflow.Context, s store.Status) error {
	at := workflow.Now(ctx)
	in := SetItemStatusInput{ItemID: w.state.ItemID, Status: s, At: at}
	if err := execute(ctx, acts.SetItemStatus, in); err != nil {
		return err
	}
	if w.state.Status.Terminal() {
		return nil
	}
	w.state.Status = s
	if s.Terminal() {
		w.state.EndedAt = &at
	}
	return nil
}

// humanDuration renders the reminder lead time in the largest whole unit,
// for example "1 day" (7-day storage) or "1 minute" (7-minute demo).
func humanDuration(d time.Duration) string {
	units := []struct {
		size time.Duration
		name string
	}{
		{24 * time.Hour, "day"},
		{time.Hour, "hour"},
		{time.Minute, "minute"},
		{time.Second, "second"},
	}
	for _, u := range units {
		if d >= u.size && d%u.size == 0 {
			n := int64(d / u.size)
			if n == 1 {
				return "1 " + u.name
			}
			return fmt.Sprintf("%d %ss", n, u.name)
		}
	}
	return d.Round(time.Second).String()
}
