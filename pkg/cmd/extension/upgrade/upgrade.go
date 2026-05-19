// SPDX-License-Identifier: AGPL-3.0-or-later

// Package upgrade implements `shithub extension upgrade {<name>|--all}`.
// For source-installed extensions this is `git pull` in the extension's
// directory; manually-installed (non-git) extensions are skipped with
// a friendly note.
package upgrade

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
	"github.com/tenseleyFlow/shithub-cli/internal/extension"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
)

type options struct {
	IO        *iostreams.IOStreams
	GitRunner git.Runner

	Name string
	All  bool
	Dir  string // override for tests
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{IO: f.IOStreams}
	cmd := &cobra.Command{
		Use:   "upgrade {<name> | --all}",
		Short: "Upgrade installed extensions via git pull",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Name = args[0]
			}
			if opts.Name == "" && !opts.All {
				return errors.New("extension upgrade: pass a name or --all")
			}
			if opts.Name != "" && opts.All {
				return errors.New("extension upgrade: --all and a name are mutually exclusive")
			}
			if opts.GitRunner == nil {
				if r, err := git.FromPath(); err == nil {
					opts.GitRunner = r
				}
			}
			return Run(c.Context(), opts)
		},
	}
	cmd.Flags().BoolVar(&opts.All, "all", false, "upgrade every installed extension")
	return cmd
}

// Run dispatches to single or all-extensions upgrade.
func Run(_ context.Context, opts *options) error {
	if opts.GitRunner == nil {
		return errors.New("extension upgrade: git binary required")
	}
	dir, err := resolveDir(opts.Dir)
	if err != nil {
		return err
	}
	if opts.All {
		xs, err := extension.List(dir)
		if err != nil {
			return err
		}
		var failed int
		for _, x := range xs {
			if err := upgradeOne(opts, x.Path, x.Name); err != nil {
				fmt.Fprintf(opts.IO.ErrOut, "%s upgrade %q: %v\n", opts.IO.FailureIcon(), x.Name, err)
				failed++
			}
		}
		if failed > 0 {
			return fmt.Errorf("extension upgrade: %d failed", failed)
		}
		return nil
	}
	if err := extension.ValidateName(opts.Name); err != nil {
		return err
	}
	target := filepath.Join(dir, extension.Prefix+opts.Name)
	return upgradeOne(opts, target, opts.Name)
}

func upgradeOne(opts *options, target, name string) error {
	// G12 (F40): distinguish "extension not installed at all" from
	// "installed but not a git checkout." The former is a hard error
	// (the targeted thing doesn't exist); the latter is a friendly
	// skip (the user can't upgrade what they vendored manually).
	// Pre-fix both paths emitted the same "skipped: not a git checkout"
	// notice and exited 0, hiding typos / missing extensions.
	if _, err := os.Stat(target); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("extension %q is not installed", name)
		}
		return err
	}
	if _, err := os.Stat(filepath.Join(target, ".git")); err != nil {
		fmt.Fprintf(opts.IO.ErrOut, "%s skipped: %q is not a git checkout (install with `extension install` to enable upgrades)\n",
			opts.IO.SuccessIcon(), name)
		return nil
	}
	if err := opts.GitRunner.Run(target, []string{"pull", "--ff-only"}, opts.IO.Out, opts.IO.ErrOut); err != nil {
		return err
	}
	// Re-flip the executable bit; gh sees the same issue when a release
	// archive is the source of truth, and the script comes back
	// non-executable after a pull on some filesystems.
	scriptPath := filepath.Join(target, extension.Prefix+name)
	if info, err := os.Stat(scriptPath); err == nil && !info.IsDir() {
		_ = os.Chmod(scriptPath, info.Mode()|0o111)
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Upgraded %q\n", opts.IO.SuccessIcon(), name)
	return nil
}

func resolveDir(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	return config.ExtensionsDir()
}

// quietWriter discards output — useful when callers want git pull's
// log lines suppressed.
type quietWriter struct{} //nolint:unused // kept for future --quiet flag

func (quietWriter) Write(p []byte) (int, error) { return len(p), nil }

var _ io.Writer = quietWriter{}
