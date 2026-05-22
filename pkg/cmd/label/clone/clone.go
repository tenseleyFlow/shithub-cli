// SPDX-License-Identifier: AGPL-3.0-or-later

// Package clone implements `shithub label clone`. Copies labels from a
// source repo into the current repo. Collisions are skipped with a
// warning by default; --force overwrites via PATCH.
package clone

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
	repocmdshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

type options struct {
	IO          *iostreams.IOStreams
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string
	GitRunner   git.Runner

	Source   string
	Repo     string
	Hostname string
	Force    bool
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
	}
	cmd := &cobra.Command{
		Use:   "clone <source-repo>",
		Short: "Clone labels from another repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts.Source = args[0]
			if opts.GitRunner == nil {
				if r, err := git.FromPath(); err == nil {
					opts.GitRunner = r
				}
			}
			return Run(c.Context(), opts)
		},
	}
	cmd.Flags().StringVarP(&opts.Repo, "repo", "R", "", "destination repository ([HOST/]OWNER/REPO format)")
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "the shithub host (default: configured host)")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "overwrite labels that already exist on the destination")
	return cmd
}

// Run executes the clone.
//
// the cyclomatic count comes from the existence + force decision tree.
//
//nolint:gocyclo // straightforward fetch-from-source then push-to-dest;
func Run(ctx context.Context, opts *options) error {
	if opts.Source == "" {
		return errors.New("label clone: source repo is required")
	}
	srcRef, err := repocmdshared.ParseRepoArg(opts.Source)
	if err != nil {
		return fmt.Errorf("label clone: source: %w", err)
	}

	dest := repocmdshared.Resolver{
		RepoFlag:    opts.Repo,
		Hostname:    opts.Hostname,
		DefaultHost: hostOrDefault(opts.DefaultHost),
		GitRunner:   opts.GitRunner,
	}
	destRef, err := dest.Resolve()
	if err != nil {
		return err
	}
	if srcRef.Host == "" {
		srcRef.Host = destRef.Host
	}
	// H30: self-clone (source and dest resolve to the same repo) walks
	// every label, marks each as "already exists", and exits 0 with no
	// useful side-effect. With --force it would overwrite each label
	// with itself. Refuse the identity case so the user notices the
	// `-R` typo / duplicated arg before any round-trip.
	if strings.EqualFold(srcRef.Host, destRef.Host) &&
		strings.EqualFold(srcRef.Owner, destRef.Owner) &&
		strings.EqualFold(srcRef.Name, destRef.Name) {
		return fmt.Errorf("label clone: source and destination are the same repo (%s)", srcRef.FullName())
	}

	srcClient, err := opts.HTTPClient(srcRef.Host)
	if err != nil {
		return err
	}
	destClient, err := opts.HTTPClient(destRef.Host)
	if err != nil {
		return err
	}
	srcLC := labels.NewClient(srcClient)
	destLC := labels.NewClient(destClient)

	srcLabels, err := srcLC.List(ctx, srcRef.Owner, srcRef.Name, labels.ListOptions{Limit: 1000})
	if err != nil {
		return fmt.Errorf("label clone: list source: %w", err)
	}
	destLabels, err := destLC.List(ctx, destRef.Owner, destRef.Name, labels.ListOptions{Limit: 1000})
	if err != nil {
		return fmt.Errorf("label clone: list dest: %w", err)
	}

	existing := map[string]labels.Label{}
	for _, l := range destLabels {
		existing[l.Name] = l
	}

	var created, overwrote, skipped int
	for _, l := range srcLabels {
		if _, exists := existing[l.Name]; exists {
			if !opts.Force {
				skipped++
				fmt.Fprintf(opts.IO.ErrOut, "skipped %s (exists; pass --force to overwrite)\n", l.Name)
				continue
			}
			color := l.Color
			desc := l.Description
			if _, err := destLC.Edit(ctx, destRef.Owner, destRef.Name, l.Name, labels.EditInput{
				Color:       &color,
				Description: &desc,
			}); err != nil {
				return fmt.Errorf("label clone: edit %s: %w", l.Name, err)
			}
			overwrote++
			continue
		}
		if _, err := destLC.Create(ctx, destRef.Owner, destRef.Name, labels.CreateInput{
			Name: l.Name, Color: l.Color, Description: l.Description,
		}); err != nil {
			return fmt.Errorf("label clone: create %s: %w", l.Name, err)
		}
		created++
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Cloned labels from %s into %s — created %d, overwrote %d, skipped %d\n",
		opts.IO.SuccessIcon(), srcRef.FullName(), destRef.FullName(), created, overwrote, skipped)
	return nil
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
