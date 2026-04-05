package ui

import (
	"fmt"
	"time"
)

// FormatFileSize returns a human-readable file size string.
func FormatFileSize(bytes int64) string {
	switch {
	case bytes < 1024:
		return fmt.Sprintf("%d B", bytes)
	case bytes < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	case bytes < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
	default:
		return fmt.Sprintf("%.1f GB", float64(bytes)/(1024*1024*1024))
	}
}

// FormatDuration returns a human-readable duration string.
func FormatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	totalSec := int(d.Seconds())
	switch {
	case totalSec < 60:
		return fmt.Sprintf("%ds", totalSec)
	case totalSec < 3600:
		m := totalSec / 60
		s := totalSec % 60
		return fmt.Sprintf("%dm %ds", m, s)
	default:
		h := totalSec / 3600
		m := (totalSec % 3600) / 60
		return fmt.Sprintf("%dh %dm", h, m)
	}
}

// FormatTokens returns a human-readable token count.
func FormatTokens(n int) string {
	switch {
	case n < 1000:
		return fmt.Sprintf("%d", n)
	case n < 1000000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	default:
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
}

// FormatTime formats a time as a short local time string (HH:MM).
func FormatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("15:04")
}

// FormatDate returns a date group label for the session picker.
// "Today", "Yesterday", or the date in "Mon Jan 2" format.
func FormatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	local := t.Local()
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	yesterday := today.AddDate(0, 0, -1)

	date := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, now.Location())

	switch {
	case date.Equal(today):
		return "Today"
	case date.Equal(yesterday):
		return "Yesterday"
	default:
		return local.Format("Mon Jan 2")
	}
}

// Truncate shortens a string to maxLen, appending "..." if truncated.
func Truncate(s string, maxLen int) string {
	if maxLen <= 3 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-3]) + "..."
}
