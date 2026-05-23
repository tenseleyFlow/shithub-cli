// SPDX-License-Identifier: AGPL-3.0-or-later

// Package api implements `shithub api` — the raw HTTP escape hatch.
// Every flag and field-typing rule mirrors gh so users can port their
// `gh api ...` scripts byte-for-byte via `s/gh /shithub /`.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/itchyny/gojq"
	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
)

// Options holds the parsed flag state. Exposed so external callers
// (notably tests) can drive Run without going through cobra.
type Options struct {
	IO         *iostreams.IOStreams
	HTTPClient func(host string) (*api.Client, error)

	// Endpoint is the positional <path> argument.
	Endpoint string

	// Method, when empty, defaults to GET — or POST when any body source
	// is present (-F/-f/--input).
	Method string
	// MethodSet distinguishes "user didn't pass -X" (default-GET path)
	// from "user passed -X with an empty value" (typo). Populated from
	// `c.Flags().Changed("method")` in RunE. I36: pre-fix `api -X ""`
	// silently defaulted to GET — hide-the-typo behavior.
	MethodSet bool

	// Headers gathered from -H. Repeatable.
	RawHeaders []string

	// Typed and raw field sources.
	Fields    []string
	RawFields []string

	// Input is the path passed to --input. Empty when unset; "-" reads
	// from stdin. Mutually exclusive with -F/-f.
	Input string

	// Paginate follows Link rel="next" until exhaustion.
	Paginate bool
	// Slurp combined with --paginate wraps pages in an outer JSON array.
	Slurp bool

	// JQ is the gojq filter expression.
	JQ string
	// Template is the text/template body.
	Template string

	// Cache is the TTL for GET responses (e.g., "1h"). Empty disables.
	Cache string

	// IncludeHeaders prepends response headers to the body output.
	IncludeHeaders bool
	// Silent suppresses the body output entirely (useful with --include).
	Silent bool
	// Verbose dumps request + response to stderr (token-redacted).
	Verbose bool

	// Hostname overrides the active host for this call.
	Hostname string
	// RepoFlag drives {owner}/{repo} placeholder resolution.
	RepoFlag string

	// Preview sets Accept: application/vnd.shithub.<name>-preview+json.
	Preview string

	// MaxPages caps --paginate. Zero means default (api.DefaultMaxPages);
	// negative disables the cap.
	MaxPages int
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &Options{
		IO:         f.IOStreams,
		HTTPClient: f.HTTPClient,
	}
	cmd := &cobra.Command{
		Use:   "api <endpoint>",
		Short: "Make an authenticated HTTP request to a shithub API endpoint",
		Long: `Invoke any shithub HTTP endpoint with auth + host wired in.

The <endpoint> argument is a path (e.g. 'user' or 'repos/{owner}/{repo}/issues');
shithub-cli prepends /api/v1 when the path does not start with '/'.

Placeholders ({owner}, {repo}, {branch}) resolve from --repo, SHITHUB_REPO,
or the current working tree's git remote — in that order.

Field flags map to a JSON body for POST/PATCH or to query string for GET:
  -F key=value     typed (true/false/null/integer/float autodetected; @file reads from disk; @- from stdin)
  -f key=value     raw (always a string)
  --input <file>   raw body from a file (mutually exclusive with -F/-f)

For paginated endpoints, --paginate follows Link rel="next" and concatenates
array bodies; --slurp wraps pages in an outer array.`,
		Args: cobra.ExactArgs(1),
		Example: `  # Read the authenticated user record
  shithub api user

  # Create an issue from typed fields
  shithub api repos/{owner}/{repo}/issues -F title=Hello -F labels[]=bug -F labels[]=ux

  # Paginate, filter via jq, print one ID per line
  shithub api repos/{owner}/{repo}/issues --paginate -q '.[].number' -R mf/cli`,
		RunE: func(c *cobra.Command, args []string) error {
			opts.Endpoint = args[0]
			opts.MethodSet = c.Flags().Changed("method")
			return Run(c.Context(), opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Method, "method", "X", "", "HTTP method (default: GET, or POST when -F/-f/--input is used)")
	cmd.Flags().StringArrayVarP(&opts.RawHeaders, "header", "H", nil, "add an outgoing request header: 'name: value' (repeatable)")
	cmd.Flags().StringArrayVarP(&opts.Fields, "field", "F", nil, "add a typed field: key=value (true/false/null/integer/float autodetected; @file or @-)")
	cmd.Flags().StringArrayVarP(&opts.RawFields, "raw-field", "f", nil, "add a string field: key=value")
	cmd.Flags().StringVar(&opts.Input, "input", "", "read the request body from a file ('-' for stdin)")
	cmd.Flags().BoolVar(&opts.Paginate, "paginate", false, "follow Link rel=\"next\" until exhaustion")
	cmd.Flags().BoolVar(&opts.Slurp, "slurp", false, "with --paginate, wrap pages in an outer JSON array instead of concatenating")
	cmd.Flags().StringVarP(&opts.JQ, "jq", "q", "", "filter the JSON response through a jq expression")
	cmd.Flags().StringVarP(&opts.Template, "template", "t", "", "format the JSON response with a Go template")
	cmd.Flags().StringVar(&opts.Cache, "cache", "", "cache GET responses on disk for the given duration (e.g. 1h); ignored for non-GET methods with a warning")
	cmd.Flags().BoolVarP(&opts.IncludeHeaders, "include", "i", false, "include the response headers in the output")
	cmd.Flags().BoolVar(&opts.Silent, "silent", false, "suppress the response body (useful with --include)")
	cmd.Flags().BoolVar(&opts.Verbose, "verbose", false, "print request and response to stderr (token-redacted)")
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "override the active host for this request")
	cmd.Flags().StringVarP(&opts.RepoFlag, "repo", "R", "", "the owner/repo used for {owner}/{repo} placeholders")
	cmd.Flags().StringVar(&opts.Preview, "preview", "", "request a shithub feature preview (Accept: application/vnd.shithub.<name>-preview+json)")
	cmd.Flags().IntVar(&opts.MaxPages, "max-pages", 0, "cap pagination at N pages (default 30; pass a negative value to disable)")
	return cmd
}

// Run is the parameterized entrypoint exposed for tests. Returns nil on
// success; typed api errors (AuthError, etc.) flow up unchanged so
// cobra surfaces them via Execute.
func Run(ctx context.Context, opts *Options) error {
	if err := validate(opts); err != nil {
		return err
	}

	var host string
	if opts.Hostname != "" {
		h, err := config.ValidateHost(opts.Hostname)
		if err != nil {
			return err
		}
		host = h
	}
	spec, err := resolvePlaceholders(opts.RepoFlag, host)
	if err != nil {
		return err
	}
	endpoint, err := spec.substitute(opts.Endpoint)
	if err != nil {
		return err
	}

	client, err := opts.HTTPClient(host)
	if err != nil {
		return err
	}

	body, isJSONBody, err := buildBody(opts)
	if err != nil {
		return err
	}

	method := opts.Method
	if method == "" {
		if body != nil {
			method = http.MethodPost
		} else {
			method = http.MethodGet
		}
	}

	// C12: -f/-F on GET should produce query-string params, not a
	// JSON body. Pre-D3b the body got attached to GET and the server
	// rejected with a confusing JSON-parse error. gh-compat: GET
	// fields land on the URL.
	if strings.EqualFold(method, http.MethodGet) && body != nil && isJSONBody {
		qs, qerr := queryStringFromFields(opts.Fields, opts.RawFields, opts.IO.In)
		if qerr != nil {
			return qerr
		}
		if qs != "" {
			if strings.Contains(endpoint, "?") {
				endpoint += "&" + qs
			} else {
				endpoint += "?" + qs
			}
		}
		body = nil
		isJSONBody = false
	}

	headers, err := buildHeaders(opts, isJSONBody)
	if err != nil {
		return err
	}

	if opts.Paginate {
		return runPaginated(ctx, opts, client, method, endpoint, headers)
	}
	return runSingle(ctx, opts, client, method, endpoint, headers, body)
}

// validate enforces the mutual-exclusion rules announced in help text.
func validate(opts *Options) error {
	// Audit A9: an empty endpoint reached the server for a useless
	// 404 and confused the user. Cobra's ExactArgs(1) admits the
	// empty string as "one arg"; reject it explicitly so the error
	// surfaces locally without a wire roundtrip.
	if strings.TrimSpace(opts.Endpoint) == "" {
		return errors.New("api: endpoint path is required (e.g. shithub api user)")
	}
	// H17: `shithub api http://shithub.sh/...` sends the bearer token
	// over plaintext HTTP on the first hop. The server's 308 → HTTPS
	// is irrelevant — the token is on the wire before the redirect
	// arrives. Refuse plain http:// for absolute URLs; the user can
	// drop the scheme or fix the typo.
	if strings.HasPrefix(strings.ToLower(opts.Endpoint), "http://") {
		return errors.New("api: refusing plain http:// URL — switch to https:// (the bearer token would otherwise be sent over plaintext before the server's redirect)")
	}
	// I36: `-X ""` (explicit empty value) is a typo, not a request to
	// silently default to GET. Distinguish from "user didn't pass -X"
	// via MethodSet so the default-GET path stays untouched.
	if opts.MethodSet && strings.TrimSpace(opts.Method) == "" {
		return errors.New("api: -X requires a value (e.g. -X GET, -X POST)")
	}
	// H18: HTTP methods are case-sensitive per RFC 9110, but every
	// well-known client uppercases by convention. Pre-fix, `api -X get`
	// produced a server-side `405 method get not allowed`. Normalize.
	if opts.Method != "" {
		opts.Method = strings.ToUpper(opts.Method)
	}
	if opts.JQ != "" && opts.Template != "" {
		return errors.New("api: --jq and --template are mutually exclusive")
	}
	if opts.Input != "" && (len(opts.Fields) > 0 || len(opts.RawFields) > 0) {
		return errors.New("api: --input is mutually exclusive with -F/-f")
	}
	if opts.Cache != "" {
		if _, err := time.ParseDuration(opts.Cache); err != nil {
			return fmt.Errorf("api: --cache duration: %w", err)
		}
		if opts.Paginate {
			return errors.New("api: --cache is not supported with --paginate")
		}
	}
	if opts.Slurp && !opts.Paginate {
		return errors.New("api: --slurp requires --paginate")
	}
	// H21: pre-fix, `--include --paginate` silently dropped subsequent
	// pages' headers (Link, X-RateLimit, X-Request-Id…). Multiple
	// per-page header blocks don't compose into a single stream; rather
	// than render only the first set and lie about the rest, refuse
	// the combination. Users debugging headers should drop --paginate
	// and walk the Link chain themselves.
	if opts.IncludeHeaders && opts.Paginate {
		return errors.New("api: --include and --paginate are mutually exclusive (per-page headers can't compose into a single stream); run without --paginate to inspect headers, or drop --include for the merged body")
	}
	return nil
}

// buildBody assembles the outgoing body bytes. Returns (nil, false, nil)
// when no body is supplied (typical GET). The isJSONBody flag tells the
// caller whether to default Content-Type: application/json.
func buildBody(opts *Options) ([]byte, bool, error) {
	if opts.Input != "" {
		raw, err := readInput(opts.Input, opts.IO.In)
		if err != nil {
			return nil, false, err
		}
		return raw, false, nil
	}
	entries, err := fieldEntriesFromFlags(opts.Fields, opts.RawFields)
	if err != nil {
		return nil, false, err
	}
	if len(entries) == 0 {
		return nil, false, nil
	}
	encoded, err := buildJSONBody(entries, opts.IO.In)
	if err != nil {
		return nil, false, err
	}
	return encoded, true, nil
}

// readInput slurps the --input source. "-" pulls from stdin; anything
// else is a file path.
func readInput(path string, stdin io.Reader) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(stdin)
	}
	return readFileContents(path)
}

