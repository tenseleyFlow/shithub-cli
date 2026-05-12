// SPDX-License-Identifier: AGPL-3.0-or-later

// Package review implements `shithub pr review`. Submits an approve /
// request-changes / comment review event with an optional body. The
// event flags are mutually exclusive; exactly one is required outside
// interactive mode. `--request-changes` mandates a non-empty body
// (matches GitHub policy and gh's UX).
package review

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
	"github.com/tenseleyFlow/shithub-cli/internal/config"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/markdown"
	"github.com/tenseleyFlow/shithub-cli/internal/prompter"
	"github.com/tenseleyFlow/shithub-cli/internal/pulls"
	prshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/pr/shared"
	repocmdshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

type options struct {
	IO          *iostreams.IOStreams
	Prompter    prompter.Prompter
	HTTPClient  func(host string) (*api.Client, error)
	ConfigFn    func() (*config.Config, error)
	DefaultHost func() string
	GitRunner   git.Runner
	Opener      func(url string) error

	Arg      string
	Repo     string
	Hostname string

	Approve        bool
	RequestChanges bool
	Comment        bool
	Body           string
	BodyFile       string
	Editor         bool
	Web            bool
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		Prompter:    f.Prompter,
		HTTPClient:  f.HTTPClient,
		ConfigFn:    f.Config,
		DefaultHost: f.DefaultHost,
		Opener:      func(_ string) error { return nil },
	}
	cmd := &cobra.Command{
		Use:   "review [<number-or-url-or-branch>]",
		Short: "Submit a review on a pull request",
		Args:  cobra.MaximumNArgs(1),
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
	cmd.Flags().BoolVarP(&opts.Approve, "approve", "a", false, "approve the PR")
	cmd.Flags().BoolVarP(&opts.RequestChanges, "request-changes", "r", false, "request changes (body required)")
	cmd.Flags().BoolVarP(&opts.Comment, "comment", "c", false, "leave a review comment without approving")
	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "review body")
	cmd.Flags().StringVarP(&opts.BodyFile, "body-file", "F", "", "read review body from file (use '-' for stdin)")
	cmd.Flags().BoolVar(&opts.Editor, "editor", false, "compose body via $EDITOR")
	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "open the PR review form in a browser")
	return cmd
}

// Run executes the review submission.
//
// with event-specific body validation.
//
//nolint:gocyclo // dispatch over web / interactive / flag-driven paths
func Run(ctx context.Context, opts *options) error {
	resolver := repocmdshared.Resolver{
		RepoFlag:    opts.Repo,
		Hostname:    opts.Hostname,
		DefaultHost: hostOrDefault(opts.DefaultHost),
		GitRunner:   opts.GitRunner,
	}
	fb, rerr := resolver.Resolve()
	if rerr != nil && (opts.Arg == "" || !strings.Contains(opts.Arg, "://")) {
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
	if opts.Arg == "" {
		branch := prshared.CurrentBranchFromGit(opts.GitRunner, "")
		if branch == "" {
			return errors.New("pr review: pass a number/URL/branch (couldn't detect current branch)")
		}
		pr, err := prshared.FindPRByBranch(ctx, pc, fb, branch)
		if err != nil {
			return err
		}
		ref = prshared.PRRef{Repo: fb, Number: pr.Number}
	} else {
		ref, err = prshared.ParsePRArg(ctx, pc, opts.Arg, fb)
		if err != nil {
			return err
		}
	}
	if ref.Repo.Host == "" {
		ref.Repo.Host = fb.Host
	}

	if opts.Web {
		url := prshared.PRWebURL(ref) + "/files"
		fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", url)
		return opts.Opener(url)
	}

	body, err := readBodyFile(opts.BodyFile, opts.Body, opts.IO.In)
	if err != nil {
		return err
	}
	opts.Body = body

	event, err := pickEvent(opts)
	if err != nil {
		return err
	}

	if opts.Editor || (opts.Body == "" && opts.IO.IsStdoutTTY() && !flagOnlyMode(opts)) {
		composed, cerr := markdown.Compose(opts.ConfigFn, markdown.ComposeOptions{
			Initial: opts.Body,
			Hints: []string{
				fmt.Sprintf("shithub-cli PR review for %s#%d", ref.Repo.FullName(), ref.Number),
				"Lines starting with # are stripped before send.",
			},
		})
		if cerr != nil {
			return cerr
		}
		opts.Body = composed
	}

	if event == pulls.ReviewRequestChanges && strings.TrimSpace(opts.Body) == "" {
		return errors.New("pr review: --request-changes requires a non-empty body (--body, --body-file, or --editor)")
	}

	out, err := pc.SubmitReview(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number, pulls.ReviewInput{
		Event: event, Body: opts.Body,
	})
	if err != nil {
		return err
	}

	verb := verbForEvent(event)
	fmt.Fprintf(opts.IO.ErrOut, "%s %s PR #%d\n", opts.IO.SuccessIcon(), verb, ref.Number)
	if out.HTMLURL != "" {
		fmt.Fprintln(opts.IO.Out, out.HTMLURL)
	}
	return nil
}

// pickEvent enforces the event mutex and (in interactive mode) prompts
// the user to pick one when no flag is set.
func pickEvent(opts *options) (pulls.ReviewEvent, error) {
	n := 0
	for _, b := range []bool{opts.Approve, opts.RequestChanges, opts.Comment} {
		if b {
			n++
		}
	}
	if n > 1 {
		return "", errors.New("pr review: --approve, --request-changes, and --comment are mutually exclusive")
	}
	if n == 0 {
		if !opts.IO.IsStdoutTTY() {
			return "", errors.New("pr review: pick one of --approve / --request-changes / --comment")
		}
		idx, err := opts.Prompter.Select("Review action", "Comment", []string{"Approve", "Request changes", "Comment"})
		if err != nil {
			return "", err
		}
		switch idx {
		case 0:
			return pulls.ReviewApprove, nil
		case 1:
			return pulls.ReviewRequestChanges, nil
		default:
			return pulls.ReviewComment, nil
		}
	}
	switch {
	case opts.Approve:
		return pulls.ReviewApprove, nil
	case opts.RequestChanges:
		return pulls.ReviewRequestChanges, nil
	}
	return pulls.ReviewComment, nil
}

// flagOnlyMode reports whether the caller provided a complete (event +
// body) set via flags. Used to decide whether to spawn the editor on
// empty body — we don't want to open the editor when the user just
// approved without a comment.
func flagOnlyMode(opts *options) bool {
	hasEvent := opts.Approve || opts.RequestChanges || opts.Comment
	hasBody := opts.Body != "" || opts.BodyFile != ""
	return hasEvent && (hasBody || (opts.Approve && !opts.Editor))
}

func verbForEvent(e pulls.ReviewEvent) string {
	switch e {
	case pulls.ReviewApprove:
		return "Approved"
	case pulls.ReviewRequestChanges:
		return "Requested changes on"
	}
	return "Commented on"
}

func readBodyFile(path, fallback string, stdin io.Reader) (string, error) {
	if path == "" {
		return fallback, nil
	}
	if path == "-" {
		if stdin == nil {
			return "", errors.New("pr review: --body-file=- but stdin is nil")
		}
		b, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("pr review: read stdin: %w", err)
		}
		return string(b), nil
	}
	b, err := os.ReadFile(path) //nolint:gosec // user-supplied path
	if err != nil {
		return "", fmt.Errorf("pr review: read %q: %w", path, err)
	}
	return string(b), nil
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
