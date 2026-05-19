// SPDX-License-Identifier: AGPL-3.0-or-later

// Package edit implements `shithub repo edit`. Patches the repo's
// description/homepage/topics/default-branch/visibility/feature flags.
// Topic mutation supports replace (--topics), add (--add-topic), remove
// (--remove-topic); the three are composable.
package edit

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/repos"
	"github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

type options struct {
	IO          *iostreams.IOStreams
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string
	GitRunner   git.Runner

	RepoArg  string
	Repo     string
	Hostname string

	// Plain-value flags (use *string so we know "user passed empty" vs "not set").
	Description       *string
	Homepage          *string
	DefaultBranch     *string
	Visibility        *string
	AllowForking      *bool
	AllowUpdateBranch *bool
	EnableIssues      *bool
	EnableProjects    *bool
	EnableWiki        *bool
	EnableDiscussions *bool

	Topics       []string
	AddTopics    []string
	RemoveTopics []string

	// Raw flag values from cobra (kept private so we can tell whether
	// the user explicitly passed them).
	descRaw     string
	homepageRaw string
	branchRaw   string
	visRaw      string
	descSet     bool
	homepageSet bool
	branchSet   bool
	visSet      bool
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
	}
	cmd := &cobra.Command{
		Use:   "edit [<owner>/<repo>]",
		Short: "Edit repository settings",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.RepoArg = args[0]
			}
			if opts.GitRunner == nil {
				if r, err := git.FromPath(); err == nil {
					opts.GitRunner = r
				}
			}
			opts.descSet = c.Flags().Changed("description")
			opts.homepageSet = c.Flags().Changed("homepage")
			opts.branchSet = c.Flags().Changed("default-branch")
			opts.visSet = c.Flags().Changed("visibility")
			if opts.descSet {
				v := opts.descRaw
				opts.Description = &v
			}
			if opts.homepageSet {
				v := opts.homepageRaw
				opts.Homepage = &v
			}
			if opts.branchSet {
				v := opts.branchRaw
				opts.DefaultBranch = &v
			}
			if opts.visSet {
				v := opts.visRaw
				opts.Visibility = &v
			}
			return Run(c.Context(), opts)
		},
	}
	cmd.Flags().StringVarP(&opts.Repo, "repo", "R", "", "select another repository using the [HOST/]OWNER/REPO format")
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "the shithub host (default: configured host)")

	cmd.Flags().StringVarP(&opts.descRaw, "description", "d", "", "description of the repository")
	// --homepage has no short flag: "-h" collides with cobra's
	// --help shorthand. See pkg/cmd/repo/create/create.go for the
	// same rationale.
	cmd.Flags().StringVar(&opts.homepageRaw, "homepage", "", "URL associated with the repository")
	cmd.Flags().StringVar(&opts.branchRaw, "default-branch", "", "default branch name")
	cmd.Flags().StringVar(&opts.visRaw, "visibility", "", "visibility: {public|private|internal}")

	cmd.Flags().StringSliceVar(&opts.Topics, "topics", nil, "replace topics with this comma-separated list")
	cmd.Flags().StringSliceVar(&opts.AddTopics, "add-topic", nil, "add a topic (repeatable)")
	cmd.Flags().StringSliceVar(&opts.RemoveTopics, "remove-topic", nil, "remove a topic (repeatable)")

	addBoolPtrFlag(cmd, &opts.AllowForking, "allow-forking", "allow forking of the repository")
	addBoolPtrFlag(cmd, &opts.AllowUpdateBranch, "allow-update-branch", "allow auto-updating of pull request branches")
	addBoolPtrFlag(cmd, &opts.EnableIssues, "enable-issues", "enable issues")
	addBoolPtrFlag(cmd, &opts.EnableProjects, "enable-projects", "enable projects (no-op on shithub for now)")
	addBoolPtrFlag(cmd, &opts.EnableWiki, "enable-wiki", "enable wiki (no-op on shithub for now)")
	addBoolPtrFlag(cmd, &opts.EnableDiscussions, "enable-discussions", "enable discussions (no-op on shithub for now)")
	return cmd
}

