// SPDX-License-Identifier: AGPL-3.0-or-later

package auth_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/auth"
)

func TestReadCredentialRequestParses(t *testing.T) {
	t.Parallel()
	in := strings.NewReader("protocol=https\nhost=shithub.sh\npath=owner/repo\n\n")
	req, err := auth.ReadCredentialRequest(in)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if req.Protocol != "https" {
		t.Errorf("Protocol: got %q", req.Protocol)
	}
	if req.Host != "shithub.sh" {
		t.Errorf("Host: got %q", req.Host)
	}
	if req.Path != "owner/repo" {
		t.Errorf("Path: got %q", req.Path)
	}
	if req.Raw["protocol"] != "https" {
		t.Errorf("Raw[protocol]: got %q", req.Raw["protocol"])
	}
}

func TestReadCredentialRequestEmpty(t *testing.T) {
	t.Parallel()
	in := strings.NewReader("\n")
	req, err := auth.ReadCredentialRequest(in)
	if err != nil {
		t.Fatalf("read empty: %v", err)
	}
	if req.Host != "" {
		t.Errorf("empty request should have empty host, got %q", req.Host)
	}
}

func TestWriteCredentialResponseHappy(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := auth.WriteCredentialResponse(&buf, "https", "mf", "shithub_pat_abc"); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := buf.String()
	wantContains := []string{
		"protocol=https\n",
		"username=mf\n",
		"password=shithub_pat_abc\n",
	}
	for _, w := range wantContains {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in output:\n%s", w, got)
		}
	}
	if !strings.HasSuffix(got, "\n\n") {
		t.Errorf("response should end with blank line, got: %q", got)
	}
}

// TestWriteCredentialResponseEchoesProtocol covers audit #151: the
// helper response must echo the protocol from the request rather than
// always emitting "https". A non-https request (rare but legal) should
// produce a matching response.
func TestWriteCredentialResponseEchoesProtocol(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := auth.WriteCredentialResponse(&buf, "http", "mf", "x"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if !strings.Contains(buf.String(), "protocol=http\n") {
		t.Errorf("expected protocol=http; got %q", buf.String())
	}
	if strings.Contains(buf.String(), "protocol=https\n") {
		t.Errorf("response should not have https when request was http: %q", buf.String())
	}
}

// TestWriteCredentialResponseDefaultsToHttps confirms the back-compat
// fallback: callers that pass an empty protocol still get an `https`
// response (the prior hard-coded behavior).
func TestWriteCredentialResponseDefaultsToHttps(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := auth.WriteCredentialResponse(&buf, "", "mf", "x"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if !strings.Contains(buf.String(), "protocol=https\n") {
		t.Errorf("empty protocol should default to https; got %q", buf.String())
	}
}

func TestWriteCredentialResponseEmptyFallsThrough(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	// Empty username → emit nothing so git falls through to its next helper.
	if err := auth.WriteCredentialResponse(&buf, "https", "", "shithub_pat_x"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("empty username should emit nothing, got %q", buf.String())
	}

	buf.Reset()
	if err := auth.WriteCredentialResponse(&buf, "https", "mf", ""); err != nil {
		t.Fatalf("write: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("empty token should emit nothing, got %q", buf.String())
	}
}
