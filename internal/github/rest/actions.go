package rest

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/utkarsh261/pho/internal/domain"
)

// maxLogLineBytes caps a kept log line; the rest of a longer line is dropped.
const maxLogLineBytes = 64 << 10

// FetchJobLogTail returns the last maxLines lines of a GitHub Actions job's
// log. truncated is true when earlier lines were dropped.
//
//	GET /repos/{owner}/{repo}/actions/jobs/{jobID}/logs
//
// GitHub answers with a redirect to a signed download URL on another host,
// which is followed without the Authorization header. The body is streamed so a large log is
// never held in memory whole. The client's own timeout is lifted because it
// would also cover streaming a long log; ctx bounds the request instead.
func (c *Client) FetchJobLogTail(ctx context.Context, owner, repo string, jobID int64, maxLines int) (lines []string, truncated bool, err error) {
	var statusCode int
	defer c.logRequest("rest job log fetch", time.Now(), &statusCode)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, buildJobLogsURL(c.BaseURL, owner, repo, jobID), nil)
	if err != nil {
		return nil, false, fmt.Errorf("rest: create request: %w", err)
	}
	req.Header.Set("Accept", acceptJSONHeader)
	req.Header.Set("User-Agent", userAgentHeader)
	req.Header.Set("Authorization", "token "+c.Token)

	base := c.HTTPClient
	if base == nil {
		base = http.DefaultClient
	}
	client := *base
	client.Timeout = 0
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		// The download URL on another host is pre-signed; don't send the token
		// there. Go itself keeps it for another port on the same hostname, so
		// compare host and port. Same-host hops keep it.
		if req.URL.Host != via[0].URL.Host {
			req.Header.Del("Authorization")
		}
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("rest: request failed: %w", err)
	}
	defer resp.Body.Close()
	statusCode = resp.StatusCode

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound, http.StatusGone:
		return nil, false, domain.ErrJobLogUnavailable
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodySize))
		return nil, false, statusError(resp.StatusCode, body)
	}

	lines, truncated, err = tailLines(resp.Body, maxLines)
	if err != nil {
		return nil, false, fmt.Errorf("rest: read log: %w", err)
	}
	return lines, truncated, nil
}

// tailLines keeps the last max lines of r in a ring buffer.
func tailLines(r io.Reader, max int) ([]string, bool, error) {
	if max <= 0 {
		return nil, false, nil
	}
	ring := make([]string, 0, max)
	next, total := 0, 0
	br := bufio.NewReaderSize(r, 64*1024)
	for {
		line, err := readCappedLine(br, maxLogLineBytes)
		if line != "" || err == nil {
			total++
			if len(ring) < max {
				ring = append(ring, line)
			} else {
				ring[next] = line
				next = (next + 1) % max
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, false, err
		}
	}
	out := make([]string, 0, len(ring))
	out = append(out, ring[next:]...)
	out = append(out, ring[:next]...)
	return out, total > max, nil
}

// readCappedLine reads one line without its line ending, keeping at most
// limit bytes of it and discarding the rest. err is nil when a newline ended
// the line, io.EOF at the end of input.
func readCappedLine(br *bufio.Reader, limit int) (string, error) {
	var buf []byte
	for {
		chunk, err := br.ReadSlice('\n')
		if room := limit - len(buf); room > 0 {
			buf = append(buf, chunk[:min(len(chunk), room)]...)
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		line := strings.TrimSuffix(strings.TrimSuffix(string(buf), "\n"), "\r")
		// A cut may split a multi-byte rune.
		return strings.ToValidUTF8(line, ""), err
	}
}

// RerunJob asks GitHub to re-run a single GitHub Actions job.
//
//	POST /repos/{owner}/{repo}/actions/jobs/{jobID}/rerun
func (c *Client) RerunJob(ctx context.Context, owner, repo string, jobID int64) error {
	status, body, err := c.sendJSON(ctx, "rest rerun job", http.MethodPost, buildRerunJobURL(c.BaseURL, owner, repo, jobID), nil)
	if err != nil {
		return err
	}
	if status != http.StatusCreated {
		return statusError(status, body)
	}
	return nil
}

// statusError reports an unexpected status, with GitHub's message when the
// body carries one ("This job is still running") rather than the raw JSON.
func statusError(status int, body []byte) error {
	var e apiError
	if json.Unmarshal(body, &e) == nil && e.Message != "" {
		return fmt.Errorf("%s (status %d)", e.Message, status)
	}
	return fmt.Errorf("rest: unexpected status %d: %s", status, string(body))
}
