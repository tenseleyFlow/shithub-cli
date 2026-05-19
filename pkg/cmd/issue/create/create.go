// SPDX-License-Identifier: AGPL-3.0-or-later

// Package create implements `shithub issue create`. Two modes:
//   - Flag-driven (--title supplied or --web): build a CreateInput and POST.
//   - Interactive (TTY, no --title): prompt for title, launch $EDITOR for
//     body, prompt for labels/assignees/milestone.
//
// `--web` opens /owner/repo/issues/new with prefilled title/body query
// params instead of posting.
package create

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/issues"
	"github.com/tenseleyFlow/shithub-cli/internal/markdown"
	"github.com/tenseleyFlow/shithub-cli/internal/prompter"
	issueshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/issue/shared"
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

	Title     string
	Body      string
	BodyFile  string
	Labels    []string
	Assignees []string
	// MilestoneRef accepts either a numeric id or a milestone title;
	// resolution happens at Run time against the live milestone list
	// (E-audit E19 — gh accepts either form, we used to require an int).
	MilestoneRef string
	Template     string
	Web          bool
	Editor       bool
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		Prompter:    f.Prompter,
		HTTPClient:  f.HTTPClient,
		ConfigFn:    f.Config,
		DefaultHost: f.DefaultHost,
		Opener:      defaultOpener,
	}
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new issue",
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
	cmd.Flags().StringVarP(&opts.Title, "title", "t", "", "issue title")
	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "issue body")
	cmd.Flags().StringVarP(&opts.BodyFile, "body-file", "F", "", "read issue body from file (use '-' for stdin)")
	cmd.Flags().StringSliceVarP(&opts.Labels, "label", "l", nil, "label to apply (repeatable)")
	cmd.Flags().StringSliceVarP(&opts.Assignees, "assignee", "a", nil, "assignee (@me supported, repeatable)")
	cmd.Flags().StringVarP(&opts.MilestoneRef, "milestone", "m", "", "milestone number or title")
	cmd.Flags().StringVar(&opts.Template, "template", "", "issue template name (server feature; ignored if unsupported)")
	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "open the new-issue form in a browser")
	cmd.Flags().BoolVar(&opts.Editor, "editor", false, "compose body via $EDITOR even when --body is set")
	return cmd
}

// Run executes the create flow.
//
//nolint:gocyclo // create unions flag-driven, interactive, and --web modes.
func Run(ctx context.Context, opts *options) error {
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

	body, err := readBodyFile(opts.BodyFile, opts.Body, opts.IO.In)
	if err != nil {
		return err
	}
	opts.Body = body

	if opts.Web {
		url := issueshared.NewIssueWebURL(ref, opts.Title, opts.Body)
		fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", url)
		return opts.Opener(url)
	}

	interactive := opts.Title == "" && opts.IO.IsStdoutTTY()
	if interactive {
		if err := promptInteractive(opts); err != nil {
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
		return errors.New("issue create: title is required (--title or interactive)")
	}

	client, err := opts.HTTPClient(ref.Host)
	if err != nil {
		return err
	}
	assignees, err := issueshared.ExpandMe(ctx, client, issueshared.SplitList(opts.Assignees))
	if err != nil {
		return err
	}

	in := issues.CreateInput{
		Title:     opts.Title,
		Body:      opts.Body,
		Labels:    issueshared.SplitList(opts.Labels),
		Assignees: assignees,
	}
	ic := issues.NewClient(client)
	if mid, err := resolveMilestone(ctx, ic, ref.Owner, ref.Name, opts.MilestoneRef); err != nil {
		return err
	} else if mid > 0 {
		mid := mid
		in.Milestone = &mid
	}

	created, err := ic.Create(ctx, ref.Owner, ref.Name, in)
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

// resolveMilestone turns the --milestone argument into the numeric id
// the server expects. Empty input returns 0 (no milestone). A numeric
// input passes through; non-numeric input lists the repo's milestones
// (state=all) and matches case-insensitively by title.
//
// E-audit E19: pre-fix, --milestone was an int flag — `--milestone v1`
// rejected the input with `strconv.ParseInt: parsing "v1"`. gh accepts
// either form, and so should we.
func resolveMilestone(ctx context.Context, ic *issues.Client, owner, repo, ref string) (int, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return 0, nil
	}
	if n, err := strconv.Atoi(ref); err == nil {
		if n <= 0 {
			return 0, fmt.Errorf("issue create: --milestone must be positive (got %d)", n)
		}
		return n, nil
	}
	list, err := ic.ListMilestones(ctx, owner, repo)
	if err != nil {
		return 0, fmt.Errorf("issue create: resolve milestone %q: %w", ref, err)
	}
	wantedLower := strings.ToLower(ref)
	for _, m := range list {
		if strings.EqualFold(m.Title, ref) {
			return int(m.ID), nil
		}
		_ = wantedLower
	}
	return 0, fmt.Errorf("issue create: no milestone matches %q (case-insensitive title or numeric id)", ref)
}

// promptInteractive walks the user through title/body/labels/assignees.
// The body step launches the editor when the user requested it (or when
// nothing has been preseeded by flags).
func promptInteractive(opts *options) error {
	title, err := opts.Prompter.Input("Title", "")
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
			return "", errors.New("issue create: --body-file=- but stdin is nil")
		}
		b, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("issue create: read stdin: %w", err)
		}
		return string(b), nil
	}
	b, err := os.ReadFile(path) //nolint:gosec // path is user-supplied; this is the documented purpose
	if err != nil {
		return "", fmt.Errorf("issue create: read %q: %w", path, err)
	}
	return string(b), nil
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}

// defaultOpener shells out via osOpenURL for `--web`. Tests inject a
// recording func via the options struct.
var defaultOpener = func(url string) error { return osOpenURL(url) }
