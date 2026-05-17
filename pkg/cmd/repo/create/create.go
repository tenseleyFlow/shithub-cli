// SPDX-License-Identifier: AGPL-3.0-or-later

// Package create implements `shithub repo create`. Two modes:
//
//   - Flag-driven (any of --public/--private/--internal supplied): build a
//     CreateInput from flags and POST; optionally clone or wire --source.
//   - Interactive (TTY only, no visibility flag): prompt the user through
//     name, description, visibility, then ask whether to clone.
//
// --source <dir> attaches an existing local directory to the new repo by
// adding `origin` and (with --push) pushing the current branch.
package create

import (
	"context"
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
	"github.com/tenseleyFlow/shithub-cli/internal/prompter"
	"github.com/tenseleyFlow/shithub-cli/internal/repos"
	"github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

type options struct {
	IO          *iostreams.IOStreams
	Prompter    prompter.Prompter
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string
	GitProtocol func() string
	GitRunner   git.Runner

	NameArg     string
	Description string
	Homepage    string
	Public      bool
	Private     bool
	Internal    bool
	License     string
	Gitignore   string
	AddReadme   bool
	Clone       bool
	Source      string
	Remote      string
	Push        bool
	Template    string
	Hostname    string
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		Prompter:    f.Prompter,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
		GitProtocol: f.GitProtocol,
		Remote:      "origin",
	}
	cmd := &cobra.Command{
		Use:   "create [<name>]",
		Short: "Create a new repository",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.NameArg = args[0]
			}
			if opts.GitRunner == nil {
				if r, err := git.FromPath(); err == nil {
					opts.GitRunner = r
				}
			}
			return Run(c.Context(), opts)
		},
	}
	cmd.Flags().StringVarP(&opts.Description, "description", "d", "", "description of the repository")
	// --homepage intentionally has no short flag — "-h" collides with
	// cobra's auto-registered --help shorthand and panics flag setup.
	// gh accepts gh repo create -h but pays for it by reassigning the
	// help short flag; we keep --help universally accessible.
	cmd.Flags().StringVar(&opts.Homepage, "homepage", "", "URL associated with the repository")
	cmd.Flags().BoolVar(&opts.Public, "public", false, "make the new repository public")
	cmd.Flags().BoolVar(&opts.Private, "private", false, "make the new repository private")
	cmd.Flags().BoolVar(&opts.Internal, "internal", false, "make the new repository internal (org-only)")
	cmd.Flags().StringVarP(&opts.License, "license", "l", "", "specify an Open Source License")
	cmd.Flags().StringVarP(&opts.Gitignore, "gitignore", "g", "", "specify a gitignore template")
	cmd.Flags().BoolVar(&opts.AddReadme, "add-readme", false, "add a README file to the new repository")
	cmd.Flags().BoolVarP(&opts.Clone, "clone", "c", false, "clone the new repository to the current directory")
	cmd.Flags().StringVarP(&opts.Source, "source", "s", "", "path to a local directory to use as the source")
	cmd.Flags().StringVarP(&opts.Remote, "remote", "r", "origin", "name of the new git remote when using --source")
	cmd.Flags().BoolVar(&opts.Push, "push", false, "push the current branch after wiring --source")
	cmd.Flags().StringVarP(&opts.Template, "template", "p", "", "make the new repository from a template <owner/repo>")
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "the shithub host (default: configured host)")
	return cmd
}

