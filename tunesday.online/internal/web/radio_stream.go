package web

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
)

// RadioStream returns the resolved audio stream URL as JSON:
// GET /teams/{slug}/radio/stream?tune_id=N
// The client plays directly from the CDN — no server proxy needed.
func (h *Handler) RadioStream(w http.ResponseWriter, r *http.Request) {
	team, _, ok := h.requireMember(w, r)
	if !ok {
		return
	}

	tuneID, err := strconv.ParseInt(r.URL.Query().Get("tune_id"), 10, 64)
	if err != nil || tuneID <= 0 {
		http.Error(w, "tune_id required", http.StatusBadRequest)
		return
	}
	tune, err := h.deps.Tunes.GetByID(tuneID)
	if err != nil || tune == nil || tune.TeamID != team.ID {
		http.Error(w, "tune not found", http.StatusNotFound)
		return
	}
	if tune.YouTubeID == "" {
		http.Error(w, "tune has no video id", http.StatusUnprocessableEntity)
		return
	}

	info, err := h.deps.Streams.Resolve(r.Context(), tune.YouTubeID)
	if err != nil {
		log.Printf("tunesday.online: stream resolve %s: %v", tune.YouTubeID, err)
		h.invalidateStream(tune.YouTubeID)
		http.Error(w, "stream unavailable", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"url":      info.URL,
		"mimeType": info.MimeType,
		"expires":  info.ExpiresAt.Unix(),
	})
}

// invalidateStream drops a possibly-dead cached URL from the resolver cache.
func (h *Handler) invalidateStream(videoID string) {
	if inv, ok := h.deps.Streams.(interface{ Invalidate(string) }); ok {
		inv.Invalidate(videoID)
	}
}
