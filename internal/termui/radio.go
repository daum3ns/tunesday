package termui

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strings"

	"tunesday/internal/core"
	"tunesday/internal/playback"

	"atomicgo.dev/keyboard"
	"atomicgo.dev/keyboard/keys"
)

func RadioMenu(ctx context.Context, data *core.Data, debug ...bool) {
	debugMode := false
	if len(debug) > 0 {
		debugMode = debug[0]
	}

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
			playTunes(ctx, tunes, debugMode)
		case 1:
			playTunes(ctx, data.Tunes, debugMode)
		case 2:
			selectAndPlayTune(ctx, data.Tunes, debugMode)
		case 3:
			browseByProvider(ctx, data, debugMode)
		case 4, -1, -2:
			return
		}
	}
}

func playTunes(ctx context.Context, tunes []core.Tune, debug ...bool) {
	debugMode := false
	if len(debug) > 0 {
		debugMode = debug[0]
	}

	if len(tunes) == 0 {
		return
	}

	player, err := playback.NewPlayer(tunes, debugMode)
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

	updateNowPlaying(player)

	done := make(chan bool, 1)

	// Start player wait goroutine
	go func() {
		player.Wait()
		done <- true
	}()

	// Run keyboard listener in MAIN goroutine (not in goroutine)
	err = keyboard.Listen(func(key keys.Key) (bool, error) {
		switch key.Code {
		case keys.Space:
			player.TogglePause()
			updateNowPlaying(player)
		case keys.Left:
			// Previous tune
			player.Prev()
			updateNowPlaying(player)
		case keys.Right:
			// Next tune
			player.Next()
			updateNowPlaying(player)
		case keys.Up:
			if err := player.VolumeUp(); err != nil {
				fmt.Printf("[VOL] Error: %v\n", err)
			}
			updateNowPlaying(player)
		case keys.Down:
			if err := player.VolumeDown(); err != nil {
				fmt.Printf("[VOL] Error: %v\n", err)
			}
			updateNowPlaying(player)
		case keys.Esc:
			player.Stop()
			return true, nil
		case keys.CtrlC:
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

	player.Stop()
}

func selectAndPlayTune(ctx context.Context, tunes []core.Tune, debug ...bool) {
	debugMode := false
	if len(debug) > 0 {
		debugMode = debug[0]
	}

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

		playSingleTune(ctx, tunes[idx], debugMode)
	}
}

func playSingleTune(ctx context.Context, tune core.Tune, debug ...bool) {
	debugMode := false
	if len(debug) > 0 {
		debugMode = debug[0]
	}

	player, err := playback.NewPlayer([]core.Tune{tune}, debugMode)
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

	updateNowPlaying(player)

	done := make(chan bool, 1)

	// Start player wait goroutine
	go func() {
		player.Wait()
		done <- true
	}()

	// Run keyboard listener in MAIN goroutine (not in goroutine)
	err = keyboard.Listen(func(key keys.Key) (bool, error) {
		switch key.Code {
		case keys.Space:
			player.TogglePause()
			updateNowPlaying(player)
		case keys.Left:
			// Previous tune
			player.Prev()
			updateNowPlaying(player)
		case keys.Right:
			// Next tune
			player.Next()
			updateNowPlaying(player)
		case keys.Up:
			if err := player.VolumeUp(); err != nil {
				fmt.Printf("[VOL] Error: %v\n", err)
			}
			updateNowPlaying(player)
		case keys.Down:
			if err := player.VolumeDown(); err != nil {
				fmt.Printf("[VOL] Error: %v\n", err)
			}
			updateNowPlaying(player)
		case keys.Esc:
			player.Stop()
			return true, nil
		case keys.CtrlC:
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

	player.Stop()
}

func browseByProvider(ctx context.Context, data *core.Data, debug ...bool) {
	debugMode := false
	if len(debug) > 0 {
		debugMode = debug[0]
	}

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

		selectAndPlayTune(ctx, providerTunes, debugMode)
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
	fmt.Printf("  Provider: %s on %s \n", current.Provider, current.AddedAt.Format("2006/01/02"))
	fmt.Printf("  Volume: %d%% %s\n", player.GetVolume(), getVolumeBar(player.GetVolume()))

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
