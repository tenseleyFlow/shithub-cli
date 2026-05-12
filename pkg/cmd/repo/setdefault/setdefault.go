// SPDX-License-Identifier: AGPL-3.0-or-later

// Package setdefault implements `shithub repo set-default`. Writes the
// resolved repo into the local `.git/config` so subsequent commands can
// pick it up without `-R`. Value format: `<host>:<owner>/<repo>` to keep
// multi-host setups disambiguated.
package setdefault

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

// ConfigKey is the .git/config key shithub-cli stores the default under.
// Keeping it under a `shithub.` namespace avoids collision with gh's
// `gh-resolved` and other tools' analogous keys.
const ConfigKey = "shithub.default-repo"

type options struct {
	IO          *iostreams.IOStreams
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string
	GitRunner   git.Runner

	RepoArg  string
	Hostname string
	Unset    bool
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
	}
	cmd := &cobra.Command{
		Use:   "set-default [<owner>/<repo>]",
		Short: "Set the default repository for the current directory",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.RepoArg = args[0]
			}
			if opts.GitRunner == nil {
				r, err := git.FromPath()
				if err != nil {
					return err
				}
				opts.GitRunner = r
			}
			return Run(c.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "the shithub host (default: configured host)")
	cmd.Flags().BoolVar(&opts.Unset, "unset", false, "remove the configured default")
	return cmd
}

// Run executes the set-default operation.
func Run(_ context.Context, opts *options) error {
	if opts.Unset {
		if err := git.UnsetConfig(opts.GitRunner, "", ConfigKey); err != nil {
			return err
		}
		fmt.Fprintf(opts.IO.ErrOut, "%s Unset %s\n", opts.IO.SuccessIcon(), ConfigKey)
		return nil
	}

	if opts.RepoArg == "" {
		return fmt.Errorf("repo set-default: positional <owner>/<repo> required (or pass --unset)")
	}
	ref, err := shared.ParseRepoArg(opts.RepoArg)
	if err != nil {
		return err
	}
	if ref.Host == "" {
		if opts.Hostname != "" {
			ref.Host = opts.Hostname
		} else if opts.DefaultHost != nil {
			ref.Host = opts.DefaultHost()
		}
	}
	value := ref.FullName()
	if ref.Host != "" {
		value = ref.Host + ":" + ref.FullName()
	}
	if err := git.SetConfig(opts.GitRunner, "", ConfigKey, value); err != nil {
		return err
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Set %s = %s\n", opts.IO.SuccessIcon(), ConfigKey, value)
	return nil
}
