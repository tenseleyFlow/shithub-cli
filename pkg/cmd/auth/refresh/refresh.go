// SPDX-License-Identifier: AGPL-3.0-or-later

// Package refresh implements `shithub auth refresh`. The command re-runs
// the OAuth device flow against a host the user is already logged in
// to, replacing the stored token with a fresh one. Use cases: expanding
// the scope set, rotating a token suspected of leakage, or recovering
// from a server-side revocation.
package refresh

import (
	"context"
	"errors"
	"fmt"
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

// Options is the parameter object Run consumes.
type Options struct {
	IO       *iostreams.IOStreams
	Prompter prompter.Prompter
	Hosts    func() (config.Hosts, error)
	Keyring  func() config.KeyringStore

	// Hostname is the --hostname flag. Empty resolves to the single
	// logged-in host (or errors when zero / multiple are configured
	// without an explicit pick).
	Hostname string
	// Scopes is comma- or space-separated. Empty replays the host's
	// LastScopes.
	Scopes string
	// InsecureStorage forces hosts.yml persistence regardless of the
	// existing entry's keyring status. Rarely set; the default is to
	// preserve whatever storage the prior login chose.
	InsecureStorage bool

	NewCandidateClient func(host, token string) (*api.Client, error)
	NewDeviceClient    func(host string) (*device.Client, error)
	OpenBrowser        func(url string) error
}

// NewCmd builds the cobra command.
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
		Use:   "refresh",
		Short: "Re-authenticate to rotate or expand scopes on the active host",
		Long: `Re-run the OAuth device flow against a host you're already logged
in to, replacing the stored token with a fresh one.

  shithub auth refresh                        # rotate the active host's token
  shithub auth refresh --scopes repo:write    # expand scopes
  shithub auth refresh --hostname shithub.sh  # pick a host explicitly

After a successful refresh, the previous token is revoked server-side;
any concurrent shithub commands using the old token will need to retry.`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return Run(c.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "the shithub host to refresh")
	cmd.Flags().StringVarP(&opts.Scopes, "scopes", "s", "", "comma- or space-separated OAuth scopes to request")
	cmd.Flags().BoolVar(&opts.InsecureStorage, "insecure-storage", false, "force hosts.yml storage for the refreshed token")
	return cmd
}

// Run executes the refresh flow.
func Run(ctx context.Context, opts *Options) error {
	if opts.NewDeviceClient == nil {
		return errors.New("auth: device-flow client unavailable in this build")
	}
	if opts.IO.NeverPrompt() {
		return errors.New("auth: refresh needs an interactive terminal")
	}

	hosts, err := opts.Hosts()
	if err != nil {
		return err
	}
	host, err := resolveHost(opts.Hostname, hosts)
	if err != nil {
		return err
	}
	entry, ok := hosts[config.NormalizeHost(host)]
	if !ok || entry == nil || entry.User == "" {
		return fmt.Errorf("auth: %s has no existing login; run `shithub auth login --hostname %s`", host, host)
	}

	scopes := strings.TrimSpace(opts.Scopes)
	if scopes == "" {
		scopes = strings.Join(entry.LastScopes, " ")
	}

	devClient, err := opts.NewDeviceClient(host)
	if err != nil {
		return fmt.Errorf("auth: build device client: %w", err)
	}
	code, err := devClient.RequestCode(ctx, scopes)
	if err != nil {
		return mapDeviceErr("request device code", err)
	}

	verifyURL := code.VerificationURIComplete
	if verifyURL == "" {
		verifyURL = code.VerificationURI
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

	client, err := opts.NewCandidateClient(host, tok.AccessToken)
	if err != nil {
		return err
	}
	result, err := auth.Validate(ctx, client)
	if err != nil {
		return fmt.Errorf("auth: validate refreshed token: %w", err)
	}
	if entry.User != "" && result.User.Login != entry.User {
		return fmt.Errorf("auth: refresh returned a token for user %q but %s was logged in as %q",
			result.User.Login, host, entry.User)
	}

	newScopes := result.Scopes
	if len(newScopes) == 0 {
		newScopes = tok.Scopes()
	}

	if err := persist(opts, hosts, host, entry, result.User.Login, tok.AccessToken, newScopes); err != nil {
		return err
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Refreshed credentials for %s as %s\n",
		opts.IO.SuccessIcon(), host, result.User.Login)
	if len(newScopes) > 0 {
		fmt.Fprintf(opts.IO.ErrOut, "  Token scopes: %s\n", strings.Join(newScopes, ", "))
	}
	return nil
}

// resolveHost returns the target host. Explicit --hostname always wins;
// otherwise pick the lone configured host (most CLIs only ever touch
// one), or the host marked Default=true. Anything ambiguous errors.
func resolveHost(flag string, hosts config.Hosts) (string, error) {
	if flag != "" {
		return config.ValidateHost(flag)
	}
	if len(hosts) == 1 {
		for h := range hosts {
			return h, nil
		}
	}
	for h, entry := range hosts {
		if entry != nil && entry.Default {
			return h, nil
		}
	}
	if len(hosts) == 0 {
		return "", errors.New("auth: no hosts configured; run `shithub auth login` first")
	}
	return "", errors.New("auth: multiple hosts configured; pass --hostname to pick one")
}

// printDeviceCode mirrors login.printDeviceCode so users see the same
// rendering across both commands.
func printDeviceCode(ios *iostreams.IOStreams, userCode, verifyURL string) {
	w := ios.ErrOut
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  Authorization required.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  Visit: "+verifyURL)
	fmt.Fprintln(w, "  Code:  "+userCode)
	fmt.Fprintln(w)
}

// mapDeviceErr surfaces the protocol sentinels with a refresh-flavoured hint.
func mapDeviceErr(stage string, err error) error {
	switch {
	case errors.Is(err, device.ErrAccessDenied):
		return errors.New("auth: refresh was denied in the browser")
	case errors.Is(err, device.ErrExpiredToken):
		return errors.New("auth: the refresh code expired before approval; re-run `shithub auth refresh`")
	case errors.Is(err, device.ErrUnauthorizedClient):
		return errors.New("auth: this build's client_id is not allowlisted on the host")
	case errors.Is(err, device.ErrInvalidScope):
		return errors.New("auth: one or more requested scopes were rejected; check --scopes")
	default:
		return fmt.Errorf("auth: %s: %w", stage, err)
	}
}

// persist updates the host entry with the refreshed token + scopes. The
// storage choice (keyring vs hosts.yml) sticks with whatever the prior
// login established unless --insecure-storage overrides. The hosts map
// is mutated in place and Save()'d here — callers MUST pass the same
// hosts instance they used to resolve the entry, not a fresh load.
func persist(opts *Options, hosts config.Hosts, host string, entry *config.HostEntry, user, token string, scopes []string) error {
	entry.User = user
	entry.LastScopes = scopes

	forceInsecure := opts.InsecureStorage || entry.InsecureStorage
	ks := opts.Keyring()
	useKeyring := !forceInsecure && ks != nil && config.KeyringAvailable(ks)

	if useKeyring {
		if err := config.SetToken(ks, host, user, token); err != nil {
			return fmt.Errorf("auth: keyring write failed: %w", err)
		}
		entry.OAuthToken = ""
		entry.InsecureStorage = false
	} else {
		entry.OAuthToken = token
		entry.InsecureStorage = true
	}
	return hosts.Save()
}
