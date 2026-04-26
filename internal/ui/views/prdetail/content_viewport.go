package prdetail

import (
	"fmt"
	"time"
)

func relativeTime(t time.Time) string {
	age := time.Since(t)
	switch {
	case age < time.Minute:
		return "just now"
	case age < time.Hour:
		return fmt.Sprintf("%dm", int(age.Minutes()))
	case age < 24*time.Hour:
		return fmt.Sprintf("%dh", int(age.Hours()))
	case age < 3*24*time.Hour:
		return fmt.Sprintf("%dd", int(age.Hours()/24))
	default:
		if t.Year() == time.Now().Year() {
			return t.Format("Jan 02")
		}
		return t.Format("Jan 02 2006")
	}
}
