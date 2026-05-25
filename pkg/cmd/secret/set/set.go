// SPDX-License-Identifier: AGPL-3.0-or-later

// Package set implements `shithub secret set <name>`. Reads the value
// from stdin, --body, or --body-file; sealed-box-encrypts against the
// scope's public key; PUTs the ciphertext. If the public-key endpoint
// returns 404 (shithub server pre-S41c migration) we fall back to
// plaintext PUT with a stderr warning so users aren't silently locked
// out of CI configuration before the migration ships.
package set

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
	"github.com/tenseleyFlow/shithub-cli/internal/crypt"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/repos"
	"github.com/tenseleyFlow/shithub-cli/internal/secrets"
	repocmdshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

type options struct {
	IO          *iostreams.IOStreams
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string
	GitRunner   git.Runner

	Name     string
	Repo     string
	Hostname string
	Org      string
	App      string

	Body         string
	BodySet      bool
	BodyFile     string
	Visibility   string
	SelectedRepo []string
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
		App:         "actions",
	}
	cmd := &cobra.Command{
		Use:   "set <name>",
		Short: "Create or update a secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts.Name = args[0]
			opts.BodySet = c.Flags().Changed("body")
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
	cmd.Flags().StringVarP(&opts.Org, "org", "o", "", "set an org-level secret")
	cmd.Flags().StringVar(&opts.App, "app", "actions", "scope: actions (only supported app today)")
	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "secret value (alternatively read from stdin)")
	cmd.Flags().StringVarP(&opts.BodyFile, "body-file", "f", "", "read secret value from a file (use '-' for stdin)")
	cmd.Flags().StringVar(&opts.Visibility, "visibility", "private", "org visibility: all | private | selected")
	cmd.Flags().StringSliceVarP(&opts.SelectedRepo, "repos", "r", nil, "selected repos for --visibility=selected (comma-separated names)")
	return cmd
}

// Run does the heavy lifting: source → encrypt → PUT.
func Run(ctx context.Context, opts *options) error {
	if opts.App != "" && opts.App != "actions" {
		return fmt.Errorf("secret set: only --app actions is supported (got %q)", opts.App)
	}
	value, err := readValue(opts)
	if err != nil {
		return err
	}

	switch {
	case opts.Org != "":
		return setOrgSecret(ctx, opts, value)
	default:
		return setRepoSecret(ctx, opts, value)
	}
}

func setRepoSecret(ctx context.Context, opts *options, value []byte) error {
	ref, err := repocmdshared.Resolver{
		RepoFlag:    opts.Repo,
		Hostname:    opts.Hostname,
		DefaultHost: hostOrDefault(opts.DefaultHost),
		GitRunner:   opts.GitRunner,
	}.Resolve()
	if err != nil {
		return err
	}
	client, err := opts.HTTPClient(ref.Host)
	if err != nil {
		return err
	}
	sc := secrets.NewClient(client)

	input, err := buildSetInput(ctx, opts.IO.ErrOut, value, func() (*secrets.PublicKey, error) {
		return sc.GetRepoPublicKey(ctx, ref.Owner, ref.Name)
	})
	if err != nil {
		return err
	}
	// I32 (audit): pre-fix `secret set` printed "Set secret X" whether
	// it was the first write or an update. variable set already
	// distinguishes Created vs Updated — secrets should match. Probe
	// existence with ListRepoSecrets (cheap; the surface returns
	// metadata only, no plaintext) so the success line picks the
	// right verb.
	existed := repoSecretExists(ctx, sc, ref.Owner, ref.Name, opts.Name)
	if err := sc.PutRepoSecret(ctx, ref.Owner, ref.Name, opts.Name, input); err != nil {
		return err
	}
	verb := "Created"
	if existed {
		verb = "Updated"
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s %s secret %s in %s/%s\n",
		opts.IO.SuccessIcon(), verb, opts.Name, ref.Owner, ref.Name)
	return nil
}

// repoSecretExists returns true when a secret of this name is already
// registered on the repo. Best-effort: a list-secrets RPC failure
// downgrades to "treat as create" so the success line still ships even
// when the server is briefly cranky.
func repoSecretExists(ctx context.Context, sc *secrets.Client, owner, repo, name string) bool {
	rows, err := sc.ListRepoSecrets(ctx, owner, repo)
	if err != nil {
		return false
	}
	for _, s := range rows {
		if s.Name == name {
			return true
		}
	}
	return false
}

