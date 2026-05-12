// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tenseleyFlow/shithub-cli/internal/build"
)

// DefaultTimeout is the per-request deadline applied when ClientOptions
// leaves Timeout unset. Long-running ops (clone, push) shell out to git,
// so 30s is plenty for any JSON API roundtrip.
const DefaultTimeout = 30 * time.Second

// DefaultMaxRetries is the upper bound on retry attempts; combined with
// the backoff cap, total wait stays bounded under a minute even in the
// pathological case.
const DefaultMaxRetries = 3

// DefaultMaxPages caps automatic pagination unless WithMaxPages overrides.
// 30 pages is generous enough for daily-driver use without DoS-of-self
// when --paginate hits a hot endpoint.
const DefaultMaxPages = 30

// EnvInsecureHTTP is the env var name that, when set to "1", lets the
// client talk to http:// URLs. Production builds error out otherwise;
// this is strictly a dev escape hatch for running shithub locally over
// plain HTTP.
const EnvInsecureHTTP = "SHITHUB_DEV_INSECURE_HTTP"

// TokenFunc returns the (token, source, error) tuple for a host. Wired
// to internal/config.ResolveToken in production; tests inject a fixed
// closure. Returning the source lets the client log "env" vs "keyring"
// without import-cycling into the config package.
type TokenFunc func(ctx context.Context, host string) (token, source string, err error)

// ClientOptions describes how to build a Client. Zero-value fields take
// the package defaults documented on each field.
type ClientOptions struct {
	// Host is the bare host (no scheme), e.g., "shithub.sh". When empty
	// the client defaults to "shithub.sh".
	Host string
	// BaseURL overrides scheme + host together. When set, Host is ignored
	// for URL composition (Host is still used for TokenFunc lookups so
	// the keyring service name is stable).
	BaseURL string
	// TokenFunc supplies the bearer token for outgoing requests. Required.
	TokenFunc TokenFunc
	// UserAgent overrides the default "shithub-cli/<version> (<os>/<arch>)".
	UserAgent string
	// Timeout overrides DefaultTimeout.
	Timeout time.Duration
	// MaxRetries overrides DefaultMaxRetries.
	MaxRetries int
	// Logger receives debug-level traces of every request when set;
	// nil disables logging entirely.
	Logger *slog.Logger
	// HTTPClient is the transport. nil installs a fresh http.Client with
	// our Timeout applied.
	HTTPClient *http.Client
}

// Client is shithub-cli's authenticated HTTP entrypoint. Construct one
// per command invocation with NewClient; do not share across goroutines
// in different invocations (the request context is the right cancellation
// channel for that case).
type Client struct {
	http      *http.Client
	host      string
	baseURL   string
	tokenFunc TokenFunc
	userAgent string
	timeout   time.Duration
	policy    retryPolicy
	logger    *slog.Logger
	whoami    whoamiCache
}

// NewClient returns a Client built from opts, applying defaults for any
// unset fields. Returns an error if TokenFunc is missing or the host /
// BaseURL cannot be resolved into a valid URL.
func NewClient(opts ClientOptions) (*Client, error) {
	if opts.TokenFunc == nil {
		return nil, errors.New("api: ClientOptions.TokenFunc is required")
	}

	host := strings.TrimSpace(opts.Host)
	if host == "" && opts.BaseURL == "" {
		host = "shithub.sh"
	}

	base := opts.BaseURL
	if base == "" {
		base = "https://" + host
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("api: parse BaseURL %q: %w", base, err)
	}
	if parsed.Scheme == "" {
		parsed.Scheme = "https"
	}
	if parsed.Scheme == "http" {
		// Allow http:// only via the dev escape hatch. This guards against
		// a misconfigured --hostname that accidentally drops to plaintext.
		if !insecureHTTPAllowed() {
			return nil, fmt.Errorf("api: refusing http:// (%s); set %s=1 to override", parsed.String(), EnvInsecureHTTP)
		}
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	retries := opts.MaxRetries
	if retries < 0 {
		retries = 0
	} else if retries == 0 {
		retries = DefaultMaxRetries
	}
	hc := opts.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: timeout}
	}
	ua := opts.UserAgent
	if ua == "" {
		ua = fmt.Sprintf("shithub-cli/%s (%s/%s)", build.Version, runtime.GOOS, runtime.GOARCH)
	}
	logger := opts.Logger
	if logger == nil {
		logger = newSilentLogger()
	}

	return &Client{
		http:      hc,
		host:      host,
		baseURL:   parsed.String(),
		tokenFunc: opts.TokenFunc,
		userAgent: ua,
		timeout:   timeout,
		policy:    defaultRetryPolicy(retries),
		logger:    logger,
	}, nil
}

