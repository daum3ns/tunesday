package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"time"

	"atomicgo.dev/keyboard"
	"atomicgo.dev/keyboard/keys"
	"github.com/kkdai/youtube/v2"
)

//tunesday application
// uses https://github.com/kkdai/youtube/ to fetch tune information from youtube
// TODO: streaming: https://github.com/kkdai/youtube/issues/193

const dataFile = "tunesday.json"

type Data struct {
	Participants map[string]int  `json:"participants"`       // name -> tunes count
	Disabled     map[string]bool `json:"disabled,omitempty"` // name -> true if deactivated
	Tunes        []Tune          `json:"tunes"`
}

type Tune struct {
	Name     string    `json:"name"` // video title
	Link     string    `json:"link"` // original YouTube URL
	ID       string    `json:"id"`   // normalized YouTube video ID
	Provider string    `json:"provider"`
	AddedAt  time.Time `json:"added_at,omitempty"`
}

/* ---------- terminal helpers ---------- */

func clearScreen() {
	// move cursor to 1,1 + erase entire screen + flush
	fmt.Print("\x1b[H\x1b[2J")
	// flush stdout so the terminal really processes it
	os.Stdout.Sync()
}

func pressEnterToContinue() {
	fmt.Print("\nPress Enter to continue...")
	bufio.NewScanner(os.Stdin).Scan()
}

/* ---------- cursor helpers ---------- */

func hideCursor() { fmt.Print("\x1b[?25l") }
func showCursor() { fmt.Print("\x1b[?25h") }

/* ---------- headers ---------- */

func printTunesdayHeader() {
	fmt.Println(`██████████████████████████████████████████████████████████████████████████`)
	fmt.Println(`█▌                                                                      ▐█`)
	fmt.Println(`█▌                                                                      ▐█`)
	fmt.Println(`█▌                                                                      ▐█`)
	fmt.Println(`█▌     ░▀█▀░▀█▀░▀░█▀▀░░░░░░░░░                                          ▐█`)
	fmt.Println(`█▌     ░░█░░░█░░░░▀▀█░░░░░░░░░                                          ▐█`)
	fmt.Println(`█▌     ░▀▀▀░░▀░░░░▀▀▀░▀░░▀░░▀░                                          ▐█`)
	fmt.Println(`█▌     ░█░█░█▀█░█▀█░█▀█░█░█░░░▀█▀░█░█░█▀█░█▀▀░█▀▀░█▀▄░█▀█░█░█░░░█░█     ▐█`)
	fmt.Println(`█▌     ░█▀█░█▀█░█▀▀░█▀▀░░█░░░░░█░░█░█░█░█░█▀▀░▀▀█░█░█░█▀█░░█░░░░▀░▀     ▐█`)
	fmt.Println(`█▌     ░▀░▀░▀░▀░▀░░░▀░░░░▀░░░░░▀░░▀▀▀░▀░▀░▀▀▀░▀▀▀░▀▀░░▀░▀░░▀░░░░▀░▀     ▐█`)
	fmt.Println(`█▌                                                                      ▐█`)
	fmt.Println(`█▌                                                                      ▐█`)
	fmt.Println(`█▌                                                                      ▐█`)
	fmt.Println(`██████████████████████████████████████████████████████████████████████████`)
	fmt.Println("")
}

func printNotTunesdayHeader() {
	fmt.Println(`████████████████████████████████████████████████████████████████████████████████████████`)
	fmt.Println(`█▌                                                                                    ▐█`)
	fmt.Println(`█▌                                                                                    ▐█`)
	fmt.Println(`█▌                                                                                    ▐█`)
	fmt.Println(`█▌     ░█░█░░░▀█▀░▀█▀░▀░█▀▀░░░█▀█░█▀█░▀█▀░░░▀█▀░█░█░█▀█░█▀▀░█▀▀░█▀▄░█▀█░█░█░░░█░█     ▐█`)
	fmt.Println(`█▌     ░▀░▀░░░░█░░░█░░░░▀▀█░░░█░█░█░█░░█░░░░░█░░█░█░█░█░█▀▀░▀▀█░█░█░█▀█░░█░░░░▀░▀     ▐█`)
	fmt.Println(`█▌     ░▀░▀░░░▀▀▀░░▀░░░░▀▀▀░░░▀░▀░▀▀▀░░▀░░░░░▀░░▀▀▀░▀░▀░▀▀▀░▀▀▀░▀▀░░▀░▀░░▀░░░░▀░▀     ▐█`)
	fmt.Println(`█▌                                                                                    ▐█`)
	fmt.Println(`█▌                                                                                    ▐█`)
	fmt.Println(`█▌                                                                                    ▐█`)
	fmt.Println(`████████████████████████████████████████████████████████████████████████████████████████`)
}

