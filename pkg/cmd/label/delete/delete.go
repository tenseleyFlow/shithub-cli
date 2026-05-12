// SPDX-License-Identifier: AGPL-3.0-or-later

// Package delete implements `shithub label delete`. Confirms unless
// --yes is set.
package delete

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/labels"
	"github.com/tenseleyFlow/shithub-cli/internal/prompter"
	repocmdshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

type options struct {
	IO          *iostreams.IOStreams
	Prompter    prompter.Prompter
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string
	GitRunner   git.Runner

	Name     string
	Repo     string
	Hostname string
	Yes      bool
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		Prompter:    f.Prompter,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
	}
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a label",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts.Name = args[0]
			if opts.GitRunner == nil {
				if r, err := git.FromPath(); err == nil {
					opts.GitRunner = r
				}
			}
			return Run(c.Context(), opts)
		},
	}
	cmd.Flags().StringVarP(&opts.Repo, "repo", "R", "", "select another repository using the [HOST/]OWNER/REPO format")
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "the shithub host (default: configured host)")
	cmd.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}

// Run executes the delete.
func Run(ctx context.Context, opts *options) error {
	resolver := repocmdshared.Resolver{
		RepoFlag:    opts.Repo,
		Hostname:    opts.Hostname,
		DefaultHost: hostOrDefault(opts.DefaultHost),
		GitRunner:   opts.GitRunner,
	}
	ref, err := resolver.Resolve()
	if err != nil {
		return err
	}

	if !opts.Yes {
		if !opts.IO.IsStdoutTTY() {
			return errors.New("label delete: confirmation required; pass --yes in non-interactive mode")
		}
		typed, perr := opts.Prompter.Input(fmt.Sprintf("Type %q to confirm deletion", opts.Name), "")
		if perr != nil {
			return perr
		}
		if strings.TrimSpace(typed) != opts.Name {
			return fmt.Errorf("label delete: confirmation mismatch; got %q", typed)
		}
	}

	client, err := opts.HTTPClient(ref.Host)
	if err != nil {
		return err
	}
	lc := labels.NewClient(client)
	if err := lc.Delete(ctx, ref.Owner, ref.Name, opts.Name); err != nil {
		return err
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Deleted label %s\n", opts.IO.SuccessIcon(), opts.Name)
	return nil
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
