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
	radioMode := false
	for _, a := range args {
		if a == "--radio" {
			radioMode = true
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	dataFile := os.Getenv("TUNESDAY_DATA_FILE")
	if dataFile == "" {
		dataFile = "tunesday.json"
	}

	store := storage.NewFileStore(dataFile)

	if radioMode {
		data, err := store.Load(ctx)
		if err != nil {
			log.Fatal(err)
		}
		termui.HideCursor()
		defer termui.ShowCursor()
		termui.RadioMenu(ctx, data)
		return
	}

	yt := playlist.NewYouTube()
	application := app.New(store, yt)

	if err := application.Run(ctx, args); err != nil {
		log.Fatal(err)
	}
}
