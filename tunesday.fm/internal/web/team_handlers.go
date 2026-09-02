package web

import (
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"tunesday/internal/core"
	"tunesday/tunesday.fm/internal/auth"
	"tunesday/tunesday.fm/internal/dataimport"
	"tunesday/tunesday.fm/internal/store"
)

// urlEscape is a tiny alias for readability.
func urlEscape(s string) string { return url.QueryEscape(s) }

const maxUploadBytes = 5 << 20 // 5 MB

// teamPage holds the data shared by all team sub-pages.
func teamPage(team *store.Team, member *store.TeamMember, tab string) map[string]any {
	return map[string]any{
		"Title":    team.Name,
		"Team":     team,
		"Member":   member,
		"Tab":      tab,
		"TeamPath": "/teams/" + team.Slug,
	}
}

// requireMember resolves the {slug} team and checks membership.
// It renders an appropriate response and returns ok=false on failure.
func (h *Handler) requireMember(w http.ResponseWriter, r *http.Request) (*store.Team, *store.TeamMember, bool) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return nil, nil, false
	}

	slug := chi.URLParam(r, "slug")
	team, err := h.deps.Teams.GetBySlug(slug)
	if err != nil {
		h.render(w, r, "message.html", map[string]any{"Title": "Error", "Message": "Something went wrong"})
		return nil, nil, false
	}
	if team == nil {
		h.render(w, r, "message.html", map[string]any{"Title": "Not found", "Message": "This team does not exist."})
		return nil, nil, false
	}

	member, err := h.deps.Members.Get(team.ID, user.ID)
	if err != nil {
		h.render(w, r, "message.html", map[string]any{"Title": "Error", "Message": "Something went wrong"})
		return nil, nil, false
	}
	if member == nil {
		h.render(w, r, "message.html", map[string]any{
			"Title":    "Not a member",
			"Message":  "You are not a member of this team.",
			"TeamName": team.Name,
		})
		return nil, nil, false
	}
	return team, member, true
}

// requireAdmin is like requireMember but also checks the admin role.
func (h *Handler) requireAdmin(w http.ResponseWriter, r *http.Request) (*store.Team, *store.TeamMember, bool) {
	team, member, ok := h.requireMember(w, r)
	if !ok {
		return nil, nil, false
	}
	if member.Role != "admin" {
		h.render(w, r, "message.html", map[string]any{
			"Title":   "Admins only",
			"Message": "You need team admin rights for this action.",
		})
		return nil, nil, false
	}
	return team, member, true
}

// Onboarding lists the user's teams and offers team creation.
func (h *Handler) Onboarding(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	teams, err := h.deps.Teams.ListByUser(user.ID)
	if err != nil {
		h.render(w, r, "message.html", map[string]any{"Title": "Error", "Message": "Something went wrong"})
		return
	}
	h.render(w, r, "onboarding.html", map[string]any{
		"Title": "Your teams",
		"Teams": teams,
	})
}

// NewTeamForm renders the team creation form.
func (h *Handler) NewTeamForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "team_new.html", map[string]any{
		"Title": "Create a team",
	})
}

