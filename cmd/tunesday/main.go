package main

import (
    "context"
    "log"
    "os"
    "os/signal"

    "tunesday/internal/app"
    "tunesday/internal/playlist"
    "tunesday/internal/storage"
    "tunesday/internal/termui"
)

func main() {
    // quick flag parsing (minimal)
    args := os.Args[1:]
    for _, a := range args {
        if a == "--radio" {
            termui.PrintTunesdayRadioHeader()
            return
        }
    }

    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
    defer stop()

    dataFile := os.Getenv("TUNESDAY_DATA_FILE")
    if dataFile == "" {
        dataFile = "tunesday.json"
    }

    store := storage.NewFileStore(dataFile)
    yt := playlist.NewYouTube()
    application := app.New(store, yt)

    if err := application.Run(ctx, args); err != nil {
        log.Fatal(err)
    }
}
