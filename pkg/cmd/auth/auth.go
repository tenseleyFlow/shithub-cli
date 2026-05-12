// SPDX-License-Identifier: AGPL-3.0-or-later

// Package auth wires the `shithub auth` subtree onto the cobra root.
// Per-subcommand logic lives in pkg/cmd/auth/<verb>; this file is just
// the parent + the AddCommand calls.
package auth

import (
	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	gitCred "github.com/tenseleyFlow/shithub-cli/pkg/cmd/auth/gitcredential"
	"github.com/tenseleyFlow/shithub-cli/pkg/cmd/auth/login"
	"github.com/tenseleyFlow/shithub-cli/pkg/cmd/auth/logout"
	"github.com/tenseleyFlow/shithub-cli/pkg/cmd/auth/refresh"
	"github.com/tenseleyFlow/shithub-cli/pkg/cmd/auth/setupgit"
	"github.com/tenseleyFlow/shithub-cli/pkg/cmd/auth/status"
	"github.com/tenseleyFlow/shithub-cli/pkg/cmd/auth/switchcmd"
	"github.com/tenseleyFlow/shithub-cli/pkg/cmd/auth/token"
)

// NewCmd returns the `auth` cobra command with every subcommand registered.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth <command>",
		Short: "Authenticate shithub-cli and configure git credential helpers",
		Long: `Manage authentication to shithub.

The minimum to do anything authenticated is:

    shithub auth login --hostname shithub.sh --with-token < token.txt

Tokens are stored in the system keyring by default. Pass --insecure-storage
to keep them in ~/.config/shithub/hosts.yml (0600) instead — for example on
a headless server with no secret-service daemon.`,
	}
	cmd.AddCommand(login.NewCmd(f))
	cmd.AddCommand(logout.NewCmd(f))
	cmd.AddCommand(status.NewCmd(f))
	cmd.AddCommand(token.NewCmd(f))
	cmd.AddCommand(refresh.NewCmd(f))
	cmd.AddCommand(setupgit.NewCmd(f))
	cmd.AddCommand(switchcmd.NewCmd(f))
	cmd.AddCommand(gitCred.NewCmd(f))
	return cmd
}
