// mockgh: a tiny local mock of the GitHub GraphQL + REST API for pho.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---------- fixture types ----------

type Fixture struct {
	Viewer   string              `json:"viewer"`
	ReposDir string              `json:"reposDir"` // default: ../world/repos relative to fixture file
	People   map[string]string   `json:"people"`   // git author email -> login
	Repos    map[string]*RepoFix `json:"repos"`    // "owner/name"
	PRs      []*PR               `json:"prs"`
}

type RepoFix struct {
	Dir           string     `json:"dir"` // dir name under reposDir
	DefaultBranch string     `json:"defaultBranch"`
	Extra         []*ExtraPR `json:"extraPRs"`
}

type ExtraPR struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	Author  string `json:"author"`
	State   string `json:"state"` // MERGED | CLOSED
	Updated string `json:"updated"`
	t       time.Time
}

type Comment struct {
	ID     string `json:"-"`
	Author string `json:"author"`
	Body   string `json:"body"`
	At     string `json:"at"`
	t      time.Time
}

type Review struct {
	ID     string `json:"-"`
	Author string `json:"author"`
	State  string `json:"state"` // APPROVED | CHANGES_REQUESTED | COMMENTED
	Body   string `json:"body"`
	At     string `json:"at"`
	t      time.Time
}

type Thread struct {
	ID         string     `json:"-"`
	Path       string     `json:"path"`
	Line       int        `json:"line"`
	Resolved   bool       `json:"resolved"`
	ResolvedBy string     `json:"resolvedBy"`
	Comments   []*Comment `json:"comments"`
}

type Label struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

type Check struct {
	Name              string `json:"name"`
	Status            string `json:"status"`     // COMPLETED | IN_PROGRESS | QUEUED
	Conclusion        string `json:"conclusion"` // SUCCESS | FAILURE | ...
	TurnsSuccessAfter int    `json:"turnsSuccessAfter"`
}

type PR struct {
	Repo             string     `json:"repo"`
	Number           int        `json:"number"`
	Branch           string     `json:"branch"`
	Base             string     `json:"base"`
	Title            string     `json:"title"`
	Body             string     `json:"body"`
	Author           string     `json:"author"`
	State            string     `json:"state"`
	IsDraft          bool       `json:"isDraft"`
	Labels           []Label    `json:"labels"`
	Assignees        []string   `json:"assignees"`
	ReviewRequests   []string   `json:"reviewRequests"`
	Reviews          []*Review  `json:"reviews"`
	Threads          []*Thread  `json:"reviewThreads"`
	Comments         []*Comment `json:"comments"`
	Checks           []*Check   `json:"checks"`
	Mergeable        string     `json:"mergeable"`
	MergeStateStatus string     `json:"mergeStateStatus"`
	ReviewDecision   *string    `json:"reviewDecision"`
	Created          string     `json:"created"`
	Updated          string     `json:"updated"`

	created, updated, mergedAt time.Time
	// from git
	diff    string
	files   []ChangedFile
	headOid string
	commits []GitCommit
}

type ChangedFile struct {
	Filename         string `json:"filename"`
	PreviousFilename string `json:"previous_filename,omitempty"`
	Status           string `json:"status"`
	Additions        int    `json:"additions"`
	Deletions        int    `json:"deletions"`
	Changes          int    `json:"changes"`
	Patch            string `json:"patch,omitempty"`
}

type GitCommit struct {
	SHA, Name, Email, Subject, Body string
	At                              time.Time
}

// ---------- server ----------

type Server struct {
	mu       sync.Mutex
	fx       *Fixture
	start    time.Time
	reposDir string
	idSeq    int
	cdiffs   map[string]string // repo/sha -> diff
}

var (
	flagAddr    = flag.String("addr", "127.0.0.1:8787", "listen address")
	flagFixture = flag.String("fixture", "", "fixture JSON (required)")
	flagLatency = flag.Duration("latency", 100*time.Millisecond, "artificial latency per request")
)

func main() {
	flag.Parse()
	if *flagFixture == "" {
		log.Fatal("-fixture is required")
	}
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	s := &Server{start: time.Now(), cdiffs: map[string]string{}}
	if err := s.load(*flagFixture); err != nil {
		log.Fatalf("load fixture: %v", err)
	}
	log.Printf("mockgh listening on http://%s (%d PRs, fixture %s)", *flagAddr, len(s.fx.PRs), *flagFixture)
	log.Fatal(http.ListenAndServe(*flagAddr, http.HandlerFunc(s.serve)))
}

var relRE = regexp.MustCompile(`(\d+)([smhdw])`)

// rel resolves "-2h15m", "-3d", "+1h", "" (=now) or an RFC3339 timestamp.
func rel(now time.Time, s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return now
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	sign := time.Duration(1)
	if strings.HasPrefix(s, "-") {
		sign = -1
	}
	var d time.Duration
	for _, m := range relRE.FindAllStringSubmatch(s, -1) {
		n, _ := strconv.Atoi(m[1])
		unit := map[string]time.Duration{"s": time.Second, "m": time.Minute, "h": time.Hour, "d": 24 * time.Hour, "w": 7 * 24 * time.Hour}[m[2]]
		d += time.Duration(n) * unit
	}
	return now.Add(sign * d)
}

