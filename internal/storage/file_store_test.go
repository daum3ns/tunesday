package storage

import (
    "context"
    "os"
    "path/filepath"
    "testing"

    "tunesday/internal/core"
)

func TestLoadWhenFileMissingReturnsEmptyData(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "does-not-exist.json")
    fs := NewFileStore(path)
    d, err := fs.Load(context.Background())
    if err != nil {
        t.Fatalf("Load returned error: %v", err)
    }
    if d == nil || d.Participants == nil {
        t.Fatalf("expected initialized Data, got %#v", d)
    }
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "playlist.json")
    fs := NewFileStore(path)

    in := &core.Data{
        Participants: map[string]int{"Bob": 1},
        Tunes: []core.Tune{
            {
                Name:     "Never Gonna Give You Up",
                Link:     "https://www.youtube.com/watch?v=dQw4w9WgXcQ&ab_channel=RickAstley",
                ID:       "dQw4w9WgXcQ",
                Provider: "alice",
            },
            {
                Name:     "Short Demo",
                Link:     "https://www.youtube.com/shorts/abc123DEF45",
                ID:       "abc123DEF45",
                Provider: "bob",
            },
        },
    }
    if err := fs.Save(context.Background(), in); err != nil {
        t.Fatalf("Save error: %v", err)
    }
    // ensure temp file cleaned up
    if _, err := os.Stat(path + ".tmp"); err == nil {
        t.Fatalf("temporary file still exists")
    }
    out, err := fs.Load(context.Background())
    if err != nil {
        t.Fatalf("Load error: %v", err)
    }
    if out.Participants["Bob"] != 1 {
        t.Fatalf("unexpected content after round trip: %#v", out)
    }
    if len(out.Tunes) != 2 {
        t.Fatalf("expected 2 tunes after round trip, got %d: %#v", len(out.Tunes), out.Tunes)
    }
    if out.Tunes[0].Link != in.Tunes[0].Link || out.Tunes[0].ID != in.Tunes[0].ID || out.Tunes[0].Provider != in.Tunes[0].Provider {
        t.Fatalf("tune[0] mismatch after round trip: got %+v want %+v", out.Tunes[0], in.Tunes[0])
    }
    if out.Tunes[1].Link != in.Tunes[1].Link || out.Tunes[1].ID != in.Tunes[1].ID || out.Tunes[1].Provider != in.Tunes[1].Provider {
        t.Fatalf("tune[1] mismatch after round trip: got %+v want %+v", out.Tunes[1], in.Tunes[1])
    }
}
