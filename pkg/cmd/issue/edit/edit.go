// SPDX-License-Identifier: AGPL-3.0-or-later

// Package edit implements `shithub issue edit`. Supports both
// wholesale-replace semantics (--add-label/--remove-label/--add-assignee/
// --remove-assignee with --label/--assignee replacing entirely) and the
// title/body/milestone scalars. The set diff for labels and assignees
// happens client-side because the server PATCH expects the full final list.
package edit

import (
	"context"
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
	issueshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/issue/shared"
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

	Title    string
	titleSet bool

	Body     string
	BodyFile string
	bodySet  bool

	Labels          []string
	AddLabels       []string
	RemoveLabels    []string
	Assignees       []string
	AddAssignees    []string
	RemoveAssignees []string

	Milestone       int
	milestoneSet    bool
	RemoveMilestone bool
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
	}
	cmd := &cobra.Command{
		Use:   "edit <number-or-url>",
		Short: "Edit an issue",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts.Arg = args[0]
			opts.titleSet = c.Flags().Changed("title")
			opts.bodySet = c.Flags().Changed("body") || c.Flags().Changed("body-file")
			opts.milestoneSet = c.Flags().Changed("milestone")
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
	cmd.Flags().StringVarP(&opts.Title, "title", "t", "", "new title")
	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "new body")
	cmd.Flags().StringVarP(&opts.BodyFile, "body-file", "F", "", "read new body from file (use '-' for stdin)")
	cmd.Flags().StringSliceVar(&opts.Labels, "label", nil, "replace labels (repeatable, CSV)")
	cmd.Flags().StringSliceVar(&opts.AddLabels, "add-label", nil, "add label (repeatable, CSV)")
	cmd.Flags().StringSliceVar(&opts.RemoveLabels, "remove-label", nil, "remove label (repeatable, CSV)")
	cmd.Flags().StringSliceVar(&opts.Assignees, "assignee", nil, "replace assignees (repeatable, @me supported)")
	cmd.Flags().StringSliceVar(&opts.AddAssignees, "add-assignee", nil, "add assignee (repeatable, @me supported)")
	cmd.Flags().StringSliceVar(&opts.RemoveAssignees, "remove-assignee", nil, "remove assignee (repeatable, @me supported)")
	cmd.Flags().IntVarP(&opts.Milestone, "milestone", "m", 0, "set milestone number")
	cmd.Flags().BoolVar(&opts.RemoveMilestone, "remove-milestone", false, "clear the issue's milestone")
	// H16: refuse --body alongside --body-file. See pr/edit for the
	// full rationale — pre-fix the file silently won.
	cmd.MarkFlagsMutuallyExclusive("body", "body-file")
	return cmd
}

