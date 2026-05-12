// SPDX-License-Identifier: AGPL-3.0-or-later

// Package run implements `shithub workflow run`. Triggers a
// workflow_dispatch event with optional `-f key=value` inputs or a
// `-F file.json` payload (mirrors shithub api's typed-field syntax).
package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/actions"
	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	repocmdshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

type options struct {
	IO          *iostreams.IOStreams
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string
	GitRunner   git.Runner

	Selector string
	Repo     string
	Hostname string
	Ref      string
	Fields   []string
	JSONFile string
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
	}
	cmd := &cobra.Command{
		Use:   "run <id-or-file>",
		Short: "Trigger a workflow_dispatch event",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts.Selector = args[0]
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
	cmd.Flags().StringVarP(&opts.Ref, "ref", "r", "", "branch or tag name (default: repo default branch)")
	cmd.Flags().StringArrayVarP(&opts.Fields, "field", "f", nil, "workflow_dispatch input as key=value (repeatable; typed: bool/number/null otherwise string)")
	cmd.Flags().StringVarP(&opts.JSONFile, "json-input", "F", "", "read workflow_dispatch inputs from a JSON file (use '-' for stdin)")
	return cmd
}

// Run dispatches.
func Run(ctx context.Context, opts *options) error {
	if len(opts.Fields) > 0 && opts.JSONFile != "" {
		return errors.New("workflow run: --field and --json-input are mutually exclusive")
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
	client, err := opts.HTTPClient(ref.Host)
	if err != nil {
		return err
	}
	ac := actions.NewClient(client)

	dispatchRef := opts.Ref
	if dispatchRef == "" {
		// Pull the repo's default branch via the workflow target. shithub
		// server also accepts an empty ref + uses the default — but
		// surfacing the resolved value in the success message is more
		// useful than a generic "(default branch)" string.
		wf, gerr := ac.GetWorkflow(ctx, ref.Owner, ref.Name, opts.Selector)
		if gerr == nil && wf != nil {
			dispatchRef = "" // server picks the default; we log "(default)"
		}
	}
	inputs, err := buildInputs(opts)
	if err != nil {
		return err
	}

	if err := ac.Dispatch(ctx, ref.Owner, ref.Name, opts.Selector, actions.DispatchInput{
		Ref: dispatchRef, Inputs: inputs,
	}); err != nil {
		return err
	}
	displayRef := dispatchRef
	if displayRef == "" {
		displayRef = "(default branch)"
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Dispatched workflow %s on %s\n",
		opts.IO.SuccessIcon(), opts.Selector, displayRef)
	return nil
}

// buildInputs parses the user's --field / --json-input flags into the
// inputs map. Field values are typed via parseFieldValue (bool/number/null
// recognition; everything else stays a string) to match `shithub api -F`'s
// behavior.
func buildInputs(opts *options) (map[string]any, error) {
	if opts.JSONFile != "" {
		blob, err := readJSON(opts.JSONFile, opts.IO.In)
		if err != nil {
			return nil, err
		}
		var out map[string]any
		if err := json.Unmarshal(blob, &out); err != nil {
			return nil, fmt.Errorf("workflow run: parse --json-input: %w", err)
		}
		return out, nil
	}
	if len(opts.Fields) == 0 {
		return nil, nil
	}
	out := make(map[string]any, len(opts.Fields))
	for _, f := range opts.Fields {
		k, v, ok := strings.Cut(f, "=")
		if !ok {
			return nil, fmt.Errorf("workflow run: --field %q is not key=value", f)
		}
		out[k] = parseFieldValue(v)
	}
	return out, nil
}

// parseFieldValue mirrors `shithub api -F` typing. We could share with
// pkg/cmd/api but the duplication is two cases and keeps the import
// graph clean.
func parseFieldValue(s string) any {
	switch s {
	case "true":
		return true
	case "false":
		return false
	case "null":
		return nil
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return s
}

func readJSON(path string, stdin io.Reader) ([]byte, error) {
	if path == "-" {
		if stdin == nil {
			return nil, errors.New("workflow run: --json-input=- but stdin is nil")
		}
		return io.ReadAll(stdin)
	}
	return os.ReadFile(path) //nolint:gosec // user-supplied path
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
