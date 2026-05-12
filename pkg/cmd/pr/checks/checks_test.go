// SPDX-License-Identifier: AGPL-3.0-or-later

package checks

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	checksclient "github.com/tenseleyFlow/shithub-cli/internal/checks"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/pulls"
)

func TestChecksListSuccess(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/pulls/1", 200, pulls.PR{
		Number: 1, State: "open", Head: pulls.Ref{SHA: "abc"},
	})
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/commits/abc/check-runs", 200, checksclient.CheckRunsResponse{
		TotalCount: 2,
		CheckRuns: []checksclient.CheckRun{
			{Name: "build", Status: "completed", Conclusion: "success", Required: true},
			{Name: "lint", Status: "completed", Conclusion: "success"},
		},
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "1",
		Repo:        "o/r",
		Interval:    DefaultInterval,
		Timeout:     DefaultTimeout,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := tf.Out.String()
	for _, want := range []string{"build", "lint", "success"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q: %s", want, out)
		}
	}
}

func TestChecksFailureExitsNonZero(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/pulls/1", 200, pulls.PR{
		Number: 1, State: "open", Head: pulls.Ref{SHA: "abc"},
	})
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/commits/abc/check-runs", 200, checksclient.CheckRunsResponse{
		CheckRuns: []checksclient.CheckRun{
			{Name: "build", Status: "completed", Conclusion: "failure"},
		},
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "1",
		Repo:        "o/r",
		Interval:    DefaultInterval,
		Timeout:     DefaultTimeout,
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected non-nil error when checks failed")
	}
}

func TestChecksRequiredFilter(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/pulls/1", 200, pulls.PR{
		Number: 1, State: "open", Head: pulls.Ref{SHA: "abc"},
	})
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/commits/abc/check-runs", 200, checksclient.CheckRunsResponse{
		CheckRuns: []checksclient.CheckRun{
			{Name: "must-pass", Status: "completed", Conclusion: "success", Required: true},
			{Name: "optional", Status: "completed", Conclusion: "success", Required: false},
		},
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "1",
		Repo:        "o/r",
		Required:    true,
		Interval:    DefaultInterval,
		Timeout:     DefaultTimeout,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := tf.Out.String()
	if !strings.Contains(out, "must-pass") {
		t.Errorf("required check missing: %s", out)
	}
	if strings.Contains(out, "optional") {
		t.Errorf("optional check should be filtered: %s", out)
	}
}

func TestChecksWatchTransitions(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/pulls/1", 200, pulls.PR{
		Number: 1, State: "open", Head: pulls.Ref{SHA: "abc"},
	})

	var calls int32
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/commits/abc/check-runs", func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		switch n {
		case 1:
			_ = json.NewEncoder(w).Encode(checksclient.CheckRunsResponse{
				CheckRuns: []checksclient.CheckRun{{Name: "build", Status: "queued"}},
			})
		case 2:
			_ = json.NewEncoder(w).Encode(checksclient.CheckRunsResponse{
				CheckRuns: []checksclient.CheckRun{{Name: "build", Status: "in_progress"}},
			})
		default:
			_ = json.NewEncoder(w).Encode(checksclient.CheckRunsResponse{
				CheckRuns: []checksclient.CheckRun{{Name: "build", Status: "completed", Conclusion: "success"}},
			})
		}
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "1",
		Repo:        "o/r",
		Watch:       true,
		Interval:    20 * time.Millisecond,
		Timeout:     2 * time.Second,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if atomic.LoadInt32(&calls) < 3 {
		t.Errorf("expected >= 3 polls, got %d", calls)
	}
}

func TestChecksWatchFailFast(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/pulls/1", 200, pulls.PR{
		Number: 1, State: "open", Head: pulls.Ref{SHA: "abc"},
	})
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/commits/abc/check-runs", 200, checksclient.CheckRunsResponse{
		CheckRuns: []checksclient.CheckRun{
			{Name: "queued-check", Status: "queued"},
			{Name: "bad", Status: "completed", Conclusion: "failure"},
		},
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "1",
		Repo:        "o/r",
		Watch:       true,
		FailFast:    true,
		Interval:    20 * time.Millisecond,
		Timeout:     2 * time.Second,
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error from --fail-fast")
	}
}

func TestChecksJSONExport(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/pulls/1", 200, pulls.PR{
		Number: 1, State: "open", Head: pulls.Ref{SHA: "abc"},
	})
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/commits/abc/check-runs", 200, checksclient.CheckRunsResponse{
		CheckRuns: []checksclient.CheckRun{
			{Name: "build", Status: "completed", Conclusion: "success", HTMLURL: "https://x"},
		},
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "1",
		Repo:        "o/r",
		Interval:    DefaultInterval,
		Timeout:     DefaultTimeout,
	}
	opts.Exporter.JSONFields = "name,status,conclusion,link"
	opts.Exporter.JSONSet = true
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(tf.Out.String(), `"name":"build"`) {
		t.Errorf("json missing: %s", tf.Out.String())
	}
}
