// SPDX-License-Identifier: AGPL-3.0-or-later

package secrets

import (
	"strings"
	"testing"
)

// TestValidateNameRejectsLowercase is the C-audit C25 regression:
// gh-compat secret-name rules forbid lowercase. The pre-D3d code
// accepted them silently.
func TestValidateNameRejectsLowercase(t *testing.T) {
	for _, bad := range []string{"lowercase", "Mixed_Case", "mostlyUpper"} {
		err := validateName(bad)
		if err == nil {
			t.Errorf("validateName(%q) should reject lowercase", bad)
		}
	}
}

// TestValidateNameRejectsLeadingDigit catches the "name can't start
// with a digit" rule documented in gh.
func TestValidateNameRejectsLeadingDigit(t *testing.T) {
	for _, bad := range []string{"1FOO", "9", "0_X"} {
		err := validateName(bad)
		if err == nil {
			t.Errorf("validateName(%q) should reject leading digit", bad)
		}
	}
}

// TestValidateNameRejectsGithubPrefix: GITHUB_* names are reserved.
func TestValidateNameRejectsGithubPrefix(t *testing.T) {
	err := validateName("GITHUB_TOKEN")
	if err == nil {
		t.Fatal("validateName should reject GITHUB_ prefix")
	}
	if !strings.Contains(err.Error(), "reserved") {
		t.Errorf("error should mention reserved; got: %v", err)
	}
}

// TestValidateNameAccepts: confirm the legal shapes still pass.
func TestValidateNameAccepts(t *testing.T) {
	for _, good := range []string{"FOO", "A", "_PRIVATE", "DEPLOY_KEY_42", "X1"} {
		if err := validateName(good); err != nil {
			t.Errorf("validateName(%q) should accept; got: %v", good, err)
		}
	}
}
