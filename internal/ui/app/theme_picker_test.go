package app

import (
	"errors"
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"

	"github.com/utkarsh261/pho/internal/config"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

// recordingSaver records saved theme names; when hold is set, each save
// waits for the test to release it.
type recordingSaver struct {
	mu    sync.Mutex
	names []string
	err   error
}

func (s *recordingSaver) save(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.names = append(s.names, name)
	return s.err
}

func (s *recordingSaver) saved() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.names...)
}

func key(m *Model, msg tea.KeyMsg) tea.Cmd {
	_, cmd := m.Update(msg)
	return cmd
}

var (
	keyDown  = tea.KeyMsg{Type: tea.KeyDown}
	keyEsc   = tea.KeyMsg{Type: tea.KeyEsc}
	keyEnter = tea.KeyMsg{Type: tea.KeyEnter}
	keyT     = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'T'}}
)

// openPicker presses T and runs what it returns: from PR detail, T asks the
// root model to open the picker with a message.
func openPicker(m *Model) {
	drainCmds(m, key(m, keyT))
}

func themePickerModel(t *testing.T) (*Model, *recordingSaver) {
	t.Helper()
	m := ansiPRDetail(t, "2")
	s := &recordingSaver{}
	m.themeSaver = &themeSaver{save: s.save}
	return m, s
}

func TestThemePickerPreviewsSynchronously(t *testing.T) {
	withColorProfile(t, termenv.TrueColor)
	m, _ := themePickerModel(t)
	openPicker(m)
	if !m.themePicking() {
		t.Fatal("T did not open the theme picker from PR detail")
	}
	key(m, keyDown)
	key(m, keyDown)
	if want := theme.Names()[2]; m.theme.Name != want {
		t.Errorf("theme after ↓↓ = %q, want %q", m.theme.Name, want)
	}
	// The diff's added-line tint comes from the PR detail view, so seeing
	// the new theme's tint shows the preview reached it.
	tint := termenv.TrueColor.Color(string(m.theme.DiffAddBg)).Sequence(true)
	if !strings.Contains(m.View(), tint) {
		t.Errorf("view lacks the previewed theme's diff tint %s", tint)
	}
	if !strings.Contains(m.status.HintOverride, "Revert") {
		t.Errorf("status hint = %q, want the picker's hints", m.status.HintOverride)
	}
}

func TestThemePickerEscRestoresExactly(t *testing.T) {
	withColorProfile(t, termenv.TrueColor)
	m, s := themePickerModel(t)
	before := m.View()

	openPicker(m)
	// Rapid movement then Esc, without draining anything in between.
	var cmds []tea.Cmd
	for _, k := range []tea.KeyMsg{keyDown, keyDown, keyDown, keyEsc} {
		cmds = append(cmds, key(m, k))
	}
	for _, c := range cmds {
		drainCmds(m, c)
	}

	if m.theme.Name != theme.DefaultName {
		t.Errorf("theme after Esc = %q, want %q", m.theme.Name, theme.DefaultName)
	}
	if m.themePicking() {
		t.Error("picker still open after Esc")
	}
	if got := m.View(); got != before {
		t.Error("screen after Esc differs from before the picker opened")
	}
	if len(s.saved()) != 0 {
		t.Errorf("Esc saved %v", s.saved())
	}
}

func TestThemePickerEnterCommitsBeforeSave(t *testing.T) {
	m, s := themePickerModel(t)
	openPicker(m)
	key(m, keyDown)
	want := theme.Names()[1]
	save := key(m, keyEnter)

	if m.theme.Name != want || m.themePicking() {
		t.Fatalf("after Enter: theme %q, picking %v; want %q, closed", m.theme.Name, m.themePicking(), want)
	}
	if len(s.saved()) != 0 {
		t.Fatal("saved before the command ran")
	}
	drainCmds(m, save)
	if got := s.saved(); len(got) != 1 || got[0] != want {
		t.Errorf("saved %v, want [%s]", got, want)
	}
}

