// SPDX-License-Identifier: AGPL-3.0-or-later

// Package merge implements `shithub pr merge`. Three strategies
// (merge/squash/rebase), optional auto-merge, optional admin override,
// optional branch deletion, optional head-SHA gate. The default
// strategy follows the repo's allowed-strategies set when no flag is
// given (matches gh).
package merge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/issues"
	"github.com/tenseleyFlow/shithub-cli/internal/pulls"
	"github.com/tenseleyFlow/shithub-cli/internal/repos"
	prshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/pr/shared"
	repocmdshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
	"github.com/tenseleyFlow/shithub-cli/pkg/cmd/shared/crosskind"
)

type options struct {
	IO          *iostreams.IOStreams
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string
	GitRunner   git.Runner

	Arg      string
	Repo     string
	Hostname string

	Merge  bool
	Squash bool
	Rebase bool

	Auto         bool
	DisableAuto  bool
	Admin        bool
	DeleteBranch bool

	MatchHead string

	Subject      string
	Body         string
	BodyFile     string
	InsertPRBody bool
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
	}
	cmd := &cobra.Command{
		Use:   "merge [<number-or-url-or-branch>]",
		Short: "Merge a pull request",
		// E-audit E22: accept zero args so the no-arg invocation can
		// resolve against the current branch's open PR (gh-compat).
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Arg = args[0]
			}
			if opts.GitRunner == nil {
				if r, err := git.FromPath(); err == nil {
					opts.GitRunner = r
				}
			}
			return Run(c.Context(), opts)
		},
	}
	cmd.Flags().StringVarP(&opts.Repo, "repo", "R", "", "select another repository using the [HOST/]OWNER/REPO format")
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "the shithub host (default: configured host)")
	cmd.Flags().BoolVarP(&opts.Merge, "merge", "m", false, "create a merge commit")
	cmd.Flags().BoolVarP(&opts.Squash, "squash", "s", false, "squash all PR commits into one merge commit")
	cmd.Flags().BoolVarP(&opts.Rebase, "rebase", "r", false, "rebase the PR commits onto the base")
	cmd.Flags().BoolVar(&opts.Auto, "auto", false, "enable auto-merge once required checks pass")
	cmd.Flags().BoolVar(&opts.DisableAuto, "disable-auto", false, "cancel a pending auto-merge")
	cmd.Flags().BoolVar(&opts.Admin, "admin", false, "override branch protections (requires admin scope)")
	cmd.Flags().BoolVarP(&opts.DeleteBranch, "delete-branch", "d", false, "delete the PR's head branch after merge")
	cmd.Flags().StringVar(&opts.MatchHead, "match-head-commit", "", "require the PR head SHA to match this value")
	cmd.Flags().StringVarP(&opts.Subject, "subject", "t", "", "merge commit subject")
	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "merge commit body")
	cmd.Flags().StringVarP(&opts.BodyFile, "body-file", "F", "", "read merge commit body from file (use '-' for stdin)")
	cmd.Flags().BoolVar(&opts.InsertPRBody, "insert-body-into-commit", false, "use the PR body as the merge commit body")
	return cmd
}

