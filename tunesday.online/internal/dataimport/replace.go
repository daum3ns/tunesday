package dataimport

import (
	"database/sql"
	"fmt"
	"sort"

	"tunesday/internal/core"
	"tunesday/internal/playlist"
	"tunesday/tunesday.online/internal/db"
	"tunesday/tunesday.online/internal/store"
)

// insertTuneTx normalizes and inserts one tune inside a transaction.
func insertTuneTx(tx *sql.Tx, teamID string, providerID int64, t core.Tune, yt *playlist.YouTube) error {
	ytID := t.ID
	link := t.Link
	if ytID == "" && link != "" {
		clean := playlist.StripTrackingParams(link)
		if id, ok := yt.NormalizeYouTubeID(clean); ok {
			ytID = id
		}
	}
	title := t.Name
	if title == "" {
		title = link
	}
	_, err := tx.Exec(
		`INSERT INTO tunes (team_id, title, link, youtube_id, provider_id, added_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		teamID, title, link, ytID, providerID, store.FormatTime(t.AddedAt),
	)
	return err
}

// ProviderNames returns the sorted union of all provider names a file
// mentions: participants, disabled members, and tune providers.
func ProviderNames(data *core.Data) []string {
	set := map[string]struct{}{}
	for name := range data.Participants {
		set[name] = struct{}{}
	}
	for name := range data.Disabled {
		set[name] = struct{}{}
	}
	for _, t := range data.Tunes {
		set[t.Provider] = struct{}{}
	}
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ReplaceResult reports what a replace swap changed.
type ReplaceResult struct {
	TunesDeleted     int
	TunesImported    int
	ProvidersCreated int
	ProvidersSynced  int
	ProvidersRemoved []string
	ProvidersKept    []string
	InvitesRevoked   int
}

// ReplaceTeam swaps a team's data with the contents of a tunesday.json.
// Semantics: the file is authoritative for tunes and the provider list,
// but team structure survives — memberships, magic tokens, and ceremony
// history are never touched. Old providers that are missing from the file
// are deleted unless a member still uses them (then they are kept and
// reported). Ceremony records pointing at a deleted provider keep their
// seed/pool audit trail; the winner id is cleared.
func ReplaceTeam(database *db.DB, teamID string, data *core.Data) (*ReplaceResult, error) {
	if data == nil {
		return nil, fmt.Errorf("no data to import")
	}

	res := &ReplaceResult{ProvidersRemoved: []string{}, ProvidersKept: []string{}}
	tx, err := database.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	del, err := tx.Exec(`DELETE FROM tunes WHERE team_id = ?`, teamID)
	if err != nil {
		return nil, fmt.Errorf("delete tunes: %w", err)
	}
	if n, err := del.RowsAffected(); err == nil {
		res.TunesDeleted = int(n)
	}

	type existingProvider struct {
		id       int64
		disabled bool
	}
	existing := map[string]existingProvider{}
	rows, err := tx.Query(`SELECT id, name, disabled FROM providers WHERE team_id = ?`, teamID)
	if err != nil {
		return nil, fmt.Errorf("list providers: %w", err)
	}
	for rows.Next() {
		var (
			name     string
			id       int64
			disabled int
		)
		if err := rows.Scan(&id, &name, &disabled); err != nil {
			rows.Close()
			return nil, err
		}
		existing[name] = existingProvider{id: id, disabled: disabled == 1}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	desired := map[string]bool{}
	for _, name := range ProviderNames(data) {
		desired[name] = true
	}

	yt := playlist.NewYouTube()
	nameToID := map[string]int64{}
	for _, name := range ProviderNames(data) {
		wantDisabled := data.Disabled[name]
		if p, ok := existing[name]; ok {
			if p.disabled != wantDisabled {
				flag := 0
				if wantDisabled {
					flag = 1
				}
				if _, err := tx.Exec(`UPDATE providers SET disabled = ? WHERE id = ?`, flag, p.id); err != nil {
					return nil, fmt.Errorf("update provider %q: %w", name, err)
				}
			}
			nameToID[name] = p.id
			res.ProvidersSynced++
			continue
		}
		flag := 0
		if wantDisabled {
			flag = 1
		}
		r, err := tx.Exec(
			`INSERT INTO providers (team_id, name, disabled) VALUES (?, ?, ?)`,
			teamID, name, flag,
		)
		if err != nil {
			return nil, fmt.Errorf("insert provider %q: %w", name, err)
		}
		id, err := r.LastInsertId()
		if err != nil {
			return nil, err
		}
		nameToID[name] = id
		res.ProvidersCreated++
	}

	for _, t := range data.Tunes {
		pid, ok := nameToID[t.Provider]
		if !ok {
			return nil, fmt.Errorf("tune %q has unknown provider %q", t.Name, t.Provider)
		}
		if err := insertTuneTx(tx, teamID, pid, t, yt); err != nil {
			return nil, fmt.Errorf("insert tune: %w", err)
		}
		res.TunesImported++
	}

	// Prune providers the file no longer mentions, unless a member needs them.
	for _, name := range sortedKeys(existing) {
		if desired[name] {
			continue
		}
		p := existing[name]
		var members int
		if err := tx.QueryRow(
			`SELECT COUNT(*) FROM team_members WHERE provider_id = ?`, p.id,
		).Scan(&members); err != nil {
			return nil, err
		}
		if members > 0 {
			res.ProvidersKept = append(res.ProvidersKept, name)
			continue
		}
		// Clear references before deleting.
		if _, err := tx.Exec(
			`UPDATE ceremonies SET winner_provider_id = NULL WHERE winner_provider_id = ?`, p.id,
		); err != nil {
			return nil, fmt.Errorf("clear ceremony winner: %w", err)
		}
		inv, err := tx.Exec(`DELETE FROM invitations WHERE provider_id = ?`, p.id)
		if err != nil {
			return nil, fmt.Errorf("revoke invitations: %w", err)
		}
		if n, err := inv.RowsAffected(); err == nil {
			res.InvitesRevoked += int(n)
		}
		if _, err := tx.Exec(`DELETE FROM providers WHERE id = ?`, p.id); err != nil {
			return nil, fmt.Errorf("delete provider %q: %w", name, err)
		}
		res.ProvidersRemoved = append(res.ProvidersRemoved, name)
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

// BuildExport assembles a tunesday.json payload from SQLite reads.
// Participant counts are recalculated from the actual tunes.
func BuildExport(providers []*store.ProviderView, tunes []*store.TuneView) *core.Data {
	data := &core.Data{
		Participants: map[string]int{},
		Disabled:     map[string]bool{},
		Tunes:        []core.Tune{},
	}
	for _, p := range providers {
		data.Participants[p.Name] = 0
		if p.Disabled {
			data.Disabled[p.Name] = true
		}
	}
	for _, t := range tunes {
		count := data.Participants[t.ProviderName] + 1
		data.Participants[t.ProviderName] = count
		data.Tunes = append(data.Tunes, core.Tune{
			Name:     t.Title,
			Link:     t.Link,
			ID:       t.YouTubeID,
			Provider: t.ProviderName,
			AddedAt:  t.AddedAt,
		})
	}
	return data
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
