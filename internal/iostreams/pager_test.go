// SPDX-License-Identifier: AGPL-3.0-or-later

package iostreams

import (
	"errors"
	"os"
	"testing"
)

// unsetEnv removes an env var for the test duration. t.Setenv can only
// set values; for the pager logic an empty value is meaningful (it means
// "disable paging"), so we need real unset semantics.
func unsetEnv(t *testing.T, key string) {
	t.Helper()
	prev, hadIt := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unsetenv %s: %v", key, err)
	}
	t.Cleanup(func() {
		if hadIt {
			_ = os.Setenv(key, prev)
		}
	})
}

// clearPagerEnv removes all pager-related env vars so each test starts
// from a known "no env set" baseline.
func clearPagerEnv(t *testing.T) {
	t.Helper()
	unsetEnv(t, EnvPager)
	unsetEnv(t, EnvPagerGeneric)
}

func TestResolvePagerNonTTYSkips(t *testing.T) {
	clearPagerEnv(t)
	s, _, _, _ := Test()
	_, ok := s.ResolvePager("")
	if ok {
		t.Error("non-TTY streams should skip paging")
	}
}

func TestResolvePagerDisabledByFlag(t *testing.T) {
	clearPagerEnv(t)
	s, _, _, _ := Test()
	s.SetStdoutTTY(true)
	s.SetPagerDisabled(true)
	_, ok := s.ResolvePager("")
	if ok {
		t.Error("pager-disabled should skip paging")
	}
}

func TestResolvePagerPrecedence(t *testing.T) {
	cases := []struct {
		name        string
		configPager string
		env         map[string]string
		wantCmd     string // first token; empty means "skipped"
	}{
		{
			name:        "config wins over env",
			configPager: "myconfigpager",
			env:         map[string]string{EnvPager: "envpager", EnvPagerGeneric: "vanilla"},
			wantCmd:     "myconfigpager",
		},
		{
			name:    "SHITHUB_PAGER wins over PAGER",
			env:     map[string]string{EnvPager: "shpager", EnvPagerGeneric: "vanilla"},
			wantCmd: "shpager",
		},
		{
			name:    "PAGER used when SHITHUB_PAGER absent",
			env:     map[string]string{EnvPagerGeneric: "vanilla"},
			wantCmd: "vanilla",
		},
		{
			name:    "empty SHITHUB_PAGER disables",
			env:     map[string]string{EnvPager: ""},
			wantCmd: "",
		},
		{
			name:    "empty PAGER disables",
			env:     map[string]string{EnvPagerGeneric: ""},
			wantCmd: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearPagerEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			s, _, _, _ := Test()
			s.SetStdoutTTY(true)

			cmd, ok := s.ResolvePager(tc.configPager)
			if tc.wantCmd == "" {
				// "Empty env var → skip" only fires when the env IS set to "".
				// Detect that case by checking the env map literally.
				if _, present := tc.env[EnvPager]; present && tc.env[EnvPager] == "" {
					if ok {
						t.Errorf("empty SHITHUB_PAGER should skip; got %v", cmd)
					}
					return
				}
				if _, present := tc.env[EnvPagerGeneric]; present && tc.env[EnvPagerGeneric] == "" {
					if ok {
						t.Errorf("empty PAGER should skip; got %v", cmd)
					}
					return
				}
				if ok {
					t.Errorf("expected skip; got %v", cmd)
				}
				return
			}
			if !ok || len(cmd) == 0 {
				t.Fatalf("expected pager %q, got skip", tc.wantCmd)
			}
			if cmd[0] != tc.wantCmd {
				t.Errorf("first token: want %q got %q", tc.wantCmd, cmd[0])
			}
		})
	}
}

func TestResolvePagerDefaultUnix(t *testing.T) {
	clearPagerEnv(t)
	s, _, _, _ := Test()
	s.SetStdoutTTY(true)

	cmd, ok := s.ResolvePager("")
	if !ok {
		t.Fatal("expected default pager")
	}
	// The platform branch is GOOS-dependent; the default on the build host
	// is what matters. Both candidates have a stable first token.
	if cmd[0] != "less" && cmd[0] != "more" {
		t.Errorf("default pager first token: got %q", cmd[0])
	}
}

func TestSplitPager(t *testing.T) {
	cases := map[string][]string{
		"":            nil,
		"  ":          nil,
		"less":        {"less"},
		"less -FRX":   {"less", "-FRX"},
		"  less  -R ": {"less", "-R"},
	}
	for in, want := range cases {
		got := splitPager(in)
		if len(got) != len(want) {
			t.Errorf("splitPager(%q): len want %d got %d", in, len(want), len(got))
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("splitPager(%q)[%d]: want %q got %q", in, i, want[i], got[i])
			}
		}
	}
}

func TestBrokenPipeWriterSwallowsEPIPE(t *testing.T) {
	t.Parallel()
	underlying := &errWriter{err: errors.New("write |1: broken pipe")}
	bpw := &brokenPipeWriter{w: underlying}

	n, err := bpw.Write([]byte("hello"))
	if err != nil {
		t.Errorf("EPIPE should be swallowed, got: %v", err)
	}
	if n != len("hello") {
		t.Errorf("Write should report full write on EPIPE; got %d", n)
	}
}

func TestBrokenPipeWriterPropagatesOther(t *testing.T) {
	t.Parallel()
	underlying := &errWriter{err: errors.New("disk full")}
	bpw := &brokenPipeWriter{w: underlying}

	if _, err := bpw.Write([]byte("hi")); err == nil {
		t.Error("non-EPIPE error should propagate")
	}
}

func TestIsBrokenPipe(t *testing.T) {
	if !isBrokenPipe(errors.New("write |1: broken pipe")) {
		t.Error("should detect broken pipe")
	}
	if !isBrokenPipe(errors.New("syscall: EPIPE")) {
		t.Error("should detect EPIPE")
	}
	if isBrokenPipe(errors.New("connection refused")) {
		t.Error("should not match unrelated errors")
	}
	if isBrokenPipe(nil) {
		t.Error("nil error should not match")
	}
}

// errWriter is a minimal io.Writer that always returns its configured
// error. Used to exercise brokenPipeWriter's error-translation path
// without spawning a real pager process.
type errWriter struct {
	err error
}

func (e *errWriter) Write(_ []byte) (int, error) {
	return 0, e.err
}