func printTunesdayRadioHeader() {
	fmt.Println(`█████████████████████████████████████████████████████████████████████`)
	fmt.Println(`█▌                                                                 ▐█`)
	fmt.Println(`█▌                                                                 ▐█`)
	fmt.Println(`█▌                                                                 ▐█`)
	fmt.Println(`█▌     ░▀█▀░█░█░█▀█░█▀▀░█▀▀░█▀▄░█▀█░█░█░░░░█▀▄░█▀█░█▀▄░▀█▀░█▀█     ▐█`)
	fmt.Println(`█▌     ░░█░░█░█░█░█░█▀▀░▀▀█░█░█░█▀█░░█░░░░░█▀▄░█▀█░█░█░░█░░█░█     ▐█`)
	fmt.Println(`█▌     ░░▀░░▀▀▀░▀░▀░▀▀▀░▀▀▀░▀▀░░▀░▀░░▀░░▀░░▀░▀░▀░▀░▀▀░░▀▀▀░▀▀▀     ▐█`)
	fmt.Println(`█▌                                                                 ▐█`)
	fmt.Println(`█▌                                                                 ▐█`)
	fmt.Println(`█▌                                                                 ▐█`)
	fmt.Println(`█████████████████████████████████████████████████████████████████████`)
}

/* ---------- arrow-key menu ---------- */

// showMenu returns the chosen index or -1 when the user pressed Ctrl-C and -2 on Esc.
func showMenu(title string, items []string) int {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	selected := 0
	finished := make(chan int, 1)

	// --- first draw ---
	clearScreen()
	printTunesdayHeader()
	if title != "" {
		fmt.Println(title)
	}
	for i, item := range items {
		cursor := "  "
		if i == selected {
			cursor = "▶ "
		}
		fmt.Printf("%s%s\n", cursor, item)
	}

	// --- keyboard listener ---
	_ = keyboard.Listen(func(key keys.Key) (stop bool, err error) {
		switch key.Code {
		case keys.Up:
			if selected > 0 {
				selected--
			}
		case keys.Down:
			if selected < len(items)-1 {
				selected++
			}
		case keys.Enter:
			finished <- selected
			return true, nil
		case keys.CtrlC:
			finished <- -1
			return true, nil
		case keys.Esc:
			finished <- -2
			return true, nil
		}

		// --- redraw ---
		clearScreen()
		printTunesdayHeader()
		if title != "" {
			fmt.Println(title)
		}
		for i, item := range items {
			cursor := "  "
			if i == selected {
				cursor = "▶ "
			}
			fmt.Printf("%s%s\n", cursor, item)
		}
		return false, nil
	})

	idx := <-finished
	return idx
}

/* ---------- main ---------- */

func main() {
	clearScreen()

	// Parse CLI flags
	skipTuesdayCheck := false
	for _, arg := range os.Args[1:] {
		if arg == "--radio" {
			printTunesdayRadioHeader()
			return
		}
		if arg == "--force-tunesday" {
			skipTuesdayCheck = true
		}
	}

	if !skipTuesdayCheck && time.Now().Weekday() != time.Tuesday {
		printNotTunesdayHeader()
		return
	}

	rand.Seed(time.Now().UnixNano())
	data := loadData()
	scanner := bufio.NewScanner(os.Stdin)

	hideCursor()
	defer showCursor()

	// ONE signal handler in main
	sigC := make(chan os.Signal, 1)
	signal.Notify(sigC, os.Interrupt)
	go func() {
		<-sigC
		saveData(data)
		os.Exit(0)
	}()

	for {
		idx := showMenu("Tunesday Menu", []string{
			"Select todays tune provider",
			"Manually add a tune to list",
			"Get complete list of tunes",
			"Manage Tunesday participants",
			"Get youtube playlist link",
			"Exit",
		})

		switch idx {
		case -1, -2, 5: //Exit
			saveData(data)
			fmt.Println("Goodbye!")
			return
		case 0: // Select provider
			selected := selectProvider(data)
			if selected != "" {
				addTuneWithProvider(data, scanner, selected)
				pressEnterToContinue()
			}
		case 1: // Add tune
			addTune(data, scanner)
		//	pressEnterToContinue()
		case 2: // List tunes
			listTunes(data, scanner)
			pressEnterToContinue()
		case 3: // Manage participants
			manageParticipants(data, scanner)
		case 4: // Get youtube playlist link
			printYouTubePlaylistLink(data)
			pressEnterToContinue()
		}
	}
}

