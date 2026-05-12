// SPDX-License-Identifier: AGPL-3.0-or-later

// Package download implements `shithub run download <run-id>` — pulls
// artifacts (zip archives) produced by a workflow run to the local
// filesystem.
package download

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/actions"
	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	repocmdshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

type options struct {
	IO          *iostreams.IOStreams
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string
	GitRunner   git.Runner

	RunID    string
	Repo     string
	Hostname string
	Dir      string
	Names    []string
	Patterns []string
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
	}
	cmd := &cobra.Command{
		Use:   "download <run-id>",
		Short: "Download artifacts produced by a workflow run",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts.RunID = args[0]
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
	cmd.Flags().StringVarP(&opts.Dir, "dir", "D", ".", "destination directory (created if missing)")
	cmd.Flags().StringArrayVarP(&opts.Names, "name", "n", nil, "download only artifacts matching this name (repeatable)")
	cmd.Flags().StringArrayVarP(&opts.Patterns, "pattern", "p", nil, "download artifacts whose names match this glob (repeatable)")
	return cmd
}

// Run lists matching artifacts and writes each as <name>.zip into Dir.
func Run(ctx context.Context, opts *options) error {
	id, err := strconv.ParseInt(opts.RunID, 10, 64)
	if err != nil {
		return fmt.Errorf("run download: %q is not a numeric run id", opts.RunID)
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
	ac := actions.NewClient(client)
	all, err := ac.ListArtifacts(ctx, ref.Owner, ref.Name, id)
	if err != nil {
		return err
	}
	picked, err := filterArtifacts(all, opts.Names, opts.Patterns)
	if err != nil {
		return err
	}
	if len(picked) == 0 {
		return errors.New("run download: no artifacts matched the given filters")
	}

	if err := os.MkdirAll(opts.Dir, 0o755); err != nil { //nolint:gosec // user-supplied dir
		return fmt.Errorf("run download: mkdir: %w", err)
	}

	for _, a := range picked {
		if a.Expired {
			fmt.Fprintf(opts.IO.ErrOut, "skipping expired artifact %q\n", a.Name)
			continue
		}
		dst := filepath.Join(opts.Dir, sanitize(a.Name)+".zip")
		if err := download(ctx, ac, ref.Owner, ref.Name, a.ID, dst); err != nil {
			return fmt.Errorf("run download: %s: %w", a.Name, err)
		}
		fmt.Fprintf(opts.IO.ErrOut, "%s %s -> %s\n", opts.IO.SuccessIcon(), a.Name, dst)
	}
	return nil
}

func download(ctx context.Context, ac *actions.Client, owner, repo string, artifactID int64, dst string) error {
	f, err := os.Create(dst) //nolint:gosec // user-supplied dst
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return ac.DownloadArtifact(ctx, owner, repo, artifactID, f)
}

// filterArtifacts applies Names (exact) and Patterns (filepath.Match
// globs).  When both sets are empty, all artifacts pass through.
func filterArtifacts(all []actions.Artifact, names, patterns []string) ([]actions.Artifact, error) {
	if len(names) == 0 && len(patterns) == 0 {
		return all, nil
	}
	nameSet := make(map[string]struct{}, len(names))
	for _, n := range names {
		nameSet[n] = struct{}{}
	}
	out := make([]actions.Artifact, 0, len(all))
	for _, a := range all {
		if _, ok := nameSet[a.Name]; ok {
			out = append(out, a)
			continue
		}
		for _, p := range patterns {
			matched, err := filepath.Match(p, a.Name)
			if err != nil {
				return nil, fmt.Errorf("run download: bad pattern %q: %w", p, err)
			}
			if matched {
				out = append(out, a)
				break
			}
		}
	}
	return out, nil
}

// sanitize strips path separators so an artifact name cannot escape Dir.
func sanitize(s string) string {
	s = strings.ReplaceAll(s, string(filepath.Separator), "_")
	s = strings.ReplaceAll(s, "/", "_")
	if s == "" {
		return "artifact"
	}
	return s
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
