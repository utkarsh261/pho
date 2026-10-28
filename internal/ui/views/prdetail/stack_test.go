package prdetail

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/utkarsh261/pho/internal/domain"
)

// testStack is main ← #1 ← #2 ← … ← #n, top first.
func testStack(n int) *domain.Stack {
	st := &domain.Stack{Repo: "org/repo", Trunk: "main"}
	for i := n; i >= 1; i-- {
		m := domain.StackMember{Number: i, Title: "layer", State: domain.PRStateOpen, Layer: i, Parent: i - 1}
		if i < n {
			m.Children = []int{i + 1}
		}
		st.Members = append(st.Members, m)
	}
	return st
}

func stackedModel(width, number int, st *domain.Stack) *PRDetailModel {
	m := makeReviewersModel(width, 30)
	m.Summary.Number = number
	m.SetStack(st)
	return m
}

func TestHeaderStackBreadcrumb(t *testing.T) {
	t.Parallel()
	m := stackedModel(120, 2, testStack(3))
	lines := strings.Split(m.renderHeader(), "\n")
	top := ansi.Strip(lines[0])
	if !strings.HasPrefix(top, "╭─ main › #1 › #2 › #3  { } ─") || !strings.HasSuffix(top, "─╮") {
		t.Errorf("top border = %q", top)
	}
}

func TestHeaderStackBreadcrumbKeepsSize(t *testing.T) {
	t.Parallel()
	for _, n := range []int{2, 5, 40} {
		for w := 10; w <= 160; w++ {
			m := stackedModel(w, n/2+1, testStack(n))
			header := m.renderHeader()
			if h := lipgloss.Height(header); h != 4 {
				t.Fatalf("stack %d, width %d: height %d, want 4", n, w, h)
			}
			for i, line := range strings.Split(header, "\n") {
				if lw := lipgloss.Width(line); lw != w {
					t.Fatalf("stack %d, width %d: line %d is %d wide: %q", n, w, i, lw, ansi.Strip(line))
				}
			}
		}
	}
}

func TestHeaderStackBreadcrumbCollapses(t *testing.T) {
	t.Parallel()
	m := stackedModel(80, 20, testStack(40))
	top := ansi.Strip(strings.Split(m.renderHeader(), "\n")[0])
	if !strings.Contains(top, "main › #1 › … › #19 › #20 › #21 › … › #40") {
		t.Errorf("collapsed breadcrumb = %q", top)
	}
}

func TestHeaderWithoutStackIsPlain(t *testing.T) {
	t.Parallel()
	m := makeReviewersModel(60, 30)
	if got := ansi.Strip(strings.Split(m.renderHeader(), "\n")[0]); got != "╭"+strings.Repeat("─", 58)+"╮" {
		t.Errorf("top border = %q", got)
	}
}

func stackKey(m *PRDetailModel, key string) tea.Msg {
	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	if cmd == nil {
		return nil
	}
	return cmd()
}

func TestStackHopKeys(t *testing.T) {
	t.Parallel()
	m := stackedModel(120, 2, testStack(3))
	if got := stackKey(m, "}"); got != (StackHop{Number: 3}) {
		t.Errorf("} = %#v, want hop to #3", got)
	}
	if got := stackKey(m, "{"); got != (StackHop{Number: 1}) {
		t.Errorf("{ = %#v, want hop to #1", got)
	}
	if _, ok := stackKey(m, "S").(OpenStackPicker); !ok {
		t.Error("S should open the stack picker")
	}

	top := stackedModel(120, 3, testStack(3))
	if stackKey(top, "}") != nil || top.StatusHint() != "Top of stack" {
		t.Errorf("} on top: hint %q", top.StatusHint())
	}
	bottom := stackedModel(120, 1, testStack(3))
	if stackKey(bottom, "{") != nil || bottom.StatusHint() != "Bottom of stack · based on main" {
		t.Errorf("{ on bottom: hint %q", bottom.StatusHint())
	}
	alone := makeReviewersModel(120, 30)
	if stackKey(alone, "}") != nil || alone.StatusHint() != "Not part of a stack" {
		t.Errorf("} unstacked: hint %q", alone.StatusHint())
	}
}

func TestStackHopAtForkSaysWhichBranch(t *testing.T) {
	t.Parallel()
	st := testStack(2)
	st.Members[1].Children = []int{5, 2} // #5 also sits on #1
	st.Members = append([]domain.StackMember{{Number: 5, Parent: 1, Layer: 2, Indent: 1}}, st.Members...)
	m := stackedModel(120, 1, st)
	hop, ok := stackKey(m, "}").(StackHop)
	if !ok || hop.Number != 5 || !strings.Contains(hop.Notice, "1 of 2") {
		t.Errorf("hop = %#v", hop)
	}
}

func TestStatusHintTeachesStackKeys(t *testing.T) {
	t.Parallel()
	if hint := stackedModel(120, 2, testStack(3)).StatusHint(); !strings.Contains(hint, "{/}: Down/Up stack | S: Stack | ?") {
		t.Errorf("stacked hint = %q", hint)
	}
	if hint := makeReviewersModel(120, 30).StatusHint(); strings.Contains(hint, "stack") {
		t.Errorf("unstacked hint = %q", hint)
	}
}
