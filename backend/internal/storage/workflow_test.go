package storage

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"

	"github.com/gosixtyfive/temporal-storage-tracker/backend/internal/store"
)

const (
	week = 7 * 24 * time.Hour
	day  = 24 * time.Hour
)

var t0 = time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)

func testInput() StoreItemInput {
	return StoreItemInput{
		ItemID:          "item-1",
		MemberName:      "Sam",
		ItemName:        "Bike frame",
		Description:     "Steel frame, mid-repair",
		StorageDuration: week,
	}
}

// harness is a test environment whose activities write to a real in-memory
// store.
type harness struct {
	env   *testsuite.TestWorkflowEnvironment
	store *store.Memory
	acts  *Activities
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	var s testsuite.WorkflowTestSuite
	env := s.NewTestWorkflowEnvironment()
	env.SetStartTime(t0)
	mem := store.NewMemory()
	a := &Activities{Store: mem}
	env.RegisterActivity(a)
	return &harness{env: env, store: mem, acts: a}
}

// storedItem reads the test item from the store.
func (h *harness) storedItem(t *testing.T) store.Item {
	t.Helper()
	it, err := h.store.GetItem(context.Background(), "item-1")
	require.NoError(t, err)
	return it
}

// delaySendNotification makes SendNotification for the given kinds (all kinds
// if none) take d on the workflow clock, still writing to the real store.
func (h *harness) delaySendNotification(d time.Duration, kinds ...store.NotificationKind) {
	match := mock.MatchedBy(func(n Notification) bool {
		return len(kinds) == 0 || slices.Contains(kinds, n.Kind)
	})
	h.env.OnActivity(h.acts.SendNotification, mock.Anything, match).After(d).Return(h.acts.SendNotification)
	if len(kinds) > 0 {
		h.env.OnActivity(h.acts.SendNotification, mock.Anything, mock.Anything).Return(h.acts.SendNotification)
	}
}

// updateResult captures the outcome of an Update sent in the test env.
type updateResult struct {
	accepted bool
	rejected error
	state    ItemState
	err      error
	done     bool
}

func sendUpdate(env *testsuite.TestWorkflowEnvironment, name string) *updateResult {
	r := &updateResult{}
	env.UpdateWorkflow(name, "", &testsuite.TestUpdateCallback{
		OnAccept: func() { r.accepted = true },
		OnReject: func(err error) { r.rejected = err },
		OnComplete: func(v any, err error) {
			r.done = true
			r.err = err
			if s, ok := v.(ItemState); ok {
				r.state = s
			}
		},
	})
	return r
}

func queryState(t *testing.T, env *testsuite.TestWorkflowEnvironment) ItemState {
	t.Helper()
	v, err := env.QueryWorkflow(QueryGetStatus)
	require.NoError(t, err)
	var s ItemState
	require.NoError(t, v.Get(&s))
	return s
}

func finalState(t *testing.T, env *testsuite.TestWorkflowEnvironment) ItemState {
	t.Helper()
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	var s ItemState
	require.NoError(t, env.GetWorkflowResult(&s))
	return s
}

func kinds(ns []store.Notification) []store.NotificationKind {
	out := make([]store.NotificationKind, len(ns))
	for i, n := range ns {
		out[i] = n.Kind
	}
	return out
}

func TestCreateItemIsFirstStep(t *testing.T) {
	h := newHarness(t)
	var created store.Item
	h.env.RegisterDelayedCallback(func() { created = h.storedItem(t) }, time.Second)
	h.env.RegisterDelayedCallback(func() { sendUpdate(h.env, UpdateCollect) }, time.Hour)

	h.env.ExecuteWorkflow(StorageItemWorkflow, testInput())
	finalState(t, h.env)

	assert.Equal(t, store.Item{
		ID:              "item-1",
		MemberName:      "Sam",
		ItemName:        "Bike frame",
		Description:     "Steel frame, mid-repair",
		StorageDuration: week,
		StoredAt:        t0,
		ReminderAt:      t0.Add(6 * day),
		ExpiresAt:       t0.Add(week),
		Status:          store.StatusStored,
	}, created)
}

