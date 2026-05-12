// SPDX-License-Identifier: AGPL-3.0-or-later

// Package create implements `shithub pr create`. Three modes:
//
//   - --web: open the compare URL with prefilled title/body/draft.
//   - flag-driven: build a CreateInput from flags + optional --fill.
//   - interactive (TTY, no --title and no --fill): prompt for title,
//     compose body via $EDITOR, confirm.
package create

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
	"github.com/tenseleyFlow/shithub-cli/internal/repos"
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

	Repo     string
	Hostname string

	Title        string
	Body         string
	BodyFile     string
	Base         string
	Head         string
	Draft        bool
	Web          bool
	Editor       bool
	Fill         bool
	FillFirst    bool
	FillVerbose  bool
	NoMaintainer bool
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
		Use:   "create",
		Short: "Create a pull request",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
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
	cmd.Flags().StringVarP(&opts.Title, "title", "t", "", "PR title")
	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "PR body")
	cmd.Flags().StringVarP(&opts.BodyFile, "body-file", "F", "", "read PR body from file (use '-' for stdin)")
	cmd.Flags().StringVarP(&opts.Base, "base", "B", "", "the branch to merge into (default: repo's default branch)")
	cmd.Flags().StringVarP(&opts.Head, "head", "H", "", "the branch you have changes on (default: current branch); use 'user:branch' for cross-fork")
	cmd.Flags().BoolVarP(&opts.Draft, "draft", "d", false, "mark the new PR as a draft")
	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "open the new-PR compare URL in a browser")
	cmd.Flags().BoolVar(&opts.Editor, "editor", false, "compose body via $EDITOR even when --body is set")
	cmd.Flags().BoolVarP(&opts.Fill, "fill", "f", false, "use commit subjects to fill title/body")
	cmd.Flags().BoolVar(&opts.FillFirst, "fill-first", false, "use only the first commit for title and body")
	cmd.Flags().BoolVar(&opts.FillVerbose, "fill-verbose", false, "use full commit messages for body")
	cmd.Flags().BoolVar(&opts.NoMaintainer, "no-maintainer-edit", false, "disable maintainer edits on this PR")
	return cmd
}

// Run executes the create operation.
//
//nolint:gocyclo // create dispatches across web/flag/interactive + --fill variants.
func Run(ctx context.Context, opts *options) error {
	if multipleFill(opts) {
		return errors.New("pr create: --fill, --fill-first, and --fill-verbose are mutually exclusive")
	}

	resolver := repocmdshared.Resolver{
		RepoFlag:    opts.Repo,
		Hostname:    opts.Hostname,
		DefaultHost: hostOrDefault(opts.DefaultHost),
		GitRunner:   opts.GitRunner,
	}
	ref, err := resolver.Resolve()
	if err != nil {
		return err
	}

	head, err := resolveHead(opts)
	if err != nil {
		return err
	}

	// Read body source first so --web has access to it.
	body, err := readBodyFile(opts.BodyFile, opts.Body, opts.IO.In)
	if err != nil {
		return err
	}
	opts.Body = body

	client, err := opts.HTTPClient(ref.Host)
	if err != nil {
		return err
	}

	// Base defaults to the repo's default branch when not specified.
	base := opts.Base
	if base == "" {
		base, err = lookupDefaultBranch(ctx, client, ref)
		if err != nil {
			return err
		}
	}

	if err := applyFill(opts, base); err != nil {
		return err
	}

	if opts.Web {
		url := prshared.NewPRWebURL(ref, base, head, opts.Title, opts.Body, opts.Draft)
		fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", url)
		return opts.Opener(url)
	}

	interactive := opts.Title == "" && !opts.Fill && !opts.FillFirst && !opts.FillVerbose && opts.IO.IsStdoutTTY()
	if interactive {
		if err := promptForFlags(opts); err != nil {
			return err
		}
	} else if opts.Editor {
		composed, cerr := markdown.Compose(opts.ConfigFn, markdown.ComposeOptions{
			Initial: opts.Body,
			Hints:   []string{"Lines starting with # are stripped before send."},
		})
		if cerr != nil {
			return cerr
		}
		opts.Body = composed
	}

	if strings.TrimSpace(opts.Title) == "" {
		return errors.New("pr create: title is required (--title, --fill, or interactive)")
	}

	in := pulls.CreateInput{
		Title: opts.Title,
		Body:  opts.Body,
		Head:  head,
		Base:  base,
		Draft: opts.Draft,
	}
	if opts.NoMaintainer {
		no := false
		in.MaintainerCanEdit = &no
	}

	pc := pulls.NewClient(client)
	created, err := pc.Create(ctx, ref.Owner, ref.Name, in)
	if err != nil {
		return err
	}
	if created.HTMLURL != "" {
		fmt.Fprintln(opts.IO.Out, created.HTMLURL)
	} else {
		fmt.Fprintf(opts.IO.Out, "%s/%s#%d\n", ref.Owner, ref.Name, created.Number)
	}
	return nil
}

