package storage

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"
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

func newEnv(t *testing.T) *testsuite.TestWorkflowEnvironment {
	t.Helper()
	var s testsuite.WorkflowTestSuite
	env := s.NewTestWorkflowEnvironment()
	env.SetStartTime(t0)
	env.RegisterActivity(SendNotification)
	return env
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

func kinds(ns []SentNotification) []NotificationKind {
	out := make([]NotificationKind, len(ns))
	for i, n := range ns {
		out[i] = n.Kind
	}
	return out
}

func TestFullLifecycleToExpired(t *testing.T) {
	env := newEnv(t)

	var initial, beforeReminder, afterReminder, afterExpiry, muchLater ItemState
	env.RegisterDelayedCallback(func() { initial = queryState(t, env) }, time.Minute)
	env.RegisterDelayedCallback(func() { beforeReminder = queryState(t, env) }, 6*day-time.Minute)
	env.RegisterDelayedCallback(func() { afterReminder = queryState(t, env) }, 6*day+time.Minute)
	env.RegisterDelayedCallback(func() { afterExpiry = queryState(t, env) }, week+time.Minute)
	env.RegisterDelayedCallback(func() { muchLater = queryState(t, env) }, 60*day)
	env.RegisterDelayedCallback(func() { sendUpdate(env, UpdateCollect) }, 60*day+time.Minute)

	env.ExecuteWorkflow(StorageItemWorkflow, testInput())
	final := finalState(t, env)

	assert.Equal(t, StatusStored, initial.Status)
	assert.Equal(t, t0, initial.StoredAt)
	assert.Equal(t, t0.Add(6*day), initial.ReminderAt)
	assert.Equal(t, t0.Add(week), initial.ExpiresAt)
	assert.Empty(t, initial.Notifications)

	assert.Equal(t, StatusStored, beforeReminder.Status)

	assert.Equal(t, StatusExpiringSoon, afterReminder.Status)
	require.Len(t, afterReminder.Notifications, 1)
	assert.Equal(t, KindExpiringSoon, afterReminder.Notifications[0].Kind)
	assert.Equal(t, t0.Add(6*day), afterReminder.Notifications[0].SentAt)
	assert.Contains(t, afterReminder.Notifications[0].Message, "1 day")

	assert.Equal(t, StatusExpired, afterExpiry.Status)
	assert.Equal(t, []NotificationKind{KindExpiringSoon, KindExpired}, kinds(afterExpiry.Notifications))
	assert.Equal(t, t0.Add(week), afterExpiry.Notifications[1].SentAt)

	// Nothing further is sent after expiry.
	assert.Equal(t, StatusExpired, muchLater.Status)
	assert.Len(t, muchLater.Notifications, 2)

	assert.Equal(t, StatusCollected, final.Status)
	assert.Len(t, final.Notifications, 2)
}

// nonTerminalStates gives a point in time inside each non-terminal state of a
// 7-day item, with the notifications sent by then.
var nonTerminalStates = []struct {
	name   string
	at     time.Duration
	before []NotificationKind
}{
	{"Stored", time.Hour, []NotificationKind{}},
	{"ExpiringSoon", 6*day + time.Hour, []NotificationKind{KindExpiringSoon}},
	{"Expired", week + time.Hour, []NotificationKind{KindExpiringSoon, KindExpired}},
}

func TestCollectFromEachState(t *testing.T) {
	for _, tc := range nonTerminalStates {
		t.Run(tc.name, func(t *testing.T) {
			env := newEnv(t)
			var r *updateResult
			env.RegisterDelayedCallback(func() { r = sendUpdate(env, UpdateCollect) }, tc.at)

			env.ExecuteWorkflow(StorageItemWorkflow, testInput())
			final := finalState(t, env)

			require.True(t, r.accepted)
			require.True(t, r.done)
			require.NoError(t, r.err)
			assert.Equal(t, StatusCollected, r.state.Status)

			assert.Equal(t, StatusCollected, final.Status)
			require.NotNil(t, final.EndedAt)
			assert.Equal(t, t0.Add(tc.at), *final.EndedAt)
			assert.Equal(t, tc.before, kinds(final.Notifications))
		})
	}
}

func TestRemoveFromEachState(t *testing.T) {
	for _, tc := range nonTerminalStates {
		t.Run(tc.name, func(t *testing.T) {
			env := newEnv(t)
			var r *updateResult
			env.RegisterDelayedCallback(func() { r = sendUpdate(env, UpdateRemove) }, tc.at)

			env.ExecuteWorkflow(StorageItemWorkflow, testInput())
			final := finalState(t, env)

			want := append(append([]NotificationKind{}, tc.before...), KindRemovedByAdmin)

			// The Update result is the final state, including the removal
			// notification.
			require.True(t, r.accepted)
			require.True(t, r.done)
			require.NoError(t, r.err)
			assert.Equal(t, StatusRemovedByAdmin, r.state.Status)
			assert.Equal(t, want, kinds(r.state.Notifications))

			assert.Equal(t, StatusRemovedByAdmin, final.Status)
			require.NotNil(t, final.EndedAt)
			assert.Equal(t, want, kinds(final.Notifications))
			assert.Contains(t, final.Notifications[len(want)-1].Message, "removed")
		})
	}
}

// While the workflow is still running, the validator rejects a second terminal
// Update. (Once the workflow has completed, the server rejects Updates itself
// with "workflow execution already completed"; the API handles that case.)
func TestSecondTerminalUpdateRejected(t *testing.T) {
	t.Run("collect while remove notification in flight", func(t *testing.T) {
		env := newEnv(t)
		env.OnActivity(SendNotification, mock.Anything, mock.Anything).After(30 * time.Second).Return(nil)

		var remove, collect *updateResult
		env.RegisterDelayedCallback(func() { remove = sendUpdate(env, UpdateRemove) }, time.Hour)
		env.RegisterDelayedCallback(func() { collect = sendUpdate(env, UpdateCollect) }, time.Hour+10*time.Second)

		env.ExecuteWorkflow(StorageItemWorkflow, testInput())
		final := finalState(t, env)

		assert.True(t, remove.accepted)
		assert.True(t, remove.done)
		assertItemEnded(t, collect)
		assert.Equal(t, StatusRemovedByAdmin, final.Status)
		assert.Equal(t, []NotificationKind{KindRemovedByAdmin}, kinds(final.Notifications))
		assert.Equal(t, t0.Add(time.Hour+30*time.Second), *final.EndedAt)
	})
}

func assertItemEnded(t *testing.T, r *updateResult) {
	t.Helper()
	require.NotNil(t, r)
	assert.False(t, r.accepted)
	var appErr *temporal.ApplicationError
	require.True(t, errors.As(r.rejected, &appErr), "want ApplicationError, got %v", r.rejected)
	assert.Equal(t, ErrTypeItemEnded, appErr.Type())
}

func TestTerminalUpdateDuringTimerNotification(t *testing.T) {
	for _, tc := range []struct {
		name   string
		kind   NotificationKind
		at     time.Duration
		update string
		status Status
		want   []NotificationKind
	}{
		{"collect during reminder", KindExpiringSoon, 6 * day, UpdateCollect, StatusCollected,
			[]NotificationKind{KindExpiringSoon}},
		{"remove during expiry", KindExpired, week, UpdateRemove, StatusRemovedByAdmin,
			[]NotificationKind{KindExpiringSoon, KindExpired, KindRemovedByAdmin}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newEnv(t)
			isKind := mock.MatchedBy(func(n Notification) bool { return n.Kind == tc.kind })
			env.OnActivity(SendNotification, mock.Anything, isKind).After(time.Minute).Return(nil)
			env.OnActivity(SendNotification, mock.Anything, mock.Anything).Return(nil)

			var r *updateResult
			env.RegisterDelayedCallback(func() { r = sendUpdate(env, tc.update) }, tc.at+10*time.Second)

			env.ExecuteWorkflow(StorageItemWorkflow, testInput())
			final := finalState(t, env)

			require.True(t, r.accepted)
			// The timer-driven transition must not overwrite the terminal
			// status, and no later timer notification is sent.
			assert.Equal(t, tc.status, final.Status)
			assert.ElementsMatch(t, tc.want, kinds(final.Notifications))
		})
	}
}

