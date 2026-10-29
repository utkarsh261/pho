package release

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIsRelease(t *testing.T) {
	tests := map[string]bool{
		"1.2.3":                                true,
		"v1.2.3":                               true,
		"10.20.30":                             true,
		"dev":                                  false,
		"":                                     false,
		"1.2":                                  false,
		"1.2.3-rc.1":                           false,
		"v1.2.3-rc.1":                          false,
		"1.2.3+meta":                           false,
		"v0.0.0-20260101120000-abcdef123456":   false,
		"v1.2.4-0.20260101120000-abcdef123456": false,
		"(devel)":                              false,
	}
	for v, want := range tests {
		if got := IsRelease(v); got != want {
			t.Errorf("IsRelease(%q) = %v, want %v", v, got, want)
		}
	}
}

func TestCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.2.3", "1.2.3", 0},
		{"v1.2.3", "1.2.3", 0},
		{"1.2.3", "1.2.4", -1},
		{"1.10.0", "1.9.9", 1},
		{"2.0.0", "1.99.99", 1},
		{"dev", "0.0.1", -1},
		{"1.0.0", "", 1},
	}
	for _, tt := range tests {
		if got := Compare(tt.a, tt.b); got != tt.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestCleanNotes(t *testing.T) {
	body := "## Changelog\r\n* 1cac322: Add a label picker on L in PR detail (#51) (@utkarsh261)\r\n* 9620d7a Show PR labels (#50)\r\n\r\n"
	want := "* Add a label picker on L in PR detail (#51)\n* Show PR labels (#50)"
	if got := CleanNotes(body); got != want {
		t.Errorf("CleanNotes =\n%q\nwant\n%q", got, want)
	}

	// A handle at the end of a list keeps the blank line before the next paragraph.
	body = "* abc1234 Fix it (@bob)\n\nThanks to everyone.\n"
	want = "* Fix it\n\nThanks to everyone."
	if got := CleanNotes(body); got != want {
		t.Errorf("CleanNotes =\n%q\nwant\n%q", got, want)
	}
}

func rels(versions ...string) []Release {
	out := make([]Release, len(versions))
	for i, v := range versions {
		out[i] = Release{Version: v}
	}
	return out
}

func versions(rs []Release) string {
	var vs []string
	for _, r := range rs {
		vs = append(vs, r.Version)
	}
	return strings.Join(vs, ",")
}

func TestBetweenAndRecent(t *testing.T) {
	all := rels("1.4.0", "1.3.0", "1.2.0", "1.1.0", "1.0.0")
	tests := []struct {
		name, since, current string
		limit                int
		want                 string
	}{
		{"one step", "1.2.0", "1.3.0", 5, "1.3.0"},
		{"skipped releases", "1.0.0", "1.3.0", 5, "1.3.0,1.2.0,1.1.0"},
		{"limit", "1.0.0", "1.4.0", 2, "1.4.0,1.3.0"},
		{"no since shows recent", "", "1.3.0", 2, "1.3.0,1.2.0"},
		{"dev shows newest", "1.0.0", "dev", 2, "1.4.0,1.3.0"},
		{"same version", "1.3.0", "1.3.0", 5, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := versions(Between(all, tt.since, tt.current, tt.limit)); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMarkdown(t *testing.T) {
	md := Markdown([]Release{{Version: "1.2.0", Body: "## Changelog\n* abc1234: Thing (#1) (@x)"}, {Version: "1.1.0"}})
	for _, want := range []string{"## v1.2.0", "* Thing (#1)", "## v1.1.0", "No notes for this release."} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

const listJSON = `[
 {"tag_name":"v1.3.0-rc.1","prerelease":true,"body":"rc"},
 {"tag_name":"v1.2.0","body":"two","html_url":"https://x/1.2.0"},
 {"tag_name":"v1.1.5","draft":true,"body":"draft"},
 {"tag_name":"nightly","body":"odd tag"},
 {"tag_name":"v1.1.0","body":"one"}
]`

func serve(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &Client{BaseURL: srv.URL}
}

func TestList(t *testing.T) {
	c := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/releases" || r.URL.Query().Get("per_page") != "20" {
			t.Errorf("unexpected request %s", r.URL)
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("request carried an Authorization header")
		}
		w.Write([]byte(listJSON))
	})
	got, err := c.List(context.Background(), 20)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if v := versions(got); v != "1.2.0,1.1.0" {
		t.Errorf("versions = %q, want 1.2.0,1.1.0", v)
	}
	if got[0].Body != "two" || got[0].URL != "https://x/1.2.0" {
		t.Errorf("first release = %+v", got[0])
	}
}

func TestLatest(t *testing.T) {
	c := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/releases/latest" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Write([]byte(`{"tag_name":"v1.4.0"}`))
	})
	got, err := c.Latest(context.Background())
	if err != nil || got.Version != "1.4.0" {
		t.Fatalf("Latest = %+v, %v; want 1.4.0", got, err)
	}
}

func TestFailuresReturnErrors(t *testing.T) {
	tests := map[string]http.HandlerFunc{
		"server error": func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
		"rate limited": func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) },
		"invalid json": func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("{not json")) },
		"bad tag":      func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(`{"tag_name":"nightly"}`)) },
		"empty body":   func(w http.ResponseWriter, _ *http.Request) {},
	}
	for name, h := range tests {
		t.Run(name, func(t *testing.T) {
			c := serve(t, h)
			if _, err := c.Latest(context.Background()); err == nil {
				t.Error("Latest succeeded, want error")
			}
		})
	}
}

func TestSlowServerTimesOut(t *testing.T) {
	release := make(chan struct{})
	c := serve(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	defer close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := c.List(ctx, 5); err == nil {
		t.Fatal("List succeeded, want timeout error")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("List took %v, want it to give up with the context", d)
	}
}

func TestUnreachableServer(t *testing.T) {
	c := &Client{BaseURL: "http://127.0.0.1:1"}
	if _, err := c.List(context.Background(), 5); err == nil {
		t.Fatal("List succeeded against a closed port, want error")
	}
}
