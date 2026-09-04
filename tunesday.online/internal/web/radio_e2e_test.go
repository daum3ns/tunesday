package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"tunesday/tunesday.online/internal/radio"
)

type listenerEntry struct {
	Alias     string `json:"alias"`
	Provider  string `json:"provider"`
	TuneID    int64  `json:"tuneId"`
	TuneTitle string `json:"tuneTitle"`
	IsYou     bool   `json:"isYou"`
}

func createTeamWithFile(t *testing.T, u *testUser, server string, fields map[string]string, payload []byte) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	if payload != nil {
		fw, _ := w.CreateFormFile("tunesday_json", "tunesday.json")
		_, _ = fw.Write(payload)
	}
	_ = w.Close()
	req, err := http.NewRequest(http.MethodPost, server+"/teams", &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	res, err := u.http.Do(req)
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	return res
}

func teamTuneID(t *testing.T, h *Handler, slug, title string) int64 {
	t.Helper()
	team, err := h.deps.Teams.GetBySlug(slug)
	if err != nil || team == nil {
		t.Fatal("team not found: " + slug)
	}
	tunes, err := h.deps.Tunes.ListAllByTeam(team.ID)
	if err != nil || len(tunes) == 0 {
		t.Fatal("no tunes after import")
	}
	for _, tu := range tunes {
		if tu.Title == title {
			return tu.ID
		}
	}
	t.Fatalf("tune %q not found", title)
	return 0
}

func readListeners(t *testing.T, conn *websocket.Conn) []listenerEntry {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		_ = conn.SetReadDeadline(deadline)
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("radio ws read: %v", err)
		}
		var m struct {
			Type    string          `json:"type"`
			Payload []listenerEntry `json:"payload"`
		}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("radio ws json: %v", err)
		}
		if m.Type == "radio_listeners" {
			return m.Payload
		}
	}
}

func TestRadioRoomEndToEnd(t *testing.T) {
	h, database, mailer := setupTestHandler(t)
	defer database.Close()
	h.deps.Radio = radio.NewManager()

	fm := newFakeMailer()
	mailer.SendFunc = fm.capture()
	ts := httptest.NewServer(h.Router())
	defer ts.Close()
	server := ts.URL
	wsBase := "ws" + strings.TrimPrefix(server, "http")

	admin := registerAndVerify(t, server, fm, "dj@example.com", "password123")

	payload := []byte(`{
      "participants": {"Alain": 1, "Drummer": 1},
      "tunes": [
        {"name": "Alpha Song", "link": "https://youtu.be/aaaaaaaaaaa", "id": "aaaaaaaaaaa", "provider": "Alain", "added_at": "2026-01-01T10:00:00Z"},
        {"name": "Beta Song",  "link": "https://youtu.be/bbbbbbbbbbb", "id": "bbbbbbbbbbb", "provider": "Drummer", "added_at": "2026-01-08T10:00:00Z"}
      ]
    }`)
	res := createTeamWithFile(t, admin, server, map[string]string{
		"team_name": "Radio Team", "your_name": "Alain",
	}, payload)
	res.Body.Close()
	if res.Request.URL.Path != "/teams/radio-team/dashboard" {
		t.Fatalf("team create landed on %s", res.Request.URL.Path)
	}

	// Page renders with the playlist.
	page := admin.get(server, "/teams/radio-team/radio")
	body := readBody(t, page)
	if !strings.Contains(body, "Radio Room") || !strings.Contains(body, "Beta Song") {
		t.Fatalf("radio page missing content")
	}

	// A non-member is refused.
	stranger := registerAndVerify(t, server, fm, "stranger@example.com", "password123")
	body = readBody(t, stranger.get(server, "/teams/radio-team/radio"))
	if !strings.Contains(body, "not a member") {
		t.Fatal("stranger should not see the radio room")
	}

	// WS join: initial listener list with one entry, no tune playing.
	conn, resp, err := websocket.DefaultDialer.Dial(wsBase+"/teams/radio-team/radio/ws", admin.cookieHeader(server))
	if err != nil {
		t.Fatalf("radio ws dial: %v", err)
	}
	defer conn.Close()
	resp.Body.Close()

	listeners := readListeners(t, conn)
	if len(listeners) != 1 {
		t.Fatalf("expected 1 listener on join, got %d: %+v", len(listeners), listeners)
	}
	if listeners[0].Alias == "" || !listeners[0].IsYou {
		t.Fatalf("listener identity: %+v", listeners[0])
	}
	if listeners[0].TuneID != 0 {
		t.Fatalf("expected no tune on join, got tuneId %d", listeners[0].TuneID)
	}

	// Report now_playing via HTTP command endpoint.
	tuneID := teamTuneID(t, h, "radio-team", "Alpha Song")
	res = admin.postForm(server, "/teams/radio-team/radio/command", url.Values{"tune_id": {fmt.Sprintf("%d", tuneID)}})
	res.Body.Close()
	listeners = readListeners(t, conn)

	if len(listeners) != 1 {
		t.Fatalf("expected 1 listener after command, got %d", len(listeners))
	}
	if listeners[0].TuneID != tuneID {
		t.Fatalf("expected tuneId %d, got %d", tuneID, listeners[0].TuneID)
	}
	if listeners[0].TuneTitle != "Alpha Song" {
		t.Fatalf("expected tune title 'Alpha Song', got %q", listeners[0].TuneTitle)
	}

	// Invite a second member (Drummer) via the members endpoint.
	form := url.Values{}
	form.Set("email", "drummer@example.com")
	form.Set("provider_name", "Drummer")
	res = admin.postForm(server, "/teams/radio-team/members", form)
	res.Body.Close()
	inviteToken := fm.inviteTokenFor("drummer@example.com")
	if inviteToken == "" {
		t.Fatal("no invite captured")
	}

	drummer := newTestUser(t, "drummer@example.com")
	res = drummer.postForm(server, "/invite/"+inviteToken, url.Values{})
	res.Body.Close()

	// Drummer connects via WebSocket.
	conn2, resp2, err := websocket.DefaultDialer.Dial(wsBase+"/teams/radio-team/radio/ws", drummer.cookieHeader(server))
	if err != nil {
		t.Fatalf("drummer ws dial: %v", err)
	}
	defer conn2.Close()
	resp2.Body.Close()

	// Drummer gets initial list with both users.
	drummerListeners := readListeners(t, conn2)
	if len(drummerListeners) != 2 {
		t.Fatalf("drummer expected 2 listeners, got %d: %+v", len(drummerListeners), drummerListeners)
	}

	// Admin also gets an updated list (broadcast on drummer join).
	adminListeners := readListeners(t, conn)
	if len(adminListeners) != 2 {
		t.Fatalf("admin expected 2 listeners after drummer joined, got %d", len(adminListeners))
	}

	// Drummer reports now_playing with a different tune.
	drummer.postForm(server, "/teams/radio-team/radio/command", url.Values{"tune_id": {"2"}})

	// Both see updated list.
	adminListeners = readListeners(t, conn)
	drummerListeners = readListeners(t, conn2)

	foundDrummer := false
	for _, l := range adminListeners {
		if l.Alias != "" && l.TuneID == 2 {
			foundDrummer = true
		}
	}
	if !foundDrummer {
		t.Fatalf("admin did not see drummer's tune: %+v", adminListeners)
	}

	foundAdmin := false
	for _, l := range drummerListeners {
		if l.TuneID == tuneID {
			foundAdmin = true
		}
	}
	if !foundAdmin {
		t.Fatalf("drummer did not see admin's tune: %+v", drummerListeners)
	}

	// Drummer disconnects: admin sees one fewer listener.
	conn2.Close()
	time.Sleep(100 * time.Millisecond)
	adminListeners = readListeners(t, conn)
	if len(adminListeners) != 1 {
		t.Fatalf("expected 1 listener after drummer left, got %d", len(adminListeners))
	}
}

