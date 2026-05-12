// SPDX-License-Identifier: AGPL-3.0-or-later

// Package add implements `shithub ssh-key add`. Reads a public-key file
// (positional, `-` for stdin, or the default-identity fallback chain)
// and uploads it via POST /user/keys.
package add

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/key"
	"github.com/tenseleyFlow/shithub-cli/internal/keys"
)

// defaultIdentityFiles is the search order for the implicit public-key
// file when neither a positional arg nor `-` is supplied. Mirrors
// OpenSSH's preference for ed25519 over RSA over ECDSA.
var defaultIdentityFiles = []string{
	"~/.ssh/id_ed25519.pub",
	"~/.ssh/id_rsa.pub",
	"~/.ssh/id_ecdsa.pub",
}

type options struct {
	IO          *iostreams.IOStreams
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string

	Path     string // positional; "" = default chain; "-" = stdin
	Title    string
	Type     string
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
		Short: "Add an SSH public key to your shithub account",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Path = args[0]
			}
			return Run(c.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "the shithub host (default: configured host)")
	cmd.Flags().StringVarP(&opts.Title, "title", "t", "", "title for the new key (default: derived from the key's comment)")
	cmd.Flags().StringVar(&opts.Type, "type", keys.SSHKindAuthentication, "key type: authentication or signing")
	return cmd
}

// Run executes the upload.
func Run(ctx context.Context, opts *options) error {
	switch opts.Type {
	case "", keys.SSHKindAuthentication, keys.SSHKindSigning:
	default:
		return fmt.Errorf("ssh-key add: --type must be 'authentication' or 'signing', got %q", opts.Type)
	}
	if opts.Type == "" {
		opts.Type = keys.SSHKindAuthentication
	}

	blob, source, err := readKey(opts)
	if err != nil {
		return err
	}
	kind, comment, err := key.DetectPublicKey(blob)
	if err != nil {
		return err
	}
	if kind != key.KindSSH {
		return errors.New("ssh-key add: input is not an SSH public key")
	}

	title := opts.Title
	if title == "" {
		title = comment
	}
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(source), ".pub")
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

	out, err := kc.AddSSH(ctx, keys.SSHKeyInput{
		Title: title,
		Key:   strings.TrimSpace(blob),
		Kind:  opts.Type,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Added SSH key %q (id=%d, fingerprint=%s)\n",
		opts.IO.SuccessIcon(), out.Title, out.ID, out.Fingerprint)
	return nil
}

// readKey returns the public-key bytes plus a "source" label suitable
// for default-title derivation. Honors the positional arg, `-` (stdin),
// and the default-identity fallback chain.
func readKey(opts *options) (blob, source string, err error) {
	if opts.Path == "-" {
		if opts.IO.In == nil {
			return "", "", errors.New("ssh-key add: stdin requested but unavailable")
		}
		b, rerr := io.ReadAll(opts.IO.In)
		if rerr != nil {
			return "", "", fmt.Errorf("ssh-key add: read stdin: %w", rerr)
		}
		return string(b), "stdin", nil
	}
	if opts.Path != "" {
		b, rerr := os.ReadFile(opts.Path) //nolint:gosec // user-supplied path
		if rerr != nil {
			return "", "", fmt.Errorf("ssh-key add: read %q: %w", opts.Path, rerr)
		}
		return string(b), opts.Path, nil
	}
	// Default chain.
	home, herr := os.UserHomeDir()
	if herr != nil {
		return "", "", fmt.Errorf("ssh-key add: locate home dir: %w", herr)
	}
	tried := make([]string, 0, len(defaultIdentityFiles))
	for _, candidate := range defaultIdentityFiles {
		path := strings.Replace(candidate, "~", home, 1)
		tried = append(tried, path)
		b, rerr := os.ReadFile(path) //nolint:gosec // candidate under user's $HOME
		if rerr == nil {
			return string(b), path, nil
		}
	}
	return "", "", fmt.Errorf("ssh-key add: no public-key file found in %v (pass a path or `-` for stdin)", tried)
}
