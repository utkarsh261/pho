package search

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/utkarsh261/pho/internal/domain"
)

type SearchService interface {
	BuildPRIndex(repo domain.Repository, snap domain.DashboardSnapshot) error
	BuildRepoIndex(repos []domain.Repository) error
	SearchPRs(query string, limit int) []domain.SearchResult
	SearchRepos(query string, limit int) []domain.SearchResult
}

type Service struct {
	mu sync.RWMutex

	prIndexes   map[string]*prIndex
	repoIndex   []repoEntry
	currentRepo string
	currentTab  domain.DashboardTab
	prTabSignal map[string]map[int]map[domain.DashboardTab]bool
}

type prIndex struct {
	repo      domain.Repository
	fetchedAt time.Time
	entries   []prEntry
}

type prEntry struct {
	repo      string
	number    int
	title     string
	branch    string
	author    string
	updatedAt time.Time
	tabs      map[domain.DashboardTab]bool
}

type repoEntry struct {
	repo domain.Repository
}

type scoredPR struct {
	result  domain.SearchResult
	updated time.Time
}

type scoredRepo struct {
	result domain.SearchResult
}

const (
	exactPRNumberBoost   = 10000
	titlePrefixBoost     = 1500
	titleSubstringBoost  = 1250
	branchPrefixBoost    = 1100
	branchSubstringBoost = 900
	authorSubstringBoost = 700
	repoSubstringBoost   = 600
	currentRepoBoost     = 300
	currentTabBoost      = 250
)

func New() *Service {
	return &Service{
		prIndexes:   make(map[string]*prIndex),
		prTabSignal: make(map[string]map[int]map[domain.DashboardTab]bool),
	}
}

func (s *Service) SetCurrentRepo(repo string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureMapsLocked()
	s.currentRepo = canonicalRepoKey(repo)
}

func (s *Service) SetCurrentTab(tab domain.DashboardTab) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureMapsLocked()
	s.currentTab = tab
}

// SetPRTabs annotates a single PR entry with explicit dashboard tab membership.
// It is optional; BuildPRIndex seeds a conservative default tab set.
func (s *Service) SetPRTabs(repo string, number int, tabs ...domain.DashboardTab) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureMapsLocked()

	key := canonicalRepoKey(repo)
	if key == "" {
		return
	}
	if _, ok := s.prTabSignal[key]; !ok {
		s.prTabSignal[key] = make(map[int]map[domain.DashboardTab]bool)
	}
	if _, ok := s.prTabSignal[key][number]; !ok {
		s.prTabSignal[key][number] = make(map[domain.DashboardTab]bool)
	}
	for _, tab := range tabs {
		s.prTabSignal[key][number][tab] = true
	}
	if idx, ok := s.prIndexes[key]; ok {
		for i := range idx.entries {
			if idx.entries[i].number == number {
				if idx.entries[i].tabs == nil {
					idx.entries[i].tabs = make(map[domain.DashboardTab]bool)
				}
				for _, tab := range tabs {
					idx.entries[i].tabs[tab] = true
				}
			}
		}
	}
}

