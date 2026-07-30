package termui

import (
	"bufio"
	"context"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strings"
	"time"

	"tunesday/internal/core"
	"tunesday/internal/playlist"
)

func SelectProvider(ctx context.Context, data *core.Data) string {
	ClearScreen()
	PrintTunesdayHeader()

	if len(data.Participants) == 0 {
		fmt.Println("No participants available.")
		return ""
	}

	active := make([]string, 0, len(data.Participants))
	for name := range data.Participants {
		if data.Disabled != nil && data.Disabled[name] {
			continue
		}
		active = append(active, name)
	}
	if len(active) == 0 {
		fmt.Println("All participants are deactivated. Activate at least one to select a provider.")
		PressEnterToContinue()
		return ""
	}

	if ctx.Err() != nil {
		return ""
	}

	sel := core.SelectProviderFromData(data, rand.New(rand.NewSource(rand.Int63())))
	names := sel.Active
	pool := sel.Pool
	winner := sel.Winner

	indexOf := func(name string) int {
		for i, n := range names {
			if n == name {
				return i
			}
		}
		return 0
	}
	winnerIdx := indexOf(winner)

	dur := time.Duration(2500+rand.Intn(1501)) * time.Millisecond
	endAt := time.Now().Add(dur)
	for time.Now().Before(endAt) {
		if ctx.Err() != nil {
			return ""
		}
		ClearScreen()
		PrintTunesdayHeader()
		fmt.Println("Selecting today's provider...")
		hiName := pool[rand.Intn(len(pool))]
		drawNameList(names, indexOf(hiName))
		time.Sleep(time.Duration(40+rand.Intn(61)) * time.Millisecond)
	}

	ClearScreen()
	PrintTunesdayHeader()
	fmt.Println("Selecting today's provider...")
	drawNameList(names, winnerIdx)
	time.Sleep(1200 * time.Millisecond)

	ClearScreen()
	PrintTunesdayHeader()
	DrawBigWinner(winner)

	fmt.Printf("\n%s is today's provider!\n", winner)
	fmt.Printf("(pool: %d of %d active participants)\n", len(pool), len(names))

	// Keep the winner banner on screen briefly before proceeding,
	// otherwise the next screen refresh would immediately overwrite it.
	time.Sleep(2 * time.Second)
	return winner
}

// removed: RemoveYouTubeTracker moved to playlist.StripTrackingParams

func AddTuneWithProvider(ctx context.Context, data *core.Data, scanner *bufio.Scanner, providerName string, yt playlist.TitleProvider) {
	// Do not clear or redraw the header here to keep the Big Winner banner visible
	fmt.Printf("Today's tune provider is: %s\n\n", providerName)

	// Show last N tunes from this provider to help avoid duplicates
	printProviderHistory(data, providerName, 0)

	fmt.Println("Paste the tune link (YouTube https://…) or press Enter to cancel:")
	fmt.Print("> ")
	if !scanner.Scan() {
		return
	}
	raw := strings.TrimSpace(scanner.Text())
	if raw == "" {
		return
	}
	raw = playlist.StripTrackingParams(raw)

	id, ok := yt.NormalizeYouTubeID(raw)
	if !ok {
		fmt.Println("Only https:// YouTube links are supported for automatic title fetch.")
		return
	}
	title, err := yt.FetchTitle(ctx, id)
	if err != nil {
		fmt.Println("Failed to fetch title:", err)
		return
	}
	t := core.Tune{Name: title, Link: raw, ID: id, Provider: providerName, AddedAt: time.Now()}
	data.Tunes = append(data.Tunes, t)
	if data.Participants != nil {
		data.Participants[providerName]++
	}
	fmt.Println("Added:", title)
}

