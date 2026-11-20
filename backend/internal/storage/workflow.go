package storage

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// notificationTimeout bounds each SendNotification attempt. Retries use
// Temporal's default policy (unlimited, with backoff).
const notificationTimeout = 10 * time.Second

// StorageItemWorkflow tracks one stored item from storage to Collected or
// RemovedByAdmin. It returns the final ItemState.
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
			Notifications:  []SentNotification{},
		},
	}
	if err := w.register(ctx); err != nil {
		return ItemState{}, err
	}
	if err := w.setStatus(ctx, StatusStored); err != nil {
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
	// ending is set to the terminal status as soon as a collect or remove
	// Update is accepted. From then on, validators reject further terminal
	// Updates and timer-driven transitions are skipped.
	ending Status
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

// run drives the timer-based part of the lifecycle and returns once a
// terminal Update has been accepted.
func (w *itemWorkflow) run(ctx workflow.Context) error {
	steps := []struct {
		at     time.Time
		timer  string
		kind   NotificationKind
		status Status
		msg    string
	}{
		{w.state.ReminderAt, "reminder", KindExpiringSoon, StatusExpiringSoon,
			fmt.Sprintf("Your item %q expires in %s. Please collect it before then.",
				w.state.ItemName, humanDuration(w.state.StorageDuration/7))},
		{w.state.ExpiresAt, "expiry", KindExpired, StatusExpired,
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
	w.ending = StatusCollected
	if err := w.end(ctx, StatusCollected); err != nil {
		return ItemState{}, err
	}
	return w.state, nil
}

func (w *itemWorkflow) remove(ctx workflow.Context) (ItemState, error) {
	w.ending = StatusRemovedByAdmin
	msg := fmt.Sprintf("Your item %q was removed from the storage area by an admin.", w.state.ItemName)
	if err := w.notify(ctx, KindRemovedByAdmin, msg); err != nil {
		return ItemState{}, err
	}
	if err := w.end(ctx, StatusRemovedByAdmin); err != nil {
		return ItemState{}, err
	}
	return w.state, nil
}

func (w *itemWorkflow) end(ctx workflow.Context, status Status) error {
	endedAt := workflow.Now(ctx)
	w.state.EndedAt = &endedAt
	return w.setStatus(ctx, status)
}

// notify composes and delivers a notification, then records it in the
// workflow state with the workflow-clock time it was sent.
func (w *itemWorkflow) notify(ctx workflow.Context, kind NotificationKind, msg string) error {
	actx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: notificationTimeout,
	})
	n := Notification{
		ItemID:     w.state.ItemID,
		MemberName: w.state.MemberName,
		Kind:       kind,
		Message:    msg,
	}
	if err := workflow.ExecuteActivity(actx, SendNotification, n).Get(ctx, nil); err != nil {
		return err
	}
	w.state.Notifications = append(w.state.Notifications, SentNotification{
		Kind:    kind,
		Message: msg,
		SentAt:  workflow.Now(ctx),
	})
	return nil
}

func (w *itemWorkflow) setStatus(ctx workflow.Context, s Status) error {
	w.state.Status = s
	return workflow.UpsertTypedSearchAttributes(ctx, ItemStatusKey.ValueSet(string(s)))
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
