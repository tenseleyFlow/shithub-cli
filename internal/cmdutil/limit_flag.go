// SPDX-License-Identifier: AGPL-3.0-or-later

package cmdutil

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

// limitValue implements pflag.Value for the --limit flag. H6 (H11):
// pflag.IntVarP leaks `strconv.ParseInt: parsing "foo": invalid syntax`
// when users pass non-integer values; gh emits a friendlier "must be a
// positive integer." The Set method's error message gets wrapped by
// cobra into `invalid argument "foo" for "-L, --limit" flag: <err>` so
// we only need the inner half. Lower-bound checks (< 1) are still
// surfaced via ValidateLimit at Run time to match the pre-existing
// gh-compat "invalid limit: N" wording.
type limitValue struct {
	p *int
}

func (v *limitValue) String() string {
	if v.p == nil {
		return "0"
	}
	return strconv.Itoa(*v.p)
}

func (v *limitValue) Set(s string) error {
	n, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("must be a positive integer (got %q)", s)
	}
	*v.p = n
	return nil
}

func (*limitValue) Type() string { return "int" }

// AddLimitFlag registers a --limit / -L flag on cmd backed by *p with a
// friendlier parse-error message than pflag.IntVarP. Default value goes
// in *p. usage is the cobra help string.
func AddLimitFlag(cmd *cobra.Command, p *int, def int, usage string) {
	*p = def
	cmd.Flags().VarP(&limitValue{p: p}, "limit", "L", usage)
}
