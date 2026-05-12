// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/build"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/prompter"
	apicmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/api"
	"github.com/tenseleyFlow/shithub-cli/pkg/cmd/auth"
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

// Execute runs the root command and exits non-zero on error.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "shithub:", err)
		os.Exit(1)
	}
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
}