// CreateTeam handles the team creation form, with optional tunesday.json upload.
func (h *Handler) CreateTeam(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())

	fail := func(msg string) {
		h.render(w, r, "team_new.html", flash(map[string]any{"Title": "Create a team"}, msg))
	}

	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		fail("Invalid form submission")
		return
	}

	teamName := strings.TrimSpace(r.FormValue("team_name"))
	myName := strings.TrimSpace(r.FormValue("your_name"))
	if teamName == "" || myName == "" {
		fail("Team name and your provider name are required")
		return
	}

	var data *core.Data
	if file, header, ferr := r.FormFile("tunesday_json"); ferr == nil {
		defer file.Close()
		if header.Size > maxUploadBytes {
			fail("File too large (max 5 MB)")
			return
		}
		parsed, perr := dataimport.Parse(file)
		if perr != nil {
			fail("Could not read tunesday.json: " + perr.Error())
			return
		}
		data = parsed
	} else if !errors.Is(ferr, http.ErrMissingFile) {
		fail("Could not read uploaded file")
		return
	}

	slug, err := h.deps.Teams.GenerateSlug(teamName)
	if err != nil {
		fail("Could not generate team URL")
		return
	}

	in := dataimport.CreateTeamInput{
		AdminUserID:       user.ID,
		TeamName:          teamName,
		Slug:              slug,
		AdminProviderName: myName,
	}
	if data != nil {
		in.Data = data
	}

	res, err := dataimport.CreateTeam(h.deps.DB, in)
	if err != nil {
		fail("Could not create team: " + err.Error())
		return
	}

	msg := "Team created!"
	if data != nil {
		msg = "Team created: " + strconv.Itoa(res.ProvidersCreated) +
			" providers, " + strconv.Itoa(res.TunesInserted) + " tunes imported."
	}
	redirectFlash(w, r, "/teams/"+slug+"/dashboard", "ok", msg)
}

// Dashboard renders the team overview.
func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	team, member, ok := h.requireMember(w, r)
	if !ok {
		return
	}

	providers, err := h.deps.Providers.ListEligibleByTeam(team.ID)
	if err != nil {
		redirectFlash(w, r, "/teams/"+team.Slug+"/dashboard", "err", "Something went wrong")
		return
	}
	membersList, err := h.deps.Members.ListByTeam(team.ID)
	if err != nil {
		redirectFlash(w, r, "/teams/"+team.Slug+"/dashboard", "err", "Something went wrong")
		return
	}
	tuneCount, err := h.deps.Tunes.CountByTeam(team.ID)
	if err != nil {
		redirectFlash(w, r, "/teams/"+team.Slug+"/dashboard", "err", "Something went wrong")
		return
	}
	recent, err := h.deps.Tunes.ListRecentByTeam(team.ID, 5)
	if err != nil {
		redirectFlash(w, r, "/teams/"+team.Slug+"/dashboard", "err", "Something went wrong")
		return
	}

	data := teamPage(team, member, "dashboard")
	data["EligibleCount"] = len(providers)
	data["MemberCount"] = len(membersList)
	data["TuneCount"] = tuneCount
	data["RecentTunes"] = recent
	data["SessionLink"] = h.cfg.BaseURL + "/teams/" + team.Slug + "/dashboard"
	h.render(w, r, "dashboard.html", data)
}

// ProvidersPage lists providers with assignment info.
func (h *Handler) ProvidersPage(w http.ResponseWriter, r *http.Request) {
	team, member, ok := h.requireMember(w, r)
	if !ok {
		return
	}
	providers, err := h.deps.Providers.ListByTeam(team.ID)
	if err != nil {
		redirectFlash(w, r, "/teams/"+team.Slug+"/providers", "err", "Something went wrong")
		return
	}
	data := teamPage(team, member, "providers")
	data["Providers"] = providers
	h.render(w, r, "providers.html", data)
}

// AddProvider creates a new provider name.
func (h *Handler) AddProvider(w http.ResponseWriter, r *http.Request) {
	team, _, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		redirectFlash(w, r, "/teams/"+team.Slug+"/providers", "err", "Invalid form")
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		redirectFlash(w, r, "/teams/"+team.Slug+"/providers", "err", "Provider name is required")
		return
	}
	if existing, _ := h.deps.Providers.GetByName(team.ID, name); existing != nil {
		redirectFlash(w, r, "/teams/"+team.Slug+"/providers", "err", "Provider already exists")
		return
	}
	if _, err := h.deps.Providers.Create(team.ID, name); err != nil {
		redirectFlash(w, r, "/teams/"+team.Slug+"/providers", "err", "Could not create provider")
		return
	}
	redirectFlash(w, r, "/teams/"+team.Slug+"/providers", "ok", "Provider added")
}

