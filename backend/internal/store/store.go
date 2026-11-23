// Package store is the application's record of stored items and their
// notifications. The workflow writes to it through activities; the HTTP API
// reads from it. See docs/architecture.md#store.
package store

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned when no item has the given ID.
var ErrNotFound = errors.New("item not found")

// Store holds items. Implementations must be safe for concurrent use.
//
// Writes are idempotent so that retried activities are harmless:
//   - CreateItem with an ID that already exists is a no-op.
//   - AddNotification is a no-op if the item already has one of that kind.
//   - SetStatus never changes an item that is already in a terminal status.
//
// All timestamps are supplied by the caller (the workflow clock).
type Store interface {
	CreateItem(ctx context.Context, item Item) error
	// SetStatus changes an item's status. Setting a terminal status also
	// sets EndedAt to at.
	SetStatus(ctx context.Context, id string, status Status, at time.Time) error
	AddNotification(ctx context.Context, id string, n Notification) error
	GetItem(ctx context.Context, id string) (Item, error)
	// ListItems returns items newest first (by StoredAt).
	ListItems(ctx context.Context, f ListFilter) ([]Item, error)
}

// Status is where an item is in its lifecycle.
type Status string

const (
	StatusStored         Status = "Stored"
	StatusExpiringSoon   Status = "ExpiringSoon"
	StatusExpired        Status = "Expired"
	StatusCollected      Status = "Collected"
	StatusRemovedByAdmin Status = "RemovedByAdmin"
)

// Valid reports whether s is a known status.
func (s Status) Valid() bool {
	switch s {
	case StatusStored, StatusExpiringSoon, StatusExpired, StatusCollected, StatusRemovedByAdmin:
		return true
	}
	return false
}

// Terminal reports whether the status ends tracking.
func (s Status) Terminal() bool {
	return s == StatusCollected || s == StatusRemovedByAdmin
}

// NotificationKind identifies why a notification was sent.
type NotificationKind string

const (
	KindExpiringSoon   NotificationKind = "ExpiringSoon"
	KindExpired        NotificationKind = "Expired"
	KindRemovedByAdmin NotificationKind = "RemovedByAdmin"
)

// Item is one stored item.
type Item struct {
	ID              string
	MemberName      string
	ItemName        string
	Description     string
	StorageDuration time.Duration
	StoredAt        time.Time
	ReminderAt      time.Time
	ExpiresAt       time.Time
	Status          Status
	EndedAt         *time.Time // set once the status is terminal
	Notifications   []Notification
}

// Notification is a message delivered to the item's member, shown in the UI.
type Notification struct {
	Kind    NotificationKind
	Message string
	SentAt  time.Time
}

// ListFilter narrows ListItems. The zero value lists every item.
type ListFilter struct {
	Status Status // only items with this status, if set
}

// clone returns a deep copy of the item, so callers never share memory with
// the store.
func (it Item) clone() Item {
	if it.EndedAt != nil {
		t := *it.EndedAt
		it.EndedAt = &t
	}
	it.Notifications = append([]Notification(nil), it.Notifications...)
	return it
}
