// SPDX-License-Identifier: AGPL-3.0-or-later

// Package create implements `shithub label create`. Posts a new label
// with --color/--description; --force overwrites an existing label of
// the same name (via PATCH after POST 422s).
package create

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

	Color       string
	Description string
	Force       bool
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
	}
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new label",
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
	cmd.Flags().StringVarP(&opts.Color, "color", "c", "", "hex color (3 or 6 digits, e.g., f29513)")
	cmd.Flags().StringVarP(&opts.Description, "description", "d", "", "label description")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "overwrite if a label with this name already exists")
	return cmd
}

// Run executes the create operation.
func Run(ctx context.Context, opts *options) error {
	if opts.Name == "" {
		return errors.New("label create: name is required")
	}
	color := ""
	if opts.Color != "" {
		c, err := labelshared.NormalizeColor(opts.Color)
		if err != nil {
			return err
		}
		color = c
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

	in := labels.CreateInput{Name: opts.Name, Color: color, Description: opts.Description}
	created, err := lc.Create(ctx, ref.Owner, ref.Name, in)
	if err != nil {
		if isAlreadyExists(err) {
			if opts.Force {
				edited, eerr := lc.Edit(ctx, ref.Owner, ref.Name, opts.Name, labels.EditInput{
					Color:       ptr(color),
					Description: ptr(opts.Description),
				})
				if eerr != nil {
					return eerr
				}
				fmt.Fprintf(opts.IO.ErrOut, "%s Overwrote label %s\n", opts.IO.SuccessIcon(), edited.Name)
				return nil
			}
			// Audit A13: the raw "shithub API: 409 label name already
			// taken" string is hostile UX (especially on fresh repos
			// where the server pre-seeds the GitHub default set, so
			// `label create bug` 409s every time). Surface the
			// situation in plain English; the --force hint nudges
			// users toward the overwrite path when they meant it.
			return fmt.Errorf("label %q already exists on %s/%s; pass --force to overwrite",
				opts.Name, ref.Owner, ref.Name)
		}
		return err
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Created label %s\n", opts.IO.SuccessIcon(), created.Name)
	return nil
}

// isAlreadyExists reports whether the server's response indicates a
// uniqueness conflict on label name. The server historically returned
// 422 (validation failed) for that case; the labels handler also
// returns 409 Conflict on the unique-name violation now. Accept either
// so the friendly-error path is robust across server versions.
func isAlreadyExists(err error) bool {
	var ae *api.APIError
	if errors.As(err, &ae) && (ae.StatusCode == 422 || ae.StatusCode == 409) {
		return true
	}
	return false
}

func ptr[T any](v T) *T { return &v }

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
