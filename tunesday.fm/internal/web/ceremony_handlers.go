package web

import (
	"context"
	"encoding/json"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"

	"tunesday/internal/core"
	"tunesday/internal/playlist"
	"tunesday/tunesday.fm/internal/auth"
	"tunesday/tunesday.fm/internal/ceremony"
	"tunesday/tunesday.fm/internal/store"
)

const ceremonyRevealDurationMs = 2500

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// attendeeInfo is one record on the ceremony dance floor.
type attendeeInfo struct {
	Alias        string `json:"alias"`
	ProviderName string `json:"provider"`
	IsYou        bool   `json:"isYou,omitempty"`
}

// ceremonyState is the full room snapshot sent on join.
type ceremonyState struct {
	Status     string         `json:"status"` // open | revealed | completed
	Attendees  []attendeeInfo `json:"attendees"`
	Winner     string         `json:"winner,omitempty"`
	TuneTitle  string         `json:"tuneTitle,omitempty"`
	CanReveal  bool           `json:"canReveal"`
	YouWin     bool           `json:"youWin"`
	CanAddTune bool           `json:"canAddTune"`
}

// StartCeremony creates a ceremony room, recording seed and pool for audit.
func (h *Handler) StartCeremony(w http.ResponseWriter, r *http.Request) {
	team, member, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	back := "/teams/" + team.Slug + "/dashboard"

	eligible, err := h.deps.Providers.ListEligibleByTeam(team.ID)
	if err != nil || len(eligible) < 2 {
		redirectFlash(w, r, back, "err", "A ceremony needs at least 2 eligible (assigned + active) providers.")
		return
	}

	names := make([]string, 0, len(eligible))
	counts := make(map[string]int, len(eligible))
	for _, p := range eligible {
		names = append(names, p.Name)
		counts[p.Name] = p.TuneCount
	}
	lastProvider, _ := h.deps.Tunes.LastSubmitterProvider(team.ID)
	pool := core.ComputeProviderPool(names, counts, lastProvider)
	if len(pool) == 0 {
		redirectFlash(w, r, back, "err", "No providers left in the pool.")
		return
	}

	seed := rand.Int63()
	cer := &store.Ceremony{
		TeamID:    team.ID,
		StartedBy: member.UserID,
		Seed:      seed,
		Pool:      pool,
	}
	if err := h.deps.Ceremonies.Create(cer); err != nil {
		redirectFlash(w, r, back, "err", "Could not start the ceremony.")
		return
	}

	http.Redirect(w, r, "/teams/"+team.Slug+"/ceremonies/"+cer.Token+"/host", http.StatusSeeOther)
}

// loadCeremony resolves team (membership) and ceremony; returns ok=false if handled.
func (h *Handler) loadCeremony(w http.ResponseWriter, r *http.Request) (*store.Team, *store.TeamMember, *store.Ceremony, bool) {
	team, member, ok := h.requireMember(w, r)
	if !ok {
		return nil, nil, nil, false
	}
	token := chi.URLParam(r, "token")
	cer, err := h.deps.Ceremonies.GetByToken(token)
	if err != nil || cer == nil || cer.TeamID != team.ID {
		h.render(w, r, "message.html", map[string]any{
			"Title": "Ceremony", "Message": "This ceremony room does not exist.",
		})
		return nil, nil, nil, false
	}
	return team, member, cer, true
}

// CeremonyPage renders the join view for a ceremony room.
func (h *Handler) CeremonyPage(w http.ResponseWriter, r *http.Request) {
	team, member, cer, ok := h.loadCeremony(w, r)
	if !ok {
		return
	}
	h.renderCeremonyPage(w, r, team, member, cer, false)
}

// CeremonyHostPage renders the host view (with the big button).
func (h *Handler) CeremonyHostPage(w http.ResponseWriter, r *http.Request) {
	team, member, cer, ok := h.loadCeremony(w, r)
	if !ok {
		return
	}
	h.renderCeremonyPage(w, r, team, member, cer, true)
}

func (h *Handler) renderCeremonyPage(w http.ResponseWriter, r *http.Request, team *store.Team, member *store.TeamMember, cer *store.Ceremony, isHost bool) {
	data := teamPage(team, member, "ceremony")
	data["Title"] = "The Tunesday Roulette"
	data["Ceremony"] = cer
	data["CeremonyPath"] = "/teams/" + team.Slug + "/ceremonies/" + cer.Token
	data["IsHost"] = isHost
	h.render(w, r, "ceremony.html", data)
}

