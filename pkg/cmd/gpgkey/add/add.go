// SPDX-License-Identifier: AGPL-3.0-or-later

// Package add implements `shithub gpg-key add`. Reads an ASCII-armored
// public key from a file or stdin and uploads it via
// POST /user/gpg_keys.
package add

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/key"
	"github.com/tenseleyFlow/shithub-cli/internal/keys"
)

type options struct {
	IO          *iostreams.IOStreams
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string

	Path     string
	Name     string
	Hostname string
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
	}
	cmd := &cobra.Command{
		Use:   "add [<key-file>]",
		Short: "Add a GPG public key to your shithub account",
		Long: `Read an ASCII-armored OpenPGP public key from a file (or "-" for stdin)
and upload it to the authenticated user's account.

Export an armored key with:
  gpg --armor --export <key-id>

The CLI refuses to upload a private key — only the public block
(` + "`-----BEGIN PGP PUBLIC KEY BLOCK-----`" + `) is accepted.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Path = args[0]
			}
			return Run(c.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "the shithub host (default: configured host)")
	cmd.Flags().StringVarP(&opts.Name, "name", "n", "", "human-readable name for this key (default: server-derived from key uid)")
	// G14 (F31): `ssh-key add` calls the same concept --title, and gh
	// uses --title for both key types. Accept --title here too so users
	// don't have to remember which spelling each subcommand wants. Hide
	// it from --help to keep one canonical form visible; both write to
	// the same opts.Name target so whichever the user passes wins.
	cmd.Flags().StringVar(&opts.Name, "title", "", "alias for --name (gh-compat)")
	_ = cmd.Flags().MarkHidden("title")
	return cmd
}

// Run executes the upload.
func Run(ctx context.Context, opts *options) error {
	blob, err := readKey(opts)
	if err != nil {
		return err
	}
	kind, _, err := key.DetectPublicKey(blob)
	if err != nil {
		return err
	}
	if kind != key.KindGPGArmor {
		return errors.New("gpg-key add: input is not an ASCII-armored GPG public key (expected -----BEGIN PGP PUBLIC KEY BLOCK-----)")
	}

	host := config.DefaultHost
	if opts.Hostname != "" {
		h, herr := config.ValidateHost(opts.Hostname)
		if herr != nil {
			return herr
		}
		host = h
	}
	client, err := opts.HTTPClient(host)
	if err != nil {
		return err
	}
	kc := keys.NewClient(client)

	out, err := kc.AddGPG(ctx, keys.GPGKeyInput{Name: opts.Name, ArmoredPublicKey: blob})
	if err != nil {
		return err
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Added GPG key (id=%d, key_id=%s)\n",
		opts.IO.SuccessIcon(), out.ID, out.KeyID)
	return nil
}

// readKey returns the armored block from the requested source. Unlike
// SSH there's no default-identity chain — GPG keyrings live in
// `gpg --homedir` land, not on the filesystem, so we don't try.
func readKey(opts *options) (string, error) {
	if opts.Path == "" || opts.Path == "-" {
		if opts.IO.In == nil {
			return "", errors.New("gpg-key add: stdin requested but unavailable")
		}
		b, err := io.ReadAll(opts.IO.In)
		if err != nil {
			return "", fmt.Errorf("gpg-key add: read stdin: %w", err)
		}
		return string(b), nil
	}
	b, err := os.ReadFile(opts.Path) //nolint:gosec // user-supplied path
	if err != nil {
		return "", fmt.Errorf("gpg-key add: read %q: %w", opts.Path, err)
	}
	return string(b), nil
}
