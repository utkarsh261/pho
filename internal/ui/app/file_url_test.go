package app

import (
	"testing"

	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/views/prdetail"
)

func TestFileBrowserURL(t *testing.T) {
	t.Parallel()
	// hex(sha256("src/main.go"))
	const anchor = "#diff-9e185f29fa355d7dd8fdd9c9ff1d0723b85206aa7d37c4eec93997005dc291eb"
	cases := []struct {
		name string
		repo domain.Repository
		msg  prdetail.OpenBrowserFile
		want string
	}{
		{
			name: "github.com PR",
			repo: domain.Repository{Host: "github.com", FullName: "o/r"},
			msg:  prdetail.OpenBrowserFile{Repo: "o/r", Number: 7, Path: "src/main.go"},
			want: "https://github.com/o/r/pull/7/files" + anchor,
		},
		{
			name: "enterprise PR",
			repo: domain.Repository{Host: "ghe.example.com", FullName: "o/r"},
			msg:  prdetail.OpenBrowserFile{Repo: "o/r", Number: 7, Path: "src/main.go"},
			want: "https://ghe.example.com/o/r/pull/7/files" + anchor,
		},
		{
			name: "enterprise commit",
			msg: prdetail.OpenBrowserFile{
				CommitRepo: domain.Repository{Host: "ghe.example.com", FullName: "o/r"},
				CommitSHA:  "abc123", Path: "src/main.go",
			},
			want: "https://ghe.example.com/o/r/commit/abc123" + anchor,
		},
	}
	for _, tc := range cases {
		if got := fileBrowserURL(tc.repo, tc.msg); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.name, got, tc.want)
		}
	}
}