// orgSecretExists is the org-side mirror of repoSecretExists.
func orgSecretExists(ctx context.Context, sc *secrets.Client, org, name string) bool {
	rows, err := sc.ListOrgSecrets(ctx, org)
	if err != nil {
		return false
	}
	for _, s := range rows {
		if s.Name == name {
			return true
		}
	}
	return false
}

func setOrgSecret(ctx context.Context, opts *options, value []byte) error {
	client, err := opts.HTTPClient(hostOrDefault(opts.DefaultHost))
	if err != nil {
		return err
	}
	sc := secrets.NewClient(client)

	input, err := buildSetInput(ctx, opts.IO.ErrOut, value, func() (*secrets.PublicKey, error) {
		return sc.GetOrgPublicKey(ctx, opts.Org)
	})
	if err != nil {
		return err
	}
	visibility, ids, err := resolveOrgVisibility(ctx, client, opts)
	if err != nil {
		return err
	}
	input.Visibility = visibility
	input.SelectedRepositoryIDs = ids
	// I32 (audit): Created/Updated wording for org secrets too.
	existed := orgSecretExists(ctx, sc, opts.Org, opts.Name)
	if err := sc.PutOrgSecret(ctx, opts.Org, opts.Name, input); err != nil {
		return err
	}
	verb := "Created"
	if existed {
		verb = "Updated"
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s %s secret %s in org %s (%s)\n",
		opts.IO.SuccessIcon(), verb, opts.Name, opts.Org, visibility)
	return nil
}

// buildSetInput fetches the public key (when available) and produces
// the SetSecretInput. The notFound branch is the explicit
// "send plaintext + warn" fallback for shithub pre-S41c.
func buildSetInput(_ context.Context, errOut io.Writer, value []byte, getKey func() (*secrets.PublicKey, error)) (secrets.SetSecretInput, error) {
	key, err := getKey()
	switch {
	case err == nil:
		ciphertext, encErr := crypt.SealAnonymous(key.Key, value)
		if encErr != nil {
			return secrets.SetSecretInput{}, encErr
		}
		return secrets.SetSecretInput{EncryptedValue: ciphertext, KeyID: key.KeyID}, nil
	case isNotFound(err):
		fmt.Fprintln(errOut, "warning: shithub host has no sealed-box public key; sending secret in plaintext. "+
			"This will be removed once shithub server S41c migration ships.")
		return secrets.SetSecretInput{PlaintextValue: string(value)}, nil
	default:
		return secrets.SetSecretInput{}, err
	}
}

func isNotFound(err error) bool {
	var nf *api.NotFoundError
	return errors.As(err, &nf)
}

// resolveOrgVisibility validates --visibility and converts --repos
// names → server-side IDs by looking each up via /repos/{org}/{name}.
func resolveOrgVisibility(ctx context.Context, client *api.Client, opts *options) (string, []int64, error) {
	v := strings.ToLower(opts.Visibility)
	switch v {
	case "all", "private":
		if len(opts.SelectedRepo) > 0 {
			return "", nil, fmt.Errorf("secret set: --repos requires --visibility=selected")
		}
		return v, nil, nil
	case "selected":
		if len(opts.SelectedRepo) == 0 {
			return "", nil, errors.New("secret set: --visibility=selected requires --repos")
		}
		rc := repos.NewClient(client)
		ids := make([]int64, 0, len(opts.SelectedRepo))
		for _, name := range opts.SelectedRepo {
			r, err := rc.View(ctx, opts.Org, strings.TrimSpace(name))
			if err != nil {
				return "", nil, fmt.Errorf("secret set: resolve %s/%s: %w", opts.Org, name, err)
			}
			ids = append(ids, r.ID)
		}
		return v, ids, nil
	default:
		return "", nil, fmt.Errorf("secret set: --visibility must be one of all|private|selected (got %q)", v)
	}
}

// readValue picks the value source: --body wins, then --body-file, then
// stdin. An empty --body is honored (gh treats it the same way).
func readValue(opts *options) ([]byte, error) {
	switch {
	case opts.BodySet:
		return []byte(opts.Body), nil
	case opts.BodyFile == "-":
		return io.ReadAll(opts.IO.In)
	case opts.BodyFile != "":
		return os.ReadFile(opts.BodyFile) //nolint:gosec // user-supplied path
	default:
		blob, err := io.ReadAll(opts.IO.In)
		if err != nil {
			return nil, fmt.Errorf("secret set: read stdin: %w", err)
		}
		if len(blob) == 0 {
			return nil, errors.New("secret set: no value provided (use --body, --body-file, or pipe via stdin)")
		}
		return blob, nil
	}
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
