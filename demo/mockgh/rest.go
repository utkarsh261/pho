package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) rest(w *rec, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/") // repos o r ...
	if len(parts) < 3 {
		w.note = "UNKNOWN"
		jsonOut(w, 404, M{"message": "mockgh: bad path"})
		return
	}
	full := parts[1] + "/" + parts[2]
	rest := parts[3:]
	wantDiff := strings.Contains(r.Header.Get("Accept"), "diff")
	switch {
	case len(rest) == 0 && r.Method == "GET":
		w.note = "repo info"
		jsonOut(w, 200, M{"default_branch": s.repoFix(full).DefaultBranch, "fork": false, "full_name": full})
	case len(rest) == 1 && rest[0] == "pulls" && r.Method == "POST":
		s.createPR(w, r, full)
	case len(rest) >= 2 && rest[0] == "pulls":
		n, _ := strconv.Atoi(rest[1])
		p := s.findPR(full, n)
		if p == nil {
			w.note = "pr not found"
			jsonOut(w, 404, M{"message": "Not Found"})
			return
		}
		switch {
		case len(rest) == 2 && r.Method == "GET" && wantDiff:
			w.note = "pr diff"
			if len(p.files) > 300 {
				w.note += " (too_large)"
				jsonOut(w, 406, M{"message": "Sorry, the diff exceeded the maximum number of files (300). Consider using 'List pull requests files' API or locally cloning the repository instead.",
					"errors": []M{{"resource": "PullRequest", "field": "diff", "code": "too_large"}}})
				return
			}
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, p.diff)
		case len(rest) == 3 && rest[2] == "files" && r.Method == "GET":
			w.note = "pr files"
			pageFiles(w, r, p.files, func(f []ChangedFile) any { return f })
		case len(rest) == 3 && rest[2] == "update-branch" && r.Method == "PUT":
			w.note = "update-branch"
			jsonOut(w, 202, M{"message": "Updating pull request branch.", "url": "https://github.com/" + full})
		default:
			w.note = "UNSUPPORTED"
			jsonOut(w, 404, M{"message": "mockgh: unsupported pulls route"})
		}
	case len(rest) == 2 && rest[0] == "commits" && r.Method == "GET":
		d, ok := s.commitDiff(full, rest[1])
		if !ok {
			w.note = "commit not found"
			jsonOut(w, 404, M{"message": "No commit found for SHA: " + rest[1]})
			return
		}
		if wantDiff {
			w.note = "commit diff"
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, d)
			return
		}
		w.note = "commit files"
		pageFiles(w, r, parseFiles(d), func(f []ChangedFile) any { return M{"sha": rest[1], "files": f} })
	default:
		w.note = "UNSUPPORTED"
		jsonOut(w, 404, M{"message": "mockgh: unsupported REST route " + r.Method + " " + r.URL.Path})
	}
}

func pageFiles(w http.ResponseWriter, r *http.Request, files []ChangedFile, wrap func([]ChangedFile) any) {
	per, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if per <= 0 || per > 100 {
		per = 100
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page <= 0 {
		page = 1
	}
	last := (len(files) + per - 1) / per
	if last < 1 {
		last = 1
	}
	lo, hi := (page-1)*per, page*per
	if lo > len(files) {
		lo = len(files)
	}
	if hi > len(files) {
		hi = len(files)
	}
	base := fmt.Sprintf("http://%s%s", r.Host, r.URL.Path)
	var links []string
	if page < last {
		links = append(links, fmt.Sprintf(`<%s?per_page=%d&page=%d>; rel="next"`, base, per, page+1))
		links = append(links, fmt.Sprintf(`<%s?per_page=%d&page=%d>; rel="last"`, base, per, last))
	}
	if len(links) > 0 {
		w.Header().Set("Link", strings.Join(links, ", "))
	}
	out := files[lo:hi]
	if out == nil {
		out = []ChangedFile{}
	}
	jsonOut(w, 200, wrap(out))
}

func (s *Server) createPR(w *rec, r *http.Request, full string) {
	var in struct {
		Title, Head, Base, Body string
		Draft                   bool
	}
	json.NewDecoder(r.Body).Decode(&in)
	max := 0
	for _, p := range s.fx.PRs {
		if strings.EqualFold(p.Repo, full) && p.Number > max {
			max = p.Number
		}
	}
	for _, e := range s.repoFix(full).Extra {
		if e.Number > max {
			max = e.Number
		}
	}
	head := in.Head
	if i := strings.Index(head, ":"); i >= 0 {
		head = head[i+1:]
	}
	p := &PR{Repo: full, Number: max + 1, Branch: head, Base: in.Base, Title: in.Title, Body: in.Body, Author: s.fx.Viewer,
		State: "OPEN", IsDraft: in.Draft, Mergeable: "MERGEABLE", created: time.Now(), updated: time.Now()}
	if p.Base == "" {
		p.Base = s.repoFix(full).DefaultBranch
	}
	s.loadGit(p)
	s.fx.PRs = append(s.fx.PRs, p)
	w.note = fmt.Sprintf("create pr #%d", p.Number)
	jsonOut(w, 201, M{"number": p.Number, "title": p.Title, "body": p.Body, "state": "open", "draft": p.IsDraft,
		"html_url": fmt.Sprintf("https://github.com/%s/pull/%d", full, p.Number),
		"head":     M{"ref": p.Branch}, "base": M{"ref": p.Base}, "created_at": ts(p.created), "updated_at": ts(p.updated),
		"user": M{"login": p.Author}})
}
