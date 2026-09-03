package web

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/websocket"

	"tunesday/tunesday.fm/internal/auth"
	"tunesday/tunesday.fm/internal/live"
	"tunesday/tunesday.fm/internal/radio"
	"tunesday/tunesday.fm/internal/store"
)

const radioIdleTune = int64(0)

// radioTuneInfo is the play-info sent to clients.
type radioTuneInfo struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	YouTubeID string `json:"youtubeId"`
	Provider  string `json:"provider"`
}

// radioStatePayload is the full sync frame: clients render *exactly* this.
type radioStatePayload struct {
	Status    string         `json:"status"` // idle | playing | paused
	Tune      *radioTuneInfo `json:"tune,omitempty"`
	StartedAt int64          `json:"startedAt,omitempty"` // server epoch ms
	Elapsed   float64        `json:"elapsedSec,omitempty"`
	Mode      string         `json:"mode"`
	Index     int            `json:"index"`
	QueueLen  int            `json:"queueLen"`
	Listeners []attendeeInfo `json:"listeners"`
}

// RadioPage renders the listening room.
func (h *Handler) RadioPage(w http.ResponseWriter, r *http.Request) {
	team, member, ok := h.requireMember(w, r)
	if !ok {
		return
	}
	tunes, err := h.deps.Tunes.ListAllByTeam(team.ID)
	if err != nil {
		redirectFlash(w, r, "/teams/"+team.Slug+"/dashboard", "err", "Could not load the playlist.")
		return
	}
	data := teamPage(team, member, "radio")
	data["Title"] = "Radio"
	data["Tunes"] = tunes
	data["HasTunes"] = len(tunes) > 0
	h.render(w, r, "radio.html", data)
}

// radioHub gives each team's radio room a dedicated hub slot.
func (h *Handler) radioHub(teamID string) *live.Room {
	return h.deps.Rooms.RoomFor("radio:" + teamID)
}

// RadioWS is the live wire for the listening room.
func (h *Handler) RadioWS(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	team, _, ok := h.requireMember(w, r)
	if !ok {
		return
	}

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	room := h.deps.Radio.For(team.ID)
	hub := h.radioHub(team.ID)

	room.AliasFor(user.ID) // stable alias for this session
	client := hub.Join(conn, user.ID)
	defer func() {
		hub.Leave(client)
		client.CloseConnection()
		h.broadcastRadio(team.ID)
	}()

	// Personal snapshot first (carries isYou); everyone else gets the
	// room-visible join notification. Exactly one frame per client.
	h.sendRadioState(conn, team.ID, user.ID)
	hub.BroadcastExcept(client, "radio_state", h.radioPayload(team.ID, ""))

	conn.SetReadLimit(128)
	_ = conn.SetReadDeadline(time.Now().Add(30 * time.Minute))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(30 * time.Minute))
	})
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

// sendRadioState delivers the full snapshot to one freshly joined client.
func (h *Handler) sendRadioState(conn *websocket.Conn, teamID, viewerID string) {
	p := h.radioPayload(teamID, viewerID)
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_ = conn.WriteJSON(live.Message{Type: "radio_state", Payload: p})
}

// broadcastRadio pushes the current state to everyone in the room.
func (h *Handler) broadcastRadio(teamID string) {
	hub := h.radioHub(teamID)
	if hub.Count() == 0 {
		return
	}
	hub.Broadcast("radio_state", h.radioPayload(teamID, ""))
}

