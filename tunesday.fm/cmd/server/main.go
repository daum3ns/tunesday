package main

import (
	"log"
	"net/http"

	"tunesday/internal/playlist"
	"tunesday/tunesday.fm/internal/auth"
	"tunesday/tunesday.fm/internal/ceremony"
	"tunesday/tunesday.fm/internal/config"
	"tunesday/tunesday.fm/internal/db"
	"tunesday/tunesday.fm/internal/email"
	"tunesday/tunesday.fm/internal/store"
	"tunesday/tunesday.fm/internal/web"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	database, err := db.Open(cfg.SQLitePath)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer database.Close()

	deps := web.Deps{
		DB:            database,
		Users:         store.NewUserStore(database),
		Verifications: store.NewVerificationTokenStore(database),
		Sessions:      auth.NewSessionStore(cfg.SessionSecret, cfg.SessionSecure, cfg.SessionLifetime),
		Email:         email.NewService(cfg),
		Teams:         store.NewTeamStore(database),
		Providers:     store.NewProviderStore(database),
		Members:       store.NewTeamMemberStore(database),
		Invitations:   store.NewInvitationStore(database),
		Tunes:         store.NewTuneStore(database),
		Ceremonies:    store.NewCeremonyStore(database),
		Rooms:         ceremony.NewManager(),
		YT:            playlist.NewYouTube(),
	}

	wh, err := web.NewHandler(cfg, deps)
	if err != nil {
		log.Fatalf("web handlers: %v", err)
	}

	log.Printf("tunesday.fm listening on %s", cfg.ListenAddr)
	if err := http.ListenAndServe(cfg.ListenAddr, wh.Router()); err != nil {
		log.Fatalf("server: %v", err)
	}
}
