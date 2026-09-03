package store

import "time"

// timeLayout is the canonical UTC timestamp format stored in SQLite.
// All writes go through this format so lexicographic ORDER BY equals
// chronological order. SQLite's datetime('now') uses the same layout.
const timeLayout = "2006-01-02 15:04:05"

// formatTime converts a time.Time to the canonical UTC string for storage.
// A zero time is stored as empty string (NULL).
func formatTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(timeLayout)
}

// FormatTime converts a time.Time to the canonical UTC string for storage.
// It is exported for packages that write timestamps inside transactions.
func FormatTime(t time.Time) any {
	return formatTime(t)
}

// formatTimePtr is like formatTime but for optional timestamps.
func formatTimePtr(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return t.UTC().Format(timeLayout)
}

// parseTime reads back a canonical (or RFC3339 fallback) timestamp.
func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(timeLayout, s)
	if err == nil {
		return t.UTC()
	}
	t, err = time.Parse(time.RFC3339, s)
	if err == nil {
		return t.UTC()
	}
	t, err = time.Parse(time.RFC3339Nano, s)
	if err == nil {
		return t.UTC()
	}
	return time.Time{}
}
