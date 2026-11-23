package store

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"
)

// Memory is an in-memory Store. Its contents are lost when the process exits.
type Memory struct {
	mu    sync.RWMutex
	items map[string]Item
}

var _ Store = (*Memory)(nil)

// NewMemory returns an empty in-memory store.
func NewMemory() *Memory {
	return &Memory{items: make(map[string]Item)}
}

func (m *Memory) CreateItem(_ context.Context, item Item) error {
	if item.ID == "" {
		return errors.New("store: item ID is required")
	}
	if item.Status == "" {
		item.Status = StatusStored
	}
	if !item.Status.Valid() {
		return fmt.Errorf("store: invalid status %q", item.Status)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.items[item.ID]; ok {
		return nil // idempotent: a retried create keeps the original
	}
	m.items[item.ID] = item.clone()
	return nil
}

func (m *Memory) SetStatus(_ context.Context, id string, status Status, at time.Time) error {
	if !status.Valid() {
		return fmt.Errorf("store: invalid status %q", status)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	it, ok := m.items[id]
	if !ok {
		return ErrNotFound
	}
	if it.Status.Terminal() {
		return nil // terminal guard: a late write never overwrites the end state
	}
	it.Status = status
	if status.Terminal() {
		it.EndedAt = &at
	}
	m.items[id] = it
	return nil
}

func (m *Memory) AddNotification(_ context.Context, id string, n Notification) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	it, ok := m.items[id]
	if !ok {
		return ErrNotFound
	}
	for _, existing := range it.Notifications {
		if existing.Kind == n.Kind {
			return nil // idempotent: one notification per kind
		}
	}
	// Callers only ever hold clones, so appending to the stored slice is safe.
	it.Notifications = append(it.Notifications, n)
	m.items[id] = it
	return nil
}

func (m *Memory) GetItem(_ context.Context, id string) (Item, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	it, ok := m.items[id]
	if !ok {
		return Item{}, ErrNotFound
	}
	return it.clone(), nil
}

func (m *Memory) ListItems(_ context.Context, f ListFilter) ([]Item, error) {
	m.mu.RLock()
	out := make([]Item, 0, len(m.items))
	for _, it := range m.items {
		if f.Status != "" && it.Status != f.Status {
			continue
		}
		out = append(out, it.clone())
	}
	m.mu.RUnlock()

	slices.SortFunc(out, func(a, b Item) int {
		if c := b.StoredAt.Compare(a.StoredAt); c != 0 {
			return c // newest first
		}
		return cmp.Compare(a.ID, b.ID) // stable order for equal times
	})
	return out, nil
}
