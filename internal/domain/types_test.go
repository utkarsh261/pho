package domain

import (
	"testing"
	"time"
)

func TestLastReview(t *testing.T) {
	t0 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		reviewers []PreviewReviewer
		viewer    string
		want      string
	}{
		{name: "no viewer", reviewers: []PreviewReviewer{{Login: "me", CommitSHA: "a", SubmittedAt: t0}}, want: ""},
		{name: "no reviews", viewer: "me", want: ""},
		{
			name:   "latest wins regardless of order",
			viewer: "me",
			reviewers: []PreviewReviewer{
				{Login: "me", State: "COMMENTED", CommitSHA: "new", SubmittedAt: t0.Add(time.Hour)},
				{Login: "me", State: "APPROVED", CommitSHA: "old", SubmittedAt: t0},
			},
			want: "new",
		},
		{
			name:   "pending and other users skipped",
			viewer: "me",
			reviewers: []PreviewReviewer{
				{Login: "me", State: "APPROVED", CommitSHA: "mine", SubmittedAt: t0},
				{Login: "me", State: "PENDING", CommitSHA: "draft", SubmittedAt: t0.Add(2 * time.Hour)},
				{Login: "bob", State: "APPROVED", CommitSHA: "bobs", SubmittedAt: t0.Add(3 * time.Hour)},
			},
			want: "mine",
		},
		{
			name:      "login matched case-insensitively",
			viewer:    "Me",
			reviewers: []PreviewReviewer{{Login: "me", State: "CHANGES_REQUESTED", CommitSHA: "x", SubmittedAt: t0}},
			want:      "x",
		},
		{
			name:      "review without commit ignored",
			viewer:    "me",
			reviewers: []PreviewReviewer{{Login: "me", State: "COMMENTED", SubmittedAt: t0}},
			want:      "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := LastReview(tt.reviewers, tt.viewer)
			if got.CommitSHA != tt.want || ok != (tt.want != "") {
				t.Errorf("LastReview = %q, %v; want %q", got.CommitSHA, ok, tt.want)
			}
		})
	}
}