// providerIDParam parses the {id} URL parameter.
func providerIDParam(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

// RenameProvider updates a provider name.
func (h *Handler) RenameProvider(w http.ResponseWriter, r *http.Request) {
	team, _, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	id, err := providerIDParam(r)
	if err != nil {
		redirectFlash(w, r, "/teams/"+team.Slug+"/providers", "err", "Invalid provider")
		return
	}
	if err := r.ParseForm(); err != nil {
		redirectFlash(w, r, "/teams/"+team.Slug+"/providers", "err", "Invalid form")
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		redirectFlash(w, r, "/teams/"+team.Slug+"/providers", "err", "Provider name is required")
		return
	}
	if existing, _ := h.deps.Providers.GetByName(team.ID, name); existing != nil && existing.ID != id {
		redirectFlash(w, r, "/teams/"+team.Slug+"/providers", "err", "Another provider already has that name")
		return
	}
	provider, err := h.deps.Providers.GetByID(id)
	if err != nil || provider == nil || provider.TeamID != team.ID {
		redirectFlash(w, r, "/teams/"+team.Slug+"/providers", "err", "Provider not found")
		return
	}
	if err := h.deps.Providers.Rename(id, name); err != nil {
		redirectFlash(w, r, "/teams/"+team.Slug+"/providers", "err", "Could not rename provider")
		return
	}
	redirectFlash(w, r, "/teams/"+team.Slug+"/providers", "ok", "Provider renamed")
}

// ToggleProvider enables or disables a provider.
func (h *Handler) ToggleProvider(w http.ResponseWriter, r *http.Request) {
	team, _, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	id, err := providerIDParam(r)
	if err != nil {
		redirectFlash(w, r, "/teams/"+team.Slug+"/providers", "err", "Invalid provider")
		return
	}
	provider, err := h.deps.Providers.GetByID(id)
	if err != nil || provider == nil || provider.TeamID != team.ID {
		redirectFlash(w, r, "/teams/"+team.Slug+"/providers", "err", "Provider not found")
		return
	}
	if err := h.deps.Providers.SetDisabled(id, !provider.Disabled); err != nil {
		redirectFlash(w, r, "/teams/"+team.Slug+"/providers", "err", "Could not update provider")
		return
	}
	state := "enabled"
	if !provider.Disabled {
		state = "disabled"
	}
	redirectFlash(w, r, "/teams/"+team.Slug+"/providers", "ok", "Provider "+state)
}

// DeleteProvider removes a provider without tunes or members.
func (h *Handler) DeleteProvider(w http.ResponseWriter, r *http.Request) {
	team, _, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	id, err := providerIDParam(r)
	if err != nil {
		redirectFlash(w, r, "/teams/"+team.Slug+"/providers", "err", "Invalid provider")
		return
	}
	provider, err := h.deps.Providers.GetByID(id)
	if err != nil || provider == nil || provider.TeamID != team.ID {
		redirectFlash(w, r, "/teams/"+team.Slug+"/providers", "err", "Provider not found")
		return
	}
	if err := h.deps.Providers.Delete(id); err != nil {
		redirectFlash(w, r, "/teams/"+team.Slug+"/providers", "err", err.Error())
		return
	}
	redirectFlash(w, r, "/teams/"+team.Slug+"/providers", "ok", "Provider deleted")
}

// MembersPage lists members and pending invitations with the invite form.
func (h *Handler) MembersPage(w http.ResponseWriter, r *http.Request) {
	team, member, ok := h.requireMember(w, r)
	if !ok {
		return
	}
	members, err := h.deps.Members.ListByTeam(team.ID)
	if err != nil {
		redirectFlash(w, r, "/teams/"+team.Slug+"/members", "err", "Something went wrong")
		return
	}
	var pending []*store.Invitation
	var providers []*store.ProviderView
	if member.Role == "admin" {
		var err error
		pending, err = h.deps.Invitations.ListPendingByTeam(team.ID)
		if err != nil {
			redirectFlash(w, r, "/teams/"+team.Slug+"/members", "err", "Something went wrong")
			return
		}
		providers, err = h.deps.Providers.ListByTeam(team.ID)
		if err != nil {
			redirectFlash(w, r, "/teams/"+team.Slug+"/members", "err", "Something went wrong")
			return
		}
	}
	data := teamPage(team, member, "members")
	data["Members"] = members
	data["PendingInvites"] = pending
	data["Providers"] = providers
	h.render(w, r, "members.html", data)
}

// InviteMember sends an invitation email with a join link.
func (h *Handler) InviteMember(w http.ResponseWriter, r *http.Request) {
	team, _, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	path := "/teams/" + team.Slug + "/members"
	if err := r.ParseForm(); err != nil {
		redirectFlash(w, r, path, "err", "Invalid form")
		return
	}

	emailAddr := strings.TrimSpace(r.FormValue("email"))
	providerName := strings.TrimSpace(r.FormValue("provider_name"))
	if emailAddr == "" {
		redirectFlash(w, r, path, "err", "Email is required")
		return
	}

	// Already a member?
	if existing, _ := h.deps.Users.GetByEmail(emailAddr); existing != nil {
		if m, _ := h.deps.Members.Get(team.ID, existing.ID); m != nil {
			redirectFlash(w, r, path, "err", "That user is already a member")
			return
		}
	}
	if has, _ := h.deps.Invitations.HasPendingForEmail(team.ID, emailAddr); has {
		redirectFlash(w, r, path, "err", "An invitation for that email is already pending")
		return
	}

	var providerID int64
	if providerName != "" {
		p, err := h.deps.Providers.GetOrCreate(team.ID, providerName)
		if err != nil {
			redirectFlash(w, r, path, "err", "Could not prepare provider")
			return
		}
		if assigned, _ := h.deps.Members.ListByTeam(team.ID); providerTaken(assigned, p.ID, "") {
			redirectFlash(w, r, path, "err", "That provider is already assigned to a member")
			return
		}
		providerID = p.ID
	}

	inv := &store.Invitation{
		TeamID:     team.ID,
		Email:      emailAddr,
		ProviderID: providerID,
	}
	if err := h.deps.Invitations.Create(inv); err != nil {
		redirectFlash(w, r, path, "err", "Could not create invitation")
		return
	}

	inviteURL := h.cfg.BaseURL + "/invite/" + inv.Token
	if err := h.deps.Email.SendInvitationEmail(emailAddr, team.Name, inviteURL); err != nil {
		_ = h.deps.Invitations.Delete(inv.ID)
		redirectFlash(w, r, path, "err", "Could not send invitation email: "+err.Error())
		return
	}

	redirectFlash(w, r, path, "ok", "Invitation sent to "+emailAddr)
}

// providerTaken reports whether a provider is assigned to any member other than exceptUserID.
func providerTaken(members []*store.TeamMemberView, providerID int64, exceptUserID string) bool {
	for _, m := range members {
		if m.ProviderID == providerID && m.UserID != exceptUserID {
			return true
		}
	}
	return false
}

// SetMemberRole promotes or demotes a member.
func (h *Handler) SetMemberRole(w http.ResponseWriter, r *http.Request) {
	team, actor, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	path := "/teams/" + team.Slug + "/members"
	target := chi.URLParam(r, "user")
	if err := r.ParseForm(); err != nil {
		redirectFlash(w, r, path, "err", "Invalid form")
		return
	}
	role := r.FormValue("role")
	if role != "admin" && role != "member" {
		redirectFlash(w, r, path, "err", "Invalid role")
		return
	}

	if role == "member" {
		admins, _ := h.deps.Members.CountAdmins(team.ID)
		if actor.UserID == target && admins <= 1 {
			redirectFlash(w, r, path, "err", "A team needs at least one admin")
			return
		}
	}

	if err := h.deps.Members.UpdateRole(team.ID, target, role); err != nil {
		redirectFlash(w, r, path, "err", "Could not update role")
		return
	}
	redirectFlash(w, r, path, "ok", "Role updated")
}

// SetMemberProvider reassigns a member to a (different) provider.
func (h *Handler) SetMemberProvider(w http.ResponseWriter, r *http.Request) {
	team, _, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	path := "/teams/" + team.Slug + "/members"
	target := chi.URLParam(r, "user")
	if err := r.ParseForm(); err != nil {
		redirectFlash(w, r, path, "err", "Invalid form")
		return
	}
	pid, err := strconv.ParseInt(r.FormValue("provider_id"), 10, 64)
	if err != nil {
		redirectFlash(w, r, path, "err", "Invalid provider")
		return
	}
	provider, err := h.deps.Providers.GetByID(pid)
	if err != nil || provider == nil || provider.TeamID != team.ID {
		redirectFlash(w, r, path, "err", "Provider not found in this team")
		return
	}
	members, _ := h.deps.Members.ListByTeam(team.ID)
	if providerTaken(members, pid, target) {
		redirectFlash(w, r, path, "err", "That provider is already assigned to another member")
		return
	}
	if err := h.deps.Members.UpdateProvider(team.ID, target, pid); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			redirectFlash(w, r, path, "err", "Member not found")
			return
		}
		redirectFlash(w, r, path, "err", "Could not reassign provider")
		return
	}
	redirectFlash(w, r, path, "ok", "Provider assignment updated")
}