func TestCompressedTiming(t *testing.T) {
	env := newEnv(t)
	in := testInput()
	in.StorageDuration = 7 * time.Minute

	var afterExpiry ItemState
	env.RegisterDelayedCallback(func() { afterExpiry = queryState(t, env) }, 7*time.Minute+time.Second)
	env.RegisterDelayedCallback(func() { sendUpdate(env, UpdateCollect) }, 10*time.Minute)

	env.ExecuteWorkflow(StorageItemWorkflow, in)
	finalState(t, env)

	require.Len(t, afterExpiry.Notifications, 2)
	assert.Equal(t, t0.Add(6*time.Minute), afterExpiry.Notifications[0].SentAt)
	assert.Contains(t, afterExpiry.Notifications[0].Message, "1 minute")
	assert.Equal(t, t0.Add(7*time.Minute), afterExpiry.Notifications[1].SentAt)
}

func TestInvalidDurationFails(t *testing.T) {
	for _, d := range []time.Duration{0, -time.Hour} {
		env := newEnv(t)
		in := testInput()
		in.StorageDuration = d

		env.ExecuteWorkflow(StorageItemWorkflow, in)

		require.True(t, env.IsWorkflowCompleted())
		var appErr *temporal.ApplicationError
		require.True(t, errors.As(env.GetWorkflowError(), &appErr))
		assert.Equal(t, ErrTypeInvalidInput, appErr.Type())
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
