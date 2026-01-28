package core

import (
	"encoding/json"
	"testing"
)

func TestNewDataInitializesParticipants(t *testing.T) {
	d := NewData()
	if d == nil {
		t.Fatalf("NewData returned nil")
	}
	if d.Participants == nil {
		t.Fatalf("Participants map is nil")
	}
	if len(d.Participants) != 0 {
		t.Fatalf("expected empty Participants, got %v", d.Participants)
	}
}

func TestDataJSONRoundTrip(t *testing.T) {
	d := &Data{
		Participants: map[string]int{"Ann": 2},
		Tunes: []Tune{
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
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out Data
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Participants["Ann"] != 2 {
		t.Fatalf("unexpected participants after round trip: %+v", out)
	}
	if len(out.Tunes) != 2 {
		t.Fatalf("expected 2 tunes, got %d: %+v", len(out.Tunes), out.Tunes)
	}
	if out.Tunes[0].Link != d.Tunes[0].Link || out.Tunes[0].ID != d.Tunes[0].ID || out.Tunes[0].Provider != d.Tunes[0].Provider {
		t.Fatalf("tune[0] mismatch after round trip: got %+v want %+v", out.Tunes[0], d.Tunes[0])
	}
	if out.Tunes[1].Link != d.Tunes[1].Link || out.Tunes[1].ID != d.Tunes[1].ID || out.Tunes[1].Provider != d.Tunes[1].Provider {
		t.Fatalf("tune[1] mismatch after round trip: got %+v want %+v", out.Tunes[1], d.Tunes[1])
	}
}
