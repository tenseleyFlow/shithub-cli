// SPDX-License-Identifier: AGPL-3.0-or-later

// Package list implements `shithub repo list`. Lists the authenticated
// user's repos by default; an optional positional arg selects another
// user or org. Filters mirror gh: visibility, fork, archived, language,
// topic, source-only, sort, order, limit.
package list

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/output"
	"github.com/tenseleyFlow/shithub-cli/internal/repos"
	"github.com/tenseleyFlow/shithub-cli/internal/tableprinter"
)

// MaxLimit caps `--limit` to a sane upper bound so a typo doesn't kick
// off a 50k-page traversal.
const MaxLimit = 1000

// DefaultLimit matches gh's `repo list` default.
const DefaultLimit = 30

type options struct {
	IO          *iostreams.IOStreams
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string

	Owner    string // positional arg (user or org); empty = authenticated user
	Hostname string

	Visibility string
	Fork       bool
	Archived   bool
	NoArchived bool
	Language   string
	Topic      string
	SourceOnly bool

	Sort      string
	Direction string
	Limit     int
	Web       bool

	// Opener is overridden in tests.
	Opener func(url string) error

	Exporter output.Options
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
		Limit:       DefaultLimit,
		Opener:      defaultOpener,
	}
	cmd := &cobra.Command{
		Use:   "list [<owner>]",
		Short: "List repositories",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Owner = args[0]
			}
			return Run(c.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "the shithub host (default: configured host)")
	cmd.Flags().StringVar(&opts.Visibility, "visibility", "", "filter by visibility: {public|private}")
	cmd.Flags().BoolVar(&opts.Fork, "fork", false, "show only forks")
	cmd.Flags().BoolVar(&opts.Archived, "archived", false, "show only archived repositories")
	cmd.Flags().BoolVar(&opts.NoArchived, "no-archived", false, "omit archived repositories")
	cmd.Flags().StringVar(&opts.Language, "language", "", "filter by primary language")
	cmd.Flags().StringVar(&opts.Topic, "topic", "", "filter by topic")
	cmd.Flags().BoolVar(&opts.SourceOnly, "source", false, "show only non-fork repositories")
	cmd.Flags().StringVar(&opts.Sort, "sort", "updated", "sort field: {created|updated|pushed|name}")
	cmd.Flags().StringVar(&opts.Direction, "order", "desc", "sort order: {asc|desc}")
	cmdutil.AddLimitFlag(cmd, &opts.Limit, DefaultLimit, "maximum number of repositories to list (max 1000)")
	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "open the repository list in a browser")
	output.AddFlags(cmd, &opts.Exporter)
	// TODO(D3a-followup): add output.MarkWebMutuallyExclusive(cmd)
	// once D3a (output-web-mutex helper) lands on trunk.
	return cmd
}

// Run executes the listing operation.
func Run(ctx context.Context, opts *options) error {
	if opts.Fork && opts.SourceOnly {
		return fmt.Errorf("repo list: --fork and --source are mutually exclusive")
	}
	if opts.Archived && opts.NoArchived {
		return fmt.Errorf("repo list: --archived and --no-archived are mutually exclusive")
	}
	if err := cmdutil.ValidateLimit(opts.Limit); err != nil {
		return err
	}
	if opts.Limit > MaxLimit {
		opts.Limit = MaxLimit
	}

	host := opts.Hostname
	if host == "" && opts.DefaultHost != nil {
		host = opts.DefaultHost()
	}

	// C23: --web sends the user to the host's repo-list page for the
	// supplied (or authenticated) user/org. gh has this flag; we
	// were the odd ones out.
	if opts.Web {
		who := opts.Owner
		if who == "" {
			// Best-effort: defer to the host's "your repos" page;
			// the server's UI handles the auth redirect.
			who = "?tab=repositories"
		}
		base := "https://"
		if host != "" {
			base += host
		}
		var url string
		if strings.HasPrefix(who, "?") {
			url = base + "/" + who
		} else {
			url = base + "/" + who + "?tab=repositories"
		}
		fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", url)
		if opts.Opener == nil {
			return nil
		}
		return opts.Opener(url)
	}
	client, err := opts.HTTPClient(host)
	if err != nil {
		return err
	}
	rc := repos.NewClient(client)

	listOpts := repos.ListOptions{
		Visibility: opts.Visibility,
		Sort:       toAPISort(opts.Sort),
		Direction:  opts.Direction,
		Type:       opts.repoType(),
		Limit:      opts.Limit,
	}

	var all []repos.Repo
	if opts.Owner == "" {
		all, err = rc.ListAuthenticated(ctx, listOpts)
	} else {
		// I42 (audit): pre-fix `repo list <org>` only ever tried
		// /users/{X}/repos. For an org owner the server 404s with
		// "user not found", and the empty-success fallback below
		// never ran. Now we also fall back to /orgs/{X}/repos on a
		// NotFoundError — this matches the audit reproducer (org has
		// 24 repos, CLI was reporting zero / bailing on 404).
		//
		// Two fallback paths: (1) user-success-but-empty (caller
		// probably meant the org form), and (2) user-404 (the owner
		// isn't a user). Both end up trying the org endpoint.
		all, err = rc.ListUser(ctx, opts.Owner, listOpts)
		switch {
		case err == nil && len(all) == 0:
			all, err = rc.ListOrg(ctx, opts.Owner, listOpts)
		case api.IsNotFoundError(err):
			orgRepos, orgErr := rc.ListOrg(ctx, opts.Owner, listOpts)
			if orgErr == nil {
				all, err = orgRepos, nil
			}
			// If the org path also 404s, keep the original user-side
			// error — the owner doesn't exist as either.
		}
	}
	if err != nil {
		return err
	}

	filtered := applyClientFilters(all, opts)

	if opts.Exporter.Active() {
		return output.Export(opts.IO.Out, opts.Exporter, exporter{}, filtered, opts.IO.IsStdoutTTY())
	}
	renderTable(opts.IO, filtered)
	return nil
}

