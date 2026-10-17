package prdetail

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	diffmodel "github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/domain"
)

// TestCtrlPWithoutFilesSaysWhy checks ctrl+p explains itself, rather than
// doing nothing, when there are no files to pick from.
func TestCtrlPWithoutFilesSaysWhy(t *testing.T) {
	for _, tc := range []struct {
		name    string
		loading bool
		diff    *diffmodel.DiffModel
		want    string
	}{
		{"loading", true, nil, "Diff still loading"},
		{"empty", false, &diffmodel.DiffModel{}, "No files to go to"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := NewModel(domain.PullRequestSummary{Repo: "org/repo", Number: 42},
				domain.Repository{Host: "github.com", FullName: "org/repo"}, nil)
			m.Detail = &domain.PRPreviewSnapshot{Repo: "org/repo", Number: 42}
			m.Diff, m.DiffLoading = tc.diff, tc.loading
			_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
			if cmd != nil {
				if _, ok := cmd().(OpenFilePicker); ok {
					t.Fatal("ctrl+p asked for the picker with no files")
				}
			}
			if got := m.StatusHint(); got != tc.want {
				t.Errorf("status hint = %q, want %q", got, tc.want)
			}
		})
	}
}
