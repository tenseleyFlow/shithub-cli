// SPDX-License-Identifier: AGPL-3.0-or-later

// Package install implements `shithub extension install <owner/repo>`.
// Source-clone install only this sprint: binary-asset detection is
// gated on shithub releases (S48), which is deferred.
package install

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
	"github.com/tenseleyFlow/shithub-cli/internal/extension"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	repocmdshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

type options struct {
	IO          *iostreams.IOStreams
	DefaultHost func() string
	GitRunner   git.Runner

	Repo  string // owner/shithub-<verb>
	Pin   string
	Force bool

	Dir string // override for tests
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		DefaultHost: f.DefaultHost,
	}
	cmd := &cobra.Command{
		Use:   "install <owner/repo>",
		Short: "Install a shithub-cli extension from a repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts.Repo = args[0]
			if opts.GitRunner == nil {
				if r, err := git.FromPath(); err == nil {
					opts.GitRunner = r
				}
			}
			return Run(c.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Pin, "pin", "", "install at a specific tag, branch, or commit")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "overwrite an existing installation of the same extension")
	return cmd
}

// Run resolves the repo, clones it into the extensions dir, and prints
// a friendly summary.
func Run(_ context.Context, opts *options) error {
	if opts.GitRunner == nil {
		return errors.New("extension install: git binary required")
	}
	verb, ok := extension.VerbFromRepo(opts.Repo)
	if !ok {
		return fmt.Errorf("extension install: repo %q must be in the form owner/shithub-<verb>", opts.Repo)
	}
	dir, err := resolveDir(opts.Dir)
	if err != nil {
		return err
	}
	target := filepath.Join(dir, extension.Prefix+verb)
	if _, statErr := os.Stat(target); statErr == nil {
		if !opts.Force {
			return fmt.Errorf("extension install: %s already exists (use --force to overwrite)", target)
		}
		if err := os.RemoveAll(target); err != nil {
			return fmt.Errorf("extension install: remove existing: %w", err)
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // user-owned ext dir
		return fmt.Errorf("extension install: mkdir: %w", err)
	}

	cloneURL := composeCloneURL(opts)
	var extra []string
	if opts.Pin != "" {
		extra = append(extra, "--branch", opts.Pin, "--single-branch")
	}
	if _, err := git.Clone(opts.GitRunner, cloneURL, target, extra, opts.IO.Out, opts.IO.ErrOut); err != nil {
		return err
	}
	// Make sure the script is executable. gh ships extensions whose
	// scripts may have been packed without the +x bit on Windows;
	// flipping it here means `Find` resolves them on first use.
	scriptPath := filepath.Join(target, extension.Prefix+verb)
	if info, err := os.Stat(scriptPath); err == nil && !info.IsDir() {
		_ = os.Chmod(scriptPath, info.Mode()|0o111)
	}

	fmt.Fprintf(opts.IO.ErrOut, "%s Installed extension %q -> %s\n",
		opts.IO.SuccessIcon(), verb, target)
	fmt.Fprintf(opts.IO.ErrOut,
		"Note: binary-asset extensions require shithub-S48 (Releases); installed from source git checkout.\n")
	return nil
}

// composeCloneURL builds the HTTPS clone URL for owner/repo against the
// configured default host.  We deliberately avoid honoring --hostname
// because extensions live wherever their owner published them; the user
// passing a slug typically means "fetch from my default host".
func composeCloneURL(opts *options) string {
	host := ""
	if opts.DefaultHost != nil {
		host = opts.DefaultHost()
	}
	if host == "" {
		host = "shithub.sh"
	}
	owner, repo, _ := strings.Cut(opts.Repo, "/")
	return repocmdshared.CloneURL(repocmdshared.RepoRef{Host: host, Owner: owner, Name: repo}, "https")
}

func resolveDir(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	return config.ExtensionsDir()
}
