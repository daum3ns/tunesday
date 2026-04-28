package playback

import (
	"testing"
	"tunesday/internal/core"
)

func TestNewPlayerDebugMode(t *testing.T) {
	tunes := []core.Tune{
		{Name: "Test Tune", Link: "https://www.youtube.com/watch?v=test123", Provider: "Test"},
	}

	// Test with debug mode on
	player, err := NewPlayer(tunes, true)
	if err != nil {
		t.Fatalf("NewPlayer with debug failed: %v", err)
	}
	if player == nil {
		t.Fatal("Player is nil")
	}
	if !player.debug {
		t.Error("Debug mode should be true")
	}

	// Test Start in debug mode (should not actually start mpv)
	err = player.Start()
	if err != nil {
		t.Errorf("Start in debug mode failed: %v", err)
	}
	// In debug mode, Start should return nil without actually starting mpv
}

func TestNewPlayerNormalMode(t *testing.T) {
	tunes := []core.Tune{
		{Name: "Test Tune", Link: "https://www.youtube.com/watch?v=test123", Provider: "Test"},
	}

	// Test with debug mode off (default)
	player, err := NewPlayer(tunes)
	if err != nil {
		t.Fatalf("NewPlayer failed: %v", err)
	}
	if player.debug {
		t.Error("Debug mode should be false by default")
	}
}