func TestFullLifecycleToExpired(t *testing.T) {
	h := newHarness(t)

	var beforeReminder, afterReminder, afterExpiry, muchLater store.Item
	var queried ItemState
	h.env.RegisterDelayedCallback(func() { beforeReminder = h.storedItem(t) }, 6*day-time.Minute)
	h.env.RegisterDelayedCallback(func() { afterReminder = h.storedItem(t) }, 6*day+time.Minute)
	h.env.RegisterDelayedCallback(func() {
		afterExpiry = h.storedItem(t)
		queried = queryState(t, h.env)
	}, week+time.Minute)
	h.env.RegisterDelayedCallback(func() { muchLater = h.storedItem(t) }, 60*day)
	h.env.RegisterDelayedCallback(func() { sendUpdate(h.env, UpdateCollect) }, 60*day+time.Minute)

	h.env.ExecuteWorkflow(StorageItemWorkflow, testInput())
	final := finalState(t, h.env)

	assert.Equal(t, store.StatusStored, beforeReminder.Status)
	assert.Empty(t, beforeReminder.Notifications)

	assert.Equal(t, store.StatusExpiringSoon, afterReminder.Status)
	require.Len(t, afterReminder.Notifications, 1)
	assert.Equal(t, store.KindExpiringSoon, afterReminder.Notifications[0].Kind)
	assert.Equal(t, t0.Add(6*day), afterReminder.Notifications[0].SentAt)
	assert.Contains(t, afterReminder.Notifications[0].Message, "1 day")

	assert.Equal(t, store.StatusExpired, afterExpiry.Status)
	assert.Equal(t, []store.NotificationKind{store.KindExpiringSoon, store.KindExpired}, kinds(afterExpiry.Notifications))
	assert.Equal(t, t0.Add(week), afterExpiry.Notifications[1].SentAt)
	assert.Nil(t, afterExpiry.EndedAt)

	// The Query mirrors the store.
	assert.Equal(t, afterExpiry.Status, queried.Status)
	assert.Equal(t, afterExpiry.Notifications, queried.Notifications)

	// Nothing further is sent after expiry.
	assert.Equal(t, store.StatusExpired, muchLater.Status)
	assert.Len(t, muchLater.Notifications, 2)

	stored := h.storedItem(t)
	assert.Equal(t, store.StatusCollected, stored.Status)
	require.NotNil(t, stored.EndedAt)
	assert.Equal(t, t0.Add(60*day+time.Minute), *stored.EndedAt)
	assert.Equal(t, store.StatusCollected, final.Status)
	assert.Equal(t, stored.Notifications, final.Notifications)
}

// nonTerminalStates gives a point in time inside each non-terminal state of a
// 7-day item, with the notifications sent by then.
var nonTerminalStates = []struct {
	name   string
	at     time.Duration
	before []store.NotificationKind
}{
	{"Stored", time.Hour, []store.NotificationKind{}},
	{"ExpiringSoon", 6*day + time.Hour, []store.NotificationKind{store.KindExpiringSoon}},
	{"Expired", week + time.Hour, []store.NotificationKind{store.KindExpiringSoon, store.KindExpired}},
}

func TestCollectFromEachState(t *testing.T) {
	for _, tc := range nonTerminalStates {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			var r *updateResult
			h.env.RegisterDelayedCallback(func() { r = sendUpdate(h.env, UpdateCollect) }, tc.at)

			h.env.ExecuteWorkflow(StorageItemWorkflow, testInput())
			final := finalState(t, h.env)

			require.True(t, r.accepted)
			require.True(t, r.done)
			require.NoError(t, r.err)
			assert.Equal(t, store.StatusCollected, r.state.Status)
			assert.Equal(t, store.StatusCollected, final.Status)

			stored := h.storedItem(t)
			assert.Equal(t, store.StatusCollected, stored.Status)
			require.NotNil(t, stored.EndedAt)
			assert.Equal(t, t0.Add(tc.at), *stored.EndedAt)
			assert.Equal(t, tc.before, kinds(stored.Notifications))
		})
	}
}

