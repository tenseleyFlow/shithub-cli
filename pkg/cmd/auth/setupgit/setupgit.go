// SPDX-License-Identifier: AGPL-3.0-or-later

// Package setupgit implements `shithub auth setup-git`. It writes a
// `credential.<scheme://host>.helper` config entry pointing git at our
// hidden git-credential subcommand, so HTTPS clone/push pick up the
// authenticated token without a manual .netrc.
package setupgit

import (
	"context"
	"errors"
	"fmt"
	"os/exec"

	"github.com/cli/safeexec"
	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
)

// Options drives Run.
type Options struct {
	IO    *iostreams.IOStreams
	Hosts func() (config.Hosts, error)

	Hostname string
	Force    bool

	// GitRunner is the indirection that lets tests intercept the actual
	// `git config` invocation without spawning a process.
	GitRunner func(args ...string) error
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &Options{
		IO:    f.IOStreams,
		Hosts: f.Hosts,
	}
	cmd := &cobra.Command{
		Use:   "setup-git",
		Short: "Configure git to use shithub as a credential helper",
		Long: `Write a 'credential.https://<host>.helper' entry into your global
git config so HTTPS clone/push against the configured host(s) authenticate
via the token stored by 'shithub auth login'.

Without --hostname, every authenticated host receives a helper line.`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if opts.GitRunner == nil {
				opts.GitRunner = realGitRunner
			}
			return Run(c.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "configure only this host (default: every authenticated host)")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "overwrite an existing credential helper for the host(s)")
	return cmd
}

// Run installs the helper config.
func Run(_ context.Context, opts *Options) error {
	hosts, err := opts.Hosts()
	if err != nil {
		return err
	}
	if len(hosts) == 0 {
		return errors.New("setup-git: no authenticated hosts — run `shithub auth login` first")
	}

	var target string
	if opts.Hostname != "" {
		h, err := config.ValidateHost(opts.Hostname)
		if err != nil {
			return fmt.Errorf("setup-git: %w", err)
		}
		target = h
		if _, ok := hosts[target]; !ok {
			return fmt.Errorf("setup-git: not authenticated to %s", target)
		}
	}

	for host := range hosts {
		if target != "" && host != target {
			continue
		}
		if err := configureHost(opts, host); err != nil {
			return err
		}
	}
	return nil
}

// configureHost emits a single host's helper line.
func configureHost(opts *Options, host string) error {
	cfgKey := fmt.Sprintf("credential.https://%s.helper", host)
	const helperCmd = "!shithub auth git-credential"

	if !opts.Force {
		// Wipe any pre-existing values so subsequent --add lands clean.
		// `git config --global --unset-all` returns 5 when the key is
		// absent, which is fine — we ignore non-zero here.
		_ = opts.GitRunner("config", "--global", "--unset-all", cfgKey)
	} else {
		// Force mode: replace outright.
		_ = opts.GitRunner("config", "--global", "--unset-all", cfgKey)
	}
	// gh writes two lines: an empty value first (clears chain) then ours.
	// Match the convention so coexistence with gh's helper is predictable.
	if err := opts.GitRunner("config", "--global", "--replace-all", cfgKey, ""); err != nil {
		return fmt.Errorf("setup-git: clear %s: %w", cfgKey, err)
	}
	if err := opts.GitRunner("config", "--global", "--add", cfgKey, helperCmd); err != nil {
		return fmt.Errorf("setup-git: add %s: %w", cfgKey, err)
	}

	fmt.Fprintf(opts.IO.ErrOut, "%s git configured to use shithub as credential helper for %s\n",
		opts.IO.SuccessIcon(), host)
	return nil
}

// realGitRunner shells out to git via safeexec.LookPath (avoiding $PATH
// surprises). Errors include the full command and exit code for easier
// debugging.
func realGitRunner(args ...string) error {
	bin, err := safeexec.LookPath("git")
	if err != nil {
		return fmt.Errorf("setup-git: locate git: %w", err)
	}
	cmd := exec.Command(bin, args...) //nolint:gosec // args from trusted callers only
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %v: %w (%s)", args, err, string(out))
	}
	return nil
}
