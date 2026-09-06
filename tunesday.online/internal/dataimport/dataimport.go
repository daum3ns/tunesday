// Package dataimport creates teams and imports tunesday.json data into SQLite.
package dataimport

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"

	"tunesday/internal/core"
	"tunesday/internal/playlist"
	"tunesday/tunesday.online/internal/db"
	"tunesday/tunesday.online/internal/store"
)

// Parse reads and validates a tunesday.json payload.
func Parse(r io.Reader) (*core.Data, error) {
	var data core.Data
	dec := json.NewDecoder(r)
	if err := dec.Decode(&data); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if data.Participants == nil {
		data.Participants = map[string]int{}
	}
	for i, t := range data.Tunes {
		if strings.TrimSpace(t.Provider) == "" {
			return nil, fmt.Errorf("tune %d: missing provider", i+1)
		}
		if strings.TrimSpace(t.Link) == "" && strings.TrimSpace(t.ID) == "" {
			return nil, fmt.Errorf("tune %d: missing link and id", i+1)
		}
	}
	return &data, nil
}

// CreateTeamInput describes everything needed to create a team atomically.
type CreateTeamInput struct {
	AdminUserID       string
	TeamName          string
	Slug              string
	AdminProviderName string
	// Data is optional parsed tunesday.json content. nil creates an empty team.
	Data *core.Data
}

// CreateTeamResult reports what was created.
type CreateTeamResult struct {
	TeamID           string
	ProvidersCreated int
	TunesInserted    int
}

// CreateTeam creates the team, its providers, imported tunes, and the admin
// membership in one transaction.
func CreateTeam(database *db.DB, in CreateTeamInput) (*CreateTeamResult, error) {
	if strings.TrimSpace(in.AdminProviderName) == "" {
		return nil, fmt.Errorf("your provider name is required")
	}

	teamID := uuid.NewString()
	tx, err := database.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(
		`INSERT INTO teams (id, name, slug, admin_id, created_at) VALUES (?, ?, ?, ?, ?)`,
		teamID, in.TeamName, in.Slug, in.AdminUserID, store.FormatTime(time.Now()),
	); err != nil {
		return nil, fmt.Errorf("insert team: %w", err)
	}

	res := &CreateTeamResult{TeamID: teamID}
	providerIDs := map[string]int64{}
	yt := playlist.NewYouTube()

	ensureProvider := func(name string, disabled bool) error {
		if _, ok := providerIDs[name]; ok {
			return nil
		}
		flag := 0
		if disabled {
			flag = 1
		}
		r, err := tx.Exec(
			`INSERT INTO providers (team_id, name, disabled) VALUES (?, ?, ?)`,
			teamID, name, flag,
		)
		if err != nil {
			return fmt.Errorf("insert provider %q: %w", name, err)
		}
		id, err := r.LastInsertId()
		if err != nil {
			return err
		}
		providerIDs[name] = id
		res.ProvidersCreated++
		return nil
	}

	if in.Data != nil {
		for name := range in.Data.Participants {
			if err := ensureProvider(name, in.Data.Disabled[name]); err != nil {
				return nil, err
			}
		}
		for name := range in.Data.Disabled {
			if err := ensureProvider(name, true); err != nil {
				return nil, err
			}
		}
		for _, t := range in.Data.Tunes {
			if err := ensureProvider(t.Provider, false); err != nil {
				return nil, err
			}
			if err := insertTuneTx(tx, teamID, providerIDs[t.Provider], t, yt); err != nil {
				return nil, fmt.Errorf("insert tune: %w", err)
			}
			res.TunesInserted++
		}
	}

	// Make sure the admin's own provider exists (may be new for empty teams).
	if err := ensureProvider(in.AdminProviderName, false); err != nil {
		return nil, err
	}

	var providerID int64
	if err := tx.QueryRow(
		`SELECT id FROM providers WHERE team_id = ? AND name = ?`,
		teamID, in.AdminProviderName,
	).Scan(&providerID); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("admin provider missing after creation")
		}
		return nil, err
	}

	if _, err := tx.Exec(
		`INSERT INTO team_members (team_id, user_id, provider_id, role, magic_token)
		 VALUES (?, ?, ?, 'admin', ?)`,
		teamID, in.AdminUserID, providerID, store.NewToken(),
	); err != nil {
		return nil, fmt.Errorf("insert admin membership: %w", err)
	}

	if _, err := tx.Exec(
		`UPDATE providers SET tune_count = (
			SELECT COUNT(*) FROM tunes WHERE tunes.provider_id = providers.id
		 ) WHERE team_id = ?`,
		teamID,
	); err != nil {
		return nil, fmt.Errorf("recalculate counts: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return res, nil
}
