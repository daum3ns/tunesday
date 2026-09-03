package store

import (
	"testing"
	"time"

	"tunesday/tunesday.fm/internal/db"
)

func seedQuizTeam(t *testing.T, database *db.DB) (string, string) {
	t.Helper()
	users := NewUserStore(database)
	teams := NewTeamStore(database)
	if err := users.Create(&User{ID: "qa", Email: "qa@example.com", PasswordHash: "x", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := users.Create(&User{ID: "qb", Email: "qb@example.com", PasswordHash: "x", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := teams.Create(&Team{ID: "qt", Name: "Quiz Team", Slug: "quiz-team", AdminID: "qa"}); err != nil {
		t.Fatal(err)
	}
	return "qt", "qa"
}

func submit(t *testing.T, qs *QuizStore, user, mode string, correct []bool) *QuizGame {
	t.Helper()
	rounds := make([]QuizRound, 0, len(correct))
	for _, c := range correct {
		rounds = append(rounds, QuizRound{
			TuneID: 0, GuessedProvider: "X", WasCorrect: c,
		})
	}
	game, err := qs.SubmitGame(&QuizSubmission{
		TeamID: "qt", UserID: user, Mode: mode, Rounds: rounds,
	})
	if err != nil {
		t.Fatalf("SubmitGame: %v", err)
	}
	return game
}

func TestQuizSubmitRecomputesScore(t *testing.T) {
	database := newTestDB(t)
	seedQuizTeam(t, database)
	qs := NewQuizStore(database)

	game := submit(t, qs, "qa", "quick", []bool{true, false, true})
	if game.Score != 2 || game.Total != 3 {
		t.Fatalf("server must recompute score, got %d/%d", game.Score, game.Total)
	}

	var storedScore, storedTotal int
	if err := database.QueryRow(`SELECT score, total FROM quiz_games WHERE id = ?`, game.ID).
		Scan(&storedScore, &storedTotal); err != nil {
		t.Fatal(err)
	}
	if storedScore != 2 || storedTotal != 3 {
		t.Fatalf("persisted mismatch: %d/%d", storedScore, storedTotal)
	}
	var rounds int
	if err := database.QueryRow(`SELECT COUNT(*) FROM quiz_rounds WHERE game_id = ?`, game.ID).Scan(&rounds); err != nil {
		t.Fatal(err)
	}
	if rounds != 3 {
		t.Fatalf("expected 3 rounds persisted, got %d", rounds)
	}
}

func TestQuizLeaderboard(t *testing.T) {
	database := newTestDB(t)
	seedQuizTeam(t, database)
	qs := NewQuizStore(database)

	// qa: 2/10 (20%) then 9/10 (90%). qb: 3/4 (75%).
	submit(t, qs, "qa", "all", []bool{true, true, false, false, false, false, false, false, false, false})
	submit(t, qs, "qa", "quick", []bool{true, true, true, true, true, true, true, true, true, false})
	submit(t, qs, "qb", "universe", []bool{true, true, true, false})

	bests, err := qs.Bests("qt")
	if err != nil {
		t.Fatalf("Bests: %v", err)
	}
	if len(bests) != 2 {
		t.Fatalf("expected 2 users, got %d", len(bests))
	}
	if bests[0].UserEmail != "qa@example.com" || bests[0].Pct != 90 {
		t.Fatalf("qa must lead with 90%% best, got %+v", bests[0])
	}
	if bests[0].Games != 2 {
		t.Fatalf("expected qa 2 games, got %d", bests[0].Games)
	}
	if bests[1].UserEmail != "qb@example.com" || bests[1].Pct != 75 {
		t.Fatalf("qb best 75%%: %+v", bests[1])
	}

	recent, err := qs.Recent("qt", 10)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(recent) != 3 {
		t.Fatalf("expected 3 recent games, got %d", len(recent))
	}
	// Newest inserted first: qb's universe game (rowid tiebreak within the same second).
	if recent[0].Score != 3 || recent[0].Mode != "universe" {
		t.Fatalf("unexpected first recent row: %+v", recent[0])
	}
	if recent[0].Pct != 75 {
		t.Fatalf("expected pct 75, got %d", recent[0].Pct)
	}
	if recent[1].Mode != "quick" || recent[1].Score != 9 {
		t.Fatalf("unexpected second recent row: %+v", recent[1])
	}
}
