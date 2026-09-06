package web

import (
	"log"
	"net/http"
	"time"
)

// winRateRow is a ceremony win count with the computed win percentage.
type winRateRow struct {
	ProviderName    string
	Wins            int
	TotalCeremonies int
	WinPct          int // 0-100
}

// StatsPage renders the team statistics page.
func (h *Handler) StatsPage(w http.ResponseWriter, r *http.Request) {
	team, member, ok := h.requireMember(w, r)
	if !ok {
		return
	}

	data := teamPage(team, member, "stats")
	data["Title"] = "Stats"

	// Radio stats.
	totalPlays, err := h.deps.PlayStats.TotalPlays(team.ID)
	if err != nil {
		log.Printf("stats: total plays: %v", err)
	}
	data["TotalPlays"] = totalPlays

	weekAgo := time.Now().AddDate(0, 0, -7)
	tuneOfWeek, err := h.deps.PlayStats.MostPlayed(team.ID, &weekAgo, 1)
	if err != nil {
		log.Printf("stats: tune of week: %v", err)
	}
	if len(tuneOfWeek) > 0 {
		data["TuneOfTheWeek"] = tuneOfWeek[0]
	}

	mostPlayed, err := h.deps.PlayStats.MostPlayed(team.ID, nil, 10)
	if err != nil {
		log.Printf("stats: most played: %v", err)
	}
	data["MostPlayed"] = mostPlayed

	providerPlays, err := h.deps.PlayStats.ProviderPlayCounts(team.ID, nil)
	if err != nil {
		log.Printf("stats: provider plays: %v", err)
	}
	data["ProviderPlays"] = providerPlays

	// Ceremony stats.
	winCounts, err := h.deps.Ceremonies.WinCounts(team.ID)
	if err != nil {
		log.Printf("stats: win counts: %v", err)
	}
	var winRates []winRateRow
	for _, wc := range winCounts {
		pct := 0
		if wc.TotalCeremonies > 0 {
			pct = wc.Wins * 100 / wc.TotalCeremonies
		}
		winRates = append(winRates, winRateRow{
			ProviderName: wc.ProviderName, Wins: wc.Wins,
			TotalCeremonies: wc.TotalCeremonies, WinPct: pct,
		})
	}
	data["WinRates"] = winRates

	ceremonyStats, err := h.deps.Ceremonies.Stats(team.ID)
	if err != nil {
		log.Printf("stats: ceremony stats: %v", err)
	}
	data["CeremonyTotal"] = ceremonyStats.TotalCeremonies
	data["AvgAttendance"] = ceremonyStats.AvgAttendance

	// Quiz stats.
	providerRecog, err := h.deps.Quiz.ProviderRecognition(team.ID)
	if err != nil {
		log.Printf("stats: provider recognition: %v", err)
	}
	data["ProviderRecognition"] = providerRecog

	trickiest, err := h.deps.Quiz.TrickiestTunes(team.ID, 5)
	if err != nil {
		log.Printf("stats: trickiest tunes: %v", err)
	}
	data["TrickiestTunes"] = trickiest

	bests, err := h.deps.Quiz.Bests(team.ID)
	if err != nil {
		log.Printf("stats: quiz bests: %v", err)
	}
	data["QuizBests"] = bests

	recent, err := h.deps.Quiz.Recent(team.ID, 10)
	if err != nil {
		log.Printf("stats: recent games: %v", err)
	}
	data["RecentGames"] = recent

	h.render(w, r, "stats.html", data)
}
