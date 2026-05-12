// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tenseleyFlow/shithub-cli/internal/config"
)

// validKeys names every config key the user can address from the
// command-line. Unknown keys are rejected on `set`; `get` returns empty
// with a stderr warning so users can introspect future-feature keys
// without errors.
//
// Aliases are managed via `shithub alias`, not `config set aliases.foo`,
// to keep the surface narrow. Per-host keys are surfaced via --host on
// `config get/set`.
var validKeys = []string{
	"git_protocol",
	"editor",
	"browser",
	"pager",
	"prompt",
	"http_unix_socket",
}

// hostScopedKeys is the subset of valid keys that participate in the
// --host overlay; setting them with --host writes the per-host entry in
// hosts.yml instead of config.yml.
var hostScopedKeys = map[string]struct{}{
	"git_protocol": {},
}

// listKeys returns the recognized global keys + the user's current alias
// keys (for `config list`). Sorted for stable output.
func listKeys(c *config.Config) []string {
	out := append([]string(nil), validKeys...)
	for name := range c.Aliases {
		out = append(out, "aliases."+name)
	}
	sort.Strings(out)
	return out
}

// readKey extracts the current value for a key. Returns ("", false)
// when the key is unset; ("<value>", true) otherwise. Errors only for
// keys outside the recognized schema AND not in the aliases.* namespace.
func readKey(c *config.Config, key string) (string, bool, error) {
	if strings.HasPrefix(key, "aliases.") {
		name := strings.TrimPrefix(key, "aliases.")
		if v, ok := c.Aliases[name]; ok {
			return v, true, nil
		}
		return "", false, nil
	}
	if !isValidKey(key) {
		return "", false, fmt.Errorf("config: unknown key %q (valid: %s)", key, strings.Join(validKeys, ", "))
	}
	switch key {
	case "git_protocol":
		return c.GitProtocol, c.GitProtocol != "", nil
	case "editor":
		return c.Editor, c.Editor != "", nil
	case "browser":
		return c.Browser, c.Browser != "", nil
	case "pager":
		return c.Pager, c.Pager != "", nil
	case "prompt":
		return c.Prompt, c.Prompt != "", nil
	case "http_unix_socket":
		return c.HTTPUnixSock, c.HTTPUnixSock != "", nil
	}
	return "", false, nil
}

// writeKey mutates c in place. Schema validation runs before persistence;
// returns an error for unknown keys or invalid values without touching
// the struct.
func writeKey(c *config.Config, key, value string) error {
	if strings.HasPrefix(key, "aliases.") {
		return fmt.Errorf("config: alias entries are managed via 'shithub alias set'")
	}
	if !isValidKey(key) {
		return fmt.Errorf("config: unknown key %q (valid: %s)", key, strings.Join(validKeys, ", "))
	}
	switch key {
	case "git_protocol":
		if value != "" && value != config.GitProtocolHTTPS && value != config.GitProtocolSSH {
			return fmt.Errorf("config: git_protocol must be %q or %q", config.GitProtocolHTTPS, config.GitProtocolSSH)
		}
		c.GitProtocol = value
	case "prompt":
		if value != "" && value != config.PromptEnabled && value != config.PromptDisabled {
			return fmt.Errorf("config: prompt must be %q or %q", config.PromptEnabled, config.PromptDisabled)
		}
		c.Prompt = value
	case "editor":
		c.Editor = value
	case "browser":
		c.Browser = value
	case "pager":
		c.Pager = value
	case "http_unix_socket":
		c.HTTPUnixSock = value
	}
	return nil
}

// writeHostKey mutates a hosts.yml entry. Only host-scoped keys land
// here; everything else gets a clear error.
func writeHostKey(entry *config.HostEntry, key, value string) error {
	if _, ok := hostScopedKeys[key]; !ok {
		return fmt.Errorf("config: --host only applies to: %s", strings.Join(hostScopedKeysList(), ", "))
	}
	switch key {
	case "git_protocol":
		if value != "" && value != config.GitProtocolHTTPS && value != config.GitProtocolSSH {
			return fmt.Errorf("config: git_protocol must be %q or %q", config.GitProtocolHTTPS, config.GitProtocolSSH)
		}
		entry.GitProtocol = value
	}
	return nil
}

// readHostKey returns the per-host value for a host-scoped key. The
// ok flag distinguishes "unset" from "absent".
func readHostKey(entry *config.HostEntry, key string) (string, bool, error) {
	if _, scoped := hostScopedKeys[key]; !scoped {
		return "", false, fmt.Errorf("config: --host only applies to: %s", strings.Join(hostScopedKeysList(), ", "))
	}
	switch key {
	case "git_protocol":
		return entry.GitProtocol, entry.GitProtocol != "", nil
	}
	return "", false, nil
}

// isValidKey reports whether key is in validKeys.
func isValidKey(key string) bool {
	for _, k := range validKeys {
		if k == key {
			return true
		}
	}
	return false
}

// hostScopedKeysList returns the host-scoped key names sorted so error
// messages are stable across calls.
func hostScopedKeysList() []string {
	out := make([]string, 0, len(hostScopedKeys))
	for k := range hostScopedKeys {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
