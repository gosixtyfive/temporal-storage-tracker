// Package temporalclient builds the Temporal client shared by the API and the
// worker, configured from the environment.
package temporalclient

import (
	"log/slog"
	"os"

	"go.temporal.io/sdk/client"
	tlog "go.temporal.io/sdk/log"
)

const (
	defaultHostPort  = "localhost:7233"
	defaultNamespace = "default"
)

// Dial connects to Temporal using TEMPORAL_ADDRESS and TEMPORAL_NAMESPACE,
// defaulting to the local dev server.
func Dial(logger *slog.Logger) (client.Client, error) {
	return client.Dial(client.Options{
		HostPort:  getenv("TEMPORAL_ADDRESS", defaultHostPort),
		Namespace: getenv("TEMPORAL_NAMESPACE", defaultNamespace),
		Logger:    tlog.NewStructuredLogger(logger),
	})
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
