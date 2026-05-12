// SPDX-License-Identifier: AGPL-3.0-or-later

// Package switchcmd implements `shithub auth switch`. Today this is the
// cross-host default flipper; multi-account-per-host UX lands with
// C04a once the data model and token storage prove out.
package switchcmd

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/prompter"
)

// Options drives Run.
type Options struct {
	IO       *iostreams.IOStreams
	Prompter prompter.Prompter
	Hosts    func() (config.Hosts, error)

	Hostname string
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &Options{
		IO:       f.IOStreams,
		Prompter: f.Prompter,
		Hosts:    f.Hosts,
	}
	cmd := &cobra.Command{
		Use:   "switch",
		Short: "Switch the default shithub host",
		Long: `Change which configured host shithub-cli treats as the default when
--hostname is not passed. Lists every authenticated host and lets you
pick interactively, or pass --hostname to switch non-interactively.`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return Run(c.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "host to switch to (skips prompt)")
	return cmd
}

// Run picks the target host (flag or prompt) and persists the Default flip.
func Run(_ context.Context, opts *Options) error {
	hosts, err := opts.Hosts()
	if err != nil {
		return err
	}
	if len(hosts) == 0 {
		return errors.New("switch: no authenticated hosts — run `shithub auth login` first")
	}
	if len(hosts) == 1 {
		return errors.New("switch: only one host configured; nothing to switch to")
	}

	var target string
	if opts.Hostname != "" {
		h, err := config.ValidateHost(opts.Hostname)
		if err != nil {
			return fmt.Errorf("switch: %w", err)
		}
		target = h
	} else {
		picked, err := pickHostInteractive(opts, hosts)
		if err != nil {
			return err
		}
		target = picked
	}
	if _, ok := hosts[target]; !ok {
		return fmt.Errorf("switch: not authenticated to %s", target)
	}

	if err := hosts.SetDefault(target); err != nil {
		return err
	}
	if err := hosts.Save(); err != nil {
		return err
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Default host switched to %s\n", opts.IO.SuccessIcon(), target)
	return nil
}

// pickHostInteractive presents a Select prompt over the sorted host
// names. Non-interactive stdin yields a clear error suggesting --hostname.
func pickHostInteractive(opts *Options, hosts config.Hosts) (string, error) {
	if opts.IO.NeverPrompt() {
		return "", errors.New("switch: non-interactive context; pass --hostname")
	}
	var names []string
	for h := range hosts {
		names = append(names, h)
	}
	sort.Strings(names)

	defaultIdx := 0
	for i, h := range names {
		if hosts[h] != nil && hosts[h].Default {
			defaultIdx = i
		}
	}

	idx, err := opts.Prompter.Select("Switch to which host?", names[defaultIdx], names)
	if err != nil {
		return "", err
	}
	return names[idx], nil
}
