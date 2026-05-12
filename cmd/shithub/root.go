// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/alias"
	"github.com/tenseleyFlow/shithub-cli/internal/build"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/prompter"
	aliascmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/alias"
	apicmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/api"
	"github.com/tenseleyFlow/shithub-cli/pkg/cmd/auth"
	"github.com/tenseleyFlow/shithub-cli/pkg/cmd/completion"
	configcmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/config"
	issuecmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/issue"
	prcmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/pr"
	repocmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo"
)

// longDescription is split out so help text stays readable and we can
// extend it without touching the cobra.Command literal.
const longDescription = `shithub is the command-line client for shithub.sh — a feature-complete,
AGPLv3-licensed reverse engineering of GitHub. The command tree mirrors
GitHub's gh CLI; flags and output shapes are gh-compatible where shithub's
data model permits.

Authentication, configuration, and host management live under
'shithub auth' and 'shithub config'. The 'shithub api' escape hatch
exposes the raw REST surface for anything the typed subcommands have not
yet wrapped.`

var rootCmd = &cobra.Command{
	Use:   "shithub",
	Short: "shithub: command-line client for shithub.sh",
	Long:  longDescription,
	// We surface errors ourselves in Execute so cobra's usage dump does
	// not bury the real failure when a user mistypes a subcommand.
	SilenceUsage:  true,
	SilenceErrors: true,
	// Version powers --version. Format mirrors `shithub version` output.
	Version: fmt.Sprintf("%s (%s) built %s", build.Version, build.Commit, build.Date),
}

// Execute runs the root command and exits non-zero on error. Before
// dispatching to cobra it tries alias expansion against the user's
// config; a matching shell alias takes over the process entirely.
func Execute() {
	if exitCode, handled := tryAlias(os.Args[1:]); handled {
		os.Exit(exitCode)
	}
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "shithub:", err)
		os.Exit(1)
	}
}

// tryAlias looks up the first non-flag arg as an alias name. Returns
// (exitCode, true) only when the request was satisfied by a shell alias
// (process has already run and we should exit). For direct aliases it
// rewrites os.Args in place and returns (0, false) so cobra picks up
// the rewritten verb. For misses or errors it returns (0, false) and
// lets cobra produce its usual "unknown command" feedback.
func tryAlias(args []string) (int, bool) {
	cfg, err := config.Load()
	if err != nil || cfg == nil || len(cfg.Aliases) == 0 {
		return 0, false
	}
	builtins := make([]string, 0, len(rootCmd.Commands()))
	for _, c := range rootCmd.Commands() {
		builtins = append(builtins, c.Name())
	}
	res, ok, err := alias.Dispatch(args, cfg.Aliases, builtins)
	if err != nil {
		fmt.Fprintln(os.Stderr, "shithub:", err)
		return 1, true
	}
	if !ok {
		return 0, false
	}
	if !res.Shell {
		// Rewrite os.Args so cobra sees the expanded verb.
		os.Args = append([]string{os.Args[0]}, res.Argv...)
		return 0, false
	}

	// Shell alias: spawn and exit with its status.
	se, err := alias.BuildShellCommand(res.Command, res.Args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "shithub: alias shell:", err)
		return 1, true
	}
	cmd := se.Cmd()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// Surface the child's exit code if the process exited non-zero;
		// fall back to 1 for spawn or signal failures.
		if cmd.ProcessState != nil {
			return cmd.ProcessState.ExitCode(), true
		}
		fmt.Fprintln(os.Stderr, "shithub:", err)
		return 1, true
	}
	return 0, true
}

func init() {
	// Match gh's "shithub <ver> (...) built ..." shape for --version.
	rootCmd.SetVersionTemplate("shithub {{.Version}}\n")
	rootCmd.AddCommand(versionCmd)

	// Build the production cmdutil.Factory once at startup. A construction
	// failure means our config substrate is broken at a level no command
	// can recover from; surface it via stderr and exit non-zero before any
	// subcommand has a chance to swallow it.
	ios := iostreams.System()
	p := prompter.NewSurvey(ios)
	f, err := cmdutil.New(p)
	if err != nil {
		fmt.Fprintln(os.Stderr, "shithub: factory init:", err)
		os.Exit(1)
	}
	// The system IOStreams instance is what we built the factory with;
	// reach back into the field so anything else needing it stays in sync.
	f.IOStreams = ios

	rootCmd.AddCommand(auth.NewCmd(f))
	rootCmd.AddCommand(apicmd.NewCmd(f))
	rootCmd.AddCommand(configcmd.NewCmd(f))
	rootCmd.AddCommand(aliascmd.NewCmd(f))
	rootCmd.AddCommand(completion.NewCmd(f))
	rootCmd.AddCommand(repocmd.NewCmd(f))
	rootCmd.AddCommand(issuecmd.NewCmd(f))
	rootCmd.AddCommand(prcmd.NewCmd(f))
}
