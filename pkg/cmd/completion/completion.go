// SPDX-License-Identifier: AGPL-3.0-or-later

// Package completion implements `shithub completion`. Thin wrapper
// around cobra's built-in completion generators — we don't customize
// the output beyond exposing --no-descriptions for terminals that
// don't render description columns nicely.
package completion

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
)

// supportedShells documents every shell cobra natively generates for.
// Adding one is a matter of extending the switch in Run, but keeping
// the list explicit means we can give a friendlier error than cobra's
// default.
var supportedShells = []string{"bash", "zsh", "fish", "powershell"}

type options struct {
	IO    *iostreams.IOStreams
	Shell string
	// NoDesc disables description columns in zsh/fish where they show.
	// bash + powershell already omit descriptions; the flag is a no-op
	// for those shells.
	NoDesc bool
}

// NewCmd returns the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{IO: f.IOStreams}
	cmd := &cobra.Command{
		Use:   "completion [shell]",
		Short: "Print a shell completion script",
		Long: `Generate a shell completion script for the named shell.

Supported shells: bash, zsh, fish, powershell.

The shell may be passed positionally (gh-style) or via -s/--shell.

    bash:        source <(shithub completion bash)
    zsh:         source <(shithub completion zsh)
    fish:        shithub completion fish | source
    powershell:  shithub completion powershell | Out-String | Invoke-Expression
`,
		// Audit A4: gh accepts the shell as a positional argument.
		// Accept up to one positional; -s/--shell stays for script
		// ports that depend on the flag form.
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: supportedShells,
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) == 1 && opts.Shell == "" {
				opts.Shell = args[0]
			}
			return Run(c.Context(), c.Root(), opts)
		},
	}
	cmd.Flags().StringVarP(&opts.Shell, "shell", "s", "", "target shell (bash, zsh, fish, powershell)")
	cmd.Flags().BoolVar(&opts.NoDesc, "no-descriptions", false, "omit description columns (zsh, fish)")
	return cmd
}

// Run emits the completion script for opts.Shell to opts.IO.Out.
func Run(_ context.Context, root *cobra.Command, opts *options) error {
	if opts.Shell == "" {
		return errors.New("completion: --shell is required (one of: " + listShells() + ")")
	}
	return generate(root, opts.IO.Out, opts.Shell, opts.NoDesc)
}

// generate dispatches to cobra's per-shell generator. Centralized so a
// future shell addition only touches the switch.
func generate(root *cobra.Command, out io.Writer, shell string, noDesc bool) error {
	switch shell {
	case "bash":
		return root.GenBashCompletionV2(out, !noDesc)
	case "zsh":
		if noDesc {
			return root.GenZshCompletionNoDesc(out)
		}
		return root.GenZshCompletion(out)
	case "fish":
		return root.GenFishCompletion(out, !noDesc)
	case "powershell":
		return root.GenPowerShellCompletionWithDesc(out)
	default:
		return fmt.Errorf("completion: unsupported shell %q (valid: %s)", shell, listShells())
	}
}

// listShells returns the supported set as a comma-joined string for
// error messages.
func listShells() string {
	out := ""
	for i, s := range supportedShells {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}
