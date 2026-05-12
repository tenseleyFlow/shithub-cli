// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import "os"

// osLookupEnv isolates the single os import the rest of the package needs.
// Centralizing the dependency makes it easy to enforce "no direct os in
// client.go" at lint time (revive's importas rule once we set it up).
func osLookupEnv(key string) string { return os.Getenv(key) }