// Run executes the edit operation.
//
//nolint:gocyclo // unavoidable dispatch: edit composes many independent settings.
func Run(ctx context.Context, opts *options) error {
	if opts.RepoArg != "" && opts.Repo != "" {
		return fmt.Errorf("repo edit: pass either a positional arg or -R, not both")
	}
	flagOrArg := opts.Repo
	if flagOrArg == "" {
		flagOrArg = opts.RepoArg
	}
	resolver := shared.Resolver{
		RepoFlag:    flagOrArg,
		Hostname:    opts.Hostname,
		DefaultHost: hostOrDefault(opts.DefaultHost),
		GitRunner:   opts.GitRunner,
	}
	ref, err := resolver.Resolve()
	if err != nil {
		return err
	}

	client, err := opts.HTTPClient(ref.Host)
	if err != nil {
		return err
	}
	rc := repos.NewClient(client)

	patch := repos.EditInput{
		Name:              nil,
		Description:       opts.Description,
		Homepage:          opts.Homepage,
		DefaultBranch:     opts.DefaultBranch,
		Visibility:        normalizeVisibilityPtr(opts.Visibility),
		Private:           privateFromVisibility(opts.Visibility),
		AllowForking:      opts.AllowForking,
		AllowUpdateBranch: opts.AllowUpdateBranch,
		HasIssues:         opts.EnableIssues,
		HasProjects:       opts.EnableProjects,
		HasWiki:           opts.EnableWiki,
		HasDiscussions:    opts.EnableDiscussions,
	}

	if patch.IsEmpty() && len(opts.Topics) == 0 && len(opts.AddTopics) == 0 && len(opts.RemoveTopics) == 0 {
		return fmt.Errorf("repo edit: nothing to do; pass at least one flag")
	}

	if !patch.IsEmpty() {
		if _, err := rc.Edit(ctx, ref.Owner, ref.Name, patch); err != nil {
			return err
		}
	}

	if len(opts.Topics) > 0 {
		if _, err := rc.ReplaceTopics(ctx, ref.Owner, ref.Name, dedupe(opts.Topics)); err != nil {
			return err
		}
	} else if len(opts.AddTopics) > 0 || len(opts.RemoveTopics) > 0 {
		current, err := rc.ListTopics(ctx, ref.Owner, ref.Name)
		if err != nil {
			return err
		}
		updated := mergeTopics(current, opts.AddTopics, opts.RemoveTopics)
		if _, err := rc.ReplaceTopics(ctx, ref.Owner, ref.Name, updated); err != nil {
			return err
		}
	}

	fmt.Fprintf(opts.IO.ErrOut, "%s Updated %s\n", opts.IO.SuccessIcon(), ref.FullName())
	return nil
}

// normalizeVisibilityPtr trims and validates the visibility string. Returns
// nil when the user didn't pass --visibility; the server enforces the
// final allowed set per its policy.
func normalizeVisibilityPtr(v *string) *string {
	if v == nil {
		return nil
	}
	trimmed := strings.ToLower(strings.TrimSpace(*v))
	return &trimmed
}

// privateFromVisibility returns a non-nil *bool only when --visibility was
// set; lets the server interpret the legacy private flag for clients that
// haven't migrated to the visibility string yet.
func privateFromVisibility(v *string) *bool {
	if v == nil {
		return nil
	}
	private := strings.ToLower(strings.TrimSpace(*v)) == "private"
	return &private
}

// mergeTopics applies add/remove against an existing list, returning a
// dedupe'd result in the original order followed by the new adds.
func mergeTopics(existing, add, remove []string) []string {
	out := make([]string, 0, len(existing)+len(add))
	seen := map[string]struct{}{}
	rmSet := map[string]struct{}{}
	for _, r := range remove {
		rmSet[strings.ToLower(r)] = struct{}{}
	}
	keep := func(t string) {
		key := strings.ToLower(t)
		if _, drop := rmSet[key]; drop {
			return
		}
		if _, dup := seen[key]; dup {
			return
		}
		seen[key] = struct{}{}
		out = append(out, t)
	}
	for _, t := range existing {
		keep(t)
	}
	for _, t := range add {
		keep(t)
	}
	return out
}

// dedupe removes case-insensitive duplicates while preserving order.
func dedupe(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, s := range in {
		key := strings.ToLower(strings.TrimSpace(s))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, s)
	}
	return out
}

// addBoolPtrFlag wires a *bool flag whose presence carries through to the
// EditInput as a non-nil pointer. cobra's BoolVar can't do that natively
// because the zero value is indistinguishable from "not set", so we use
// a small custom Value.
//
// E-audit E14: NoOptDefVal makes the bare flag form (`--enable-issues`)
// equivalent to `--enable-issues=true`, matching gh's behavior. Without
// this users had to spell `--enable-issues=true` explicitly and the
// help text gave no hint.
func addBoolPtrFlag(cmd *cobra.Command, dest **bool, name, usage string) {
	cmd.Flags().Var(&boolPtrValue{dest: dest}, name, usage)
	cmd.Flags().Lookup(name).NoOptDefVal = "true"
}

// boolPtrValue is a pflag.Value that records whether it was set, so the
// EditInput sees nil when the user didn't pass the flag.
type boolPtrValue struct {
	dest **bool
}

func (b *boolPtrValue) String() string {
	if b == nil || b.dest == nil || *b.dest == nil {
		return ""
	}
	if **b.dest {
		return "true"
	}
	return "false"
}

func (b *boolPtrValue) Set(s string) error {
	switch strings.ToLower(s) {
	case "1", "true", "t", "yes", "y":
		v := true
		*b.dest = &v
		return nil
	case "0", "false", "f", "no", "n":
		v := false
		*b.dest = &v
		return nil
	}
	return fmt.Errorf("invalid bool %q", s)
}

func (b *boolPtrValue) Type() string { return "bool" }

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
