// Package reminder runs the no-tune reminder scheduler: on the morning after
// a team's Tunesday, if no tune was registered that day, every member gets an
// email pointing back at the ceremony so the winner can still provide one.
package reminder

import (
	"context"
	"log"
	"time"

	"tunesday/tunesday.online/internal/email"
	"tunesday/tunesday.online/internal/store"
)

// Scheduler periodically checks teams whose Tunesday ended without a tune and
// emails the winner once (deduplicated via the reminder_sent table).
type Scheduler struct {
	teams      *store.TeamStore
	members    *store.TeamMemberStore
	tunes      *store.TuneStore
	reminders  *store.ReminderStore
	ceremonies *store.CeremonyStore
	mailer     *email.Service

	runEvery time.Duration
	now      func() time.Time
	logf     func(format string, args ...any)
}

// New builds a reminder scheduler. now/logf override the defaults for tests.
func New(teams *store.TeamStore, members *store.TeamMemberStore,
	tunes *store.TuneStore, reminders *store.ReminderStore, ceremonies *store.CeremonyStore,
	mailer *email.Service, runEvery time.Duration) *Scheduler {
	return &Scheduler{
		teams:      teams,
		members:    members,
		tunes:      tunes,
		reminders:  reminders,
		ceremonies: ceremonies,
		mailer:     mailer,
		runEvery:   runEvery,
		now:        time.Now,
		logf:       log.Printf,
	}
}

// SetClock injects the clock source (used by tests).
func (s *Scheduler) SetClock(now func() time.Time) { s.now = now }

// SetLogger injects the logger (used by tests).
func (s *Scheduler) SetLogger(logf func(format string, args ...any)) { s.logf = logf }

// Run checks once immediately, then every runEvery until ctx is done.
func (s *Scheduler) Run(ctx context.Context) {
	s.check()
	ticker := time.NewTicker(s.runEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.check()
		}
	}
}

// check scans every team for a Tunesday that ended without a tune.
func (s *Scheduler) check() {
	teams, err := s.teams.ListAll()
	if err != nil {
		s.logf("reminder: list teams: %v", err)
		return
	}
	now := s.now()
	for _, team := range teams {
		s.checkTeam(team, now)
	}
}

func (s *Scheduler) checkTeam(team *store.Team, now time.Time) {
	loc := team.Location()
	today := now.In(loc)

	// The reminder fires any time after the Tunesday ends, i.e., when today is
	// strictly after the team's Tunesday. daysBack is how many days ago that
	// Tunesday was.
	daysBack := (int(today.Weekday()) - team.TunesdayWeekday + 7) % 7
	if daysBack == 0 {
		return // still Tunesday; the day is not over yet
	}

	date := today.AddDate(0, 0, -daysBack).Format("2006-01-02")
	sent, err := s.reminders.Sent(team.ID, date)
	if err != nil {
		s.logf("reminder: check %s: %v", team.Name, err)
		return
	}
	if sent {
		return
	}

	start := dayStart(today.AddDate(0, 0, -daysBack), loc)
	end := dayStart(today.AddDate(0, 0, -daysBack+1), loc)
	count, err := s.tunes.CountAddedBetween(team.ID, store.FormatTime(start).(string), store.FormatTime(end).(string))
	if err != nil {
		s.logf("reminder: count tunes for %s: %v", team.Name, err)
		return
	}
	if count > 0 {
		return // the day was covered; nothing to nudge
	}

	// Get the first revealed ceremony on this date to find the winner
	dateUTC := start.Format("2006-01-02")
	ceremony, err := s.ceremonies.GetFirstRevealedByTeamOnDate(team.ID, dateUTC)
	if err != nil {
		s.logf("reminder: get ceremony for %s on %s: %v", team.Name, dateUTC, err)
		return
	}
	if ceremony == nil {
		return // no ceremony that day
	}
	if ceremony.WinnerProviderID == 0 {
		return // ceremony not yet revealed
	}

	// Get the winner's member info to find their email
	members, err := s.members.ListByTeam(team.ID)
	if err != nil {
		s.logf("reminder: list members for %s: %v", team.Name, err)
		return
	}

	var winnerEmail string
	for _, m := range members {
		if m.ProviderID == ceremony.WinnerProviderID {
			winnerEmail = m.Email
			break
		}
	}

	if winnerEmail == "" {
		return // winner not found or no email
	}

	// Email only the winner
	if err := s.mailer.SendNoTuneReminderEmail(winnerEmail, team.Name, date); err != nil {
		s.logf("reminder: email to winner of %s for %s failed: %v", team.Name, date, err)
	}

	// Mark as sent to prevent re-sending
	if err := s.reminders.Mark(team.ID, date); err != nil {
		s.logf("reminder: mark %s for %s: %v", team.Name, date, err)
	}
}

// dayStart returns the start of the given date in loc, at UTC.
func dayStart(date time.Time, loc *time.Location) time.Time {
	y, m, d := date.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc).UTC()
}
