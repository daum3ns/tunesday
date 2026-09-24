package playlist

import (
	"context"
	"net/url"
	"strings"

	"github.com/kkdai/youtube/v2"
)

// Platform names. A tune's platform is stored in tunes.platform and is one of
// these constants — anything else means the link could not be normalized.
const (
	PlatformYouTube    = "youtube"
	PlatformSoundCloud = "soundcloud"
	PlatformBandcamp   = "bandcamp"
)

// Media is a normalized, platform-labeled link.
type Media struct {
	Platform string // one of the Platform* constants
	URL      string // canonical https link (what tunes.link stores)
	ID       string // YouTube video id; empty for other platforms
}

// TitleProvider resolves + titles any supported link.
type TitleProvider interface {
	Normalize(raw string) (Media, bool)
	FetchTitle(ctx context.Context, link string) (string, error)
}

type YouTube struct{ c *youtube.Client }

func NewYouTube() *YouTube { return &YouTube{c: &youtube.Client{}} }

// Normalize accepts https links on the allowlist (YouTube, SoundCloud,
// Bandcamp) and returns the canonical link. Everything else is denied.
func Normalize(raw string) (Media, bool) {
	link := StripTrackingParams(strings.TrimSpace(raw))
	u, err := url.Parse(link)
	if err != nil {
		return Media{}, false
	}
	if strings.ToLower(u.Scheme) != "https" {
		return Media{}, false
	}
	host := strings.ToLower(u.Host)
	host = strings.TrimPrefix(host, "www.")
	host = strings.TrimPrefix(host, "m.")

	switch host {
	case "youtube.com", "music.youtube.com", "youtu.be":
		id, ok := normalizeYouTubeID(link)
		if !ok {
			return Media{}, false
		}
		return Media{Platform: PlatformYouTube, URL: link, ID: id}, true
	case "soundcloud.com":
		parts := pathParts(u.Path)
		// <user>/<track>. Playlists (/sets/) are rejected for v1.
		if len(parts) >= 2 && parts[0] != "" && parts[1] != "" && parts[0] != "sets" {
			return Media{Platform: PlatformSoundCloud, URL: link}, true
		}
	case "bandcamp.com":
		// path len 1 means bare https://bandcamp.com/... — not a track host.
		return Media{}, false
	}
	if strings.HasSuffix(host, ".bandcamp.com") {
		parts := pathParts(u.Path)
		if len(parts) >= 2 && parts[0] == "track" && parts[1] != "" {
			return Media{Platform: PlatformBandcamp, URL: link}, true
		}
	}
	return Media{}, false
}

// NormalizeYouTubeID validates that the URL is https and points to a YouTube
// video. It returns the normalized video ID and true if valid.
func (y *YouTube) NormalizeYouTubeID(raw string) (string, bool) {
	return normalizeYouTubeID(raw)
}

func normalizeYouTubeID(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", false
	}
	if strings.ToLower(u.Scheme) != "https" {
		return "", false
	}
	host := strings.ToLower(u.Host)
	host = strings.TrimPrefix(host, "www.")
	host = strings.TrimPrefix(host, "m.")
	switch host {
	case "youtube.com", "music.youtube.com":
		if u.Path == "/watch" {
			v := u.Query().Get("v")
			if v != "" {
				return v, true
			}
		}
		if strings.HasPrefix(u.Path, "/shorts/") {
			id := strings.TrimPrefix(u.Path, "/shorts/")
			id = strings.SplitN(id, "/", 2)[0]
			if id != "" {
				return id, true
			}
		}
	case "youtu.be":
		id := strings.Trim(u.Path, "/")
		if id != "" {
			return id, true
		}
	}
	return "", false
}

// FetchTitle implements TitleProvider for the kkdai client (YouTube only).
func (y *YouTube) FetchTitle(ctx context.Context, link string) (string, error) {
	v, err := y.c.GetVideo(link)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(v.Title), nil
}

// StripTrackingParams removes common tracking/query parameters from a pasted link.
// Current behavior keeps everything before the first '&'. It is intentionally simple.
func StripTrackingParams(link string) string {
	parts := strings.Split(link, "&")
	if len(parts) == 0 {
		return link
	}
	return parts[0]
}

// pathParts splits a URL path into non-empty segments.
func pathParts(path string) []string {
	return strings.Split(strings.Trim(path, "/"), "/")
}
