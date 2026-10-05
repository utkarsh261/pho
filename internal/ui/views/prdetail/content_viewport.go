package prdetail

import (
	"time"

	"github.com/utkarsh261/pho/internal/ui/timefmt"
)

func relativeTime(t time.Time) string {
	return timefmt.Relative(t, time.Now())
}
