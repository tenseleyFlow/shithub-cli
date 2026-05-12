// SPDX-License-Identifier: AGPL-3.0-or-later

// Package checks owns the typed wire shape and helper client for
// shithub's /repos/{o}/{r}/commits/{sha}/check-runs endpoint (S24).
// Field tags mirror GitHub's Checks API 1:1.
package checks

import "time"

// CheckRun is one row in the check matrix. Status drives whether the
// run is queued / in_progress / completed; Conclusion is set only once
// Status == "completed".
type CheckRun struct {
	ID          int64      `json:"id"`
	NodeID      string     `json:"node_id,omitempty"`
	HeadSHA     string     `json:"head_sha"`
	Name        string     `json:"name"`
	Status      string     `json:"status"` // queued | in_progress | completed
	Conclusion  string     `json:"conclusion,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	DetailsURL  string     `json:"details_url,omitempty"`
	HTMLURL     string     `json:"html_url,omitempty"`
	ExternalID  string     `json:"external_id,omitempty"`
	App         *App       `json:"app,omitempty"`
	Required    bool       `json:"required,omitempty"`
	Output      *Output    `json:"output,omitempty"`
}

// IsCompleted reports whether the run reached a terminal state.
func (r CheckRun) IsCompleted() bool { return r.Status == "completed" }

// IsFailure reports whether the run completed with a non-success
// conclusion. "success" and "skipped" / "neutral" are treated as
// non-failures here; gh treats only success+skipped+neutral as passing
// for `--fail-fast` purposes. We mirror that.
func (r CheckRun) IsFailure() bool {
	if !r.IsCompleted() {
		return false
	}
	switch r.Conclusion {
	case "success", "neutral", "skipped":
		return false
	}
	return true
}

// IsSuccess reports whether the run completed without a problem.
func (r CheckRun) IsSuccess() bool {
	return r.IsCompleted() && (r.Conclusion == "success" || r.Conclusion == "neutral" || r.Conclusion == "skipped")
}

// App is the integration that produced the check run.
type App struct {
	ID    int64  `json:"id"`
	Slug  string `json:"slug"`
	Name  string `json:"name"`
	Owner *struct {
		Login string `json:"login"`
		Type  string `json:"type,omitempty"`
	} `json:"owner,omitempty"`
}

// Output is the optional summary/annotation payload on a check run.
type Output struct {
	Title       string `json:"title,omitempty"`
	Summary     string `json:"summary,omitempty"`
	Text        string `json:"text,omitempty"`
	Annotations int    `json:"annotations_count,omitempty"`
}

// CheckRunsResponse is the paginated envelope shithub returns for the
// check-runs endpoint. TotalCount is GitHub-compatible; the CLI mostly
// ignores it.
type CheckRunsResponse struct {
	TotalCount int        `json:"total_count"`
	CheckRuns  []CheckRun `json:"check_runs"`
}
