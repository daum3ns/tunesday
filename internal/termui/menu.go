package termui

import (
    "context"
    "fmt"

    "atomicgo.dev/keyboard"
    "atomicgo.dev/keyboard/keys"
)

// ShowMenu returns the chosen index or -1 when the user pressed Ctrl-C and -2 on Esc.
func ShowMenu(ctx context.Context, title string, items []string) int {
    selected := 0
    finished := make(chan int, 1)

    // first draw
    ClearScreen()
    PrintTunesdayHeader()
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

    _ = keyboard.Listen(func(key keys.Key) (bool, error) {
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

        // redraw
        ClearScreen()
        PrintTunesdayHeader()
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

    select {
    case <-ctx.Done():
        return -1
    case idx := <-finished:
        return idx
    }
}
