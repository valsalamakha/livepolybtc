// Package api implements REST clients for the Polymarket CLOB (trading) and
// Gamma (market discovery) APIs, including L2 request signing.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/valsalamakha/livepolybtc/internal/auth"
)

// Client is an HTTP client for the Polymarket APIs. The zero value is not
// usable; construct it with New.
type Client struct {
	clobBase  string
	gammaBase string
	chainID   int

	httpc  *http.Client
	signer *auth.Signer

	// MaxRetries controls transient-failure retries for idempotent requests.
	MaxRetries int
	// RetryBackoff is the base backoff between retries.
	RetryBackoff time.Duration
}

// Options configures a Client.
type Options struct {
	CLOBBaseURL  string
	GammaBaseURL string
	ChainID      int
	Signer       *auth.Signer
	HTTPClient   *http.Client
	MaxRetries   int
	RetryBackoff time.Duration
}

// New constructs a Client. When opts.HTTPClient is nil a sensible default with
// timeouts is used. The default transport honours the HTTPS_PROXY environment.
func New(opts Options) *Client {
	hc := opts.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	backoff := opts.RetryBackoff
	if backoff <= 0 {
		backoff = 200 * time.Millisecond
	}
	return &Client{
		clobBase:     strings.TrimRight(opts.CLOBBaseURL, "/"),
		gammaBase:    strings.TrimRight(opts.GammaBaseURL, "/"),
		chainID:      opts.ChainID,
		httpc:        hc,
		signer:       opts.Signer,
		MaxRetries:   opts.MaxRetries,
		RetryBackoff: backoff,
	}
}

// APIError represents a non-2xx HTTP response.
type APIError struct {
	StatusCode int
	Body       string
	Method     string
	URL        string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s %s: status %d: %s", e.Method, e.URL, e.StatusCode, e.Body)
}

// Retryable reports whether the error is a transient failure worth retrying.
func (e *APIError) Retryable() bool {
	return e.StatusCode == http.StatusTooManyRequests || e.StatusCode >= 500
}

// request performs an HTTP request with optional L2 signing and retry. base is
// the host base URL; path is the request path; signed controls whether L2
// headers are attached. out, when non-nil, receives the decoded JSON response.
func (c *Client) request(ctx context.Context, method, base, path string, body any, signed bool, out any) error {
	var rawBody []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}
		rawBody = b
	}

	var lastErr error
	attempts := c.MaxRetries + 1
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(c.backoffFor(attempt)):
			}
		}

		err := c.do(ctx, method, base, path, rawBody, signed, out)
		if err == nil {
			return nil
		}
		lastErr = err

		var apiErr *APIError
		if ok := asAPIError(err, &apiErr); ok && !apiErr.Retryable() {
			return err
		}
		// Network errors and retryable API errors fall through to retry.
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return lastErr
}

func (c *Client) do(ctx context.Context, method, base, path string, rawBody []byte, signed bool, out any) error {
	url := base + path
	var reader io.Reader
	if rawBody != nil {
		reader = bytes.NewReader(rawBody)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if rawBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if signed {
		if c.signer == nil {
			return fmt.Errorf("signed request to %s requires credentials", path)
		}
		headers, err := c.signer.Headers(method, path, string(rawBody))
		if err != nil {
			return fmt.Errorf("sign request: %w", err)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
	}

	resp, err := c.httpc.Do(req)
	if err != nil {
		return fmt.Errorf("http do: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{StatusCode: resp.StatusCode, Body: string(respBody), Method: method, URL: url}
	}

	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode response from %s: %w", url, err)
		}
	}
	return nil
}

func (c *Client) backoffFor(attempt int) time.Duration {
	d := c.RetryBackoff
	for i := 1; i < attempt; i++ {
		d *= 2
	}
	return d
}

func asAPIError(err error, target **APIError) bool {
	for e := err; e != nil; {
		if ae, ok := e.(*APIError); ok {
			*target = ae
			return true
		}
		u, ok := e.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		e = u.Unwrap()
	}
	return false
}
