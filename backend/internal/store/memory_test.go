package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var t0 = time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)

func newItem(id string, storedAt time.Time) Item {
	return Item{
		ID:              id,
		MemberName:      "Sam",
		ItemName:        "Bike frame",
		Description:     "Steel frame",
		StorageDuration: 7 * 24 * time.Hour,
		StoredAt:        storedAt,
		ReminderAt:      storedAt.Add(6 * 24 * time.Hour),
		ExpiresAt:       storedAt.Add(7 * 24 * time.Hour),
		Status:          StatusStored,
	}
}

func mustGet(t *testing.T, s Store, id string) Item {
	t.Helper()
	it, err := s.GetItem(context.Background(), id)
	require.NoError(t, err)
	return it
}

func TestCreateAndGet(t *testing.T) {
	ctx := context.Background()
	s := NewMemory()
	want := newItem("item-1", t0)
	require.NoError(t, s.CreateItem(ctx, want))

	want.Notifications = nil // compare without the nil/empty distinction
	got := mustGet(t, s, "item-1")
	got.Notifications = nil
	assert.Equal(t, want, got)
}

func TestCreateDefaultsAndValidation(t *testing.T) {
	ctx := context.Background()
	s := NewMemory()

	it := newItem("item-1", t0)
	it.Status = ""
	require.NoError(t, s.CreateItem(ctx, it))
	assert.Equal(t, StatusStored, mustGet(t, s, "item-1").Status)

	assert.Error(t, s.CreateItem(ctx, newItem("", t0)), "empty ID")

	bad := newItem("item-2", t0)
	bad.Status = "Lost"
	assert.Error(t, s.CreateItem(ctx, bad), "unknown status")
}

func TestCreateIsIdempotent(t *testing.T) {
	ctx := context.Background()
	s := NewMemory()
	require.NoError(t, s.CreateItem(ctx, newItem("item-1", t0)))
	require.NoError(t, s.SetStatus(ctx, "item-1", StatusExpiringSoon, t0.Add(time.Hour)))

	// A retried create must not reset the item.
	require.NoError(t, s.CreateItem(ctx, newItem("item-1", t0.Add(time.Minute))))
	got := mustGet(t, s, "item-1")
	assert.Equal(t, StatusExpiringSoon, got.Status)
	assert.Equal(t, t0, got.StoredAt)
}

func TestSetStatus(t *testing.T) {
	ctx := context.Background()
	s := NewMemory()
	require.NoError(t, s.CreateItem(ctx, newItem("item-1", t0)))

	require.NoError(t, s.SetStatus(ctx, "item-1", StatusExpiringSoon, t0.Add(time.Hour)))
	got := mustGet(t, s, "item-1")
	assert.Equal(t, StatusExpiringSoon, got.Status)
	assert.Nil(t, got.EndedAt, "non-terminal status leaves EndedAt unset")

	assert.ErrorIs(t, s.SetStatus(ctx, "missing", StatusExpired, t0), ErrNotFound)
	assert.Error(t, s.SetStatus(ctx, "item-1", "Lost", t0))
}

func TestTerminalGuard(t *testing.T) {
	for _, terminal := range []Status{StatusCollected, StatusRemovedByAdmin} {
		t.Run(string(terminal), func(t *testing.T) {
			ctx := context.Background()
			s := NewMemory()
			require.NoError(t, s.CreateItem(ctx, newItem("item-1", t0)))

			endedAt := t0.Add(2 * time.Hour)
			require.NoError(t, s.SetStatus(ctx, "item-1", terminal, endedAt))

			// Late writes, terminal or not, change nothing.
			for _, late := range []Status{StatusExpiringSoon, StatusExpired, StatusCollected, StatusRemovedByAdmin} {
				require.NoError(t, s.SetStatus(ctx, "item-1", late, t0.Add(3*time.Hour)))
			}
			got := mustGet(t, s, "item-1")
			assert.Equal(t, terminal, got.Status)
			require.NotNil(t, got.EndedAt)
			assert.Equal(t, endedAt, *got.EndedAt)
		})
	}
}

func TestAddNotification(t *testing.T) {
	ctx := context.Background()
	s := NewMemory()
	require.NoError(t, s.CreateItem(ctx, newItem("item-1", t0)))

	reminder := Notification{Kind: KindExpiringSoon, Message: "soon", SentAt: t0.Add(time.Hour)}
	expired := Notification{Kind: KindExpired, Message: "expired", SentAt: t0.Add(2 * time.Hour)}
	require.NoError(t, s.AddNotification(ctx, "item-1", reminder))
	require.NoError(t, s.AddNotification(ctx, "item-1", expired))

	// A retried delivery is a no-op, even with a different timestamp.
	retry := reminder
	retry.SentAt = t0.Add(90 * time.Minute)
	require.NoError(t, s.AddNotification(ctx, "item-1", retry))

	assert.Equal(t, []Notification{reminder, expired}, mustGet(t, s, "item-1").Notifications)
	assert.ErrorIs(t, s.AddNotification(ctx, "missing", reminder), ErrNotFound)
}

