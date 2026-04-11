package layout

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCalculate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		width    int
		height   int
		wantRepo int
		wantPR   int
		wantPrev int
	}{
		{
			name:     "120 columns",
			width:    120,
			height:   40,
			wantRepo: 27,
			wantPR:   50,
			wantPrev: 34,
		},
		{
			name:     "80 columns",
			width:    80,
			height:   40,
			wantRepo: 17,
			wantPR:   32,
			wantPrev: 22,
		},
		{
			name:     "60 columns",
			width:    60,
			height:   40,
			wantRepo: 19,
			wantPR:   35,
			wantPrev: 0,
		},
		{
			name:     "40 columns",
			width:    40,
			height:   40,
			wantRepo: 0,
			wantPR:   37,
			wantPrev: 0,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := Calculate(tc.width, tc.height)
			if got.Width != tc.width {
				t.Fatalf("width = %d, want %d", got.Width, tc.width)
			}
			if got.Height != tc.height {
				t.Fatalf("height = %d, want %d", got.Height, tc.height)
			}
			if got.Repo != tc.wantRepo {
				t.Fatalf("repo width = %d, want %d", got.Repo, tc.wantRepo)
			}
			if got.PR != tc.wantPR {
				t.Fatalf("pr width = %d, want %d", got.PR, tc.wantPR)
			}
			if got.Preview != tc.wantPrev {
				t.Fatalf("preview width = %d, want %d", got.Preview, tc.wantPrev)
			}
		})
	}
}

func TestLayoutState_Update_WindowSizeMsg(t *testing.T) {
	t.Parallel()

	state := NewLayoutState(120, 40)
	if got := state.Current; got.Repo != 27 || got.PR != 50 || got.Preview != 34 {
		t.Fatalf("initial layout = %+v, want repo=27 pr=50 preview=34", got)
	}

	state = state.Update(tea.WindowSizeMsg{Width: 60, Height: 24})

	if state.Width != 60 {
		t.Fatalf("width = %d, want 60", state.Width)
	}
	if state.Height != 24 {
		t.Fatalf("height = %d, want 24", state.Height)
	}
	if state.Current.Repo != 19 || state.Current.PR != 35 || state.Current.Preview != 0 {
		t.Fatalf("layout after resize = %+v, want repo=19 pr=35 preview=0", state.Current)
	}
}
