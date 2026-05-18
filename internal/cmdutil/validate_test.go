// SPDX-License-Identifier: AGPL-3.0-or-later

package cmdutil

import (
	"strings"
	"testing"
)

func TestValidateLimit(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in          int
		wantErr     bool
		wantSnippet string
	}{
		{1, false, ""},
		{30, false, ""},
		{999999, false, ""},
		{0, true, "invalid limit: 0"},
		{-1, true, "invalid limit: -1"},
		{-9999, true, "invalid limit: -9999"},
	}
	for _, tc := range cases {
		err := ValidateLimit(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("ValidateLimit(%d) err=%v, wantErr=%v", tc.in, err, tc.wantErr)
			continue
		}
		if tc.wantErr && !strings.Contains(err.Error(), tc.wantSnippet) {
			t.Errorf("ValidateLimit(%d) = %q, want substring %q", tc.in, err.Error(), tc.wantSnippet)
		}
	}
}
