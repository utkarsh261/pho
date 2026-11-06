// Package fakegh is an in-process GitHub API (GraphQL and REST) serving
// seeded PRs and recorded diffs, with injectable latency and a per-operation
// request counter. Only tests and benchmarks should import it.
package fakegh

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/utkarsh261/pho/internal/bench"
	"github.com/utkarsh261/pho/internal/domain"
	githubpkg "github.com/utkarsh261/pho/internal/github"
)

// Latency models the round trip to a GitHub host.
type Latency struct {
	Name   string
	Base   time.Duration
	Jitter time.Duration
}

var (
	Local   = Latency{Name: "local"}
	Fast    = Latency{Name: "fast", Base: 40 * time.Millisecond, Jitter: 10 * time.Millisecond}
	GHESVPN = Latency{Name: "ghes-vpn", Base: 250 * time.Millisecond, Jitter: 80 * time.Millisecond}
)

type Options struct {
	Repos      int
	PRsPerRepo int
	Diff       string // raw unified diff served for every PR
	FilesJSON  []byte // "list PR files" response served for every PR
	Latency    Latency
	Seed       uint64
}

type Server struct {
	*httptest.Server
	opts    Options
	repos   []domain.Repository
	prs     map[string][]domain.PullRequestSummary
	preview map[string]any

	mu    sync.Mutex
	rng   *rand.Rand
	count map[string]int
}

func New(opts Options) *Server {
	s := &Server{
		opts:  opts,
		repos: bench.Repos(opts.Repos),
		prs:   map[string][]domain.PullRequestSummary{},
		rng:   rand.New(rand.NewPCG(opts.Seed, 7)),
		count: map[string]int{},
	}
	for i, r := range s.repos {
		s.prs[r.FullName] = bench.PRs(r.FullName, opts.PRsPerRepo, opts.Seed+uint64(i))
	}
	if err := json.Unmarshal(mustFixture("preview"), &s.preview); err != nil {
		panic(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /graphql", s.graphql)
	mux.HandleFunc("GET /repos/{owner}/{name}/pulls/{num}", s.pullDiff)
	mux.HandleFunc("GET /repos/{owner}/{name}/pulls/{num}/files", s.pullFiles)
	mux.HandleFunc("GET /repos/{owner}/{name}/labels", s.emptyList("labels"))
	mux.HandleFunc("GET /repos/{owner}/{name}/issues/{num}/labels", s.emptyList("issue labels"))
	mux.HandleFunc("GET /repos/{owner}/{name}", s.repoInfo)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.hit("UNHANDLED " + r.Method + " " + r.URL.Path)
		http.NotFound(w, r)
	})
	s.Server = httptest.NewServer(mux)
	return s
}

// Repos are the repositories the server knows, for building a discovery root.
func (s *Server) Repos() []domain.Repository { return s.repos }

// Profile is an authenticated host pointing at this server.
func (s *Server) Profile() githubpkg.GitHubHostProfile {
	return githubpkg.GitHubHostProfile{
		Host:                   "github.com",
		GraphQLURL:             s.URL + "/graphql",
		RESTURL:                s.URL,
		Token:                  "fake",
		ViewerLogin:            "octocat",
		SupportsTopLevelRollup: true,
		SupportsHeadRefOID:     true,
		SupportsPRStack:        true,
	}
}

// WriteRoot creates one directory per repo under root whose git origin is
// that repo, as pho's discovery expects.
func (s *Server) WriteRoot(root string) error {
	for _, r := range s.repos {
		dir := filepath.Join(root, r.Name, ".git")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		cfg := fmt.Sprintf("[remote \"origin\"]\n\turl = https://github.com/%s.git\n", r.FullName)
		if err := os.WriteFile(filepath.Join(dir, "config"), []byte(cfg), 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// Requests returns the count per operation since the last Reset.
func (s *Server) Requests() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]int, len(s.count))
	for k, v := range s.count {
		out[k] = v
	}
	return out
}

