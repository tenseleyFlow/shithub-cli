// SPDX-License-Identifier: AGPL-3.0-or-later

// Package cmdutil owns the dependency-injection seam every shithub-cli
// command builder pulls from. A *Factory bundles the runtime collaborators
// (IO, config, keyring, prompter, HTTP) so a command file declares its
// inputs as one struct rather than reaching into globals. Production
// builds wire `New()` into the cobra root; tests instantiate a
// *Factory with fakes via cmdutil/cmdutiltest.
package cmdutil

import (
	"context"
	"fmt"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/prompter"
)

// Factory is the single dependency container passed to every command
// builder (NewCmdLogin, NewCmdView, ...). Members are lazy (functions)
// where construction has cost or can fail, eager (values) where it doesn't.
type Factory struct {
	// IOStreams handles stdout/stderr/stdin, color, pager.
	IOStreams *iostreams.IOStreams

	// Prompter handles interactive questions. Test factories inject a
	// scripted prompter; production uses survey/v2.
	Prompter prompter.Prompter

	// Config returns a freshly-loaded user config. Reloaded each call so
	// long-running flows pick up changes written by sibling commands.
	Config func() (*config.Config, error)

	// Hosts returns a freshly-loaded hosts map.
	Hosts func() (config.Hosts, error)

	// Keyring returns the active keyring backend.
	Keyring func() config.KeyringStore

	// HTTPClient returns an authenticated api.Client for the given host.
	// Token + base URL are resolved per-call so changes to hosts.yml are
	// picked up between commands within the same invocation. An error from
	// HTTPClient typically means "no token configured" — commands should
	// translate to "run shithub auth login".
	HTTPClient func(host string) (*api.Client, error)

	// DefaultHost resolves the active host for commands that take an
	// optional --hostname. Looks at --hostname > SHITHUB_HOST > hosts.yml
	// > package default.
	DefaultHost func() string

	// GitProtocol returns the user's preferred git protocol ("https" or
	// "ssh") for commands that emit clone URLs (repo clone, fork).
	GitProtocol func() string
}

// New returns a production Factory wired to System IOStreams, the real
// keyring, the survey prompter, and config+hosts loaded from disk. The
// returned Factory closes over a single keyring instance so probe checks
// are consistent for the lifetime of the command invocation.
func New(p prompter.Prompter) (*Factory, error) {
	if p == nil {
		return nil, fmt.Errorf("cmdutil: prompter is required")
	}
	ios := iostreams.System()
	ks := config.NewSystemKeyring()

	cfgFn := func() (*config.Config, error) { return config.Load() }
	hostsFn := func() (config.Hosts, error) { return config.LoadHosts() }
	keyringFn := func() config.KeyringStore { return ks }
	gitProtoFn := func() string {
		c, err := cfgFn()
		if err != nil || c.GitProtocol == "" {
			return config.DefaultGitProtocol
		}
		return c.GitProtocol
	}
	defaultHostFn := func() string {
		h, _ := hostsFn()
		return config.ResolveHost("", h)
	}
	clientFn := func(host string) (*api.Client, error) {
		host = config.NormalizeHost(host)
		hosts, err := hostsFn()
		if err != nil {
			return nil, err
		}
		defaultHost := hosts.DefaultHostName()
		tokenFn := func(_ context.Context, h string) (string, string, error) {
			token, src, err := config.ResolveToken(ks, hosts, defaultHost, h)
			if err != nil {
				return "", "", err
			}
			return token, src.String(), nil
		}
		return api.NewClient(api.ClientOptions{
			Host:      host,
			TokenFunc: tokenFn,
			Logger:    nil, // SHITHUB_DEBUG-aware logger lands in a later sprint
		})
	}

	return &Factory{
		IOStreams:   ios,
		Prompter:    p,
		Config:      cfgFn,
		Hosts:       hostsFn,
		Keyring:     keyringFn,
		HTTPClient:  clientFn,
		DefaultHost: defaultHostFn,
		GitProtocol: gitProtoFn,
	}, nil
}
