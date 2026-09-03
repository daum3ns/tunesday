package web

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// mediaServer fakes a googlevideo-style origin: fixed bytes, honest Range.
func mediaServer(t *testing.T) (string, *http.ServeMux) {
	t.Helper()
	payload := []byte("0123456789ABCDEF")
	mux := http.NewServeMux()
	mux.HandleFunc("/media", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Type", "audio/mp4")
		if rng := r.Header.Get("Range"); rng != "" {
			var s, e int
			if _, err := fmt.Sscanf(rng, "bytes=%d-%d", &s, &e); err == nil &&
				s >= 0 && e < len(payload) && e >= s {
				w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", s, e, len(payload)))
				w.WriteHeader(http.StatusPartialContent)
				_, _ = w.Write(payload[s : e+1])
				return
			}
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		_, _ = w.Write(payload)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL + "/media", mux
}

func streamTestTeam(t *testing.T, h *Handler, server string, fm *fakeMailer) *testUser {
	t.Helper()
	admin := registerAndVerify(t, server, fm, "streamer@example.com", "password123")
	payload := []byte(`{
      "participants": {"A": 1, "B": 1},
      "tunes": [
        {"name": "Playable", "link": "https://youtu.be/aaaaaaaaaaa", "id": "aaaaaaaaaaa", "provider": "A", "added_at": "2026-01-01T10:00:00Z"},
        {"name": "Other",    "link": "https://youtu.be/bbbbbbbbbbb", "id": "bbbbbbbbbbb", "provider": "B", "added_at": "2026-01-02T10:00:00Z"}
      ]
    }`)
	res := createTeamWithFile(t, admin, server, map[string]string{
		"team_name": "Stream Team", "your_name": "A",
	}, payload)
	res.Body.Close()
	return admin
}

// streamTuneID returns the id of the tune named "Playable" in the team.
func streamTuneID(t *testing.T, h *Handler) int64 {
	t.Helper()
	team, err := h.deps.Teams.GetBySlug("stream-team")
	if err != nil || team == nil {
		t.Fatal("stream team missing")
	}
	tunes, err := h.deps.Tunes.ListAllByTeam(team.ID)
	if err != nil || len(tunes) == 0 {
		t.Fatal("no tunes after import")
	}
	for _, tu := range tunes {
		if tu.Title == "Playable" {
			return tu.ID
		}
	}
	t.Fatal("Playable tune not found")
	return 0
}

func TestRadioStreamEndpoint(t *testing.T) {
	h, database, mailer := setupTestHandler(t)
	defer database.Close()

	url, _ := mediaServer(t)
	fr := &fakeStreamResolver{url: url}
	h.deps.Streams = fr

	fm := newFakeMailer()
	mailer.SendFunc = fm.capture()
	ts := httptest.NewServer(h.Router())
	defer ts.Close()
	server := ts.URL

	admin := streamTestTeam(t, h, server, fm)
	tuneID := streamTuneID(t, h)
	path := fmt.Sprintf("/teams/stream-team/radio/stream?tune_id=%d", tuneID)

	// Full fetch.
	res := admin.get(server, path)
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK || string(body) != "0123456789ABCDEF" {
		t.Fatalf("full stream: %d %q", res.StatusCode, body)
	}
	if ct := res.Header.Get("Content-Type"); ct != "audio/mp4" {
		t.Fatalf("unexpected content type %q", ct)
	}
	if res.Header.Get("Accept-Ranges") != "bytes" {
		t.Fatal("missing Accept-Ranges")
	}

	// Range passthrough -> 206.
	req, _ := http.NewRequest(http.MethodGet, server+path, nil)
	req.Header.Set("Range", "bytes=4-7")
	ranged, err := admin.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(ranged.Body)
	ranged.Body.Close()
	if ranged.StatusCode != http.StatusPartialContent || string(body) != "4567" {
		t.Fatalf("range stream: %d %q", ranged.StatusCode, body)
	}
	if cr := ranged.Header.Get("Content-Range"); cr != "bytes 4-7/16" {
		t.Fatalf("bad Content-Range %q", cr)
	}

	// Unknown tune -> 404.
	res = admin.get(server, "/teams/stream-team/radio/stream?tune_id=9999")
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown tune should 404, got %d", res.StatusCode)
	}
	res.Body.Close()

	// Missing tune_id -> 400.
	res = admin.get(server, "/teams/stream-team/radio/stream")
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing tune_id should 400, got %d", res.StatusCode)
	}
	res.Body.Close()

	// Non-member cannot stream (gets the member message page).
	stranger := registerAndVerify(t, server, fm, "streamfan@example.com", "password123")
	res = stranger.get(server, path)
	notMember := readBody(t, res)
	if len(notMember) > 32*1024 || notMember == "" {
		t.Fatal("stranger should get member-required page")
	}
}

func TestRadioStreamResolverFailure(t *testing.T) {
	h, database, mailer := setupTestHandler(t)
	defer database.Close()

	h.deps.Streams = &fakeStreamResolver{err: errors.New("bot wall")}
	fm := newFakeMailer()
	mailer.SendFunc = fm.capture()
	ts := httptest.NewServer(h.Router())
	defer ts.Close()
	server := ts.URL

	admin := streamTestTeam(t, h, server, fm)
	res := admin.get(server, fmt.Sprintf("/teams/stream-team/radio/stream?tune_id=%d", streamTuneID(t, h)))
	res.Body.Close()
	if res.StatusCode != http.StatusBadGateway {
		t.Fatalf("resolver failure should 502, got %d", res.StatusCode)
	}
}

func TestStreamGate(t *testing.T) {
	g := newConcurrencyGate(2)
	if !g.acquire("t") || !g.acquire("t") {
		t.Fatal("first two acquires must pass")
	}
	if g.acquire("t") {
		t.Fatal("third acquire must fail at limit 2")
	}
	g.release("t")
	if !g.acquire("t") {
		t.Fatal("acquire after release must pass")
	}
	// Other teams have their own budget.
	if !g.acquire("other") || !g.acquire("other") {
		t.Fatal("per-team isolation broken")
	}
	if g.acquire("other") {
		t.Fatal("second team must honour the same limit")
	}
}

func TestStreamGateConcurrent(t *testing.T) {
	g := newConcurrencyGate(3)
	var mu sync.Mutex
	running, max := 0, 0
	done := make(chan struct{})
	for i := 0; i < 12; i++ {
		go func() {
			if g.acquire("x") {
				mu.Lock()
				running++
				if running > max {
					max = running
				}
				mu.Unlock()
				g.release("x")
				mu.Lock()
				running--
				mu.Unlock()
			}
			done <- struct{}{}
		}()
	}
	for i := 0; i < 12; i++ {
		<-done
	}
	if max > 3 {
		t.Fatalf("gate leaked: max concurrent %d", max)
	}
}
