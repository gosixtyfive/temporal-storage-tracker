// Package storage holds the StorageItemWorkflow, its activities, and the
// Temporal-facing types shared with the API. Domain types (Status,
// NotificationKind, Item) live in internal/store. See docs/storage-workflow.md.
package storage

import (
	"time"

	"github.com/gosixtyfive/temporal-storage-tracker/backend/internal/store"
)

// TaskQueue is the Temporal task queue the worker polls and the API starts
// workflows on.
const TaskQueue = "storage-tracker"

// Names of the workflow's Updates and Query, used by the API.
const (
	UpdateCollect  = "collect"
	UpdateRemove   = "remove"
	QueryGetStatus = "getStatus"
)

// ApplicationError types used by the workflow and its activities.
const (
	// ErrTypeItemEnded: a collect or remove Update was rejected because the
	// item has already ended (or is ending).
	ErrTypeItemEnded = "ItemEnded"
	// ErrTypeInvalidInput: the workflow was started with invalid input.
	ErrTypeInvalidInput = "InvalidInput"
	// ErrTypeItemNotFound: an activity found no item in the store. Not
	// retryable; the workflow fails.
	ErrTypeItemNotFound = "ItemNotFound"
)

// StoreItemInput starts a StorageItemWorkflow.
type StoreItemInput struct {
	ItemID          string
	MemberName      string
	ItemName        string
	Description     string
	StorageDuration time.Duration // fixed per run; set by the API
}

// Notification is what the SendNotification activity delivers. The workflow
// composes it, including SentAt from the workflow clock.
type Notification struct {
	ItemID     string
	MemberName string
	Kind       store.NotificationKind
	Message    string
	SentAt     time.Time
}

// SetItemStatusInput is the SetItemStatus activity's input.
type SetItemStatusInput struct {
	ItemID string
	Status store.Status
	At     time.Time
}

// ItemState is the workflow's own view of one item. It mirrors what the
// workflow has written to the store and is returned by the getStatus Query
// and both Updates; the application reads the store instead.
type ItemState struct {
	StoreItemInput
	StoredAt      time.Time
	ReminderAt    time.Time
	ExpiresAt     time.Time
	Status        store.Status
	EndedAt       *time.Time `json:",omitempty"`
	Notifications []store.Notification
}
