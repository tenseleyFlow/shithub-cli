// SPDX-License-Identifier: AGPL-3.0-or-later

// Package status implements `shithub auth status`. Reports one row per
// configured host: token storage, username, scopes, git protocol, server
// URL. Network-touching only for the live /api/v1/user roundtrip used to
// confirm the token is still valid and to read X-OAuth-Scopes (which may
// have changed since last login).
package status

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/auth"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
)

// HostStatus is the JSON shape emitted by --json. Field names are stable.
type HostStatus struct {
	Host        string   `json:"host"`
	User        string   `json:"user"`
	TokenSource string   `json:"token_source"`
	GitProtocol string   `json:"git_protocol"`
	Scopes      []string `json:"scopes"`
	Active      bool     `json:"active"`
	Reachable   bool     `json:"reachable"`
	Error       string   `json:"error,omitempty"`
}

// Options drives Run.
type Options struct {
	IO         *iostreams.IOStreams
	Hosts      func() (config.Hosts, error)
	Keyring    func() config.KeyringStore
	HTTPClient func(host string) (*api.Client, error)

	Hostname  string
	ShowToken bool
	JSON      bool
	Active    bool
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &Options{
		IO:         f.IOStreams,
		Hosts:      f.Hosts,
		Keyring:    f.Keyring,
		HTTPClient: f.HTTPClient,
	}
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show authentication status for each configured shithub host",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return Run(c.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "filter to a single host")
	cmd.Flags().BoolVar(&opts.ShowToken, "show-token", false, "include the stored token in the output (refused on a TTY without confirmation)")
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "emit machine-readable JSON")
	cmd.Flags().BoolVar(&opts.Active, "active", false, "exit 0 if any host has a valid token, non-zero otherwise (no per-host output)")
	return cmd
}

// Run resolves each host's status and renders it. The --active mode is
// silent-by-design (script friendly): only the exit code carries signal.
func Run(ctx context.Context, opts *Options) error {
	hosts, err := opts.Hosts()
	if err != nil {
		return err
	}

	if opts.ShowToken && opts.IO.IsStdoutTTY() && !opts.JSON {
		return errors.New("auth: --show-token in a terminal would leak the token; re-run with output piped or use --json")
	}

	rows := collect(ctx, opts, hosts)

	if opts.Active {
		for _, r := range rows {
			if r.Reachable {
				return nil
			}
		}
		return fmt.Errorf("auth: no host has a reachable, validated token")
	}

	if opts.JSON {
		return writeJSON(opts.IO, rows, opts.ShowToken, opts.Keyring(), hosts)
	}
	return writeHuman(opts.IO, rows, opts.ShowToken, opts.Keyring(), hosts)
}

// collect builds a HostStatus per filtered host, in deterministic order.
func collect(ctx context.Context, opts *Options, hosts config.Hosts) []HostStatus {
	var keys []string
	for h := range hosts {
		if opts.Hostname != "" && config.NormalizeHost(opts.Hostname) != h {
			continue
		}
		keys = append(keys, h)
	}
	sort.Strings(keys)

	out := make([]HostStatus, 0, len(keys))
	for _, host := range keys {
		entry := hosts[host]
		row := HostStatus{
			Host:        host,
			User:        entry.User,
			GitProtocol: entry.GitProtocol,
			Active:      entry.Default,
		}
		_, src, err := config.ResolveToken(opts.Keyring(), hosts, hosts.DefaultHostName(), host)
		if err != nil {
			row.Error = err.Error()
			out = append(out, row)
			continue
		}
		row.TokenSource = src.String()

		client, err := opts.HTTPClient(host)
		if err != nil {
			row.Error = err.Error()
			out = append(out, row)
			continue
		}
		result, err := auth.Validate(ctx, client)
		if err != nil {
			row.Error = err.Error()
			out = append(out, row)
			continue
		}
		row.User = result.User.Username
		row.Scopes = result.Scopes
		row.Reachable = true
		// Refresh cached scopes for the user's hosts.yml entry — best effort.
		entry.LastScopes = result.Scopes
		out = append(out, row)
	}
	return out
}

// writeHuman renders the table-of-hosts shape gh users expect.
func writeHuman(ios *iostreams.IOStreams, rows []HostStatus, showToken bool, ks config.KeyringStore, hosts config.Hosts) error {
	if len(rows) == 0 {
		fmt.Fprintln(ios.ErrOut, "auth: no hosts configured. Run `shithub auth login` to add one.")
		return nil
	}
	for i, r := range rows {
		if i > 0 {
			fmt.Fprintln(ios.Out)
		}
		marker := " "
		if r.Active {
			marker = "*"
		}
		fmt.Fprintf(ios.Out, "%s %s\n", marker, r.Host)
		if r.Error != "" {
			fmt.Fprintf(ios.Out, "    %s %s\n", ios.FailureIcon(), r.Error)
			continue
		}
		fmt.Fprintf(ios.Out, "    %s logged in as %s\n", ios.SuccessIcon(), r.User)
		fmt.Fprintf(ios.Out, "    Token source: %s\n", r.TokenSource)
		if r.GitProtocol != "" {
			fmt.Fprintf(ios.Out, "    Git protocol: %s\n", r.GitProtocol)
		}
		if len(r.Scopes) > 0 {
			fmt.Fprintf(ios.Out, "    Scopes:       %s\n", strings.Join(r.Scopes, ", "))
		}
		if showToken {
			token := lookupToken(ks, hosts, r.Host)
			if token != "" {
				fmt.Fprintf(ios.Out, "    Token:        %s\n", token)
			}
		}
	}
	return nil
}

// writeJSON emits the rows as JSON. Tokens go in the payload only when
// --show-token is set (the TTY-guard up in Run still applies).
func writeJSON(ios *iostreams.IOStreams, rows []HostStatus, showToken bool, ks config.KeyringStore, hosts config.Hosts) error {
	if showToken {
		// Embed tokens directly under a separate field per row; do not
		// reuse the slim struct so callers without --show-token get a
		// stable schema that never includes tokens.
		type withTok struct {
			HostStatus
			Token string `json:"token,omitempty"`
		}
		enriched := make([]withTok, len(rows))
		for i, r := range rows {
			enriched[i] = withTok{HostStatus: r, Token: lookupToken(ks, hosts, r.Host)}
		}
		enc := json.NewEncoder(ios.Out)
		enc.SetIndent("", "  ")
		return enc.Encode(enriched)
	}
	enc := json.NewEncoder(ios.Out)
	enc.SetIndent("", "  ")
	return enc.Encode(rows)
}

// lookupToken returns the bearer for host or "" if unavailable. Error
// modes (missing entry, keyring failure) deliberately collapse to "" so
// status output stays a best-effort report rather than a hard failure.
func lookupToken(ks config.KeyringStore, hosts config.Hosts, host string) string {
	token, _, err := config.ResolveToken(ks, hosts, hosts.DefaultHostName(), host)
	if err != nil {
		return ""
	}
	return token
}