func TestRemoveFromEachState(t *testing.T) {
	for _, tc := range nonTerminalStates {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			var r *updateResult
			h.env.RegisterDelayedCallback(func() { r = sendUpdate(h.env, UpdateRemove) }, tc.at)

			h.env.ExecuteWorkflow(StorageItemWorkflow, testInput())
			final := finalState(t, h.env)

			want := append(append([]store.NotificationKind{}, tc.before...), store.KindRemovedByAdmin)

			// The Update result is the final state, including the removal
			// notification.
			require.True(t, r.accepted)
			require.True(t, r.done)
			require.NoError(t, r.err)
			assert.Equal(t, store.StatusRemovedByAdmin, r.state.Status)
			assert.Equal(t, want, kinds(r.state.Notifications))
			assert.Equal(t, store.StatusRemovedByAdmin, final.Status)

			stored := h.storedItem(t)
			assert.Equal(t, store.StatusRemovedByAdmin, stored.Status)
			require.NotNil(t, stored.EndedAt)
			assert.Equal(t, want, kinds(stored.Notifications))
			assert.Contains(t, stored.Notifications[len(want)-1].Message, "removed")
		})
	}
}

// While the workflow is still running, the validator rejects a second terminal
// Update. (Once the workflow has completed, the server rejects Updates itself
// with "workflow execution already completed"; the API handles that case.)
func TestSecondTerminalUpdateRejected(t *testing.T) {
	h := newHarness(t)
	h.delaySendNotification(30 * time.Second)

	var remove, collect *updateResult
	h.env.RegisterDelayedCallback(func() { remove = sendUpdate(h.env, UpdateRemove) }, time.Hour)
	h.env.RegisterDelayedCallback(func() { collect = sendUpdate(h.env, UpdateCollect) }, time.Hour+10*time.Second)

	h.env.ExecuteWorkflow(StorageItemWorkflow, testInput())
	finalState(t, h.env)

	assert.True(t, remove.accepted)
	assert.True(t, remove.done)
	assertRejected(t, collect, ErrTypeItemEnded)

	stored := h.storedItem(t)
	assert.Equal(t, store.StatusRemovedByAdmin, stored.Status)
	assert.Equal(t, []store.NotificationKind{store.KindRemovedByAdmin}, kinds(stored.Notifications))
	assert.Equal(t, t0.Add(time.Hour+30*time.Second), *stored.EndedAt)
}

func assertRejected(t *testing.T, r *updateResult, errType string) {
	t.Helper()
	require.NotNil(t, r)
	assert.False(t, r.accepted)
	var appErr *temporal.ApplicationError
	require.True(t, errors.As(r.rejected, &appErr), "want ApplicationError, got %v", r.rejected)
	assert.Equal(t, errType, appErr.Type())
}

func TestTerminalUpdateDuringTimerNotification(t *testing.T) {
	for _, tc := range []struct {
		name   string
		kind   store.NotificationKind
		at     time.Duration
		update string
		status store.Status
		want   []store.NotificationKind
	}{
		{"collect during reminder", store.KindExpiringSoon, 6 * day, UpdateCollect, store.StatusCollected,
			[]store.NotificationKind{store.KindExpiringSoon}},
		{"remove during expiry", store.KindExpired, week, UpdateRemove, store.StatusRemovedByAdmin,
			[]store.NotificationKind{store.KindExpiringSoon, store.KindExpired, store.KindRemovedByAdmin}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.delaySendNotification(time.Minute, tc.kind)

			var r *updateResult
			h.env.RegisterDelayedCallback(func() { r = sendUpdate(h.env, tc.update) }, tc.at+10*time.Second)

			h.env.ExecuteWorkflow(StorageItemWorkflow, testInput())
			final := finalState(t, h.env)

			require.True(t, r.accepted)
			// The timer-driven transition must not overwrite the terminal
			// status, and no later timer notification is sent.
			assert.Equal(t, tc.status, final.Status)
			stored := h.storedItem(t)
			assert.Equal(t, tc.status, stored.Status)
			assert.ElementsMatch(t, tc.want, kinds(stored.Notifications))
		})
	}
}

// A timer-driven SetItemStatus still in flight when collect writes Collected
// must not overwrite it, in the store or in the workflow state.
func TestLateTimerStatusWriteIgnored(t *testing.T) {
	h := newHarness(t)
	isExpiringSoon := mock.MatchedBy(func(in SetItemStatusInput) bool { return in.Status == store.StatusExpiringSoon })
	h.env.OnActivity(h.acts.SetItemStatus, mock.Anything, isExpiringSoon).After(time.Minute).Return(h.acts.SetItemStatus)
	h.env.OnActivity(h.acts.SetItemStatus, mock.Anything, mock.Anything).Return(h.acts.SetItemStatus)

	h.env.RegisterDelayedCallback(func() { sendUpdate(h.env, UpdateCollect) }, 6*day+10*time.Second)

	h.env.ExecuteWorkflow(StorageItemWorkflow, testInput())
	final := finalState(t, h.env)

	assert.Equal(t, store.StatusCollected, final.Status)
	assert.Equal(t, store.StatusCollected, h.storedItem(t).Status)
}