func ManuallyAddTune(ctx context.Context, data *core.Data, scanner *bufio.Scanner, yt playlist.TitleProvider) {
	ClearScreen()
	PrintTunesdayHeader()
	fmt.Println("Manually add a tune to list")

	// First, ask which participant to add the tune for
	if len(data.Participants) == 0 {
		fmt.Println("No participants available. Please add participants first.")
		PressEnterToContinue()
		return
	}

	names := make([]string, 0, len(data.Participants))
	for n := range data.Participants {
		names = append(names, n)
	}
	sort.Strings(names)

	sel := ShowMenu(ctx, "Select participant to add tune for", names)
	switch sel {
	case -1:
		fmt.Println("Goodbye!")
		os.Exit(0)
	case -2:
		return
	}
	participantName := names[sel]

	ClearScreen()
	PrintTunesdayHeader()
	fmt.Printf("Adding tune for: %s\n\n", participantName)
	fmt.Println("Paste the link (any URL), for YouTube links we'll try to fetch the title.")
	fmt.Print("Link: ")
	if !scanner.Scan() {
		return
	}
	link := strings.TrimSpace(scanner.Text())
	if link == "" {
		return
	}

	// Try to fetch title if it's a YouTube link
	var title string
	var id string
	if yt != nil {
		id, _ = yt.NormalizeYouTubeID(link)
		if id != "" {
			fetchedTitle, err := yt.FetchTitle(ctx, id)
			if err == nil {
				title = fetchedTitle
			} else {
				fmt.Println("Failed to fetch title:", err)
				fmt.Print("Enter title manually (or press Enter to use URL): ")
				if scanner.Scan() {
					manualTitle := strings.TrimSpace(scanner.Text())
					if manualTitle != "" {
						title = manualTitle
					}
				}
			}
		}
	}

	// Fallback: if no title, use URL display
	if title == "" {
		title = linkDisplay(link)
	}

	t := core.Tune{Name: title, Link: link, ID: id, Provider: participantName, AddedAt: time.Now()}
	data.Tunes = append(data.Tunes, t)

	// Increment participant's tune count
	if data.Participants != nil {
		data.Participants[participantName]++
	}

	fmt.Println("Added.")
	PressEnterToContinue()
}

func ListTunes(data *core.Data, scanner *bufio.Scanner) {
	ClearScreen()
	PrintTunesdayHeader()
	fmt.Println("Get complete list of tunes")
	fmt.Println("Total tunes:", len(data.Tunes))
	fmt.Println("")

	// columns
	w := termWidth()
	nameW := 52
	linkW := 26
	dateW := 16
	if w < 90 {
		// shrink proportionally
		nameW = 36
		linkW = 20
		dateW = 12
	}

	fmt.Println(PadRight("Title", nameW) + "  " + PadRight("Link", linkW) + "  Date")
	fmt.Println(strings.Repeat("-", nameW+linkW+dateW+4))
	for _, t := range data.Tunes {
		title := t.Name
		if title == "" {
			title = linkDisplay(t.Link)
		}
		title = TruncateRunes(title, nameW)
		link := TruncateRunes(linkDisplay(t.Link), linkW)
		date := t.AddedAt.Format("2006-01-02")
		if t.AddedAt.IsZero() {
			date = ""
		}
		fmt.Println(PadRight(title, nameW) + "  " + PadRight(link, linkW) + "  " + date)
	}
}

