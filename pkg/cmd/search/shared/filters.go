// SPDX-License-Identifier: AGPL-3.0-or-later

package shared

import (
	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/search"
)

// CommonFlags wraps the flags every `search <kind>` subcommand shares:
// sort, order, limit, --web, --json. Subcommands bind their own filter
// flags on top, then call ToOptions to produce a typed search.Options.
type CommonFlags struct {
	Sort  string
	Order string
	Limit int
	Web   bool
}

// AddCommonFlags binds the shared knobs onto cmd. sortValues / order
// docs differ per kind, so the subcommand passes its own descriptions.
func AddCommonFlags(cmd *cobra.Command, f *CommonFlags, sortDesc, orderDesc string) {
	cmd.Flags().StringVar(&f.Sort, "sort", "", sortDesc)
	cmd.Flags().StringVar(&f.Order, "order", "desc", orderDesc)
	cmdutil.AddLimitFlag(cmd, &f.Limit, 30, "max items to return")
	cmd.Flags().BoolVarP(&f.Web, "web", "w", false, "open the search results in a browser")
}

// Validate rejects nonsensical flag combinations before issuing a
// request. Currently only `--limit`; if more cross-flag invariants
// surface (e.g. mutually exclusive sort/order pairs) they belong here.
func (f CommonFlags) Validate() error {
	return cmdutil.ValidateLimit(f.Limit)
}

// ToOptions lowers CommonFlags into the search package's Options
// struct. PerPage is left to the client (clamped to 100) — callers
// shouldn't need to set it directly.
func (f CommonFlags) ToOptions() search.Options {
	return search.Options{
		Sort:  f.Sort,
		Order: f.Order,
		Limit: f.Limit,
	}
}
