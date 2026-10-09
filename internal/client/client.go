// Package client is a small HTTP client for the Hostereo External API v1.
//
// It handles the response envelope ({"data", "meta"} / {"error", "meta"}),
// retries with backoff on 429 and transient 5xx (honouring Retry-After), and
// polling of asynchronous operations. The API key is only ever sent in the
// Authorization header and never logged.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const DefaultEndpoint = "https://api-public.hostereo.com/api/v1"

// Client talks to the Hostereo External API.
type Client struct {
	endpoint   string
	apiKey     string
	userAgent  string
	httpClient *http.Client

	// MaxRetries is the number of retries after the first attempt.
	MaxRetries int
	// Sleep is replaceable in tests.
	Sleep func(ctx context.Context, d time.Duration) error
	// PollInterval is the default delay between operation polls when the
	// server sends no Retry-After header.
	PollInterval time.Duration
}

// New returns a client. endpoint may be empty for the default.
func New(endpoint, apiKey, userAgent string) *Client {
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	return &Client{
		endpoint:     strings.TrimRight(endpoint, "/"),
		apiKey:       apiKey,
		userAgent:    userAgent,
		httpClient:   &http.Client{Timeout: 60 * time.Second},
		MaxRetries:   5,
		Sleep:        sleepCtx,
		PollInterval: 5 * time.Second,
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// APIError is an error response from the API.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
	Details    map[string][]string
	RequestID  string
}

func (e *APIError) Error() string {
	msg := fmt.Sprintf("hostereo API: HTTP %d", e.StatusCode)
	if e.Code != "" {
		msg += " " + e.Code
	}
	if e.Message != "" {
		msg += ": " + e.Message
	}
	for field, msgs := range e.Details {
		msg += fmt.Sprintf(" [%s: %s]", field, strings.Join(msgs, "; "))
	}
	if e.RequestID != "" {
		msg += " (request_id " + e.RequestID + ")"
	}
	return msg
}

// IsNotFound reports whether err is a 404 from the API.
func IsNotFound(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.StatusCode == http.StatusNotFound
}

// HasCode reports whether err is an API error with the given error.code.
func HasCode(err error, code string) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.Code == code
}

type envelope struct {
	Data  json.RawMessage `json:"data"`
	Meta  json.RawMessage `json:"meta"`
	Error *struct {
		Code    string              `json:"code"`
		Message string              `json:"message"`
		Status  int                 `json:"status"`
		Details map[string][]string `json:"details"`
	} `json:"error"`
}

// Response is a decoded success response.
type Response struct {
	StatusCode int
	Data       json.RawMessage
	Meta       json.RawMessage
	RetryAfter time.Duration
}

// Do performs a request. body may be nil. headers may be nil. Retries on 429
// always, and on 5xx only for requests that are safe to repeat (anything but
// POST, or a POST carrying an Idempotency-Key).
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body any, headers map[string]string) (*Response, error) {
	var payload []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encoding request: %w", err)
		}
		payload = b
	}
	u := c.endpoint + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	_, idempotent := headers["Idempotency-Key"]
	retry5xx := method != http.MethodPost || idempotent

	var lastErr error
	for attempt := 0; ; attempt++ {
		var rdr io.Reader
		if payload != nil {
			rdr = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, u, rdr)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if c.userAgent != "" {
			req.Header.Set("User-Agent", c.userAgent)
		}
		if c.apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+c.apiKey)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("%s %s: %w", method, path, err)
			if ctx.Err() != nil || attempt >= c.MaxRetries || !retry5xx {
				return nil, lastErr
			}
			if err := c.Sleep(ctx, backoff(attempt, 0)); err != nil {
				return nil, err
			}
			continue
		}
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("%s %s: reading response: %w", method, path, readErr)
		}
		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))

		if resp.StatusCode < 400 {
			out := &Response{StatusCode: resp.StatusCode, RetryAfter: retryAfter}
			if len(bytes.TrimSpace(raw)) > 0 {
				var env envelope
				if err := json.Unmarshal(raw, &env); err != nil {
					return nil, fmt.Errorf("%s %s: decoding response: %w", method, path, err)
				}
				out.Data, out.Meta = env.Data, env.Meta
			}
			return out, nil
		}

		apiErr := decodeError(resp.StatusCode, raw)
		lastErr = apiErr
		retryable := resp.StatusCode == http.StatusTooManyRequests ||
			(retry5xx && (resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusGatewayTimeout))
		if !retryable || attempt >= c.MaxRetries {
			return nil, apiErr
		}
		if err := c.Sleep(ctx, backoff(attempt, retryAfter)); err != nil {
			return nil, err
		}
	}
}

func decodeError(status int, raw []byte) *APIError {
	ae := &APIError{StatusCode: status}
	var env envelope
	if json.Unmarshal(raw, &env) == nil && env.Error != nil {
		ae.Code, ae.Message, ae.Details = env.Error.Code, env.Error.Message, env.Error.Details
		var meta struct {
			RequestID string `json:"request_id"`
		}
		if json.Unmarshal(env.Meta, &meta) == nil {
			ae.RequestID = meta.RequestID
		}
	} else {
		ae.Message = strings.TrimSpace(string(raw))
		if len(ae.Message) > 200 {
			ae.Message = ae.Message[:200]
		}
	}
	return ae
}

func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if s, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && s >= 0 {
		return time.Duration(s) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

func backoff(attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		if retryAfter > 120*time.Second {
			return 120 * time.Second
		}
		return retryAfter
	}
	d := time.Second << attempt
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d + time.Duration(rand.Int63n(int64(250*time.Millisecond)))
}

// GetData performs a GET and decodes data into out.
func (c *Client) GetData(ctx context.Context, path string, query url.Values, out any) error {
	resp, err := c.Do(ctx, http.MethodGet, path, query, nil, nil)
	if err != nil {
		return err
	}
	return decodeData(resp, out)
}

func decodeData(resp *Response, out any) error {
	if out == nil || len(resp.Data) == 0 {
		return nil
	}
	if err := json.Unmarshal(resp.Data, out); err != nil {
		return fmt.Errorf("decoding data: %w", err)
	}
	return nil
}

// ListAll follows page/per_page pagination for list endpoints.
func ListAll[T any](ctx context.Context, c *Client, path string, perPage int) ([]T, error) {
	var all []T
	for page := 1; page <= 1000; page++ {
		q := url.Values{"page": {strconv.Itoa(page)}, "per_page": {strconv.Itoa(perPage)}}
		resp, err := c.Do(ctx, http.MethodGet, path, q, nil, nil)
		if err != nil {
			return nil, err
		}
		var items []T
		if err := decodeData(resp, &items); err != nil {
			return nil, err
		}
		all = append(all, items...)
		var meta struct {
			LastPage   int `json:"last_page"`
			Pagination struct {
				LastPage int `json:"last_page"`
			} `json:"pagination"`
		}
		_ = json.Unmarshal(resp.Meta, &meta)
		last := meta.LastPage
		if last == 0 {
			last = meta.Pagination.LastPage
		}
		if len(items) < perPage || last == 0 || page >= last {
			return all, nil
		}
	}
	return all, nil
}
