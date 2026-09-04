package main

import (
	"log"
	"net/http"

	"tunesday/tunesday.online/internal/auth"
	"tunesday/tunesday.online/internal/config"
	"tunesday/tunesday.online/internal/db"
	"tunesday/tunesday.online/internal/email"
	"tunesday/tunesday.online/internal/live"
	"tunesday/tunesday.online/internal/radio"
	"tunesday/tunesday.online/internal/store"
	"tunesday/tunesday.online/internal/stream"
	"tunesday/tunesday.online/internal/web"
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

	// Bootstrap master admin from env var.
	if cfg.MasterAdminEmail != "" {
		users := store.NewUserStore(database)
		user, err := users.GetByEmail(cfg.MasterAdminEmail)
		if err != nil {
			log.Printf("tunesday.online: WARNING master admin lookup failed: %v", err)
		} else if user == nil {
			log.Printf("tunesday.online: WARNING TUNESDAY_MASTER_ADMIN_EMAIL=%s: user not found", cfg.MasterAdminEmail)
		} else if !user.MasterAdmin {
			if err := users.SetMasterAdmin(user.ID, true); err != nil {
				log.Printf("tunesday.online: WARNING failed to set master admin: %v", err)
			} else {
				log.Printf("tunesday.online: master admin set to %s", cfg.MasterAdminEmail)
			}
		}
	}

	// One yt-dlp extractor serves both the stream resolver and title
	// fetching — the proven path the CLI's mpv radio uses.
	extractor := stream.NewYTDLP()
	if err := extractor.Available(); err != nil {
		log.Printf("tunesday.online: WARNING yt-dlp not available (%v); "+
			"radio stream mode will fall back to the iframe player and title "+
			"lookup will fail. Install yt-dlp or set TUNESDAY_ONLINE_YTDLP_PATH.", err)
	}

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
		PlayStats:     store.NewPlayStatStore(database),
		Quiz:          store.NewQuizStore(database),
		Rooms:         live.NewManager(),
		Radio:         radio.NewManager(),
		Streams:       stream.NewCached(extractor, 0, 0),
		YT:            extractor,
	}

	wh, err := web.NewHandler(cfg, deps)
	if err != nil {
		log.Fatalf("web handlers: %v", err)
	}

	log.Printf("tunesday.online listening on %s", cfg.ListenAddr)
	if err := http.ListenAndServe(cfg.ListenAddr, wh.Router()); err != nil {
		log.Fatalf("server: %v", err)
	}
}
