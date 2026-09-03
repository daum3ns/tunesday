// Package stream resolves YouTube video IDs to playable audio-only stream
// URLs for the radio room's direct-stream mode. Extraction is delegated to
// yt-dlp (see ytdlp.go) — the same proven path the CLI's mpv radio uses.
// Signed upstream URLs expire, so results are cached and identical lookups
// are deduplicated while in flight.
package stream

import (
	"context"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// Resolver turns a YouTube video ID into an audio stream. Production uses
// Cached(YTDLP); tests substitute fakes at this seam.
type Resolver interface {
	Resolve(ctx context.Context, videoID string) (Info, error)
}

// Info is one playable audio stream.
type Info struct {
	URL       string
	MimeType  string
	ExpiresAt time.Time // zero means "unknown, trust the cache TTL"
}

// Valid reports whether the info is still usable at time now.
func (i Info) Valid(now time.Time) bool {
	return i.URL != "" && (i.ExpiresAt.IsZero() || now.Before(i.ExpiresAt))
}

// Cached decorates a Resolver with a TTL cache and in-flight dedupe.
type Cached struct {
	inner Resolver
	ttl   time.Duration
	max   int

	mu      sync.Mutex
	entries map[string]entry
	group   singleflight.Group
	now     func() time.Time
}

type entry struct {
	info      Info
	fetchedAt time.Time
}

// NewCached wraps inner. ttl <= 0 uses 3 hours; maxEntries <= 0 uses 256.
func NewCached(inner Resolver, ttl time.Duration, maxEntries int) *Cached {
	if ttl <= 0 {
		ttl = 3 * time.Hour
	}
	if maxEntries <= 0 {
		maxEntries = 256
	}
	return &Cached{
		inner:   inner,
		ttl:     ttl,
		max:     maxEntries,
		entries: map[string]entry{},
		now:     time.Now,
	}
}

// Resolve returns a cached stream or fetches one, collapsing concurrent
// lookups for the same video into a single upstream call.
func (c *Cached) Resolve(ctx context.Context, videoID string) (Info, error) {
	if videoID == "" {
		return Info{}, errors.New("empty video id")
	}

	c.mu.Lock()
	if e, ok := c.entries[videoID]; ok && c.fresh(e) {
		c.mu.Unlock()
		return e.info, nil
	}
	c.mu.Unlock()

	ch := c.group.DoChan(videoID, func() (any, error) {
		fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		info, err := c.inner.Resolve(fctx, videoID)
		if err != nil {
			return nil, err
		}
		c.store(videoID, info)
		return info, nil
	})

	select {
	case <-ctx.Done():
		return Info{}, ctx.Err()
	case res := <-ch:
		if res.Err != nil {
			return Info{}, res.Err
		}
		return res.Val.(Info), nil
	}
}

func (c *Cached) fresh(e entry) bool {
	if !e.info.Valid(c.now()) {
		return false
	}
	return c.now().Sub(e.fetchedAt) < c.ttl
}

func (c *Cached) store(videoID string, info Info) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[videoID] = entry{info: info, fetchedAt: c.now()}
	if len(c.entries) <= c.max {
		return
	}
	// Evict the oldest entries down to the cap.
	type kv struct {
		key string
		at  time.Time
	}
	all := make([]kv, 0, len(c.entries))
	for k, e := range c.entries {
		all = append(all, kv{k, e.fetchedAt})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].at.Before(all[j].at) })
	for _, old := range all[:len(all)-c.max] {
		delete(c.entries, old.key)
	}
}

// Invalidate drops a cached entry (used when the upstream URL fails).
func (c *Cached) Invalidate(videoID string) {
	c.mu.Lock()
	delete(c.entries, videoID)
	c.mu.Unlock()
}

// mimeFromURL extracts a media type hint from a googlevideo-style URL
// (`&mime=audio%2Fmp4`), defaulting to audio/mp4.
func mimeFromURL(raw string) string {
	if i := strings.Index(raw, "?"); i >= 0 {
		for _, part := range strings.Split(raw[i+1:], "&") {
			k, v, found := strings.Cut(part, "=")
			if found && k == "mime" {
				if mt, err := url.QueryUnescape(v); err == nil && mt != "" {
					return mt
				}
			}
		}
	}
	return "audio/mp4"
}

// expireFromURL extracts the signed expiry (`&expire=1712345678`), if any.
func expireFromURL(raw string) time.Time {
	if i := strings.Index(raw, "?"); i >= 0 {
		for _, part := range strings.Split(raw[i+1:], "&") {
			k, v, found := strings.Cut(part, "=")
			if found && k == "expire" {
				if unix, err := strconv.ParseInt(v, 10, 64); err == nil && unix > 0 {
					return time.Unix(unix, 0)
				}
			}
		}
	}
	return time.Time{}
}