// Total is the number of requests since the last Reset.
func (s *Server) Total() int {
	n := 0
	for _, v := range s.Requests() {
		n += v
	}
	return n
}

// Summary lists operations and counts, sorted, for logs.
func (s *Server) Summary() string {
	req := s.Requests()
	keys := make([]string, 0, len(req))
	for k := range req {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s=%d", k, req[k])
	}
	return strings.Join(parts, " ")
}

func (s *Server) Reset() {
	s.mu.Lock()
	s.count = map[string]int{}
	s.mu.Unlock()
}

func (s *Server) hit(op string) {
	s.mu.Lock()
	s.count[op]++
	d := s.opts.Latency.Base
	if j := s.opts.Latency.Jitter; j > 0 {
		d += time.Duration(s.rng.Int64N(int64(2*j))) - j
	}
	s.mu.Unlock()
	if d > 0 {
		time.Sleep(d)
	}
}

var opName = regexp.MustCompile(`^\s*(?:query|mutation)\s+([A-Za-z]+)`)

func (s *Server) graphql(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	op := "anonymous"
	if m := opName.FindStringSubmatch(req.Query); m != nil {
		op = m[1]
	}
	s.hit("graphql " + op)
	str := func(k string) string { v, _ := req.Variables[k].(string); return v }
	var data any
	switch op {
	case "ViewerQuery":
		data = map[string]any{"viewer": map[string]any{"login": "octocat"}}
	case "DashboardPRsQuery":
		data = s.repoPRs(str("owner")+"/"+str("name"), str("after"), true)
	case "AllPRsQuery":
		m := regexp.MustCompile(`repository\(owner: "([^"]+)", name: "([^"]+)"\)`).FindStringSubmatch(req.Query)
		after := ""
		if a := regexp.MustCompile(`after: "([^"]+)"`).FindStringSubmatch(req.Query); a != nil {
			after = a[1]
		}
		if m != nil {
			data = s.repoPRs(m[1]+"/"+m[2], after, false)
		}
	case "InvolvingPRsQuery":
		repo := ""
		if m := regexp.MustCompile(`repo:(\S+)`).FindStringSubmatch(str("query")); m != nil {
			repo = m[1]
		}
		prs := s.prs[repo]
		prs = prs[:min(len(prs), 10)]
		nodes := make([]any, len(prs))
		for i, p := range prs {
			n := prNode(p)
			n["__typename"] = "PullRequest"
			nodes[i] = n
		}
		data = map[string]any{"search": map[string]any{
			"issueCount": len(nodes),
			"pageInfo":   map[string]any{"hasNextPage": false, "endCursor": nil},
			"nodes":      nodes,
		}}
	case "PreviewQuery":
		data = s.previewFor(str("owner")+"/"+str("name"), int(num(req.Variables["number"])))
	case "CommitsQuery":
		data = map[string]any{"repository": map[string]any{"pullRequest": map[string]any{
			"commits": map[string]any{"nodes": []any{}},
		}}}
	default:
		data = map[string]any{}
	}
	writeJSON(w, map[string]any{"data": data})
}

func num(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return f
	}
	return 0
}

const pageSize = 100

func (s *Server) repoPRs(repo, after string, withTotal bool) any {
	prs := s.prs[repo]
	start, _ := strconv.Atoi(strings.TrimPrefix(after, "c"))
	end := min(start+pageSize, len(prs))
	start = min(start, end)
	nodes := make([]any, 0, end-start)
	for _, p := range prs[start:end] {
		nodes = append(nodes, prNode(p))
	}
	conn := map[string]any{
		"pageInfo": map[string]any{"hasNextPage": end < len(prs), "endCursor": fmt.Sprintf("c%d", end)},
		"nodes":    nodes,
	}
	if withTotal {
		conn["totalCount"] = len(prs)
	}
	return map[string]any{"repository": map[string]any{"nameWithOwner": repo, "pullRequests": conn}}
}

