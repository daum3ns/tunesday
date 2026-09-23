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
// web path inherits yt-dlp's ongoing maintenance against upstream changes for
// every supported platform (YouTube, SoundCloud, Bandcamp).
//
// It implements both stream.Resolver and playlist.TitleProvider.
type YTDLP struct {
	// Bin is the yt-dlp executable; empty consults
	// TUNESDAY_ONLINE_YTDLP_PATH and falls back to "yt-dlp".
	Bin string
	// Timeout bounds each yt-dlp invocation.
	Timeout time.Duration
}

// NewYTDLP builds the default extractor.
func NewYTDLP() *YTDLP {
	return &YTDLP{Timeout: 30 * time.Second}
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

// audioFormat prefers progressive (non-HLS) streams because browsers cannot
// decode application/vnd.apple.mpegurl. The !^=m3u8 clauses exclude
// m3u8/m3u8_native/hls protocols; [acodec=aac] keeps Bandcamp's aac-hi ahead
// of its ALAC-only (falac) m4a, and the [acodec=mp3] fallback covers
// SoundCloud, whose AAC is HLS-only.
const audioFormat = "bestaudio[protocol!^=m3u8][acodec=aac]" +
	"/bestaudio[protocol!^=m3u8][ext=m4a][acodec!=alac]" +
	"/bestaudio[protocol!^=m3u8][acodec=mp3]" +
	"/bestaudio/best"

// Resolve returns a direct audio stream URL for the canonical link.
func (y *YTDLP) Resolve(ctx context.Context, target string) (Info, error) {
	out, err := y.run(ctx, "--no-playlist", "--no-warnings",
		"-f", audioFormat, "--print", "%(url)s|%(ext)s", target)
	if err != nil {
		return Info{}, err
	}
	urlStr, ext, _ := strings.Cut(strings.TrimSpace(out), "|")
	if urlStr == "" {
		return Info{}, fmt.Errorf("yt-dlp: empty stream url")
	}
	return Info{
		URL:       urlStr,
		MimeType:  mimeForExt(ext),
		ExpiresAt: expireFromURL(urlStr),
	}, nil
}

// FetchTitle implements playlist.TitleProvider. The argument is always a
// canonical link (the caller's responsibility).
func (y *YTDLP) FetchTitle(ctx context.Context, link string) (string, error) {
	out, err := y.run(ctx, "--no-playlist", "--no-warnings",
		"--print", "%(title)s", strings.TrimSpace(link))
	if err != nil {
		return "", err
	}
	title, _, _ := strings.Cut(out, "\n")
	return strings.TrimSpace(title), nil
}

// Normalize implements playlist.TitleProvider (pure string logic).
func (y *YTDLP) Normalize(raw string) (playlist.Media, bool) {
	return playlist.Normalize(raw)
}

// Available reports whether the configured yt-dlp binary can be found.
func (y *YTDLP) Available() error {
	_, err := exec.LookPath(y.bin())
	return err
}
