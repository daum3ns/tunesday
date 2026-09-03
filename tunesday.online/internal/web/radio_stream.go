package web

import (
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// upstreamUA keeps googlevideo happy; it mirrors what browsers send.
const upstreamUA = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"

// streamUpstream has no overall timeout (streams are long-lived); it bounds
// only connection setup, while the request context drives cancellation.
var streamUpstream = &http.Client{
	Transport: &http.Transport{
		ResponseHeaderTimeout: 20 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		IdleConnTimeout:       90 * time.Second,
	},
}

// concurrencyGate caps parallel upstream fetches per team so a room of
// listeners costs one stream from Google, not N.
type concurrencyGate struct {
	mu     sync.Mutex
	counts map[string]int
	limit  int
}

func newConcurrencyGate(limit int) *concurrencyGate {
	return &concurrencyGate{counts: map[string]int{}, limit: limit}
}

func (g *concurrencyGate) acquire(key string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.counts[key] >= g.limit {
		return false
	}
	g.counts[key]++
	return true
}

func (g *concurrencyGate) release(key string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.counts[key] <= 1 {
		delete(g.counts, key)
		return
	}
	g.counts[key]--
}

// RadioStream proxies the audio of one team tune:
// GET /teams/{slug}/radio/stream?tune_id=N
// Only tune ids owned by the team resolve — this never becomes an open proxy.
func (h *Handler) RadioStream(w http.ResponseWriter, r *http.Request) {
	team, _, ok := h.requireMember(w, r)
	if !ok {
		return
	}

	tuneID, err := strconv.ParseInt(r.URL.Query().Get("tune_id"), 10, 64)
	if err != nil || tuneID <= 0 {
		http.Error(w, "tune_id required", http.StatusBadRequest)
		return
	}
	tune, err := h.deps.Tunes.GetByID(tuneID)
	if err != nil || tune == nil || tune.TeamID != team.ID {
		http.Error(w, "tune not found", http.StatusNotFound)
		return
	}
	if tune.YouTubeID == "" {
		http.Error(w, "tune has no video id", http.StatusUnprocessableEntity)
		return
	}

	if !h.streamGate.acquire(team.ID) {
		w.Header().Set("Retry-After", "2")
		http.Error(w, "stream capacity reached", http.StatusTooManyRequests)
		return
	}
	defer h.streamGate.release(team.ID)

	info, err := h.deps.Streams.Resolve(r.Context(), tune.YouTubeID)
	if err != nil {
		log.Printf("tunesday.online: stream resolve %s: %v", tune.YouTubeID, err)
		h.invalidateStream(tune.YouTubeID)
		http.Error(w, "stream unavailable", http.StatusBadGateway)
		return
	}

	up, err := http.NewRequestWithContext(r.Context(), http.MethodGet, info.URL, nil)
	if err != nil {
		http.Error(w, "stream unavailable", http.StatusBadGateway)
		return
	}
	up.Header.Set("User-Agent", upstreamUA)
	if rng := r.Header.Get("Range"); rng != "" {
		up.Header.Set("Range", rng)
	}

	resp, err := streamUpstream.Do(up)
	if err != nil {
		log.Printf("tunesday.online: stream fetch %s: %v", tune.YouTubeID, err)
		h.invalidateStream(tune.YouTubeID)
		http.Error(w, "stream unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		h.invalidateStream(tune.YouTubeID)
		http.Error(w, "upstream refused the stream", http.StatusBadGateway)
		return
	}

	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	} else {
		w.Header().Set("Content-Type", info.MimeType)
	}
	w.Header().Set("Accept-Ranges", "bytes")
	for _, hdr := range []string{"Content-Length", "Content-Range"} {
		if v := resp.Header.Get(hdr); v != "" {
			w.Header().Set(hdr, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	rc := http.NewResponseController(w)
	buf := make([]byte, 64*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return // client vanished
			}
			_ = rc.Flush()
		}
		if rerr != nil {
			if rerr != io.EOF {
				log.Printf("tunesday.online: stream copy %s: %v", tune.YouTubeID, rerr)
			}
			return
		}
	}
}

// invalidateStream drops a possibly-dead cached URL from the resolver cache.
func (h *Handler) invalidateStream(videoID string) {
	if inv, ok := h.deps.Streams.(interface{ Invalidate(string) }); ok {
		inv.Invalidate(videoID)
	}
}
