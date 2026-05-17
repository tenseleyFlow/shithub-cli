// SPDX-License-Identifier: AGPL-3.0-or-later

// Package login implements `shithub auth login`. The default interactive
// flow is RFC 8628 OAuth device authorization — the CLI requests a
// short user code, opens the browser to the host's consent page, polls
// until the user approves, and persists the resulting PAT. `--with-token`
// preserves the headless stdin-paste path for CI / scripts.
package login

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/auth"
	"github.com/tenseleyFlow/shithub-cli/internal/auth/device"
	"github.com/tenseleyFlow/shithub-cli/internal/browser"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/prompter"
)

// blockedHosts can never be the target of `auth login`. shithub-cli is
// not a gh substitute; refusing github.com prevents accidental token
// misrouting if a user copy-pastes a command from gh's docs.
var blockedHosts = map[string]struct{}{
	"github.com":     {},
	"api.github.com": {},
}

// Options is the parameter object Run consumes. Kept in its own type so
// tests can compose options + run without touching cobra.
type Options struct {
	IO       *iostreams.IOStreams
	Prompter prompter.Prompter
	Hosts    func() (config.Hosts, error)
	Keyring  func() config.KeyringStore

	// Hostname is the --hostname flag. Empty defaults to config.DefaultHost.
	Hostname string
	// GitProtocol is the --git-protocol flag. Empty leaves the per-host setting unchanged.
	GitProtocol string
	// InsecureStorage forces persistence in hosts.yml (0600) rather than the keyring.
	InsecureStorage bool
	// WithToken reads from stdin instead of running the device flow.
	WithToken bool
	// Web explicitly opts in to the OAuth device flow. The flag exists
	// for muscle-memory parity with gh; when neither WithToken nor Web
	// is set the device flow is the default anyway.
	Web bool
	// Scopes is the comma- or space-separated scope set requested from
	// the server. Empty means "the server's default set." Only honored
	// on the device-flow path; --with-token tokens are minted on the
	// settings page with whatever scopes the user picked there.
	Scopes string

	// NewCandidateClient builds the *api.Client used to call /api/v1/user
	// for token validation. Production wires auth.NewCandidateClient;
	// tests inject a fakeapi-backed builder.
	NewCandidateClient func(host, token string) (*api.Client, error)
	// NewDeviceClient builds the device-flow client bound to host.
	// Tests inject a closure pointing at a httptest server; production
	// wires device.NewClient. Leaving this nil disables the device-flow
	// path so legacy tests that only exercise --with-token keep working.
	NewDeviceClient func(host string) (*device.Client, error)
	// OpenBrowser is the URL opener invoked during device flow. nil
	// falls back to internal/browser.Open.
	OpenBrowser func(url string) error
}

// NewCmd builds the cobra command and wires flags onto Options.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &Options{
		IO:                 f.IOStreams,
		Prompter:           f.Prompter,
		Hosts:              f.Hosts,
		Keyring:            f.Keyring,
		NewCandidateClient: auth.NewCandidateClient,
		NewDeviceClient: func(host string) (*device.Client, error) {
			return device.NewClient(device.Options{Host: host})
		},
		OpenBrowser: browser.Open,
	}
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate to a shithub host",
		Long: `Authenticate via OAuth device flow (default) or by piping a
personal access token through stdin (--with-token).

Device flow prints a short user code, opens the browser to the host's
consent page, polls until you approve, and stores the resulting token.

--with-token reads a pre-minted PAT from stdin — useful in CI:
    shithub auth login --with-token < ~/.shithub-token

Tokens are stored in your system keyring by default. Pass --insecure-storage
to write to ~/.config/shithub/hosts.yml (0600 perms) instead.`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return Run(c.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "the shithub host to authenticate with")
	cmd.Flags().StringVar(&opts.GitProtocol, "git-protocol", "", "default git protocol for this host: ssh or https")
	cmd.Flags().BoolVar(&opts.InsecureStorage, "insecure-storage", false, "store the token in hosts.yml (0600) instead of the system keyring")
	cmd.Flags().BoolVar(&opts.WithToken, "with-token", false, "read the token from stdin (e.g. shithub auth login --with-token < token.txt)")
	cmd.Flags().BoolVar(&opts.Web, "web", false, "explicitly request the OAuth device-flow path (alias for the default)")
	cmd.Flags().StringVarP(&opts.Scopes, "scopes", "s", "", "comma- or space-separated OAuth scopes to request (device flow only)")
	return cmd
}

