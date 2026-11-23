package storage

import (
	"context"
	"errors"
	"fmt"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"

	"github.com/gosixtyfive/temporal-storage-tracker/backend/internal/store"
)

// Activities perform the workflow's writes to the store. Register a
// *Activities with the worker; the workflow refers to its methods by name.
//
// Every store write is idempotent, so Temporal's retries are safe.
type Activities struct {
	Store store.Store
}

// CreateItem writes the new item. It is the workflow's first step.
func (a *Activities) CreateItem(ctx context.Context, item store.Item) error {
	return a.Store.CreateItem(ctx, item)
}

// SetItemStatus records a status change. The store ignores changes to an
// item that has already ended.
func (a *Activities) SetItemStatus(ctx context.Context, in SetItemStatusInput) error {
	return notFoundIsFinal(a.Store.SetStatus(ctx, in.ItemID, in.Status, in.At), in.ItemID)
}

// SendNotification delivers a notification to the member. In the demo,
// delivery means logging it and recording it in the store, where the UI shows
// it. Real email or SMS would be added here without changing the workflow.
func (a *Activities) SendNotification(ctx context.Context, n Notification) error {
	err := a.Store.AddNotification(ctx, n.ItemID, store.Notification{
		Kind:    n.Kind,
		Message: n.Message,
		SentAt:  n.SentAt,
	})
	if err != nil {
		return notFoundIsFinal(err, n.ItemID)
	}
	activity.GetLogger(ctx).Info("notification sent",
		"itemID", n.ItemID,
		"member", n.MemberName,
		"kind", n.Kind,
		"message", n.Message,
	)
	return nil
}

// notFoundIsFinal turns store.ErrNotFound into a non-retryable error: the
// item will not appear by retrying (for example, the in-memory store was lost
// on a restart).
func notFoundIsFinal(err error, itemID string) error {
	if errors.Is(err, store.ErrNotFound) {
		return temporal.NewNonRetryableApplicationError(
			fmt.Sprintf("item %s not found in store", itemID), ErrTypeItemNotFound, err)
	}
	return err
}
