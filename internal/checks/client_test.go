// SPDX-License-Identifier: AGPL-3.0-or-later

package checks

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/api/fakeapi"
)

func TestListForRef(t *testing.T) {
	srv := fakeapi.New(t)
	srv.Handle(http.MethodGet, "/api/v1/repos/o/r/commits/abc/check-runs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(CheckRunsResponse{
			TotalCount: 2,
			CheckRuns: []CheckRun{
				{ID: 1, Name: "build", Status: "completed", Conclusion: "success", Required: true},
				{ID: 2, Name: "lint", Status: "in_progress"},
			},
		})
	})

	c := NewClient(srv.NewClient())
	out, err := c.ListForRef(context.Background(), "o", "r", "abc")
	if err != nil {
		t.Fatalf("ListForRef: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("len: %d", len(out))
	}
	if !out[0].IsCompleted() || !out[0].IsSuccess() {
		t.Errorf("build state wrong: %+v", out[0])
	}
	if out[1].IsCompleted() {
		t.Errorf("lint should be in-progress: %+v", out[1])
	}
}

func TestIsFailureSemantics(t *testing.T) {
	cases := []struct {
		status, conclusion string
		failure            bool
	}{
		{"completed", "failure", true},
		{"completed", "timed_out", true},
		{"completed", "success", false},
		{"completed", "skipped", false},
		{"completed", "neutral", false},
		{"in_progress", "", false},
		{"queued", "", false},
	}
	for _, tc := range cases {
		r := CheckRun{Status: tc.status, Conclusion: tc.conclusion}
		if r.IsFailure() != tc.failure {
			t.Errorf("(%s,%s): IsFailure = %v want %v", tc.status, tc.conclusion, r.IsFailure(), tc.failure)
		}
	}
}
