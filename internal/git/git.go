// SPDX-License-Identifier: AGPL-3.0-or-later

package git

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/cli/safeexec"
)

// Runner abstracts the `git` binary so tests can inject a fake without
// requiring git on the test runner's PATH. The production implementation
// is FromPath which resolves git via safeexec.
type Runner interface {
	// Run executes git with the given args from dir (empty = cwd).
	// stdout/stderr are streamed to the caller-provided writers; pass
	// io.Discard to drop output. Returns the process exit error.
	Run(dir string, args []string, stdout, stderr io.Writer) error

	// Output runs git with the given args from dir and returns captured
	// stdout. stderr is discarded on success and wrapped into the error
	// on failure so callers can surface git's diagnostics.
	Output(dir string, args ...string) ([]byte, error)
}

// FromPath locates `git` via safeexec and returns a Runner that shells
// out to it. Returns an error when git is not on PATH so the caller can
// fall back gracefully (some commands work API-only).
func FromPath() (Runner, error) {
	bin, err := safeexec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("git: locate binary: %w", err)
	}
	return &pathRunner{bin: bin}, nil
}

// MustFromPath panics if git is not on PATH. Use only at process startup
// or in tests where a missing git binary is an unrecoverable condition.
func MustFromPath() Runner {
	r, err := FromPath()
	if err != nil {
		panic(err)
	}
	return r
}

type pathRunner struct{ bin string }

func (p *pathRunner) Run(dir string, args []string, stdout, stderr io.Writer) error {
	cmd := exec.Command(p.bin, args...) //nolint:gosec // bin from safeexec; args trusted callers
	if dir != "" {
		cmd.Dir = dir
	}
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func (p *pathRunner) Output(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command(p.bin, args...) //nolint:gosec // bin from safeexec; args trusted callers
	if dir != "" {
		cmd.Dir = dir
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return out, err
		}
		return out, fmt.Errorf("%w: %s", err, msg)
	}
	return out, nil
}

// IsRepo reports whether dir (empty = cwd) is inside a git working tree.
// Returns false (no error) when git is on PATH but dir is not a repo;
// returns an error only when git itself is missing.
func IsRepo(r Runner, dir string) (bool, error) {
	if r == nil {
		return false, fmt.Errorf("git: nil runner")
	}
	_, err := r.Output(dir, "rev-parse", "--is-inside-work-tree")
	if err == nil {
		return true, nil
	}
	// Any error from git here means "not a repo"; we don't try to
	// distinguish "not a git dir" from "permission denied" because
	// callers don't care for the cases we use this from.
	return false, nil
}

