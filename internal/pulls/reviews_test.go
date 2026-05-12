// SPDX-License-Identifier: AGPL-3.0-or-later

package pulls

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/api/fakeapi"
)

func TestSubmitReviewApprove(t *testing.T) {
	srv := fakeapi.New(t)
	var body json.RawMessage
	srv.Handle(http.MethodPost, "/api/v1/repos/o/r/pulls/1/reviews", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(Review{ID: 42, State: "APPROVED", User: &api.User{Login: "rev"}})
	})

	c := NewClient(srv.NewClient())
	out, err := c.SubmitReview(context.Background(), "o", "r", 1, ReviewInput{
		Event: ReviewApprove, Body: "lgtm",
	})
	if err != nil {
		t.Fatalf("SubmitReview: %v", err)
	}
	if out.ID != 42 {
		t.Errorf("got %+v", out)
	}
	if !strings.Contains(string(body), `"event":"APPROVE"`) || !strings.Contains(string(body), `"body":"lgtm"`) {
		t.Errorf("body: %s", body)
	}
}

func TestListReviewsPaginates(t *testing.T) {
	srv := fakeapi.New(t)
	srv.Handle(http.MethodGet, "/api/v1/repos/o/r/pulls/1/reviews", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]Review{
			{ID: 1, State: "APPROVED", User: &api.User{Login: "a"}},
			{ID: 2, State: "COMMENTED", User: &api.User{Login: "b"}},
		})
	})

	c := NewClient(srv.NewClient())
	out, err := c.ListReviews(context.Background(), "o", "r", 1)
	if err != nil {
		t.Fatalf("ListReviews: %v", err)
	}
	if len(out) != 2 || out[0].ID != 1 || out[1].ID != 2 {
		t.Errorf("reviews: %v", out)
	}
}
