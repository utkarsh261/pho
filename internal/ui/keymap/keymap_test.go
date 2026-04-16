package keymap

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/domain"
)

func TestDispatch_PRListPanelBindings(t *testing.T) {
	got := Dispatch(domain.FocusPRListPanel, keyRune('j'))
	assertAction(t, got.Action, MovePRSelection{Delta: +1})
	if got.PassThrough {
		t.Fatal("expected j on PR list to be handled, got PassThrough")
	}

	got = Dispatch(domain.FocusPRListPanel, keyRune('h'))
	assertAction(t, got.Action, ChangeTab{Direction: TabPrev})
	if got.PassThrough {
		t.Fatal("expected h on PR list to be handled, got PassThrough")
	}
}

func TestDispatch_CmdPaletteBindings(t *testing.T) {
	got := Dispatch(domain.FocusCmdPalette, keyRune('j'))
	assertAction(t, got.Action, MovePaletteSelection{Delta: +1})
	if got.PassThrough {
		t.Fatal("expected j in CmdPalette to be handled, got PassThrough")
	}

	got = Dispatch(domain.FocusCmdPalette, keyRune('h'))
	if got.Action != nil {
		t.Fatalf("expected h in CmdPalette to pass through, got %#v", got.Action)
	}
	if !got.PassThrough {
		t.Fatal("expected h in CmdPalette to pass through to text input")
	}

	got = Dispatch(domain.FocusCmdPalette, keyMsg(tea.KeyEsc))
	assertAction(t, got.Action, CloseCmdPalette{})
}

func TestDispatch_RepoPanelBindings(t *testing.T) {
	got := Dispatch(domain.FocusRepoPanel, keyMsg(tea.KeyEnter))
	assertAction(t, got.Action, SelectRepo{})

	got = Dispatch(domain.FocusRepoPanel, keyRune('j'))
	assertAction(t, got.Action, MoveRepoSelection{Delta: +1})

	got = Dispatch(domain.FocusRepoPanel, keyRune('k'))
	assertAction(t, got.Action, MoveRepoSelection{Delta: -1})
}

func TestDispatch_GlobalBindings(t *testing.T) {
	got := Dispatch(domain.FocusPreviewPanel, keyMsg(tea.KeyCtrlP))
	assertAction(t, got.Action, ToggleCmdPalette{})

	got = Dispatch(domain.FocusPreviewPanel, keyRune('r'))
	assertAction(t, got.Action, TriggerRefresh{})

	got = Dispatch(domain.FocusPreviewPanel, keyRune('/'))
	assertAction(t, got.Action, OpenDashboardFilter{})

	got = Dispatch(domain.FocusPreviewPanel, keyRune('q'))
	assertAction(t, got.Action, Quit{})
}

func keyRune(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func keyMsg(t tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{Type: t}
}

func assertAction(t *testing.T, got Action, want Action) {
	t.Helper()

	switch want := want.(type) {
	case MovePRSelection:
		gotTyped, ok := got.(MovePRSelection)
		if !ok {
			t.Fatalf("action type = %T, want MovePRSelection", got)
		}
		if gotTyped != want {
			t.Fatalf("action = %#v, want %#v", gotTyped, want)
		}
	case MoveRepoSelection:
		gotTyped, ok := got.(MoveRepoSelection)
		if !ok {
			t.Fatalf("action type = %T, want MoveRepoSelection", got)
		}
		if gotTyped != want {
			t.Fatalf("action = %#v, want %#v", gotTyped, want)
		}
	case MovePaletteSelection:
		gotTyped, ok := got.(MovePaletteSelection)
		if !ok {
			t.Fatalf("action type = %T, want MovePaletteSelection", got)
		}
		if gotTyped != want {
			t.Fatalf("action = %#v, want %#v", gotTyped, want)
		}
	case ChangeTab:
		gotTyped, ok := got.(ChangeTab)
		if !ok {
			t.Fatalf("action type = %T, want ChangeTab", got)
		}
		if gotTyped != want {
			t.Fatalf("action = %#v, want %#v", gotTyped, want)
		}
	case SelectRepo:
		if _, ok := got.(SelectRepo); !ok {
			t.Fatalf("action type = %T, want SelectRepo", got)
		}
	case CloseCmdPalette:
		if _, ok := got.(CloseCmdPalette); !ok {
			t.Fatalf("action type = %T, want CloseCmdPalette", got)
		}
	case ToggleCmdPalette:
		if _, ok := got.(ToggleCmdPalette); !ok {
			t.Fatalf("action type = %T, want ToggleCmdPalette", got)
		}
	case TriggerRefresh:
		if _, ok := got.(TriggerRefresh); !ok {
			t.Fatalf("action type = %T, want TriggerRefresh", got)
		}
	case OpenDashboardFilter:
		if _, ok := got.(OpenDashboardFilter); !ok {
			t.Fatalf("action type = %T, want OpenDashboardFilter", got)
		}
	case Quit:
		if _, ok := got.(Quit); !ok {
			t.Fatalf("action type = %T, want Quit", got)
		}
	default:
		t.Fatalf("unsupported want action type %T", want)
	}
}
