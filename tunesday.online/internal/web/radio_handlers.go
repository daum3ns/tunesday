package web

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/websocket"

	"tunesday/tunesday.online/internal/auth"
	"tunesday/tunesday.online/internal/live"
	"tunesday/tunesday.online/internal/store"
)

// listenerInfo is one entry in the "who's listening" broadcast.
type listenerInfo struct {
	Alias     string `json:"alias"`
	Provider  string `json:"provider"`
	TuneID    int64  `json:"tuneId"`
	TuneTitle string `json:"tuneTitle"`
	IsYou     bool   `json:"isYou"`
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

// radioHub gives each team's radio room a dedicated WebSocket hub slot.
func (h *Handler) radioHub(teamID string) *live.Room {
	return h.deps.Rooms.RoomFor("radio:" + teamID)
}

// RadioWS is the live wire for the listening room. Clients send
// "now_playing" messages; the server broadcasts the listener list.
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

	room.AliasFor(user.ID)
	client := hub.Join(conn, user.ID)
	defer func() {
		hub.Leave(client)
		client.CloseConnection()
		h.broadcastListeners(team.ID)
	}()

	h.sendListeners(conn, team.ID, user.ID)
	h.broadcastListenersExcept(client, team.ID)

	conn.SetReadLimit(256)
	_ = conn.SetReadDeadline(time.Now().Add(30 * time.Minute))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(30 * time.Minute))
	})
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var msg struct {
			Type   string `json:"type"`
			TuneID int64  `json:"tune_id"`
		}
		if json.Unmarshal(raw, &msg) != nil {
			continue
		}
		switch msg.Type {
		case "now_playing":
			room.SetNowPlaying(user.ID, msg.TuneID)
			h.broadcastListeners(team.ID)
		default:
			log.Printf("radio ws: unknown message type %q", msg.Type)
		}
	}
}

// RadioCommand handles the single command endpoint for now_playing reports
// over HTTP (fallback for clients that prefer POST over WebSocket).
func (h *Handler) RadioCommand(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	team, _, ok := h.requireMember(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	tuneID, _ := strconv.ParseInt(r.FormValue("tune_id"), 10, 64)

	room := h.deps.Radio.For(team.ID)
	if tuneID > 0 {
		tune, err := h.deps.Tunes.GetByID(tuneID)
		if err != nil || tune == nil || tune.TeamID != team.ID {
			http.Error(w, "tune not found", http.StatusNotFound)
			return
		}
		room.SetNowPlaying(user.ID, tuneID)
		_ = h.deps.PlayStats.Record(&store.PlayStat{
			TeamID: team.ID,
			TuneID: tuneID,
			UserID: user.ID,
		})
	} else {
		room.SetNowPlaying(user.ID, 0)
	}

	h.broadcastListeners(team.ID)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) broadcastListeners(teamID string) {
	hub := h.radioHub(teamID)
	if hub.Count() == 0 {
		return
	}
	listeners := h.buildListenerList(teamID, "")
	hub.Broadcast("radio_listeners", listeners)
}

func (h *Handler) broadcastListenersExcept(skip *live.Client, teamID string) {
	hub := h.radioHub(teamID)
	listeners := h.buildListenerList(teamID, "")
	hub.BroadcastExcept(skip, "radio_listeners", listeners)
}

func (h *Handler) sendListeners(conn *websocket.Conn, teamID, viewerID string) {
	listeners := h.buildListenerList(teamID, viewerID)
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_ = conn.WriteJSON(live.Message{Type: "radio_listeners", Payload: listeners})
}

func (h *Handler) buildListenerList(teamID, viewerID string) []listenerInfo {
	room := h.deps.Radio.For(teamID)
	hub := h.radioHub(teamID)
	members, _ := h.deps.Members.ListByTeam(teamID)
	providerByUser := map[string]string{}
	for _, m := range members {
		providerByUser[m.UserID] = m.ProviderName
	}
	np := room.NowPlayingSnapshot()

	var list []listenerInfo
	for _, uid := range hub.Participants() {
		tuneID := np[uid]
		info := listenerInfo{
			Alias:    room.AliasFor(uid),
			Provider: providerByUser[uid],
			TuneID:   tuneID,
			IsYou:    uid == viewerID,
		}
		if tuneID > 0 {
			if t, err := h.deps.Tunes.GetByID(tuneID); err == nil && t != nil {
				info.TuneTitle = t.Title
			}
		}
		list = append(list, info)
	}
	return list
}
