package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func postJSON(t *testing.T, u *testUser, server, path string, body any) *http.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, server+path, strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := u.http.Do(req)
	if err != nil {
		t.Fatalf("post json %s: %v", path, err)
	}
	return res
}

func quizRound(tuneID int64, guess string, correct bool) map[string]any {
	return map[string]any{"tune_id": tuneID, "guess": guess, "correct": correct}
}

func TestQuizPageAndResult(t *testing.T) {
	h, database, mailer := setupTestHandler(t)
	defer database.Close()

	fm := newFakeMailer()
	mailer.SendFunc = fm.capture()
	ts := httptest.NewServer(h.Router())
	defer ts.Close()
	server := ts.URL

	admin := registerAndVerify(t, server, fm, "quizadmin@example.com", "password123")

	payload := []byte(`{
      "participants": {"Alain": 3, "Lukas": 3},
      "tunes": [
        {"name": "Alpha Song", "link": "https://youtu.be/aaaaaaaaaaa", "id": "aaaaaaaaaaa", "provider": "Alain", "added_at": "2026-01-01T10:00:00Z"},
        {"name": "Beta Song",  "link": "https://youtu.be/bbbbbbbbbbb", "id": "bbbbbbbbbbb", "provider": "Lukas", "added_at": "2026-01-02T10:00:00Z"},
        {"name": "Gamma Song", "link": "https://youtu.be/ccccccccccc", "id": "ccccccccccc", "provider": "Alain", "added_at": "2026-01-03T10:00:00Z"},
        {"name": "Delta Song", "link": "https://youtu.be/ddddddddddd", "id": "ddddddddddd", "provider": "Lukas", "added_at": "2026-01-04T10:00:00Z"},
        {"name": "Epsilon Song", "link": "https://youtu.be/eeeeeeeeeee", "id": "eeeeeeeeeee", "provider": "Alain", "added_at": "2026-01-05T10:00:00Z"}
      ]
    }`)
	res := createTeamWithFile(t, admin, server, map[string]string{
		"team_name": "Quiz Squad", "your_name": "Alain",
	}, payload)
	res.Body.Close()

	// Page renders with injected data.
	page := readBody(t, admin.get(server, "/teams/quiz-squad/quiz"))
	if !strings.Contains(page, "Guess the Provider") || !strings.Contains(page, `id="quiz-data"`) {
		t.Fatal("quiz page missing heading or data blob")
	}
	if !strings.Contains(page, "Epsilon Song") {
		t.Fatal("quiz data not injected")
	}

	// Stranger is refused.
	stranger := registerAndVerify(t, server, fm, "noshow@example.com", "password123")
	res = postJSON(t, stranger, server, "/teams/quiz-squad/quiz/result", map[string]any{
		"mode": "quick", "started_at": "", "score": 1, "total": 1,
		"rounds": []map[string]any{quizRound(0, "Alain", true)},
	})
	body := readBody(t, res)
	if !strings.Contains(body, "not a member") {
		t.Fatalf("stranger must be refused, got %s", body)
	}

	// Invalid mode rejected.
	res = postJSON(t, admin, server, "/teams/quiz-squad/quiz/result", map[string]any{
		"mode": "infinite", "started_at": "", "score": 1, "total": 1,
		"rounds": []map[string]any{quizRound(0, "Alain", true)},
	})
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad mode should 400, got %d", res.StatusCode)
	}

	// Valid submission; server recomputes the score from the rounds.
	quizSubmitThrottle.reset()
	res = postJSON(t, admin, server, "/teams/quiz-squad/quiz/result", map[string]any{
		"mode": "quick", "started_at": "", "score": 999, "total": 3,
		"rounds": []map[string]any{
			quizRound(1, "Alain", true),
			quizRound(2, "Bogus", false),
			quizRound(99999, "Lukas", true), // foreign tune_id sanitized to NULL
		},
	})
	if res.StatusCode != http.StatusOK {
		body, _ := readAll(res)
		t.Fatalf("submit failed: %d %s", res.StatusCode, body)
	}
	var out struct {
		OK    bool `json:"ok"`
		Score int  `json:"score"`
		Total int  `json:"total"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Score != 2 || out.Total != 3 {
		t.Fatalf("server must recompute 2/3, got %d/%d", out.Score, out.Total)
	}

	// Rounds persisted: both real tune ids kept, the foreign id became NULL.
	var nulls, kept int
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM quiz_rounds WHERE game_id = (SELECT id FROM quiz_games ORDER BY rowid DESC LIMIT 1) AND tune_id IS NULL`,
	).Scan(&nulls); err != nil {
		t.Fatal(err)
	}
	if nulls != 1 {
		t.Fatalf("expected exactly the foreign tune id as NULL, got %d nulls", nulls)
	}
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM quiz_rounds WHERE game_id = (SELECT id FROM quiz_games ORDER BY rowid DESC LIMIT 1) AND tune_id IS NOT NULL`,
	).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if kept != 2 {
		t.Fatalf("expected 2 valid tune links, got %d", kept)
	}

	// Leaderboard now shows the game (no reset needed: GETs don't touch the throttle).
	page = readBody(t, admin.get(server, "/teams/quiz-squad/quiz"))
	if !strings.Contains(page, "quizadmin@example.com") || !strings.Contains(page, "2/3") {
		t.Fatalf("leaderboard missing the new entry:\n%s", page)
	}

	// Rate limit trips on immediate resubmission.
	res = postJSON(t, admin, server, "/teams/quiz-squad/quiz/result", map[string]any{
		"mode": "quick", "started_at": "", "score": 1, "total": 1,
		"rounds": []map[string]any{quizRound(0, "Alain", true)},
	})
	res.Body.Close()
	if res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429 on rapid resubmit, got %d", res.StatusCode)
	}
}

func readAll(res *http.Response) (string, error) {
	defer res.Body.Close()
	var sb strings.Builder
	buf := make([]byte, 32*1024)
	for {
		n, err := res.Body.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			return sb.String(), nil
		}
	}
}
