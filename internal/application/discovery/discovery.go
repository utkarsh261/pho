// Package discovery scans the local filesystem for Git repositories and turns
// remote URLs into canonical repository identities used by the application.
package discovery

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/utkarsh261/pho/internal/domain"
	pholog "github.com/utkarsh261/pho/internal/log"
)

type DiscoveryService interface {
	Discover(ctx context.Context, root string) ([]domain.Repository, error)
}

type Config struct {
	Pin     []string
	Exclude []string
	// KnownHosts are the hosts gh is logged in to; remotes on them are used as-is.
	KnownHosts []string
	// HostAliases maps a remote host to the GitHub host to use instead.
	HostAliases map[string]string
}

type Service struct {
	mu      sync.Mutex
	pinned  map[string]struct{}
	exclude map[string]struct{}
	cache   map[string][]domain.Repository
	Log     *pholog.Logger

	knownHosts  map[string]struct{}
	hostAliases map[string]string
	hostMu      sync.Mutex
	sshHosts    map[string]string
	// SSHHostname returns ssh's HostName for alias, or "" if unknown.
	SSHHostname func(ctx context.Context, alias string) string
}

func New(cfg Config) *Service {
	known := map[string]struct{}{"github.com": {}}
	for _, h := range cfg.KnownHosts {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			known[h] = struct{}{}
		}
	}
	aliases := make(map[string]string, len(cfg.HostAliases))
	for alias, host := range cfg.HostAliases {
		aliases[strings.ToLower(alias)] = strings.ToLower(host)
	}
	return &Service{
		pinned:      normalizeRepoSet(cfg.Pin),
		exclude:     normalizeRepoSet(cfg.Exclude),
		cache:       make(map[string][]domain.Repository),
		knownHosts:  known,
		hostAliases: aliases,
		sshHosts:    make(map[string]string),
		SSHHostname: sshHostname,
	}
}

func (s *Service) Discover(ctx context.Context, root string) ([]domain.Repository, error) {
	defer s.log().Timer("discovery scan", "root", root)()
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("discovery: resolve root: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if cached, ok := s.cached(absRoot); ok {
		return cached, nil
	}

	candidates, err := s.scan(ctx, absRoot)
	if err != nil {
		if cached, ok := s.loadDiskCache(absRoot); ok {
			s.store(absRoot, cached)
			return cached, nil
		}
		if cached, ok := s.cached(absRoot); ok {
			return cached, nil
		}
		return nil, err
	}

	repos := s.filterAndDedup(absRoot, candidates)
	s.store(absRoot, repos)
	_ = s.saveDiskCache(absRoot, repos)
	return repos, nil
}

type candidate struct {
	repo     domain.Repository
	key      string
	isPinned bool
	isCWD    bool
}

func (s *Service) scan(ctx context.Context, root string) ([]candidate, error) {
	entries := []string{root}

	dirEntries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("discovery: read %s: %w", root, err)
	}
	for _, entry := range dirEntries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !entry.IsDir() {
			continue
		}
		entries = append(entries, filepath.Join(root, entry.Name()))
	}

	var out []candidate
	for _, dir := range entries {
		repo, ok, err := repoFromPath(dir)
		if err != nil || !ok {
			continue // skip unreadable or non-GitHub repos
		}
		if host := s.resolveHost(ctx, repo.Host, isSSHRemote(repo.RemoteURL)); host != repo.Host {
			s.log().Debug("remote host resolved", "remote_host", repo.Host, "host", host, "path", repo.LocalPath)
			repo.Host = host
			repo.ID = host + "/" + repo.FullName
		}
		key := repo.FullName
		if _, excluded := s.exclude[key]; excluded {
			continue
		}
		activity, _ := repoActivityAt(dir) // best-effort
		repo.LastScannedAt = time.Now().UTC()
		if !activity.IsZero() {
			a := activity.UTC()
			repo.LastActivityAt = &a
		}
		out = append(out, candidate{
			repo:     repo,
			key:      key,
			isPinned: s.isPinned(key),
			isCWD:    dir == root,
		})
	}
	return out, nil
}

func (s *Service) filterAndDedup(root string, candidates []candidate) []domain.Repository {
	best := make(map[string]candidate)
	for _, cand := range candidates {
		if existing, ok := best[cand.key]; ok {
			if betterCandidate(cand, existing) {
				best[cand.key] = cand
			}
			continue
		}
		best[cand.key] = cand
	}

	out := make([]candidate, 0, len(best))
	for _, cand := range best {
		out = append(out, cand)
	}

	sort.SliceStable(out, func(i, j int) bool {
		return candidateLess(out[i], out[j])
	})

	repos := make([]domain.Repository, 0, len(out))
	for _, cand := range out {
		repos = append(repos, cand.repo)
	}
	return repos
}

func (s *Service) isPinned(key string) bool {
	_, ok := s.pinned[key]
	return ok
}

func (s *Service) cached(root string) ([]domain.Repository, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	repos, ok := s.cache[root]
	if !ok {
		return nil, false
	}
	out := make([]domain.Repository, len(repos))
	copy(out, repos)
	return out, true
}

