package rest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/utkarsh261/pho/internal/domain"
)

// serveLabels serves total labels across pages of 100 under prefix+path,
// advertising only rel="next" (as GHES sometimes does).
func serveLabels(t *testing.T, prefix, path string, total int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != prefix+path {
			t.Errorf("path = %s, want %s", r.URL.Path, prefix+path)
		}
		if got := r.URL.Query().Get("per_page"); got != "100" {
			t.Errorf("per_page = %q", got)
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		start := (page - 1) * filesPerPage
		n := min(filesPerPage, max(total-start, 0))
		if start+n < total {
			w.Header().Set("Link", fmt.Sprintf(`<%s%s%s?per_page=100&page=%d>; rel="next"`, srv.URL, prefix, path, page+1))
		}
		out := make([]Label, n)
		for i := range out {
			out[i] = Label{Name: fmt.Sprintf("l%04d", start+i), Color: "ededed"}
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestListRepoLabelsPages(t *testing.T) {
	t.Parallel()
	for _, prefix := range apiPrefixes {
		srv, calls := serveLabels(t, prefix, "/repos/o/r/labels", 250)
		c := &Client{BaseURL: srv.URL + prefix, Token: "tok", HTTPClient: srv.Client()}
		labels, truncated, err := c.ListRepoLabels(context.Background(), "o", "r")
		if err != nil {
			t.Fatalf("prefix %q: %v", prefix, err)
		}
		if len(labels) != 250 || truncated || calls.Load() != 3 {
			t.Fatalf("prefix %q: %d labels, truncated=%v, %d calls", prefix, len(labels), truncated, calls.Load())
		}
		if labels[249].Name != "l0249" {
			t.Errorf("last label = %q", labels[249].Name)
		}
	}
}

func TestListRepoLabelsCapsPages(t *testing.T) {
	t.Parallel()
	srv, calls := serveLabels(t, "", "/repos/o/r/labels", 5000)
	c := &Client{BaseURL: srv.URL, Token: "tok", HTTPClient: srv.Client()}
	labels, truncated, err := c.ListRepoLabels(context.Background(), "o", "r")
	if err != nil {
		t.Fatal(err)
	}
	if len(labels) != maxLabelPages*filesPerPage || !truncated || int(calls.Load()) != maxLabelPages {
		t.Fatalf("%d labels, truncated=%v, %d calls", len(labels), truncated, calls.Load())
	}
}

func TestListIssueLabels(t *testing.T) {
	t.Parallel()
	srv, _ := serveLabels(t, "/api/v3", "/repos/o/r/issues/7/labels", 30)
	c := &Client{BaseURL: srv.URL + "/api/v3", Token: "tok", HTTPClient: srv.Client()}
	labels, err := c.ListIssueLabels(context.Background(), "o", "r", 7)
	if err != nil || len(labels) != 30 {
		t.Fatalf("%d labels, err %v", len(labels), err)
	}
}

// labelServer answers every request with status and body, recording the
// method, escaped path and decoded JSON body of the last request.
type labelServer struct {
	method, path string
	body         map[string]any
}

func newLabelServer(t *testing.T, status int, respBody string) (*Client, *labelServer) {
	t.Helper()
	rec := &labelServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.method, rec.path = r.Method, r.URL.EscapedPath()
		if got := r.Header.Get("Authorization"); got != "token tok" {
			t.Errorf("Authorization = %q", got)
		}
		if b, _ := io.ReadAll(r.Body); len(b) > 0 {
			if ct := r.Header.Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q", ct)
			}
			_ = json.Unmarshal(b, &rec.body)
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, respBody)
	}))
	t.Cleanup(srv.Close)
	return &Client{BaseURL: srv.URL, Token: "tok", HTTPClient: srv.Client()}, rec
}

func TestAddLabels(t *testing.T) {
	t.Parallel()
	c, rec := newLabelServer(t, http.StatusOK, "[]")
	if err := c.AddLabels(context.Background(), "o", "r", 7, []string{"bug", "good first issue"}); err != nil {
		t.Fatal(err)
	}
	if rec.method != http.MethodPost || rec.path != "/repos/o/r/issues/7/labels" {
		t.Errorf("request = %s %s", rec.method, rec.path)
	}
	if got := fmt.Sprint(rec.body["labels"]); got != "[bug good first issue]" {
		t.Errorf("labels = %s", got)
	}
}

