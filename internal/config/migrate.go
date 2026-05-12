// SPDX-License-Identifier: AGPL-3.0-or-later

package config

// Migrate is the schema-evolution hook for config.yml. It's intentionally
// a stub at schema version 1: there is no prior version to migrate from.
// When SchemaVersion bumps, add a case for each prior version that mutates
// the loaded Config in place toward the current shape. The stub exists so
// the migration path is wired the moment we ship a v2 schema; we never
// want a "first migration" sprint to also build the migration framework.
//
// Contract:
//   - Returns the resulting Config (never nil on success).
//   - Mutating in place is fine — c is the same pointer Load handed back.
//   - Errors should be returned without partial mutation; Load surfaces
//     them so users see what failed and can repair manually.
func Migrate(c *Config) (*Config, error) {
	if c == nil {
		return Default(), nil
	}
	// Future versions:
	//
	//   if c.Version < 2 {
	//       // ... in-place mutations toward v2 ...
	//       c.Version = 2
	//   }
	//   if c.Version < 3 {
	//       // ... etc ...
	//   }
	c.Version = SchemaVersion
	return c, nil
}
