// SPDX-License-Identifier: AGPL-3.0-or-later

package setupgit

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
)

// recordingGit captures every invocation so tests can assert the config
// lines we wrote.
type recordingGit struct {
	mu   sync.Mutex
	args [][]string
}

func (r *recordingGit) run(args ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.args = append(r.args, append([]string(nil), args...))
	return nil
}

func TestSetupGitWritesHelper(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Config.Hosts.Get("shithub.sh").User = "mf"
	_ = tf.Config.Hosts.Save()

	rg := &recordingGit{}
	opts := &Options{
		IO:        tf.IOStreams,
		Hosts:     tf.Factory.Hosts,
		GitRunner: rg.run,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}

	cfgKey := "credential.https://shithub.sh.helper"
	found := false
	for _, a := range rg.args {
		if len(a) >= 4 && a[3] == cfgKey && strings.Contains(strings.Join(a, " "), "shithub auth git-credential") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected git config writing %q with shithub helper; got args=%v", cfgKey, rg.args)
	}
}

func TestSetupGitFilterByHostname(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Config.Hosts.Get("a.example").User = "u"
	tf.Config.Hosts.Get("b.example").User = "u"
	_ = tf.Config.Hosts.Save()

	rg := &recordingGit{}
	opts := &Options{
		IO:        tf.IOStreams,
		Hosts:     tf.Factory.Hosts,
		Hostname:  "a.example",
		GitRunner: rg.run,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, a := range rg.args {
		joined := strings.Join(a, " ")
		if strings.Contains(joined, "b.example") {
			t.Errorf("unexpected b.example invocation: %v", a)
		}
	}
}

func TestSetupGitErrorsWithoutAnyHost(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &Options{
		IO:        tf.IOStreams,
		Hosts:     tf.Factory.Hosts,
		GitRunner: func(_ ...string) error { return nil },
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error when no hosts configured")
	}
}
