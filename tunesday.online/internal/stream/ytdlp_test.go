package stream

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeYTDLP writes an executable stub that mimics yt-dlp for the flags we use.
// For Resolve it echoes streamURL+"|"+ext when it sees the url|ext print
// format; for anything else it returns a fake title. If TUNESDAY_TEST_ARGS_FILE
// is set, the full argument list is written there for assertions.
func fakeYTDLP(t *testing.T, streamURL, ext string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "yt-dlp")
	script := `#!/bin/sh
if [ -n "$TUNESDAY_TEST_ARGS_FILE" ]; then
	echo "$@" > "$TUNESDAY_TEST_ARGS_FILE"
fi
for arg in "$@"; do
	if [ "$arg" = "%(url)s|%(ext)s" ]; then
		echo '` + streamURL + `|` + ext + `'
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

const fixtureStream = "https://googlevideo.example/videoplayback?id=abc&expire=1893456000&source=yt"

func TestYTDLPResolve(t *testing.T) {
	y := &YTDLP{Bin: fakeYTDLP(t, fixtureStream, "m4a")}
	info, err := y.Resolve(context.Background(), "https://youtu.be/abcdefghij1")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if info.URL != fixtureStream {
		t.Fatalf("bad url %q", info.URL)
	}
	if info.MimeType != "audio/mp4" {
		t.Fatalf("mime should come from the ext line, got %q", info.MimeType)
	}
	if want := time.Unix(1893456000, 0); !info.ExpiresAt.Equal(want) {
		t.Fatalf("expire param should set ExpiresAt, got %v want %v", info.ExpiresAt, want)
	}
}

func TestYTDLPResolveMimeMapping(t *testing.T) {
	cases := []struct{ ext, want string }{
		{"mp3", "audio/mpeg"},
		{"m4a", "audio/mp4"},
		{"opus", "audio/ogg"},
		{"m3u8", "application/vnd.apple.mpegurl"},
		{"weird", "audio/mpeg"},
	}
	for _, tc := range cases {
		y := &YTDLP{Bin: fakeYTDLP(t, "https://x.example/a", tc.ext)}
		info, err := y.Resolve(context.Background(), "https://x.example/a")
		if err != nil {
			t.Fatal(err)
		}
		if info.MimeType != tc.want {
			t.Fatalf("ext %q -> mime %q, want %q", tc.ext, info.MimeType, tc.want)
		}
	}
}

func TestYTDLPFetchTitleAndNormalize(t *testing.T) {
	y := &YTDLP{Bin: fakeYTDLP(t, fixtureStream, "m4a")}

	title, err := y.FetchTitle(context.Background(), "https://youtu.be/abcdefghij1")
	if err != nil || title != "Fake Video Title" {
		t.Fatalf("FetchTitle: %q %v", title, err)
	}

	m, ok := y.Normalize("https://www.youtube.com/watch?v=abcdefghij1&si=x")
	if !ok || m.Platform != "youtube" || m.ID != "abcdefghij1" {
		t.Fatalf("Normalize youtube: %+v %v", m, ok)
	}
	m, ok = y.Normalize("https://soundcloud.com/user/track")
	if !ok || m.Platform != "soundcloud" {
		t.Fatalf("Normalize soundcloud: %+v %v", m, ok)
	}
	if _, ok := y.Normalize("https://open.spotify.com/track/abc"); ok {
		t.Fatal("spotify must be denied")
	}
}

func TestYTDLPMissingBinary(t *testing.T) {
	y := &YTDLP{Bin: "/nonexistent/yt-dlp-here"}
	if _, err := y.Resolve(context.Background(), "https://x.example/a"); err == nil {
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
	if got := mimeForExt("mp3"); got != "audio/mpeg" {
		t.Fatalf("mp3 mime: %q", got)
	}
	if got := mimeForExt("m4a"); got != "audio/mp4" {
		t.Fatalf("m4a mime: %q", got)
	}
	if got := mimeForExt("MP3"); got != "audio/mpeg" {
		t.Fatalf("mimeForExt must be case-insensitive: %q", got)
	}
	if !expireFromURL("https://x/a?expire=1893456000").Equal(time.Unix(1893456000, 0)) {
		t.Fatal("expireFromURL")
	}
	if got := expireFromURL(strings.TrimSpace("https://x/a")); !got.IsZero() {
		t.Fatalf("no expire param should be zero, got %v", got)
	}
}

func TestYTDLPResolveUsesNonHLSSelector(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	t.Setenv("TUNESDAY_TEST_ARGS_FILE", argsFile)
	y := &YTDLP{Bin: fakeYTDLP(t, "https://x.example/a", "mp3")}
	_, err := y.Resolve(context.Background(), "https://x.example/track")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	raw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read args file: %v", err)
	}
	args := string(raw)
	if !strings.Contains(args, "protocol!^=m3u8") {
		t.Fatalf("selector must exclude HLS protocols, got: %s", args)
	}
}