func prNode(p domain.PullRequestSummary) map[string]any {
	review := map[domain.ReviewDecision]string{
		domain.ReviewDecisionApproved:         "APPROVED",
		domain.ReviewDecisionChangesRequested: "CHANGES_REQUESTED",
		domain.ReviewDecisionReviewRequired:   "REVIEW_REQUIRED",
	}[p.ReviewDecision]
	ci := map[domain.CIStatus]string{
		domain.CIStatusSuccess: "SUCCESS", domain.CIStatusFailure: "FAILURE", domain.CIStatusPending: "PENDING",
	}[p.CIStatus]
	reqs := make([]any, len(p.RequestedReviewers))
	for i, l := range p.RequestedReviewers {
		reqs[i] = map[string]any{"requestedReviewer": map[string]any{"__typename": "User", "login": l}}
	}
	return map[string]any{
		"id": p.ID, "number": p.Number, "title": p.Title, "state": "OPEN", "isDraft": p.IsDraft,
		"createdAt": p.CreatedAt.Format(time.RFC3339), "updatedAt": p.UpdatedAt.Format(time.RFC3339),
		"headRefName": p.HeadRefName, "headRefOid": p.HeadRefOID, "baseRefName": p.BaseRefName,
		"additions": p.Additions, "deletions": p.Deletions, "changedFiles": p.FileCount,
		"comments":                 map[string]any{"totalCount": p.CommentCount},
		"reviewThreads":            map[string]any{"totalCount": 0},
		"labels":                   map[string]any{"totalCount": 0, "nodes": []any{}},
		"author":                   map[string]any{"__typename": "User", "login": p.Author},
		"assignees":                map[string]any{"nodes": []any{}},
		"reviewRequests":           map[string]any{"nodes": reqs},
		"latestOpinionatedReviews": map[string]any{"nodes": []any{}},
		"reviewDecision":           review,
		"repository":               map[string]any{"nameWithOwner": p.Repo},
		"statusCheckRollup":        map[string]any{"state": ci},
		"commits":                  map[string]any{"nodes": []any{map[string]any{"commit": map[string]any{"statusCheckRollup": map[string]any{"state": ci}}}}},
	}
}

func (s *Server) previewFor(repo string, number int) any {
	data, _ := s.preview["data"].(map[string]any)
	src, _ := data["repository"].(map[string]any)
	pr, _ := src["pullRequest"].(map[string]any)
	out := make(map[string]any, len(pr))
	for k, v := range pr {
		out[k] = v
	}
	out["number"] = number
	out["id"] = fmt.Sprintf("PR_%s_%d", repo, number)
	for _, p := range s.prs[repo] {
		if p.Number == number {
			out["title"], out["headRefName"], out["headRefOid"], out["baseRefName"] = p.Title, p.HeadRefName, p.HeadRefOID, p.BaseRefName
		}
	}
	return map[string]any{"repository": map[string]any{"nameWithOwner": repo, "pullRequest": out}}
}

func (s *Server) pullDiff(w http.ResponseWriter, r *http.Request) {
	s.hit("rest GET pulls/{n} (diff)")
	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write([]byte(s.opts.Diff))
}

func (s *Server) pullFiles(w http.ResponseWriter, r *http.Request) {
	s.hit("rest GET pulls/{n}/files")
	if page := r.URL.Query().Get("page"); page != "" && page != "1" {
		writeJSON(w, []any{})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(s.opts.FilesJSON)
}

func (s *Server) repoInfo(w http.ResponseWriter, r *http.Request) {
	s.hit("rest GET repo")
	writeJSON(w, map[string]any{
		"full_name": r.PathValue("owner") + "/" + r.PathValue("name"), "default_branch": "main",
		"allow_merge_commit": true, "allow_squash_merge": true, "allow_rebase_merge": true,
	})
}

func (s *Server) emptyList(op string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.hit("rest GET " + op)
		writeJSON(w, []any{})
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func mustFixture(name string) []byte {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..", "..")
	b, err := os.ReadFile(filepath.Join(root, "testdata", "graphql", name, "response.json"))
	if err != nil {
		panic(err)
	}
	return b
}
