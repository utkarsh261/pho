package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type M = map[string]any

var opRE = regexp.MustCompile(`^\s*(query|mutation)\s+(\w+)`)
var evRE = regexp.MustCompile(`event:\s*(\w+)`)
var allRE = regexp.MustCompile(`owner:\s*"([^"]+)",\s*name:\s*"([^"]+)"`)
var afterRE = regexp.MustCompile(`after:\s*"([^"]+)"`)
var repoQRE = regexp.MustCompile(`repo:(\S+)`)
var involvesRE = regexp.MustCompile(`involves:(\S+)`)

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func (s *Server) graphql(w *rec, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Query     string `json:"query"`
		Variables M      `json:"variables"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		jsonOut(w, 400, M{"message": "bad json"})
		return
	}
	m := opRE.FindStringSubmatch(req.Query)
	if m == nil {
		w.note = "gql ?"
		jsonOut(w, 404, M{"message": "mockgh: cannot find operation name"})
		return
	}
	op, v := m[2], req.Variables
	w.note = "gql " + op
	str := func(k string) string { x, _ := v[k].(string); return x }
	num := func(k string) int { f, _ := v[k].(float64); return int(f) }
	fail := func(msg string) { jsonOut(w, 200, M{"errors": []M{{"message": msg}}}) }
	ok := func(d M) { jsonOut(w, 200, M{"data": d}) }
	prByVars := func() *PR { return s.findPR(str("owner")+"/"+str("name"), num("number")) }
	prByID := func() *PR { return s.findPRByID(str("pullRequestId")) }
	touch := func(p *PR) { p.updated = time.Now() }

	switch op {
	case "ViewerQuery":
		ok(M{"viewer": M{"login": s.fx.Viewer}})
	case "DashboardPRsQuery":
		full := str("owner") + "/" + str("name")
		var prs []*PR
		for _, p := range s.fx.PRs {
			if strings.EqualFold(p.Repo, full) && p.State == "OPEN" {
				prs = append(prs, p)
			}
		}
		sortedPRs(prs)
		off, _ := strconv.Atoi(str("after"))
		page := prs
		if off < len(page) {
			page = page[off:]
		} else {
			page = nil
		}
		next := len(page) > 100
		if next {
			page = page[:100]
		}
		nodes := []M{}
		for _, p := range page {
			nodes = append(nodes, s.prNode(p))
		}
		ok(M{"repository": M{"nameWithOwner": full, "pullRequests": M{
			"totalCount": len(prs), "pageInfo": M{"hasNextPage": next, "endCursor": strconv.Itoa(off + len(nodes))}, "nodes": nodes}}})
	case "AllPRsQuery":
		am := allRE.FindStringSubmatch(req.Query)
		if am == nil {
			fail("mockgh: cannot parse AllPRsQuery repo")
			return
		}
		full := am[1] + "/" + am[2]
		off := 0
		if x := afterRE.FindStringSubmatch(req.Query); x != nil {
			off, _ = strconv.Atoi(x[1])
		}
		type item struct {
			t time.Time
			n M
		}
		var items []item
		for _, p := range s.fx.PRs {
			if strings.EqualFold(p.Repo, full) {
				items = append(items, item{p.updated, s.prNode(p)})
			}
		}
		for _, e := range s.repoFix(full).Extra {
			items = append(items, item{e.t, s.extraNode(full, e)})
		}
		sort.SliceStable(items, func(i, j int) bool { return items[i].t.After(items[j].t) })
		nodes := []M{}
		end := off
		for i := off; i < len(items) && i < off+100; i++ {
			nodes = append(nodes, items[i].n)
			end = i + 1
		}
		ok(M{"repository": M{"pullRequests": M{"pageInfo": M{"hasNextPage": end < len(items), "endCursor": strconv.Itoa(end)}, "nodes": nodes}}})
	case "InvolvingPRsQuery":
		q := str("query")
		repo, who := "", s.fx.Viewer
		if x := repoQRE.FindStringSubmatch(q); x != nil {
			repo = x[1]
		}
		if x := involvesRE.FindStringSubmatch(q); x != nil {
			who = x[1]
		}
		var prs []*PR
		for _, p := range s.fx.PRs {
			if strings.EqualFold(p.Repo, repo) && p.State == "OPEN" && involves(p, who) {
				prs = append(prs, p)
			}
		}
		sortedPRs(prs)
		nodes := []M{}
		for _, p := range prs {
			n := s.prNode(p)
			n["__typename"] = "PullRequest"
			nodes = append(nodes, n)
		}
		ok(M{"search": M{"issueCount": len(nodes), "pageInfo": M{"hasNextPage": false, "endCursor": ""}, "nodes": nodes}})
	case "PreviewQuery":
		p := prByVars()
		if p == nil {
			ok(M{"repository": M{"nameWithOwner": str("owner") + "/" + str("name"), "pullRequest": nil}})
			return
		}
		ok(M{"repository": M{"nameWithOwner": p.Repo, "pullRequest": s.prNode(p)}})
	case "CommitsQuery":
		p := prByVars()
		if p == nil {
			ok(M{"repository": M{"pullRequest": nil}})
			return
		}
		nodes := []M{}
		for _, c := range p.commits {
			nodes = append(nodes, M{"commit": M{"oid": c.SHA, "messageHeadline": c.Subject, "messageBody": c.Body, "committedDate": ts(c.At),
				"author": M{"name": c.Name, "email": c.Email, "user": s.userByEmail(c.Email)}}})
		}
		ok(M{"repository": M{"pullRequest": M{"commits": M{"nodes": nodes}}}})
	case "CheckMergeable":
		p := prByVars()
		if p == nil {
			fail("not found")
			return
		}
		ok(M{"repository": M{"pullRequest": M{"mergeable": p.Mergeable, "mergeStateStatus": s.mergeState(p), "headRefOid": p.headOid}}})

	case "AddComment":
		p := prByID2(s, str("subjectId"))
		if p == nil {
			fail("subject not found")
			return
		}
		c := &Comment{ID: s.nextID("IC"), Author: s.fx.Viewer, Body: str("body"), t: time.Now()}
		p.Comments = append(p.Comments, c)
		touch(p)
		ok(M{"addComment": M{"subject": M{"id": p.idStr()}}})
	case "AddPullRequestReviewThreadReply":
		t, _ := s.findThread(str("threadId"))
		if t == nil {
			fail("thread not found")
			return
		}
		c := &Comment{ID: s.nextID("PRRC"), Author: s.fx.Viewer, Body: str("body"), t: time.Now()}
		t.Comments = append(t.Comments, c)
		ok(M{"addPullRequestReviewThreadReply": M{"comment": M{"id": c.ID}}})
	case "ResolveReviewThread", "UnresolveReviewThread":
		t, p := s.findThread(str("threadId"))
		if t == nil {
			fail("thread not found")
			return
		}
		res := op == "ResolveReviewThread"
		t.Resolved = res
		t.ResolvedBy = ""
		if res {
			t.ResolvedBy = s.fx.Viewer
		}
		touch(p)
		key := strings.ToLower(op[:1]) + op[1:]
		ok(M{key: M{"thread": M{"id": t.ID, "isResolved": res}}})
	case "SubmitReview", "SubmitReviewWithComments":
		p := prByID()
		if p == nil {
			fail("pull request not found")
			return
		}
		ev := str("event")
		if x := evRE.FindStringSubmatch(req.Query); x != nil {
			ev = x[1]
		}
		state := map[string]string{"APPROVE": "APPROVED", "COMMENT": "COMMENTED", "REQUEST_CHANGES": "CHANGES_REQUESTED"}[ev]
		if state == "" {
			state = "COMMENTED"
		}
		rv := &Review{ID: s.nextID("PRR"), Author: s.fx.Viewer, State: state, Body: str("body"), t: time.Now()}
		p.Reviews = append(p.Reviews, rv)
		if ths, _ := v["threads"].([]any); ths != nil {
			for _, x := range ths {
				d, _ := x.(M)
				line, _ := d["line"].(float64)
				path, _ := d["path"].(string)
				b, _ := d["body"].(string)
				p.Threads = append(p.Threads, &Thread{ID: s.nextID("PRRT"), Path: path, Line: int(line),
					Comments: []*Comment{{ID: s.nextID("PRRC"), Author: s.fx.Viewer, Body: b, t: time.Now()}}})
			}
		}
		p.ReviewRequests = without(p.ReviewRequests, s.fx.Viewer)
		touch(p)
		ok(M{"addPullRequestReview": M{"pullRequestReview": M{"id": rv.ID}}})
	case "MergePullRequest":
		p := prByID()
		if p == nil {
			fail("pull request not found")
			return
		}
		p.State, p.mergedAt = "MERGED", time.Now()
		touch(p)
		ok(M{"mergePullRequest": M{"pullRequest": M{"id": p.idStr(), "state": "MERGED"}}})
	case "ClosePullRequest", "ReopenPullRequest":
		p := prByID()
		if p == nil {
			fail("pull request not found")
			return
		}
		p.State = map[string]string{"ClosePullRequest": "CLOSED", "ReopenPullRequest": "OPEN"}[op]
		touch(p)
		key := strings.ToLower(op[:1]) + op[1:]
		ok(M{key: M{"pullRequest": M{"id": p.idStr(), "state": p.State}}})
	case "UpdatePullRequest":
		p := prByID()
		if p == nil {
			fail("pull request not found")
			return
		}
		if t, has := v["title"].(string); has && t != "" {
			p.Title = t
		}
		if b, has := v["body"].(string); has {
			p.Body = b
		}
		touch(p)
		ok(M{"updatePullRequest": M{"pullRequest": M{"id": p.idStr()}}})
	default:
		w.note += " UNSUPPORTED"
		jsonOut(w, 404, M{"message": "mockgh: unsupported graphql operation " + op})
	}
}

func prByID2(s *Server, id string) *PR { return s.findPRByID(id) }
func (p *PR) idStr() string            { return prID(p) }

func without(a []string, x string) []string {
	var out []string
	for _, s := range a {
		if s != x {
			out = append(out, s)
		}
	}
	return out
}

func (s *Server) findThread(id string) (*Thread, *PR) {
	for _, p := range s.fx.PRs {
		for _, t := range p.Threads {
			if t.ID == id {
				return t, p
			}
		}
	}
	return nil, nil
}

func involves(p *PR, who string) bool {
	if p.Author == who {
		return true
	}
	for _, l := range append(append([]string{}, p.ReviewRequests...), p.Assignees...) {
		if l == who {
			return true
		}
	}
	for _, r := range p.Reviews {
		if r.Author == who {
			return true
		}
	}
	for _, c := range p.Comments {
		if c.Author == who || strings.Contains(c.Body, "@"+who) {
			return true
		}
	}
	for _, t := range p.Threads {
		for _, c := range t.Comments {
			if c.Author == who {
				return true
			}
		}
	}
	return strings.Contains(p.Body, "@"+who)
}

func (s *Server) userByEmail(email string) any {
	if l, ok := s.fx.People[email]; ok {
		return M{"login": l, "avatarUrl": ""}
	}
	return nil
}

func actor(login string) M { return M{"__typename": "User", "login": login, "avatarUrl": ""} }

func (s *Server) rollup(p *PR) (string, []M) {
	if len(p.Checks) == 0 {
		return "", nil
	}
	state := "SUCCESS"
	var nodes []M
	for _, c := range p.Checks {
		status, concl := c.Status, c.Conclusion
		if c.TurnsSuccessAfter > 0 && time.Since(s.start) >= time.Duration(c.TurnsSuccessAfter)*time.Second {
			status, concl = "COMPLETED", "SUCCESS"
		}
		if status == "" {
			status = "COMPLETED"
		}
		switch {
		case status != "COMPLETED":
			if state == "SUCCESS" {
				state = "PENDING"
			}
		case concl == "FAILURE" || concl == "TIMED_OUT" || concl == "CANCELLED":
			state = "FAILURE"
		}
		nodes = append(nodes, M{"__typename": "CheckRun", "name": c.Name, "status": status, "conclusion": concl,
			"detailsUrl": "https://github.com/" + p.Repo + "/actions/runs/1"})
	}
	return state, nodes
}

func (s *Server) mergeState(p *PR) string {
	if p.MergeStateStatus != "" {
		return p.MergeStateStatus
	}
	switch {
	case p.Mergeable == "CONFLICTING":
		return "DIRTY"
	case p.IsDraft:
		return "DRAFT"
	}
	if st, _ := s.rollup(p); st == "PENDING" || st == "FAILURE" {
		return "UNSTABLE"
	}
	return "CLEAN"
}

func (s *Server) reviewDecision(p *PR) any {
	if p.ReviewDecision != nil {
		return *p.ReviewDecision
	}
	latest := latestReviews(p)
	dec := ""
	for _, r := range latest {
		if r.State == "CHANGES_REQUESTED" {
			return "CHANGES_REQUESTED"
		}
		if r.State == "APPROVED" {
			dec = "APPROVED"
		}
	}
	if dec == "" && len(p.ReviewRequests) > 0 {
		dec = "REVIEW_REQUIRED"
	}
	if dec == "" {
		return nil
	}
	return dec
}

func latestReviews(p *PR) []*Review {
	by := map[string]*Review{}
	var order []string
	for _, r := range p.Reviews {
		if _, seen := by[r.Author]; !seen {
			order = append(order, r.Author)
		}
		by[r.Author] = r
	}
	var out []*Review
	for _, a := range order {
		out = append(out, by[a])
	}
	return out
}

func (s *Server) prNode(p *PR) M {
	adds, dels := p.additions()
	state, ctxs := s.rollup(p)
	var rollup any
	if state != "" {
		rollup = M{"state": state, "contexts": M{"nodes": ctxs}}
	}
	rr := []M{}
	for _, l := range p.ReviewRequests {
		rr = append(rr, M{"requestedReviewer": actor(l)})
	}
	asg := []M{}
	for _, l := range p.Assignees {
		asg = append(asg, actor(l))
	}
	lab := []M{}
	for _, l := range p.Labels {
		lab = append(lab, M{"name": l.Name, "color": l.Color})
	}
	lat := []M{}
	revs := []M{}
	for _, r := range latestReviews(p) {
		lat = append(lat, M{"state": r.State, "submittedAt": ts(r.t), "author": actor(r.Author), "commit": M{"oid": p.headOid}})
	}
	for _, r := range p.Reviews {
		revs = append(revs, M{"author": actor(r.Author), "state": r.State, "submittedAt": ts(r.t), "body": r.Body})
	}
	thr := []M{}
	for _, t := range p.Threads {
		cs := []M{}
		for _, c := range t.Comments {
			cs = append(cs, M{"id": c.ID, "author": actor(c.Author), "body": c.Body, "createdAt": ts(c.t)})
		}
		var resBy any
		if t.Resolved && t.ResolvedBy != "" {
			resBy = actor(t.ResolvedBy)
		}
		thr = append(thr, M{"id": t.ID, "path": t.Path, "line": t.Line, "originalLine": t.Line, "isResolved": t.Resolved,
			"resolvedBy": resBy, "comments": M{"nodes": cs}})
	}
	cms := []M{}
	for _, c := range p.Comments {
		cms = append(cms, M{"id": c.ID, "author": actor(c.Author), "body": c.Body, "createdAt": ts(c.t)})
	}
	top := []M{}
	for i, f := range p.files {
		if i >= 20 {
			break
		}
		top = append(top, M{"path": f.Filename, "additions": f.Additions, "deletions": f.Deletions})
	}
	return M{
		"id": prID(p), "number": p.Number, "title": p.Title, "body": p.Body, "state": p.State, "isDraft": p.IsDraft,
		"createdAt": ts(p.created), "updatedAt": ts(p.updated),
		"headRefName": p.Branch, "headRefOid": p.headOid, "baseRefName": p.Base,
		"additions": adds, "deletions": dels, "changedFiles": len(p.files),
		"comments": M{"totalCount": len(p.Comments), "nodes": cms}, "reviewThreads": M{"totalCount": len(p.Threads), "nodes": thr},
		"author": actor(p.Author), "assignees": M{"nodes": asg}, "reviewRequests": M{"nodes": rr},
		"latestOpinionatedReviews": M{"nodes": lat}, "reviews": M{"nodes": revs},
		"reviewDecision": s.reviewDecision(p), "mergeable": p.Mergeable, "mergeStateStatus": s.mergeState(p),
		"labels": M{"nodes": lab}, "files": M{"nodes": top},
		"timelineItems":     M{"nodes": s.timeline(p)},
		"statusCheckRollup": rollup,
		"repository":        M{"nameWithOwner": p.Repo},
	}
}

func (s *Server) timeline(p *PR) []M {
	var best M
	var bt time.Time
	consider := func(t time.Time, n M) {
		if best == nil || t.After(bt) {
			best, bt = n, t
		}
	}
	for _, c := range p.commits {
		consider(c.At, M{"__typename": "PullRequestCommit", "id": "PRC_" + c.SHA[:8], "commit": M{"oid": c.SHA, "messageHeadline": c.Subject,
			"committedDate": ts(c.At), "author": M{"user": s.userByEmail(c.Email), "name": c.Name}}})
	}
	for _, c := range p.Comments {
		consider(c.t, M{"__typename": "IssueComment", "id": c.ID, "body": c.Body, "createdAt": ts(c.t), "author": actor(c.Author)})
	}
	for _, r := range p.Reviews {
		consider(r.t, M{"__typename": "PullRequestReview", "id": r.ID, "state": r.State, "body": r.Body, "submittedAt": ts(r.t), "author": actor(r.Author)})
	}
	if p.State == "MERGED" && !p.mergedAt.IsZero() {
		consider(p.mergedAt, M{"__typename": "MergedEvent", "id": "ME_" + prID(p), "createdAt": ts(p.mergedAt), "actor": actor(s.fx.Viewer),
			"commit": M{"oid": p.headOid}, "mergeRefName": p.Base})
	}
	if best == nil {
		return []M{}
	}
	return []M{best}
}

func (s *Server) extraNode(full string, e *ExtraPR) M {
	return M{"id": fmt.Sprintf("PR_%s_%d", strings.ReplaceAll(full, "/", "_"), e.Number), "number": e.Number, "title": e.Title, "state": e.State,
		"isDraft": false, "createdAt": ts(e.t.Add(-48 * time.Hour)), "updatedAt": ts(e.t),
		"headRefName": fmt.Sprintf("archive/pr-%d", e.Number), "baseRefName": "main", "additions": 0, "deletions": 0, "changedFiles": 0,
		"comments": M{"totalCount": 0}, "reviewThreads": M{"totalCount": 0}, "author": actor(e.Author),
		"assignees": M{"nodes": []M{}}, "reviewRequests": M{"nodes": []M{}}, "latestOpinionatedReviews": M{"nodes": []M{}},
		"reviewDecision": nil, "repository": M{"nameWithOwner": full}}
}
