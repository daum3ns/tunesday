package store

import (
	"database/sql"

	"tunesday/tunesday.online/internal/db"
)

// ReminderStore records which teams already got their no-tune reminder for a
// given date, so the midnight email only fires once per Tunesday per team.
type ReminderStore struct {
	db *db.DB
}

// NewReminderStore builds a ReminderStore on the shared database.
func NewReminderStore(db *db.DB) *ReminderStore {
	return &ReminderStore{db: db}
}

// Sent reports whether the reminder for teamID/date was already dispatched.
func (s *ReminderStore) Sent(teamID, date string) (bool, error) {
	var one int
	err := s.db.QueryRow(
		`SELECT 1 FROM reminder_sent WHERE team_id = ? AND for_date = ?`,
		teamID, date,
	).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// Mark records that the reminder for teamID/date was dispatched.
func (s *ReminderStore) Mark(teamID, date string) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO reminder_sent (team_id, for_date) VALUES (?, ?)`,
		teamID, date,
	)
	return err
}
