package termui

import (
	"context"
	"fmt"

	"atomicgo.dev/keyboard"
	"atomicgo.dev/keyboard/keys"
)

// ShowMenu returns the chosen index or -1 when the user pressed Ctrl-C and -2 on Esc.
// headerFunc is optional; if provided, it will be called to display the header.
// If omitted, PrintTunesdayHeader is used by default.
func ShowMenu(ctx context.Context, title string, items []string, headerFunc ...func()) int {
	selected := 0
	finished := make(chan int, 1)

	// Determine header function
	header := PrintTunesdayHeader
	if len(headerFunc) > 0 && headerFunc[0] != nil {
		header = headerFunc[0]
	}

	// first draw
	ClearScreen()
	header()
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
		header()
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

// ShowMultiSelect returns nil when the user cancelled (Ctrl-C or Esc),
// or a []bool slice of the same length as items with the final checked state of each item.
// Space toggles the currently selected item. Enter confirms.
func ShowMultiSelect(ctx context.Context, title string, items []string, checked []bool, headerFunc ...func()) []bool {
	selected := 0
	finished := make(chan []bool, 1)

	header := PrintTunesdayHeader
	if len(headerFunc) > 0 && headerFunc[0] != nil {
		header = headerFunc[0]
	}

	draw := func() {
		ClearScreen()
		header()
		if title != "" {
			fmt.Println(title)
		}
		for i, item := range items {
			cursor := "  "
			if i == selected {
				cursor = "▶ "
			}
			mark := "[ ]"
			if checked[i] {
				mark = "[x]"
			}
			fmt.Printf("%s%s %s\n", cursor, mark, item)
		}
	}

	draw()

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
		case keys.Space:
			checked[selected] = !checked[selected]
		case keys.Enter:
			result := make([]bool, len(checked))
			copy(result, checked)
			finished <- result
			return true, nil
		case keys.CtrlC:
			finished <- nil
			return true, nil
		case keys.Esc:
			finished <- nil
			return true, nil
		}

		draw()
		return false, nil
	})

	select {
	case <-ctx.Done():
		return nil
	case result := <-finished:
		return result
	}
}
