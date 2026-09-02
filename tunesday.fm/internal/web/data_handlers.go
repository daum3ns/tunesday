package web

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"tunesday/tunesday.fm/internal/dataimport"
)

const maxEncodedPayload = 8 << 20 // base64 of a 5 MB tunesday.json

// ExportTeam downloads the team's SQLite state as a tunesday.json file.
func (h *Handler) ExportTeam(w http.ResponseWriter, r *http.Request) {
	team, _, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}

	providers, err := h.deps.Providers.ListByTeam(team.ID)
	if err != nil {
		redirectFlash(w, r, "/teams/"+team.Slug+"/dashboard", "err", "Export failed.")
		return
	}
	tunes, err := h.deps.Tunes.ListAllByTeam(team.ID)
	if err != nil {
		redirectFlash(w, r, "/teams/"+team.Slug+"/dashboard", "err", "Export failed.")
		return
	}

	data := dataimport.BuildExport(providers, tunes)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="tunesday-%s.json"`, team.Slug))
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(data)
}

// ImportPage renders the tunesday.json upload form.
func (h *Handler) ImportPage(w http.ResponseWriter, r *http.Request) {
	team, member, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	data := teamPage(team, member, "dashboard")
	data["Title"] = "Import tunesday.json"
	h.render(w, r, "import.html", data)
}

// ImportPreview parses the uploaded file and shows the destructive warning.
func (h *Handler) ImportPreview(w http.ResponseWriter, r *http.Request) {
	team, member, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	fail := func(msg string) {
		data := teamPage(team, member, "dashboard")
		data["Title"] = "Import tunesday.json"
		h.render(w, r, "import.html", flash(data, msg))
	}

	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		fail("Invalid upload")
		return
	}
	file, header, ferr := r.FormFile("tunesday_json")
	if ferr != nil {
		fail("Please attach a tunesday.json file.")
		return
	}
	defer file.Close()
	if header.Size > maxUploadBytes {
		fail("File too large (max 5 MB)")
		return
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxUploadBytes+1))
	if err != nil || len(raw) > maxUploadBytes {
		fail("Could not read the file.")
		return
	}

	parsed, perr := dataimport.Parse(bytes.NewReader(raw))
	if perr != nil {
		fail("Could not parse the file: " + perr.Error())
		return
	}

	// Compute the replace plan for the warning page.
	providers, err := h.deps.Providers.ListByTeam(team.ID)
	if err != nil {
		fail("Could not inspect the current team state.")
		return
	}
	currentTunes, _ := h.deps.Tunes.CountByTeam(team.ID)

	want := map[string]bool{}
	for _, name := range dataimport.ProviderNames(parsed) {
		want[name] = true
	}
	var toRemove, toKeep []string
	for _, p := range providers {
		if want[p.Name] {
			continue
		}
		if p.MemberUserID == "" {
			toRemove = append(toRemove, p.Name)
		} else {
			toKeep = append(toKeep, p.Name)
		}
	}

	data := teamPage(team, member, "dashboard")
	data["Title"] = "Confirm replace"
	data["CurrentTunes"] = currentTunes
	data["CurrentProviders"] = len(providers)
	data["NewTunes"] = len(parsed.Tunes)
	data["NewProviders"] = len(want)
	data["RemoveProviders"] = toRemove
	data["KeepProviders"] = toKeep
	data["PayloadB64"] = base64.StdEncoding.EncodeToString(raw)
	h.render(w, r, "import_confirm.html", data)
}

// ImportConfirm runs the replace transaction for a previously previewed file.
func (h *Handler) ImportConfirm(w http.ResponseWriter, r *http.Request) {
	team, _, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	back := "/teams/" + team.Slug + "/import"

	if err := r.ParseForm(); err != nil {
		redirectFlash(w, r, back, "err", "Invalid confirmation form.")
		return
	}
	b64 := r.FormValue("payload_b64")
	if b64 == "" || len(b64) > maxEncodedPayload {
		redirectFlash(w, r, back, "err", "The uploaded file is missing or too large.")
		return
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		redirectFlash(w, r, back, "err", "Corrupted upload, please try again.")
		return
	}

	parsed, perr := dataimport.Parse(bytes.NewReader(raw))
	if perr != nil {
		redirectFlash(w, r, back, "err", "Could not parse the file: "+perr.Error())
		return
	}

	res, err := dataimport.ReplaceTeam(h.deps.DB, team.ID, parsed)
	if err != nil {
		redirectFlash(w, r, back, "err", "Replace failed: "+err.Error())
		return
	}

	parts := []string{
		fmt.Sprintf("%d tunes deleted", res.TunesDeleted),
		fmt.Sprintf("%d imported", res.TunesImported),
	}
	if res.ProvidersCreated > 0 {
		parts = append(parts, fmt.Sprintf("%d providers added", res.ProvidersCreated))
	}
	if len(res.ProvidersRemoved) > 0 {
		parts = append(parts, "removed "+strings.Join(res.ProvidersRemoved, ", "))
	}
	if len(res.ProvidersKept) > 0 {
		parts = append(parts, "kept "+strings.Join(res.ProvidersKept, " (still assigned)"))
	}
	if res.InvitesRevoked > 0 {
		parts = append(parts, fmt.Sprintf("%d invitations revoked", res.InvitesRevoked))
	}
	redirectFlash(w, r, "/teams/"+team.Slug+"/dashboard", "ok",
		"Data replaced: "+strings.Join(parts, ". ")+".")
}