// readFileContents wraps os.ReadFile so an extra import doesn't sneak
// into other files in the package.
var readFileContents = func(path string) ([]byte, error) {
	return readFile(path)
}

// buildHeaders parses -H flags, sets Accept (with preview override),
// and lets the api.Client layer add Authorization itself.
func buildHeaders(opts *Options, isJSONBody bool) (http.Header, error) {
	h := http.Header{}
	for _, raw := range opts.RawHeaders {
		idx := strings.IndexByte(raw, ':')
		if idx <= 0 {
			return nil, fmt.Errorf("api: header %q must be 'name: value'", raw)
		}
		name := strings.TrimSpace(raw[:idx])
		value := strings.TrimSpace(raw[idx+1:])
		h.Add(name, value)
	}
	if opts.Preview != "" {
		h.Set("Accept", fmt.Sprintf("application/vnd.shithub.%s-preview+json", opts.Preview))
	}
	if isJSONBody && h.Get("Content-Type") == "" {
		h.Set("Content-Type", "application/json")
	}
	return h, nil
}

// runSingle handles one request, applying cache + jq/template + verbose.
func runSingle(ctx context.Context, opts *Options, client *api.Client, method, endpoint string, headers http.Header, body []byte) error {
	cacheTTL, _ := parseCacheTTL(opts.Cache)
	cacheKey := ""

	// --cache is GET-only by design (mutating methods aren't cacheable
	// in any meaningful sense). Warn loudly when the combination is
	// nonsensical rather than silently dropping the cache — users hit
	// this when they paste an existing GET command and add -X PUT.
	// Audit #150.
	if cacheTTL > 0 && !strings.EqualFold(method, http.MethodGet) {
		fmt.Fprintf(opts.IO.ErrOut, "warning: --cache only applies to GET; ignoring for %s\n", method)
	}

	// Cache lookup (GET only).
	if cacheTTL > 0 && strings.EqualFold(method, http.MethodGet) {
		cacheKey = CacheKey(client.Host(), method, endpoint, headers, body)
		if entry, hit, _ := CacheGet(cacheKey, cacheTTL); hit {
			return emit(opts, entry.Status, entry.Headers, entry.Body)
		}
	}

	if opts.Verbose {
		printVerboseRequest(opts.IO, method, client.BaseURL()+resolveLeading(endpoint), headers, body)
	}

	reqOpts := []api.RequestOption{}
	for k, vs := range headers {
		for _, v := range vs {
			reqOpts = append(reqOpts, api.WithHeader(k, v))
		}
	}

	var reqBody any
	if body != nil {
		reqBody = body
	}
	resp, err := client.RESTRaw(ctx, method, endpoint, reqBody, reqOpts...)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("api: read response: %w", err)
	}

	if opts.Verbose {
		printVerboseResponse(opts.IO, resp.StatusCode, resp.Header, rawBody)
	}

	if cacheKey != "" && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		_ = CachePut(cacheKey, resp.StatusCode, resp.Header, rawBody)
	}

	return emit(opts, resp.StatusCode, resp.Header, rawBody)
}

