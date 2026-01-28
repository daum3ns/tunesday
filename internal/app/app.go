package app

import (
    "bufio"
    "context"
    "fmt"
    "math/rand"
    "os"
    "os/signal"
    "time"

    "tunesday/internal/playlist"
    "tunesday/internal/storage"
    "tunesday/internal/termui"
)

type App struct {
    store storage.Store
    yt    playlist.TitleProvider
}

func New(store storage.Store, yt playlist.TitleProvider) *App {
    return &App{store: store, yt: yt}
}

func (a *App) Run(ctx context.Context, args []string) error {
    skipTuesdayCheck := false
    for _, arg := range args {
        if arg == "--force-tunesday" {
            skipTuesdayCheck = true
        }
    }

    if !skipTuesdayCheck && time.Now().Weekday() != time.Tuesday {
        termui.PrintNotTunesdayHeader()
        return nil
    }

    rand.Seed(time.Now().UnixNano())

    data, err := a.store.Load(ctx)
    if err != nil {
        return err
    }

    scanner := bufio.NewScanner(os.Stdin)

    termui.HideCursor()
    defer termui.ShowCursor()

    // single signal handler to save on interrupt
    sigC := make(chan os.Signal, 1)
    signal.Notify(sigC, os.Interrupt)
    go func() {
        <-sigC
        _ = a.store.Save(context.Background(), data)
        os.Exit(0)
    }()

    for {
        idx := termui.ShowMenu(ctx, "Tunesday Menu", []string{
            "Select todays tune provider",
            "Manually add a tune to list",
            "Get complete list of tunes",
            "Manage Tunesday participants",
            "Get youtube playlist link",
            "Exit",
        })

        switch idx {
        case -1, -2, 5: // Exit
            _ = a.store.Save(ctx, data)
            fmt.Println("Goodbye!")
            return nil
        case 0: // Select provider
            selected := termui.SelectProvider(ctx, data)
            if selected != "" {
                termui.AddTuneWithProvider(ctx, data, scanner, selected, a.yt)
                termui.PressEnterToContinue()
            }
        case 1: // Add tune
            termui.AddTune(data, scanner)
        case 2: // List tunes
            termui.ListTunes(data, scanner)
            termui.PressEnterToContinue()
        case 3: // Manage participants
            termui.ManageParticipants(ctx, data, scanner)
        case 4: // Playlist link
            termui.PrintYouTubePlaylistLink(data)
            termui.PressEnterToContinue()
        }
        // Persist after each loop iteration
        _ = a.store.Save(ctx, data)
    }
}
