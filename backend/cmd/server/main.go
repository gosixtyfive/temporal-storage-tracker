// Command server runs the whole backend in one process: the HTTP API, the
// Temporal worker hosting StorageItemWorkflow and its activities, and the
// in-memory item store they share. See docs/architecture.md#components.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.temporal.io/sdk/worker"

	"github.com/gosixtyfive/temporal-storage-tracker/backend/internal/api"
	"github.com/gosixtyfive/temporal-storage-tracker/backend/internal/storage"
	"github.com/gosixtyfive/temporal-storage-tracker/backend/internal/store"
	"github.com/gosixtyfive/temporal-storage-tracker/backend/internal/temporalclient"
)

const defaultStorageDuration = 7 * 24 * time.Hour

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(logger); err != nil {
		logger.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	storageDuration, err := parseStorageDuration(os.Getenv("STORAGE_DURATION"))
	if err != nil {
		return err
	}

	c, err := temporalclient.Dial(logger)
	if err != nil {
		return fmt.Errorf("connect to Temporal: %w", err)
	}
	defer c.Close()

	// One store, shared by the worker's activities (writes) and the API
	// (reads). It lives only as long as this process.
	st := store.NewMemory()

	errCh := make(chan error, 2)

	w := worker.New(c, storage.TaskQueue, worker.Options{
		OnFatalError: func(err error) { errCh <- fmt.Errorf("worker: %w", err) },
	})
	w.RegisterWorkflow(storage.StorageItemWorkflow)
	w.RegisterActivity(&storage.Activities{Store: st})
	if err := w.Start(); err != nil {
		return fmt.Errorf("start worker: %w", err)
	}
	defer w.Stop()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	api.New(api.Config{
		Store:           st,
		Workflows:       api.NewWorkflows(c),
		StorageDuration: storageDuration,
		Logger:          logger,
	}).Register(mux)

	addr := ":" + getenv("PORT", "8080")
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("server listening",
			"addr", addr, "taskQueue", storage.TaskQueue, "storageDuration", storageDuration)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("http: %w", err)
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("shutting down")
	}

	// Stop taking requests first; the deferred w.Stop then lets in-flight
	// activities finish.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// parseStorageDuration reads STORAGE_DURATION as a Go duration (for example
// "168h" or "7m"), defaulting to 7 days.
func parseStorageDuration(s string) (time.Duration, error) {
	if s == "" {
		return defaultStorageDuration, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid STORAGE_DURATION %q: %w", s, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("STORAGE_DURATION must be positive, got %q", s)
	}
	return d, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
