// Package stream resolves YouTube video IDs to playable audio-only stream
// URLs for the radio room's direct-stream mode. Signed upstream URLs expire
// after roughly six hours, so results are cached with a conservative TTL
// and identical lookups are deduplicated while in flight.
package stream

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kkdai/youtube/v2"
	"golang.org/x/sync/singleflight"
)

// ErrNoAudio is returned when a video exposes no audio-only format.
var ErrNoAudio = errors.New("no audio-only stream available")

// Info is one playable audio stream.
type Info struct {
	URL      string
	MimeType string
}

// Resolver turns a YouTube video ID into an audio stream. Production code
// uses Cached(youtube-backed); tests substitute fakes at this seam.
type Resolver interface {
	Resolve(ctx context.Context, videoID string) (Info, error)
}

// YouTube resolves streams through the kkdai client (shared with the
// playlist package's tooling).
type YouTube struct{ c *youtube.Client }

// NewYouTube builds a live resolver.
func NewYouTube() *YouTube { return &YouTube{c: &youtube.Client{}} }

// Resolve fetches video metadata, prefers itag 140 (AAC — broadest browser
// support), then 251 (opus), then any audio-only format.
func (y *YouTube) Resolve(ctx context.Context, videoID string) (Info, error) {
	video, err := y.c.GetVideoContext(ctx, "https://www.youtube.com/watch?v="+videoID)
	if err != nil {
		return Info{}, fmt.Errorf("fetch video: %w", err)
	}
	format, err := pickAudio(video.Formats)
	if err != nil {
		return Info{}, err
	}
	u, err := y.c.GetStreamURLContext(ctx, video, &format)
	if err != nil {
		return Info{}, fmt.Errorf("stream url: %w", err)
	}
	return Info{URL: u, MimeType: mimeTypeFor(format)}, nil
}

func pickAudio(formats []youtube.Format) (youtube.Format, error) {
	for _, want := range []int{140, 251} {
		for _, f := range formats {
			if f.ItagNo == want {
				return f, nil
			}
		}
	}
	for _, f := range formats {
		if strings.HasPrefix(strings.ToLower(f.MimeType), "audio") {
			return f, nil
		}
	}
	return youtube.Format{}, ErrNoAudio
}

// mimeTypeFor extracts the container (e.g. "audio/mp4") from a
// "audio/mp4; codecs=\"...\"" mime string.
func mimeTypeFor(f youtube.Format) string {
	if base := strings.ToLower(strings.Split(f.MimeType, ";")[0]); strings.HasPrefix(base, "audio") {
		return base
	}
	switch f.ItagNo {
	case 251:
		return "audio/webm"
	default:
		return "audio/mp4"
	}
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

// NewCached wraps inner. ttl <= 0 uses 5 hours; maxEntries <= 0 uses 256.
func NewCached(inner Resolver, ttl time.Duration, maxEntries int) *Cached {
	if ttl <= 0 {
		ttl = 5 * time.Hour
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
	if e, ok := c.entries[videoID]; ok && c.now().Sub(e.fetchedAt) < c.ttl {
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
