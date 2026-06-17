package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// maxResponseBytes caps how much of a response body we read, guarding against a
// hostile or broken server streaming forever (GOAL.md §9 decode discipline).
const maxResponseBytes = 32 << 20 // 32 MiB

// pageSize is clamped to 25: Google caps exercise/sleep dataPoints at 25 per
// page regardless of the documented 10k max (GOAL.md §9).
const pageSize = 25

// Error is a typed API failure. The CLI maps it to exit code 2 (GOAL.md §12).
// Message is Google's error-envelope message when available, else an HTTP/IO
// description.
type Error struct {
	StatusCode int
	Message    string
}

func (e *Error) Error() string { return e.Message }

// Client issues read-only exercise dataPoints.list requests. The *http.Client it
// holds already injects + refreshes the OAuth2 bearer token and carries the
// request timeout (built in internal/auth); this type only owns URL building,
// pagination, and decode (GOAL.md §9).
type Client struct {
	http    *http.Client
	baseURL string // e.g. https://health.googleapis.com
	user    string // e.g. users/me
	logger  *slog.Logger
}

// New builds a Client. baseURL and user come from config (both overridable as a
// test seam); httpClient is the authenticated client from internal/auth.
func New(httpClient *http.Client, baseURL, user string, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Client{
		http:    httpClient,
		baseURL: strings.TrimRight(baseURL, "/"),
		user:    strings.Trim(user, "/"),
		logger:  logger,
	}
}

// ListExerciseDataPoints returns every exercise data point whose civil start
// time falls in [from, to). It follows nextPageToken until the API stops
// paginating (GOAL.md §9). from/to are naive local civil datetimes.
func (c *Client) ListExerciseDataPoints(ctx context.Context, from, to time.Time) ([]DataPoint, error) {
	filter := FilterFromRange(from, to)
	endpoint := fmt.Sprintf("%s/v4/%s/dataTypes/exercise/dataPoints", c.baseURL, c.user)

	var points []DataPoint
	pageToken := ""
	for {
		q := url.Values{}
		q.Set("filter", filter)
		q.Set("pageSize", fmt.Sprintf("%d", pageSize))
		if pageToken != "" {
			q.Set("pageToken", pageToken)
		}
		reqURL := endpoint + "?" + q.Encode()

		resp, err := c.get(ctx, reqURL)
		if err != nil {
			return nil, err
		}
		for _, raw := range resp.DataPoints {
			var dp DataPoint
			if err := json.Unmarshal(raw, &dp); err != nil {
				// A single malformed point shouldn't abort the list, nor should
				// it disappear from `sessions --raw`. Keep the raw bytes; the
				// zero-valued typed fields make ParseSession yield nils, exactly
				// as the tolerant Python parse_session does (GOAL.md §12).
				c.logger.Warn("data point typed-decode failed; keeping raw", "err", err)
			}
			dp.Raw = raw
			points = append(points, dp)
		}
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	return points, nil
}

// get performs one GET and decodes the list envelope, mapping non-2xx responses
// to a typed *Error carrying Google's error message (GOAL.md §9).
func (c *Client) get(ctx context.Context, reqURL string) (*listResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, &Error{Message: fmt.Sprintf("request failed: %v", err)}
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, &Error{StatusCode: resp.StatusCode, Message: fmt.Sprintf("read response: %v", err)}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &Error{StatusCode: resp.StatusCode, Message: errorMessage(data, resp.StatusCode)}
	}

	var out listResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, &Error{StatusCode: resp.StatusCode, Message: fmt.Sprintf("decode response: %v", err)}
	}
	return &out, nil
}

// errorMessage extracts Google's error-envelope message, falling back to a
// generic HTTP status description.
func errorMessage(body []byte, status int) string {
	var ae apiError
	if err := json.Unmarshal(body, &ae); err == nil && ae.Error.Message != "" {
		return ae.Error.Message
	}
	snippet := strings.TrimSpace(string(body))
	if len(snippet) > 300 {
		snippet = snippet[:300]
	}
	if snippet == "" {
		return fmt.Sprintf("HTTP %d", status)
	}
	return fmt.Sprintf("HTTP %d: %s", status, snippet)
}