// RemoveMember deletes a membership.
func (h *Handler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	team, actor, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	path := "/teams/" + team.Slug + "/members"
	target := chi.URLParam(r, "user")

	if actor.UserID == target {
		admins, _ := h.deps.Members.CountAdmins(team.ID)
		if admins <= 1 {
			redirectFlash(w, r, path, "err", "You cannot remove the last admin")
			return
		}
	}

	if err := h.deps.Members.Delete(team.ID, target); err != nil {
		redirectFlash(w, r, path, "err", "Could not remove member")
		return
	}
	redirectFlash(w, r, path, "ok", "Member removed")
}

// AcceptInvitePage renders the invitation acceptance screen.
func (h *Handler) AcceptInvitePage(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	inv, team, err := h.loadInvitation(token)
	if err != nil {
		h.render(w, r, "message.html", map[string]any{"Title": "Invitation", "Message": err.Error()})
		return
	}
	if !inv.Pending() {
		h.render(w, r, "message.html", map[string]any{
			"Title": "Invitation", "Message": "This invitation was already accepted.",
		})
		return
	}

	data := map[string]any{
		"Title":    "Join " + team.Name,
		"TeamName": team.Name,
		"Email":    inv.Email,
		"Token":    token,
	}
	if inv.ProviderID != 0 {
		data["ProviderID"] = inv.ProviderID
		if p, _ := h.deps.Providers.GetByID(inv.ProviderID); p != nil {
			data["ProviderName"] = p.Name
		}
	} else {
		unassigned, _ := h.unassignedProviders(team.ID)
		data["Unassigned"] = unassigned
	}
	h.render(w, r, "invite_accept.html", data)
}