func (s *Service) store(root string, repos []domain.Repository) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.Repository, len(repos))
	copy(out, repos)
	s.cache[root] = out
}

type diskCache struct {
	Root    string              `json:"root"`
	Pin     []string            `json:"pin"`
	Exclude []string            `json:"exclude"`
	Repos   []domain.Repository `json:"repos"`
}

func (s *Service) cacheFile(root string) string {
	sum := sha256.Sum256([]byte(root + "\x00" + strings.Join(sortedKeys(s.pinned), ",") + "\x00" + strings.Join(sortedKeys(s.exclude), ",")))
	return filepath.Join(os.TempDir(), "pho-discovery", fmt.Sprintf("%x.json", sum[:]))
}

func (s *Service) saveDiskCache(root string, repos []domain.Repository) error {
	payload := diskCache{
		Root:    root,
		Pin:     sortedKeys(s.pinned),
		Exclude: sortedKeys(s.exclude),
		Repos:   append([]domain.Repository(nil), repos...),
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("discovery: marshal cache: %w", err)
	}
	path := s.cacheFile(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("discovery: mkdir cache dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("discovery: write cache %s: %w", path, err)
	}
	return nil
}

func (s *Service) loadDiskCache(root string) ([]domain.Repository, bool) {
	path := s.cacheFile(root)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var payload diskCache
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, false
	}
	if payload.Root != root {
		return nil, false
	}
	if !equalStringSets(payload.Pin, sortedKeys(s.pinned)) || !equalStringSets(payload.Exclude, sortedKeys(s.exclude)) {
		return nil, false
	}
	out := make([]domain.Repository, len(payload.Repos))
	copy(out, payload.Repos)
	return out, true
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func equalStringSets(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aa := append([]string(nil), a...)
	bb := append([]string(nil), b...)
	sort.Strings(aa)
	sort.Strings(bb)
	for i := range aa {
		if aa[i] != bb[i] {
			return false
		}
	}
	return true
}

func betterCandidate(a, b candidate) bool {
	if a.isPinned != b.isPinned {
		return a.isPinned
	}
	if a.isCWD != b.isCWD {
		return a.isCWD
	}
	ta := activityValue(a.repo.LastActivityAt)
	tb := activityValue(b.repo.LastActivityAt)
	if !ta.Equal(tb) {
		return ta.After(tb)
	}
	if a.repo.Host != b.repo.Host {
		return strings.ToLower(a.repo.Host) < strings.ToLower(b.repo.Host)
	}
	return strings.ToLower(a.repo.LocalPath) < strings.ToLower(b.repo.LocalPath)
}

func candidateLess(a, b candidate) bool {
	if a.isPinned != b.isPinned {
		return a.isPinned
	}
	if a.isCWD != b.isCWD {
		return a.isCWD
	}
	ta := activityValue(a.repo.LastActivityAt)
	tb := activityValue(b.repo.LastActivityAt)
	if !ta.Equal(tb) {
		return ta.After(tb)
	}
	if a.repo.FullName != b.repo.FullName {
		return strings.ToLower(a.repo.FullName) < strings.ToLower(b.repo.FullName)
	}
	if a.repo.Host != b.repo.Host {
		return strings.ToLower(a.repo.Host) < strings.ToLower(b.repo.Host)
	}
	return strings.ToLower(a.repo.LocalPath) < strings.ToLower(b.repo.LocalPath)
}

func activityValue(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

func (s *Service) log() *pholog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return pholog.NewNop()
}

func normalizeRepoSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, v := range values {
		key := normalizeRepoKey(v)
		if key == "" {
			continue
		}
		out[key] = struct{}{}
	}
	return out
}

func normalizeRepoKey(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimSuffix(value, ".git")
	if value == "" {
		return ""
	}
	value = strings.Trim(value, "/")
	parts := strings.Split(value, "/")
	parts = compactParts(parts)
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-2] + "/" + parts[len(parts)-1]
}