func TestGetMissing(t *testing.T) {
	_, err := NewMemory().GetItem(context.Background(), "missing")
	assert.True(t, errors.Is(err, ErrNotFound))
}

func TestListItems(t *testing.T) {
	ctx := context.Background()
	s := NewMemory()
	require.NoError(t, s.CreateItem(ctx, newItem("item-a", t0)))
	require.NoError(t, s.CreateItem(ctx, newItem("item-b", t0.Add(2*time.Hour))))
	require.NoError(t, s.CreateItem(ctx, newItem("item-c", t0.Add(time.Hour))))
	require.NoError(t, s.CreateItem(ctx, newItem("item-d", t0.Add(time.Hour)))) // same time as c
	require.NoError(t, s.SetStatus(ctx, "item-c", StatusExpired, t0))

	ids := func(items []Item) []string {
		out := make([]string, len(items))
		for i, it := range items {
			out[i] = it.ID
		}
		return out
	}

	all, err := s.ListItems(ctx, ListFilter{})
	require.NoError(t, err)
	assert.Equal(t, []string{"item-b", "item-c", "item-d", "item-a"}, ids(all))

	expired, err := s.ListItems(ctx, ListFilter{Status: StatusExpired})
	require.NoError(t, err)
	assert.Equal(t, []string{"item-c"}, ids(expired))

	none, err := s.ListItems(ctx, ListFilter{Status: StatusCollected})
	require.NoError(t, err)
	assert.NotNil(t, none)
	assert.Empty(t, none)
}

func TestReturnedItemsAreCopies(t *testing.T) {
	ctx := context.Background()
	s := NewMemory()
	in := newItem("item-1", t0)
	require.NoError(t, s.CreateItem(ctx, in))
	require.NoError(t, s.AddNotification(ctx, "item-1", Notification{Kind: KindExpiringSoon, Message: "soon"}))
	require.NoError(t, s.SetStatus(ctx, "item-1", StatusCollected, t0.Add(time.Hour)))

	// Changing what the caller passed in or got back must not reach the store.
	in.ItemName = "changed"
	got := mustGet(t, s, "item-1")
	got.Notifications[0].Message = "changed"
	*got.EndedAt = t0.Add(99 * time.Hour)
	list, err := s.ListItems(ctx, ListFilter{})
	require.NoError(t, err)
	list[0].Notifications[0].Message = "changed"

	fresh := mustGet(t, s, "item-1")
	assert.Equal(t, "Bike frame", fresh.ItemName)
	assert.Equal(t, "soon", fresh.Notifications[0].Message)
	assert.Equal(t, t0.Add(time.Hour), *fresh.EndedAt)
}

// Run with -race: exercises the store the way one workflow per item plus
// polling API handlers would.
func TestConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	s := NewMemory()
	const items = 50

	var wg sync.WaitGroup
	for i := range items {
		id := fmt.Sprintf("item-%02d", i)
		wg.Go(func() {
			assert.NoError(t, s.CreateItem(ctx, newItem(id, t0.Add(time.Duration(i)*time.Minute))))
			assert.NoError(t, s.AddNotification(ctx, id, Notification{Kind: KindExpiringSoon}))
			assert.NoError(t, s.SetStatus(ctx, id, StatusExpiringSoon, t0))
			// A terminal Update racing a late timer-driven write.
			var inner sync.WaitGroup
			inner.Go(func() { assert.NoError(t, s.SetStatus(ctx, id, StatusCollected, t0)) })
			inner.Go(func() { assert.NoError(t, s.SetStatus(ctx, id, StatusExpired, t0)) })
			inner.Wait()
		})
		wg.Go(func() {
			_, _ = s.GetItem(ctx, id)
			_, err := s.ListItems(ctx, ListFilter{})
			assert.NoError(t, err)
		})
	}
	wg.Wait()

	all, err := s.ListItems(ctx, ListFilter{})
	require.NoError(t, err)
	require.Len(t, all, items)
	for _, it := range all {
		// Whichever write won, Collected must stick once it lands: either
		// Expired came first and Collected overwrote it, or Collected came
		// first and the guard blocked Expired.
		assert.Equal(t, StatusCollected, it.Status, it.ID)
		assert.Len(t, it.Notifications, 1)
	}
}
