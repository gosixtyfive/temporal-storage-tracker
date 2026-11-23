// Package api serves the HTTP API the frontend uses. Reads come from the item
// store; every change goes through Temporal. See docs/architecture.md#http-api.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/gosixtyfive/temporal-storage-tracker/backend/internal/storage"
	"github.com/gosixtyfive/temporal-storage-tracker/backend/internal/store"
)

const (
	maxBodyBytes      = 64 << 10
	maxNameLen        = 100
	maxDescriptionLen = 1000
	// defaultTemporalTimeout bounds each call to Temporal. An Update blocks
	// until a worker runs it, so without a bound a stopped worker would hang
	// the request.
	defaultTemporalTimeout = 10 * time.Second
)

var itemIDPattern = regexp.MustCompile(`^item-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Config wires the API to its dependencies.
type Config struct {
	Store           store.Store
	Workflows       Workflows
	StorageDuration time.Duration
	Logger          *slog.Logger
	// TemporalTimeout bounds each Temporal call; defaults to 10s.
	TemporalTimeout time.Duration
	// NewID generates item IDs; defaults to "item-" + a random UUID.
	NewID func() string
}

// Server holds the API handlers.
type Server struct {
	cfg Config
}

// New returns the API, filling in defaults for optional Config fields.
func New(cfg Config) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.TemporalTimeout == 0 {
		cfg.TemporalTimeout = defaultTemporalTimeout
	}
	if cfg.NewID == nil {
		cfg.NewID = func() string { return "item-" + uuid.NewString() }
	}
	return &Server{cfg: cfg}
}

// Register adds the API routes to mux.
func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/items", s.createItem)
	mux.HandleFunc("GET /api/items", s.listItems)
	mux.HandleFunc("GET /api/items/{id}", s.getItem)
	mux.HandleFunc("POST /api/items/{id}/collect", s.update(storage.UpdateCollect))
	mux.HandleFunc("POST /api/items/{id}/remove", s.update(storage.UpdateRemove))
	mux.HandleFunc("GET /api/config", s.config)
}

type createItemRequest struct {
	MemberName  string `json:"memberName"`
	ItemName    string `json:"itemName"`
	Description string `json:"description"`
}

func (r *createItemRequest) validate() string {
	r.MemberName = strings.TrimSpace(r.MemberName)
	r.ItemName = strings.TrimSpace(r.ItemName)
	r.Description = strings.TrimSpace(r.Description)
	switch {
	case r.MemberName == "":
		return "memberName is required"
	case r.ItemName == "":
		return "itemName is required"
	case utf8.RuneCountInString(r.MemberName) > maxNameLen:
		return "memberName is too long"
	case utf8.RuneCountInString(r.ItemName) > maxNameLen:
		return "itemName is too long"
	case utf8.RuneCountInString(r.Description) > maxDescriptionLen:
		return "description is too long"
	}
	return ""
}

// createItem starts the item's workflow and returns its ID. The workflow's
// first activity writes the item to the store, so it may briefly be missing
// from GET /api/items/{id}; the UI shows it as pending until then.
func (s *Server) createItem(w http.ResponseWriter, r *http.Request) {
	var req createItemRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if msg := req.validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	id := s.cfg.NewID()
	in := storage.StoreItemInput{
		ItemID:          id,
		MemberName:      req.MemberName,
		ItemName:        req.ItemName,
		Description:     req.Description,
		StorageDuration: s.cfg.StorageDuration,
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.TemporalTimeout)
	defer cancel()
	if err := s.cfg.Workflows.Start(ctx, in); err != nil {
		s.writeWorkflowError(w, err, "start", id)
		return
	}

	w.Header().Set("Location", "/api/items/"+id)
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *Server) listItems(w http.ResponseWriter, r *http.Request) {
	var f store.ListFilter
	if v := r.URL.Query().Get("status"); v != "" {
		f.Status = store.Status(v)
		if !f.Status.Valid() {
			writeError(w, http.StatusBadRequest, "unknown status "+v)
			return
		}
	}
	items, err := s.cfg.Store.ListItems(r.Context(), f)
	if err != nil {
		s.internalError(w, "list items", err)
		return
	}
	out := make([]itemJSON, len(items))
	for i, it := range items {
		out[i] = toItemJSON(it)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) getItem(w http.ResponseWriter, r *http.Request) {
	it, ok := s.lookup(w, r)
	if ok {
		writeJSON(w, http.StatusOK, toItemJSON(it))
	}
}

// update sends a collect or remove Update. It checks the store first, so
// unknown and already-ended items are answered without calling Temporal, and
// returns the item as stored once the Update has completed (the Update's own
// activity has written the new status by then).
func (s *Server) update(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		it, ok := s.lookup(w, r)
		if !ok {
			return
		}
		if it.Status.Terminal() {
			writeError(w, http.StatusConflict, "item is already "+string(it.Status))
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), s.cfg.TemporalTimeout)
		defer cancel()
		if err := s.cfg.Workflows.Update(ctx, it.ID, name); err != nil {
			s.writeWorkflowError(w, err, name, it.ID)
			return
		}

		it, err := s.cfg.Store.GetItem(r.Context(), it.ID)
		if err != nil {
			s.internalError(w, "read item after "+name, err)
			return
		}
		writeJSON(w, http.StatusOK, toItemJSON(it))
	}
}

type configJSON struct {
	StorageDurationSeconds int64 `json:"storageDurationSeconds"`
	ReminderLeadSeconds    int64 `json:"reminderLeadSeconds"`
}

func (s *Server) config(w http.ResponseWriter, _ *http.Request) {
	d := s.cfg.StorageDuration
	writeJSON(w, http.StatusOK, configJSON{
		StorageDurationSeconds: int64(d / time.Second),
		ReminderLeadSeconds:    int64(d / 7 / time.Second),
	})
}

// lookup reads the item named in the path, writing a 404 if the ID is
// malformed or the store does not have it (yet).
func (s *Server) lookup(w http.ResponseWriter, r *http.Request) (store.Item, bool) {
	id := r.PathValue("id")
	if !itemIDPattern.MatchString(id) {
		writeError(w, http.StatusNotFound, "item not found")
		return store.Item{}, false
	}
	it, err := s.cfg.Store.GetItem(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "item not found")
		return store.Item{}, false
	}
	if err != nil {
		s.internalError(w, "get item", err)
		return store.Item{}, false
	}
	return it, true
}

func (s *Server) writeWorkflowError(w http.ResponseWriter, err error, op, id string) {
	switch {
	case errors.Is(err, ErrItemEnded):
		writeError(w, http.StatusConflict, "item has already ended")
	case errors.Is(err, ErrUnavailable):
		writeError(w, http.StatusServiceUnavailable, "workflow service unavailable")
	case errors.Is(err, ErrTimeout):
		writeError(w, http.StatusGatewayTimeout, "timed out waiting for the workflow")
	default:
		s.cfg.Logger.Error("workflow call failed", "op", op, "itemID", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func (s *Server) internalError(w http.ResponseWriter, op string, err error) {
	s.cfg.Logger.Error("api error", "op", op, "err", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
