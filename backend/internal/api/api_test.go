package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/temporal"

	"github.com/gosixtyfive/temporal-storage-tracker/backend/internal/storage"
	"github.com/gosixtyfive/temporal-storage-tracker/backend/internal/store"
)

const (
	id1 = "item-00000000-0000-4000-8000-000000000001"
	id2 = "item-00000000-0000-4000-8000-000000000002"
	id3 = "item-00000000-0000-4000-8000-000000000003"
)

var t0 = time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)

// fakeWorkflows stands in for Temporal. Unless err is set, an Update applies
// the effect the real workflow would have on the store.
type fakeWorkflows struct {
	store   *store.Memory
	err     error
	started []storage.StoreItemInput
	updates []string
	// hadDeadline records whether each call's context carried a deadline.
	hadDeadline []bool
}

func (f *fakeWorkflows) Start(ctx context.Context, in storage.StoreItemInput) error {
	_, ok := ctx.Deadline()
	f.hadDeadline = append(f.hadDeadline, ok)
	f.started = append(f.started, in)
	return f.err
}

func (f *fakeWorkflows) Update(ctx context.Context, itemID, update string) error {
	_, ok := ctx.Deadline()
	f.hadDeadline = append(f.hadDeadline, ok)
	f.updates = append(f.updates, update+" "+itemID)
	if f.err != nil {
		return f.err
	}
	at := t0.Add(time.Hour)
	switch update {
	case storage.UpdateCollect:
		return f.store.SetStatus(ctx, itemID, store.StatusCollected, at)
	case storage.UpdateRemove:
		if err := f.store.AddNotification(ctx, itemID, store.Notification{
			Kind: store.KindRemovedByAdmin, Message: "removed", SentAt: at,
		}); err != nil {
			return err
		}
		return f.store.SetStatus(ctx, itemID, store.StatusRemovedByAdmin, at)
	}
	return fmt.Errorf("unknown update %q", update)
}

type fixture struct {
	t     *testing.T
	store *store.Memory
	wf    *fakeWorkflows
	mux   *http.ServeMux
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	mem := store.NewMemory()
	wf := &fakeWorkflows{store: mem}
	mux := http.NewServeMux()
	New(Config{
		Store:           mem,
		Workflows:       wf,
		StorageDuration: 7 * time.Minute,
		Logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		NewID:           func() string { return id1 },
	}).Register(mux)
	return &fixture{t: t, store: mem, wf: wf, mux: mux}
}

func (f *fixture) addItem(id string, storedAt time.Time, status store.Status) {
	f.t.Helper()
	require.NoError(f.t, f.store.CreateItem(context.Background(), store.Item{
		ID:              id,
		MemberName:      "Sam",
		ItemName:        "Bike frame",
		Description:     "Steel frame",
		StorageDuration: 7 * time.Minute,
		StoredAt:        storedAt,
		ReminderAt:      storedAt.Add(6 * time.Minute),
		ExpiresAt:       storedAt.Add(7 * time.Minute),
	}))
	if status != store.StatusStored {
		require.NoError(f.t, f.store.SetStatus(context.Background(), id, status, storedAt.Add(time.Minute)))
	}
}

func (f *fixture) do(method, path, body string) *httptest.ResponseRecorder {
	f.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	var v T
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &v), rec.Body.String())
	return v
}

func errorMessage(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	return decode[map[string]string](t, rec)["error"]
}

func TestCreateItem(t *testing.T) {
	f := newFixture(t)
	rec := f.do("POST", "/api/items",
		`{"memberName":"  Sam ","itemName":"Bike frame","description":" Steel frame "}`)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.Equal(t, "/api/items/"+id1, rec.Header().Get("Location"))
	assert.Equal(t, map[string]string{"id": id1}, decode[map[string]string](t, rec))

	require.Len(t, f.wf.started, 1)
	assert.Equal(t, storage.StoreItemInput{
		ItemID:          id1,
		MemberName:      "Sam",
		ItemName:        "Bike frame",
		Description:     "Steel frame",
		StorageDuration: 7 * time.Minute,
	}, f.wf.started[0])
	assert.Equal(t, []bool{true}, f.wf.hadDeadline, "Temporal calls are bounded by a timeout")
}

