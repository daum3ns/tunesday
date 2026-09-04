package store

import (
	"testing"
	"time"

	"tunesday/tunesday.online/internal/db"
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

func TestProviderRecognition(t *testing.T) {
	database := newTestDB(t)
	teams := NewTeamStore(database)
	providers := NewProviderStore(database)
	qs := NewQuizStore(database)

	createUser(t, database, "qa", "qa@example.com")
	teams.Create(&Team{ID: "qt", Name: "Q", Slug: "q", AdminID: "qa"})
	p1, _ := providers.Create("qt", "Lukas")
	p2, _ := providers.Create("qt", "Marcel")

	t1 := insertTune(t, database, "qt", "A", "aaaaaaaaaaa", p1.ID)
	t2 := insertTune(t, database, "qt", "B", "bbbbbbbbbbb", p2.ID)

	// Submit quiz rounds with known correctness.
	rounds := []QuizRound{
		{TuneID: t1, GuessedProvider: "Lukas", WasCorrect: true},
		{TuneID: t1, GuessedProvider: "Lukas", WasCorrect: true},
		{TuneID: t1, GuessedProvider: "Marcel", WasCorrect: false},
		{TuneID: t2, GuessedProvider: "Marcel", WasCorrect: true},
		{TuneID: t2, GuessedProvider: "Marcel", WasCorrect: false},
	}
	qs.SubmitGame(&QuizSubmission{TeamID: "qt", UserID: "qa", Mode: "all", Rounds: rounds})

	recog, err := qs.ProviderRecognition("qt")
	if err != nil {
		t.Fatalf("ProviderRecognition: %v", err)
	}
	if len(recog) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(recog))
	}
	// Lukas: 2/3 correct ≈ 67%
	if recog[0].ProviderName != "Lukas" || recog[0].Correct != 2 || recog[0].Total != 3 {
		t.Fatalf("unexpected Lukas: %+v", recog[0])
	}
	// Marcel: 1/2 correct = 50%
	if recog[1].ProviderName != "Marcel" || recog[1].Correct != 1 || recog[1].Total != 2 {
		t.Fatalf("unexpected Marcel: %+v", recog[1])
	}
}

func TestTrickiestTunes(t *testing.T) {
	database := newTestDB(t)
	teams := NewTeamStore(database)
	providers := NewProviderStore(database)
	qs := NewQuizStore(database)

	createUser(t, database, "qa", "qa@example.com")
	teams.Create(&Team{ID: "qt", Name: "Q", Slug: "q", AdminID: "qa"})
	p, _ := providers.Create("qt", "Lukas")

	t1 := insertTune(t, database, "qt", "Hard Song", "aaaaaaaaaaa", p.ID)
	t2 := insertTune(t, database, "qt", "Easy Song", "bbbbbbbbbbb", p.ID)

	// Hard Song: 1 correct out of 5 (20%).
	for i := 0; i < 4; i++ {
		qs.SubmitGame(&QuizSubmission{TeamID: "qt", UserID: "qa", Mode: "all", Rounds: []QuizRound{
			{TuneID: t1, GuessedProvider: "X", WasCorrect: false},
		}})
	}
	qs.SubmitGame(&QuizSubmission{TeamID: "qt", UserID: "qa", Mode: "all", Rounds: []QuizRound{
		{TuneID: t1, GuessedProvider: "X", WasCorrect: true},
	}})

	// Easy Song: 4 correct out of 4 (100%).
	for i := 0; i < 4; i++ {
		qs.SubmitGame(&QuizSubmission{TeamID: "qt", UserID: "qa", Mode: "all", Rounds: []QuizRound{
			{TuneID: t2, GuessedProvider: "X", WasCorrect: true},
		}})
	}

	tricky, err := qs.TrickiestTunes("qt", 5)
	if err != nil {
		t.Fatalf("TrickiestTunes: %v", err)
	}
	if len(tricky) != 2 {
		t.Fatalf("expected 2 tunes, got %d", len(tricky))
	}
	if tricky[0].Title != "Hard Song" || tricky[0].Accuracy != 20 {
		t.Fatalf("expected Hard Song 20%%, got %+v", tricky[0])
	}
	if tricky[1].Title != "Easy Song" || tricky[1].Accuracy != 100 {
		t.Fatalf("expected Easy Song 100%%, got %+v", tricky[1])
	}
}