// emit writes the response per the user's --include/--silent/--jq/--template flags.
func emit(opts *Options, status int, headers http.Header, body []byte) error {
	if opts.IncludeHeaders {
		writeHeaders(opts.IO, status, headers)
	}
	if opts.Silent {
		return nil
	}
	if opts.JQ != "" {
		return runJQ(opts.IO.Out, opts.JQ, body)
	}
	if opts.Template != "" {
		return runTemplate(opts.IO.Out, opts.Template, body)
	}
	// Default: emit body verbatim with a trailing newline if missing.
	if _, err := opts.IO.Out.Write(body); err != nil {
		return err
	}
	if len(body) == 0 || body[len(body)-1] != '\n' {
		_, _ = opts.IO.Out.Write([]byte{'\n'})
	}
	return nil
}

// runJQ pipes body through the supplied gojq expression, emitting one
// value per line. Strings are unquoted (matches `jq -r` behavior).
func runJQ(out io.Writer, expr string, body []byte) error {
	q, err := gojq.Parse(expr)
	if err != nil {
		return fmt.Errorf("api: parse jq: %w", err)
	}
	var input any
	if err := json.Unmarshal(body, &input); err != nil {
		return fmt.Errorf("api: decode for jq: %w", err)
	}
	iter := q.Run(input)
	for {
		v, ok := iter.Next()
		if !ok {
			return nil
		}
		if e, isErr := v.(error); isErr {
			return fmt.Errorf("api: jq runtime: %w", e)
		}
		if s, ok := v.(string); ok {
			fmt.Fprintln(out, s)
			continue
		}
		enc, err := json.Marshal(v)
		if err != nil {
			return err
		}
		if _, err := out.Write(append(enc, '\n')); err != nil {
			return err
		}
	}
}

