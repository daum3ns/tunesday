package stream

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type stubResolver struct {
	calls atomic.Int64
	delay time.Duration
	err   error
}

func (s *stubResolver) Resolve(_ context.Context, id string) (Info, error) {
	s.calls.Add(1)
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	if s.err != nil {
		return Info{}, s.err
	}
	return Info{URL: "https://example/" + id, MimeType: "audio/mp4"}, nil
}

func TestCachedServesFromCache(t *testing.T) {
	inner := &stubResolver{}
	c := NewCached(inner, time.Hour, 10)

	for i := 0; i < 3; i++ {
		info, err := c.Resolve(context.Background(), "vid1")
		if err != nil || info.URL != "https://example/vid1" {
			t.Fatalf("resolve %d: %v %+v", i, err, info)
		}
	}
	if n := inner.calls.Load(); n != 1 {
		t.Fatalf("expected 1 upstream call, got %d", n)
	}
}

func TestCachedExpiry(t *testing.T) {
	inner := &stubResolver{}
	c := NewCached(inner, time.Minute, 10)
	now := time.Unix(1700000000, 0)
	c.now = func() time.Time { return now }

	if _, err := c.Resolve(context.Background(), "v"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(59 * time.Second)
	if _, err := c.Resolve(context.Background(), "v"); err != nil {
		t.Fatal(err)
	}
	if n := inner.calls.Load(); n != 1 {
		t.Fatalf("still fresh, expected 1 call, got %d", n)
	}

	now = now.Add(2 * time.Minute)
	if _, err := c.Resolve(context.Background(), "v"); err != nil {
		t.Fatal(err)
	}
	if n := inner.calls.Load(); n != 2 {
		t.Fatalf("expired entry must refetch, got %d calls", n)
	}
}

func TestCachedDedupesConcurrentLookups(t *testing.T) {
	inner := &stubResolver{delay: 50 * time.Millisecond}
	c := NewCached(inner, time.Hour, 10)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.Resolve(context.Background(), "same"); err != nil {
				t.Errorf("resolve: %v", err)
			}
		}()
	}
	wg.Wait()
	if n := inner.calls.Load(); n != 1 {
		t.Fatalf("singleflight should collapse to 1 call, got %d", n)
	}
}

func TestCachedEviction(t *testing.T) {
	inner := &stubResolver{}
	c := NewCached(inner, time.Hour, 2)
	step := time.Unix(1700000000, 0)
	c.now = func() time.Time { return step }

	c.Resolve(context.Background(), "a") //nolint
	step = step.Add(time.Second)
	c.Resolve(context.Background(), "b") //nolint
	step = step.Add(time.Second)
	c.Resolve(context.Background(), "c") //nolint -> evicts "a"
	step = step.Add(time.Second)
	c.Resolve(context.Background(), "a") //nolint -> refetch

	if n := inner.calls.Load(); n != 4 {
		t.Fatalf("expected oldest entry evicted (4 calls), got %d", n)
	}
}

func TestCachedErrorsAreNotCached(t *testing.T) {
	inner := &stubResolver{err: errors.New("bot wall")}
	c := NewCached(inner, time.Hour, 10)
	if _, err := c.Resolve(context.Background(), "x"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := c.Resolve(context.Background(), "x"); err == nil {
		t.Fatal("expected error again")
	}
	if n := inner.calls.Load(); n != 2 {
		t.Fatalf("failures must retry upstream, got %d calls", n)
	}
}

func TestInvalidate(t *testing.T) {
	inner := &stubResolver{}
	c := NewCached(inner, time.Hour, 10)
	c.Resolve(context.Background(), "v") //nolint
	c.Invalidate("v")
	c.Resolve(context.Background(), "v") //nolint
	if n := inner.calls.Load(); n != 2 {
		t.Fatalf("invalidate must force refetch, got %d", n)
	}
}
