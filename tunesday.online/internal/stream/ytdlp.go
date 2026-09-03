package stream

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"tunesday/internal/playlist"
)

// YTDLP resolves streams and titles by shelling out to yt-dlp — the very
// extractor the CLI's mpv radio relies on via ytdl_hook. This guarantees the
// web path inherits yt-dlp's ongoing maintenance against YouTube changes.
//
// It implements both stream.Resolver and playlist.TitleProvider.
type YTDLP struct {
	// Bin is the yt-dlp executable; empty consults
	// TUNESDAY_ONLINE_YTDLP_PATH and falls back to "yt-dlp".
	Bin string
	// Timeout bounds each yt-dlp invocation.
	Timeout time.Duration

	norm *playlist.YouTube // pure URL normalisation, no network
}

// NewYTDLP builds the default extractor.
func NewYTDLP() *YTDLP {
	return &YTDLP{Timeout: 30 * time.Second, norm: playlist.NewYouTube()}
}

func (y *YTDLP) bin() string {
	if y.Bin != "" {
		return y.Bin
	}
	if v := os.Getenv("TUNESDAY_ONLINE_YTDLP_PATH"); v != "" {
		return v
	}
	return "yt-dlp"
}

func (y *YTDLP) timeout() time.Duration {
	if y.Timeout <= 0 {
		return 30 * time.Second
	}
	return y.Timeout
}

// run executes yt-dlp and returns trimmed stdout.
func (y *YTDLP) run(ctx context.Context, args ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, y.timeout())
	defer cancel()

	cmd := exec.CommandContext(cctx, y.bin(), args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		tail := strings.TrimSpace(stderr.String())
		if lines := strings.Split(tail, "\n"); len(lines) > 0 {
			tail = strings.TrimSpace(lines[len(lines)-1])
		}
		if tail == "" {
			tail = err.Error()
		}
		return "", fmt.Errorf("yt-dlp: %s", tail)
	}
	out := strings.TrimSpace(stdout.String())
	if out == "" {
		return "", fmt.Errorf("yt-dlp: empty output")
	}
	return out, nil
}

// audioFormat prefers m4a (AAC — best browser support), then any audio.
const audioFormat = "bestaudio[ext=m4a]/bestaudio/best"

// Resolve returns a direct audio stream URL for the video.
func (y *YTDLP) Resolve(ctx context.Context, videoID string) (Info, error) {
	out, err := y.run(ctx, "-g", "--no-playlist", "--no-warnings",
		"-f", audioFormat, watchURL(videoID))
	if err != nil {
		return Info{}, err
	}
	line, _, _ := strings.Cut(out, "\n") // -g may print per-format lines
	return Info{
		URL:       line,
		MimeType:  mimeFromURL(line),
		ExpiresAt: expireFromURL(line),
	}, nil
}

// FetchTitle implements playlist.TitleProvider.
func (y *YTDLP) FetchTitle(ctx context.Context, linkOrID string) (string, error) {
	target := strings.TrimSpace(linkOrID)
	if !strings.HasPrefix(target, "http") {
		target = watchURL(target)
	}
	out, err := y.run(ctx, "--no-playlist", "--no-warnings",
		"--print", "%(title)s", target)
	if err != nil {
		return "", err
	}
	title, _, _ := strings.Cut(out, "\n")
	return strings.TrimSpace(title), nil
}

// NormalizeYouTubeID implements playlist.TitleProvider (pure string logic).
func (y *YTDLP) NormalizeYouTubeID(raw string) (string, bool) {
	return y.norm.NormalizeYouTubeID(raw)
}

func watchURL(videoID string) string {
	return "https://www.youtube.com/watch?v=" + videoID
}

// Available reports whether the configured yt-dlp binary can be found.
func (y *YTDLP) Available() error {
	_, err := exec.LookPath(y.bin())
	return err
}
