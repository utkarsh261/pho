// Package release reads pho's own releases from GitHub, for the "What's new"
// panel and the update notice. Every call is unauthenticated and goes to
// github.com, whichever host the user works against.
package release

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is pho's repository on the GitHub REST API.
const DefaultBaseURL = "https://api.github.com/repos/utkarsh261/pho"

// ReleasesURL is where users can read every release's notes.
const ReleasesURL = "https://github.com/utkarsh261/pho/releases"

// Timeout bounds every request, on top of the caller's context.
const Timeout = 3 * time.Second

// Release is one published, stable release.
type Release struct {
	Version string // without the leading v, e.g. "1.2.0"
	Body    string // release notes, as markdown
	URL     string
}

// Client fetches releases. The zero value is ready to use.
type Client struct {
	BaseURL    string // DefaultBaseURL when empty
	HTTPClient *http.Client
}

type apiRelease struct {
	TagName    string `json:"tag_name"`
	Body       string `json:"body"`
	HTMLURL    string `json:"html_url"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

// List returns up to n of the newest stable releases, newest first. Drafts,
// prereleases and tags that aren't a plain version are left out.
func (c *Client) List(ctx context.Context, n int) ([]Release, error) {
	var raw []apiRelease
	if err := c.get(ctx, fmt.Sprintf("/releases?per_page=%d", n), &raw); err != nil {
		return nil, err
	}
	out := make([]Release, 0, len(raw))
	for _, r := range raw {
		if rel, ok := convert(r); ok {
			out = append(out, rel)
		}
	}
	return out, nil
}

// Latest returns the newest stable release.
func (c *Client) Latest(ctx context.Context) (Release, error) {
	var raw apiRelease
	if err := c.get(ctx, "/releases/latest", &raw); err != nil {
		return Release{}, err
	}
	rel, ok := convert(raw)
	if !ok {
		return Release{}, fmt.Errorf("release: latest release has unexpected tag %q", raw.TagName)
	}
	return rel, nil
}

func convert(r apiRelease) (Release, bool) {
	if r.Draft || r.Prerelease || !IsRelease(r.TagName) {
		return Release{}, false
	}
	return Release{Version: Normalize(r.TagName), Body: r.Body, URL: r.HTMLURL}, true
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	base := c.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	hc := c.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: Timeout}
	}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+path, nil)
	if err != nil {
		return fmt.Errorf("release: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("release: GET %s: %s", path, resp.Status)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(out); err != nil {
		return fmt.Errorf("release: decode %s: %w", path, err)
	}
	return nil
}

var versionRe = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)$`)

// IsRelease reports whether v is a plain stable version, "1.2.3" or
// "v1.2.3". Dev builds, pseudo-versions and prerelease or build suffixes
// are not.
func IsRelease(v string) bool {
	return versionRe.MatchString(v)
}

// Normalize drops a leading v, so GoReleaser builds ("1.2.3") and
// go install builds ("v1.2.3") read the same.
func Normalize(v string) string {
	return strings.TrimPrefix(v, "v")
}

// Compare orders two versions that pass IsRelease: -1 if a is older than b,
// 0 if equal, 1 if newer. A version that doesn't parse sorts as oldest.
func Compare(a, b string) int {
	pa, pb := parse(a), parse(b)
	for i := range pa {
		switch {
		case pa[i] < pb[i]:
			return -1
		case pa[i] > pb[i]:
			return 1
		}
	}
	return 0
}

func parse(v string) [3]int {
	m := versionRe.FindStringSubmatch(v)
	if m == nil {
		return [3]int{-1, -1, -1}
	}
	var out [3]int
	for i := range out {
		out[i], _ = strconv.Atoi(m[i+1])
	}
	return out
}

var (
	changelogHeadingRe = regexp.MustCompile(`(?m)^#{1,6}\s*Changelog\s*\n+`)
	shaBulletRe        = regexp.MustCompile(`(?m)^(\s*[*-]\s+)[0-9a-f]{7,40}:?\s+`)
	authorRe           = regexp.MustCompile(`(?m)\s+\(@[\w-]+\)\s*$`)
)

// CleanNotes tidies GoReleaser's release notes for reading in the terminal:
// it drops the "## Changelog" heading, and the commit SHA and author handle
// around each bullet.
func CleanNotes(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = changelogHeadingRe.ReplaceAllString(body, "")
	body = shaBulletRe.ReplaceAllString(body, "$1")
	body = authorRe.ReplaceAllString(body, "")
	return strings.TrimSpace(body)
}

// Between returns the releases newer than since and no newer than current,
// newest first, at most limit of them. Without a since, or on a dev build,
// it falls back to Recent.
func Between(releases []Release, since, current string, limit int) []Release {
	if !IsRelease(since) || !IsRelease(current) {
		return Recent(releases, current, limit)
	}
	var out []Release
	for _, r := range releases {
		if len(out) == limit {
			break
		}
		if Compare(r.Version, current) <= 0 && Compare(r.Version, since) > 0 {
			out = append(out, r)
		}
	}
	return out
}

// Recent returns up to limit releases no newer than current, newest first,
// for `pho --changelog` and W without a recorded upgrade. On a dev build it
// returns the newest ones.
func Recent(releases []Release, current string, limit int) []Release {
	var out []Release
	for _, r := range releases {
		if len(out) == limit {
			break
		}
		if IsRelease(current) && Compare(r.Version, current) > 0 {
			continue
		}
		out = append(out, r)
	}
	return out
}

// Markdown renders releases as one markdown document, a heading per
// release.
func Markdown(releases []Release) string {
	var b strings.Builder
	for i, r := range releases {
		if i > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "## v%s\n\n", r.Version)
		if notes := CleanNotes(r.Body); notes != "" {
			b.WriteString(notes)
		} else {
			b.WriteString("No notes for this release.")
		}
	}
	return b.String()
}
