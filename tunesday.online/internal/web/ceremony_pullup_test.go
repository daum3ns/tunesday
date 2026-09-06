package web

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestPullUpMajorityResetsCeremony verifies that when more than half of the
// connected attendees vote to pull up, the reveal is cleared and the room
// returns to the open (hanging needle) state.
func TestPullUpMajorityResetsCeremony(t *testing.T) {
	h, database, mailer := setupTestHandler(t)
	defer database.Close()

	fm := newFakeMailer()
	mailer.SendFunc = fm.capture()
	ts := httptest.NewServer(h.Router())
	defer ts.Close()
	server := ts.URL
	wsBase := "ws" + strings.TrimPrefix(server, "http")

	admin := registerAndVerify(t, server, fm, "dj@example.com", "password123")
	res := admin.postMultipart(server, "/teams", map[string]string{
		"team_name": "Two Tone", "your_name": "DJ",
	})
	res.Body.Close()

	form := url.Values{}
	form.Set("email", "producer@example.com")
	form.Set("provider_name", "Producer")
	rr := admin.postForm(server, "/teams/two-tone/members", form)
	rr.Body.Close()
	waiter := newTestUser(t, "producer@example.com")
	acc := waiter.postForm(server, "/invite/"+fm.inviteTokenFor("producer@example.com"), url.Values{})
	acc.Body.Close()

	token := startCeremony(t, server, admin, "two-tone")
	aConn := dialRoom(t, wsBase, "two-tone", token, admin, server)
	defer aConn.Close()
	wConn := dialRoom(t, wsBase, "two-tone", token, waiter, server)
	defer wConn.Close()

	if winner := revealWinner(t, server, admin, "two-tone", token); winner == "" {
		t.Fatal("no winner revealed")
	}
	// Drain the reveal broadcast so the reset assertion below is unambiguous.
	readUntil(t, wConn, "reveal")

	// Both attendees vote to pull up. In a room of 2 the majority threshold is
	// 2/2+1 = 2 votes, so the second vote trips the reset.
	for _, conn := range []*websocket.Conn{aConn, wConn} {
		if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"pullup"}`)); err != nil {
			t.Fatalf("send pullup: %v", err)
		}
	}

	// Each vote fans out a "pullup" tally first; keep draining until reset.
	if !waitFor(t, wConn, "reset") {
		t.Fatalf("no reset broadcast after majority pull-up vote")
	}

	cer, err := h.deps.Ceremonies.GetByToken(token)
	if err != nil {
		t.Fatalf("get ceremony: %v", err)
	}
	if cer.Revealed() {
		t.Fatalf("ceremony still revealed after pull-up reset")
	}
	if cer.Completed() {
		t.Fatalf("ceremony completed after pull-up reset")
	}
}

// TestPullUpMinorityKeepsWinner verifies a single pull-up vote does not reset.
func TestPullUpMinorityKeepsWinner(t *testing.T) {
	h, database, mailer := setupTestHandler(t)
	defer database.Close()

	fm := newFakeMailer()
	mailer.SendFunc = fm.capture()
	ts := httptest.NewServer(h.Router())
	defer ts.Close()
	server := ts.URL
	wsBase := "ws" + strings.TrimPrefix(server, "http")

	admin := registerAndVerify(t, server, fm, "dj2@example.com", "password123")
	res := admin.postMultipart(server, "/teams", map[string]string{
		"team_name": "Minority", "your_name": "DJ",
	})
	res.Body.Close()

	form := url.Values{}
	form.Set("email", "producer2@example.com")
	form.Set("provider_name", "Producer")
	rr := admin.postForm(server, "/teams/minority/members", form)
	rr.Body.Close()
	waiter := newTestUser(t, "producer2@example.com")
	acc := waiter.postForm(server, "/invite/"+fm.inviteTokenFor("producer2@example.com"), url.Values{})
	acc.Body.Close()

	token := startCeremony(t, server, admin, "minority")
	aConn := dialRoom(t, wsBase, "minority", token, admin, server)
	defer aConn.Close()
	wConn := dialRoom(t, wsBase, "minority", token, waiter, server)
	defer wConn.Close()

	if winner := revealWinner(t, server, admin, "minority", token); winner == "" {
		t.Fatal("no winner revealed")
	}
	// Drain the reveal broadcast before the pull-up vote.
	readUntil(t, wConn, "reveal")

	// One vote in a room of two is a minority: no reset.
	if err := wConn.WriteMessage(websocket.TextMessage, []byte(`{"type":"pullup"}`)); err != nil {
		t.Fatalf("send pullup: %v", err)
	}
	msg, _ := readNext(t, wConn)
	if msg.Type == "reset" {
		t.Fatalf("minority vote reset the ceremony")
	}

	cer, err := h.deps.Ceremonies.GetByToken(token)
	if err != nil {
		t.Fatalf("get ceremony: %v", err)
	}
	if !cer.Revealed() {
		t.Fatalf("ceremony cleared while votes were a minority")
	}
}

// readNext reads exactly one WS message (with a short deadline).
func readNext(t *testing.T, conn *websocket.Conn) (wsMsg, error) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, raw, err := conn.ReadMessage()
	if err != nil {
		return wsMsg{}, err
	}
	var m wsMsg
	if err := json.Unmarshal(raw, &m); err != nil {
		return wsMsg{}, err
	}
	return m, nil
}

// waitFor drains messages until the wanted type arrives or the deadline hits.
func waitFor(t *testing.T, conn *websocket.Conn, want string) bool {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(deadline)
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return false
		}
		var m wsMsg
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		if m.Type == want {
			return true
		}
	}
	return false
}
