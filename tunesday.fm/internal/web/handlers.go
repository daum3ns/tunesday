package web

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"
)

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed static/*
var staticFS embed.FS

// Handler holds the web handlers and templates.
type Handler struct {
	tmpl *template.Template
}

// NewHandler creates a new web handler, parsing embedded templates.
func NewHandler() (*Handler, error) {
	tmpl, err := template.ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &Handler{tmpl: tmpl}, nil
}

// StaticFiles returns an http.Handler for embedded static assets.
func (h *Handler) StaticFiles() http.Handler {
	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(staticSub))
}

// Landing renders the landing page.
func (h *Handler) Landing(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Title  string
		Header string
	}{
		Title:  "tunesday.fm",
		Header: headerASCII(),
	}
	_ = h.tmpl.ExecuteTemplate(w, "base.html", data)
}

func headerASCII() string {
	return `██████████████████████████████████████████████████████████████████████████
█▌                                                                      ▐█
█▌                                                                      ▐█
█▌                                                                      ▐█
█▌     ░▀█▀░▀█▀░▀░█▀▀░░░░░░░░░                                          ▐█
█▌     ░░█░░░█░░░░▀▀█░░░░░░░░░                                          ▐█
█▌     ░▀▀▀░░▀░░░░▀▀▀░▀░░▀░░▀░                                          ▐█
█▌     ░█░█░█▀█░█▀█░█▀█░█░█░░░▀█▀░█░█░█▀█░█▀▀░█▀▀░█▀▄░█▀█░█░█░░░█░█     ▐█
█▌     ░█▀█░█▀█░█▀▀░█▀▀░░█░░░░░█░░█░█░█░█░█▀▀░▀▀█░█░█░█▀█░░█░░░░▀░▀     ▐█
█▌     ░▀░▀░▀░▀░▀░░░▀░░░░▀░░░░░▀░░▀▀▀░▀░▀░▀▀▀░▀▀▀░▀▀░░▀░▀░░▀░░░░▀░▀     ▐█
█▌                                                                      ▐█
█▌                                                                      ▐█
█▌                                                                      ▐█
██████████████████████████████████████████████████████████████████████████`
}