// AcceptInvite handles the invitation acceptance form.
func (h *Handler) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	inv, team, err := h.loadInvitation(token)
	if err != nil {
		h.render(w, r, "message.html", map[string]any{"Title": "Invitation", "Message": err.Error()})
		return
	}
	if !inv.Pending() {
		h.render(w, r, "message.html", map[string]any{
			"Title": "Invitation", "Message": "This invitation was already accepted.",
		})
		return
	}

	if err := r.ParseForm(); err != nil {
		redirectFlash(w, r, "/invite/"+token, "err", "Invalid form")
		return
	}

	// Resolve or create the user for this invitation.
	user := auth.UserFromContext(r.Context())
	if user == nil {
		user, err = h.deps.Users.GetOrCreateByEmail(inv.Email)
		if err != nil || user == nil {
			redirectFlash(w, r, "/invite/"+token, "err", "Could not create your account")
			return
		}
		if err := h.deps.Sessions.SetUserID(w, r, user.ID); err != nil {
			redirectFlash(w, r, "/invite/"+token, "err", "Could not start your session")
			return
		}
	}

	// If already a member, just close the invitation.
	if existing, _ := h.deps.Members.Get(team.ID, user.ID); existing != nil {
		_ = h.deps.Invitations.MarkAccepted(inv.ID, user.ID)
		redirectFlash(w, r, "/teams/"+team.Slug+"/dashboard", "ok", "Welcome back to "+team.Name)
		return
	}

	// Determine the provider: pre-assigned, selected, or newly typed.
	providerID := inv.ProviderID
	switch {
	case providerID != 0:
		// keep admin's pre-assignment
	default:
		if pid, perr := strconv.ParseInt(r.FormValue("provider_id"), 10, 64); perr == nil && pid > 0 {
			p, gerr := h.deps.Providers.GetByID(pid)
			if gerr != nil || p == nil || p.TeamID != team.ID {
				redirectFlash(w, r, "/invite/"+token, "err", "Invalid provider selection")
				return
			}
			members, _ := h.deps.Members.ListByTeam(team.ID)
			if providerTaken(members, pid, "") {
				redirectFlash(w, r, "/invite/"+token, "err", "That provider is already taken")
				return
			}
			providerID = pid
		} else if name := strings.TrimSpace(r.FormValue("provider_name")); name != "" {
			p, cerr := h.deps.Providers.GetOrCreate(team.ID, name)
			if cerr != nil {
				redirectFlash(w, r, "/invite/"+token, "err", "Could not create your provider")
				return
			}
			providerID = p.ID
		} else {
			redirectFlash(w, r, "/invite/"+token, "err", "Please choose or enter a provider name")
			return
		}
	}

	m := &store.TeamMember{
		TeamID:     team.ID,
		UserID:     user.ID,
		ProviderID: providerID,
		Role:       "member",
	}
	if err := h.deps.Members.Create(m); err != nil {
		redirectFlash(w, r, "/invite/"+token, "err", "Could not complete the invitation: "+err.Error())
		return
	}
	_ = h.deps.Invitations.MarkAccepted(inv.ID, user.ID)

	redirectFlash(w, r, "/teams/"+team.Slug+"/dashboard", "ok", "Welcome to "+team.Name+"! You are now a provider.")
}

