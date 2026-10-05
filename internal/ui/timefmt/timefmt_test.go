package timefmt

import (
	"testing"
	"time"
)

func TestRelative(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		t    time.Time
		want string
	}{
		{time.Time{}, ""},
		{now.Add(-30 * time.Second), "just now"},
		{now.Add(-5 * time.Minute), "5m"},
		{now.Add(-3 * time.Hour), "3h"},
		{now.Add(-50 * time.Hour), "2d"},
		{now.Add(-4 * 24 * time.Hour), "Oct 01"},
		{time.Date(2025, 12, 31, 9, 0, 0, 0, time.UTC), "Dec 31 2025"},
	}
	for _, c := range cases {
		if got := Relative(c.t, now); got != c.want {
			t.Errorf("Relative(%v) = %q, want %q", c.t, got, c.want)
		}
	}
}