func TestRemoveLabelEscapesName(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]string{
		"bug":              "/repos/o/r/issues/7/labels/bug",
		"good first issue": "/repos/o/r/issues/7/labels/good%20first%20issue",
		"area/diff":        "/repos/o/r/issues/7/labels/area%2Fdiff",
		"100%":             "/repos/o/r/issues/7/labels/100%25",
		"c#":               "/repos/o/r/issues/7/labels/c%23",
		"🚀 ship?":          "/repos/o/r/issues/7/labels/%F0%9F%9A%80%20ship%3F",
	} {
		c, rec := newLabelServer(t, http.StatusOK, "[]")
		if err := c.RemoveLabel(context.Background(), "o", "r", 7, name); err != nil {
			t.Fatalf("%q: %v", name, err)
		}
		if rec.method != http.MethodDelete || rec.path != want {
			t.Errorf("%q: request = %s %s, want DELETE %s", name, rec.method, rec.path, want)
		}
	}
}

func TestRemoveLabelAlreadyGoneIsFine(t *testing.T) {
	t.Parallel()
	c, _ := newLabelServer(t, http.StatusNotFound, `{"message":"Label does not exist"}`)
	if err := c.RemoveLabel(context.Background(), "o", "r", 7, "bug"); err != nil {
		t.Fatalf("404 should be success, got %v", err)
	}
}

func TestCreateLabel(t *testing.T) {
	t.Parallel()
	c, rec := newLabelServer(t, http.StatusCreated, `{"name":"needs-qa"}`)
	if err := c.CreateLabel(context.Background(), "o", "r", "needs-qa", "1d76db"); err != nil {
		t.Fatal(err)
	}
	if rec.method != http.MethodPost || rec.path != "/repos/o/r/labels" {
		t.Errorf("request = %s %s", rec.method, rec.path)
	}
	if rec.body["name"] != "needs-qa" || rec.body["color"] != "1d76db" {
		t.Errorf("body = %v", rec.body)
	}
}

func TestCreateLabelAlreadyExistsIsFine(t *testing.T) {
	t.Parallel()
	c, _ := newLabelServer(t, http.StatusUnprocessableEntity,
		`{"message":"Validation Failed","errors":[{"resource":"Label","code":"already_exists","field":"name"}]}`)
	if err := c.CreateLabel(context.Background(), "o", "r", "bug", "1d76db"); err != nil {
		t.Fatalf("already_exists should be success, got %v", err)
	}
}

func TestLabelErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	c, _ := newLabelServer(t, http.StatusForbidden, `{"message":"Must have admin rights"}`)
	var perr *LabelPermissionError
	if err := c.CreateLabel(ctx, "o", "r", "x", "ededed"); !errors.As(err, &perr) || perr.Op != LabelOpCreate {
		t.Errorf("create 403 = %v, want create permission error", err)
	}
	if err := c.AddLabels(ctx, "o", "r", 7, []string{"x"}); !errors.As(err, &perr) || perr.Op != LabelOpEdit {
		t.Errorf("add 403 = %v, want edit permission error", err)
	}
	if err := c.RemoveLabel(ctx, "o", "r", 7, "x"); !errors.Is(err, domain.ErrLabelPermission) || err.Error() != "no permission to edit labels in o/r" {
		t.Errorf("remove 403 = %v", err)
	}

	c, _ = newLabelServer(t, http.StatusUnprocessableEntity,
		`{"message":"Validation Failed","errors":[{"resource":"Label","code":"invalid","field":"name","message":"name is too long (maximum is 50 characters)"}]}`)
	if err := c.CreateLabel(ctx, "o", "r", strings.Repeat("x", 60), "ededed"); err == nil || err.Error() != "name is too long (maximum is 50 characters)" {
		t.Errorf("422 = %v", err)
	}

	c, _ = newLabelServer(t, http.StatusForbidden, `{"message":"You have exceeded a secondary rate limit."}`)
	if err := c.AddLabels(ctx, "o", "r", 7, []string{"x"}); errors.Is(err, domain.ErrLabelPermission) || err == nil {
		t.Errorf("rate-limit 403 = %v, want a plain status error", err)
	}

	c, _ = newLabelServer(t, http.StatusInternalServerError, "boom")
	if err := c.AddLabels(ctx, "o", "r", 7, []string{"x"}); err == nil || !strings.Contains(err.Error(), "unexpected status 500") {
		t.Errorf("500 = %v", err)
	}
}
