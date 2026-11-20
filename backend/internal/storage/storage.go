// Package storage holds the StorageItemWorkflow, its activities, and the types
// shared with the API. See docs/storage-workflow.md.
package storage

import (
	"time"

	"go.temporal.io/sdk/temporal"
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

// ErrTypeItemEnded is the ApplicationError type returned when a collect or
// remove Update is rejected because the item has already ended (or is ending).
const ErrTypeItemEnded = "ItemEnded"

// ErrTypeInvalidInput is the ApplicationError type the workflow fails with
// when it is started with invalid input.
const ErrTypeInvalidInput = "InvalidInput"

// ItemStatusKey is the custom search attribute that holds the item's Status.
// The dashboard filters on it.
var ItemStatusKey = temporal.NewSearchAttributeKeyKeyword("ItemStatus")

// Status is where an item is in its lifecycle.
type Status string

const (
	StatusStored         Status = "Stored"
	StatusExpiringSoon   Status = "ExpiringSoon"
	StatusExpired        Status = "Expired"
	StatusCollected      Status = "Collected"
	StatusRemovedByAdmin Status = "RemovedByAdmin"
)

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

// StoreItemInput starts a StorageItemWorkflow.
type StoreItemInput struct {
	ItemID          string
	MemberName      string
	ItemName        string
	Description     string
	StorageDuration time.Duration // fixed per run; set by the API
}

// Notification is what the SendNotification activity delivers.
type Notification struct {
	ItemID     string
	MemberName string
	Kind       NotificationKind
	Message    string
}

// SentNotification is a notification the workflow has delivered, as shown in
// the UI.
type SentNotification struct {
	Kind    NotificationKind
	Message string
	SentAt  time.Time
}

// ItemState is the workflow's view of one item. The getStatus Query and both
// Updates return it.
type ItemState struct {
	StoreItemInput
	StoredAt      time.Time
	ReminderAt    time.Time
	ExpiresAt     time.Time
	Status        Status
	EndedAt       *time.Time `json:",omitempty"`
	Notifications []SentNotification
}
