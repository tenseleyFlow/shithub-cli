// SPDX-License-Identifier: AGPL-3.0-or-later

// Package cmdutiltest builds test-isolated *cmdutil.Factory instances so
// command tests don't touch the real config dir, keyring, or network. The
// factory wires a buffer-backed IOStreams, a tmpdir-scoped config, an
// in-memory keyring, a scripted prompter, and a fakeapi-backed HTTPClient.
package cmdutiltest

import (
	"bytes"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/api/fakeapi"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/testing/fakeconfig"
)

// Factory bundles a test-isolated *cmdutil.Factory together with handles
// to the underlying fakes so assertions can poke at the keyring/hosts/
// stdout buffers directly.
type Factory struct {
	*cmdutil.Factory

	In     *bytes.Buffer
	Out    *bytes.Buffer
	ErrOut *bytes.Buffer
	Config *fakeconfig.TestConfig
	Server *fakeapi.Server
	Prompt *RecordingPrompter
}

// New returns a populated test Factory. The fake HTTP server is reachable
// via the returned `Server` handle; register routes with Server.RegisterJSON.
// HTTPClient(host) always returns a client pointed at the fake server,
// regardless of `host` — tests assert on the server-side calls.
func New(t *testing.T) *Factory {
	t.Helper()

	ios, in, out, errOut := iostreams.Test()
	tc := fakeconfig.New(t)
	srv := fakeapi.New(t)
	rp := NewRecordingPrompter()

	hostsFn := func() (config.Hosts, error) { return tc.Hosts, nil }
	cfgFn := func() (*config.Config, error) { return tc.Config, nil }
	keyringFn := func() config.KeyringStore { return tc.Keyring }
	gitProtoFn := func() string {
		if tc.Config.GitProtocol != "" {
			return tc.Config.GitProtocol
		}
		return config.DefaultGitProtocol
	}
	defaultHostFn := func() string {
		return config.ResolveHost("", tc.Hosts)
	}
	clientFn := func(_ string) (*api.Client, error) {
		return srv.NewClientWithToken("shithub_pat_testtoken"), nil
	}

	f := &cmdutil.Factory{
		IOStreams:   ios,
		Prompter:    rp,
		Config:      cfgFn,
		Hosts:       hostsFn,
		Keyring:     keyringFn,
		HTTPClient:  clientFn,
		DefaultHost: defaultHostFn,
		GitProtocol: gitProtoFn,
	}

	return &Factory{
		Factory: f,
		In:      in,
		Out:     out,
		ErrOut:  errOut,
		Config:  tc,
		Server:  srv,
		Prompt:  rp,
	}
}
