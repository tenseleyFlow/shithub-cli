// SPDX-License-Identifier: AGPL-3.0-or-later

// Package comment implements `shithub issue comment`. Posts a new comment
// or — with --edit-last — patches the authenticated user's most-recent
// comment on the issue.
package comment

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
	"github.com/tenseleyFlow/shithub-cli/internal/issues"
	"github.com/tenseleyFlow/shithub-cli/internal/markdown"
	issueshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/issue/shared"
	repocmdshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

type options struct {
	IO          *iostreams.IOStreams
	HTTPClient  func(host string) (*api.Client, error)
	ConfigFn    func() (*config.Config, error)
	DefaultHost func() string
	GitRunner   git.Runner
	Opener      func(url string) error

	Arg      string
	Repo     string
	Hostname string

	Body     string
	BodyFile string
	Editor   bool
	EditLast bool
	Web      bool
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		ConfigFn:    f.Config,
		DefaultHost: f.DefaultHost,
		Opener:      func(_ string) error { return nil },
	}
	cmd := &cobra.Command{
		Use:   "comment <number-or-url>",
		Short: "Add or edit a comment on an issue",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts.Arg = args[0]
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
	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "comment body")
	cmd.Flags().StringVarP(&opts.BodyFile, "body-file", "F", "", "read comment body from file (use '-' for stdin)")
	cmd.Flags().BoolVar(&opts.Editor, "editor", false, "compose body via $EDITOR")
	cmd.Flags().BoolVar(&opts.EditLast, "edit-last", false, "edit the caller's most-recent comment on this issue")
	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "open the issue in a browser to comment")
	return cmd
}

// Run executes the comment operation.
//
//nolint:gocyclo // create/edit dispatch + body source picker keeps everything close.
func Run(ctx context.Context, opts *options) error {
	ref, err := resolve(opts)
	if err != nil {
		return err
	}

	if opts.Web {
		url := issueshared.IssueWebURL(ref)
		fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", url)
		return opts.Opener(url)
	}

	body, err := readBodyFile(opts.BodyFile, opts.Body, opts.IO.In)
	if err != nil {
		return err
	}
	if body == "" && opts.Editor {
		composed, cerr := markdown.Compose(opts.ConfigFn, markdown.ComposeOptions{
			Hints: []string{"Lines starting with # are stripped before send."},
		})
		if cerr != nil {
			return cerr
		}
		body = composed
	}
	if strings.TrimSpace(body) == "" {
		return errors.New("issue comment: body is required (use --body, --body-file, or --editor)")
	}

	client, err := opts.HTTPClient(ref.Repo.Host)
	if err != nil {
		return err
	}
	ic := issues.NewClient(client)

	if opts.EditLast {
		commentID, err := pickLastByCaller(ctx, client, ic, ref)
		if err != nil {
			return err
		}
		edited, err := ic.EditComment(ctx, ref.Repo.Owner, ref.Repo.Name, commentID, body)
		if err != nil {
			return err
		}
		printCommentResult(opts.IO, ref, edited.HTMLURL, "Edited")
		return nil
	}

	created, err := ic.AddComment(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number, body)
	if err != nil {
		return err
	}
	printCommentResult(opts.IO, ref, created.HTMLURL, "Commented on")
	return nil
}

// printCommentResult writes the post-comment success indicator.
// Audit A18: pre-fix this fell through silently when the server's
// response had an empty html_url (current shithub state) — users
// couldn't tell whether their comment landed. Now: stdout gets the
// URL when present (script-friendly), stderr gets a confirmation
// line on the empty-URL path so the interactive flow is never silent.
func printCommentResult(ios *iostreams.IOStreams, ref issueshared.IssueRef, htmlURL, verb string) {
	if htmlURL != "" {
		fmt.Fprintln(ios.Out, htmlURL)
		return
	}
	fmt.Fprintf(ios.ErrOut, "%s %s %s/%s#%d\n",
		ios.SuccessIcon(), verb, ref.Repo.Owner, ref.Repo.Name, ref.Number)
}

// pickLastByCaller resolves the current user and finds their most-recent
// comment on the issue. Returns an error when the caller has never
// commented on the thread — matches gh's UX.
func pickLastByCaller(ctx context.Context, client *api.Client, ic *issues.Client, ref issueshared.IssueRef) (int64, error) {
	me, err := client.CurrentUser(ctx)
	if err != nil {
		return 0, err
	}
	list, err := ic.ListComments(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number)
	if err != nil {
		return 0, err
	}
	for i := len(list) - 1; i >= 0; i-- {
		c := list[i]
		if c.User != nil && strings.EqualFold(c.User.Login, me.Login) {
			return c.ID, nil
		}
	}
	return 0, fmt.Errorf("issue comment: no prior comment by %s on this issue", me.Login)
}

func resolve(opts *options) (issueshared.IssueRef, error) {
	rs := repocmdshared.Resolver{
		RepoFlag:    opts.Repo,
		Hostname:    opts.Hostname,
		DefaultHost: hostOrDefault(opts.DefaultHost),
		GitRunner:   opts.GitRunner,
	}
	fb, rerr := rs.Resolve()
	if rerr != nil && !strings.Contains(opts.Arg, "://") {
		return issueshared.IssueRef{}, rerr
	}
	if rerr != nil {
		fb = repocmdshared.RepoRef{}
	}
	ref, _, err := issueshared.ParseIssueArg(opts.Arg, fb)
	if err != nil {
		return issueshared.IssueRef{}, err
	}
	if ref.Repo.Host == "" {
		ref.Repo.Host = fb.Host
	}
	return ref, nil
}

func readBodyFile(path, fallback string, stdin io.Reader) (string, error) {
	if path == "" {
		return fallback, nil
	}
	if path == "-" {
		if stdin == nil {
			return "", fmt.Errorf("issue comment: --body-file=- but stdin is nil")
		}
		b, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("issue comment: read stdin: %w", err)
		}
		return string(b), nil
	}
	b, err := os.ReadFile(path) //nolint:gosec // user-supplied path
	if err != nil {
		return "", fmt.Errorf("issue comment: read %q: %w", path, err)
	}
	return string(b), nil
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