// runTemplate parses tmpl and executes it against the decoded body.
// Audit A6: gh's `--template` appends a trailing newline when the
// template body itself doesn't end in one — without it the rendered
// value runs into the next shell prompt or pipeline reader's line
// boundary. We buffer the template output, then add `\n` iff missing.
func runTemplate(out io.Writer, tmpl string, body []byte) error {
	// C13: missingkey=error means `{{.totally_missing}}` returns a
	// template-execution error instead of printing the literal string
	// "<no value>". Scripts piping into `xargs` or `read` no longer
	// silently consume an unintended marker.
	t, err := template.New("api").Option("missingkey=error").Parse(tmpl)
	if err != nil {
		return fmt.Errorf("api: parse template: %w", err)
	}
	var input any
	if err := json.Unmarshal(body, &input); err != nil {
		return fmt.Errorf("api: decode for template: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, input); err != nil {
		return err
	}
	rendered := buf.Bytes()
	if _, err := out.Write(rendered); err != nil {
		return err
	}
	if len(rendered) == 0 || rendered[len(rendered)-1] != '\n' {
		_, err := io.WriteString(out, "\n")
		return err
	}
	return nil
}

// writeHeaders renders the status line + headers in HTTP-ish text form
// for --include. Authorization is never echoed (we never send it on this
// path either, but a defense-in-depth scrub keeps the contract explicit).
func writeHeaders(ios *iostreams.IOStreams, status int, h http.Header) {
	fmt.Fprintf(ios.Out, "HTTP/1.1 %d %s\n", status, http.StatusText(status))
	for k, vs := range h {
		for _, v := range vs {
			if strings.EqualFold(k, "Authorization") {
				v = "[redacted]"
			}
			fmt.Fprintf(ios.Out, "%s: %s\n", k, v)
		}
	}
	fmt.Fprintln(ios.Out)
}

// printVerboseRequest dumps the outgoing call to stderr. Token already
// stripped from URL (we pass an absolute URL composed against BaseURL).
// Authorization will be added by the api.Client; we don't see it here
// so there's nothing to redact at this layer.
func printVerboseRequest(ios *iostreams.IOStreams, method, urlStr string, headers http.Header, body []byte) {
	fmt.Fprintf(ios.ErrOut, "> %s %s\n", method, urlStr)
	for k, vs := range headers {
		for _, v := range vs {
			fmt.Fprintf(ios.ErrOut, "> %s: %s\n", k, v)
		}
	}
	if len(body) > 0 {
		fmt.Fprintf(ios.ErrOut, ">\n> %s\n", bytes.TrimRight(body, "\n"))
	}
}

// printVerboseResponse mirrors printVerboseRequest for the response side.
func printVerboseResponse(ios *iostreams.IOStreams, status int, headers http.Header, body []byte) {
	fmt.Fprintf(ios.ErrOut, "< HTTP/1.1 %d\n", status)
	for k, vs := range headers {
		for _, v := range vs {
			if strings.EqualFold(k, "Authorization") {
				v = "[redacted]"
			}
			fmt.Fprintf(ios.ErrOut, "< %s: %s\n", k, v)
		}
	}
	if len(body) > 0 {
		fmt.Fprintf(ios.ErrOut, "<\n< %s\n", bytes.TrimRight(body, "\n"))
	}
}

// parseCacheTTL is a tiny wrapper that returns zero duration on empty
// input rather than erroring — validate() has already rejected invalid
// inputs, so the parse here is best-effort.
func parseCacheTTL(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	return time.ParseDuration(s)
}

// resolveLeading ensures the printed verbose URL has a leading slash so
// "user" and "/api/v1/user" render identically. The api.Client layer
// does the actual /api/v1 prefixing; this helper just makes the verbose
// echo cosmetically correct.
func resolveLeading(p string) string {
	if strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://") {
		return p
	}
	if !strings.HasPrefix(p, "/") {
		return "/api/v1/" + p
	}
	return p
}
