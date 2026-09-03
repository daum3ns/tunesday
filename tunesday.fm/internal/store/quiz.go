package store

import (
	"database/sql"
	"sort"
	"time"

	"tunesday/tunesday.fm/internal/db"
)

// QuizRound is a single submitted guess.
type QuizRound struct {
	TuneID          int64
	GuessedProvider string
	WasCorrect      bool
}

// QuizSubmission is what a client sends after finishing a game. Score is
// recomputed from the rounds on the server; the client's claim is ignored.
type QuizSubmission struct {
	TeamID    string
	UserID    string
	Mode      string
	StartedAt time.Time
	Rounds    []QuizRound
}

// QuizGame is a persisted game row.
type QuizGame struct {
	ID      string
	TeamID  string
	UserID  string
	Mode    string
	Score   int
	Total   int
	Started time.Time
}

// LeaderboardRow is a rendered recent-game entry.
type LeaderboardRow struct {
	UserEmail  string
	Mode       string
	Score      int
	Total      int
	Pct        int
	FinishedAt time.Time
}

// TeamBest is a user's best quiz accuracy for a team.
type TeamBest struct {
	UserEmail string
	Score     int
	Total     int
	Pct       int
	Games     int
}

// QuizStore persists quiz results.
type QuizStore struct {
	db *db.DB
}

// NewQuizStore creates a new QuizStore.
func NewQuizStore(database *db.DB) *QuizStore {
	return &QuizStore{db: database}
}

// SubmitGame stores a game plus its rounds in one transaction and returns the
// server-computed score/total.
func (s *QuizStore) SubmitGame(sub *QuizSubmission) (*QuizGame, error) {
	score := 0
	for _, r := range sub.Rounds {
		if r.WasCorrect {
			score++
		}
	}

	game := &QuizGame{
		ID:      newID(),
		TeamID:  sub.TeamID,
		UserID:  sub.UserID,
		Mode:    sub.Mode,
		Score:   score,
		Total:   len(sub.Rounds),
		Started: sub.StartedAt,
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	started := formatTime(sub.StartedAt)
	if started == nil {
		started = formatTime(time.Now())
	}
	if _, err := tx.Exec(
		`INSERT INTO quiz_games (id, team_id, user_id, mode, score, total, started_at, finished_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'))`,
		game.ID, game.TeamID, game.UserID, game.Mode, game.Score, game.Total, started,
	); err != nil {
		return nil, err
	}

	for i, r := range sub.Rounds {
		var tuneID any
		if r.TuneID != 0 {
			tuneID = r.TuneID
		}
		correct := 0
		if r.WasCorrect {
			correct = 1
		}
		if _, err := tx.Exec(
			`INSERT INTO quiz_rounds (game_id, round_num, tune_id, guessed_provider, was_correct)
			 VALUES (?, ?, ?, ?, ?)`,
			game.ID, i+1, tuneID, r.GuessedProvider, correct,
		); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return game, nil
}

// Recent returns the newest games for a team with player email.
func (s *QuizStore) Recent(teamID string, limit int) ([]*LeaderboardRow, error) {
	rows, err := s.db.Query(
		`SELECT COALESCE(u.email, ''), g.mode, g.score, g.total, g.finished_at
		 FROM quiz_games g LEFT JOIN users u ON u.id = g.user_id
		 WHERE g.team_id = ?
		 ORDER BY g.finished_at DESC, g.rowid DESC LIMIT ?`,
		teamID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*LeaderboardRow
	for rows.Next() {
		var r LeaderboardRow
		var finished sql.NullString
		if err := rows.Scan(&r.UserEmail, &r.Mode, &r.Score, &r.Total, &finished); err != nil {
			return nil, err
		}
		if finished.Valid {
			r.FinishedAt = parseTime(finished.String)
		}
		if r.Total > 0 {
			r.Pct = r.Score * 100 / r.Total
		}
		out = append(out, &r)
	}
	return out, rows.Err()
}

// Bests returns each user's best accuracy for a team, aggregated in Go from
// up to recentBestScan games. Simpler and safer than correlated SQL, and the
// dataset is small (per-team quiz history).
func (s *QuizStore) Bests(teamID string) ([]*TeamBest, error) {
	const recentBestScan = 500
	rows, err := s.db.Query(
		`SELECT COALESCE(u.email, ''), g.score, g.total
		 FROM quiz_games g LEFT JOIN users u ON u.id = g.user_id
		 WHERE g.team_id = ? AND g.total > 0
		 ORDER BY g.finished_at DESC LIMIT ?`,
		teamID, recentBestScan,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type agg struct {
		score, total, games int
		pct                 float64
	}
	byUser := map[string]*agg{}
	var order []string
	for rows.Next() {
		var email string
		var score, total int
		if err := rows.Scan(&email, &score, &total); err != nil {
			return nil, err
		}
		a := byUser[email]
		if a == nil {
			a = &agg{}
			byUser[email] = a
			order = append(order, email)
		}
		a.games++
		if p := float64(score) / float64(total); p > a.pct || (p == a.pct && score > a.score) {
			a.pct, a.score, a.total = p, score, total
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var out []*TeamBest
	for _, email := range order {
		a := byUser[email]
		out = append(out, &TeamBest{
			UserEmail: email,
			Score:     a.score,
			Total:     a.total,
			Pct:       int(a.pct * 100),
			Games:     a.games,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Pct != out[j].Pct {
			return out[i].Pct > out[j].Pct
		}
		if out[i].Games != out[j].Games {
			return out[i].Games > out[j].Games
		}
		return out[i].UserEmail < out[j].UserEmail
	})
	return out, nil
}
