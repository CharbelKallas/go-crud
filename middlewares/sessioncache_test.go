package middlewares

import (
	"context"
	"errors"
	"testing"
	"time"
)

type countingStore struct {
	calls   int
	version int64
	err     error
}

func (s *countingStore) SessionVersion(context.Context, int64) (int64, error) {
	s.calls++
	return s.version, s.err
}

func TestCachedSessions(t *testing.T) {
	store := &countingStore{version: 3}
	c := NewCachedSessions(store, 5*time.Second)
	now := time.Unix(1_700_000_000, 0)
	c.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		if v, err := c.SessionVersion(context.Background(), 1); v != 3 || err != nil {
			t.Fatalf("v=%d err=%v", v, err)
		}
	}
	if store.calls != 1 {
		t.Fatalf("store called %d times within ttl, want 1", store.calls)
	}

	store.version = 4 // sessions revoked
	now = now.Add(5 * time.Second)
	if v, _ := c.SessionVersion(context.Background(), 1); v != 4 {
		t.Fatalf("stale version %d after ttl", v)
	}

	store.version = 5
	c.Invalidate(1)
	if v, _ := c.SessionVersion(context.Background(), 1); v != 5 {
		t.Fatalf("version %d after Invalidate, want 5", v)
	}

	store.err = errors.New("db down")
	if _, err := c.SessionVersion(context.Background(), 2); err == nil {
		t.Fatal("error swallowed")
	}
	store.err = nil
	calls := store.calls
	c.SessionVersion(context.Background(), 2)
	if store.calls != calls+1 {
		t.Fatal("an error was cached")
	}
}