func (s *Server) nextID(prefix string) string {
	s.idSeq++
	return fmt.Sprintf("%s_mock%04d", prefix, s.idSeq)
}

func (s *Server) load(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var fx Fixture
	if err := json.Unmarshal(b, &fx); err != nil {
		return err
	}
	if fx.Viewer == "" {
		fx.Viewer = "kmiller"
	}
	if fx.ReposDir == "" {
		fx.ReposDir = filepath.Join(filepath.Dir(path), "..", "repos")
	} else if !filepath.IsAbs(fx.ReposDir) {
		fx.ReposDir = filepath.Join(filepath.Dir(path), fx.ReposDir)
	}
	s.reposDir = fx.ReposDir
	s.fx = &fx
	for name, r := range fx.Repos {
		if r.Dir == "" {
			r.Dir = name[strings.Index(name, "/")+1:]
		}
		if r.DefaultBranch == "" {
			r.DefaultBranch = "main"
		}
		for _, e := range r.Extra {
			e.t = rel(s.start, e.Updated)
		}
	}
	for _, pr := range fx.PRs {
		if pr.State == "" {
			pr.State = "OPEN"
		}
		if pr.Mergeable == "" {
			pr.Mergeable = "MERGEABLE"
		}
		if pr.Base == "" {
			pr.Base = s.repoFix(pr.Repo).DefaultBranch
		}
		pr.created = rel(s.start, pr.Created)
		pr.updated = rel(s.start, pr.Updated)
		if pr.Updated == "" {
			pr.updated = pr.created
		}
		for _, c := range pr.Comments {
			c.ID, c.t = s.nextID("IC"), rel(s.start, c.At)
		}
		for _, r := range pr.Reviews {
			r.ID, r.t = s.nextID("PRR"), rel(s.start, r.At)
		}
		for _, t := range pr.Threads {
			t.ID = s.nextID("PRRT")
			for _, c := range t.Comments {
				c.ID, c.t = s.nextID("PRRC"), rel(s.start, c.At)
			}
		}
		s.loadGit(pr)
		log.Printf("loaded %s#%d branch=%s files=%d commits=%d head=%.7s", pr.Repo, pr.Number, pr.Branch, len(pr.files), len(pr.commits), pr.headOid)
	}
	return nil
}

func (s *Server) repoFix(full string) *RepoFix {
	if r, ok := s.fx.Repos[full]; ok {
		return r
	}
	name := full[strings.Index(full, "/")+1:]
	r := &RepoFix{Dir: name, DefaultBranch: "main"}
	if s.fx.Repos == nil {
		s.fx.Repos = map[string]*RepoFix{}
	}
	s.fx.Repos[full] = r
	return r
}

func (s *Server) repoPath(full string) string {
	return filepath.Join(s.reposDir, s.repoFix(full).Dir)
}

func (s *Server) findPR(full string, n int) *PR {
	for _, p := range s.fx.PRs {
		if strings.EqualFold(p.Repo, full) && p.Number == n {
			return p
		}
	}
	return nil
}

func (s *Server) findPRByID(id string) *PR {
	for _, p := range s.fx.PRs {
		if prID(p) == id {
			return p
		}
	}
	return nil
}

func prID(p *PR) string {
	return fmt.Sprintf("PR_%s_%d", strings.ReplaceAll(p.Repo, "/", "_"), p.Number)
}

// ---------- HTTP plumbing ----------

type rec struct {
	http.ResponseWriter
	code int
	note string
}

func (r *rec) WriteHeader(c int) { r.code = c; r.ResponseWriter.WriteHeader(c) }

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	t0 := time.Now()
	time.Sleep(*flagLatency)
	rc := &rec{ResponseWriter: w, code: 200}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case r.URL.Path == "/graphql" && r.Method == "POST":
		s.graphql(rc, r)
	case strings.HasPrefix(r.URL.Path, "/repos/"):
		s.rest(rc, r)
	default:
		rc.note = "UNKNOWN ROUTE"
		jsonOut(rc, 404, map[string]any{"message": "mockgh: unknown route " + r.Method + " " + r.URL.Path})
	}
	tag := ""
	if rc.code >= 400 {
		tag = " <<< MOCK GAP/ERROR"
	}
	log.Printf("%s %s%s -> %d %s (%dms)%s", r.Method, r.URL.Path, qs(r), rc.code, rc.note, time.Since(t0).Milliseconds(), tag)
}

func qs(r *http.Request) string {
	if r.URL.RawQuery == "" {
		return ""
	}
	return "?" + r.URL.RawQuery
}

func jsonOut(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func sortedPRs(prs []*PR) {
	sort.SliceStable(prs, func(i, j int) bool { return prs[i].updated.After(prs[j].updated) })
}
