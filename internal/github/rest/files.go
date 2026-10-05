package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	pholog "github.com/utkarsh261/pho/internal/log"
)

// ErrDiffTooLarge is returned when GitHub refuses to render a diff, e.g.
// "the diff exceeded the maximum number of files (300)". Callers fall back
// to the per-file endpoints.
var ErrDiffTooLarge = errors.New("rest: diff too large")

// isDiffTooLarge reports whether a failed diff request was GitHub declining
// to render the diff. Any 406 counts: GitHub Enterprise admins can set their
// own limits and the error body differs between versions.
func isDiffTooLarge(status int, body []byte) bool {
	return status == http.StatusNotAcceptable || bytes.Contains(body, []byte("too_large"))
}

const (
	filesPerPage = 100
	// maxFilePages matches GitHub's cap of 3,000 files on the files endpoints.
	maxFilePages     = 30
	filePageWorkers  = 4
	maxErrorBodySize = 512
)

// ChangedFile is one entry of the "List pull request files" and commit
// "files" responses. Patch is empty when GitHub omits it (binary files and
// files whose diff is too large to render).
type ChangedFile struct {
	Filename         string `json:"filename"`
	PreviousFilename string `json:"previous_filename"`
	Status           string `json:"status"`
	Additions        int    `json:"additions"`
	Deletions        int    `json:"deletions"`
	Changes          int    `json:"changes"`
	Patch            string `json:"patch"`
}

// FetchPRFiles lists every changed file of a PR with its patch, up to
// GitHub's limit of 3,000 files. The first page reveals the page count via
// the Link header; remaining pages are fetched concurrently and returned in
// order.
func (c *Client) FetchPRFiles(ctx context.Context, owner, repo string, number int) ([]ChangedFile, error) {
	base := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/files", c.BaseURL, owner, repo, number)
	return c.fetchFilePages(ctx, base, func(body []byte) ([]ChangedFile, error) {
		var files []ChangedFile
		err := json.Unmarshal(body, &files)
		return files, err
	})
}

// FetchCommitFiles lists every changed file of a commit with its patch.
func (c *Client) FetchCommitFiles(ctx context.Context, owner, repo, sha string) ([]ChangedFile, error) {
	base := buildCommitDiffURL(c.BaseURL, owner, repo, sha)
	return c.fetchFilePages(ctx, base, func(body []byte) ([]ChangedFile, error) {
		var commit struct {
			Files []ChangedFile `json:"files"`
		}
		err := json.Unmarshal(body, &commit)
		return commit.Files, err
	})
}

func (c *Client) fetchFilePages(ctx context.Context, base string, decode func([]byte) ([]ChangedFile, error)) ([]ChangedFile, error) {
	first, link, err := c.getJSON(ctx, pageURL(base, 1))
	if err != nil {
		return nil, err
	}
	files, err := decode(first)
	if err != nil {
		return nil, fmt.Errorf("rest: decode files page 1: %w", err)
	}
	last := lastPage(link)
	if last <= 1 {
		if nextPage(link) && len(files) == filesPerPage {
			return c.fetchFilePagesSequential(ctx, base, files, decode)
		}
		return files, nil
	}
	last = min(last, maxFilePages)

	pages := make([][]ChangedFile, last+1)
	pages[1] = files
	errs := make([]error, last+1)
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range filePageWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range jobs {
				body, _, err := c.getJSON(ctx, pageURL(base, p))
				if err == nil {
					pages[p], err = decode(body)
				}
				errs[p] = err
			}
		}()
	}
	for p := 2; p <= last; p++ {
		jobs <- p
	}
	close(jobs)
	wg.Wait()

	for p := 2; p <= last; p++ {
		if errs[p] != nil {
			return nil, fmt.Errorf("rest: files page %d: %w", p, errs[p])
		}
		files = append(files, pages[p]...)
	}
	return files, nil
}

// fetchFilePagesSequential follows rel="next" links when the server gives
// no rel="last" (some GitHub Enterprise versions).
func (c *Client) fetchFilePagesSequential(ctx context.Context, base string, files []ChangedFile, decode func([]byte) ([]ChangedFile, error)) ([]ChangedFile, error) {
	for p := 2; p <= maxFilePages; p++ {
		body, link, err := c.getJSON(ctx, pageURL(base, p))
		if err != nil {
			return nil, fmt.Errorf("rest: files page %d: %w", p, err)
		}
		page, err := decode(body)
		if err != nil {
			return nil, fmt.Errorf("rest: decode files page %d: %w", p, err)
		}
		files = append(files, page...)
		if !nextPage(link) || len(page) < filesPerPage {
			break
		}
	}
	return files, nil
}

func pageURL(base string, page int) string {
	return fmt.Sprintf("%s?per_page=%d&page=%d", base, filesPerPage, page)
}

var lastLinkRE = regexp.MustCompile(`<([^>]+)>;\s*rel="last"`)

// lastPage returns the page number of the rel="last" link, or 0.
func lastPage(link string) int {
	m := lastLinkRE.FindStringSubmatch(link)
	if m == nil {
		return 0
	}
	u, err := url.Parse(m[1])
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(u.Query().Get("page"))
	return n
}

func nextPage(link string) bool { return strings.Contains(link, `rel="next"`) }

// getJSON performs an authenticated GET and returns the body and Link header.
func (c *Client) getJSON(ctx context.Context, u string) ([]byte, string, error) {
	var statusCode int
	defer c.logRequest("rest files fetch", time.Now(), &statusCode)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", fmt.Errorf("rest: create request: %w", err)
	}
	req.Header.Set("Accept", acceptJSONHeader)
	req.Header.Set("User-Agent", userAgentHeader)
	req.Header.Set("Authorization", "token "+c.Token)

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("rest: request failed: %w", err)
	}
	defer resp.Body.Close()
	statusCode = resp.StatusCode

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodySize))
		return nil, "", fmt.Errorf("rest: unexpected status %d: %s", resp.StatusCode, string(body))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("rest: read body: %w", err)
	}
	return body, resp.Header.Get("Link"), nil
}

// logRequest logs a request's duration and final status code. Pass a pointer
// so the status set after the deferred call is registered is the one logged.
func (c *Client) logRequest(msg string, start time.Time, statusCode *int) {
	if c.log == nil {
		return
	}
	c.log.Debug(msg, pholog.FieldDurationMS, time.Since(start).Milliseconds(),
		pholog.FieldHost, c.BaseURL, pholog.FieldStatusCode, *statusCode)
}
