package stream

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"tunesday/internal/playlist"
)

// fakeYTDLP writes an executable stub that mimics yt-dlp for the flags we use.
func fakeYTDLP(t *testing.T, streamURL string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "yt-dlp")
	script := `#!/bin/sh
for arg in "$@"; do
	if [ "$arg" = "-g" ]; then
		echo '` + streamURL + `'
		exit 0
	fi
done
echo "Fake Video Title"
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

const fixtureStream = "https://googlevideo.example/videoplayback?id=abc&mime=audio%2Fwebm&expire=1893456000&source=yt"

func TestYTDLPResolve(t *testing.T) {
	y := &YTDLP{Bin: fakeYTDLP(t, fixtureStream)}
	info, err := y.Resolve(context.Background(), "abcdefghij1")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if info.URL != fixtureStream {
		t.Fatalf("bad url %q", info.URL)
	}
	if info.MimeType != "audio/webm" {
		t.Fatalf("mime should be parsed from the url, got %q", info.MimeType)
	}
	if want := time.Unix(1893456000, 0); !info.ExpiresAt.Equal(want) {
		t.Fatalf("expire param should set ExpiresAt, got %v want %v", info.ExpiresAt, want)
	}
}

func TestYTDLPResolveDefaults(t *testing.T) {
	y := &YTDLP{Bin: fakeYTDLP(t, "https://x.example/a?b=1\nhttps://x.example/second")}
	info, err := y.Resolve(context.Background(), "id")
	if err != nil {
		t.Fatal(err)
	}
	if info.URL != "https://x.example/a?b=1" {
		t.Fatalf("must use the first -g line, got %q", info.URL)
	}
	if info.MimeType != "audio/mp4" || !info.ExpiresAt.IsZero() {
		t.Fatalf("fallbacks wrong: %+v", info)
	}
}

func TestYTDLPFetchTitleAndNormalize(t *testing.T) {
	y := &YTDLP{Bin: fakeYTDLP(t, fixtureStream), norm: playlist.NewYouTube()}

	title, err := y.FetchTitle(context.Background(), "abcdefghij1")
	if err != nil || title != "Fake Video Title" {
		t.Fatalf("FetchTitle: %q %v", title, err)
	}
	title, err = y.FetchTitle(context.Background(), "https://youtu.be/abcdefghij1")
	if err != nil || title != "Fake Video Title" {
		t.Fatalf("FetchTitle via url: %q %v", title, err)
	}

	id, ok := y.NormalizeYouTubeID("https://www.youtube.com/watch?v=abcdefghij1&si=x")
	if !ok || id != "abcdefghij1" {
		t.Fatalf("NormalizeYouTubeID: %q %v", id, ok)
	}
}

func TestYTDLPMissingBinary(t *testing.T) {
	y := &YTDLP{Bin: "/nonexistent/yt-dlp-here"}
	if _, err := y.Resolve(context.Background(), "id"); err == nil {
		t.Fatal("expected error for missing binary")
	}
	if err := y.Available(); err == nil {
		t.Fatal("Available should report missing binary")
	}
}

func TestCachedHonoursSignedExpiry(t *testing.T) {
	base := time.Unix(1700000000, 0)
	expiring := &expiryStub{expiresAt: base.Add(10 * time.Minute)}
	c := NewCached(expiring, time.Hour, 10) // long TTL…
	now := base
	c.now = func() time.Time { return now }

	if _, err := c.Resolve(context.Background(), "v"); err != nil {
		t.Fatal(err)
	}
	now = base.Add(11 * time.Minute) // signed URL dead, cache TTL not reached
	if _, err := c.Resolve(context.Background(), "v"); err != nil {
		t.Fatal(err)
	}
	if n := expiring.calls.Load(); n != 2 {
		t.Fatalf("expired URL must refetch despite fresh TTL, got %d calls", n)
	}

	// Info.Valid gates cache hits on the signed expiry.
	expired := Info{URL: "x", ExpiresAt: base.Add(time.Minute)}
	if expired.Valid(base.Add(2 * time.Minute)) {
		t.Fatal("expired info must be invalid")
	}
	noExpiry := Info{URL: "x"}
	if !noExpiry.Valid(base.Add(time.Hour)) {
		t.Fatal("zero-expiry info should stay valid")
	}
}

type expiryStub struct {
	expiresAt time.Time
	calls     atomic.Int64
}

func (e *expiryStub) Resolve(_ context.Context, _ string) (Info, error) {
	e.calls.Add(1)
	return Info{URL: "https://x/v", MimeType: "audio/mp4", ExpiresAt: e.expiresAt}, nil
}

func TestMimeHelpers(t *testing.T) {
	if got := mimeFromURL("https://x/a?mime=audio%2Fwebm"); got != "audio/webm" {
		t.Fatalf("mimeFromURL: %q", got)
	}
	if got := mimeFromURL("https://x/a?b=1"); got != "audio/mp4" {
		t.Fatalf("mimeFromURL default: %q", got)
	}
	if !expireFromURL("https://x/a?expire=1893456000").Equal(time.Unix(1893456000, 0)) {
		t.Fatal("expireFromURL")
	}
	if got := expireFromURL(strings.TrimSpace("https://x/a")); !got.IsZero() {
		t.Fatalf("no expire param should be zero, got %v", got)
	}
}