/* ---------- business logic ---------- */

// isYouTubeHTTPS validates that the URL is https and points to a YouTube video.
// It returns the normalized video ID and true if valid.
func isYouTubeHTTPS(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", false
	}
	if strings.ToLower(u.Scheme) != "https" {
		return "", false
	}
	// Normalize host by trimming www. and m.
	host := strings.ToLower(u.Host)
	host = strings.TrimPrefix(host, "www.")
	host = strings.TrimPrefix(host, "m.")
	// Extract ID for common patterns
	switch host {
	case "youtube.com", "music.youtube.com":
		if u.Path == "/watch" {
			v := u.Query().Get("v")
			if v != "" {
				return v, true
			}
		}
		// Support Shorts URLs: /shorts/{id}
		if strings.HasPrefix(u.Path, "/shorts/") {
			id := strings.TrimPrefix(u.Path, "/shorts/")
			id = strings.SplitN(id, "/", 2)[0]
			if id != "" {
				return id, true
			}
		}
	case "youtu.be":
		id := strings.Trim(u.Path, "/")
		if id != "" {
			return id, true
		}
	}
	return "", false
}

func fetchYouTubeTitle(linkOrID string) (string, error) {
	client := youtube.Client{}
	v, err := client.GetVideo(linkOrID)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(v.Title), nil
}

// ---------- formatting helpers ----------
func numDigits(n int) int {
	if n <= 0 {
		return 1
	}
	d := 0
	for n > 0 {
		n /= 10
		d++
	}
	return d
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 0 {
		return ""
	}
	if max == 1 {
		return "…"
	}
	return string(r[:max-1]) + "…"
}

func padRight(s string, width int) string {
	r := []rune(s)
	if len(r) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(r))
}

// linkDisplay returns a concise representation of the link for table view.
// For valid YouTube https links, it shows youtu.be/{id}. Otherwise host+path.
func linkDisplay(link string) string {
	if link == "" {
		return ""
	}
	if id, ok := isYouTubeHTTPS(link); ok && id != "" {
		return "youtu.be/" + id
	}
	u, err := url.Parse(link)
	if err != nil || u.Host == "" {
		return link
	}
	host := strings.ToLower(u.Host)
	host = strings.TrimPrefix(host, "www.")
	host = strings.TrimPrefix(host, "m.")
	path := u.EscapedPath()
	return host + path
}

// termWidth returns terminal width from $COLUMNS when available, else 80.
func termWidth() int {
	if c := os.Getenv("COLUMNS"); c != "" {
		if n, err := strconv.Atoi(c); err == nil && n > 20 {
			return n
		}
	}
	return 80
}

func centerText(width int, s string) string {
	r := []rune(s)
	w := len(r)
	if w >= width {
		return string(r[:width])
	}
	pad := (width - w) / 2
	return strings.Repeat(" ", pad) + s
}

func drawNameList(names []string, highlight int) {
	for i, n := range names {
		cursor := "  "
		line := n
		if i == highlight {
			cursor = "▶ "
			// Bold + cyan for highlight
			line = "\x1b[1m\x1b[36m" + n + "\x1b[0m"
		}
		fmt.Printf("%s%s\n", cursor, line)
	}
}

