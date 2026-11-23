// Command worker hosts the StorageItemWorkflow and its activities.
package main

import (
	"log/slog"
	"os"

	"go.temporal.io/sdk/worker"

	"github.com/gosixtyfive/temporal-storage-tracker/backend/internal/storage"
	"github.com/gosixtyfive/temporal-storage-tracker/backend/internal/store"
	"github.com/gosixtyfive/temporal-storage-tracker/backend/internal/temporalclient"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	c, err := temporalclient.Dial(logger)
	if err != nil {
		logger.Error("connect to Temporal", "err", err)
		os.Exit(1)
	}
	defer c.Close()

	w := worker.New(c, storage.TaskQueue, worker.Options{})
	w.RegisterWorkflow(storage.StorageItemWorkflow)
	// Interim: a private store until cmd/server shares one with the API (tst-ofp.10).
	w.RegisterActivity(&storage.Activities{Store: store.NewMemory()})

	logger.Info("worker starting", "taskQueue", storage.TaskQueue)
	if err := w.Run(worker.InterruptCh()); err != nil {
		logger.Error("worker stopped", "err", err)
		os.Exit(1)
	}
}
