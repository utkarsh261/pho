package main

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"
)

func git(dir string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	return string(out), err
}

var diffFlags = []string{"-c", "core.quotepath=off", "diff", "--no-color", "--no-ext-diff", "-M", "--src-prefix=a/", "--dst-prefix=b/"}

func (s *Server) loadGit(pr *PR) {
	dir := s.repoPath(pr.Repo)
	if pr.Branch == "" {
		return
	}
	oid, err := git(dir, "rev-parse", pr.Branch)
	if err != nil {
		log.Printf("WARN %s#%d: branch %q not found in %s", pr.Repo, pr.Number, pr.Branch, dir)
		pr.headOid = fmt.Sprintf("%040d", pr.Number)
		return
	}
	pr.headOid = strings.TrimSpace(oid)
	args := append(append([]string{}, diffFlags...), pr.Base+"..."+pr.Branch)
	pr.diff, _ = git(dir, args...)
	pr.files = parseFiles(pr.diff)
	out, _ := git(dir, "log", "--reverse", "--format=%H%x1f%an%x1f%ae%x1f%aI%x1f%s%x1f%b%x1e", pr.Base+".."+pr.Branch)
	pr.commits = nil
	for _, rec := range strings.Split(out, "\x1e") {
		rec = strings.TrimLeft(rec, "\n")
		f := strings.SplitN(rec, "\x1f", 6)
		if len(f) < 6 {
			continue
		}
		t, _ := time.Parse(time.RFC3339, f[3])
		pr.commits = append(pr.commits, GitCommit{f[0], f[1], f[2], f[4], strings.TrimSpace(f[5]), t.Add(s.shift)})
	}
}

// parseFiles splits a unified git diff into per-file entries like GitHub's files API.
func parseFiles(diff string) []ChangedFile {
	var files []ChangedFile
	if diff == "" {
		return files
	}
	sections := strings.Split("\n"+diff, "\ndiff --git ")[1:]
	for _, sec := range sections {
		lines := strings.Split(sec, "\n")
		f := ChangedFile{Status: "modified"}
		if i := strings.LastIndex(lines[0], " b/"); i >= 0 {
			f.Filename = lines[0][i+3:]
		}
		patchStart := -1
		for i, l := range lines[1:] {
			switch {
			case strings.HasPrefix(l, "new file mode"):
				f.Status = "added"
			case strings.HasPrefix(l, "deleted file mode"):
				f.Status = "removed"
			case strings.HasPrefix(l, "rename from "):
				f.Status = "renamed"
				f.PreviousFilename = strings.TrimPrefix(l, "rename from ")
			case strings.HasPrefix(l, "@@") && patchStart < 0:
				patchStart = i + 1
			}
		}
		if patchStart >= 0 {
			patch := strings.TrimRight(strings.Join(lines[patchStart:], "\n"), "\n")
			f.Patch = patch
			for _, l := range strings.Split(patch, "\n") {
				if strings.HasPrefix(l, "+") {
					f.Additions++
				} else if strings.HasPrefix(l, "-") {
					f.Deletions++
				}
			}
		}
		f.Changes = f.Additions + f.Deletions
		files = append(files, f)
	}
	return files
}

func (s *Server) commitDiff(repo, sha string) (string, bool) {
	key := repo + "/" + sha
	if d, ok := s.cdiffs[key]; ok {
		return d, true
	}
	args := append(append([]string{}, diffFlags...), sha+"^", sha)
	d, err := git(s.repoPath(repo), args...)
	if err != nil { // root commit
		d, err = git(s.repoPath(repo), "-c", "core.quotepath=off", "show", "--format=", "--no-color", sha)
		if err != nil {
			return "", false
		}
	}
	s.cdiffs[key] = d
	return d, true
}

func (p *PR) additions() (a, d int) {
	for _, f := range p.files {
		a += f.Additions
		d += f.Deletions
	}
	return
}

// compare answers GitHub's compare API from git: the status of head relative
// to base, the commits in base..head, and the diff from base to head.
func (s *Server) compare(repo, base, head string) (status string, commits []M, diff string, ok bool) {
	dir := s.repoPath(repo)
	b, err1 := git(dir, "rev-parse", base)
	h, err2 := git(dir, "rev-parse", head)
	if err1 != nil || err2 != nil {
		return "", nil, "", false
	}
	b, h = strings.TrimSpace(b), strings.TrimSpace(h)
	mb, _ := git(dir, "merge-base", b, h)
	mb = strings.TrimSpace(mb)
	switch {
	case b == h:
		status = "identical"
	case mb == b:
		status = "ahead"
	case mb == h:
		status = "behind"
	default:
		status = "diverged"
	}
	out, _ := git(dir, "log", "--reverse", "--format=%H %P", b+".."+h)
	commits = []M{}
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Fields(l)
		if len(f) == 0 {
			continue
		}
		ps := []M{}
		for _, p := range f[1:] {
			ps = append(ps, M{"sha": p})
		}
		commits = append(commits, M{"sha": f[0], "parents": ps})
	}
	diff, _ = git(dir, append(append([]string{}, diffFlags...), b+"..."+h)...)
	return status, commits, diff, true
}
