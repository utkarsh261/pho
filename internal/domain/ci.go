package domain

import (
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// ErrJobLogUnavailable is matched (errors.Is) when GitHub has no log for a
// job: it is still running, or its logs have expired.
var ErrJobLogUnavailable = errors.New("job log not available")

// JobLog is the tail of a GitHub Actions job's log.
type JobLog struct {
	Lines     []string
	Truncated bool // earlier lines were dropped
}

// actionsJobPath matches the path of a GitHub Actions check run's details
// URL: /<owner>/<repo>/actions/runs/<runID>/job/<jobID>.
var actionsJobPath = regexp.MustCompile(`^/([^/]+)/([^/]+)/actions/runs/(\d+)/job/(\d+)$`)

// ActionsJobID returns the GitHub Actions job ID behind a check's details URL.
// ok is false for checks from other CI providers, for commit statuses, and
// for jobs that belong to a repository other than repoNames ("owner/name").
// Several names can be given: after a rename the git remote may still use
// the old one while GitHub's URLs use the new one.
func ActionsJobID(detailsURL string, repoNames ...string) (id int64, ok bool) {
	ref, ok := parseActionsURL(detailsURL, repoNames)
	return ref.jobID, ok
}

// ActionsRunID returns the workflow run ID behind a check's details URL, with
// the same rules as ActionsJobID. Re-running a job gives it a new job ID but
// keeps the run ID.
func ActionsRunID(detailsURL string, repoNames ...string) (id int64, ok bool) {
	ref, ok := parseActionsURL(detailsURL, repoNames)
	return ref.runID, ok
}

// ActionsRepo returns the owner and name a check's details URL uses for its
// repo, with the same rules as ActionsJobID. That is GitHub's current name,
// so API calls made with it need no rename redirect.
func ActionsRepo(detailsURL string, repoNames ...string) (owner, name string, ok bool) {
	ref, ok := parseActionsURL(detailsURL, repoNames)
	return ref.owner, ref.name, ok
}

type actionsURL struct {
	owner, name  string
	runID, jobID int64
}

func parseActionsURL(detailsURL string, repoNames []string) (actionsURL, bool) {
	u, err := url.Parse(detailsURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return actionsURL{}, false
	}
	m := actionsJobPath.FindStringSubmatch(u.Path)
	if m == nil || !matchesAnyRepo(m[1]+"/"+m[2], repoNames) {
		return actionsURL{}, false
	}
	runID, err1 := strconv.ParseInt(m[3], 10, 64)
	jobID, err2 := strconv.ParseInt(m[4], 10, 64)
	if err1 != nil || err2 != nil {
		return actionsURL{}, false
	}
	return actionsURL{owner: m[1], name: m[2], runID: runID, jobID: jobID}, true
}

func matchesAnyRepo(fullName string, repoNames []string) bool {
	for _, n := range repoNames {
		if n != "" && strings.EqualFold(fullName, n) {
			return true
		}
	}
	return false
}
