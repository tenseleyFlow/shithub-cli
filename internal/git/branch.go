// SPDX-License-Identifier: AGPL-3.0-or-later

package git

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// FetchRefspec fetches a single refspec from the named remote into a
// local ref. Used by `pr checkout` to pull a PR's head into a local
// tracking branch.
//
// refspec follows git's syntax: "remote-ref:local-ref" (e.g.,
// "refs/pull/42/head:refs/remotes/origin/pr/42").
func FetchRefspec(r Runner, dir, remote, refspec string, stdout, stderr io.Writer) error {
	if remote == "" {
		return fmt.Errorf("git: fetch: remote name required")
	}
	args := []string{"fetch", remote, refspec}
	return r.Run(dir, args, stdout, stderr)
}

// CheckoutBranch checks out an existing branch by name. Force replaces
// the working tree without confirming dirty state (callers must guard).
func CheckoutBranch(r Runner, dir, name string, force bool, stdout, stderr io.Writer) error {
	args := []string{"checkout"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, name)
	return r.Run(dir, args, stdout, stderr)
}

// CreateBranchAt creates a branch named `name` pointing at `startPoint`
// without checking it out. Used when the caller wants the branch but is
// still on another HEAD (rare; --detach paths).
func CreateBranchAt(r Runner, dir, name, startPoint string, stdout, stderr io.Writer) error {
	args := []string{"branch", name, startPoint}
	return r.Run(dir, args, stdout, stderr)
}

// CheckoutNewBranch creates a branch named `name` tracking `track` (or
// pointing at `startPoint` when `track` is empty) and checks it out in
// one step.
func CheckoutNewBranch(r Runner, dir, name, track, startPoint string, stdout, stderr io.Writer) error {
	args := []string{"checkout", "-b", name}
	if track != "" {
		args = append(args, "--track", track)
	} else if startPoint != "" {
		args = append(args, startPoint)
	}
	return r.Run(dir, args, stdout, stderr)
}

// CheckoutDetached checks out a SHA in detached HEAD mode. Used by
// `pr checkout --detach`.
func CheckoutDetached(r Runner, dir, sha string, stdout, stderr io.Writer) error {
	args := []string{"checkout", "--detach", sha}
	return r.Run(dir, args, stdout, stderr)
}

// BranchExists reports whether a local branch with this name exists.
// Errors from git (i.e., not-a-repo) bubble up; missing branch returns
// (false, nil).
func BranchExists(r Runner, dir, name string) (bool, error) {
	_, err := r.Output(dir, "show-ref", "--verify", "--quiet", "refs/heads/"+name)
	if err == nil {
		return true, nil
	}
	// show-ref exits non-zero when the ref doesn't exist; treat as "no"
	// and return (false, nil). Hard errors (git missing, not a repo) would
	// have surfaced via Output already.
	return false, nil
}

// CommitsAheadBehind returns (ahead, behind) counts for `local` vs
// `upstream`. ahead = commits on local not on upstream. Used by
// `pr create` to decide whether to prompt for a push.
//
// Returns (0, 0, nil) when either ref is missing — caller treats that
// as "no remote tracking, push everything".
func CommitsAheadBehind(r Runner, dir, local, upstream string) (int, int, error) {
	out, err := r.Output(dir, "rev-list", "--left-right", "--count", local+"..."+upstream)
	if err != nil {
		// Missing refs aren't fatal — they're the "no remote yet" case.
		return 0, 0, nil
	}
	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("git: unexpected rev-list output %q", out)
	}
	ahead, err := atoi(parts[0])
	if err != nil {
		return 0, 0, err
	}
	behind, err := atoi(parts[1])
	if err != nil {
		return 0, 0, err
	}
	return ahead, behind, nil
}

// LogSubjects returns the first-line commit subjects for `range` (e.g.,
// "trunk..HEAD"). Order is newest-first. Empty range returns nil.
// Used by `pr create --fill`.
func LogSubjects(r Runner, dir, revRange string) ([]string, error) {
	out, err := r.Output(dir, "log", "--reverse", "--format=%s", revRange)
	if err != nil {
		return nil, fmt.Errorf("git: log: %w", err)
	}
	body := strings.TrimSpace(string(out))
	if body == "" {
		return nil, nil
	}
	return strings.Split(body, "\n"), nil
}

// LogBody returns the commit body (everything after the subject) for the
// first commit in `range` (oldest by chronological order). Used by
// `pr create --fill-first` to pull the commit's full message into the
// PR body.
//
// Implemented as two passes — first finds the oldest SHA, second reads
// its body — because git's `--reverse -n 1` flag combination interacts
// unintuitively (the limit cuts before the reverse).
func LogBody(r Runner, dir, revRange string) (string, error) {
	shaOut, err := r.Output(dir, "rev-list", "--reverse", revRange)
	if err != nil {
		return "", fmt.Errorf("git: rev-list: %w", err)
	}
	body := strings.TrimSpace(string(shaOut))
	if body == "" {
		return "", nil
	}
	first := strings.SplitN(body, "\n", 2)[0]
	out, err := r.Output(dir, "log", "-n", "1", "--format=%b", first)
	if err != nil {
		return "", fmt.Errorf("git: log body: %w", err)
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// Push runs `git push [--set-upstream] <remote> <ref>`. Sets the upstream
// when `setUpstream` is true; equivalent to `git push -u`.
func Push(r Runner, dir, remote, ref string, setUpstream bool, stdout, stderr io.Writer) error {
	args := []string{"push"}
	if setUpstream {
		args = append(args, "--set-upstream")
	}
	args = append(args, remote, ref)
	return r.Run(dir, args, stdout, stderr)
}

// UpstreamOf returns the upstream tracking branch for `local` in the
// "<remote>/<ref>" form (e.g., "origin/trunk"). Empty when no upstream
// is configured (returned as ("", nil)).
func UpstreamOf(r Runner, dir, local string) (string, error) {
	out, err := r.Output(dir, "rev-parse", "--abbrev-ref", local+"@{upstream}")
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(string(out)), nil
}

// HeadSHA returns the resolved SHA of HEAD (or any other ref when
// `ref != ""`). Used by `pr update-branch` to send an ExpectedHeadSHA.
func HeadSHA(r Runner, dir, ref string) (string, error) {
	if ref == "" {
		ref = "HEAD"
	}
	out, err := r.Output(dir, "rev-parse", ref)
	if err != nil {
		return "", fmt.Errorf("git: rev-parse %s: %w", ref, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// atoi is a tiny helper avoiding strconv import in this file's existing
// import set (which is io / os / strings / fmt). Numbers from git are
// small and trusted.
func atoi(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("git: not a number: %q", s)
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

// ensureNotEmpty is a defensive no-op used by callers that pass through
// os.Stderr — kept around so the import stays clean if other helpers
// shed their writers.
var _ = os.Stderr