// Run executes the login flow. Exposed for tests.
func Run(ctx context.Context, opts *Options) error {
	host := config.DefaultHost
	if opts.Hostname != "" {
		h, err := config.ValidateHost(opts.Hostname)
		if err != nil {
			return err
		}
		host = h
	}
	if _, blocked := blockedHosts[host]; blocked {
		return fmt.Errorf("auth: %s is not a shithub host; use the gh CLI for GitHub", host)
	}
	if opts.GitProtocol != "" && opts.GitProtocol != config.GitProtocolSSH && opts.GitProtocol != config.GitProtocolHTTPS {
		return fmt.Errorf("auth: --git-protocol must be 'ssh' or 'https', got %q", opts.GitProtocol)
	}
	if opts.WithToken && opts.Web {
		return errors.New("auth: --with-token and --web are mutually exclusive")
	}
	if opts.WithToken && opts.Scopes != "" {
		return errors.New("auth: --scopes is a device-flow option; remove it or drop --with-token")
	}

	if opts.WithToken {
		return runWithToken(ctx, opts, host)
	}
	return runDeviceFlow(ctx, opts, host)
}

// runWithToken handles the headless stdin-paste path.
func runWithToken(ctx context.Context, opts *Options, host string) error {
	data, err := io.ReadAll(opts.IO.In)
	if err != nil {
		return fmt.Errorf("auth: read token from stdin: %w", err)
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return errors.New("auth: --with-token received empty input on stdin")
	}
	if hint := tokenShapeHint(token); hint != "" {
		fmt.Fprintln(opts.IO.ErrOut, opts.IO.WarningIcon()+" "+hint)
	}

	client, err := opts.NewCandidateClient(host, token)
	if err != nil {
		return err
	}
	result, err := auth.Validate(ctx, client)
	if err != nil {
		return fmt.Errorf("auth: token rejected by server (check that the PAT is current and unrevoked): %w", err)
	}
	if err := persist(opts, host, result.User, token, result.Scopes); err != nil {
		return err
	}
	printSuccess(opts.IO, host, result.User.Login, result.Scopes, storageDestination(opts))
	return nil
}

// runDeviceFlow runs the RFC 8628 device authorization grant against
// host. The post-Exchange validate-and-persist tail mirrors the
// --with-token path so token storage is the single code path.
func runDeviceFlow(ctx context.Context, opts *Options, host string) error {
	if opts.NewDeviceClient == nil {
		return errors.New("auth: device-flow client unavailable; rebuild the CLI or use --with-token")
	}
	if opts.IO.NeverPrompt() {
		return errors.New("auth: device flow needs an interactive terminal; use --with-token in non-interactive contexts")
	}
	if v := strings.TrimSpace(os.Getenv("CI")); v != "" && v != "false" && v != "0" {
		return errors.New("auth: refusing device flow under CI=" + v + "; use --with-token <file> in CI environments")
	}

	devClient, err := opts.NewDeviceClient(host)
	if err != nil {
		return fmt.Errorf("auth: build device client: %w", err)
	}

	code, err := devClient.RequestCode(ctx, opts.Scopes)
	if err != nil {
		return mapDeviceErr("request device code", err)
	}

	// Open the bare verification_uri rather than the pre-filled
	// verification_uri_complete: forcing the user to transcribe the
	// short code closes the phishing-redirect window where a malicious
	// link pre-fills an attacker's code and the user clicks Approve
	// without reading. The consent page still validates the code, so
	// the only cost here is ~5 seconds of typing.
	verifyURL := code.VerificationURI
	if err := devClient.ValidateVerificationURI(verifyURL); err != nil {
		return fmt.Errorf("auth: %w", err)
	}
	printDeviceCode(opts.IO, code.UserCode, verifyURL)

	openBrowser := opts.OpenBrowser
	if openBrowser == nil {
		openBrowser = browser.Open
	}
	consented, err := opts.Prompter.Confirm("Press Enter to open the browser", true)
	if err != nil {
		return fmt.Errorf("auth: confirm browser open: %w", err)
	}
	if consented {
		if err := openBrowser(verifyURL); err != nil {
			fmt.Fprintf(opts.IO.ErrOut, "%s couldn't open browser (%v); open the URL above manually.\n",
				opts.IO.WarningIcon(), err)
		}
	} else {
		fmt.Fprintln(opts.IO.ErrOut, "  Open the URL above in your browser to approve the request.")
	}

	fmt.Fprintln(opts.IO.ErrOut, "  Waiting for approval...")
	tok, err := devClient.Poll(ctx, code)
	if err != nil {
		return mapDeviceErr("poll device token", err)
	}

	// Validate the freshly minted token so we know its owner before
	// persisting — the device flow's token response doesn't carry user
	// identity, and we need entry.User to be correct for git auth.
	client, err := opts.NewCandidateClient(host, tok.AccessToken)
	if err != nil {
		return err
	}
	result, err := auth.Validate(ctx, client)
	if err != nil {
		return fmt.Errorf("auth: validate minted token: %w", err)
	}

	scopes := result.Scopes
	if len(scopes) == 0 {
		scopes = tok.Scopes() // fall back to the token-endpoint scopes
	}

	if err := persist(opts, host, result.User, tok.AccessToken, scopes); err != nil {
		return err
	}
	printSuccess(opts.IO, host, result.User.Login, scopes, storageDestination(opts))
	return nil
}

