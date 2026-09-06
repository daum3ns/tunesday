package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

// startCeremony creates a new ceremony room and returns its token.
func startCeremony(t *testing.T, server string, admin *testUser, slug string) string {
	t.Helper()
	res := admin.postForm(server, "/teams/"+slug+"/ceremonies", url.Values{})
	res.Body.Close()
	finalPath := res.Request.URL.Path
	if !strings.HasPrefix(finalPath, "/teams/"+slug+"/ceremonies/") || !strings.HasSuffix(finalPath, "/host") {
		t.Fatalf("ceremony start did not land on host page: %s", finalPath)
	}
	token := strings.TrimSuffix(strings.TrimPrefix(finalPath, "/teams/"+slug+"/ceremonies/"), "/host")
	if token == "" || strings.Contains(token, "/") {
		t.Fatalf("bad token from %s", finalPath)
	}
	return token
}

func revealWinner(t *testing.T, server string, admin *testUser, slug, token string) string {
	t.Helper()
	res := admin.postForm(server, "/teams/"+slug+"/ceremonies/"+token+"/reveal", url.Values{})
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("reveal status %d", res.StatusCode)
	}
	var out struct {
		Winner string `json:"winner"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("reveal decode: %v", err)
	}
	return out.Winner
}

func dialRoom(t *testing.T, wsBase, slug, token string, user *testUser, server string) *websocket.Conn {
	t.Helper()
	conn, resp, err := websocket.DefaultDialer.Dial(
		wsBase+"/teams/"+slug+"/ceremonies/"+token+"/ws", user.cookieHeader(server))
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	resp.Body.Close()
	readUntil(t, conn, "state") // drain initial state
	return conn
}

func TestRevealRestrictedToAttendees(t *testing.T) {
	h, database, mailer := setupTestHandler(t)
	defer database.Close()

	fm := newFakeMailer()
	mailer.SendFunc = fm.capture()
	ts := httptest.NewServer(h.Router())
	defer ts.Close()
	server := ts.URL
	wsBase := "ws" + strings.TrimPrefix(server, "http")

	// Team with three providers; the "Ghost" never connects to ceremonies.
	admin := registerAndVerify(t, server, fm, "boss@example.com", "password123")
	res := admin.postMultipart(server, "/teams", map[string]string{
		"team_name": "Room Only", "your_name": "Boss",
	})
	res.Body.Close()

	invite := func(email, provider string) *testUser {
		form := url.Values{}
		form.Set("email", email)
		form.Set("provider_name", provider)
		rr := admin.postForm(server, "/teams/room-only/members", form)
		rr.Body.Close()
		tok := fm.inviteTokenFor(email)
		if tok == "" {
			t.Fatalf("no invite for %s", email)
		}
		u := newTestUser(t, email)
		rr = u.postForm(server, "/invite/"+tok, url.Values{})
		rr.Body.Close()
		return u
	}
	drummer := invite("drummer@example.com", "Drummer")
	invite("ghost@example.com", "Ghost") // exists, eligible, never attends

	const rounds = 20
	for i := 0; i < rounds; i++ {
		token := startCeremony(t, server, admin, "room-only")
		aConn := dialRoom(t, wsBase, "room-only", token, admin, server)
		dConn := dialRoom(t, wsBase, "room-only", token, drummer, server)

		winner := revealWinner(t, server, admin, "room-only", token)
		if winner != "Boss" && winner != "Drummer" {
			t.Fatalf("round %d: absentee %q won an attendees-only ceremony", i, winner)
		}
		aConn.Close()
		dConn.Close()
	}
}

func TestRevealRequiresTwoEligibleInRoom(t *testing.T) {
	h, database, mailer := setupTestHandler(t)
	defer database.Close()

	fm := newFakeMailer()
	mailer.SendFunc = fm.capture()
	ts := httptest.NewServer(h.Router())
	defer ts.Close()
	server := ts.URL

	admin := registerAndVerify(t, server, fm, "alone@example.com", "password123")
	res := admin.postMultipart(server, "/teams", map[string]string{
		"team_name": "Lonely Room", "your_name": "Solo",
	})
	res.Body.Close()

	// Invite a second provider who accepts (so the roster has 2 eligible)
	// but never connects to the ceremony room.
	form := url.Values{}
	form.Set("email", "away@example.com")
	form.Set("provider_name", "Away")
	rr := admin.postForm(server, "/teams/lonely-room/members", form)
	rr.Body.Close()
	away := newTestUser(t, "away")
	acc := away.postForm(server, "/invite/"+fm.inviteTokenFor("away@example.com"), url.Values{})
	acc.Body.Close()

	token := startCeremony(t, server, admin, "lonely-room")

	resp := admin.postForm(server, "/teams/lonely-room/ceremonies/"+token+"/reveal", url.Values{})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 with only one in-room, got %d", resp.StatusCode)
	}
}

func TestRevealPoolIncludesLastSubmitter(t *testing.T) {
	h, database, mailer := setupTestHandler(t)
	defer database.Close()

	fm := newFakeMailer()
	mailer.SendFunc = fm.capture()
	ts := httptest.NewServer(h.Router())
	defer ts.Close()
	server := ts.URL
	wsBase := "ws" + strings.TrimPrefix(server, "http")

	admin := registerAndVerify(t, server, fm, "spin@example.com", "password123")
	res := admin.postMultipart(server, "/teams", map[string]string{
		"team_name": "Spin Cycle", "your_name": "Spinner",
	})
	res.Body.Close()

	form := url.Values{}
	form.Set("email", "waiting@example.com")
	form.Set("provider_name", "Waiting")
	rr := admin.postForm(server, "/teams/spin-cycle/members", form)
	rr.Body.Close()
	waiter := newTestUser(t, "waiting@example.com")
	accept := waiter.postForm(server, "/invite/"+fm.inviteTokenFor("waiting@example.com"), url.Values{})
	accept.Body.Close()

	token := startCeremony(t, server, admin, "spin-cycle")
	aConn := dialRoom(t, wsBase, "spin-cycle", token, admin, server)
	wConn := dialRoom(t, wsBase, "spin-cycle", token, waiter, server)

	winner := revealWinner(t, server, admin, "spin-cycle", token)

	// Winner adds their tune, making them the last submitter.
	form = url.Values{}
	form.Set("link", "https://youtu.be/zzzzzzzzzzz")
	resp := admin.postForm(server, "/teams/spin-cycle/ceremonies/"+token+"/tune", form)
	resp.Body.Close()
	aConn.Close()
	wConn.Close()

	// Next ceremony: both in the room. The last submitter stays in the pool —
	// re-selection is handled by pull-up voting, not by excluding them.
	token2 := startCeremony(t, server, admin, "spin-cycle")
	a2 := dialRoom(t, wsBase, "spin-cycle", token2, admin, server)
	defer a2.Close()
	w2 := dialRoom(t, wsBase, "spin-cycle", token2, waiter, server)
	defer w2.Close()

	// The first worker receives an attendees broadcast (pool preview) when the
	// second worker joins. Assert the last submitter is still a candidate.
	msg := readUntil(t, a2, "attendees")
	var payload struct {
		PoolPreview []string `json:"poolPreview"`
	}
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		t.Fatalf("decode attendees: %v", err)
	}
	found := false
	for _, p := range payload.PoolPreview {
		if p == winner {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("last submitter %q excluded from the pool; pool=%v", winner, payload.PoolPreview)
	}
}
