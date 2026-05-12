// SPDX-License-Identifier: AGPL-3.0-or-later

package prompter

import (
	"errors"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
)

// TestSurveyNeverPromptReturnsNotInteractive locks the substrate's
// promise: tests using iostreams.Test() can build a survey prompter
// without it ever attempting to read raw stdin.
func TestSurveyNeverPromptReturnsNotInteractive(t *testing.T) {
	t.Parallel()
	ios, _, _, _ := iostreams.Test()
	p := NewSurvey(ios)

	cases := []struct {
		name string
		fn   func() error
	}{
		{"Confirm", func() error { _, err := p.Confirm("x", false); return err }},
		{"Input", func() error { _, err := p.Input("x", ""); return err }},
		{"Password", func() error { _, err := p.Password("x"); return err }},
		{"Select", func() error { _, err := p.Select("x", "", []string{"a"}); return err }},
		{"MultiSelect", func() error { _, err := p.MultiSelect("x", nil, []string{"a"}); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.fn()
			if err == nil {
				t.Fatal("expected NotInteractive")
			}
			var ni NotInteractive
			if !errors.As(err, &ni) {
				t.Errorf("expected NotInteractive, got %T: %v", err, err)
			}
		})
	}
}

func TestSurveyInterruptTranslatesToAborted(t *testing.T) {
	t.Parallel()
	if err := translateSurveyError(surveyInterruptErr()); err == nil {
		t.Fatal("interrupt should translate to non-nil error")
	} else {
		var ab Aborted
		if !errors.As(err, &ab) {
			t.Errorf("interrupt should translate to Aborted, got %T", err)
		}
	}
}

func TestSurveyOtherErrorPassesThrough(t *testing.T) {
	t.Parallel()
	myErr := errors.New("disk full")
	if got := translateSurveyError(myErr); got != myErr {
		t.Errorf("unrelated error should pass through, got %v", got)
	}
}