func compactParts(parts []string) []string {
	out := parts[:0]
	for _, part := range parts {
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func repoFromPath(dir string) (domain.Repository, bool, error) {
	gitDir, ok, err := locateGitDir(dir)
	if err != nil || !ok {
		return domain.Repository{}, ok, err
	}
	remote, err := remoteURLFromGitDir(gitDir)
	if err != nil {
		return domain.Repository{}, false, err
	}
	if remote == "" {
		return domain.Repository{}, false, nil // no remote — skip
	}
	host, owner, name, ok := parseRemoteURL(remote)
	if !ok {
		return domain.Repository{}, false, nil
	}
	absPath, err := filepath.Abs(dir)
	if err != nil {
		return domain.Repository{}, false, err
	}
	fullName := owner + "/" + name
	return domain.Repository{
		ID:        host + "/" + fullName,
		Host:      host,
		Owner:     owner,
		Name:      name,
		FullName:  fullName,
		LocalPath: absPath,
		RemoteURL: remote,
	}, true, nil
}

func locateGitDir(dir string) (string, bool, error) {
	gitPath := filepath.Join(dir, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("discovery: stat %s: %w", gitPath, err)
	}
	if info.IsDir() {
		return gitPath, true, nil
	}

	data, err := os.ReadFile(gitPath)
	if err != nil {
		return "", false, fmt.Errorf("discovery: read %s: %w", gitPath, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "gitdir:") {
			continue
		}
		path := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}
		return filepath.Clean(path), true, nil
	}
	return "", false, nil
}

func remoteURLFromGitDir(gitDir string) (string, error) {
	cfgPath := filepath.Join(gitDir, "config")
	f, err := os.Open(cfgPath)
	if err != nil {
		return "", fmt.Errorf("discovery: open %s: %w", cfgPath, err)
	}
	defer f.Close()

	var first, origin string
	var currentSection string
	var currentName string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection, currentName = parseGitSection(line)
			continue
		}
		if !strings.HasPrefix(strings.ToLower(line), "url =") {
			continue
		}
		urlValue := strings.TrimSpace(line[len("url ="):])
		if urlValue == "" {
			continue
		}
		if first == "" {
			first = urlValue
		}
		if currentSection == "remote" && currentName == "origin" && origin == "" {
			origin = urlValue
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("discovery: scan %s: %w", cfgPath, err)
	}
	if origin != "" {
		return origin, nil
	}
	if first != "" {
		return first, nil
	}
	return "", nil // no remote configured — caller will skip this repo
}

func parseGitSection(line string) (section, name string) {
	inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
	if inner == "" {
		return "", ""
	}
	if i := strings.IndexByte(inner, ' '); i >= 0 {
		section = strings.TrimSpace(inner[:i])
		name = strings.Trim(inner[i+1:], `"`+" ")
		return section, name
	}
	return inner, ""
}

func parseRemoteURL(raw string) (host, owner, name string, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", "", false
	}

	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", "", "", false
		}
		host = strings.ToLower(u.Hostname())
		return splitRemotePath(host, u.Path)
	}

	// SCP-like syntax: git@github.com:org/repo.git
	if i := strings.IndexByte(raw, ':'); i >= 0 {
		left := raw[:i]
		right := raw[i+1:]
		if at := strings.LastIndexByte(left, '@'); at >= 0 {
			left = left[at+1:]
		}
		host = strings.ToLower(strings.TrimSpace(left))
		return splitRemotePath(host, right)
	}

	return "", "", "", false
}

// resolveHost maps a remote's host to the GitHub host behind it, so a remote
// like git@work.github.com:org/repo (an ~/.ssh/config alias) reaches the API
// on github.com.
func (s *Service) resolveHost(ctx context.Context, host string, ssh bool) string {
	if mapped, ok := s.hostAliases[host]; ok {
		return mapped
	}
	if _, ok := s.knownHosts[host]; ok || !ssh {
		return host
	}

	s.hostMu.Lock()
	defer s.hostMu.Unlock()
	if resolved, ok := s.sshHosts[host]; ok {
		return resolved
	}
	resolved := host
	if s.SSHHostname != nil && validSSHHost(host) {
		if h := strings.ToLower(strings.TrimSpace(s.SSHHostname(ctx, host))); h != "" {
			resolved = h
		}
	}
	// GitHub's SSH-over-443 endpoint; the API still lives on github.com.
	if resolved == "ssh.github.com" {
		resolved = "github.com"
	}
	s.sshHosts[host] = resolved
	return resolved
}

func isSSHRemote(raw string) bool {
	raw = strings.TrimSpace(raw)
	scheme, _, ok := strings.Cut(raw, "://")
	if !ok {
		return strings.Contains(raw, ":") // SCP-like syntax always uses ssh
	}
	switch strings.ToLower(scheme) {
	case "ssh", "git+ssh", "ssh+git":
		return true
	}
	return false
}

// validSSHHost rejects anything ssh could read as an option or that no
// ~/.ssh/config Host pattern would name; the value comes from a repo's config.
func validSSHHost(host string) bool {
	if host == "" || host[0] == '-' {
		return false
	}
	for _, r := range host {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

// sshHostname asks ssh how it would expand alias; ssh -G prints the resolved
// config without connecting.
func sshHostname(ctx context.Context, alias string) string {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ssh", "-G", alias).Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if v, ok := strings.CutPrefix(line, "hostname "); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func splitRemotePath(host, p string) (string, string, string, bool) {
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, "/")
	p = strings.TrimSuffix(p, ".git")
	p = strings.Trim(p, "/")
	if p == "" {
		return "", "", "", false
	}
	parts := compactParts(strings.Split(p, "/"))
	if len(parts) < 2 {
		return "", "", "", false
	}
	owner := strings.ToLower(parts[0])
	name := strings.ToLower(parts[1])
	return host, owner, name, true
}

func repoActivityAt(dir string) (time.Time, error) {
	gitPath := filepath.Join(dir, ".git")
	candidates := []string{dir, gitPath, filepath.Join(gitPath, "config")}
	var newest time.Time
	for _, p := range candidates {
		info, err := os.Stat(p)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return time.Time{}, fmt.Errorf("discovery: stat %s: %w", p, err)
		}
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	return newest, nil
}