// JoinMagicLink logs a member in via their personal magic token.
func (h *Handler) JoinMagicLink(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	member, err := h.deps.Members.GetByMagicToken(token)
	if err != nil || member == nil {
		h.render(w, r, "message.html", map[string]any{
			"Title":   "Join",
			"Message": "This magic link is no longer valid. Ask a team admin to resend it.",
		})
		return
	}
	user, err := h.deps.Users.GetByID(member.UserID)
	if err != nil || user == nil {
		h.render(w, r, "message.html", map[string]any{
			"Title":   "Join",
			"Message": "Account not found for this link.",
		})
		return
	}
	if err := h.deps.Sessions.SetUserID(w, r, user.ID); err != nil {
		h.render(w, r, "message.html", map[string]any{"Title": "Join", "Message": "Could not start your session."})
		return
	}
	team, _ := h.deps.Teams.GetByID(member.TeamID)
	if team == nil {
		h.render(w, r, "message.html", map[string]any{"Title": "Join", "Message": "Team not found."})
		return
	}
	http.Redirect(w, r, "/teams/"+team.Slug+"/dashboard?ok="+urlEscape("Welcome back, "+team.Name+"!"), http.StatusSeeOther)
}

func (h *Handler) loadInvitation(token string) (*store.Invitation, *store.Team, error) {
	inv, err := h.deps.Invitations.GetByToken(token)
	if err != nil {
		return nil, nil, errors.New("something went wrong")
	}
	if inv == nil {
		return nil, nil, errors.New("this invitation link is not valid")
	}
	team, err := h.deps.Teams.GetByID(inv.TeamID)
	if err != nil || team == nil {
		return nil, nil, errors.New("the team for this invitation no longer exists")
	}
	return inv, team, nil
}

func (h *Handler) unassignedProviders(teamID string) ([]*store.ProviderView, error) {
	all, err := h.deps.Providers.ListByTeam(teamID)
	if err != nil {
		return nil, err
	}
	var out []*store.ProviderView
	for _, p := range all {
		if p.MemberUserID == "" {
			out = append(out, p)
		}
	}
	return out, nil
}
