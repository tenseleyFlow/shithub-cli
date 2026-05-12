// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build dev

// Package dev registers hidden test-only commands compiled in via the
// `dev` build tag. Production builds (`go build`, goreleaser) do NOT
// include this file, so the surface is invisible to real users.
//
// Build locally with:
//
//	go build -tags dev -o bin/shithub ./cmd/shithub
//
// then exercise commands like `shithub _dev hello --json greeting`. The
// dev tag is also wired into integration tests that need a real binary
// to drive end-to-end.
package dev

import (
	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/output"
)

// NewCmd returns the parent `_dev` command. Hidden so it doesn't pollute
// help output even when compiled in.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "_dev",
		Short:  "Hidden developer-only test commands (dev build tag).",
		Hidden: true,
	}
	cmd.AddCommand(newHelloCmd())
	return cmd
}

type helloExporter struct{}

func (helloExporter) Fields() []string { return []string{"greeting", "subject"} }

func (helloExporter) Filter(v any) (any, error) {
	// `v` arrives as a map already shaped to the export contract; pass through.
	return v, nil
}

func newHelloCmd() *cobra.Command {
	var (
		subject string
		opts    output.Options
	)
	cmd := &cobra.Command{
		Use:    "hello",
		Short:  "Emit a greeting; used by iostreams + output integration tests.",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			data := map[string]any{
				"greeting": "hello",
				"subject":  subject,
			}
			if opts.Active() {
				return output.Export(c.OutOrStdout(), opts, helloExporter{}, data, true)
			}
			_, err := c.OutOrStdout().Write([]byte("hello, " + subject + "\n"))
			return err
		},
	}
	cmd.Flags().StringVar(&subject, "subject", "world", "subject of the greeting")
	output.AddFlags(cmd, &opts)
	return cmd
}
