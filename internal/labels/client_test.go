// SPDX-License-Identifier: AGPL-3.0-or-later

package labels

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tenseleyFlow/shithub-cli/internal/api/fakeapi"
)

func TestListSortsClientSide(t *testing.T) {
	srv := fakeapi.New(t)
	srv.Handle(http.MethodGet, "/api/v1/repos/o/r/labels", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]Label{
			{Name: "zeta", Color: "ff0000", CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
			{Name: "alpha", Color: "00ff00", CreatedAt: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)},
			{Name: "mid", Color: "0000ff", CreatedAt: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)},
		})
	})

	c := NewClient(srv.NewClient())
	out, err := c.List(context.Background(), "o", "r", ListOptions{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if out[0].Name != "alpha" || out[2].Name != "zeta" {
		t.Errorf("name sort: %v", out)
	}

	out, _ = c.List(context.Background(), "o", "r", ListOptions{Sort: "created", Direction: "asc"})
	if out[0].Name != "zeta" {
		t.Errorf("created/asc: %v", out)
	}
}

func TestCreate(t *testing.T) {
	srv := fakeapi.New(t)
	var body json.RawMessage
	srv.Handle(http.MethodPost, "/api/v1/repos/o/r/labels", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(Label{Name: "bug", Color: "ff0000"})
	})

	c := NewClient(srv.NewClient())
	_, err := c.Create(context.Background(), "o", "r", CreateInput{Name: "bug", Color: "ff0000", Description: "x"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !strings.Contains(string(body), `"name":"bug"`) || !strings.Contains(string(body), `"color":"ff0000"`) {
		t.Errorf("body: %s", body)
	}
}

func TestEditRename(t *testing.T) {
	srv := fakeapi.New(t)
	var body json.RawMessage
	srv.Handle(http.MethodPatch, "/api/v1/repos/o/r/labels/old", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Label{Name: "new"})
	})

	c := NewClient(srv.NewClient())
	newName := "new"
	if _, err := c.Edit(context.Background(), "o", "r", "old", EditInput{NewName: &newName}); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	// E6: server expects `name`, not `new_name`. Pre-fix the CLI sent
	// the wrong field and the server silently no-op'd.
	if !strings.Contains(string(body), `"name":"new"`) {
		t.Errorf("rename: %s", body)
	}
}

func TestDelete(t *testing.T) {
	srv := fakeapi.New(t)
	srv.Handle(http.MethodDelete, "/api/v1/repos/o/r/labels/bug", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	c := NewClient(srv.NewClient())
	if err := c.Delete(context.Background(), "o", "r", "bug"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	srv.AssertCalled(http.MethodDelete, "/api/v1/repos/o/r/labels/bug")
}
