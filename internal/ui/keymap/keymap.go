package keymap

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/domain"
)

// Action is a typed UI intent emitted by the keymap registry.
type Action interface {
	isAction()
}

// Result describes how a key press should be routed.
//
// When PassThrough is true, the caller should let the focused component
// consume the key. When Action is non-nil, the registry has handled the key
// and produced an intent for the root model.
type Result struct {
	Action      Action
	PassThrough bool
}

// Registry routes key presses based on the active focus target.
type Registry struct{}

func NewRegistry() *Registry {
	return &Registry{}
}

func DefaultRegistry() *Registry {
	return NewRegistry()
}

// Dispatch resolves a key press into an action or a pass-through signal.
func Dispatch(focus domain.FocusTarget, msg tea.KeyMsg) Result {
	return DefaultRegistry().Dispatch(focus, msg)
}

// Dispatch resolves a key press into an action or a pass-through signal.
func (r *Registry) Dispatch(focus domain.FocusTarget, msg tea.KeyMsg) Result {
	switch focus {
	case domain.FocusCmdPalette:
		return dispatchCmdPalette(msg)
	case domain.FocusRepoPanel:
		return dispatchRepoPanel(msg)
	case domain.FocusPRListPanel:
		return dispatchPRListPanel(msg)
	case domain.FocusPreviewPanel:
		return dispatchPreviewPanel(msg)
	default:
		return dispatchGlobal(msg)
	}
}

func dispatchGlobal(msg tea.KeyMsg) Result {
	switch msg.Type {
	case tea.KeyCtrlP:
		return Result{Action: ToggleCmdPalette{}}
	case tea.KeyTab:
		return Result{Action: CycleFocus{Direction: FocusNext}}
	case tea.KeyShiftTab:
		return Result{Action: CycleFocus{Direction: FocusPrev}}
	case tea.KeyRunes:
		if len(msg.Runes) != 1 {
			return Result{}
		}
		switch msg.Runes[0] {
		case 'q':
			return Result{Action: Quit{}}
		case '/':
			return Result{Action: OpenDashboardFilter{}}
		case 'R':
			return Result{Action: TriggerRefresh{}}
		}
	}
	return Result{}
}

func dispatchRepoPanel(msg tea.KeyMsg) Result {
	switch msg.Type {
	case tea.KeyEnter:
		return Result{Action: SelectRepo{}}
	case tea.KeyCtrlP:
		return Result{Action: ToggleCmdPalette{}}
	case tea.KeyTab:
		return Result{Action: CycleFocus{Direction: FocusNext}}
	case tea.KeyShiftTab:
		return Result{Action: CycleFocus{Direction: FocusPrev}}
	case tea.KeyRunes:
		if len(msg.Runes) != 1 {
			return Result{}
		}
		switch msg.Runes[0] {
		case 'j':
			return Result{Action: MoveRepoSelection{Delta: +1}}
		case 'k':
			return Result{Action: MoveRepoSelection{Delta: -1}}
		case 'q':
			return Result{Action: Quit{}}
		case '/':
			return Result{Action: OpenDashboardFilter{}}
		case 'R':
			return Result{Action: TriggerRefresh{}}
		}
	}
	return dispatchGlobal(msg)
}

func dispatchPRListPanel(msg tea.KeyMsg) Result {
	switch msg.Type {
	case tea.KeyEnter:
		return Result{Action: SelectPR{}}
	case tea.KeyCtrlP:
		return Result{Action: ToggleCmdPalette{}}
	case tea.KeyTab:
		return Result{Action: CycleFocus{Direction: FocusNext}}
	case tea.KeyShiftTab:
		return Result{Action: CycleFocus{Direction: FocusPrev}}
	case tea.KeyRunes:
		if len(msg.Runes) != 1 {
			return Result{}
		}
		switch msg.Runes[0] {
		case 'j':
			return Result{Action: MovePRSelection{Delta: +1}}
		case 'k':
			return Result{Action: MovePRSelection{Delta: -1}}
		case 'h':
			return Result{Action: ChangeTab{Direction: TabPrev}}
		case 'l':
			return Result{Action: ChangeTab{Direction: TabNext}}
		case 'o':
			return Result{Action: OpenBrowser{}}
		case 'q':
			return Result{Action: Quit{}}
		case '/':
			return Result{Action: OpenDashboardFilter{}}
		case 'R':
			return Result{Action: TriggerRefresh{}}
		}
	}
	return dispatchGlobal(msg)
}