// Host returns the host this client targets. Useful for callers that
// need to compose URLs outside the api package (e.g., browser open).
func (c *Client) Host() string { return c.host }

// BaseURL returns the resolved scheme://host[:port] string.
func (c *Client) BaseURL() string { return c.baseURL }

// requestOptions captures per-request overrides applied via RequestOption.
type requestOptions struct {
	owner    string
	repo     string
	headers  http.Header
	accept   string
	maxPages int
}

// RequestOption mutates a requestOptions for a single call. Use With*
// helpers; the type stays package-private so we can add fields without
// breaking callers.
type RequestOption func(*requestOptions)

// WithOwner sets the owner used for {owner} placeholder substitution.
func WithOwner(owner string) RequestOption {
	return func(o *requestOptions) { o.owner = owner }
}

// WithRepo sets the repo used for {repo} placeholder substitution.
func WithRepo(repo string) RequestOption {
	return func(o *requestOptions) { o.repo = repo }
}

// WithHeader adds (or overrides) an outgoing request header. Repeatable.
func WithHeader(name, value string) RequestOption {
	return func(o *requestOptions) {
		if o.headers == nil {
			o.headers = http.Header{}
		}
		o.headers.Set(name, value)
	}
}

// WithAccept overrides the default Accept: application/json header.
// Useful for shithub's `.diff` / `.patch` endpoints (text/plain).
func WithAccept(mime string) RequestOption {
	return func(o *requestOptions) { o.accept = mime }
}

// WithMaxPages caps DoPaginated. 0 means "use DefaultMaxPages"; a negative
// value means "no cap" — callers opting out of safety must know what
// they're asking for.
func WithMaxPages(n int) RequestOption {
	return func(o *requestOptions) { o.maxPages = n }
}

// REST issues an authenticated JSON request and decodes the response.
//
// `body` may be nil (no body sent), a value that JSON-marshals (struct,
// map, []byte), or an io.Reader for streaming bodies. `into` may be nil
// (discard the response) or a pointer to decode into.
//
// 2xx with non-empty body and into != nil → JSON unmarshal into into.
// 2xx with empty body or into == nil → return nil.
// Anything else → typed error (AuthError / ScopeError / ...).
func (c *Client) REST(ctx context.Context, method, path string, body, into any, opts ...RequestOption) error {
	resp, err := c.RESTRaw(ctx, method, path, body, opts...)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if into == nil {
		// Drain so the connection can return to the pool.
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("api: read response: %w", err)
	}
	if len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, into); err != nil {
		return fmt.Errorf("api: decode response: %w", err)
	}
	return nil
}

