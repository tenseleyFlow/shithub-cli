// SPDX-License-Identifier: AGPL-3.0-or-later

package orgs

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/api/fakeapi"
)

func TestListAuthenticatedDecodes(t *testing.T) {
	srv := fakeapi.New(t)
	srv.Handle(http.MethodGet, "/api/v1/user/orgs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]Org{
			{Login: "tenseleyFlow", Role: "admin"},
			{Login: "other", Role: "member"},
		})
	})

	c := NewClient(srv.NewClient())
	out, err := c.ListAuthenticated(context.Background(), ListOptions{})
	if err != nil {
		t.Fatalf("ListAuthenticated: %v", err)
	}
	if len(out) != 2 || out[0].Login != "tenseleyFlow" || out[1].Role != "member" {
		t.Errorf("got %+v", out)
	}
}

func TestGetDecodesProfile(t *testing.T) {
	srv := fakeapi.New(t)
	srv.Handle(http.MethodGet, "/api/v1/orgs/tenseleyFlow", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Org{
			Login:        "tenseleyFlow",
			Name:         "Tenseley Flow",
			Description:  "shithub org",
			Location:     "the cloud",
			PublicRepos:  4,
			MembersCount: 7,
		})
	})

	c := NewClient(srv.NewClient())
	o, err := c.Get(context.Background(), "tenseleyFlow")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if o.Description != "shithub org" || o.PublicRepos != 4 || o.MembersCount != 7 {
		t.Errorf("got %+v", o)
	}
}

func TestListUserUsesPublicEndpoint(t *testing.T) {
	srv := fakeapi.New(t)
	srv.Handle(http.MethodGet, "/api/v1/users/octocat/orgs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]Org{{Login: "public-org"}})
	})

	c := NewClient(srv.NewClient())
	out, err := c.ListUser(context.Background(), "octocat", ListOptions{})
	if err != nil {
		t.Fatalf("ListUser: %v", err)
	}
	if len(out) != 1 || out[0].Login != "public-org" {
		t.Errorf("got %+v", out)
	}
}
