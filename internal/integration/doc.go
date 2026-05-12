// SPDX-License-Identifier: AGPL-3.0-or-later

// Package integration holds end-to-end tests that talk to a real
// shithub server. Tests are guarded by the `integration` build tag so
// the default `go test ./...` and `make ci` flows stay hermetic. To
// run them:
//
//	make integration SHITHUB_INTEGRATION_HOST=localhost:8080 \
//	  SHITHUB_INTEGRATION_TOKEN=shithub_pat_<32hex>
//
// The fixture server is expected to be reachable, the token must have
// at least repo:read + user:read scope, and the host must accept HTTP
// (set SHITHUB_DEV_INSECURE_HTTP=1) or HTTPS depending on the deployment.
//
// Each *_test.go in this package calls `requireIntegration(t)` first,
// which skips when the env vars aren't set. That keeps the substrate
// in-tree without making local laptops or CI containers spin up a full
// shithub stack on every push. Audit #153 (2026-05-12) ratified this
// opt-in design.
package integration