// RESTRaw is the lower-level call site for callers that need direct
// access to the *http.Response (header inspection, streaming download).
// On 4xx/5xx the body is consumed and a typed error returned; the
// caller's into-pointer (if any) is not relevant.
func (c *Client) RESTRaw(ctx context.Context, method, path string, body any, opts ...RequestOption) (*http.Response, error) {
	o := requestOptions{}
	for _, opt := range opts {
		opt(&o)
	}

	target, err := c.composeURL(path, o)
	if err != nil {
		return nil, err
	}

	bodyBytes, bodyReader, canReplay, err := prepareBody(body)
	if err != nil {
		return nil, err
	}

	token, _, err := c.tokenFunc(ctx, c.host)
	if err != nil {
		return nil, err
	}

	requestID := uuid.NewString()

	for attempt := 0; ; attempt++ {
		var reqBody io.Reader
		if canReplay && bodyBytes != nil {
			reqBody = bytes.NewReader(bodyBytes)
		} else if bodyReader != nil {
			reqBody = bodyReader
		}

		req, err := http.NewRequestWithContext(ctx, method, target, reqBody)
		if err != nil {
			return nil, fmt.Errorf("api: build request: %w", err)
		}

		c.applyHeaders(req, &o, token, requestID, bodyBytes != nil)

		start := time.Now()
		resp, transportErr := c.http.Do(req)
		c.logRequest(method, target, resp, transportErr, start, requestID)

		if transportErr == nil && resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return resp, nil
		}

		nap, shouldRetry := c.policy.shouldRetry(method, resp, transportErr, canReplay, attempt)
		if !shouldRetry {
			if transportErr != nil {
				return nil, transportErr
			}
			return nil, c.errorFromResponse(resp)
		}

		if resp != nil {
			_ = resp.Body.Close()
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(nap):
		}
	}
}

// DoPaginated yields one response body per page, following the Link
// header's rel="next". The iterator stops on any error and on the
// max-pages cap (DefaultMaxPages unless WithMaxPages overrides).
//
// Use range-over-func:
//
//	for raw, err := range c.DoPaginated(ctx, "GET", "repos/{owner}/{repo}/issues",
//	    api.WithOwner("o"), api.WithRepo("r")) {
//	    if err != nil { ... }
//	    var page []Issue
//	    _ = json.Unmarshal(raw, &page)
//	    ...
//	}
func (c *Client) DoPaginated(ctx context.Context, method, path string, opts ...RequestOption) iter.Seq2[json.RawMessage, error] {
	return func(yield func(json.RawMessage, error) bool) {
		o := requestOptions{}
		for _, opt := range opts {
			opt(&o)
		}
		cap := o.maxPages
		switch {
		case cap == 0:
			cap = DefaultMaxPages
		case cap < 0:
			cap = -1 // unbounded; trust the caller
		}

		nextURL := ""
		page := 0
		for {
			page++
			if cap > 0 && page > cap {
				return
			}

			var (
				resp *http.Response
				err  error
			)
			if nextURL == "" {
				// First page: build via composeURL like a normal call.
				resp, err = c.RESTRaw(ctx, method, path, nil, opts...)
			} else {
				// Subsequent pages: the Link URL is already absolute; bypass composeURL.
				resp, err = c.doRaw(ctx, method, nextURL, nil, o)
			}
			if err != nil {
				yield(nil, err)
				return
			}

			data, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if readErr != nil {
				yield(nil, fmt.Errorf("api: read page %d: %w", page, readErr))
				return
			}

			if !yield(json.RawMessage(data), nil) {
				return
			}

			links := ParseLinkHeader(resp.Header.Get("Link"))
			nextURL = links["next"]
			if nextURL == "" {
				return
			}
		}
	}
}

// doRaw is the "URL is already absolute" variant used by pagination for
// pages beyond the first. Shares header/auth wiring with RESTRaw but
// skips composeURL.
func (c *Client) doRaw(ctx context.Context, method, target string, body any, o requestOptions) (*http.Response, error) {
	bodyBytes, _, _, err := prepareBody(body)
	if err != nil {
		return nil, err
	}
	token, _, err := c.tokenFunc(ctx, c.host)
	if err != nil {
		return nil, err
	}
	requestID := uuid.NewString()

	var reqBody io.Reader
	if bodyBytes != nil {
		reqBody = bytes.NewReader(bodyBytes)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reqBody)
	if err != nil {
		return nil, err
	}
	c.applyHeaders(req, &o, token, requestID, bodyBytes != nil)

	start := time.Now()
	resp, transportErr := c.http.Do(req)
	c.logRequest(method, target, resp, transportErr, start, requestID)
	if transportErr != nil {
		return nil, transportErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, c.errorFromResponse(resp)
	}
	return resp, nil
}

