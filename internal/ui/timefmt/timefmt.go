// Package timefmt holds time formatting shared by the UI views, so the
// dashboard and PR detail always describe the same moment the same way.
package timefmt

import (
	"fmt"
	"time"
)

// Relative formats t compactly relative to now: "just now", "5m", "3h",
// "2d" for the last three days, then "Jan 02", or "Jan 02 2006" for other years.
// A zero t formats as "".
func Relative(t, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	age := now.Sub(t)
	switch {
	case age < time.Minute:
		return "just now"
	case age < time.Hour:
		return fmt.Sprintf("%dm", int(age.Minutes()))
	case age < 24*time.Hour:
		return fmt.Sprintf("%dh", int(age.Hours()))
	case age < 3*24*time.Hour:
		return fmt.Sprintf("%dd", int(age.Hours()/24))
	case t.Year() == now.Year():
		return t.Format("Jan 02")
	default:
		return t.Format("Jan 02 2006")
	}
}