func drawBigWinner(name string) {
	w := termWidth()
	// Leave some margin
	inner := w - 8
	if inner < 20 {
		inner = 20
	}
	display := name + " is today's tune provider!!"
	if len([]rune(display)) > inner-2 {
		display = truncateRunes(display, inner-2)
	}
	border := centerText(w, strings.Repeat("█", inner))
	padLine := centerText(w, "█"+strings.Repeat(" ", inner-2)+"█")
	text := centerText(w, "█   "+display+strings.Repeat(" ", inner-4-len([]rune(display)))+"█")

	// Bold + inverse for the block for emphasis
	fmt.Println("\x1b[1m\x1b[7m" + border + "\x1b[0m")
	fmt.Println("\x1b[1m\x1b[7m" + padLine + "\x1b[0m")
	fmt.Println("\x1b[1m\x1b[7m" + text + "\x1b[0m")
	fmt.Println("\x1b[1m\x1b[7m" + padLine + "\x1b[0m")
	fmt.Println("\x1b[1m\x1b[7m" + border + "\x1b[0m")
}

func selectProvider(data *Data) string {
	clearScreen()
	printTunesdayHeader()

	if len(data.Participants) == 0 {
		fmt.Println("No participants available.")
		return ""
	}

	// Filter only active participants
	active := make([]string, 0, len(data.Participants))
	for name := range data.Participants {
		if data.Disabled != nil && data.Disabled[name] {
			continue
		}
		active = append(active, name)
	}
	if len(active) == 0 {
		fmt.Println("All participants are deactivated. Activate at least one to select a provider.")
		pressEnterToContinue()
		return ""
	}

	// Build a stable, sorted list of active participants for the animation and selection
	names := append([]string(nil), active...)
	sort.Strings(names)

	// Uniform random winner among active participants
	winner := names[rand.Intn(len(names))]

	// Flicker animation
	dur := time.Duration(1500+rand.Intn(1501)) * time.Millisecond
	endAt := time.Now().Add(dur)

	for time.Now().Before(endAt) {
		clearScreen()
		printTunesdayHeader()
		fmt.Println("Selecting today's provider…")
		hi := rand.Intn(len(names))
		drawNameList(names, hi)
		time.Sleep(time.Duration(40+rand.Intn(61)) * time.Millisecond)
	}

	// Ensure the last highlight matches the winner, show for a short moment
	clearScreen()
	printTunesdayHeader()
	fmt.Println("Selecting today's provider…")
	winnerIdx := 0
	for i, n := range names {
		if n == winner {
			winnerIdx = i
			break
		}
	}
	drawNameList(names, winnerIdx)
	time.Sleep(350 * time.Millisecond)

	// Final big reveal
	clearScreen()
	fmt.Println("")
	fmt.Println("")
	drawBigWinner(winner)
	fmt.Println("")
	pressEnterToContinue()

	return winner
}

func removeYoutubeTracker(link string) string {
    // Remove YouTube tracking query parameters like "si" if present
    if link == "" {
        return link
    }
    u, err := url.Parse(link)
    if err != nil {
        return link
    }

    // Only attempt to modify for YouTube domains
    host := strings.ToLower(strings.TrimPrefix(u.Host, "www."))
    switch host {
    case "youtube.com", "music.youtube.com", "youtu.be":
        // proceed
    default:
        return link
    }

    q := u.Query()
    // remove known tracking parameter
    if q.Has("si") {
        q.Del("si")
    }
    // If no query params remain, clear RawQuery entirely
    if len(q) == 0 {
        u.RawQuery = ""
    } else {
        u.RawQuery = q.Encode()
    }
    return u.String()
}