func TestRadioCommandRejectsUnknownTune(t *testing.T) {
	h, database, mailer := setupTestHandler(t)
	defer database.Close()
	h.deps.Radio = radio.NewManager()

	fm := newFakeMailer()
	mailer.SendFunc = fm.capture()
	ts := httptest.NewServer(h.Router())
	defer ts.Close()
	server := ts.URL

	admin := registerAndVerify(t, server, fm, "cmd@example.com", "password123")
	payload := []byte(`{
      "participants": {"A": 1},
      "tunes": [
        {"name": "Song", "link": "https://youtu.be/aaaaaaaaaaa", "id": "aaaaaaaaaaa", "provider": "A", "added_at": "2026-01-01T10:00:00Z"}
      ]
    }`)
	res := createTeamWithFile(t, admin, server, map[string]string{
		"team_name": "Cmd Team", "your_name": "A",
	}, payload)
	res.Body.Close()

	// Unknown tune_id -> 404.
	res = admin.postForm(server, "/teams/cmd-team/radio/command", url.Values{"tune_id": {"9999"}})
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown tune should 404, got %d", res.StatusCode)
	}
}

func TestRadioCommandClearsNowPlaying(t *testing.T) {
	h, database, mailer := setupTestHandler(t)
	defer database.Close()
	h.deps.Radio = radio.NewManager()

	fm := newFakeMailer()
	mailer.SendFunc = fm.capture()
	ts := httptest.NewServer(h.Router())
	defer ts.Close()
	server := ts.URL
	wsBase := "ws" + strings.TrimPrefix(server, "http")

	admin := registerAndVerify(t, server, fm, "clear@example.com", "password123")
	payload := []byte(`{
      "participants": {"A": 1},
      "tunes": [
        {"name": "Song", "link": "https://youtu.be/aaaaaaaaaaa", "id": "aaaaaaaaaaa", "provider": "A", "added_at": "2026-01-01T10:00:00Z"}
      ]
    }`)
	res := createTeamWithFile(t, admin, server, map[string]string{
		"team_name": "Clear Team", "your_name": "A",
	}, payload)
	res.Body.Close()

	conn, resp, err := websocket.DefaultDialer.Dial(wsBase+"/teams/clear-team/radio/ws", admin.cookieHeader(server))
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer conn.Close()
	resp.Body.Close()
	readListeners(t, conn) // initial join message

	// Set now_playing.
	res = admin.postForm(server, "/teams/clear-team/radio/command", url.Values{"tune_id": {"1"}})
	res.Body.Close()
	listeners := readListeners(t, conn)
	if listeners[0].TuneID != 1 {
		t.Fatalf("expected tuneId 1, got %d", listeners[0].TuneID)
	}

	// Clear now_playing with tune_id=0.
	res = admin.postForm(server, "/teams/clear-team/radio/command", url.Values{"tune_id": {"0"}})
	res.Body.Close()
	listeners = readListeners(t, conn)
	if listeners[0].TuneID != 0 {
		t.Fatalf("expected tuneId 0 after clear, got %d", listeners[0].TuneID)
	}
}
