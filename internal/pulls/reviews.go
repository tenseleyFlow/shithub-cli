// SPDX-License-Identifier: AGPL-3.0-or-later

package pulls

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
)

// ReviewEvent is the gh / GitHub-compatible review-submission enum.
// Empty (zero) value is invalid — callers must pick one before posting.
type ReviewEvent string

// Review events. PENDING is the wire value for "saved without submit"
// (no CLI command produces that today, but the server returns it on
// drafts and we round-trip cleanly).
const (
	ReviewApprove        ReviewEvent = "APPROVE"
	ReviewRequestChanges ReviewEvent = "REQUEST_CHANGES"
	ReviewComment        ReviewEvent = "COMMENT"
	ReviewPending        ReviewEvent = "PENDING"
)

// ReviewInput is the body for POST /pulls/{n}/reviews.
type ReviewInput struct {
	Event    ReviewEvent `json:"event"`
	Body     string      `json:"body,omitempty"`
	CommitID string      `json:"commit_id,omitempty"`
}

// SubmitReview posts a review event with optional body. Returns the
// server's echoed envelope (with ID, submitted_at filled in).
func (c *Client) SubmitReview(ctx context.Context, owner, repo string, number int, in ReviewInput) (*Review, error) {
	path := fmt.Sprintf("/repos/{owner}/{repo}/pulls/%d/reviews", number)
	var out Review
	if err := c.api.REST(ctx, http.MethodPost, path, in, &out,
		api.WithOwner(owner), api.WithRepo(repo)); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListReviews fetches every review submitted on a PR. Useful for
// `pr view --comments` (later) and for surfacing review history.
func (c *Client) ListReviews(ctx context.Context, owner, repo string, number int) ([]Review, error) {
	path := fmt.Sprintf("/repos/{owner}/{repo}/pulls/%d/reviews", number)
	var out []Review
	for raw, err := range c.api.DoPaginated(ctx, http.MethodGet, path,
		api.WithOwner(owner), api.WithRepo(repo)) {
		if err != nil {
			return nil, err
		}
		var page []Review
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, fmt.Errorf("pulls: decode reviews: %w", err)
		}
		out = append(out, page...)
	}
	return out, nil
}