func ManageParticipants(ctx context.Context, data *core.Data, scanner *bufio.Scanner) {
	for {
		idx := ShowMenu(ctx, "Manage Tunesday participants", []string{
			"Add",
			"Remove",
			"List",
			"Activate/Deactivate",
			"Back",
		})
		switch idx {
		case -1:
			fmt.Println("Goodbye!")
			os.Exit(0)
		case 0: // Add
			fmt.Print("Enter participant name: ")
			if !scanner.Scan() {
				continue
			}
			name := strings.TrimSpace(scanner.Text())
			if name == "" {
				continue
			}
			if data.Participants == nil {
				data.Participants = map[string]int{}
			}
			if _, exists := data.Participants[name]; exists {
				fmt.Println("Participant already exists.")
			} else {
				data.Participants[name] = 0
				fmt.Println("Participant added.")
			}
			PressEnterToContinue()
		case 1: // Remove
			if len(data.Participants) == 0 {
				fmt.Println("No participants.")
				PressEnterToContinue()
				continue
			}
			names := make([]string, 0, len(data.Participants))
			for n := range data.Participants {
				names = append(names, n)
			}
			sort.Strings(names)
			sel := ShowMenu(ctx, "Select participant to remove", names)
			switch sel {
			case -1:
				fmt.Println("Goodbye!")
				os.Exit(0)
			case -2:
				continue
			}
			delete(data.Participants, names[sel])
			if data.Disabled != nil {
				delete(data.Disabled, names[sel])
			}
			// remove their tunes
			newTunes := data.Tunes[:0]
			for _, t := range data.Tunes {
				if t.Provider != names[sel] {
					newTunes = append(newTunes, t)
				}
			}
			data.Tunes = newTunes
			fmt.Println("Removed.")
			PressEnterToContinue()
		case 2: // List
			if len(data.Participants) == 0 {
				fmt.Println("No participants.")
			} else {
				ClearScreen()
				PrintTunesdayHeader()
				fmt.Println("Participants:")
				names := make([]string, 0, len(data.Participants))
				for n := range data.Participants {
					names = append(names, n)
				}
				sort.Strings(names)
				for _, name := range names {
					count := data.Participants[name]
					status := "active"
					if data.Disabled != nil && data.Disabled[name] {
						status = "deactivated"
					}
					fmt.Printf("  %s  (tunes: %d, %s)\n", name, count, status)
				}
			}
			PressEnterToContinue()
		case 3: // Activate/Deactivate
			if len(data.Participants) == 0 {
				fmt.Println("No participants.")
				PressEnterToContinue()
				continue
			}
			names := make([]string, 0, len(data.Participants))
			for n := range data.Participants {
				names = append(names, n)
			}
			sort.Strings(names)
			checked := make([]bool, len(names))
			for i, n := range names {
				checked[i] = data.Disabled == nil || !data.Disabled[n]
			}
			result := ShowMultiSelect(ctx, "Toggle activation (Space: toggle, Enter: confirm, Esc: cancel)", names, checked)
			if result == nil {
				continue
			}
			if data.Disabled == nil {
				data.Disabled = make(map[string]bool)
			}
			toggled := 0
			for i, n := range names {
				if !result[i] {
					data.Disabled[n] = true
					toggled++
				} else {
					delete(data.Disabled, n)
					toggled++
				}
			}
			if toggled > 0 {
				fmt.Println("Activation updated.")
			}
			PressEnterToContinue()
		case 4, -2:
			return
		}
	}
}

func printProviderHistory(data *core.Data, provider string, n int) {
	providerTunes := make([]core.Tune, 0)
	for _, t := range data.Tunes {
		if t.Provider == provider {
			providerTunes = append(providerTunes, t)
		}
	}
	if len(providerTunes) == 0 {
		return
	}

	// Take last N, most recent first; n <= 0 means show all
	if n > 0 && n < len(providerTunes) {
		providerTunes = providerTunes[len(providerTunes)-n:]
	}
	fmt.Println(fmt.Sprintf("%s's previous tunes:", provider))
	for i := len(providerTunes) - 1; i >= 0; i-- {
		t := providerTunes[i]
		title := t.Name
		if title == "" {
			title = linkDisplay(t.Link)
		}
		title = TruncateRunes(title, 55)
		date := ""
		if !t.AddedAt.IsZero() {
			date = t.AddedAt.Format("2006-01-02")
		}
		if date != "" {
			fmt.Printf("  %s (%s)\n", title, date)
		} else {
			fmt.Printf("  %s\n", title)
		}
	}
	fmt.Println()
}

func PrintYouTubePlaylistLink(data *core.Data) {
	ClearScreen()
	PrintTunesdayHeader()
	if len(data.Tunes) == 0 {
		fmt.Println("No tunes yet.")
		return
	}
	ids := make([]string, 0, len(data.Tunes))
	for _, t := range data.Tunes {
		if t.ID != "" {
			ids = append(ids, t.ID)
		}
	}
	if len(ids) == 0 {
		fmt.Println("No valid YouTube video IDs found to build a playlist (no tunes with titles).")
		return
	}
	link := "https://www.youtube.com/watch_videos?video_ids=" + strings.Join(ids, ",")
	fmt.Println("Get youtube playlist link")
	fmt.Println("")
	fmt.Println(link)
	fmt.Println("")
	fmt.Println("Links for pasting: (https://www.terrific.tools/youtube/playlist-generator)")
	fmt.Println("")
	for _, t := range data.Tunes {
		fmt.Println(t.Link)
	}
}
