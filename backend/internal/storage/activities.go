package storage

import (
	"context"

	"go.temporal.io/sdk/activity"
)

// SendNotification delivers a notification to the member. In the demo,
// delivery means logging it; the workflow records it for the UI once this
// succeeds. Real email or SMS would be added here without changing the
// workflow.
func SendNotification(ctx context.Context, n Notification) error {
	activity.GetLogger(ctx).Info("notification sent",
		"itemID", n.ItemID,
		"member", n.MemberName,
		"kind", n.Kind,
		"message", n.Message,
	)
	return nil
}
