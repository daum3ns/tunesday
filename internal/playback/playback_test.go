package playback

import (
	"testing"
	"tunesday/internal/core"
)

func TestNewPlayer(t *testing.T) {
	tunes := []core.Tune{
		{Name: "Test Tune", Link: "https://www.youtube.com/watch?v=test123", Provider: "Test"},
	}

	player, err := NewPlayer(tunes)
	if err != nil {
		t.Fatalf("NewPlayer failed: %v", err)
	}
	if player == nil {
		t.Fatal("Player is nil")
	}
}