// radioPayload assembles the sync frame from server state.
func (h *Handler) radioPayload(teamID, viewerID string) radioStatePayload {
	room := h.deps.Radio.For(teamID)
	st := room.Snapshot()

	p := radioStatePayload{
		Status:   "idle",
		Mode:     st.Mode,
		Index:    st.Index,
		QueueLen: len(st.Queue),
	}
	if !st.Idle() {
		if tune, err := h.deps.Tunes.GetByID(st.TuneID()); err == nil && tune != nil {
			p.Status = "playing"
			if st.Paused {
				p.Status = "paused"
				p.Elapsed = st.PausedAt.Sub(st.StartedAt).Seconds()
			} else {
				p.Elapsed = time.Since(st.StartedAt).Seconds()
				p.StartedAt = st.StartedAt.UnixMilli()
			}
			p.Tune = &radioTuneInfo{
				ID:        tune.ID,
				Title:     tune.Title,
				YouTubeID: tune.YouTubeID,
				Provider:  tune.ProviderName,
			}
		}
	}

	hub := h.radioHub(teamID)
	members, _ := h.deps.Members.ListByTeam(teamID)
	providerByUser := map[string]string{}
	for _, m := range members {
		providerByUser[m.UserID] = m.ProviderName
	}
	for _, uid := range hub.Participants() {
		p.Listeners = append(p.Listeners, attendeeInfo{
			Alias:        room.AliasFor(uid),
			ProviderName: providerByUser[uid],
			IsYou:        uid == viewerID,
			Live:         true,
		})
	}
	return p
}

// radioControl is the shared flow for every POST endpoint: resolve member,
// fetch the fresh catalogue, apply the command, log starts, broadcast.
func (h *Handler) radioControl(apply func(*Handler, *radio.Room, []int64, *http.Request, string) radio.State) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.UserFromContext(r.Context())
		team, _, ok := h.requireMember(w, r)
		if !ok {
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}

		tunes, err := h.deps.Tunes.ListAllByTeam(team.ID)
		if err != nil {
			http.Error(w, "catalogue unavailable", http.StatusInternalServerError)
			return
		}
		ids := make([]int64, 0, len(tunes))
		for _, t := range tunes {
			ids = append(ids, t.ID)
		}

		room := h.deps.Radio.For(team.ID)
		before := room.Snapshot()
		after := apply(h, room, ids, r, user.ID)

		if before.TuneID() != after.TuneID() && after.TuneID() != radioIdleTune {
			_ = h.deps.PlayStats.Record(&store.PlayStat{
				TeamID:    team.ID,
				TuneID:    after.TuneID(),
				UserID:    user.ID,
				SessionID: room.SessionID(),
			})
		}

		h.broadcastRadio(team.ID)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}
}

func formTuneID(r *http.Request) int64 {
	id, _ := strconv.ParseInt(r.FormValue("tune_id"), 10, 64)
	return id
}

// Command implementations (DJ-anarchy: any member may use all of them).

func radioPlay(h *Handler, room *radio.Room, ids []int64, r *http.Request, actor string) radio.State {
	tuneID := formTuneID(r)
	if tuneID != 0 && !containsTune(ids, tuneID) {
		return room.Snapshot() // unknown/deleted tune: no-op
	}
	return room.Play(ids, tuneID)
}

func radioPause(h *Handler, room *radio.Room, ids []int64, r *http.Request, actor string) radio.State {
	return room.Pause()
}

func radioNext(h *Handler, room *radio.Room, ids []int64, r *http.Request, actor string) radio.State {
	return room.Next(ids)
}

func radioPrev(h *Handler, room *radio.Room, ids []int64, r *http.Request, actor string) radio.State {
	return room.Prev(ids)
}

func radioEnded(h *Handler, room *radio.Room, ids []int64, r *http.Request, actor string) radio.State {
	return room.Ended(ids, formTuneID(r), time.Now())
}

func radioMode(h *Handler, room *radio.Room, ids []int64, r *http.Request, actor string) radio.State {
	mode := r.FormValue("mode")
	if mode != radio.ModeOrdered && mode != radio.ModeShuffled {
		return room.Snapshot()
	}
	return room.SetMode(mode)
}

// containsTune reports whether the catalogue includes the tune.
func containsTune(ids []int64, v int64) bool {
	for _, id := range ids {
		if id == v {
			return true
		}
	}
	return false
}
