// SPDX-License-Identifier: AGPL-3.0-or-later

package prompter

import "github.com/AlecAivazis/survey/v2/terminal"

// surveyInterruptErr returns the survey/terminal package's interrupt
// sentinel. Kept in its own file so survey/terminal's import only loads
// the package boundary needed for this one constant.
func surveyInterruptErr() error { return terminal.InterruptErr }
