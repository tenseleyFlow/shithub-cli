// SPDX-License-Identifier: AGPL-3.0-or-later

// Package comment implements `shithub pr comment`. Issue-style comment
// on the PR conversation (not a file-line review comment). Shares the
// /issues/{n}/comments endpoint with `issue comment`.
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
	"github.com/tenseleyFlow/shithub-cli/internal/pulls"
	prshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/pr/shared"
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

	Body         string
	BodyFile     string
	Editor       bool
	EditLast     bool
	CreateIfNone bool
	Web          bool
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
		Use:   "comment <number-or-url-or-branch>",
		Short: "Add or edit a comment on a pull request",
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
	cmd.Flags().BoolVar(&opts.EditLast, "edit-last", false, "edit the caller's most-recent comment on this PR")
	cmd.Flags().BoolVar(&opts.CreateIfNone, "create-if-none", false, "with --edit-last, create a new comment when no prior comment exists")
	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "open the PR in a browser to comment")
	return cmd
}

// Run executes the comment operation.
//
//nolint:gocyclo // create/edit dispatch + body picker + create-if-none.
func Run(ctx context.Context, opts *options) error {
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

	ref, err := prshared.ParsePRArg(ctx, pc, opts.Arg, fb)
	if err != nil {
		return err
	}
	if ref.Repo.Host == "" {
		ref.Repo.Host = fb.Host
	}

	if opts.Web {
		url := prshared.PRWebURL(ref)
		fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", url)
		return opts.Opener(url)
	}

	body, err := readBodyFile(opts.BodyFile, opts.Body, opts.IO.In)
	if err != nil {
		return err
	}
	if body == "" && opts.Editor {
		composed, cerr := markdown.Compose(opts.ConfigFn, markdown.ComposeOptions{
			Hints: []string{
				fmt.Sprintf("shithub-cli PR comment for %s#%d", ref.Repo.FullName(), ref.Number),
				"Lines starting with # are stripped before send.",
			},
		})
		if cerr != nil {
			return cerr
		}
		body = composed
	}
	if strings.TrimSpace(body) == "" {
		return errors.New("pr comment: body is required (use --body, --body-file, or --editor)")
	}

	ic := issues.NewClient(client)

	if opts.EditLast {
		commentID, err := pickLastByCaller(ctx, client, ic, ref)
		if err != nil {
			if opts.CreateIfNone && isNoPrior(err) {
				return postNew(ctx, ic, opts.IO, ref, body)
			}
			return err
		}
		edited, err := ic.EditComment(ctx, ref.Repo.Owner, ref.Repo.Name, commentID, body)
		if err != nil {
			return err
		}
		if edited.HTMLURL != "" {
			fmt.Fprintln(opts.IO.Out, edited.HTMLURL)
		}
		return nil
	}

	return postNew(ctx, ic, opts.IO, ref, body)
}

// postNew is the shared "POST a fresh comment" path used by the default
// flow and by --create-if-none.
func postNew(ctx context.Context, ic *issues.Client, io *iostreams.IOStreams, ref prshared.PRRef, body string) error {
	created, err := ic.AddComment(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number, body)
	if err != nil {
		return err
	}
	if created.HTMLURL != "" {
		fmt.Fprintln(io.Out, created.HTMLURL)
	}
	return nil
}

// pickLastByCaller resolves the current user and finds their most-recent
// comment on the issue thread.
func pickLastByCaller(ctx context.Context, client *api.Client, ic *issues.Client, ref prshared.PRRef) (int64, error) {
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
	return 0, noPriorErr(me.Login)
}

// noPriorErr is the sentinel error pickLastByCaller returns when the
// caller has never commented on the thread. --create-if-none uses
// isNoPrior to recover.
type noPriorErr string

func (e noPriorErr) Error() string {
	return "pr comment: no prior comment by " + string(e) + " on this PR"
}

func isNoPrior(err error) bool {
	_, ok := err.(noPriorErr)
	return ok
}

func readBodyFile(path, fallback string, stdin io.Reader) (string, error) {
	if path == "" {
		return fallback, nil
	}
	if path == "-" {
		if stdin == nil {
			return "", errors.New("pr comment: --body-file=- but stdin is nil")
		}
		b, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("pr comment: read stdin: %w", err)
		}
		return string(b), nil
	}
	b, err := os.ReadFile(path) //nolint:gosec // user-supplied path
	if err != nil {
		return "", fmt.Errorf("pr comment: read %q: %w", path, err)
	}
	return string(b), nil
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
