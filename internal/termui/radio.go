package termui

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	"tunesday/internal/core"
	"tunesday/internal/playback"

	"atomicgo.dev/keyboard"
	"atomicgo.dev/keyboard/keys"
)

func RadioMenu(ctx context.Context, data *core.Data) {
	if len(data.Tunes) == 0 {
		fmt.Println("No tunes available. Add some tunes first!")
		PressEnterToContinue()
		return
	}

	for {
		ClearScreen()
		PrintTunesdayRadioHeader()
		fmt.Println("")

		idx := ShowMenu(ctx, "Tunesday Radio", []string{
			"Play all tunes (shuffled)",
			"Play all tunes (in order)",
			"Select a tune to play",
			"Browse by provider",
			"Exit radio mode",
		}, PrintTunesdayRadioHeader)

		switch idx {
		case 0:
			tunes := make([]core.Tune, len(data.Tunes))
			copy(tunes, data.Tunes)
			rand.Shuffle(len(tunes), func(i, j int) {
				tunes[i], tunes[j] = tunes[j], tunes[i]
			})
			playTunes(ctx, tunes)
		case 1:
			playTunes(ctx, data.Tunes)
		case 2:
			selectAndPlayTune(ctx, data.Tunes)
		case 3:
			browseByProvider(ctx, data)
		case 4, -1, -2:
			return
		}
	}
}

func playTunes(ctx context.Context, tunes []core.Tune) {
	if len(tunes) == 0 {
		return
	}

	player, err := playback.NewPlayer(tunes)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		PressEnterToContinue()
		return
	}

	if err := player.Start(); err != nil {
		fmt.Printf("Playback error: %v\n", err)
		PressEnterToContinue()
		return
	}

	uiUpdateChan := make(chan struct{}, 1)
	player.SetOnTrackChange(func(idx int) {
		select {
		case uiUpdateChan <- struct{}{}:
		default:
		}
	})
	player.SetOnUpdate(func() {
		select {
		case uiUpdateChan <- struct{}{}:
		default:
		}
	})

	updateNowPlaying(player)
	player.StartPolling(500 * time.Millisecond)

	done := make(chan bool, 1)

	// Start player wait goroutine
	go func() {
		player.Wait()
		done <- true
	}()

	// Start UI update goroutine
	go func() {
		for {
			select {
			case <-uiUpdateChan:
				updateNowPlaying(player)
			case <-done:
				return
			}
		}
	}()

	// Run keyboard listener in MAIN goroutine (not in goroutine)
	err = keyboard.Listen(func(key keys.Key) (bool, error) {
		switch key.Code {
		case keys.Space:
			player.TogglePause()
			player.TriggerUpdate()
		case keys.Left:
			// Previous tune
			player.Prev()
			player.TriggerUpdate()
		case keys.Right:
			// Next tune
			player.Next()
			player.TriggerUpdate()
		case keys.Up:
			if err := player.VolumeUp(); err != nil {
				fmt.Printf("[VOL] Error: %v\n", err)
			}
			player.TriggerUpdate()
		case keys.Down:
			if err := player.VolumeDown(); err != nil {
				fmt.Printf("[VOL] Error: %v\n", err)
			}
			player.TriggerUpdate()
		case keys.Esc:
			player.StopPolling()
			player.Stop()
			return true, nil
		case keys.CtrlC:
			player.StopPolling()
			player.Stop()
			os.Exit(0)
		}

		// Check if player finished
		select {
		case <-done:
			return true, nil
		default:
			return false, nil
		}
	})

	if err != nil {
		fmt.Printf("[KEYBOARD ERROR] %v\n", err)
	}

	player.StopPolling()
	player.Stop()
}

func selectAndPlayTune(ctx context.Context, tunes []core.Tune) {
	if len(tunes) == 0 {
		return
	}

	for {
		ClearScreen()
		fmt.Println("Select a tune to play:")
		fmt.Println("")

		items := make([]string, len(tunes))
		for i, t := range tunes {
			title := t.Name
			if title == "" {
				title = tuneDisplayName(t)
			}
			items[i] = fmt.Sprintf("%s - %s", title, t.Provider)
		}

		idx := ShowMenu(ctx, "Select Tune", items, PrintTunesdayRadioHeader)
		if idx < 0 {
			return
		}

		playSingleTune(ctx, tunes[idx])
	}
}

