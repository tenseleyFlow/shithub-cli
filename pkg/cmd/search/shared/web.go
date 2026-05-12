// SPDX-License-Identifier: AGPL-3.0-or-later

package shared

import (
	"fmt"
	"net/url"

	"github.com/tenseleyFlow/shithub-cli/internal/config"
)

// WebSearchURL composes shithub's web search URL. shithub mirrors
// GitHub's UI route: `/search?q=...&type=...` where type is one of
// `repositories|issues|pullrequests|code|commits`. host falls back to
// the configured default when empty.
func WebSearchURL(host, kind, query string) string {
	if host == "" {
		host = config.DefaultHost
	}
	v := url.Values{}
	v.Set("q", query)
	v.Set("type", kind)
	return fmt.Sprintf("https://%s/search?%s", host, v.Encode())
}
