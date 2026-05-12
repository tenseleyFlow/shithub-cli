// SPDX-License-Identifier: AGPL-3.0-or-later

package commits

import (
	"fmt"

	"github.com/tenseleyFlow/shithub-cli/internal/search"
)

type exporter struct{}

func (exporter) Fields() []string {
	return []string{"author", "committer", "commit", "repository", "sha", "url"}
}

func (exporter) Filter(v any) (any, error) {
	xs, ok := v.([]search.CommitItem)
	if !ok {
		return nil, fmt.Errorf("search commits exporter: want []search.CommitItem, got %T", v)
	}
	out := make([]map[string]any, 0, len(xs))
	for _, it := range xs {
		var repo any
		if it.Repository != nil {
			repo = map[string]any{"fullName": it.Repository.FullName}
		}
		var author any
		if it.Author != nil {
			author = map[string]any{"login": it.Author.Login, "id": it.Author.ID}
		}
		var committer any
		if it.Committer != nil {
			committer = map[string]any{"login": it.Committer.Login, "id": it.Committer.ID}
		}
		out = append(out, map[string]any{
			"author":     author,
			"committer":  committer,
			"commit":     it.Commit,
			"repository": repo,
			"sha":        it.SHA,
			"url":        it.HTMLURL,
		})
	}
	return out, nil
}
