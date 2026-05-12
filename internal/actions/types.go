// SPDX-License-Identifier: AGPL-3.0-or-later

// Package actions owns the typed wire shape and helper client for
// shithub's GitHub-Actions-compatible /actions surface (shithub S41).
// Endpoints referenced as "(S41g)" are server-pending — the CLI stubs
// the commands that hit them in pkg/cmd/{workflow,run,cache} so the
// surface is discoverable without depending on parked work.
package actions

import (
	"time"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
)

// Workflow is one entry from /repos/{owner}/{repo}/actions/workflows.
type Workflow struct {
	ID        int64     `json:"id"`
	NodeID    string    `json:"node_id,omitempty"`
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	State     string    `json:"state"` // "active" | "disabled_manually" | "disabled_inactivity"
	URL       string    `json:"url,omitempty"`
	HTMLURL   string    `json:"html_url,omitempty"`
	BadgeURL  string    `json:"badge_url,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// IsActive reports whether the workflow is currently runnable.
func (w Workflow) IsActive() bool { return w.State == "active" }

// WorkflowsResponse is the paginated list envelope.
type WorkflowsResponse struct {
	TotalCount int        `json:"total_count"`
	Workflows  []Workflow `json:"workflows"`
}

// DispatchInput is the body for POST /actions/workflows/{id}/dispatches.
// Ref is the branch or tag to dispatch against; Inputs are the
// workflow_dispatch input values (S41a validates against the workflow's
// declared inputs schema).
type DispatchInput struct {
	Ref    string         `json:"ref"`
	Inputs map[string]any `json:"inputs,omitempty"`
}

// WorkflowRun is one entry from /repos/.../actions/runs.
type WorkflowRun struct {
	ID              int64      `json:"id"`
	Name            string     `json:"name,omitempty"`
	NodeID          string     `json:"node_id,omitempty"`
	HeadBranch      string     `json:"head_branch,omitempty"`
	HeadSHA         string     `json:"head_sha,omitempty"`
	Path            string     `json:"path,omitempty"`
	DisplayTitle    string     `json:"display_title,omitempty"`
	Status          string     `json:"status"`               // "queued"|"in_progress"|"completed"|...
	Conclusion      string     `json:"conclusion,omitempty"` // "success"|"failure"|"cancelled"|"skipped"|...
	WorkflowID      int64      `json:"workflow_id"`
	RunNumber       int        `json:"run_number"`
	RunAttempt      int        `json:"run_attempt,omitempty"`
	Event           string     `json:"event,omitempty"`
	Actor           *api.User  `json:"actor,omitempty"`
	TriggeringActor *api.User  `json:"triggering_actor,omitempty"`
	URL             string     `json:"url,omitempty"`
	HTMLURL         string     `json:"html_url,omitempty"`
	JobsURL         string     `json:"jobs_url,omitempty"`
	LogsURL         string     `json:"logs_url,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	RunStartedAt    *time.Time `json:"run_started_at,omitempty"`
}

// IsCompleted reports whether the run has reached a terminal state.
func (r WorkflowRun) IsCompleted() bool { return r.Status == "completed" }

// IsFailure reports whether the run finished with a non-success conclusion.
// Cancelled and timed_out are surfaced as failures for --exit-status.
func (r WorkflowRun) IsFailure() bool {
	if !r.IsCompleted() {
		return false
	}
	switch r.Conclusion {
	case "success", "skipped", "neutral":
		return false
	}
	return true
}

// RunsResponse is the paginated list envelope.
type RunsResponse struct {
	TotalCount   int           `json:"total_count"`
	WorkflowRuns []WorkflowRun `json:"workflow_runs"`
}

// Job is one job within a workflow run.
type Job struct {
	ID          int64      `json:"id"`
	RunID       int64      `json:"run_id"`
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Conclusion  string     `json:"conclusion,omitempty"`
	URL         string     `json:"url,omitempty"`
	HTMLURL     string     `json:"html_url,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Steps       []Step     `json:"steps,omitempty"`
	RunnerName  string     `json:"runner_name,omitempty"`
	RunnerGroup string     `json:"runner_group_name,omitempty"`
}

// Step is one step within a Job.
type Step struct {
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Conclusion  string     `json:"conclusion,omitempty"`
	Number      int        `json:"number"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// JobsResponse is the paginated jobs envelope.
type JobsResponse struct {
	TotalCount int   `json:"total_count"`
	Jobs       []Job `json:"jobs"`
}

// Artifact is one entry from /actions/runs/{id}/artifacts.
type Artifact struct {
	ID                 int64      `json:"id"`
	NodeID             string     `json:"node_id,omitempty"`
	Name               string     `json:"name"`
	SizeInBytes        int64      `json:"size_in_bytes"`
	URL                string     `json:"url,omitempty"`
	ArchiveDownloadURL string     `json:"archive_download_url"`
	Expired            bool       `json:"expired,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	ExpiresAt          *time.Time `json:"expires_at,omitempty"`
	WorkflowRun        *struct {
		ID         int64  `json:"id"`
		HeadBranch string `json:"head_branch"`
		HeadSHA    string `json:"head_sha"`
	} `json:"workflow_run,omitempty"`
}

// ArtifactsResponse is the paginated artifacts envelope.
type ArtifactsResponse struct {
	TotalCount int        `json:"total_count"`
	Artifacts  []Artifact `json:"artifacts"`
}

// Cache is one entry from /actions/caches.
type Cache struct {
	ID             int64     `json:"id"`
	Ref            string    `json:"ref,omitempty"`
	Key            string    `json:"key,omitempty"`
	Version        string    `json:"version,omitempty"`
	LastAccessedAt time.Time `json:"last_accessed_at,omitempty"`
	CreatedAt      time.Time `json:"created_at,omitempty"`
	SizeInBytes    int64     `json:"size_in_bytes,omitempty"`
}

// CachesResponse is the paginated caches envelope.
type CachesResponse struct {
	TotalCount    int     `json:"total_count"`
	ActionsCaches []Cache `json:"actions_caches"`
}

// RunListOptions narrows /actions/runs queries.
type RunListOptions struct {
	WorkflowFile string // "ci.yml" or workflow ID stringified
	Branch       string
	Event        string
	Actor        string
	Status       string // queued | in_progress | completed | success | failure | ...
	PerPage      int
	Limit        int
}

// CacheListOptions narrows /actions/caches queries.
type CacheListOptions struct {
	Key     string
	Ref     string
	Sort    string // "created_at" | "last_accessed_at" | "size_in_bytes"
	Order   string // "asc" | "desc"
	PerPage int
	Limit   int
}
