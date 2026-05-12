// SPDX-License-Identifier: AGPL-3.0-or-later

package prompter

import (
	"errors"
	"fmt"

	survey "github.com/AlecAivazis/survey/v2"

	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
)

// surveyPrompter is the production implementation of Prompter backed by
// AlecAivazis/survey/v2. It wires survey to the supplied IOStreams so
// prompts honor color settings + go to the same stderr the rest of the
// CLI uses.
type surveyPrompter struct {
	ios *iostreams.IOStreams
}

// NewSurvey returns a survey/v2-backed Prompter. When ios.NeverPrompt()
// is true every prompt method returns NotInteractive — useful in tests
// that exercise command builders without exposing them to survey's
// terminal raw-mode handling.
func NewSurvey(ios *iostreams.IOStreams) Prompter {
	return &surveyPrompter{ios: ios}
}

func (p *surveyPrompter) refuseIfNonInteractive(method string) error {
	if p.ios.NeverPrompt() {
		return NotInteractive{Reason: fmt.Sprintf("%s called in non-interactive context", method)}
	}
	return nil
}

// Confirm satisfies Prompter.
func (p *surveyPrompter) Confirm(message string, defaultValue bool) (bool, error) {
	if err := p.refuseIfNonInteractive("Confirm"); err != nil {
		return defaultValue, err
	}
	var answer bool
	prompt := &survey.Confirm{Message: message, Default: defaultValue}
	if err := survey.AskOne(prompt, &answer); err != nil {
		return defaultValue, translateSurveyError(err)
	}
	return answer, nil
}

// Input satisfies Prompter.
func (p *surveyPrompter) Input(message, defaultValue string) (string, error) {
	if err := p.refuseIfNonInteractive("Input"); err != nil {
		return defaultValue, err
	}
	var answer string
	prompt := &survey.Input{Message: message, Default: defaultValue}
	if err := survey.AskOne(prompt, &answer); err != nil {
		return defaultValue, translateSurveyError(err)
	}
	return answer, nil
}

// Password satisfies Prompter.
func (p *surveyPrompter) Password(message string) (string, error) {
	if err := p.refuseIfNonInteractive("Password"); err != nil {
		return "", err
	}
	var answer string
	prompt := &survey.Password{Message: message}
	if err := survey.AskOne(prompt, &answer); err != nil {
		return "", translateSurveyError(err)
	}
	return answer, nil
}

// Select satisfies Prompter.
func (p *surveyPrompter) Select(message, defaultValue string, options []string) (int, error) {
	if err := p.refuseIfNonInteractive("Select"); err != nil {
		return -1, err
	}
	prompt := &survey.Select{Message: message, Options: options, Default: defaultValue}
	var idx int
	if err := survey.AskOne(prompt, &idx); err != nil {
		return -1, translateSurveyError(err)
	}
	return idx, nil
}

// MultiSelect satisfies Prompter.
func (p *surveyPrompter) MultiSelect(message string, defaults, options []string) ([]int, error) {
	if err := p.refuseIfNonInteractive("MultiSelect"); err != nil {
		return nil, err
	}
	prompt := &survey.MultiSelect{Message: message, Options: options, Default: defaults}
	var idxs []int
	if err := survey.AskOne(prompt, &idxs); err != nil {
		return nil, translateSurveyError(err)
	}
	return idxs, nil
}

// translateSurveyError maps survey's package-level abort sentinel to our
// typed Aborted error so callers can use errors.As cleanly.
func translateSurveyError(err error) error {
	if errors.Is(err, terminalInterruptErr) {
		return Aborted{}
	}
	return err
}

// terminalInterruptErr is the sentinel survey returns for Ctrl-C. Read
// from the survey/terminal subpackage via surveyInterruptErr (defined in
// survey_interrupt.go) to keep the import boundary explicit.
var terminalInterruptErr = surveyInterruptErr()