// multipleFill reports whether more than one of the three fill flags is set.
func multipleFill(opts *options) bool {
	n := 0
	for _, b := range []bool{opts.Fill, opts.FillFirst, opts.FillVerbose} {
		if b {
			n++
		}
	}
	return n > 1
}

// resolveHead figures out the head ref. Empty --head reads the current
// git branch; explicit "user:branch" passes through.
func resolveHead(opts *options) (string, error) {
	if opts.Head != "" {
		return opts.Head, nil
	}
	if opts.GitRunner == nil {
		return "", errors.New("pr create: --head required (no git binary on PATH)")
	}
	branch, err := git.CurrentBranch(opts.GitRunner, "")
	if err != nil {
		return "", fmt.Errorf("pr create: detect current branch: %w", err)
	}
	return branch, nil
}

// lookupDefaultBranch hits /repos/{o}/{r} to read default_branch. Cached
// per command since base is normally only needed once.
func lookupDefaultBranch(ctx context.Context, client *api.Client, ref repocmdshared.RepoRef) (string, error) {
	rc := repos.NewClient(client)
	meta, err := rc.View(ctx, ref.Owner, ref.Name)
	if err != nil {
		return "", fmt.Errorf("pr create: read repo default branch: %w", err)
	}
	if meta.DefaultBranch == "" {
		return "trunk", nil
	}
	return meta.DefaultBranch, nil
}

// applyFill runs the user's chosen --fill variant against the local
// repo's commit log. Only one mode is enabled at a time (validated up top).
func applyFill(opts *options, base string) error {
	mode := prshared.FillNone
	switch {
	case opts.Fill:
		mode = prshared.FillStandard
	case opts.FillFirst:
		mode = prshared.FillFirst
	case opts.FillVerbose:
		mode = prshared.FillVerbose
	}
	if mode == prshared.FillNone {
		return nil
	}
	if opts.GitRunner == nil {
		return errors.New("pr create: --fill needs the git binary on PATH")
	}
	title, body, err := prshared.Fill(opts.GitRunner, "", prshared.FillOptions{
		Mode:     mode,
		RevRange: base + "..HEAD",
	})
	if err != nil {
		return err
	}
	// User-provided flags take precedence over fill output so the user
	// can override what fill picked up.
	if opts.Title == "" {
		opts.Title = title
	}
	if opts.Body == "" {
		opts.Body = body
	}
	return nil
}

// promptForFlags drives the interactive flow. The body step always opens
// the editor — composing a multi-line PR body inside a prompt is too
// constrained.
func promptForFlags(opts *options) error {
	title, err := opts.Prompter.Input("Title", opts.Title)
	if err != nil {
		return err
	}
	opts.Title = strings.TrimSpace(title)

	choice, err := opts.Prompter.Select("Body", "Skip", []string{"Open editor", "Skip"})
	if err != nil {
		return err
	}
	if choice == 0 {
		composed, cerr := markdown.Compose(opts.ConfigFn, markdown.ComposeOptions{
			Initial: opts.Body,
			Hints:   []string{"Lines starting with # are stripped before send."},
		})
		if cerr != nil {
			return cerr
		}
		opts.Body = composed
	}
	return nil
}

// readBodyFile picks the body source: --body-file takes precedence over
// --body. "-" means stdin. The flag-set body is returned unchanged when
// no file flag is in effect.
func readBodyFile(path, fallback string, stdin io.Reader) (string, error) {
	if path == "" {
		return fallback, nil
	}
	if path == "-" {
		if stdin == nil {
			return "", errors.New("pr create: --body-file=- but stdin is nil")
		}
		b, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("pr create: read stdin: %w", err)
		}
		return string(b), nil
	}
	b, err := os.ReadFile(path) //nolint:gosec // user-supplied path is the documented purpose
	if err != nil {
		return "", fmt.Errorf("pr create: read %q: %w", path, err)
	}
	return string(b), nil
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