// composeURL resolves placeholders, prefixes /api/v1 when needed, and
// joins against the client's base URL. Returns the absolute URL string.
func (c *Client) composeURL(path string, o requestOptions) (string, error) {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path, nil
	}
	p := path
	p = strings.ReplaceAll(p, "{owner}", o.owner)
	p = strings.ReplaceAll(p, "{repo}", o.repo)

	if !strings.HasPrefix(p, "/api/") && !strings.HasPrefix(p, "/login") {
		p = strings.TrimPrefix(p, "/")
		p = "/api/v1/" + p
	} else if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}

	target, err := url.Parse(c.baseURL + p)
	if err != nil {
		return "", fmt.Errorf("api: build URL %q: %w", p, err)
	}
	return target.String(), nil
}

// applyHeaders sets the standard auth + identity headers plus any caller
// overrides. Authorization uses the gh-compatible "token <pat>" form which
// shithub's PAT middleware accepts (along with Bearer).
func (c *Client) applyHeaders(req *http.Request, o *requestOptions, token, requestID string, hasBody bool) {
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("X-Request-Id", requestID)

	accept := o.accept
	if accept == "" {
		accept = "application/json"
	}
	req.Header.Set("Accept", accept)

	if hasBody && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	for k, vs := range o.headers {
		for _, v := range vs {
			req.Header.Set(k, v)
		}
	}
}

// errorFromResponse drains the body, classifies, and returns the typed
// error. The response body is always closed before this returns.
func (c *Client) errorFromResponse(resp *http.Response) error {
	if resp == nil {
		return errors.New("api: nil response")
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return classifyResponse(resp, body)
}

// logRequest emits a single structured slog event per attempt. Logs are
// silenced by default (newSilentLogger) so this is no-op overhead in
// production. The URL and any headers we surface go through redaction
// so tokens never reach stderr or sinks.
func (c *Client) logRequest(method, target string, resp *http.Response, err error, start time.Time, requestID string) {
	if c.logger == nil {
		return
	}
	args := []any{
		"method", method,
		"url", redactURL(target),
		"request_id", requestID,
		"elapsed_ms", time.Since(start).Milliseconds(),
	}
	if resp != nil {
		args = append(args, "status", resp.StatusCode)
	}
	if err != nil {
		args = append(args, "error", err.Error())
		c.logger.Error("http request", args...)
		return
	}
	c.logger.Debug("http request", args...)
}

// prepareBody normalizes the body argument:
//
//	nil               → (nil, nil, true, nil)
//	io.Reader         → (nil, reader, false, nil) — single-pass, no retry
//	[]byte            → ([]byte, nil, true, nil)
//	json.RawMessage   → ([]byte, nil, true, nil)
//	any other type    → (json.Marshal(v), nil, true, nil)
//
// canReplay tells the retry policy whether we can re-issue a write on a
// transient failure. io.Reader bodies are not retried because we can't
// rewind them safely.
func prepareBody(body any) (bytes []byte, reader io.Reader, canReplay bool, err error) {
	if body == nil {
		return nil, nil, true, nil
	}
	if r, ok := body.(io.Reader); ok {
		return nil, r, false, nil
	}
	if b, ok := body.([]byte); ok {
		return b, nil, true, nil
	}
	if rm, ok := body.(json.RawMessage); ok {
		return rm, nil, true, nil
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, nil, false, fmt.Errorf("api: marshal body: %w", err)
	}
	return encoded, nil, true, nil
}

// insecureHTTPAllowed reports whether the dev escape hatch is active.
// Centralized so tests can swap it via t.Setenv.
func insecureHTTPAllowed() bool {
	return getenv(EnvInsecureHTTP) == "1"
}

// getenv is a thin wrapper so tests can monkey-patch if ever needed.
// Currently it's just os.Getenv; the indirection is here so the rest of
// the package never imports os directly.
var getenv = func(key string) string {
	// Indirected through a function variable to keep os off the import
	// surface of files that don't otherwise need it. The actual lookup
	// happens via the unexported osLookupEnv helper.
	return osLookupEnv(key)
}