// CeremonyWS is the live wire for a ceremony room.
func (h *Handler) CeremonyWS(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	_, _, cer, ok := h.loadCeremony(w, r)
	if !ok {
		return
	}

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	room := h.deps.Rooms.RoomFor(cer.Token)

	// ensureAlias persists the attendee record (side effect) and reuses
	// an existing alias when someone reconnects.
	h.ensureAlias(cer, user.ID)
	room.Join(conn)
	defer func() {
		room.Leave(conn)
		h.deps.Rooms.Forget(cer.Token)
	}()

	h.broadcastAttendees(cer)
	h.sendState(conn, cer, user.ID)

	conn.SetReadLimit(512)
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Minute))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(10 * time.Minute))
	})
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

// ensureAlias returns the user's ceremony alias, assigning a fresh one if new.
func (h *Handler) ensureAlias(cer *store.Ceremony, userID string) string {
	attendees, _ := h.deps.Ceremonies.ListAttendees(cer.ID)
	used := map[string]struct{}{}
	for _, a := range attendees {
		used[a.Alias] = struct{}{}
		if a.UserID == userID {
			return a.Alias
		}
	}
	alias := ceremony.NewAlias(used)
	if err := h.deps.Ceremonies.AddAttendee(cer.ID, userID, alias); err != nil {
		alias = "Mystery Listener"
	}
	return alias
}

// ceremonyState assembles the full snapshot for one viewer.
func (h *Handler) ceremonyState(cer *store.Ceremony, viewerID string) ceremonyState {
	st := ceremonyState{
		Status:    "open",
		CanReveal: !cer.Revealed() && h.isCeremonyHost(cer, viewerID),
	}

	attendees, _ := h.deps.Ceremonies.ListAttendees(cer.ID)
	members, _ := h.deps.Members.ListByTeam(cer.TeamID)
	providerByUser := map[string]string{}
	userIDByProvider := map[string]string{}
	for _, m := range members {
		providerByUser[m.UserID] = m.ProviderName
		userIDByProvider[m.ProviderName] = m.UserID
	}
	for _, a := range attendees {
		st.Attendees = append(st.Attendees, attendeeInfo{
			Alias:        a.Alias,
			ProviderName: providerByUser[a.UserID],
			IsYou:        a.UserID == viewerID,
		})
	}

	if !cer.Revealed() {
		return st
	}
	st.Status = "revealed"
	winner, _ := h.deps.Providers.GetByID(cer.WinnerProviderID)
	if winner != nil {
		st.Winner = winner.Name
		st.YouWin = userIDByProvider[winner.Name] == viewerID
	}
	if cer.Completed() {
		st.Status = "completed"
		if winner != nil {
			if tunes, err := h.deps.Tunes.ListRecentByTeam(cer.TeamID, 25); err == nil {
				for _, t := range tunes {
					if t.ProviderID == winner.ID {
						st.TuneTitle = t.Title
						break
					}
				}
			}
		}
	}
	st.CanAddTune = !cer.Completed() && (st.YouWin || h.isTeamAdmin(cer.TeamID, viewerID))
	return st
}

// isCeremonyHost reports whether the user started the ceremony or is a team admin.
func (h *Handler) isCeremonyHost(cer *store.Ceremony, userID string) bool {
	return cer.StartedBy == userID || h.isTeamAdmin(cer.TeamID, userID)
}

func (h *Handler) isTeamAdmin(teamID, userID string) bool {
	member, _ := h.deps.Members.Get(teamID, userID)
	return member != nil && member.Role == "admin"
}

// broadcastAttendees pushes the current attendee list to the whole room.
func (h *Handler) broadcastAttendees(cer *store.Ceremony) {
	room := h.deps.Rooms.RoomFor(cer.Token)
	attendees, _ := h.deps.Ceremonies.ListAttendees(cer.ID)
	members, _ := h.deps.Members.ListByTeam(cer.TeamID)
	providerByUser := map[string]string{}
	for _, m := range members {
		providerByUser[m.UserID] = m.ProviderName
	}
	list := make([]attendeeInfo, 0, len(attendees))
	for _, a := range attendees {
		list = append(list, attendeeInfo{Alias: a.Alias, ProviderName: providerByUser[a.UserID]})
	}
	room.Broadcast("attendees", map[string]any{"attendees": list})
}