func addTuneWithProvider(data *Data, scanner *bufio.Scanner, providerName string) {
	clearScreen()
	printTunesdayHeader()

	fmt.Printf("%s was selected to provide todays tune!\n", providerName)

	fmt.Print("Enter YouTube https link: ")
	scanner.Scan()
	link := strings.TrimSpace(scanner.Text())
	if link == "" {
		fmt.Println("Empty link")
		return
	}
	id, ok := isYouTubeHTTPS(link)
	if !ok {
		fmt.Println("Warning: Link must be an https YouTube video URL (e.g., https://www.youtube.com/watch?v=...).")
		return
	}
	link = removeYoutubeTracker(link)
	// Try to fetch title using kkdai/youtube
	fmt.Println("Fetching video info from YouTube…")
	title, err := fetchYouTubeTitle(link)
	if err != nil || title == "" {
		fmt.Printf("Warning: Failed to fetch video title (%v). Storing tune with empty title.\n", err)
		title = "<couldn't fetch title>"
	} else {
		fmt.Printf("Fetched title: %s\n", title)
	}
	data.Tunes = append(data.Tunes, Tune{Name: title, Link: link, ID: id, Provider: providerName, AddedAt: time.Now()})
	data.Participants[providerName]++
	fmt.Println("Tune added.")
}

func addTune(data *Data, scanner *bufio.Scanner) {
	clearScreen()
	printTunesdayHeader()

	if len(data.Participants) == 0 {
		fmt.Println("No participants available. Add one first.")
		return
	}
	names := make([]string, 0, len(data.Participants))
	for n := range data.Participants {
		names = append(names, n)
	}
	sort.Strings(names)

	sel := showMenu("Choose a participant", names)
	switch sel {
	case -1:
		saveData(data)
		fmt.Println("Goodbye!")
		os.Exit(0)
	case -2:
		// back
		return
	}
	selected := names[sel]

	fmt.Print("Enter YouTube https link: ")
	scanner.Scan()
	link := strings.TrimSpace(scanner.Text())
	if link == "" {
		fmt.Println("Empty link")
		pressEnterToContinue()
		return
	}
	id, ok := isYouTubeHTTPS(link)
	if !ok {
		fmt.Println("Warning: Link must be an https YouTube video URL (e.g., https://www.youtube.com/watch?v=...).")
		pressEnterToContinue()
		return
	}
	fmt.Println("Fetching video info from YouTube…")
	title, err := fetchYouTubeTitle(link)
	if err != nil || title == "" {
		fmt.Printf("Warning: Failed to fetch video title (%v). Storing tune with empty title.\n", err)
		title = ""
	} else {
		fmt.Printf("Fetched title: %s\n", title)
	}
	data.Tunes = append(data.Tunes, Tune{Name: title, Link: link, ID: id, Provider: selected, AddedAt: time.Now()})
	data.Participants[selected]++
	fmt.Println("Tune added.")
	pressEnterToContinue()
}

func listTunes(data *Data, scanner *bufio.Scanner) {
	clearScreen()
	printTunesdayHeader()

	if len(data.Tunes) == 0 {
		fmt.Println("No tunes yet.")
		return
	}

	// Sort tunes by AddedAt (newest first). Entries without a timestamp go last.
	tunes := append([]Tune(nil), data.Tunes...)
	sort.Slice(tunes, func(i, j int) bool {
		ti, tj := tunes[i].AddedAt, tunes[j].AddedAt
		if ti.IsZero() && tj.IsZero() {
			// Stable fallback: sort by name when both have no timestamp
			return strings.ToLower(tunes[i].Name) < strings.ToLower(tunes[j].Name)
		}
		if ti.IsZero() {
			return false // i after j
		}
		if tj.IsZero() {
			return true // i before j
		}
		return ti.After(tj)
	})

	// Compute column widths (full link should be shown, no truncation)
	idxW := numDigits(len(tunes))
	titleMax, provMax := 50, 16
	titleW := len("Title")
	provW := len("Provided by")
	linkW := len("Link")
	for _, t := range tunes {
		if w := len([]rune(t.Name)); w > titleW {
			if w > titleMax {
				titleW = titleMax
			} else {
				titleW = w
			}
		}
		if w := len([]rune(t.Provider)); w > provW {
			if w > provMax {
				provW = provMax
			} else {
				provW = w
			}
		}
		if w := len([]rune(t.Link)); w > linkW {
			linkW = w
		}
	}
	dateW := len("YYYY-MM-DD HH:MM") // 16
	if dateW < 16 {
		dateW = 16
	}

	// Header
	header := fmt.Sprintf(" %*s  %-*s  %-*s  %-*s  %-*s", idxW, "#", titleW, "Title", provW, "Provided by", dateW, "Date", linkW, "Link")
	fmt.Println(header)
	fmt.Println(strings.Repeat("-", len([]rune(header))))

	// Rows
	for i, t := range tunes {
		title := truncateRunes(t.Name, titleW)
		provider := truncateRunes(t.Provider, provW)
		date := "-"
		if !t.AddedAt.IsZero() {
			date = t.AddedAt.Format("2006-01-02 15:04")
		}
		ld := t.Link // show full link to be clickable
		fmt.Printf(" %*d  %-*s  %-*s  %-*s  %-*s\n", idxW, i+1, titleW, title, provW, provider, dateW, date, linkW, ld)
	}
}