// Run executes the create operation.
//
//nolint:gocyclo // create is the union of two modes; the dispatch is the function.
func Run(ctx context.Context, opts *options) error {
	if err := validateFlags(opts); err != nil {
		return err
	}

	host := opts.Hostname
	if host == "" && opts.DefaultHost != nil {
		host = opts.DefaultHost()
	}
	if host == "" {
		host = config.DefaultHost
	}

	owner, name, err := splitNameArg(opts.NameArg)
	if err != nil {
		return err
	}

	// Interactive mode kicks in only when (a) no visibility flag set,
	// (b) we have a TTY, and (c) the user didn't pass --source/--template.
	interactive := !opts.Public && !opts.Private && !opts.Internal && opts.IO.IsStdoutTTY() && opts.Source == "" && opts.Template == ""
	if interactive {
		if err := promptForFlags(opts, &name); err != nil {
			return err
		}
	}

	if name == "" {
		return fmt.Errorf("repo create: name is required")
	}

	client, err := opts.HTTPClient(host)
	if err != nil {
		return err
	}
	rc := repos.NewClient(client)

	created, err := executeCreate(ctx, rc, owner, name, opts)
	if err != nil {
		return err
	}

	// Success line — prefer the server-provided html_url, but fall back
	// to "<owner>/<name> on <host>" when the server skipped that field
	// (audit A11: shithub's create response doesn't currently carry it,
	// so the gh-style URL form is unrenderable). The fallback also
	// behaves well if a future server downgrade drops the URL.
	createdLabel := created.HTMLURL
	if createdLabel == "" {
		createdLabel = created.FullName + " on " + host
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Created repository %s\n", opts.IO.SuccessIcon(), createdLabel)

	if opts.Source != "" {
		if err := wireSource(opts, created); err != nil {
			return err
		}
	}
	if opts.Clone {
		if err := cloneInto(opts, created); err != nil {
			return err
		}
	}
	return nil
}

// validateFlags enforces the visibility mutex and other flag combos.
func validateFlags(opts *options) error {
	count := 0
	for _, b := range []bool{opts.Public, opts.Private, opts.Internal} {
		if b {
			count++
		}
	}
	if count > 1 {
		return fmt.Errorf("repo create: --public, --private, and --internal are mutually exclusive")
	}
	if opts.Source != "" && opts.Clone {
		return fmt.Errorf("repo create: --source and --clone are mutually exclusive")
	}
	if opts.Push && opts.Source == "" {
		return fmt.Errorf("repo create: --push requires --source")
	}
	if opts.Template != "" && (opts.License != "" || opts.Gitignore != "" || opts.AddReadme) {
		return fmt.Errorf("repo create: --template cannot combine with --license/--gitignore/--add-readme")
	}
	return nil
}

// splitNameArg accepts "name", "owner/name", or "". Empty owner means
// "create under the authenticated user".
func splitNameArg(arg string) (owner, name string, err error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return "", "", nil
	}
	if strings.Contains(arg, "/") {
		ref, perr := shared.ParseRepoArg(arg)
		if perr != nil {
			return "", "", perr
		}
		return ref.Owner, ref.Name, nil
	}
	return "", arg, nil
}

// promptForFlags drives the interactive flow. The defaults are deliberately
// conservative: private repo, no README pre-init, no clone — match gh's
// safer defaults so users opt into the riskier choices.
func promptForFlags(opts *options, name *string) error {
	if *name == "" {
		v, err := opts.Prompter.Input("Repository name", "")
		if err != nil {
			return err
		}
		*name = strings.TrimSpace(v)
	}
	if opts.Description == "" {
		desc, err := opts.Prompter.Input("Description", "")
		if err != nil {
			return err
		}
		opts.Description = desc
	}
	vis, err := opts.Prompter.Select("Visibility", "Private", []string{"Public", "Private", "Internal"})
	if err != nil {
		return err
	}
	switch vis {
	case 0:
		opts.Public = true
	case 1:
		opts.Private = true
	case 2:
		opts.Internal = true
	}
	addReadme, err := opts.Prompter.Confirm("Add a README file?", false)
	if err != nil {
		return err
	}
	opts.AddReadme = addReadme
	clone, err := opts.Prompter.Confirm("Clone the new repository?", false)
	if err != nil {
		return err
	}
	opts.Clone = clone
	return nil
}

// executeCreate dispatches across the three creation surfaces — template,
// user, org — based on the resolved owner and template flag.
func executeCreate(ctx context.Context, rc *repos.Client, owner, name string, opts *options) (*repos.Repo, error) {
	if opts.Template != "" {
		ref, err := shared.ParseRepoArg(opts.Template)
		if err != nil {
			return nil, fmt.Errorf("repo create: --template: %w", err)
		}
		return rc.Generate(ctx, ref.Owner, ref.Name, repos.GenerateInput{
			Owner:       owner,
			Name:        name,
			Description: opts.Description,
			Private:     !opts.Public,
		})
	}

	in := repos.CreateInput{
		Name:              name,
		Description:       opts.Description,
		Homepage:          opts.Homepage,
		Private:           opts.Private || (!opts.Public && !opts.Internal),
		AutoInit:          opts.AddReadme,
		GitignoreTemplate: opts.Gitignore,
		LicenseTemplate:   opts.License,
	}
	switch {
	case opts.Public:
		in.Visibility = "public"
	case opts.Internal:
		in.Visibility = "internal"
	default:
		in.Visibility = "private"
	}

	if owner == "" {
		return rc.CreateUser(ctx, in)
	}
	return rc.CreateOrg(ctx, owner, in)
}

