// SPDX-License-Identifier: AGPL-3.0-or-later

// Package edit implements `shithub pr edit`. Patches a PR's scalars
// (title/body/base/milestone) and applies delta-style mutations on
// labels/assignees/reviewers. Labels and assignees route through the
// issues package since shithub mirrors GitHub's "PR ⊂ Issue" model;
// reviewers go through the pulls package's review-request endpoint
// (server expects /requested_reviewers).
package edit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
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
	prshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/pr/shared"
	repocmdshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
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

	Base    string
	baseSet bool

	Labels          []string
	AddLabels       []string
	RemoveLabels    []string
	Assignees       []string
	AddAssignees    []string
	RemoveAssignees []string
	AddReviewers    []string
	RemoveReviewers []string

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
		Use:   "edit [<number-or-url-or-branch>]",
		Short: "Edit a pull request",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Arg = args[0]
			}
			opts.titleSet = c.Flags().Changed("title")
			opts.bodySet = c.Flags().Changed("body") || c.Flags().Changed("body-file")
			opts.baseSet = c.Flags().Changed("base")
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
	cmd.Flags().StringVarP(&opts.Base, "base", "B", "", "new base branch")
	cmd.Flags().StringSliceVar(&opts.Labels, "label", nil, "replace labels (repeatable, CSV)")
	cmd.Flags().StringSliceVar(&opts.AddLabels, "add-label", nil, "add label (repeatable, CSV)")
	cmd.Flags().StringSliceVar(&opts.RemoveLabels, "remove-label", nil, "remove label (repeatable, CSV)")
	cmd.Flags().StringSliceVar(&opts.Assignees, "assignee", nil, "replace assignees (repeatable, @me supported)")
	cmd.Flags().StringSliceVar(&opts.AddAssignees, "add-assignee", nil, "add assignee (repeatable, @me supported)")
	cmd.Flags().StringSliceVar(&opts.RemoveAssignees, "remove-assignee", nil, "remove assignee (repeatable, @me supported)")
	cmd.Flags().StringSliceVar(&opts.AddReviewers, "add-reviewer", nil, "request review (repeatable, CSV)")
	cmd.Flags().StringSliceVar(&opts.RemoveReviewers, "remove-reviewer", nil, "remove review request (repeatable, CSV)")
	cmd.Flags().IntVarP(&opts.Milestone, "milestone", "m", 0, "set milestone number")
	cmd.Flags().BoolVar(&opts.RemoveMilestone, "remove-milestone", false, "clear milestone")
	return cmd
}

