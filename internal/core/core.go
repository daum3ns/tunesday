package core

import "time"

// Data holds participants and tunes.
type Data struct {
    Participants map[string]int  `json:"participants"`       // name -> tunes count
    Disabled     map[string]bool `json:"disabled,omitempty"` // name -> true if deactivated
    Tunes        []Tune          `json:"tunes"`
}

// Tune represents a single YouTube tune entry.
type Tune struct {
    Name     string    `json:"name"` // video title
    Link     string    `json:"link"` // original YouTube URL
    ID       string    `json:"id"`   // normalized YouTube video ID
    Provider string    `json:"provider"`
    AddedAt  time.Time `json:"added_at,omitempty"`
}

// NewData creates an empty Data structure with initialized maps.
func NewData() *Data { return &Data{Participants: make(map[string]int)} }
