// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build dev

package dev

import (
	"bytes"
	"strings"
	"testing"
)

func TestHelloHumanOutput(t *testing.T) {
	cmd := NewCmd()
	cmd.SetArgs([]string{"hello", "--subject", "shithub"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(buf.String(), "hello, shithub") {
		t.Errorf("human output: got %q", buf.String())
	}
}

func TestHelloJSONOutput(t *testing.T) {
	cmd := NewCmd()
	cmd.SetArgs([]string{"hello", "--subject", "shithub", "--json", "greeting,subject"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, `"greeting": "hello"`) {
		t.Errorf("JSON missing greeting: %q", got)
	}
	if !strings.Contains(got, `"subject": "shithub"`) {
		t.Errorf("JSON missing subject: %q", got)
	}
}

func TestHelloJQOutput(t *testing.T) {
	cmd := NewCmd()
	cmd.SetArgs([]string{"hello", "--subject", "x", "--json", "greeting", "-q", ".greeting"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if buf.String() != "hello\n" {
		t.Errorf("jq output: got %q", buf.String())
	}
}
