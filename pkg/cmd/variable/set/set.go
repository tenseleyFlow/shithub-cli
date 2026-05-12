// SPDX-License-Identifier: AGPL-3.0-or-later

// Package set implements `shithub variable set <name>`. Variables go
// plaintext: no encryption, no public-key dance. The create-vs-update
// split mirrors gh: probe the existing variable; POST if missing,
// PATCH if present.
package set

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/repos"
	"github.com/tenseleyFlow/shithub-cli/internal/secrets"
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
	Org      string

	Body         string
	BodySet      bool
	BodyFile     string
	Visibility   string
	SelectedRepo []string
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
	}
	cmd := &cobra.Command{
		Use:   "set <name>",
		Short: "Create or update a variable",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts.Name = args[0]
			opts.BodySet = c.Flags().Changed("body")
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
	cmd.Flags().StringVarP(&opts.Org, "org", "o", "", "set an org-level variable")
	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "variable value (alternatively read from stdin)")
	cmd.Flags().StringVarP(&opts.BodyFile, "body-file", "f", "", "read variable value from a file (use '-' for stdin)")
	cmd.Flags().StringVar(&opts.Visibility, "visibility", "private", "org visibility: all | private | selected")
	cmd.Flags().StringSliceVarP(&opts.SelectedRepo, "repos", "r", nil, "selected repos for --visibility=selected (comma-separated names)")
	return cmd
}

// Run executes.
func Run(ctx context.Context, opts *options) error {
	value, err := readValue(opts)
	if err != nil {
		return err
	}
	if opts.Org != "" {
		return setOrgVariable(ctx, opts, string(value))
	}
	return setRepoVariable(ctx, opts, string(value))
}

func setRepoVariable(ctx context.Context, opts *options, value string) error {
	ref, err := repocmdshared.Resolver{
		RepoFlag:    opts.Repo,
		Hostname:    opts.Hostname,
		DefaultHost: hostOrDefault(opts.DefaultHost),
		GitRunner:   opts.GitRunner,
	}.Resolve()
	if err != nil {
		return err
	}
	client, err := opts.HTTPClient(ref.Host)
	if err != nil {
		return err
	}
	sc := secrets.NewClient(client)
	existing, _ := sc.GetRepoVariable(ctx, ref.Owner, ref.Name, opts.Name)
	if existing != nil {
		if err := sc.UpdateRepoVariable(ctx, ref.Owner, ref.Name, opts.Name,
			secrets.SetVariableInput{Value: value}); err != nil {
			return err
		}
		fmt.Fprintf(opts.IO.ErrOut, "%s Updated variable %s in %s/%s\n",
			opts.IO.SuccessIcon(), opts.Name, ref.Owner, ref.Name)
		return nil
	}
	if err := sc.CreateRepoVariable(ctx, ref.Owner, ref.Name,
		secrets.SetVariableInput{Name: opts.Name, Value: value}); err != nil {
		return err
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Created variable %s in %s/%s\n",
		opts.IO.SuccessIcon(), opts.Name, ref.Owner, ref.Name)
	return nil
}

func setOrgVariable(ctx context.Context, opts *options, value string) error {
	client, err := opts.HTTPClient(hostOrDefault(opts.DefaultHost))
	if err != nil {
		return err
	}
	sc := secrets.NewClient(client)
	visibility, ids, err := resolveOrgVisibility(ctx, client, opts)
	if err != nil {
		return err
	}
	input := secrets.SetVariableInput{
		Value:                 value,
		Visibility:            visibility,
		SelectedRepositoryIDs: ids,
	}
	existing, _ := sc.GetOrgVariable(ctx, opts.Org, opts.Name)
	if existing != nil {
		if err := sc.UpdateOrgVariable(ctx, opts.Org, opts.Name, input); err != nil {
			return err
		}
		fmt.Fprintf(opts.IO.ErrOut, "%s Updated variable %s in org %s\n",
			opts.IO.SuccessIcon(), opts.Name, opts.Org)
		return nil
	}
	input.Name = opts.Name
	if err := sc.CreateOrgVariable(ctx, opts.Org, input); err != nil {
		return err
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Created variable %s in org %s\n",
		opts.IO.SuccessIcon(), opts.Name, opts.Org)
	return nil
}

// resolveOrgVisibility mirrors the secret-side helper. Kept local
// because the import graph for a tiny shared helper isn't worth a
// dedicated package yet.
func resolveOrgVisibility(ctx context.Context, client *api.Client, opts *options) (string, []int64, error) {
	v := strings.ToLower(opts.Visibility)
	switch v {
	case "all", "private":
		if len(opts.SelectedRepo) > 0 {
			return "", nil, errors.New("variable set: --repos requires --visibility=selected")
		}
		return v, nil, nil
	case "selected":
		if len(opts.SelectedRepo) == 0 {
			return "", nil, errors.New("variable set: --visibility=selected requires --repos")
		}
		rc := repos.NewClient(client)
		ids := make([]int64, 0, len(opts.SelectedRepo))
		for _, name := range opts.SelectedRepo {
			r, err := rc.View(ctx, opts.Org, strings.TrimSpace(name))
			if err != nil {
				return "", nil, fmt.Errorf("variable set: resolve %s/%s: %w", opts.Org, name, err)
			}
			ids = append(ids, r.ID)
		}
		return v, ids, nil
	default:
		return "", nil, fmt.Errorf("variable set: --visibility must be one of all|private|selected (got %q)", v)
	}
}

func readValue(opts *options) ([]byte, error) {
	switch {
	case opts.BodySet:
		return []byte(opts.Body), nil
	case opts.BodyFile == "-":
		return io.ReadAll(opts.IO.In)
	case opts.BodyFile != "":
		return os.ReadFile(opts.BodyFile) //nolint:gosec // user-supplied path
	default:
		blob, err := io.ReadAll(opts.IO.In)
		if err != nil {
			return nil, fmt.Errorf("variable set: read stdin: %w", err)
		}
		if len(blob) == 0 {
			return nil, errors.New("variable set: no value provided (use --body, --body-file, or pipe via stdin)")
		}
		return blob, nil
	}
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
