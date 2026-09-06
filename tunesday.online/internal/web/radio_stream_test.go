package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

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

	h.deps.Streams = &fakeStreamResolver{url: "https://cdn.google.com/audio/123"}

	fm := newFakeMailer()
	mailer.SendFunc = fm.capture()
	ts := httptest.NewServer(h.Router())
	defer ts.Close()
	server := ts.URL

	admin := streamTestTeam(t, h, server, fm)
	tuneID := streamTuneID(t, h)
	path := fmt.Sprintf("/teams/stream-team/radio/stream?tune_id=%d", tuneID)

	// Full fetch -> JSON with url, mimeType, expires.
	res := admin.get(server, path)
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("stream: status %d, body %q", res.StatusCode, body)
	}
	if ct := res.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected application/json, got %q", ct)
	}
	var info struct {
		URL      string `json:"url"`
		MimeType string `json:"mimeType"`
		Expires  int64  `json:"expires"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if info.URL != "https://cdn.google.com/audio/123" {
		t.Fatalf("unexpected url: %q", info.URL)
	}
	if info.MimeType != "audio/mp4" {
		t.Fatalf("unexpected mimeType: %q", info.MimeType)
	}

	// Unknown tune -> 404.
	res = admin.get(server, "/teams/stream-team/radio/stream?tune_id=9999")
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown tune should 404, got %d", res.StatusCode)
	}

	// Missing tune_id -> 400.
	res = admin.get(server, "/teams/stream-team/radio/stream")
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing tune_id should 400, got %d", res.StatusCode)
	}

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
