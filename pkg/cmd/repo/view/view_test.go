// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/repos"
)

func TestViewRendersRepo(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/octo/hello", 200, repos.Repo{
		Name:          "hello",
		FullName:      "octo/hello",
		Owner:         repos.Owner{Login: "octo"},
		Description:   "say hi",
		DefaultBranch: "trunk",
		Stargazers:    3,
		HTMLURL:       "https://shithub.sh/octo/hello",
		Topics:        []string{"go", "cli"},
	})
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/octo/hello/readme", 200, repos.README{
		Name:     "README.md",
		Encoding: "base64",
		Content:  base64.StdEncoding.EncodeToString([]byte("# Hello\n")),
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		RepoArg:     "octo/hello",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := tf.Out.String()
	for _, want := range []string{"octo/hello", "say hi", "default branch: trunk", "go, cli"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q; got: %s", want, out)
		}
	}
}

func TestViewWebOpensBrowser(t *testing.T) {
	tf := cmdutiltest.New(t)
	var opened string
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		RepoArg:     "octo/hello",
		Web:         true,
		Opener: func(url string) error {
			opened = url
			return nil
		},
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.HasSuffix(opened, "/octo/hello") {
		t.Errorf("expected web URL ending in /octo/hello, got %q", opened)
	}
	if len(tf.Server.Calls()) != 0 {
		t.Errorf("--web should skip API calls; got %d", len(tf.Server.Calls()))
	}
}

func TestViewJSONExport(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/octo/hello", 200, repos.Repo{
		Name:          "hello",
		FullName:      "octo/hello",
		Owner:         repos.Owner{Login: "octo"},
		DefaultBranch: "trunk",
		Stargazers:    42,
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		RepoArg:     "octo/hello",
	}
	opts.Exporter.JSONFields = "fullName,stargazers"
	opts.Exporter.JSONSet = true
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := tf.Out.String()
	if !strings.Contains(out, `"fullName":"octo/hello"`) || !strings.Contains(out, `"stargazers":42`) {
		t.Errorf("--json projection missing; got %s", out)
	}
}

func TestViewRejectsArgAndFlagCombo(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		RepoArg:     "a/b",
		Repo:        "c/d",
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error when both arg and -R present")
	}
}
