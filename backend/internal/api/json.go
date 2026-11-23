package api

import (
	"time"

	"github.com/gosixtyfive/temporal-storage-tracker/backend/internal/store"
)

// itemJSON is the API's representation of an item: camelCase keys, RFC 3339
// timestamps (UTC), durations in seconds.
type itemJSON struct {
	ID                     string             `json:"id"`
	MemberName             string             `json:"memberName"`
	ItemName               string             `json:"itemName"`
	Description            string             `json:"description"`
	StorageDurationSeconds int64              `json:"storageDurationSeconds"`
	StoredAt               time.Time          `json:"storedAt"`
	ReminderAt             time.Time          `json:"reminderAt"`
	ExpiresAt              time.Time          `json:"expiresAt"`
	Status                 store.Status       `json:"status"`
	EndedAt                *time.Time         `json:"endedAt"` // null until the item ends
	Notifications          []notificationJSON `json:"notifications"`
}

type notificationJSON struct {
	Kind    store.NotificationKind `json:"kind"`
	Message string                 `json:"message"`
	SentAt  time.Time              `json:"sentAt"`
}

func toItemJSON(it store.Item) itemJSON {
	out := itemJSON{
		ID:                     it.ID,
		MemberName:             it.MemberName,
		ItemName:               it.ItemName,
		Description:            it.Description,
		StorageDurationSeconds: int64(it.StorageDuration / time.Second),
		StoredAt:               it.StoredAt.UTC(),
		ReminderAt:             it.ReminderAt.UTC(),
		ExpiresAt:              it.ExpiresAt.UTC(),
		Status:                 it.Status,
		Notifications:          make([]notificationJSON, len(it.Notifications)),
	}
	if it.EndedAt != nil {
		t := it.EndedAt.UTC()
		out.EndedAt = &t
	}
	for i, n := range it.Notifications {
		out.Notifications[i] = notificationJSON{Kind: n.Kind, Message: n.Message, SentAt: n.SentAt.UTC()}
	}
	return out
}
