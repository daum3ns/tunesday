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

	names := append([]string(nil), active...)
	sort.Strings(names)
	winner := names[rand.Intn(len(names))]

	dur := time.Duration(1500+rand.Intn(1501)) * time.Millisecond
	endAt := time.Now().Add(dur)
	for time.Now().Before(endAt) {
		ClearScreen()
		PrintTunesdayHeader()
		fmt.Println("Selecting today's provider…")
		hi := rand.Intn(len(names))
		drawNameList(names, hi)
		time.Sleep(time.Duration(40+rand.Intn(61)) * time.Millisecond)
	}

	ClearScreen()
	PrintTunesdayHeader()
	fmt.Println("Selecting today's provider…")
	winnerIdx := 0
	for i, n := range names {
		if n == winner {
			winnerIdx = i
			break
		}
	}
	drawNameList(names, winnerIdx)
	time.Sleep(1200 * time.Millisecond)

	ClearScreen()
	PrintTunesdayHeader()
	DrawBigWinner(winner)
	if data.Participants != nil {
		data.Participants[winner]++
	}
	return winner
}

// removed: RemoveYouTubeTracker moved to playlist.StripTrackingParams

func AddTuneWithProvider(ctx context.Context, data *core.Data, scanner *bufio.Scanner, providerName string, yt playlist.TitleProvider) {
	ClearScreen()
	PrintTunesdayHeader()
	fmt.Printf("Today's tune provider is: %s\n\n", providerName)
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
	t := core.Tune{Name: title, Link: raw, ID: id, Provider: "youtube", AddedAt: time.Now()}
	data.Tunes = append(data.Tunes, t)
	fmt.Println("Added:", title)
}

func AddTune(data *core.Data, scanner *bufio.Scanner) {
	ClearScreen()
	PrintTunesdayHeader()
	fmt.Println("Manually add a tune to list")
	fmt.Println("Paste the link (any), and the title shown in the list will be the URL host/path.")
	fmt.Print("Link: ")
	if !scanner.Scan() {
		return
	}
	link := strings.TrimSpace(scanner.Text())
	if link == "" {
		return
	}
	// keep only minimal info (no auto title)
	t := core.Tune{Link: link, Provider: "manual", AddedAt: time.Now()}
	data.Tunes = append(data.Tunes, t)
	fmt.Println("Added.")
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
			sel := ShowMenu(ctx, "Select participant to toggle activation", names)
			switch sel {
			case -1:
				fmt.Println("Goodbye!")
				os.Exit(0)
			case -2:
				continue
			}
			name := names[sel]
			if data.Disabled == nil {
				data.Disabled = make(map[string]bool)
			}
			data.Disabled[name] = !data.Disabled[name]
			if data.Disabled[name] {
				fmt.Printf("%s deactivated.\n", name)
			} else {
				fmt.Printf("%s activated.\n", name)
			}
			PressEnterToContinue()
		case 4, -2:
			return
		}
	}
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