// CurrentBranch returns the symbolic short-name of HEAD, or an error
// when HEAD is detached. Empty dir means cwd.
func CurrentBranch(r Runner, dir string) (string, error) {
	out, err := r.Output(dir, "symbolic-ref", "--short", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git: current branch: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// IsClean reports whether the working tree has zero unstaged or untracked
// changes. The implementation uses `status --porcelain` and treats any
// non-empty output as "dirty".
func IsClean(r Runner, dir string) (bool, error) {
	out, err := r.Output(dir, "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("git: status: %w", err)
	}
	return strings.TrimSpace(string(out)) == "", nil
}

// Init runs `git init` in dir, creating it if missing. Default branch is
// passed via -b so the very first commit lands on `trunk` (shithub's
// default) when callers ask for it.
func Init(r Runner, dir, defaultBranch string) error {
	if dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("git: mkdir %q: %w", dir, err)
	}
	args := []string{"init"}
	if defaultBranch != "" {
		args = append(args, "-b", defaultBranch)
	}
	args = append(args, dir)
	return r.Run("", args, io.Discard, os.Stderr)
}

// Clone executes `git clone <url> [dir] -- <extra>...` and streams output
// to stdout/stderr from the iostreams package. Returns the resolved local
// directory path so callers can chdir / open files inside it.
func Clone(r Runner, url, dir string, extra []string, stdout, stderr io.Writer) (string, error) {
	args := []string{"clone", url}
	if dir != "" {
		args = append(args, dir)
	}
	if len(extra) > 0 {
		args = append(args, extra...)
	}
	if err := r.Run("", args, stdout, stderr); err != nil {
		return "", fmt.Errorf("git clone: %w", err)
	}
	if dir != "" {
		return dir, nil
	}
	// `git clone` derives the local dir from the URL's basename minus .git.
	return dirFromCloneURL(url), nil
}

// dirFromCloneURL replicates git's own basename logic for the case where
// the caller didn't pass an explicit dir to Clone. Strips trailing slashes
// and the `.git` suffix, then takes the last path segment.
func dirFromCloneURL(raw string) string {
	s := strings.TrimRight(raw, "/")
	s = strings.TrimSuffix(s, ".git")
	// SSH form "git@host:owner/repo" — split on ':' and take the tail.
	if i := strings.LastIndexByte(s, ':'); i >= 0 && !strings.Contains(s[i:], "//") {
		s = s[i+1:]
	}
	if i := strings.LastIndexByte(s, '/'); i >= 0 {
		s = s[i+1:]
	}
	return s
}

// AddRemote registers a new remote pointing at url. Returns an error if
// the remote already exists; use SetRemoteURL to overwrite.
func AddRemote(r Runner, dir, name, url string) error {
	if name == "" || url == "" {
		return fmt.Errorf("git: remote name and url required")
	}
	return r.Run(dir, []string{"remote", "add", name, url}, io.Discard, os.Stderr)
}

// RenameRemote renames an existing remote. Useful for repo fork's
// "origin → upstream" rotation.
func RenameRemote(r Runner, dir, oldName, newName string) error {
	return r.Run(dir, []string{"remote", "rename", oldName, newName}, io.Discard, os.Stderr)
}

// SetRemoteURL overwrites the URL of an existing remote.
func SetRemoteURL(r Runner, dir, name, url string) error {
	return r.Run(dir, []string{"remote", "set-url", name, url}, io.Discard, os.Stderr)
}

// RemoteExists reports whether a remote with the given name is configured
// in dir. False (no error) when the remote isn't there; error only when
// git itself can't run.
func RemoteExists(r Runner, dir, name string) (bool, error) {
	_, err := r.Output(dir, "config", "--get", "remote."+name+".url")
	if err == nil {
		return true, nil
	}
	// `git config --get` exits 1 when the key is missing; that's the
	// expected non-existence signal and not an error to propagate.
	return false, nil
}

// SetConfig writes a local `.git/config` value. Used by `repo set-default`
// to persist the resolved repo under `shithub.default-repo`.
func SetConfig(r Runner, dir, key, value string) error {
	return r.Run(dir, []string{"config", key, value}, io.Discard, os.Stderr)
}

// GetConfig reads a local `.git/config` value. Returns the empty string
// (no error) when the key isn't set.
func GetConfig(r Runner, dir, key string) (string, error) {
	out, err := r.Output(dir, "config", "--get", key)
	if err != nil {
		// Missing keys exit with status 1; treat as empty value.
		return "", nil
	}
	return strings.TrimSpace(string(out)), nil
}

// UnsetConfig removes a local `.git/config` key. Used by
// `repo set-default --unset`. Missing keys are not an error.
func UnsetConfig(r Runner, dir, key string) error {
	err := r.Run(dir, []string{"config", "--unset", key}, io.Discard, io.Discard)
	if err != nil {
		// `git config --unset` exits 5 when the section/key doesn't exist;
		// silently ignore so the operation is idempotent.
		return nil
	}
	return nil
}

// Fetch runs `git fetch <remote> [refspec...]` with output streamed to
// stderr (or the caller's writer when set). Used by `repo sync` when the
// server-side merge-upstream endpoint is unavailable.
func Fetch(r Runner, dir, remote string, refspecs []string, stdout, stderr io.Writer) error {
	args := []string{"fetch", remote}
	args = append(args, refspecs...)
	return r.Run(dir, args, stdout, stderr)
}

// MergeFastForward runs `git merge --ff-only <ref>`. Returns the wrapped
// error from git on a non-FF history; the caller surfaces a human message.
func MergeFastForward(r Runner, dir, ref string, stdout, stderr io.Writer) error {
	return r.Run(dir, []string{"merge", "--ff-only", ref}, stdout, stderr)
}