func playSingleTune(ctx context.Context, tune core.Tune) {
	player, err := playback.NewPlayer([]core.Tune{tune})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		PressEnterToContinue()
		return
	}

	if err := player.Start(); err != nil {
		fmt.Printf("Playback error: %v\n", err)
		PressEnterToContinue()
		return
	}

	uiUpdateChan := make(chan struct{}, 1)
	player.SetOnTrackChange(func(idx int) {
		select {
		case uiUpdateChan <- struct{}{}:
		default:
		}
	})
	player.SetOnUpdate(func() {
		select {
		case uiUpdateChan <- struct{}{}:
		default:
		}
	})

	updateNowPlaying(player)
	player.StartPolling(500 * time.Millisecond)

	done := make(chan bool, 1)

	// Start player wait goroutine
	go func() {
		player.Wait()
		done <- true
	}()

	// Start UI update goroutine
	go func() {
		for {
			select {
			case <-uiUpdateChan:
				updateNowPlaying(player)
			case <-done:
				return
			}
		}
	}()

	// Run keyboard listener in MAIN goroutine (not in goroutine)
	err = keyboard.Listen(func(key keys.Key) (bool, error) {
		switch key.Code {
		case keys.Space:
			player.TogglePause()
			player.TriggerUpdate()
		case keys.Left:
			// Previous tune
			player.Prev()
			player.TriggerUpdate()
		case keys.Right:
			// Next tune
			player.Next()
			player.TriggerUpdate()
		case keys.Up:
			if err := player.VolumeUp(); err != nil {
				fmt.Printf("[VOL] Error: %v\n", err)
			}
			player.TriggerUpdate()
		case keys.Down:
			if err := player.VolumeDown(); err != nil {
				fmt.Printf("[VOL] Error: %v\n", err)
			}
			player.TriggerUpdate()
		case keys.Esc:
			player.StopPolling()
			player.Stop()
			return true, nil
		case keys.CtrlC:
			player.StopPolling()
			player.Stop()
			os.Exit(0)
		}

		// Check if player finished
		select {
		case <-done:
			return true, nil
		default:
			return false, nil
		}
	})

	if err != nil {
		fmt.Printf("[KEYBOARD ERROR] %v\n", err)
	}

	player.StopPolling()
	player.Stop()
}

func browseByProvider(ctx context.Context, data *core.Data) {
	providers := make([]string, 0, len(data.Participants))
	for p := range data.Participants {
		providers = append(providers, p)
	}

	for {
		ClearScreen()
		fmt.Println("Select provider:")
		fmt.Println("")

		idx := ShowMenu(ctx, "Browse by Provider", providers, PrintTunesdayRadioHeader)
		if idx < 0 {
			return
		}

		provider := providers[idx]
		providerTunes := make([]core.Tune, 0)
		for _, t := range data.Tunes {
			if t.Provider == provider {
				providerTunes = append(providerTunes, t)
			}
		}

		if len(providerTunes) == 0 {
			fmt.Println("No tunes from this provider.")
			PressEnterToContinue()
			continue
		}

		selectAndPlayTune(ctx, providerTunes)
	}
}

func updateNowPlaying(player *playback.Player) {
	tunes := player.Tunes()
	idx := player.CurrentIndex()

	if idx < 0 || idx >= len(tunes) {
		return
	}

	ClearScreen()
	PrintTunesdayRadioHeader()
	fmt.Println("")

	fmt.Println("Now Playing:")
	fmt.Println("")

	current := tunes[idx]
	title := current.Name
	if title == "" {
		title = tuneDisplayName(current)
	}

	fmt.Printf("  Title: %s\n", title)

	// Display playback position directly below title, blinking when paused
	pos := player.GetCachedTimePos()
	dur := player.GetCachedDuration()
	if dur > 0 {
		timeStr := fmt.Sprintf("%s / %s ", formatTime(pos), formatTime(dur))
		// Calculate available width for progress bar
		targetWidth := 65
		barWidth := targetWidth - len(timeStr) - 2
		if barWidth < 10 {
			barWidth = 10
		}
		progress := int((pos / dur) * float64(barWidth))
		if progress > barWidth {
			progress = barWidth
		}
		if progress < 0 {
			progress = 0
		}
		bar := "[" + strings.Repeat(">", progress) + strings.Repeat("-", barWidth-progress) + "]"

		fullLine := "  " + timeStr + bar
		if player.IsPaused() {
			fmt.Printf("\x1b[5m%s\x1b[0m\n", fullLine)
		} else {
			fmt.Println(fullLine)
		}
	}

	fmt.Printf("  Provider: %s on %s \n", current.Provider, current.AddedAt.Format("2006/01/02"))
	fmt.Printf("  Volume: %d%% %s\n", player.GetVolume(), getVolumeBar(player.GetVolume()))
	fmt.Println("")

	if idx+1 < len(tunes) {
		fmt.Printf("  Tune %d of %d\n", idx+1, len(tunes))
		next := tunes[idx+1]
		nextTitle := next.Name
		if nextTitle == "" {
			nextTitle = tuneDisplayName(next)
		}
		fmt.Printf("  Next: %s\n", nextTitle)
	}

	fmt.Println("")
	fmt.Println("  [Space] Pause/Play    [↑/↓] Volume    [←/→] Songs")
	fmt.Println("  [Esc] Quit to Menu    [Ctrl+C] Quit App")

	// Display playback history
	history := player.GetHistory()
	if len(history) > 0 {
		fmt.Println("")
		fmt.Println("  Played:")
		for _, t := range history {
			title := t.Name
			if title == "" {
				title = tuneDisplayName(t)
			}
			fmt.Printf("  - %s (%s)\n", title, t.Provider)
		}
	}
}

func formatTime(seconds float64) string {
	minutes := int(seconds) / 60
	secs := int(seconds) % 60
	return fmt.Sprintf("%d:%02d", minutes, secs)
}

func tuneDisplayName(t core.Tune) string {
	if t.Link == "" {
		return "Unknown"
	}
	return linkDisplay(t.Link)
}

// getVolumeBar returns a simple volume bar using ASCII characters
func getVolumeBar(vol int) string {
	barWidth := 20
	filled := (vol * barWidth) / 100
	return "[" + strings.Repeat("#", filled) + strings.Repeat("-", barWidth-filled) + "]"
}
