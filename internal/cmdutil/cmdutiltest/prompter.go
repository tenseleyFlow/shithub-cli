// SPDX-License-Identifier: AGPL-3.0-or-later

package cmdutiltest

import (
	"fmt"

	"github.com/tenseleyFlow/shithub-cli/internal/prompter"
)

// RecordingPrompter is a programmable prompter.Prompter for tests. Each
// prompt method consults its corresponding queue, popping the next pre-
// programmed answer. Unmatched prompts fail loudly so a test missing a
// queued answer crashes the test, not silently hangs or defaults.
type RecordingPrompter struct {
	confirms     []bool
	inputs       []string
	passwords    []string
	selects      []int
	multiSelects [][]int

	// History records every prompt the unit under test issued, so tests
	// can assert on the questions asked.
	History []PromptCall
}

// PromptCall is one recorded prompt invocation.
type PromptCall struct {
	Method  string // "Confirm", "Input", "Password", "Select", "MultiSelect"
	Message string
}

// NewRecordingPrompter returns a fresh prompter with empty queues.
func NewRecordingPrompter() *RecordingPrompter {
	return &RecordingPrompter{}
}

// QueueConfirm appends a Yes/No answer for the next Confirm call.
func (r *RecordingPrompter) QueueConfirm(answers ...bool) {
	r.confirms = append(r.confirms, answers...)
}

// QueueInput appends a free-form answer for the next Input call.
func (r *RecordingPrompter) QueueInput(answers ...string) { r.inputs = append(r.inputs, answers...) }

// QueuePassword appends a secret answer for the next Password call.
func (r *RecordingPrompter) QueuePassword(answers ...string) {
	r.passwords = append(r.passwords, answers...)
}

// QueueSelect appends an index for the next Select call.
func (r *RecordingPrompter) QueueSelect(indexes ...int) { r.selects = append(r.selects, indexes...) }

// QueueMultiSelect appends a slice of indexes for the next MultiSelect call.
func (r *RecordingPrompter) QueueMultiSelect(indexes []int) {
	r.multiSelects = append(r.multiSelects, indexes)
}

// Confirm satisfies prompter.Prompter.
func (r *RecordingPrompter) Confirm(message string, _ bool) (bool, error) {
	r.History = append(r.History, PromptCall{Method: "Confirm", Message: message})
	if len(r.confirms) == 0 {
		return false, fmt.Errorf("RecordingPrompter: no Confirm answer queued for %q", message)
	}
	ans := r.confirms[0]
	r.confirms = r.confirms[1:]
	return ans, nil
}

// Input satisfies prompter.Prompter.
func (r *RecordingPrompter) Input(message, _ string) (string, error) {
	r.History = append(r.History, PromptCall{Method: "Input", Message: message})
	if len(r.inputs) == 0 {
		return "", fmt.Errorf("RecordingPrompter: no Input answer queued for %q", message)
	}
	ans := r.inputs[0]
	r.inputs = r.inputs[1:]
	return ans, nil
}

// Password satisfies prompter.Prompter.
func (r *RecordingPrompter) Password(message string) (string, error) {
	r.History = append(r.History, PromptCall{Method: "Password", Message: message})
	if len(r.passwords) == 0 {
		return "", fmt.Errorf("RecordingPrompter: no Password answer queued for %q", message)
	}
	ans := r.passwords[0]
	r.passwords = r.passwords[1:]
	return ans, nil
}

// Select satisfies prompter.Prompter.
func (r *RecordingPrompter) Select(message, _ string, options []string) (int, error) {
	r.History = append(r.History, PromptCall{Method: "Select", Message: message})
	if len(r.selects) == 0 {
		return 0, fmt.Errorf("RecordingPrompter: no Select answer queued for %q (options=%v)", message, options)
	}
	ans := r.selects[0]
	r.selects = r.selects[1:]
	if ans < 0 || ans >= len(options) {
		return 0, fmt.Errorf("RecordingPrompter: queued Select index %d out of range for %d options", ans, len(options))
	}
	return ans, nil
}

// MultiSelect satisfies prompter.Prompter.
func (r *RecordingPrompter) MultiSelect(message string, _, options []string) ([]int, error) {
	r.History = append(r.History, PromptCall{Method: "MultiSelect", Message: message})
	if len(r.multiSelects) == 0 {
		return nil, fmt.Errorf("RecordingPrompter: no MultiSelect answer queued for %q (options=%v)", message, options)
	}
	ans := r.multiSelects[0]
	r.multiSelects = r.multiSelects[1:]
	for _, i := range ans {
		if i < 0 || i >= len(options) {
			return nil, fmt.Errorf("RecordingPrompter: queued MultiSelect index %d out of range for %d options", i, len(options))
		}
	}
	return ans, nil
}

// Compile-time assertion: RecordingPrompter satisfies prompter.Prompter.
var _ prompter.Prompter = (*RecordingPrompter)(nil)
