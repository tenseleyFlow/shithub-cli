// SPDX-License-Identifier: AGPL-3.0-or-later

package cmdutil

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

// DeferredSpec configures a stub subcommand that points at a feature
// shithub's server doesn't ship yet. Callers don't construct one
// directly; pass the fields into NewDeferredCmd.
//
// E-audit E20: the previous stubs declared zero flags, so any
// gh-style flag-bearing invocation (`shithub release create v1 --notes
// 'first'`) blew up with "unknown flag: --notes" before the friendly
// notice could fire. NewDeferredCmd uses FParseErrWhitelist.UnknownFlags
// so the user gets the deferred message regardless of how they invoked
// the command — including E21's `-R` case on pin/unpin/transfer.
type DeferredSpec struct {
	Use   string // cobra `Use` (e.g. "create <tag>")
	Short string // one-line description
	Name  string // command name used in the user-facing message
	Track string // sprint or doc reference users can follow
}

// NotYetSupportedError carries the user-facing deferred message. Tests
// can errors.As against it without grepping strings.
type NotYetSupportedError struct {
	Name  string
	Track string
}

func (e *NotYetSupportedError) Error() string {
	if e.Track == "" {
		return fmt.Sprintf("%s: not yet supported by this shithub host", e.Name)
	}
	return fmt.Sprintf("%s: not yet supported by this shithub host (tracked in %s)", e.Name, e.Track)
}

// IsNotYetSupported reports whether err is (or wraps) a deferred-feature
// signal. The CLI's main translates this to exit code 2 so scripts can
// distinguish "feature not available" from a hard error (exit 1).
func IsNotYetSupported(err error) bool {
	var n *NotYetSupportedError
	return errors.As(err, &n)
}

// NewDeferredCmd builds a stub cobra command for a feature whose
// server-side contract isn't shipped yet. Accepts arbitrary positional
// args and unknown flags so it always reaches the RunE and emits the
// friendly notice; downstream tooling (and `--help`) still see the
// command so it's discoverable.
//
// The returned command also accepts the `-R` / `--repo` flag because
// pin/unpin/transfer specifically advertised "deferred server-side" —
// users in a non-default cwd still need to point at a repo even
// without a working endpoint (E21).
func NewDeferredCmd(spec DeferredSpec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   spec.Use,
		Short: spec.Short,
		Args:  cobra.ArbitraryArgs,
		// Don't choke on flags the user passes — gh-style invocations
		// should always reach RunE, where they get the friendly notice.
		FParseErrWhitelist: cobra.FParseErrWhitelist{UnknownFlags: true},
		RunE: func(_ *cobra.Command, _ []string) error {
			return &NotYetSupportedError{Name: spec.Name, Track: spec.Track}
		},
	}
	// Register --repo / -R so users who *do* care about which repo
	// they're pointing at see the flag in --help and don't have to
	// remove it to discover the deferred status (E21).
	var repoFlag, hostnameFlag string
	cmd.Flags().StringVarP(&repoFlag, "repo", "R", "", "select another repository using the [HOST/]OWNER/REPO format (deferred — flag accepted for forward-compat)")
	cmd.Flags().StringVar(&hostnameFlag, "hostname", "", "the shithub host (deferred — flag accepted for forward-compat)")
	return cmd
}