// Run executes the edit.
//
//nolint:gocyclo // dispatch over scalars + delta semantics is intentional surface.
func Run(ctx context.Context, opts *options) error {
	if opts.RemoveMilestone && opts.milestoneSet {
		return errors.New("pr edit: --milestone and --remove-milestone are mutually exclusive")
	}
	if len(opts.Labels) > 0 && (len(opts.AddLabels) > 0 || len(opts.RemoveLabels) > 0) {
		return errors.New("pr edit: --label (replace) cannot combine with --add-label/--remove-label")
	}
	if len(opts.Assignees) > 0 && (len(opts.AddAssignees) > 0 || len(opts.RemoveAssignees) > 0) {
		return errors.New("pr edit: --assignee (replace) cannot combine with --add-assignee/--remove-assignee")
	}

	resolver := repocmdshared.Resolver{
		RepoFlag:    opts.Repo,
		Hostname:    opts.Hostname,
		DefaultHost: hostOrDefault(opts.DefaultHost),
		GitRunner:   opts.GitRunner,
	}
	fb, err := resolver.Resolve()
	if err != nil && (opts.Arg == "" || !strings.Contains(opts.Arg, "://")) {
		return err
	}
	if err != nil {
		fb = repocmdshared.RepoRef{}
	}

	body, err := readBodyFile(opts.BodyFile, opts.Body, opts.IO.In)
	if err != nil {
		return err
	}
	opts.Body = body

	client, err := opts.HTTPClient(fb.Host)
	if err != nil {
		return err
	}
	pc := pulls.NewClient(client)

	var ref prshared.PRRef
	if opts.Arg == "" {
		branch := prshared.CurrentBranchFromGit(opts.GitRunner, "")
		if branch == "" {
			return errors.New("pr edit: pass a number/URL/branch (couldn't detect current branch)")
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

	// PR-scalar patch first (title/body/base).
	patch := pulls.EditInput{}
	if opts.titleSet {
		t := opts.Title
		patch.Title = &t
	}
	if opts.bodySet {
		b := opts.Body
		patch.Body = &b
	}
	if opts.baseSet {
		b := opts.Base
		patch.Base = &b
	}
	if !patch.IsEmpty() {
		if _, err := pc.Edit(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number, patch); err != nil {
			return err
		}
	}

	// Issue-shape mutations: labels/assignees/milestone via /issues/{n}.
	ic := issues.NewClient(client)
	issuePatch := issues.EditInput{}
	if opts.milestoneSet && opts.Milestone > 0 {
		m := opts.Milestone
		issuePatch.Milestone = &m
	}
	if opts.RemoveMilestone {
		zero := 0
		issuePatch.Milestone = &zero
	}
	if len(opts.Labels) > 0 {
		issuePatch.Labels = issueshared.SplitList(opts.Labels)
	} else if len(opts.AddLabels) > 0 || len(opts.RemoveLabels) > 0 {
		base, err := currentLabels(ctx, ic, ref)
		if err != nil {
			return err
		}
		issuePatch.Labels = mergeStrings(base, opts.AddLabels, opts.RemoveLabels)
	}
	if len(opts.Assignees) > 0 {
		expanded, err := issueshared.ExpandMe(ctx, client, issueshared.SplitList(opts.Assignees))
		if err != nil {
			return err
		}
		issuePatch.Assignees = expanded
	} else if len(opts.AddAssignees) > 0 || len(opts.RemoveAssignees) > 0 {
		add, err := issueshared.ExpandMe(ctx, client, issueshared.SplitList(opts.AddAssignees))
		if err != nil {
			return err
		}
		rm, err := issueshared.ExpandMe(ctx, client, issueshared.SplitList(opts.RemoveAssignees))
		if err != nil {
			return err
		}
		base, err := currentAssignees(ctx, ic, ref)
		if err != nil {
			return err
		}
		issuePatch.Assignees = mergeStrings(base, add, rm)
	}
	if !issuePatch.IsEmpty() {
		if _, err := ic.Edit(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number, issuePatch); err != nil {
			return err
		}
	}

	// Reviewer ops go through the pulls dedicated endpoint.
	if len(opts.AddReviewers) > 0 {
		if err := requestReviewers(ctx, client, ref, issueshared.SplitList(opts.AddReviewers)); err != nil {
			return err
		}
	}
	if len(opts.RemoveReviewers) > 0 {
		if err := removeReviewers(ctx, client, ref, issueshared.SplitList(opts.RemoveReviewers)); err != nil {
			return err
		}
	}

	// Nothing-to-do guard.
	if patch.IsEmpty() && issuePatch.IsEmpty() && len(opts.AddReviewers) == 0 && len(opts.RemoveReviewers) == 0 {
		return errors.New("pr edit: nothing to do; pass at least one flag")
	}

	fmt.Fprintf(opts.IO.ErrOut, "%s Edited PR #%d\n", opts.IO.SuccessIcon(), ref.Number)
	return nil
}

// currentLabels reads the issue envelope and extracts label names. Used
// by --add-label / --remove-label so we can produce the merged set.
func currentLabels(ctx context.Context, ic *issues.Client, ref prshared.PRRef) ([]string, error) {
	cur, err := ic.View(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(cur.Labels))
	for _, l := range cur.Labels {
		out = append(out, l.Name)
	}
	return out, nil
}

// currentAssignees reads the issue envelope and extracts assignee logins.
func currentAssignees(ctx context.Context, ic *issues.Client, ref prshared.PRRef) ([]string, error) {
	cur, err := ic.View(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(cur.Assignees))
	for _, a := range cur.Assignees {
		out = append(out, a.Login)
	}
	return out, nil
}

// requestReviewers hits POST /pulls/{n}/requested_reviewers with a
// {reviewers: [...]} body. shithub mirrors GitHub's contract here; the
// pulls client doesn't expose a typed method yet (it's a thin wrapper).
func requestReviewers(ctx context.Context, client *api.Client, ref prshared.PRRef, logins []string) error {
	if len(logins) == 0 {
		return nil
	}
	path := fmt.Sprintf("/repos/{owner}/{repo}/pulls/%d/requested_reviewers", ref.Number)
	body := map[string]any{"reviewers": logins}
	return client.REST(ctx, http.MethodPost, path, body, nil,
		api.WithOwner(ref.Repo.Owner), api.WithRepo(ref.Repo.Name))
}

// removeReviewers hits DELETE on the same endpoint with the same body
// (the contract is /requested_reviewers + DELETE + body for removals).
func removeReviewers(ctx context.Context, client *api.Client, ref prshared.PRRef, logins []string) error {
	if len(logins) == 0 {
		return nil
	}
	path := fmt.Sprintf("/repos/{owner}/{repo}/pulls/%d/requested_reviewers", ref.Number)
	body := map[string]any{"reviewers": logins}
	return client.REST(ctx, http.MethodDelete, path, body, nil,
		api.WithOwner(ref.Repo.Owner), api.WithRepo(ref.Repo.Name))
}

// mergeStrings applies add/remove against base case-insensitively.
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

func readBodyFile(path, fallback string, stdin io.Reader) (string, error) {
	if path == "" {
		return fallback, nil
	}
	if path == "-" {
		if stdin == nil {
			return "", errors.New("pr edit: --body-file=- but stdin is nil")
		}
		b, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("pr edit: read stdin: %w", err)
		}
		return string(b), nil
	}
	b, err := os.ReadFile(path) //nolint:gosec // user-supplied path
	if err != nil {
		return "", fmt.Errorf("pr edit: read %q: %w", path, err)
	}
	return string(b), nil
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