// BuildPRIndex replaces the PR index for one repository.
func (s *Service) BuildPRIndex(repo domain.Repository, snap domain.DashboardSnapshot) error {
	key := canonicalRepoKey(repo.FullName)
	if key == "" {
		key = canonicalRepoKey(repoKey(repo))
	}
	effectiveRepo := repo
	if key == "" {
		key = canonicalRepoKey(repoKey(snap.Repo))
		effectiveRepo = snap.Repo
	}
	if key == "" {
		return fmt.Errorf("search: build PR index: missing repo identity")
	}

	index := &prIndex{
		repo:      effectiveRepo,
		fetchedAt: snap.FetchedAt,
		entries:   make([]prEntry, 0, len(snap.PRs)),
	}

	s.mu.RLock()
	prevTabs := s.prTabSignal[key]
	s.mu.RUnlock()

	for _, pr := range snap.PRs {
		entry := prEntry{
			repo:      repoKey(effectiveRepo),
			number:    pr.Number,
			title:     pr.Title,
			branch:    pr.HeadRefName,
			author:    pr.Author,
			updatedAt: pr.UpdatedAt,
			tabs:      defaultTabFlags(pr),
		}
		if len(prevTabs[pr.Number]) > 0 {
			if entry.tabs == nil {
				entry.tabs = make(map[domain.DashboardTab]bool)
			}
			for tab := range prevTabs[pr.Number] {
				entry.tabs[tab] = true
			}
		}
		index.entries = append(index.entries, entry)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureMapsLocked()
	s.prIndexes[key] = index
	if s.currentRepo == "" {
		s.currentRepo = key
	}
	if _, ok := s.prTabSignal[key]; !ok {
		s.prTabSignal[key] = make(map[int]map[domain.DashboardTab]bool)
	}
	for _, entry := range index.entries {
		if _, ok := s.prTabSignal[key][entry.number]; !ok {
			s.prTabSignal[key][entry.number] = make(map[domain.DashboardTab]bool)
		}
		for tab := range entry.tabs {
			s.prTabSignal[key][entry.number][tab] = true
		}
	}
	return nil
}

// BuildRepoIndex replaces the repo search index.
func (s *Service) BuildRepoIndex(repos []domain.Repository) error {
	entries := make([]repoEntry, 0, len(repos))
	for _, repo := range repos {
		entries = append(entries, repoEntry{repo: repo})
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureMapsLocked()
	s.repoIndex = entries
	return nil
}

// SearchPRs returns ranked PR results from hydrated repositories.
func (s *Service) SearchPRs(query string, limit int) []domain.SearchResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		return nil
	}

	q := normalizeQuery(query)
	now := time.Now()
	currentRepo := s.currentRepo
	currentTab := s.currentTab

	results := make([]scoredPR, 0)
	for _, index := range s.prIndexes {
		for _, entry := range index.entries {
			score := scorePR(entry, q, currentRepo, currentTab, now)
			if !matchesPR(entry, q) && q != "" {
				// Keep exact number matches and fuzzy matches only.
				continue
			}
			results = append(results, scoredPR{
				result: domain.SearchResult{
					Kind:   domain.SearchResultPR,
					Repo:   entry.repo,
					Number: entry.number,
					Title:  entry.title,
					Branch: entry.branch,
					Author: entry.author,
					Score:  score,
				},
				updated: entry.updatedAt,
			})
		}
	}

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].result.Score != results[j].result.Score {
			return results[i].result.Score > results[j].result.Score
		}
		if !results[i].updated.Equal(results[j].updated) {
			return results[i].updated.After(results[j].updated)
		}
		if results[i].result.Repo != results[j].result.Repo {
			return results[i].result.Repo < results[j].result.Repo
		}
		return results[i].result.Number < results[j].result.Number
	})

	return scoredPRResults(results, limit)
}