// sendState delivers the full snapshot to one freshly joined client.
func (h *Handler) sendState(conn *websocket.Conn, cer *store.Ceremony, viewerID string) {
	st := h.ceremonyState(cer, viewerID)
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_ = conn.WriteJSON(ceremony.Message{Type: "state", Payload: st})
}

// CeremonyReveal draws the winner with the recorded seed and tells the room.
func (h *Handler) CeremonyReveal(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	team, _, cer, ok := h.loadCeremony(w, r)
	if !ok {
		return
	}
	if !h.isCeremonyHost(cer, user.ID) {
		http.Error(w, "only the ceremony host may drop the needle", http.StatusForbidden)
		return
	}
	if cer.Revealed() {
		http.Error(w, "the needle has already dropped", http.StatusConflict)
		return
	}
	if len(cer.Pool) == 0 {
		http.Error(w, "ceremony has no pool", http.StatusInternalServerError)
		return
	}

	rng := rand.New(rand.NewSource(cer.Seed))
	winnerName := cer.Pool[rng.Intn(len(cer.Pool))]

	provider, err := h.deps.Providers.GetByName(team.ID, winnerName)
	if err != nil || provider == nil {
		http.Error(w, "winner provider missing", http.StatusInternalServerError)
		return
	}

	if err := h.deps.Ceremonies.RecordReveal(cer.ID, provider.ID); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	room := h.deps.Rooms.RoomFor(cer.Token)
	room.Broadcast("reveal", map[string]any{
		"pool":        cer.Pool,
		"winner":      provider.Name,
		"seed":        cer.Seed,
		"duration_ms": ceremonyRevealDurationMs,
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "winner": provider.Name})
}

// CeremonyAddTune lets the winner (or an admin) register today's tune.
func (h *Handler) CeremonyAddTune(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	team, _, cer, ok := h.loadCeremony(w, r)
	if !ok {
		return
	}
	back := "/teams/" + team.Slug + "/ceremonies/" + cer.Token
	if !cer.Revealed() || cer.Completed() {
		redirectFlash(w, r, back, "err", "The tune can only be added after the winner is revealed.")
		return
	}

	winner, _ := h.deps.Providers.GetByID(cer.WinnerProviderID)
	if winner == nil {
		redirectFlash(w, r, back, "err", "Winner provider is missing.")
		return
	}
	member, _ := h.deps.Members.Get(team.ID, user.ID)
	isWinner := member != nil && member.ProviderID == winner.ID
	isAdmin := member != nil && member.Role == "admin"
	if !isWinner && !isAdmin {
		redirectFlash(w, r, back, "err", "Only the winner or an admin can add today's tune.")
		return
	}

	if err := r.ParseForm(); err != nil {
		redirectFlash(w, r, back, "err", "Invalid form")
		return
	}
	link := strings.TrimSpace(r.FormValue("link"))
	if link == "" {
		redirectFlash(w, r, back, "err", "Paste a YouTube link please.")
		return
	}
	link = playlist.StripTrackingParams(link)
	ytID, ok := h.deps.YT.NormalizeYouTubeID(link)
	if !ok {
		redirectFlash(w, r, back, "err", "Only https:// YouTube links are supported.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	title, err := h.deps.YT.FetchTitle(ctx, ytID)
	if err != nil {
		redirectFlash(w, r, back, "err", "Could not fetch the title: "+err.Error())
		return
	}

	if _, err := h.deps.DB.Exec(
		`INSERT INTO tunes (team_id, title, link, youtube_id, provider_id, added_at) VALUES (?, ?, ?, ?, ?, ?)`,
		team.ID, title, link, ytID, winner.ID, store.FormatTime(time.Now()),
	); err != nil {
		redirectFlash(w, r, back, "err", "Could not save the tune.")
		return
	}
	if err := h.deps.Providers.RecalculateCounts(team.ID); err != nil {
		redirectFlash(w, r, back, "err", "Tune saved, but counts need a refresh.")
		return
	}
	if err := h.deps.Ceremonies.MarkCompleted(cer.ID); err != nil {
		redirectFlash(w, r, back, "err", "Tune saved, but ceremony state failed to update.")
		return
	}

	room := h.deps.Rooms.RoomFor(cer.Token)
	room.Broadcast("complete", map[string]any{"title": title, "provider": winner.Name})

	redirectFlash(w, r, back, "ok", "Tune registered. Happy Tunesday!")
}
