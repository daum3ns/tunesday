package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func statsTestTeam(t *testing.T, h *Handler, server string, fm *fakeMailer) *testUser {
	t.Helper()
	admin := registerAndVerify(t, server, fm, "statsadmin@example.com", "password123")
	payload := []byte(`{
      "participants": {"A": 1, "B": 1},
      "tunes": [
        {"name": "Hot Track", "link": "https://youtu.be/aaaaaaaaaaa", "id": "aaaaaaaaaaa", "provider": "A", "added_at": "2026-01-01T10:00:00Z"},
        {"name": "Cool Track", "link": "https://youtu.be/bbbbbbbbbbb", "id": "bbbbbbbbbbb", "provider": "B", "added_at": "2026-01-02T10:00:00Z"}
      ]
    }`)
	res := createTeamWithFile(t, admin, server, map[string]string{
		"team_name": "Stats Team", "your_name": "A",
	}, payload)
	res.Body.Close()

	// Record a play stat on the first tune via the radio command endpoint.
	team, _ := h.deps.Teams.GetBySlug("stats-team")
	tunes, _ := h.deps.Tunes.ListAllByTeam(team.ID)
	form := url.Values{}
	form.Set("tune_id", strconv.FormatInt(tunes[0].ID, 10))
	res = admin.postForm(server, "/teams/stats-team/radio/command", form)
	io.Copy(io.Discard, res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("record play status: %d", res.StatusCode)
	}
	return admin
}

func TestStatsPageLoads(t *testing.T) {
	h, database, mailer := setupTestHandler(t)
	defer database.Close()
	fm := newFakeMailer()
	mailer.SendFunc = fm.capture()
	ts := httptest.NewServer(h.Router())
	defer ts.Close()
	server := ts.URL

	admin := statsTestTeam(t, h, server, fm)

	body := readBody(t, admin.get(server, "/teams/stats-team/stats"))
	for _, want := range []string{"Team Stats", "Radio", "Ceremonies", "Quiz", "Most Played", "Hot Track", "1 total plays"} {
		if !strings.Contains(body, want) {
			t.Fatalf("stats page missing %q", want)
		}
	}
}

func TestStatsPageQuizAndWinRates(t *testing.T) {
	h, database, mailer := setupTestHandler(t)
	defer database.Close()
	fm := newFakeMailer()
	mailer.SendFunc = fm.capture()
	ts := httptest.NewServer(h.Router())
	defer ts.Close()
	server := ts.URL

	admin := statsTestTeam(t, h, server, fm)
	team, _ := h.deps.Teams.GetBySlug("stats-team")
	tunes, _ := h.deps.Tunes.ListAllByTeam(team.ID)

	// Submit a quiz game.
	quizSubmitThrottle.reset()
	res := postJSON(t, admin, server, "/teams/stats-team/quiz/result", map[string]any{
		"mode": "quick", "started_at": "", "score": 1, "total": 2,
		"rounds": []map[string]any{
			quizRound(tunes[0].ID, "A", true),
			quizRound(tunes[1].ID, "B", false),
		},
	})
	io.Copy(io.Discard, res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusSeeOther {
		t.Fatalf("quiz submit status: %d", res.StatusCode)
	}

	body := readBody(t, admin.get(server, "/teams/stats-team/stats"))
	for _, want := range []string{"Leaderboard", "Recent Games", "accuracy", "correct"} {
		if !strings.Contains(body, want) {
			t.Fatalf("stats quiz section missing %q", want)
		}
	}
}

func TestStatsPageStrangerDenied(t *testing.T) {
	h, database, mailer := setupTestHandler(t)
	defer database.Close()
	fm := newFakeMailer()
	mailer.SendFunc = fm.capture()
	ts := httptest.NewServer(h.Router())
	defer ts.Close()
	server := ts.URL

	statsTestTeam(t, h, server, fm)

	stranger := registerAndVerify(t, server, fm, "statsfan@example.com", "password123")
	body := readBody(t, stranger.get(server, "/teams/stats-team/stats"))
	if !strings.Contains(body, "not a member") {
		t.Fatalf("stranger should be refused, got: %s", body)
	}
}
