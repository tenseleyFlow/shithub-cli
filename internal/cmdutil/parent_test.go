// SPDX-License-Identifier: AGPL-3.0-or-later

package cmdutil_test

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
)

// TestParentRunE_NoArgsPrintsHelp pins the bare-invocation path.
func TestParentRunE_NoArgsPrintsHelp(t *testing.T) {
	parent := &cobra.Command{Use: "shithub", RunE: cmdutil.ParentRunE()}
	parent.SetOut(&strings.Builder{})
	parent.SetErr(&strings.Builder{})
	parent.AddCommand(&cobra.Command{Use: "create"})
	if err := parent.RunE(parent, nil); err != nil {
		t.Fatalf("bare invoke should print help and return nil; got %v", err)
	}
}

// TestParentRunE_UnknownSubcommandErrors pins exit-code-1 path.
func TestParentRunE_UnknownSubcommandErrors(t *testing.T) {
	parent := &cobra.Command{Use: "parent", RunE: cmdutil.ParentRunE()}
	parent.AddCommand(&cobra.Command{Use: "create"})
	err := parent.RunE(parent, []string{"totally-unknown-name"})
	if err == nil {
		t.Fatal("unknown subcommand should return non-nil error")
	}
	if !strings.Contains(err.Error(), `unknown command "totally-unknown-name"`) {
		t.Errorf("error should quote the bad arg: %v", err)
	}
}

// TestParentRunE_LevenshteinSuggestion pins audit-I5: typos at the
// subcommand level get a "Did you mean?" line. Pre-fix only the root
// command's auto-error path had suggestions; once execution reached
// a parent's RunE, the suggestion machinery was bypassed.
func TestParentRunE_LevenshteinSuggestion(t *testing.T) {
	parent := &cobra.Command{Use: "pr", RunE: cmdutil.ParentRunE()}
	parent.AddCommand(&cobra.Command{Use: "close"})
	parent.AddCommand(&cobra.Command{Use: "create"})
	parent.AddCommand(&cobra.Command{Use: "review"})

	err := parent.RunE(parent, []string{"klose"})
	if err == nil {
		t.Fatal("expected error from typo")
	}
	if !strings.Contains(err.Error(), "Did you mean this?") {
		t.Errorf("missing suggestion preface: %v", err)
	}
	if !strings.Contains(err.Error(), "close") {
		t.Errorf("expected to suggest 'close': %v", err)
	}
}

// TestParentRunE_PrefixSuggestion: 3+ char prefix match wins over
// Levenshtein. `shithub repo cre` → `create`.
func TestParentRunE_PrefixSuggestion(t *testing.T) {
	parent := &cobra.Command{Use: "repo", RunE: cmdutil.ParentRunE()}
	parent.AddCommand(&cobra.Command{Use: "create"})
	parent.AddCommand(&cobra.Command{Use: "delete"})

	err := parent.RunE(parent, []string{"cre"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "create") {
		t.Errorf("prefix 'cre' should suggest 'create': %v", err)
	}
}

// TestParentRunE_NoCloseMatchSkipsSuggestion: a typo far from every
// child should NOT emit a misleading suggestion.
func TestParentRunE_NoCloseMatchSkipsSuggestion(t *testing.T) {
	parent := &cobra.Command{Use: "pr", RunE: cmdutil.ParentRunE()}
	parent.AddCommand(&cobra.Command{Use: "close"})
	parent.AddCommand(&cobra.Command{Use: "create"})

	err := parent.RunE(parent, []string{"absolutely-nothing-like-our-verbs"})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "Did you mean this?") {
		t.Errorf("should not suggest for far-off typo: %v", err)
	}
}

// TestParentRunE_HiddenChildrenIgnored: hidden subcommands don't
// pollute the suggestion candidate set.
func TestParentRunE_HiddenChildrenIgnored(t *testing.T) {
	parent := &cobra.Command{Use: "x", RunE: cmdutil.ParentRunE()}
	parent.AddCommand(&cobra.Command{Use: "visible"})
	parent.AddCommand(&cobra.Command{Use: "hidden-target", Hidden: true})

	err := parent.RunE(parent, []string{"hiddn-targt"})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "hidden-target") {
		t.Errorf("hidden subcommand should not be suggested: %v", err)
	}
}