// SearchRepos returns ranked repository results from discovery data.
func (s *Service) SearchRepos(query string, limit int) []domain.SearchResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		return nil
	}

	q := normalizeQuery(query)
	currentRepo := s.currentRepo

	results := make([]scoredRepo, 0, len(s.repoIndex))
	for _, entry := range s.repoIndex {
		score := scoreRepo(entry.repo, q, currentRepo)
		if !matchesRepo(entry.repo, q) && q != "" {
			continue
		}
		results = append(results, scoredRepo{
			result: domain.SearchResult{
				Kind:   domain.SearchResultRepo,
				Repo:   repoKey(entry.repo),
				Title:  repoKey(entry.repo),
				Branch: entry.repo.LocalPath,
				Score:  score,
			},
		})
	}

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].result.Score != results[j].result.Score {
			return results[i].result.Score > results[j].result.Score
		}
		if results[i].result.Title != results[j].result.Title {
			return results[i].result.Title < results[j].result.Title
		}
		return results[i].result.Branch < results[j].result.Branch
	})

	out := make([]domain.SearchResult, 0, min(limit, len(results)))
	for _, res := range results {
		out = append(out, res.result)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func scoredPRResults(results []scoredPR, limit int) []domain.SearchResult {
	out := make([]domain.SearchResult, 0, min(limit, len(results)))
	for _, res := range results {
		out = append(out, res.result)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func scorePR(entry prEntry, query, currentRepo string, currentTab domain.DashboardTab, now time.Time) float64 {
	score := 0.0
	if query == "" {
		score += recencyScore(entry.updatedAt, now)
	} else {
		if n, err := strconv.Atoi(query); err == nil && n == entry.number {
			score += exactPRNumberBoost
		}
		title := normalizeQuery(entry.title)
		branch := normalizeQuery(entry.branch)
		author := normalizeQuery(entry.author)
		repo := normalizeQuery(entry.repo)

		switch {
		case strings.HasPrefix(title, query):
			score += titlePrefixBoost
		case strings.Contains(title, query):
			score += titleSubstringBoost
		}

		switch {
		case strings.HasPrefix(branch, query):
			score += branchPrefixBoost
		case strings.Contains(branch, query):
			score += branchSubstringBoost
		}

		if strings.Contains(author, query) {
			score += authorSubstringBoost
		}
		if strings.Contains(repo, query) {
			score += repoSubstringBoost
		}

		if strings.Contains(title, query) || strings.Contains(branch, query) || strings.Contains(author, query) || strings.Contains(repo, query) {
			score += recencyScore(entry.updatedAt, now)
		}
	}

	if currentRepo != "" && canonicalRepoKey(entry.repo) == currentRepo {
		score += currentRepoBoost
	}
	if currentTab != "" && entry.tabs != nil && entry.tabs[currentTab] {
		score += currentTabBoost
	}
	return score
}

func scoreRepo(repo domain.Repository, query, currentRepo string) float64 {
	score := 0.0
	if query == "" {
		if canonicalRepoKey(repoKey(repo)) == currentRepo {
			score += currentRepoBoost
		}
		return score
	}

	full := normalizeQuery(repoKey(repo))
	path := normalizeQuery(repo.LocalPath)

	switch {
	case full == query:
		score += exactPRNumberBoost
	case strings.HasPrefix(full, query):
		score += titlePrefixBoost
	case strings.HasPrefix(path, query):
		score += branchPrefixBoost
	case strings.Contains(full, query):
		score += titleSubstringBoost
	case strings.Contains(path, query):
		score += branchSubstringBoost
	}

	if canonicalRepoKey(repoKey(repo)) == currentRepo {
		score += currentRepoBoost
	}
	return score
}

func matchesPR(entry prEntry, query string) bool {
	if query == "" {
		return true
	}
	if n, err := strconv.Atoi(query); err == nil && n == entry.number {
		return true
	}
	title := normalizeQuery(entry.title)
	branch := normalizeQuery(entry.branch)
	author := normalizeQuery(entry.author)
	repo := normalizeQuery(entry.repo)
	return strings.Contains(title, query) || strings.Contains(branch, query) || strings.Contains(author, query) || strings.Contains(repo, query)
}

func matchesRepo(repo domain.Repository, query string) bool {
	if query == "" {
		return true
	}
	full := normalizeQuery(repoKey(repo))
	path := normalizeQuery(repo.LocalPath)
	return strings.Contains(full, query) || strings.Contains(path, query)
}

func defaultTabFlags(pr domain.PullRequestSummary) map[domain.DashboardTab]bool {
	tabs := map[domain.DashboardTab]bool{
		domain.TabRecent: true,
	}
	if len(pr.RequestedReviewers) > 0 || len(pr.AssigneeLogins) > 0 || pr.UnresolvedCount > 0 || pr.ReviewThreadCount > 0 {
		tabs[domain.TabInvolving] = true
	}
	if pr.State == domain.PRStateOpen && (len(pr.RequestedReviewers) > 0 || len(pr.AssigneeLogins) > 0) && pr.ReviewDecision != domain.ReviewDecisionApproved {
		tabs[domain.TabNeedsReview] = true
	}
	return tabs
}

func recencyScore(updatedAt, now time.Time) float64 {
	if updatedAt.IsZero() {
		return 0
	}
	age := now.Sub(updatedAt)
	if age <= 0 {
		return 100
	}
	hours := age.Hours()
	if hours >= 240 {
		return 0
	}
	return 100 - (hours / 2.4)
}

func normalizeQuery(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func canonicalRepoKey(repo string) string {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return ""
	}
	return strings.ToLower(repo)
}

func repoKey(repo domain.Repository) string {
	if repo.FullName != "" {
		return repo.FullName
	}
	if repo.Owner != "" && repo.Name != "" {
		return repo.Owner + "/" + repo.Name
	}
	return repo.Name
}

func (s *Service) ensureMapsLocked() {
	if s.prIndexes == nil {
		s.prIndexes = make(map[string]*prIndex)
	}
	if s.prTabSignal == nil {
		s.prTabSignal = make(map[string]map[int]map[domain.DashboardTab]bool)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