// TestThemePickerReopenBeforeSave keeps the choice made with Enter when the
// picker is reopened and cancelled before the save has run.
func TestThemePickerReopenBeforeSave(t *testing.T) {
	m, s := themePickerModel(t)
	openPicker(m)
	key(m, keyDown)
	chosen := theme.Names()[1]
	save := key(m, keyEnter)

	openPicker(m)
	key(m, keyDown)
	key(m, keyEsc)
	drainCmds(m, save)

	if m.theme.Name != chosen {
		t.Errorf("theme = %q, want %q", m.theme.Name, chosen)
	}
	if got := s.saved(); len(got) != 1 || got[0] != chosen {
		t.Errorf("saved %v, want [%s]", got, chosen)
	}
}

// TestThemePickerSavesInOrder runs two saves out of order: the older one is
// dropped, so the newer choice stays on disk, and only its result counts.
func TestThemePickerSavesInOrder(t *testing.T) {
	m, s := themePickerModel(t)
	s.err = nil

	openPicker(m)
	key(m, keyDown)
	saveA := key(m, keyEnter)
	openPicker(m)
	key(m, keyDown)
	saveB := key(m, keyEnter)
	b := theme.Names()[2]

	msgB, msgA := saveB(), saveA()
	if got := s.saved(); len(got) != 1 || got[0] != b {
		t.Fatalf("saved %v, want only [%s]", got, b)
	}

	// A late failure report for the older save must not show.
	stale := msgA.(ThemeSaved)
	stale.Err = errors.New("disk full")
	m.Update(stale)
	m.Update(msgB)
	if len(m.state.Errors.Errors) != 0 {
		t.Errorf("stale save result shown: %v", m.state.Errors.Errors)
	}
}

func TestThemePickerNoMatchEnterStaysOpen(t *testing.T) {
	m, s := themePickerModel(t)
	openPicker(m)
	for _, r := range "zzz" {
		key(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if cmd := key(m, keyEnter); cmd != nil {
		drainCmds(m, cmd)
	}
	if !m.themePicking() {
		t.Error("Enter with no matches closed the picker")
	}
	if m.theme.Name != theme.DefaultName {
		t.Errorf("theme = %q, want the original", m.theme.Name)
	}
	if len(s.saved()) != 0 {
		t.Errorf("saved %v", s.saved())
	}
}

func TestThemePickerSaveFailureKeepsTheme(t *testing.T) {
	for name, saveErr := range map[string]error{
		"write error": errors.New("permission denied"),
		"manual edit": config.ErrManualEdit,
	} {
		t.Run(name, func(t *testing.T) {
			m, s := themePickerModel(t)
			s.err = saveErr
			openPicker(m)
			key(m, keyDown)
			drainCmds(m, key(m, keyEnter))

			if want := theme.Names()[1]; m.theme.Name != want {
				t.Errorf("theme = %q, want %q", m.theme.Name, want)
			}
			errs := m.state.Errors.Errors
			if len(errs) != 1 || errs[0].Kind != domain.ErrorKindConfig || !strings.Contains(errs[0].Message, "theme set for this session") {
				t.Errorf("errors = %+v", errs)
			}
		})
	}
}

// TestThemePickerKeyTypesInSearch checks T is text, not a shortcut, while
// the PR detail search is being typed.
func TestThemePickerKeyTypesInSearch(t *testing.T) {
	m, _ := themePickerModel(t)
	ansiKeys(m, "/")
	openPicker(m)
	if m.themePicking() {
		t.Error("T opened the theme picker while typing a search")
	}
}

func TestThemePickerFromDashboard(t *testing.T) {
	m := ansiDashboard(t, 120, 40)
	openPicker(m)
	if !m.themePicking() {
		t.Fatal("T did not open the theme picker from the dashboard")
	}
	key(m, keyDown)
	if want := theme.Names()[1]; m.theme.Name != want {
		t.Errorf("theme = %q, want %q", m.theme.Name, want)
	}
}
