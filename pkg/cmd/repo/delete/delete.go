// SPDX-License-Identifier: AGPL-3.0-or-later

// Package delete implements `shithub repo delete`. Hard-deletes a repo.
// Always confirms via name-typing unless --yes is passed, matching gh's
// safety behavior on a destructive op.
package delete

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/prompter"
	"github.com/tenseleyFlow/shithub-cli/internal/repos"
	"github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

type options struct {
	IO          *iostreams.IOStreams
	Prompter    prompter.Prompter
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string
	GitRunner   git.Runner

	RepoArg  string
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
		Use:   "delete [<owner>/<repo>]",
		Short: "Delete a repository",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.RepoArg = args[0]
			}
			if opts.GitRunner == nil {
				if r, err := git.FromPath(); err == nil {
					opts.GitRunner = r
				}
			}
			return Run(c.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "the shithub host (default: configured host)")
	cmd.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "skip the confirmation prompt (still requires interactive name-typing without this)")
	return cmd
}

// Run executes the delete operation.
func Run(ctx context.Context, opts *options) error {
	// G12 (F38): `repo create cli-audit-rt` accepts an unqualified name
	// and infers owner=current-user; `repo delete cli-audit-rt` used to
	// error "expected owner/name". Symmetrize the rule for the identity-
	// space delete verb.
	repoArg := opts.RepoArg
	if repoArg != "" && !strings.ContainsRune(repoArg, '/') {
		host := opts.Hostname
		if host == "" {
			host = hostOrDefault(opts.DefaultHost)
		}
		client, err := opts.HTTPClient(host)
		if err != nil {
			return err
		}
		u, err := client.CurrentUser(ctx)
		if err != nil {
			return fmt.Errorf("repo delete: resolve current user to qualify %q: %w", repoArg, err)
		}
		if u == nil || u.Login == "" {
			return fmt.Errorf("repo delete: current user has no login")
		}
		repoArg = u.Login + "/" + repoArg
	}

	resolver := shared.Resolver{
		RepoFlag:    repoArg,
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
			return fmt.Errorf("repo delete: confirmation required; pass --yes in non-interactive mode")
		}
		typed, perr := opts.Prompter.Input(fmt.Sprintf("Type %q to confirm deletion", ref.FullName()), "")
		if perr != nil {
			return perr
		}
		if strings.TrimSpace(typed) != ref.FullName() {
			return fmt.Errorf("repo delete: confirmation mismatch; got %q", typed)
		}
	}

	client, err := opts.HTTPClient(ref.Host)
	if err != nil {
		return err
	}
	rc := repos.NewClient(client)

	if err := rc.Delete(ctx, ref.Owner, ref.Name); err != nil {
		return err
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Deleted %s\n", opts.IO.SuccessIcon(), ref.FullName())
	return nil
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
