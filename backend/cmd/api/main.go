// Command api serves the HTTP API that the frontend uses. It is a thin layer
// over the Temporal client. See docs/architecture.md.
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

	"github.com/gosixtyfive/temporal-storage-tracker/backend/internal/temporalclient"
)

const defaultStorageDuration = 7 * 24 * time.Hour

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(logger); err != nil {
		logger.Error("api stopped", "err", err)
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

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	// API routes are added in tst-ofp.4.

	addr := ":" + getenv("PORT", "8080")
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("api listening", "addr", addr, "storageDuration", storageDuration)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

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
