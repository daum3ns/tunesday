package reminder

import (
	"strings"
	"testing"
	"time"

	"tunesday/tunesday.online/internal/db"
	"tunesday/tunesday.online/internal/email"
	"tunesday/tunesday.online/internal/store"
)

type env struct {
	teams      *store.TeamStore
	members    *store.TeamMemberStore
	tunes      *store.TuneStore
	reminders  *store.ReminderStore
	mailer     *email.Service
	emails     *strings.Builder
	db         *db.DB
	providerID int64
}

// setup creates an in-memory DB with one team ("t1", Tunesday=Tuesday,
// timezone=America/New_York) and one admin member, plus a capturing mailer.
func setup(t *testing.T) *env {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	teams := store.NewTeamStore(database)
	members := store.NewTeamMemberStore(database)
	tunes := store.NewTuneStore(database)
	reminders := store.NewReminderStore(database)

	users := store.NewUserStore(database)
	u := &store.User{ID: "u1", Email: "admin@example.com", PasswordHash: "h",
		EmailVerified: true, CreatedAt: time.Now()}
	if err := users.Create(u); err != nil {
		t.Fatal(err)
	}
	if err := teams.Create(&store.Team{
		ID: "t1", Name: "Synths", Slug: "synths", AdminID: u.ID,
		Timezone: "America/New_York", TunesdayWeekday: int(time.Tuesday),
	}); err != nil {
		t.Fatal(err)
	}
	p, err := store.NewProviderStore(database).Create("t1", "Alan")
	if err != nil {
		t.Fatal(err)
	}
	if err := members.Create(&store.TeamMember{TeamID: "t1", UserID: u.ID,
		ProviderID: p.ID, Role: "admin"}); err != nil {
		t.Fatal(err)
	}

	mailer := &email.Service{}
	var emails strings.Builder
	mailer.SendFunc = func(to, subject, body string) error {
		emails.WriteString(to + "\n" + subject + "\n")
		return nil
	}
	return &env{teams, members, tunes, reminders, mailer, &emails, database, p.ID}
}

// newScheduler returns a pre-wired Scheduler whose clock is frozen at the
// given instant (format "2006-01-02 15:04") in America/New_York.
func (e *env) newScheduler(at string) *Scheduler {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		panic(err)
	}
	frozen, err := time.ParseInLocation("2006-01-02 15:04", at, loc)
	if err != nil {
		panic(err)
	}
	s := New(e.teams, e.members, e.tunes, e.reminders, e.mailer, time.Hour)
	s.SetClock(func() time.Time { return frozen })
	return s
}

func TestReminderFiresAfterTunesdayAndDedupes(t *testing.T) {
	e := setup(t)
	s := e.newScheduler("2026-09-09 06:00") // Wednesday — Tuesday ended, no tune
	s.check()
	if !strings.Contains(e.emails.String(), "admin@example.com") {
		t.Fatalf("expected reminder email, got:\n%s", e.emails.String())
	}
	s.check() // must be deduped via reminder_sent
	if got := strings.Count(e.emails.String(), "admin@example.com"); got != 1 {
		t.Fatalf("reminder sent %d times, want 1", got)
	}
}

func TestReminderSkipsWhileStillTunesday(t *testing.T) {
	e := setup(t)
	e.newScheduler("2026-09-08 10:00").check() // Tuesday 10:00, day not over
	if e.emails.Len() != 0 {
		t.Fatalf("unexpected email while still Tunesday:\n%s", e.emails.String())
	}
}

func TestReminderSkipsWhenTuneRegistered(t *testing.T) {
	e := setup(t)
	// A tune added during Tuesday noon EDT = 16:00 UTC.
	if _, err := e.db.Exec(
		`INSERT INTO tunes (team_id, title, link, youtube_id, provider_id, added_at)
		 VALUES ('t1', 'Song', 'https://youtu.be/aaaaaaaaaaa', 'aaaaaaaaaaa', ?, ?)`,
		e.providerID, store.FormatTime(time.Date(2026, 9, 8, 16, 0, 0, 0, time.UTC)),
	); err != nil {
		t.Fatalf("insert tune: %v", err)
	}

	e.newScheduler("2026-09-09 06:00").check()
	if e.emails.Len() != 0 {
		t.Fatalf("unexpected email despite tune registered:\n%s", e.emails.String())
	}
}