func TestCreateItemDefaultID(t *testing.T) {
	s := New(Config{})
	assert.Regexp(t, itemIDPattern, s.cfg.NewID())
	assert.NotEqual(t, s.cfg.NewID(), s.cfg.NewID())
}

func TestCreateItemValidation(t *testing.T) {
	long := strings.Repeat("x", maxNameLen+1)
	for name, tc := range map[string]struct{ body, msg string }{
		"invalid JSON":     {`{"memberName":`, "invalid JSON body"},
		"missing member":   {`{"itemName":"Bike"}`, "memberName is required"},
		"blank member":     {`{"memberName":"  ","itemName":"Bike"}`, "memberName is required"},
		"missing item":     {`{"memberName":"Sam"}`, "itemName is required"},
		"member too long":  {`{"memberName":"` + long + `","itemName":"Bike"}`, "memberName is too long"},
		"item too long":    {`{"memberName":"Sam","itemName":"` + long + `"}`, "itemName is too long"},
		"description long": {`{"memberName":"Sam","itemName":"Bike","description":"` + strings.Repeat("x", maxDescriptionLen+1) + `"}`, "description is too long"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			rec := f.do("POST", "/api/items", tc.body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Equal(t, tc.msg, errorMessage(t, rec))
			assert.Empty(t, f.wf.started, "no workflow is started for an invalid request")
		})
	}
}

func TestCreateItemWorkflowErrors(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code int
	}{
		{ErrUnavailable, http.StatusServiceUnavailable},
		{ErrTimeout, http.StatusGatewayTimeout},
		{errors.New("boom"), http.StatusInternalServerError},
	} {
		f := newFixture(t)
		f.wf.err = tc.err
		rec := f.do("POST", "/api/items", `{"memberName":"Sam","itemName":"Bike"}`)
		assert.Equal(t, tc.code, rec.Code, tc.err.Error())
	}
}

func TestGetItem(t *testing.T) {
	f := newFixture(t)
	f.addItem(id1, t0, store.StatusStored)
	require.NoError(t, f.store.AddNotification(context.Background(), id1, store.Notification{
		Kind: store.KindExpiringSoon, Message: "soon", SentAt: t0.Add(6 * time.Minute),
	}))

	rec := f.do("GET", "/api/items/"+id1, "")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{
		"id": "`+id1+`",
		"memberName": "Sam",
		"itemName": "Bike frame",
		"description": "Steel frame",
		"storageDurationSeconds": 420,
		"storedAt": "2026-11-02T09:00:00Z",
		"reminderAt": "2026-11-02T09:06:00Z",
		"expiresAt": "2026-11-02T09:07:00Z",
		"status": "Stored",
		"endedAt": null,
		"notifications": [
			{"kind": "ExpiringSoon", "message": "soon", "sentAt": "2026-11-02T09:06:00Z"}
		]
	}`, rec.Body.String())
}

func TestGetItemNotFound(t *testing.T) {
	f := newFixture(t)
	for _, path := range []string{
		"/api/items/" + id1,         // well-formed but not (yet) in the store: pending
		"/api/items/not-an-item-id", // malformed
		"/api/items/item-123",
	} {
		rec := f.do("GET", path, "")
		assert.Equal(t, http.StatusNotFound, rec.Code, path)
		assert.Equal(t, "item not found", errorMessage(t, rec))
	}
}

func TestListItems(t *testing.T) {
	f := newFixture(t)
	f.addItem(id1, t0, store.StatusStored)
	f.addItem(id2, t0.Add(2*time.Minute), store.StatusExpired)
	f.addItem(id3, t0.Add(time.Minute), store.StatusCollected)

	type list struct {
		Items []struct {
			ID            string            `json:"id"`
			Status        string            `json:"status"`
			EndedAt       *time.Time        `json:"endedAt"`
			Notifications []json.RawMessage `json:"notifications"`
		} `json:"items"`
	}
	ids := func(l list) []string {
		out := []string{}
		for _, it := range l.Items {
			out = append(out, it.ID)
		}
		return out
	}

	rec := f.do("GET", "/api/items", "")
	require.Equal(t, http.StatusOK, rec.Code)
	all := decode[list](t, rec)
	assert.Equal(t, []string{id2, id3, id1}, ids(all), "newest first")
	assert.NotNil(t, all.Items[1].EndedAt)
	assert.NotNil(t, all.Items[0].Notifications, "notifications is [] rather than null")

	rec = f.do("GET", "/api/items?status=Expired", "")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, []string{id2}, ids(decode[list](t, rec)))

	rec = f.do("GET", "/api/items?status=RemovedByAdmin", "")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"items":[]}`, rec.Body.String())

	rec = f.do("GET", "/api/items?status=Lost", "")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "unknown status Lost", errorMessage(t, rec))
}

