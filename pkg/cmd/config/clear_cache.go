// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
)

type clearCacheOptions struct {
	IO *iostreams.IOStreams
}

func newClearCacheCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &clearCacheOptions{IO: f.IOStreams}
	cmd := &cobra.Command{
		Use:   "clear-cache",
		Short: "Remove every entry from the on-disk shithub-cli cache",
		Long: `Wipe the cache directory used by 'shithub api --cache' and any future
sprint-introduced disk caches. Recreates an empty 0700 directory so the
next cache write doesn't have to set up permissions.`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return clearCacheRun(c.Context(), opts)
		},
	}
	return cmd
}

func clearCacheRun(_ context.Context, opts *clearCacheOptions) error {
	dir, err := config.CacheDir()
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("config: clear-cache: %w", err)
	}
	// Recreate the empty dir so callers don't trip on a missing path on
	// the next API call.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("config: recreate cache dir: %w", err)
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s cache cleared\n", opts.IO.SuccessIcon())
	return nil
}
