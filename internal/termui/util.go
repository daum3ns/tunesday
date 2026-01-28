package termui

import (
    "bufio"
    "fmt"
    "net/url"
    "os"
    "strconv"
    "strings"
)

func ClearScreen() {
    fmt.Print("\x1b[H\x1b[2J")
    os.Stdout.Sync()
}

func PressEnterToContinue() {
    fmt.Print("\nPress Enter to continue...")
    bufio.NewScanner(os.Stdin).Scan()
}

func HideCursor() { fmt.Print("\x1b[?25l") }
func ShowCursor() { fmt.Print("\x1b[?25h") }

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

func DrawBigWinner(name string) {
    w := termWidth()
    inner := w - 8
    if inner < 20 { inner = 20 }
    display := name + " is today's tune provider!!"
    if len([]rune(display)) > inner-2 {
        display = TruncateRunes(display, inner-2)
    }
    border := centerText(w, strings.Repeat("█", inner))
    padLine := centerText(w, "█"+strings.Repeat(" ", inner-2)+"█")
    text := centerText(w, "█   "+display+strings.Repeat(" ", inner-4-len([]rune(display)))+"█")
    fmt.Println("\x1b[1m\x1b[7m" + border + "\x1b[0m")
    fmt.Println("\x1b[1m\x1b[7m" + padLine + "\x1b[0m")
    fmt.Println("\x1b[1m\x1b[7m" + text + "\x1b[0m")
    fmt.Println("\x1b[1m\x1b[7m" + padLine + "\x1b[0m")
    fmt.Println("\x1b[1m\x1b[7m" + border + "\x1b[0m")
}

func drawNameList(names []string, highlight int) {
    for i, n := range names {
        cursor := "  "
        line := n
        if i == highlight {
            cursor = "▶ "
            line = "\x1b[1m\x1b[36m" + n + "\x1b[0m"
        }
        fmt.Printf("%s%s\n", cursor, line)
    }
}

func TruncateRunes(s string, max int) string {
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

func PadRight(s string, width int) string {
    r := []rune(s)
    if len(r) >= width {
        return s
    }
    return s + strings.Repeat(" ", width-len(r))
}

// linkDisplay returns a concise representation of the link for table view.
// For valid YouTube https links, it shows youtu.be/{id}. Otherwise host+path.
func linkDisplay(link string) string {
    if link == "" { return "" }
    if strings.HasPrefix(link, "https://www.youtube.") || strings.HasPrefix(link, "https://youtu.be/") || strings.Contains(link, "youtube.com") {
        if i := strings.Index(link, "watch?v="); i != -1 {
            id := link[i+8:]
            if j := strings.IndexAny(id, "&#?"); j != -1 { id = id[:j] }
            if id != "" { return "youtu.be/" + id }
        }
        if i := strings.Index(link, "/shorts/"); i != -1 {
            id := link[i+8:]
            if j := strings.Index(id, "/"); j != -1 { id = id[:j] }
            if id != "" { return "youtu.be/" + id }
        }
        if strings.HasPrefix(link, "https://youtu.be/") {
            id := strings.TrimPrefix(link, "https://youtu.be/")
            if j := strings.Index(id, "?"); j != -1 { id = id[:j] }
            if id != "" { return "youtu.be/" + id }
        }
    }
    u, err := url.Parse(link)
    if err != nil || u.Host == "" { return link }
    host := strings.ToLower(u.Host)
    host = strings.TrimPrefix(host, "www.")
    host = strings.TrimPrefix(host, "m.")
    path := u.EscapedPath()
    return host + path
}