func TestCollectAndRemove(t *testing.T) {
	for _, tc := range []struct {
		action string
		status store.Status
		notes  int
	}{
		{"collect", store.StatusCollected, 0},
		{"remove", store.StatusRemovedByAdmin, 1},
	} {
		t.Run(tc.action, func(t *testing.T) {
			f := newFixture(t)
			f.addItem(id1, t0, store.StatusExpired)

			rec := f.do("POST", "/api/items/"+id1+"/"+tc.action, "")
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			assert.Equal(t, []string{tc.action + " " + id1}, f.wf.updates)
			assert.Equal(t, []bool{true}, f.wf.hadDeadline, "Temporal calls are bounded by a timeout")

			// The response is the item as stored after the Update completed.
			got := decode[itemJSON](t, rec)
			assert.Equal(t, tc.status, got.Status)
			require.NotNil(t, got.EndedAt)
			assert.Equal(t, t0.Add(time.Hour), *got.EndedAt)
			assert.Len(t, got.Notifications, tc.notes)
		})
	}
}

func TestUpdateChecksStoreFirst(t *testing.T) {
	f := newFixture(t)
	f.addItem(id1, t0, store.StatusCollected)

	rec := f.do("POST", "/api/items/"+id1+"/remove", "")
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Equal(t, "item is already Collected", errorMessage(t, rec))

	for _, path := range []string{"/api/items/" + id2 + "/collect", "/api/items/bogus/remove"} {
		rec = f.do("POST", path, "")
		assert.Equal(t, http.StatusNotFound, rec.Code, path)
	}

	assert.Empty(t, f.wf.updates, "Temporal is not called for unknown or ended items")
}

func TestUpdateWorkflowErrors(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code int
	}{
		{ErrItemEnded, http.StatusConflict},
		{ErrUnavailable, http.StatusServiceUnavailable},
		{ErrTimeout, http.StatusGatewayTimeout},
		{errors.New("boom"), http.StatusInternalServerError},
	} {
		f := newFixture(t)
		f.addItem(id1, t0, store.StatusStored)
		f.wf.err = tc.err
		rec := f.do("POST", "/api/items/"+id1+"/collect", "")
		assert.Equal(t, tc.code, rec.Code, tc.err.Error())
	}
}

func TestConfig(t *testing.T) {
	f := newFixture(t)
	rec := f.do("GET", "/api/config", "")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"storageDurationSeconds":420,"reminderLeadSeconds":60}`, rec.Body.String())
}

func TestClassify(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   error
		want error
	}{
		{"nil", nil, nil},
		{"validator rejection", temporal.NewApplicationError("item is already Collected", storage.ErrTypeItemEnded), ErrItemEnded},
		{"wrapped rejection", fmt.Errorf("update: %w", temporal.NewApplicationError("x", storage.ErrTypeItemEnded)), ErrItemEnded},
		{"workflow completed", serviceerror.NewNotFound("workflow execution already completed"), ErrItemEnded},
		{"unavailable", serviceerror.NewUnavailable("connection refused"), ErrUnavailable},
		{"server deadline", serviceerror.NewDeadlineExceeded("deadline"), ErrTimeout},
		{"context deadline", fmt.Errorf("get: %w", context.DeadlineExceeded), ErrTimeout},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, classify(tc.in))
		})
	}

	other := temporal.NewApplicationError("nope", "SomethingElse")
	assert.Same(t, other, classify(other), "unrecognised errors pass through")
}
