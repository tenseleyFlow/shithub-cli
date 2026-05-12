// SPDX-License-Identifier: AGPL-3.0-or-later

package alias

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"

	"github.com/tenseleyFlow/shithub-cli/internal/alias"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
)

type importOptions struct {
	IO     *iostreams.IOStreams
	Config func() (*config.Config, error)

	Source       string // "-" for stdin, otherwise file path
	Clobber      bool
	BuiltinNames []string
}

func newImportCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &importOptions{IO: f.IOStreams, Config: f.Config}
	cmd := &cobra.Command{
		Use:   "import [<file|->]",
		Short: "Load multiple aliases from a YAML file",
		Long: `Read a YAML map of alias-name → expansion and persist every entry.
By default existing aliases with conflicting names cause the whole
import to abort (atomic semantics); pass --clobber to overwrite.

Use '-' for stdin.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts.Source = "-"
			if len(args) == 1 {
				opts.Source = args[0]
			}
			opts.BuiltinNames = builtinNames(c.Root())
			return importRun(c.Context(), opts)
		},
	}
	cmd.Flags().BoolVar(&opts.Clobber, "clobber", false, "overwrite aliases that already exist")
	return cmd
}

func importRun(_ context.Context, opts *importOptions) error {
	data, err := readImportSource(opts.Source, opts.IO.In)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return errors.New("alias import: empty input")
	}
	var incoming map[string]string
	if err := yaml.Unmarshal(data, &incoming); err != nil {
		return fmt.Errorf("alias import: parse yaml: %w", err)
	}

	// Pre-validate everything before mutating; one bad entry means no
	// partial state on disk.
	for name := range incoming {
		if err := alias.Validate(name, opts.BuiltinNames); err != nil {
			return err
		}
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	if cfg.Aliases == nil {
		cfg.Aliases = map[string]string{}
	}
	if !opts.Clobber {
		for name := range incoming {
			if _, exists := cfg.Aliases[name]; exists {
				return fmt.Errorf("alias import: alias %q already exists; pass --clobber to overwrite", name)
			}
		}
	}
	for name, expansion := range incoming {
		cfg.Aliases[name] = expansion
	}
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("alias import: save: %w", err)
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s imported %d alias(es)\n", opts.IO.SuccessIcon(), len(incoming))
	return nil
}

func readImportSource(path string, stdin io.Reader) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(stdin)
	}
	return os.ReadFile(path) //nolint:gosec // path is user-supplied; that's the command's whole point
}