// wireSource attaches the local --source dir to the freshly created repo:
// optionally `git init`s, ensures `origin` points at the new URL, and
// pushes when --push is set.
func wireSource(opts *options, created *repos.Repo) error {
	if opts.GitRunner == nil {
		return fmt.Errorf("repo create: git binary required for --source")
	}
	dir := opts.Source
	isRepo, _ := git.IsRepo(opts.GitRunner, dir)
	if !isRepo {
		if err := git.Init(opts.GitRunner, dir, created.DefaultBranch); err != nil {
			return err
		}
	}
	protocol := "https"
	if opts.GitProtocol != nil {
		protocol = opts.GitProtocol()
	}
	url := cloneURLFromRepo(created, protocol)

	has, _ := git.RemoteExists(opts.GitRunner, dir, opts.Remote)
	switch {
	case has:
		if err := git.SetRemoteURL(opts.GitRunner, dir, opts.Remote, url); err != nil {
			return err
		}
	default:
		if err := git.AddRemote(opts.GitRunner, dir, opts.Remote, url); err != nil {
			return err
		}
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Added remote %s -> %s\n", opts.IO.SuccessIcon(), opts.Remote, url)

	if !opts.Push {
		return nil
	}
	branch, err := git.CurrentBranch(opts.GitRunner, dir)
	if err != nil {
		// No commits yet on a fresh init — surface a friendlier hint.
		fmt.Fprintf(opts.IO.ErrOut, "no current branch; commit first then `git push -u %s %s`\n", opts.Remote, created.DefaultBranch)
		return nil
	}
	pushArgs := []string{"push", "-u", opts.Remote, branch}
	return opts.GitRunner.Run(dir, pushArgs, opts.IO.ErrOut, opts.IO.ErrOut)
}

// cloneInto runs git clone for the freshly created repo. Destination is
// the cwd's `<name>/` subdir, matching gh's behavior.
func cloneInto(opts *options, created *repos.Repo) error {
	if opts.GitRunner == nil {
		return fmt.Errorf("repo create: git binary required for --clone")
	}
	protocol := "https"
	if opts.GitProtocol != nil {
		protocol = opts.GitProtocol()
	}
	url := cloneURLFromRepo(created, protocol)

	stdout := opts.IO.Out
	stderr := opts.IO.ErrOut
	if !opts.IO.IsStderrTTY() {
		// In non-TTY scripts, mute progress chatter — only surface errors.
		stdout = io.Discard
	}
	resolved, err := git.Clone(opts.GitRunner, url, "", nil, stdout, stderr)
	if err != nil {
		return err
	}
	if resolved != "" {
		fmt.Fprintf(opts.IO.ErrOut, "%s Cloned to %s/\n", opts.IO.SuccessIcon(), resolved)
	}
	return nil
}

// cloneURLFromRepo prefers the server's reported SSHURL when the user's
// protocol is ssh; defaults to CloneURL (https) otherwise. Falls back to
// a constructed URL when the server hasn't filled either field.
func cloneURLFromRepo(r *repos.Repo, protocol string) string {
	switch strings.ToLower(protocol) {
	case "ssh":
		if r.SSHURL != "" {
			return r.SSHURL
		}
	default:
		if r.CloneURL != "" {
			return r.CloneURL
		}
	}
	// Last-ditch: use the configured default host because the server
	// envelope didn't carry a clone URL.
	owner, name := splitFullName(r.FullName)
	return shared.CloneURL(shared.RepoRef{Owner: owner, Name: name}, protocol)
}

// splitFullName splits "owner/name" without erroring; passes through the
// (empty, full) pair on malformed input so the URL composer still returns
// something usable.
func splitFullName(s string) (string, string) {
	if i := strings.IndexByte(s, '/'); i > 0 {
		return s[:i], s[i+1:]
	}
	return "", s
}

// statFn is the os.Stat indirection so tests can pretend Source dirs
// exist without filesystem state. Currently unused in tests but kept for
// future-proofing the --source check.
var statFn = os.Stat //nolint:gochecknoglobals // single test seam
