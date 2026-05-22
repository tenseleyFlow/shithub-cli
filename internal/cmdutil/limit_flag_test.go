// SPDX-License-Identifier: AGPL-3.0-or-later

package cmdutil_test

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
)

// TestAddLimitFlag pins H11: parse-time errors are friendly. Pre-fix
// pflag.IntVarP leaked `strconv.ParseInt: parsing "foo": invalid syntax`.
func TestAddLimitFlag(t *testing.T) {
	cases := []struct {
		arg      string
		wantErr  bool
		errMatch string
	}{
		{"30", false, ""},
		{"1", false, ""},
		{"-1", false, ""}, // parse succeeds; ValidateLimit catches the bound at Run time
		{"foo", true, "must be a positive integer"},
		{"1.5", true, "must be a positive integer"},
		{"1e100", true, "must be a positive integer"},
		{"", true, "must be a positive integer"},
	}
	for _, tc := range cases {
		var got int
		cmd := &cobra.Command{Use: "x"}
		cmdutil.AddLimitFlag(cmd, &got, 30, "max items")
		err := cmd.Flags().Set("limit", tc.arg)
		if tc.wantErr {
			if err == nil {
				t.Errorf("arg=%q: want error, got nil", tc.arg)
				continue
			}
			if !strings.Contains(err.Error(), tc.errMatch) {
				t.Errorf("arg=%q: error %q missing %q", tc.arg, err.Error(), tc.errMatch)
			}
		} else if err != nil {
			t.Errorf("arg=%q: unexpected error: %v", tc.arg, err)
		}
	}
}
