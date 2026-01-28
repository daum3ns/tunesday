package playlist

import (
	"context"
	"net/url"
	"strings"

	"github.com/kkdai/youtube/v2"
)

// TitleProvider exposes title fetching and ID normalization.
type TitleProvider interface {
	NormalizeYouTubeID(raw string) (string, bool)
	FetchTitle(ctx context.Context, linkOrID string) (string, error)
}

type YouTube struct{ c *youtube.Client }

func NewYouTube() *YouTube { return &YouTube{c: &youtube.Client{}} }

// NormalizeYouTubeID validates that the URL is https and points to a YouTube video.
// It returns the normalized video ID and true if valid.
func (y *YouTube) NormalizeYouTubeID(raw string) (string, bool) {
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

func (y *YouTube) FetchTitle(ctx context.Context, linkOrID string) (string, error) {
	v, err := y.c.GetVideo(linkOrID)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(v.Title), nil
}

// StripTrackingParams removes common tracking/query parameters from a pasted YouTube URL.
// Current behavior keeps everything before the first '&'. It is intentionally simple.
func StripTrackingParams(link string) string {
	parts := strings.Split(link, "&")
	if len(parts) == 0 {
		return link
	}
	return parts[0]
}
