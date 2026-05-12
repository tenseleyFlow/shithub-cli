// SPDX-License-Identifier: AGPL-3.0-or-later

// Package login implements `shithub auth login`. The v1 flow is token-
// paste only: either via --with-token (stdin) or via an interactive
// prompt that points the user at shithub.sh/settings/tokens. OAuth
// device flow lands in C04a once shithub server-side support exists.
package login

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/auth"
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
	// WithToken reads from stdin instead of prompting interactively.
	WithToken bool

	// NewCandidateClient builds the *api.Client used to call /api/v1/user
	// for token validation. Production wires auth.NewCandidateClient;
	// tests inject a fakeapi-backed builder.
	NewCandidateClient func(host, token string) (*api.Client, error)
}

// NewCmd builds the cobra command and wires flags onto Options.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &Options{
		IO:                 f.IOStreams,
		Prompter:           f.Prompter,
		Hosts:              f.Hosts,
		Keyring:            f.Keyring,
		NewCandidateClient: auth.NewCandidateClient,
	}
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate to a shithub host",
		Long: `Authenticate by pasting (or piping) a personal access token (PAT).

Mint a PAT at https://<host>/settings/tokens with the scopes you need
(typically: repo:read, repo:write, user:read).

Tokens are stored in your system keyring by default. Pass --insecure-storage
to write to ~/.config/shithub/hosts.yml (0600 perms) instead — useful on
headless servers without a secret-service daemon.

OAuth device flow (--web) ships in a follow-up sprint once shithub server
support lands.`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return Run(c.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "the shithub host to authenticate with")
	cmd.Flags().StringVar(&opts.GitProtocol, "git-protocol", "", "default git protocol for this host: ssh or https")
	cmd.Flags().BoolVar(&opts.InsecureStorage, "insecure-storage", false, "store the token in hosts.yml (0600) instead of the system keyring")
	cmd.Flags().BoolVar(&opts.WithToken, "with-token", false, "read the token from stdin (e.g. shithub auth login --with-token < token.txt)")
	return cmd
}

// Run executes the login flow. Exposed for tests.
func Run(ctx context.Context, opts *Options) error {
	host := config.NormalizeHost(opts.Hostname)
	if host == "" {
		host = config.DefaultHost
	}
	if _, blocked := blockedHosts[host]; blocked {
		return fmt.Errorf("auth: %s is not a shithub host; use the gh CLI for GitHub", host)
	}
	if opts.GitProtocol != "" && opts.GitProtocol != config.GitProtocolSSH && opts.GitProtocol != config.GitProtocolHTTPS {
		return fmt.Errorf("auth: --git-protocol must be 'ssh' or 'https', got %q", opts.GitProtocol)
	}

	token, err := readToken(opts, host)
	if err != nil {
		return err
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

// readToken returns the candidate token, sourced from stdin (--with-token)
// or an interactive password prompt.
func readToken(opts *Options, host string) (string, error) {
	if opts.WithToken {
		data, err := io.ReadAll(opts.IO.In)
		if err != nil {
			return "", fmt.Errorf("auth: read token from stdin: %w", err)
		}
		token := strings.TrimSpace(string(data))
		if token == "" {
			return "", errors.New("auth: --with-token received empty input on stdin")
		}
		return token, nil
	}

	if opts.IO.NeverPrompt() {
		return "", errors.New("auth: interactive prompt unavailable; use --with-token")
	}
	fmt.Fprintf(opts.IO.ErrOut, "Mint a personal access token at:\n  https://%s/settings/tokens\n\n", host)
	token, err := opts.Prompter.Password("Paste your token:")
	if err != nil {
		return "", err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", errors.New("auth: empty token")
	}
	return token, nil
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
		fmt.Fprintf(ios.ErrOut, "  Token scopes:    (server did not advertise; see shithub S50 §1)\n")
	}
}
