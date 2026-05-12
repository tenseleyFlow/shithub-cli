// SPDX-License-Identifier: AGPL-3.0-or-later

package iostreams

import (
	"strings"
	"testing"
)

// clearColorEnv removes every color-related env var so the table cases
// below start from a known-clean baseline. We use Unsetenv-with-cleanup
// rather than t.Setenv("") because some keys (CLICOLOR=0) have an explicit
// disable-on-empty semantic that would muddy the table.
func clearColorEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{EnvNoColor, EnvCLIColor, EnvCLIColorForce, EnvForceTTY, EnvTerm, EnvColorTerm} {
		unsetEnv(t, k)
	}
}

func TestResolveColorTable(t *testing.T) {
	cases := []struct {
		name        string
		env         map[string]string
		stdoutTTY   bool
		wantEnabled bool
		want256     bool
		wantTrue    bool
	}{
		{
			name:        "TTY only enables color",
			stdoutTTY:   true,
			wantEnabled: true,
		},
		{
			name:        "non-TTY default off",
			stdoutTTY:   false,
			wantEnabled: false,
		},
		{
			name:        "NO_COLOR forces off even with TTY",
			env:         map[string]string{EnvNoColor: "1"},
			stdoutTTY:   true,
			wantEnabled: false,
		},
		{
			name:        "CLICOLOR=0 forces off",
			env:         map[string]string{EnvCLIColor: "0"},
			stdoutTTY:   true,
			wantEnabled: false,
		},
		{
			name:        "CLICOLOR_FORCE on without TTY",
			env:         map[string]string{EnvCLIColorForce: "1"},
			stdoutTTY:   false,
			wantEnabled: true,
		},
		{
			name:        "NO_COLOR wins over CLICOLOR_FORCE",
			env:         map[string]string{EnvNoColor: "1", EnvCLIColorForce: "1"},
			stdoutTTY:   true,
			wantEnabled: false,
		},
		{
			name:        "COLORTERM=truecolor implies 256+true",
			env:         map[string]string{EnvColorTerm: "truecolor"},
			stdoutTTY:   true,
			wantEnabled: true,
			want256:     true,
			wantTrue:    true,
		},
		{
			name:        "TERM=xterm-256color implies 256",
			env:         map[string]string{EnvTerm: "xterm-256color"},
			stdoutTTY:   true,
			wantEnabled: true,
			want256:     true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearColorEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			enabled, is256, isTrue := resolveColor(tc.stdoutTTY)
			if enabled != tc.wantEnabled {
				t.Errorf("enabled: want %v got %v", tc.wantEnabled, enabled)
			}
			if is256 != tc.want256 {
				t.Errorf("is256: want %v got %v", tc.want256, is256)
			}
			if isTrue != tc.wantTrue {
				t.Errorf("isTrue: want %v got %v", tc.wantTrue, isTrue)
			}
		})
	}
}

// TestTestFactory ensures the Test() factory wires up buffers and disables
// interactive prompts; every test in later sprints relies on this contract.
func TestTestFactory(t *testing.T) {
	t.Parallel()

	streams, in, out, errOut := Test()

	if streams.IsStdoutTTY() {
		t.Error("Test() stdout should report non-TTY")
	}
	if streams.IsStderrTTY() {
		t.Error("Test() stderr should report non-TTY")
	}
	if streams.ColorEnabled() {
		t.Error("Test() should default color off")
	}
	if !streams.NeverPrompt() {
		t.Error("Test() should set NeverPrompt so interactive helpers fail fast")
	}

	// Buffer round-trip.
	in.WriteString("hello stdin")
	buf := make([]byte, 32)
	n, _ := streams.In.Read(buf)
	if string(buf[:n]) != "hello stdin" {
		t.Errorf("stdin round trip: got %q", buf[:n])
	}

	if _, err := streams.Out.Write([]byte("o\n")); err != nil {
		t.Fatalf("Out.Write: %v", err)
	}
	if _, err := streams.ErrOut.Write([]byte("e\n")); err != nil {
		t.Fatalf("ErrOut.Write: %v", err)
	}
	if !strings.Contains(out.String(), "o") {
		t.Errorf("out buf: got %q", out.String())
	}
	if !strings.Contains(errOut.String(), "e") {
		t.Errorf("errOut buf: got %q", errOut.String())
	}
}

func TestSetColorEnabledOverride(t *testing.T) {
	t.Parallel()
	s, _, _, _ := Test()
	s.SetColorEnabled(true)
	if !s.ColorEnabled() {
		t.Error("SetColorEnabled(true) should flip the flag")
	}
}

func TestSetStdoutTTYOverride(t *testing.T) {
	t.Parallel()
	s, _, _, _ := Test()
	s.SetStdoutTTY(true)
	if !s.IsStdoutTTY() {
		t.Error("SetStdoutTTY(true) should flip the flag")
	}
}

func TestTerminalWidthFallback(t *testing.T) {
	t.Parallel()
	s, _, _, _ := Test()
	if got := s.TerminalWidth(); got != DefaultTerminalWidth {
		t.Errorf("TerminalWidth on non-TTY: want %d got %d", DefaultTerminalWidth, got)
	}
}

func TestEnvBool(t *testing.T) {
	cases := map[string]bool{
		"":          false,
		"0":         false,
		"1":         true,
		"true":      true,
		"false":     false,
		"yes":       true,
		"truecolor": true,
		"24bit":     true,
	}
	for v, want := range cases {
		t.Run(v, func(t *testing.T) {
			t.Setenv("SHITHUB_TEST_BOOL", v)
			if got := envBool("SHITHUB_TEST_BOOL"); got != want {
				t.Errorf("envBool(%q): want %v got %v", v, want, got)
			}
		})
	}
}