func dispatchPreviewPanel(msg tea.KeyMsg) Result {
	switch msg.Type {
	case tea.KeyEnter:
		return Result{Action: OpenPRDetail{}}
	case tea.KeyCtrlP:
		return Result{Action: ToggleCmdPalette{}}
	case tea.KeyTab:
		return Result{Action: CycleFocus{Direction: FocusNext}}
	case tea.KeyShiftTab:
		return Result{Action: CycleFocus{Direction: FocusPrev}}
	case tea.KeyRunes:
		if len(msg.Runes) != 1 {
			return Result{}
		}
		switch msg.Runes[0] {
		case 'j':
			return Result{Action: ScrollPreview{Delta: +1}}
		case 'k':
			return Result{Action: ScrollPreview{Delta: -1}}
		case 'o':
			return Result{Action: OpenBrowser{}}
		case 'q':
			return Result{Action: Quit{}}
		case '/':
			return Result{Action: OpenDashboardFilter{}}
		case 'R':
			return Result{Action: TriggerRefresh{}}
		}
	}
	return dispatchGlobal(msg)
}

func dispatchCmdPalette(msg tea.KeyMsg) Result {
	switch msg.Type {
	case tea.KeyEsc:
		return Result{Action: CloseCmdPalette{}}
	case tea.KeyEnter:
		return Result{PassThrough: true}
	case tea.KeyCtrlP:
		return Result{Action: ToggleCmdPalette{}}
	case tea.KeyTab:
		return Result{PassThrough: true}
	case tea.KeyShiftTab:
		return Result{PassThrough: true}
	case tea.KeyRunes:
		if len(msg.Runes) != 1 {
			return Result{}
		}
		switch msg.Runes[0] {
		case 'j':
			return Result{Action: MovePaletteSelection{Delta: +1}}
		case 'k':
			return Result{Action: MovePaletteSelection{Delta: -1}}
		case 'h':
			return Result{PassThrough: true}
		}
		return Result{PassThrough: true}
	}
	return Result{}
}

type TabDirection int

const (
	TabPrev TabDirection = -1
	TabNext TabDirection = 1
)

type CycleDirection int

const (
	FocusPrev CycleDirection = -1
	FocusNext CycleDirection = 1
)

type MoveRepoSelection struct {
	Delta int
}

func (MoveRepoSelection) isAction() {}

type MovePRSelection struct {
	Delta int
}

func (MovePRSelection) isAction() {}

type ScrollPreview struct {
	Delta int
}

func (ScrollPreview) isAction() {}

type MovePaletteSelection struct {
	Delta int
}

func (MovePaletteSelection) isAction() {}

type ChangeTab struct {
	Direction TabDirection
}

func (ChangeTab) isAction() {}

type CycleFocus struct {
	Direction CycleDirection
}

func (CycleFocus) isAction() {}

type SelectRepo struct{}

func (SelectRepo) isAction() {}

type SelectPR struct{}

func (SelectPR) isAction() {}

type ToggleCmdPalette struct{}

func (ToggleCmdPalette) isAction() {}

type CloseCmdPalette struct{}

func (CloseCmdPalette) isAction() {}

type OpenDashboardFilter struct{}

func (OpenDashboardFilter) isAction() {}

type TriggerRefresh struct{}

func (TriggerRefresh) isAction() {}

type OpenBrowser struct{}

func (OpenBrowser) isAction() {}

// OpenPRDetail is emitted when Enter is pressed on the preview panel.
// The root model handles this by pushing the PR detail view.
type OpenPRDetail struct{}

func (OpenPRDetail) isAction() {}

type Quit struct{}

func (Quit) isAction() {}
