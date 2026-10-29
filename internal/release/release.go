// Package release reads pho's own releases from github.com, unauthenticated.
package release

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const DefaultBaseURL = "https://api.github.com/repos/utkarsh261/pho"

const ReleasesURL = "https://github.com/utkarsh261/pho/releases"

const Timeout = 3 * time.Second

type Release struct {
	Version string // without the leading v, e.g. "1.2.0"
	Body    string // release notes, as markdown
	URL     string
}

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

// List returns up to n stable releases, newest first.
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

// IsRelease reports whether v is a stable "[v]X.Y.Z".
func IsRelease(v string) bool {
	return versionRe.MatchString(v)
}

// Normalize drops the v that go install builds carry.
func Normalize(v string) string {
	return strings.TrimPrefix(v, "v")
}

// Compare returns -1, 0 or 1. Unparseable versions sort as oldest.
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
	changelogHeadingRe = regexp.MustCompile(`(?m)^#{1,6}[ \t]*Changelog[ \t]*\n+`)
	shaBulletRe        = regexp.MustCompile(`(?m)^([ \t]*[*-][ \t]+)[0-9a-f]{7,40}:?[ \t]+`)
	authorRe           = regexp.MustCompile(`(?m)[ \t]+\(@[\w-]+\)[ \t]*$`)
)

// CleanNotes strips GoReleaser's heading, commit SHAs and author handles.
func CleanNotes(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = changelogHeadingRe.ReplaceAllString(body, "")
	body = shaBulletRe.ReplaceAllString(body, "$1")
	body = authorRe.ReplaceAllString(body, "")
	return strings.TrimSpace(body)
}

// Between returns releases in (since, current], newest first. Falls back to
// Recent when since or current isn't a release.
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

// Recent returns releases up to current, or the newest on a dev build.
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

// BrewFormula includes the tap so it can't clash with another "pho".
const BrewFormula = "utkarsh261/tap/pho"

// DetectBrew returns brew's path when pho runs from Homebrew's Cellar,
// preferring the brew of that Homebrew install over the one on PATH.
func DetectBrew(executable func() (string, error), lookPath func(string) (string, error)) (string, bool) {
	exe, err := executable()
	if err != nil {
		return "", false
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	prefix, _, ok := strings.Cut(filepath.ToSlash(exe), "/Cellar/pho/")
	if !ok {
		return "", false
	}
	for _, name := range []string{filepath.Join(filepath.FromSlash(prefix), "bin", "brew"), "brew"} {
		if brew, err := lookPath(name); err == nil {
			return brew, true
		}
	}
	return "", false
}
