package api

import (
	"context"
	"errors"

	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"

	"github.com/gosixtyfive/temporal-storage-tracker/backend/internal/storage"
)

// Workflows is what the API needs from Temporal: every change to an item
// goes through its workflow. Errors are classified into the Err* values
// below so handlers can map them to HTTP statuses.
type Workflows interface {
	Start(ctx context.Context, in storage.StoreItemInput) error
	// Update sends the collect or remove Update and waits for it to complete.
	Update(ctx context.Context, itemID, update string) error
}

var (
	// ErrItemEnded: the item has already ended, or another terminal Update
	// is in progress.
	ErrItemEnded = errors.New("item has already ended")
	// ErrUnavailable: Temporal could not be reached.
	ErrUnavailable = errors.New("temporal unavailable")
	// ErrTimeout: Temporal did not finish in time (for example, no worker).
	ErrTimeout = errors.New("timed out waiting for temporal")
)

// NewWorkflows returns Workflows backed by a Temporal client.
func NewWorkflows(c client.Client) Workflows {
	return &temporalWorkflows{c: c}
}

type temporalWorkflows struct {
	c client.Client
}

func (t *temporalWorkflows) Start(ctx context.Context, in storage.StoreItemInput) error {
	_, err := t.c.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        in.ItemID,
		TaskQueue: storage.TaskQueue,
	}, storage.StorageItemWorkflow, in)
	return classify(err)
}

func (t *temporalWorkflows) Update(ctx context.Context, itemID, update string) error {
	h, err := t.c.UpdateWorkflow(ctx, client.UpdateWorkflowOptions{
		WorkflowID:   itemID,
		UpdateName:   update,
		WaitForStage: client.WorkflowUpdateStageCompleted,
	})
	if err != nil {
		return classify(err)
	}
	return classify(h.Get(ctx, nil))
}

// classify maps Temporal errors to the API's error values. Unrecognised
// errors are returned unchanged.
func classify(err error) error {
	if err == nil {
		return nil
	}
	var appErr *temporal.ApplicationError
	var notFound *serviceerror.NotFound
	var unavailable *serviceerror.Unavailable
	var deadline *serviceerror.DeadlineExceeded
	switch {
	case errors.As(err, &appErr) && appErr.Type() == storage.ErrTypeItemEnded:
		return ErrItemEnded
	case errors.As(err, &notFound):
		// Handlers only send Updates for items the store has, so a missing
		// workflow means it has already completed.
		return ErrItemEnded
	case errors.As(err, &unavailable):
		return ErrUnavailable
	case errors.As(err, &deadline), errors.Is(err, context.DeadlineExceeded):
		return ErrTimeout
	}
	return err
}