// Updates that arrive before CreateItem has finished wait for it, so the
// terminal status is written to an item that exists.
func TestUpdateBeforeItemCreated(t *testing.T) {
	h := newHarness(t)
	h.env.OnActivity(h.acts.CreateItem, mock.Anything, mock.Anything).After(time.Minute).Return(h.acts.CreateItem)

	var r *updateResult
	h.env.RegisterDelayedCallback(func() { r = sendUpdate(h.env, UpdateCollect) }, 10*time.Second)

	h.env.ExecuteWorkflow(StorageItemWorkflow, testInput())
	finalState(t, h.env)

	require.True(t, r.accepted)
	require.NoError(t, r.err)
	stored := h.storedItem(t)
	assert.Equal(t, store.StatusCollected, stored.Status)
	assert.Equal(t, t0.Add(time.Minute), *stored.EndedAt)
}

func TestCompressedTiming(t *testing.T) {
	h := newHarness(t)
	in := testInput()
	in.StorageDuration = 7 * time.Minute

	var afterExpiry store.Item
	h.env.RegisterDelayedCallback(func() { afterExpiry = h.storedItem(t) }, 7*time.Minute+time.Second)
	h.env.RegisterDelayedCallback(func() { sendUpdate(h.env, UpdateCollect) }, 10*time.Minute)

	h.env.ExecuteWorkflow(StorageItemWorkflow, in)
	finalState(t, h.env)

	assert.Equal(t, t0.Add(6*time.Minute), afterExpiry.ReminderAt)
	require.Len(t, afterExpiry.Notifications, 2)
	assert.Equal(t, t0.Add(6*time.Minute), afterExpiry.Notifications[0].SentAt)
	assert.Contains(t, afterExpiry.Notifications[0].Message, "1 minute")
	assert.Equal(t, t0.Add(7*time.Minute), afterExpiry.Notifications[1].SentAt)
}

// If the store has lost the item (for example, after a restart of the
// in-memory store), the activity fails without retrying and so does the
// workflow.
func TestItemMissingFromStoreFailsWorkflow(t *testing.T) {
	h := newHarness(t)
	// CreateItem "succeeds" without writing anything.
	h.env.OnActivity(h.acts.CreateItem, mock.Anything, mock.Anything).Return(nil)

	h.env.ExecuteWorkflow(StorageItemWorkflow, testInput())

	require.True(t, h.env.IsWorkflowCompleted())
	var appErr *temporal.ApplicationError
	require.True(t, errors.As(h.env.GetWorkflowError(), &appErr), "got %v", h.env.GetWorkflowError())
	assert.Equal(t, ErrTypeItemNotFound, appErr.Type())
	assert.True(t, appErr.NonRetryable())
}

func TestInvalidDurationFails(t *testing.T) {
	for _, d := range []time.Duration{0, -time.Hour} {
		h := newHarness(t)
		in := testInput()
		in.StorageDuration = d

		h.env.ExecuteWorkflow(StorageItemWorkflow, in)

		require.True(t, h.env.IsWorkflowCompleted())
		var appErr *temporal.ApplicationError
		require.True(t, errors.As(h.env.GetWorkflowError(), &appErr))
		assert.Equal(t, ErrTypeInvalidInput, appErr.Type())
		_, err := h.store.GetItem(context.Background(), "item-1")
		assert.ErrorIs(t, err, store.ErrNotFound, "nothing is written for invalid input")
	}
}

func TestHumanDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{
		day:                     "1 day",
		2 * day:                 "2 days",
		time.Hour:               "1 hour",
		time.Minute:             "1 minute",
		90 * time.Minute:        "90 minutes",
		time.Second:             "1 second",
		1500 * time.Millisecond: "2s",
	} {
		assert.Equal(t, want, humanDuration(d), d.String())
	}
}