// Run executes the merge operation.
//
// auto-merge enable/disable, admin override, branch delete — all paths
// share the same setup.
//
//nolint:gocyclo // merge orchestrates strategy selection, head-match,
func Run(ctx context.Context, opts *options) error {
	// H4 (H6): --auto / --disable-auto are vapor flags — advertised in
	// --help, but the server endpoint isn't shipped. Pre-flight reject
	// with the typed deferred-stub error so the root maps it to exit 2
	// and scripts can distinguish "feature pending" from a real merge
	// failure. When auto-merge ships server-side, swap this guard for
	// the real `pc.EnableAutoMerge / DisableAutoMerge` call paths.
	if opts.Auto {
		return &cmdutil.NotYetSupportedError{Name: "shithub pr merge --auto", Track: "shithub auto-merge endpoint"}
	}
	if opts.DisableAuto {
		return &cmdutil.NotYetSupportedError{Name: "shithub pr merge --disable-auto", Track: "shithub auto-merge endpoint"}
	}
	if !opts.DisableAuto {
		if strategyCount(opts) > 1 {
			return errors.New("pr merge: --merge, --squash, --rebase are mutually exclusive")
		}
	}
	if opts.InsertPRBody && (opts.Body != "" || opts.BodyFile != "") {
		return errors.New("pr merge: --insert-body-into-commit cannot combine with --body/--body-file")
	}

	resolver := repocmdshared.Resolver{
		RepoFlag:    opts.Repo,
		Hostname:    opts.Hostname,
		DefaultHost: hostOrDefault(opts.DefaultHost),
		GitRunner:   opts.GitRunner,
	}
	fb, rerr := resolver.Resolve()
	if rerr != nil && !strings.Contains(opts.Arg, "://") {
		return rerr
	}
	if rerr != nil {
		fb = repocmdshared.RepoRef{}
	}
	client, err := opts.HTTPClient(fb.Host)
	if err != nil {
		return err
	}
	pc := pulls.NewClient(client)

	var ref prshared.PRRef
	if opts.Arg != "" {
		ref, err = prshared.ParsePRArg(ctx, pc, opts.Arg, fb)
		if err != nil {
			return err
		}
	} else {
		// E-audit E22: no arg → resolve the open PR for the current
		// branch. Matches gh's `gh pr merge` shape; without it users
		// inside a checked-out PR branch had to look up the number
		// before they could merge.
		branch := prshared.CurrentBranchFromGit(opts.GitRunner, "")
		if branch == "" {
			return errors.New("pr merge: pass a number/URL/branch (couldn't detect current branch)")
		}
		pr, perr := prshared.FindPRByBranch(ctx, pc, fb, branch)
		if perr != nil {
			return perr
		}
		ref = prshared.PRRef{Repo: fb, Number: pr.Number}
	}
	if ref.Repo.Host == "" {
		ref.Repo.Host = fb.Host
	}

	if opts.DisableAuto {
		if err := pc.DisableAutoMerge(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number); err != nil {
			return err
		}
		fmt.Fprintf(opts.IO.ErrOut, "%s Cancelled auto-merge on PR #%d\n", opts.IO.SuccessIcon(), ref.Number)
		return nil
	}

	pr, err := pc.View(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number)
	if err != nil {
		// H2: cross-namespace verb routing — if the number is an issue,
		// surface a friendly redirect instead of "pull request not found".
		if api.IsNotFoundError(err) {
			ic := issues.NewClient(client)
			if cerr := crosskind.Check(ctx, ic, pc, ref.Repo.Owner, ref.Repo.Name, ref.Number, "pr", "pr merge", "merge"); cerr != nil {
				return cerr
			}
		}
		return err
	}
	if pr.Merged {
		return fmt.Errorf("pr merge: PR #%d is already merged", ref.Number)
	}
	if pr.State == "closed" {
		return fmt.Errorf("pr merge: PR #%d is closed; reopen first", ref.Number)
	}

	if opts.MatchHead != "" && !strings.EqualFold(pr.Head.SHA, opts.MatchHead) {
		return fmt.Errorf("pr merge: --match-head-commit %s != current head %s", opts.MatchHead, pr.Head.SHA)
	}

	method, err := chooseStrategy(ctx, opts, client, ref)
	if err != nil {
		return err
	}

	body, err := composeBody(opts, pr)
	if err != nil {
		return err
	}

	if opts.Auto {
		in := pulls.AutoMergeInput{
			MergeMethod:   method,
			CommitTitle:   opts.Subject,
			CommitMessage: body,
		}
		if err := pc.EnableAutoMerge(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number, in); err != nil {
			return err
		}
		fmt.Fprintf(opts.IO.ErrOut, "%s Enabled auto-merge (%s) on PR #%d\n", opts.IO.SuccessIcon(), method, ref.Number)
		return nil
	}

	in := pulls.MergeInput{
		MergeMethod:   method,
		CommitTitle:   opts.Subject,
		CommitMessage: body,
		SHA:           opts.MatchHead,
	}
	result, err := pc.Merge(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number, in, opts.Admin)
	if err != nil {
		return err
	}
	if !result.Merged {
		msg := result.Message
		if msg == "" {
			msg = "server refused merge"
		}
		return fmt.Errorf("pr merge: %s", msg)
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Merged PR #%d (%s) at %s\n", opts.IO.SuccessIcon(), ref.Number, method, shortSHA(result.CommitSHA()))

	if opts.DeleteBranch {
		if err := deleteBranch(ctx, pc, ref, pr); err != nil {
			fmt.Fprintf(opts.IO.ErrOut, "warning: --delete-branch: %v\n", err)
		}
	}
	return nil
}

// chooseStrategy returns the merge method to use. Explicit flag wins;
// otherwise read the repo's `allow_*_merge` settings and pick the first
// allowed in gh's preference order (merge → squash → rebase). When the
// server doesn't echo any of the flags (pre-S50 §4, or older shithub
// builds) every field is the zero value and we fall back to plain merge
// — the same behavior as gh against a repo that omits the fields.
func chooseStrategy(ctx context.Context, opts *options, client *api.Client, ref prshared.PRRef) (pulls.MergeMethod, error) {
	switch {
	case opts.Merge:
		return pulls.MergeMerge, nil
	case opts.Squash:
		return pulls.MergeSquash, nil
	case opts.Rebase:
		return pulls.MergeRebase, nil
	}
	rc := repos.NewClient(client)
	repo, err := rc.View(ctx, ref.Repo.Owner, ref.Repo.Name)
	if err != nil {
		// Default to plain merge if we can't read the repo settings.
		return pulls.MergeMerge, nil //nolint:nilerr // fall-through is intentional
	}
	switch {
	case repo.AllowMergeCommit:
		return pulls.MergeMerge, nil
	case repo.AllowSquashMerge:
		return pulls.MergeSquash, nil
	case repo.AllowRebaseMerge:
		return pulls.MergeRebase, nil
	}
	// All flags zero — server didn't advertise. Default to plain merge.
	return pulls.MergeMerge, nil
}

// composeBody picks the merge commit body. Precedence: --body-file >
// --body > --insert-body-into-commit (PR body) > empty.
func composeBody(opts *options, pr *pulls.PR) (string, error) {
	body, err := readBodyFile(opts.BodyFile, opts.Body, opts.IO.In)
	if err != nil {
		return "", err
	}
	if body != "" {
		return body, nil
	}
	if opts.InsertPRBody {
		return pr.Body, nil
	}
	return "", nil
}

// deleteBranch removes the PR's head branch on the server, then locally
// when the working tree has it. Best-effort: errors are warnings.
func deleteBranch(ctx context.Context, pc *pulls.Client, ref prshared.PRRef, pr *pulls.PR) error {
	head := pr.Head.Ref
	if head == "" {
		return errors.New("PR has no head ref")
	}
	if pr.Head.Repo != nil && pr.Base.Repo != nil && pr.Head.Repo.FullName == pr.Base.Repo.FullName {
		if err := pc.DeleteBranch(ctx, ref.Repo.Owner, ref.Repo.Name, head); err != nil {
			return fmt.Errorf("remote: %w", err)
		}
	}
	return nil
}

func strategyCount(opts *options) int {
	n := 0
	for _, b := range []bool{opts.Merge, opts.Squash, opts.Rebase} {
		if b {
			n++
		}
	}
	return n
}

func readBodyFile(path, fallback string, stdin io.Reader) (string, error) {
	if path == "" {
		return fallback, nil
	}
	if path == "-" {
		if stdin == nil {
			return "", errors.New("pr merge: --body-file=- but stdin is nil")
		}
		b, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("pr merge: read stdin: %w", err)
		}
		return string(b), nil
	}
	b, err := os.ReadFile(path) //nolint:gosec // user-supplied path
	if err != nil {
		return "", fmt.Errorf("pr merge: read %q: %w", path, err)
	}
	return string(b), nil
}

func shortSHA(sha string) string {
	if len(sha) < 7 {
		return sha
	}
	return sha[:7]
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
