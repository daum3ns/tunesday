package main

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"tunesday/tunesday.fm/internal/config"
	"tunesday/tunesday.fm/internal/db"
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

	wh, err := web.NewHandler()
	if err != nil {
		log.Fatalf("web handlers: %v", err)
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	r.Handle("/static/*", http.StripPrefix("/static/", wh.StaticFiles()))
	r.Get("/", wh.Landing)

	addr := cfg.ListenAddr
	log.Printf("tunesday.fm listening on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("server: %v", err)
	}
}
