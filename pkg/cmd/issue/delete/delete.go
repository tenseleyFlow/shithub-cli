// SPDX-License-Identifier: AGPL-3.0-or-later

// Package delete implements `shithub issue delete`. Always requires
// confirmation — the user types the issue number — unless --yes bypasses.
package delete

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/issues"
	"github.com/tenseleyFlow/shithub-cli/internal/prompter"
	issueshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/issue/shared"
	repocmdshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

type options struct {
	IO          *iostreams.IOStreams
	Prompter    prompter.Prompter
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string
	GitRunner   git.Runner

	Arg      string
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
		Use:   "delete <number-or-url>",
		Short: "Delete an issue",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts.Arg = args[0]
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
	cmd.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "skip the name-typed confirmation prompt")
	return cmd
}

// Run executes the delete operation.
func Run(ctx context.Context, opts *options) error {
	ref, err := resolve(opts)
	if err != nil {
		return err
	}

	if !opts.Yes {
		if !opts.IO.IsStdoutTTY() {
			return fmt.Errorf("issue delete: confirmation required; pass --yes in non-interactive mode")
		}
		want := strconv.Itoa(ref.Number)
		typed, perr := opts.Prompter.Input(fmt.Sprintf("Type %q to confirm deletion of %s#%d", want, ref.Repo.FullName(), ref.Number), "")
		if perr != nil {
			return perr
		}
		if strings.TrimSpace(typed) != want {
			return fmt.Errorf("issue delete: confirmation mismatch; got %q", typed)
		}
	}

	client, err := opts.HTTPClient(ref.Repo.Host)
	if err != nil {
		return err
	}
	ic := issues.NewClient(client)
	if err := ic.Delete(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number); err != nil {
		return err
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Deleted %s#%d\n", opts.IO.SuccessIcon(), ref.Repo.FullName(), ref.Number)
	return nil
}

func resolve(opts *options) (issueshared.IssueRef, error) {
	rs := repocmdshared.Resolver{
		RepoFlag:    opts.Repo,
		Hostname:    opts.Hostname,
		DefaultHost: hostOrDefault(opts.DefaultHost),
		GitRunner:   opts.GitRunner,
	}
	fb, rerr := rs.Resolve()
	if rerr != nil && !strings.Contains(opts.Arg, "://") {
		return issueshared.IssueRef{}, rerr
	}
	if rerr != nil {
		fb = repocmdshared.RepoRef{}
	}
	ref, _, err := issueshared.ParseIssueArg(opts.Arg, fb)
	if err != nil {
		return issueshared.IssueRef{}, err
	}
	if ref.Repo.Host == "" {
		ref.Repo.Host = fb.Host
	}
	return ref, nil
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
