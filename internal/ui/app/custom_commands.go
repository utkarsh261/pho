package app

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/customcmd"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/components/keymapoverlay"
)

func (m *Model) customBinding(msg tea.KeyMsg) (customcmd.Binding, bool) {
	if msg.Type != tea.KeyRunes || len(msg.Runes) != 1 || msg.Alt || msg.Paste {
		return customcmd.Binding{}, false
	}
	return m.customBindingFor(msg.String())
}

func (m *Model) customBindingFor(key string) (customcmd.Binding, bool) {
	for _, b := range m.deps.Keybindings {
		if b.Key == key {
			return b, true
		}
	}
	return customcmd.Binding{}, false
}

func (m *Model) customOverlayBindings() []keymapoverlay.Binding {
	var out []keymapoverlay.Binding
	for _, b := range m.deps.Keybindings {
		out = append(out, keymapoverlay.Binding{Key: b.Key, Description: b.Label()})
	}
	return out
}

func (m *Model) runCustomOnDashboard(b customcmd.Binding) tea.Cmd {
	pr, ok := m.currentSelectedPR()
	if !ok {
		m.setCustomNotice("No PR selected")
		return nil
	}
	return m.runCustom(b, m.repoForPR(pr.Repo), pr)
}

func (m *Model) runCustomInPRDetail(key string) tea.Cmd {
	b, ok := m.customBindingFor(key)
	if !ok || m.currentView() != domain.PrimaryViewPRDetail || m.prDetail == nil {
		return nil
	}
	// A `pho pr N` summary is bare until the PR loads; every PR has a head.
	if m.prDetail.Summary.HeadRefName == "" {
		m.setCustomNotice("PR still loading")
		return nil
	}
	return m.runCustom(b, m.prDetail.Repo, m.prDetail.Summary)
}

// repoForPR finds the PR's repo among the discovered ones, for its local
// checkout; a PR from elsewhere gets a repo without one.
func (m *Model) repoForPR(fullName string) domain.Repository {
	if repo, ok := m.selectedRepo(); ok && sameRepo(repo.FullName, fullName) {
		return repo
	}
	for _, repo := range m.state.Repos.Discovered {
		if sameRepo(repo.FullName, fullName) {
			return repo
		}
	}
	return m.selectedRepoForURL(fullName)
}

func (m *Model) runCustom(b customcmd.Binding, repo domain.Repository, pr domain.PullRequestSummary) tea.Cmd {
	if b.UsesRepoPath() && repo.LocalPath == "" {
		m.setCustomNotice("Not a local repo · " + b.Key + " needs RepoPath")
		return nil
	}
	name := repo.FullName
	if name == "" {
		name = pr.Repo
	}
	command, err := b.Render(customcmd.Data{
		RepoName:    name,
		RepoPath:    repo.LocalPath,
		PrNumber:    pr.Number,
		HeadRefName: pr.HeadRefName,
		BaseRefName: pr.BaseRefName,
		HeadSHA:     pr.HeadRefOID,
		Author:      pr.Author,
		URL:         browserURL(repo, pr.Repo, pr.Number),
	})
	if err != nil {
		m.setCustomNotice("Custom command " + b.Key + ": " + err.Error())
		return nil
	}
	m.logDebug("running custom command", "key", b.Key, "command", command, "dir", repo.LocalPath)
	run := m.deps.RunCustom
	if run == nil {
		run = customcmd.Exec
	}
	return run(b.Key, command, repo.LocalPath)
}

func (m *Model) handleCustomFinished(msg customcmd.FinishedMsg) {
	if msg.Err == nil {
		m.setCustomNotice("")
		return
	}
	m.logWarn("custom command failed", "key", msg.Key, "err", msg.Err)
	m.setCustomNotice(customcmd.FailureNotice(msg))
}

// setCustomNotice shows s in the current view until the next key.
func (m *Model) setCustomNotice(s string) {
	if m.currentView() == domain.PrimaryViewPRDetail && m.prDetail != nil {
		m.prDetail.SetNotice(s)
	} else {
		m.customNotice = s
	}
	m.syncStatus()
}
