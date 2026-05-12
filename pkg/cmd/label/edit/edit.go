// SPDX-License-Identifier: AGPL-3.0-or-later

// Package edit implements `shithub label edit`. Rename via --name,
// update color/description in place via --color/--description.
package edit

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/labels"
	labelshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/label/shared"
	repocmdshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

type options struct {
	IO          *iostreams.IOStreams
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string
	GitRunner   git.Runner

	Name     string
	Repo     string
	Hostname string

	NewName        string
	newNameSet     bool
	Color          string
	colorSet       bool
	Description    string
	descriptionSet bool
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
	}
	cmd := &cobra.Command{
		Use:   "edit <name>",
		Short: "Edit a label",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts.Name = args[0]
			opts.newNameSet = c.Flags().Changed("name")
			opts.colorSet = c.Flags().Changed("color")
			opts.descriptionSet = c.Flags().Changed("description")
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
	cmd.Flags().StringVar(&opts.NewName, "name", "", "rename the label")
	cmd.Flags().StringVarP(&opts.Color, "color", "c", "", "new hex color (3 or 6 digits)")
	cmd.Flags().StringVarP(&opts.Description, "description", "d", "", "new description")
	return cmd
}

// Run executes the edit.
func Run(ctx context.Context, opts *options) error {
	if !opts.newNameSet && !opts.colorSet && !opts.descriptionSet {
		return errors.New("label edit: nothing to do; pass --name, --color, or --description")
	}

	patch := labels.EditInput{}
	if opts.newNameSet {
		if opts.NewName == "" {
			return errors.New("label edit: --name cannot be empty")
		}
		v := opts.NewName
		patch.NewName = &v
	}
	if opts.colorSet {
		c, err := labelshared.NormalizeColor(opts.Color)
		if err != nil {
			return err
		}
		patch.Color = &c
	}
	if opts.descriptionSet {
		v := opts.Description
		patch.Description = &v
	}

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
	client, err := opts.HTTPClient(ref.Host)
	if err != nil {
		return err
	}
	lc := labels.NewClient(client)

	out, err := lc.Edit(ctx, ref.Owner, ref.Name, opts.Name, patch)
	if err != nil {
		return err
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Edited label %s\n", opts.IO.SuccessIcon(), out.Name)
	return nil
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
