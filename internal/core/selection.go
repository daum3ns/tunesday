package core

import (
	"math/rand"
	"sort"
	"time"
)

// ProviderSelection holds the result of a provider selection.
type ProviderSelection struct {
	Active []string // all eligible providers, sorted alphabetically
	Pool   []string // providers in the selection pool, sorted alphabetically
	Winner string   // the selected provider
}

// SelectProvider picks a provider using the bottom-half pool rule:
//   - active providers are sorted by tune count ascending
//   - the bottom ceil(activeCount/2) providers form the base pool
//   - ties at the cutoff count are included
//   - the most recent submitter (lastProvider) is removed from the pool when possible
//   - a winner is picked uniformly at random from the remaining pool
//
// active is the list of eligible provider names.
// counts maps provider names to tune counts; missing entries default to 0.
// lastProvider is the provider name of the most recent submitter, or empty.
// r is the random source (injected for testability).
func SelectProvider(active []string, counts map[string]int, lastProvider string, r *rand.Rand) ProviderSelection {
	if r == nil {
		panic("SelectProvider requires a non-nil random source")
	}

	activeCopy := make([]string, len(active))
	copy(activeCopy, active)
	sort.Strings(activeCopy)

	pool := ComputeProviderPool(activeCopy, counts, lastProvider)
	if len(pool) == 0 {
		return ProviderSelection{Active: activeCopy}
	}

	winner := pool[r.Intn(len(pool))]

	return ProviderSelection{
		Active: activeCopy,
		Pool:   pool,
		Winner: winner,
	}
}

// ComputeProviderPool returns the eligible pool under the bottom-half rule
// without drawing a winner. The pool is sorted alphabetically. This split lets
// callers (e.g. tunesday.fm ceremonies) record the pool and seed at start time
// and draw the winner later, keeping every ceremony reproducible.
func ComputeProviderPool(active []string, counts map[string]int, lastProvider string) []string {
	if len(active) == 0 {
		return nil
	}

	// Sort by tune count to find the bottom half.
	byCount := append([]string(nil), active...)
	sort.SliceStable(byCount, func(i, j int) bool {
		return countFor(counts, byCount[i]) < countFor(counts, byCount[j])
	})

	n := (len(byCount) + 1) / 2 // ceil(activeCount / 2)
	if n < 2 {
		n = 2
	}
	if n > len(byCount) {
		n = len(byCount)
	}

	// Include ties at the cutoff count.
	cutoff := countFor(counts, byCount[n-1])
	pool := make([]string, 0)
	for _, name := range byCount {
		if countFor(counts, name) <= cutoff {
			pool = append(pool, name)
		}
	}
	sort.Strings(pool)

	// Exclude the last submitter when there is another eligible provider.
	if len(pool) > 1 && lastProvider != "" {
		filtered := make([]string, 0, len(pool))
		for _, name := range pool {
			if name != lastProvider {
				filtered = append(filtered, name)
			}
		}
		if len(filtered) > 0 {
			pool = filtered
		}
	}

	return pool
}

// SelectProviderFromData is a convenience wrapper that builds the active
// provider list and last-submitter from a Data struct, then calls SelectProvider.
func SelectProviderFromData(data *Data, r *rand.Rand) ProviderSelection {
	if data == nil {
		return SelectProvider(nil, nil, "", r)
	}

	active := make([]string, 0, len(data.Participants))
	for name := range data.Participants {
		if data.Disabled != nil && data.Disabled[name] {
			continue
		}
		active = append(active, name)
	}

	lastProvider := ""
	var lastTime time.Time
	for _, tune := range data.Tunes {
		if tune.AddedAt.After(lastTime) {
			lastTime = tune.AddedAt
			lastProvider = tune.Provider
		}
	}

	return SelectProvider(active, data.Participants, lastProvider, r)
}

// countFor returns the tune count for a provider, defaulting to 0.
func countFor(counts map[string]int, name string) int {
	if counts == nil {
		return 0
	}
	if c, ok := counts[name]; ok {
		return c
	}
	return 0
}
