package web

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"tunesday/tunesday.fm/internal/radio"
)

type radioState struct {
	Status string `json:"status"`
	Tune   *struct {
		ID        int64  `json:"id"`
		Title     string `json:"title"`
		YouTubeID string `json:"youtubeId"`
		Provider  string `json:"provider"`
	} `json:"tune"`
	StartedAt int64   `json:"startedAt"`
	Elapsed   float64 `json:"elapsedSec"`
	Mode      string  `json:"mode"`
	Index     int     `json:"index"`
	QueueLen  int     `json:"queueLen"`
	Listeners []struct {
		Alias string `json:"alias"`
		IsYou bool   `json:"isYou"`
	} `json:"listeners"`
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

func readRadio(t *testing.T, conn *websocket.Conn) radioState {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		_ = conn.SetReadDeadline(deadline)
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("radio ws read: %v", err)
		}
		var m struct {
			Type    string     `json:"type"`
			Payload radioState `json:"payload"`
		}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("radio ws json: %v", err)
		}
		if m.Type == "radio_state" {
			return m.Payload
		}
	}
}

func TestRadioRoomEndToEnd(t *testing.T) {
	h, database, mailer := setupTestHandler(t)
	defer database.Close()
	h.deps.Radio = radio.NewManagerWithEndedGuard(0) // no 10s sanity wait in tests

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

	// WS join: idle state with one listener.
	conn, resp, err := websocket.DefaultDialer.Dial(wsBase+"/teams/radio-team/radio/ws", admin.cookieHeader(server))
	if err != nil {
		t.Fatalf("radio ws dial: %v", err)
	}
	defer conn.Close()
	resp.Body.Close()

	st := readRadio(t, conn)
	if st.Status != "idle" || st.QueueLen != 0 || len(st.Listeners) != 1 {
		t.Fatalf("initial state: %+v", st)
	}
	if st.Listeners[0].Alias == "" || !st.Listeners[0].IsYou {
		t.Fatalf("listener identity: %+v", st.Listeners[0])
	}

	// Play -> first track live, one play stat.
	res = admin.postForm(server, "/teams/radio-team/radio/play", url.Values{})
	res.Body.Close()
	st = readRadio(t, conn)
	if st.Status != "playing" || st.Tune == nil || st.Tune.Title != "Alpha Song" {
		t.Fatalf("after play: %+v", st)
	}
	if st.StartedAt == 0 || st.QueueLen != 2 {
		t.Fatalf("broadcast must carry timing + queue size: %+v", st)
	}

	// Pause -> position frozen.
	res = admin.postForm(server, "/teams/radio-team/radio/pause", url.Values{})
	res.Body.Close()
	st = readRadio(t, conn)
	if st.Status != "paused" || st.Tune == nil || st.Tune.Title != "Alpha Song" {
		t.Fatalf("after pause: %+v", st)
	}

	// Next -> Beta Song playing.
	res = admin.postForm(server, "/teams/radio-team/radio/next", url.Values{})
	res.Body.Close()
	st = readRadio(t, conn)
	if st.Status != "playing" || st.Tune == nil || st.Tune.Title != "Beta Song" {
		t.Fatalf("after next: %+v", st)
	}
	betaID := st.Tune.ID

	// Ended with a stale id: no advance.
	res = admin.postForm(server, "/teams/radio-team/radio/ended", url.Values{"tune_id": {"999"}})
	res.Body.Close()
	st = readRadio(t, conn)
	if st.Tune == nil || st.Tune.Title != "Beta Song" {
		t.Fatalf("stale ended must not advance: %+v", st)
	}

	// Ended with the live id: wraps back to Alpha Song (loop).
	res = admin.postForm(server, "/teams/radio-team/radio/ended", url.Values{"tune_id": {strconv.FormatInt(betaID, 10)}})
	res.Body.Close()
	st = readRadio(t, conn)
	if st.Tune == nil || st.Tune.Title != "Alpha Song" {
		t.Fatalf("valid ended must loop the queue: %+v", st)
	}

	// Shuffle mode toggle.
	res = admin.postForm(server, "/teams/radio-team/radio/mode", url.Values{"mode": {"shuffled"}})
	res.Body.Close()
	st = readRadio(t, conn)
	if st.Mode != "shuffled" {
		t.Fatalf("mode not broadcast: %+v", st)
	}

	// Play stats recorded: Alpha started twice, Beta once.
	if n, _ := h.deps.PlayStats.CountByTune(1); n != 2 {
		t.Fatalf("expected 2 plays of tune 1, got %d", n)
	}
	if n, _ := h.deps.PlayStats.CountByTune(betaID); n != 1 {
		t.Fatalf("expected 1 play of tune %d, got %d", betaID, n)
	}
}