func manageParticipants(data *Data, scanner *bufio.Scanner) {
	for {
		idx := showMenu("Manage Participants", []string{
			"Add participant",
			"Remove participant",
			"List participants",
			"Activate/Deactivate participant",
			"Back",
		})

		switch idx {
		case -1:
			saveData(data)
			fmt.Println("Goodbye!")
			os.Exit(0)
		case 0: // Add
			fmt.Print("Enter participant name: ")
			scanner.Scan()
			name := strings.TrimSpace(scanner.Text())
			if name == "" {
				fmt.Println("Empty name")
				pressEnterToContinue()
				continue
			}
			if _, ok := data.Participants[name]; ok {
				fmt.Println("Already exists")
				pressEnterToContinue()
				continue
			}
			data.Participants[name] = 0
			if data.Disabled == nil {
				data.Disabled = make(map[string]bool)
			}
			data.Disabled[name] = false
			fmt.Println("Added.")
			pressEnterToContinue()

		case 1: // Remove
			if len(data.Participants) == 0 {
				fmt.Println("No participants to remove.")
				pressEnterToContinue()
				continue
			}
			names := make([]string, 0, len(data.Participants))
			for n := range data.Participants {
				names = append(names, n)
			}
			sort.Strings(names)
			sel := showMenu("Select participant to remove", names)
			switch sel {
			case -1:
				saveData(data)
				fmt.Println("Goodbye!")
				os.Exit(0)
			case -2:
				continue
			}

			toRemove := names[sel]

			delete(data.Participants, toRemove)
			if data.Disabled != nil {
				delete(data.Disabled, toRemove)
			}
			var newTunes []Tune
			for _, t := range data.Tunes {
				if t.Provider != toRemove {
					newTunes = append(newTunes, t)
				}
			}
			data.Tunes = newTunes
			fmt.Println("Removed.")
			pressEnterToContinue()

		case 2: // List
			if len(data.Participants) == 0 {
				fmt.Println("No participants.")
			} else {
				clearScreen()
				printTunesdayHeader()
				fmt.Println("Participants:")
				// stable order
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
			pressEnterToContinue()

		case 3: // Activate/Deactivate
			if len(data.Participants) == 0 {
				fmt.Println("No participants.")
				pressEnterToContinue()
				continue
			}
			names := make([]string, 0, len(data.Participants))
			for n := range data.Participants {
				names = append(names, n)
			}
			sort.Strings(names)
			sel := showMenu("Select participant to toggle activation", names)
			switch sel {
			case -1:
				saveData(data)
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
			pressEnterToContinue()

		case 4, -2: // Back
			return
		}
	}
}

/* ---------- persistence ---------- */

func loadData() *Data {
	d := &Data{Participants: make(map[string]int)}
	b, err := ioutil.ReadFile(dataFile)
	if err != nil {
		return d
	}
	_ = json.Unmarshal(b, d)
	return d
}

func saveData(data *Data) {
	fmt.Println("saving to ", dataFile)
	b, _ := json.MarshalIndent(data, "", "  ")
	_ = ioutil.WriteFile(dataFile, b, 0644)
}

// printYouTubePlaylistLink builds a YouTube playlist link with all stored tune IDs and prints it.
func printYouTubePlaylistLink(data *Data) {
	clearScreen()
	printTunesdayHeader()

	if len(data.Tunes) == 0 {
		fmt.Println("No tunes yet.")
		return
	}

	ids := make([]string, 0, len(data.Tunes))
	for _, t := range data.Tunes {
		if t.ID == "" {
			continue // cannot add without an ID
		}
		ids = append(ids, t.ID)
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