// Run executes the edit operation.
//
//nolint:gocyclo // union of scalars + delta semantics for labels/assignees.
func Run(ctx context.Context, opts *options) error {
	if opts.RemoveMilestone && opts.milestoneSet {
		return fmt.Errorf("issue edit: --milestone and --remove-milestone are mutually exclusive")
	}
	if len(opts.Labels) > 0 && (len(opts.AddLabels) > 0 || len(opts.RemoveLabels) > 0) {
		return fmt.Errorf("issue edit: --label (replace) cannot combine with --add-label/--remove-label")
	}
	if len(opts.Assignees) > 0 && (len(opts.AddAssignees) > 0 || len(opts.RemoveAssignees) > 0) {
		return fmt.Errorf("issue edit: --assignee (replace) cannot combine with --add-assignee/--remove-assignee")
	}

	ref, err := resolve(opts)
	if err != nil {
		return err
	}

	body, err := readBodyFile(opts.BodyFile, opts.Body, opts.IO.In)
	if err != nil {
		return err
	}
	opts.Body = body

	client, err := opts.HTTPClient(ref.Repo.Host)
	if err != nil {
		return err
	}
	ic := issues.NewClient(client)
	pc := pulls.NewClient(client)

	// H2: cross-namespace verb routing. PATCH on a PR via /issues
	// surfaces a raw 422 about the shared issue/PR table; redirect
	// instead.
	if err := crosskind.Check(ctx, ic, pc, ref.Repo.Owner, ref.Repo.Name, ref.Number, "issue", "issue edit", "edit"); err != nil {
		return err
	}

	// Resolve labels and assignees against the current issue when the
	// user passed delta flags (we need the existing list to mutate).
	patch := issues.EditInput{}
	if opts.titleSet {
		t := opts.Title
		patch.Title = &t
	}
	if opts.bodySet {
		b := opts.Body
		patch.Body = &b
	}
	if opts.milestoneSet && opts.Milestone > 0 {
		m := opts.Milestone
		patch.Milestone = &m
	}
	if opts.RemoveMilestone {
		zero := 0
		patch.Milestone = &zero
	}

	if len(opts.Labels) > 0 {
		patch.Labels = issueshared.SplitList(opts.Labels)
	} else if len(opts.AddLabels) > 0 || len(opts.RemoveLabels) > 0 {
		current, err := ic.View(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number)
		if err != nil {
			return err
		}
		base := make([]string, 0, len(current.Labels))
		for _, l := range current.Labels {
			base = append(base, l.Name)
		}
		patch.Labels = mergeStrings(base, opts.AddLabels, opts.RemoveLabels)
	}

	if len(opts.Assignees) > 0 {
		raw := issueshared.SplitList(opts.Assignees)
		expanded, err := issueshared.ExpandMe(ctx, client, raw)
		if err != nil {
			return err
		}
		patch.Assignees = expanded
	} else if len(opts.AddAssignees) > 0 || len(opts.RemoveAssignees) > 0 {
		add, err := issueshared.ExpandMe(ctx, client, issueshared.SplitList(opts.AddAssignees))
		if err != nil {
			return err
		}
		rm, err := issueshared.ExpandMe(ctx, client, issueshared.SplitList(opts.RemoveAssignees))
		if err != nil {
			return err
		}
		current, err := ic.View(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number)
		if err != nil {
			return err
		}
		base := make([]string, 0, len(current.Assignees))
		for _, a := range current.Assignees {
			base = append(base, a.Login)
		}
		patch.Assignees = mergeStrings(base, add, rm)
	}

	if patch.IsEmpty() && patch.Labels == nil && patch.Assignees == nil {
		return fmt.Errorf("issue edit: nothing to do; pass at least one flag")
	}

	if _, err := ic.Edit(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number, patch); err != nil {
		return err
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Edited #%d\n", opts.IO.SuccessIcon(), ref.Number)
	return nil
}

// mergeStrings applies add/remove against base in a case-insensitive way,
// preserving the base ordering followed by new adds.
func mergeStrings(base, add, remove []string) []string {
	rm := map[string]struct{}{}
	for _, r := range remove {
		rm[strings.ToLower(strings.TrimSpace(r))] = struct{}{}
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(base)+len(add))
	keep := func(s string) {
		key := strings.ToLower(strings.TrimSpace(s))
		if key == "" {
			return
		}
		if _, drop := rm[key]; drop {
			return
		}
		if _, dup := seen[key]; dup {
			return
		}
		seen[key] = struct{}{}
		out = append(out, s)
	}
	for _, s := range base {
		keep(s)
	}
	for _, s := range add {
		keep(s)
	}
	return out
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
			return "", fmt.Errorf("issue edit: --body-file=- but stdin is nil")
		}
		b, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("issue edit: read stdin: %w", err)
		}
		return string(b), nil
	}
	b, err := os.ReadFile(path) //nolint:gosec // user-supplied path is the documented purpose
	if err != nil {
		return "", fmt.Errorf("issue edit: read %q: %w", path, err)
	}
	return string(b), nil
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