// toAPISort maps the gh-style sort flag values onto the shithub REST
// contract's accepted set. `name` becomes `full_name`; everything else
// passes through.
func toAPISort(s string) string {
	if s == "name" {
		return "full_name"
	}
	return s
}

// repoType maps the cluster of "show only X" flags onto the server's
// `type` query parameter. Returns empty when no flag selects a subset.
func (o *options) repoType() string {
	switch {
	case o.Fork:
		return "fork"
	case o.SourceOnly:
		return "source"
	}
	return ""
}

// applyClientFilters narrows the server's result with the filters the
// /repos endpoint doesn't natively support (language, topic, archived).
// Doing this client-side keeps the server contract minimal.
func applyClientFilters(in []repos.Repo, opts *options) []repos.Repo {
	out := make([]repos.Repo, 0, len(in))
	for _, r := range in {
		switch {
		case opts.Archived && !r.Archived:
			continue
		case opts.NoArchived && r.Archived:
			continue
		case opts.Language != "" && !strings.EqualFold(r.Language, opts.Language):
			continue
		case opts.Topic != "" && !containsTopic(r.Topics, opts.Topic):
			continue
		}
		out = append(out, r)
	}
	return out
}

// containsTopic is a case-insensitive contains check on the repo's topics.
func containsTopic(topics []string, needle string) bool {
	for _, t := range topics {
		if strings.EqualFold(t, needle) {
			return true
		}
	}
	return false
}

// renderTable emits a tabular listing for TTY output. Non-TTY falls
// through the tableprinter's tab-separated branch automatically.
func renderTable(io *iostreams.IOStreams, rs []repos.Repo) {
	if len(rs) == 0 {
		fmt.Fprintln(io.ErrOut, "no repositories found")
		return
	}
	tp := tableprinter.New(io.Out, io.IsStdoutTTY(), io.TerminalWidth())
	for _, r := range rs {
		tp.AddRow(
			r.FullName,
			truncate(r.Description, 80),
			badgeString(r),
			r.UpdatedAt.Format("2006-01-02"),
		)
	}
	_ = tp.Render()
}

// badgeString reduces visibility/state into a single column for the table.
func badgeString(r repos.Repo) string {
	parts := []string{}
	if r.Private {
		parts = append(parts, "private")
	} else if r.Visibility != "" {
		parts = append(parts, r.Visibility)
	} else {
		parts = append(parts, "public")
	}
	if r.Fork {
		parts = append(parts, "fork")
	}
	if r.Archived {
		parts = append(parts, "archived")
	}
	return strings.Join(parts, ",")
}

// truncate clips s to n runes (not bytes) with an ellipsis tail. Used to
// keep the description column from blowing out the row width.
func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}

// defaultOpener is the prod-time no-op; iostreams will plumb a real
// browser launcher in a later sprint. Tests override.
var defaultOpener = func(_ string) error { return nil }
