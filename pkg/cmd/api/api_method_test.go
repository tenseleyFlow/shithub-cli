// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
)

// TestRunRejectsEmptyMethodFlag pins I36: pre-fix `api -X ""` silently
// defaulted to GET, hiding the user's typo. With MethodSet populated
// from cobra's Changed("method"), an explicit-empty -X now errors.
func TestRunRejectsEmptyMethodFlag(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &Options{
		IO:         tf.IOStreams,
		HTTPClient: tf.Factory.HTTPClient,
		Endpoint:   "/user",
		Method:     "",
		MethodSet:  true, // simulates `-X ""` from the cobra parse
	}
	err := Run(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error from -X '', got nil")
	}
	if !strings.Contains(err.Error(), "-X requires a value") {
		t.Errorf("error should call out -X: %v", err)
	}
}

// TestRunAcceptsUnsetMethod confirms the default-GET path stays alive
// when -X isn't passed at all. The MethodSet=false branch must not
// trigger the I36 check.
func TestRunAcceptsUnsetMethod(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON("GET", "/api/v1/user", 200, map[string]any{"id": 1})
	opts := &Options{
		IO:         tf.IOStreams,
		HTTPClient: tf.Factory.HTTPClient,
		Endpoint:   "/user",
		Method:     "",
		MethodSet:  false, // default — user didn't pass -X
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("default-GET path errored: %v", err)
	}
}
