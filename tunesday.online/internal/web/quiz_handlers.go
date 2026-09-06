package web

import (
	"encoding/json"
	"html/template"
	"net/http"
	"sort"
	"strings"
	"time"

	"tunesday/tunesday.online/internal/auth"
	"tunesday/tunesday.online/internal/store"
)

const (
	quizMaxRounds   = 50
	quizProviderMax = 64
)

// quizTune is one playable entry injected into the quiz engine.
type quizTune struct {
	ID       int64  `json:"id"`
	YouTube  string `json:"yt"`
	Name     string `json:"name"`
	Link     string `json:"link"`
	Provider string `json:"provider"`
}

// QuizPage renders the team quiz with server-injected data.
func (h *Handler) QuizPage(w http.ResponseWriter, r *http.Request) {
	team, member, ok := h.requireMember(w, r)
	if !ok {
		return
	}

	tunes, err := h.deps.Tunes.ListAllByTeam(team.ID)
	if err != nil {
		redirectFlash(w, r, "/teams/"+team.Slug+"/dashboard", "err", "Could not load the quiz data.")
		return
	}

	var quizTunes []quizTune
	providerSet := map[string]bool{}
	for _, t := range tunes {
		if len(t.YouTubeID) < 10 || t.ProviderName == "" {
			continue
		}
		quizTunes = append(quizTunes, quizTune{
			ID: t.ID, YouTube: t.YouTubeID, Name: t.Title, Link: t.Link, Provider: t.ProviderName,
		})
		providerSet[t.ProviderName] = true
	}
	providers := make([]string, 0, len(providerSet))
	for p := range providerSet {
		providers = append(providers, p)
	}
	sort.Strings(providers)

	payload, err := json.Marshal(map[string]any{
		"tunes":     quizTunes,
		"providers": providers,
	})
	if err != nil {
		redirectFlash(w, r, "/teams/"+team.Slug+"/dashboard", "err", "Could not prepare the quiz.")
		return
	}

	recent, _ := h.deps.Quiz.Recent(team.ID, 10)
	bests, _ := h.deps.Quiz.Bests(team.ID)

	data := teamPage(team, member, "quiz")
	data["Title"] = "Guess the Provider"
	data["QuizData"] = template.JS(payload)
	data["Playable"] = len(quizTunes) >= 5 && len(providers) >= 2
	data["TuneCount"] = len(quizTunes)
	data["ProviderCount"] = len(providers)
	data["RecentGames"] = recent
	data["TeamBests"] = bests
	h.render(w, r, "quiz.html", data)
}

// quizRoundPayload is one client-submitted round.
type quizRoundPayload struct {
	TuneID  int64  `json:"tune_id"`
	Guess   string `json:"guess"`
	Correct bool   `json:"correct"`
}

// quizResultPayload is the finished-game report.
type quizResultPayload struct {
	Mode      string             `json:"mode"`
	StartedAt string             `json:"started_at"`
	Score     int                `json:"score"` // informational; server recomputes
	Total     int                `json:"total"` // informational; server recomputes
	Rounds    []quizRoundPayload `json:"rounds"`
}

var quizModes = map[string]bool{"quick": true, "universe": true, "all": true}

// QuizResult stores a finished game. The authoritative score is recomputed
// from the rounds; the client's claimed numbers are only sanity-checked.
func (h *Handler) QuizResult(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	team, _, ok := h.requireMember(w, r)
	if !ok {
		return
	}

	if !quizSubmitThrottle.allow(user.ID) {
		http.Error(w, "slow down, DJ", http.StatusTooManyRequests)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var in quizResultPayload
	if err := dec.Decode(&in); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	if !quizModes[in.Mode] {
		http.Error(w, "unknown mode", http.StatusBadRequest)
		return
	}
	if len(in.Rounds) == 0 || len(in.Rounds) > quizMaxRounds {
		http.Error(w, "rounds out of range", http.StatusBadRequest)
		return
	}
	started := parseQuizStart(in.StartedAt)

	tunes, err := h.deps.Tunes.ListAllByTeam(team.ID)
	if err != nil {
		http.Error(w, "team data unavailable", http.StatusInternalServerError)
		return
	}
	owned := map[int64]bool{}
	for _, t := range tunes {
		owned[t.ID] = true
	}

	rounds := make([]store.QuizRound, 0, len(in.Rounds))
	for _, rd := range in.Rounds {
		guess := strings.TrimSpace(rd.Guess)
		if len(guess) > quizProviderMax {
			http.Error(w, "guess too long", http.StatusBadRequest)
			return
		}
		var tuneID int64
		if rd.TuneID != 0 && owned[rd.TuneID] {
			tuneID = rd.TuneID
		}
		rounds = append(rounds, store.QuizRound{
			TuneID: tuneID, GuessedProvider: guess, WasCorrect: rd.Correct,
		})
	}

	game, err := h.deps.Quiz.SubmitGame(&store.QuizSubmission{
		TeamID:    team.ID,
		UserID:    user.ID,
		Mode:      in.Mode,
		StartedAt: started,
		Rounds:    rounds,
	})
	if err != nil {
		http.Error(w, "could not store the game", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok": true, "id": game.ID, "score": game.Score, "total": game.Total,
	})
}

// parseQuizStart accepts an RFC3339 timestamp that is plausible (not future,
// not older than a day); anything else falls back to zero (defaults to now).
func parseQuizStart(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	now := time.Now()
	if t.After(now.Add(time.Minute)) || now.Sub(t) > 24*time.Hour {
		return time.Time{}
	}
	return t.UTC()
}

// quizSubmitThrottle rate-limits score submissions per user (the throttle
// mechanism itself is shared with the login-link page).
var quizSubmitThrottle = newIPThrottle(5 * time.Second)