// printDeviceCode renders the one-time user code + verification URL.
// The "type this code" framing is deliberate — opening a pre-filled
// URL is the device-flow phishing footgun, so we surface the code
// prominently and frame the next step as a transcription.
func printDeviceCode(ios *iostreams.IOStreams, userCode, verifyURL string) {
	w := ios.ErrOut
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  Authorization required. Type this code in your browser:")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "      "+userCode)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  URL: "+verifyURL)
	fmt.Fprintln(w)
}

// mapDeviceErr surfaces the RFC 8628 sentinel errors with a hint
// tailored to the user's likely next action.
func mapDeviceErr(stage string, err error) error {
	switch {
	case errors.Is(err, device.ErrAccessDenied):
		return errors.New("auth: the request was denied in the browser")
	case errors.Is(err, device.ErrExpiredToken):
		return errors.New("auth: the user code expired before approval; re-run `shithub auth login`")
	case errors.Is(err, device.ErrUnauthorizedClient):
		return errors.New("auth: this build's client_id is not allowlisted on the host; rebuild with SHITHUB_OAUTH_CLIENT_ID set or contact the admin")
	case errors.Is(err, device.ErrInvalidScope):
		return errors.New("auth: one or more requested scopes were rejected; check --scopes")
	default:
		return fmt.Errorf("auth: %s: %w", stage, err)
	}
}

// persist writes the token to keyring or hosts.yml, updates HostEntry
// metadata, and saves Hosts.
func persist(opts *Options, host string, user api.User, token string, scopes []string) error {
	hosts, err := opts.Hosts()
	if err != nil {
		return err
	}
	entry := hosts.Get(host)
	entry.User = user.Login
	entry.LastScopes = scopes
	if opts.GitProtocol != "" {
		entry.GitProtocol = opts.GitProtocol
	}
	// Mark default if this is the only host, or no host has default yet.
	if len(hosts) == 1 || !anyDefault(hosts) {
		_ = hosts.SetDefault(host)
	}

	ks := opts.Keyring()
	useKeyring := !opts.InsecureStorage && ks != nil && config.KeyringAvailable(ks)

	if useKeyring {
		if err := config.SetToken(ks, host, user.Login, token); err != nil {
			return fmt.Errorf("auth: keyring write failed: %w", err)
		}
		entry.OAuthToken = ""
		entry.InsecureStorage = false
	} else {
		if !opts.InsecureStorage {
			fmt.Fprintln(opts.IO.ErrOut, opts.IO.WarningIcon()+" keyring unavailable; storing token in hosts.yml (0600)")
		}
		entry.OAuthToken = token
		entry.InsecureStorage = true
	}

	return hosts.Save()
}

// shithubPATPrefix is the expected leading marker for a v1 personal
// access token issued by shithub server. Future token types (OAuth
// access tokens from C04a's device-flow, fine-grained PATs) may use
// different prefixes — this is a hint, not a hard contract.
const shithubPATPrefix = "shithub_pat_"

// tokenShapeHint returns a short stderr-suitable warning when the
// pasted token doesn't look like a shithub PAT — typically the user
// mistakenly pasted a GitHub token (gh_pat_/ghp_/ghu_), or only the
// suffix without the prefix. Empty return means "the token is plausibly
// a shithub PAT; suppress the warning." We never reject here — OAuth
// tokens (C04a) and future PAT variants may legitimately differ, and
// the wire validation (auth.Validate) is the authoritative gate.
func tokenShapeHint(token string) string {
	switch {
	case strings.HasPrefix(token, shithubPATPrefix):
		return ""
	case strings.HasPrefix(token, "gh_pat_"), strings.HasPrefix(token, "ghp_"),
		strings.HasPrefix(token, "ghu_"), strings.HasPrefix(token, "ghs_"):
		return "this looks like a GitHub token; shithub PATs start with " + shithubPATPrefix
	default:
		return "token doesn't start with " + shithubPATPrefix +
			"; continuing anyway (the server will reject if it's wrong)"
	}
}

// anyDefault reports whether any host carries Default=true.
func anyDefault(h config.Hosts) bool {
	for _, e := range h {
		if e != nil && e.Default {
			return true
		}
	}
	return false
}

// storageDestination returns a human-readable label for the success line.
func storageDestination(opts *Options) string {
	if opts.InsecureStorage {
		return "hosts.yml (insecure)"
	}
	ks := opts.Keyring()
	if ks == nil || !config.KeyringAvailable(ks) {
		return "hosts.yml (keyring unavailable)"
	}
	return "system keyring"
}

// printSuccess emits the post-login summary on ErrOut so scripts piping
// other commands don't capture it as data.
func printSuccess(ios *iostreams.IOStreams, host, username string, scopes []string, dest string) {
	fmt.Fprintf(ios.ErrOut, "%s Authenticated to %s as %s\n", ios.SuccessIcon(), host, username)
	fmt.Fprintf(ios.ErrOut, "  Token stored in: %s\n", dest)
	if len(scopes) > 0 {
		fmt.Fprintf(ios.ErrOut, "  Token scopes:    %s\n", strings.Join(scopes, ", "))
	} else {
		fmt.Fprintf(ios.ErrOut, "  Token scopes:    (server did not advertise)\n")
	}
}
