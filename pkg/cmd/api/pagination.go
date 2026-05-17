// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
)

// runPaginated iterates over all pages returned by the Link header
// chain. The composition rules:
//
//   - Each page is decoded once; an array body's elements get appended
//     to the concat slice unless --slurp is set, in which case the page
//     bodies themselves go into the outer array.
//   - Object bodies (no top-level array) are emitted unchanged; --slurp
//     wraps them in an array. We do not look for a conventional "items"
//     field — gh doesn't either, and shithub's S50 contract is array-
//     bodied for list endpoints.
//   - --max-pages: 0 (default) → api.DefaultMaxPages; negative → no cap.
//
// We do not honor --include in this mode (multiple sets of headers don't
// compose into a single output stream); --jq/--template still apply to
// the final assembled payload.
func runPaginated(ctx context.Context, opts *Options, client *api.Client, method, endpoint string, headers http.Header) error {
	if !strings.EqualFold(method, http.MethodGet) {
		return errors.New("api: --paginate currently only supports GET")
	}

	reqOpts := []api.RequestOption{}
	for k, vs := range headers {
		for _, v := range vs {
			reqOpts = append(reqOpts, api.WithHeader(k, v))
		}
	}
	if opts.MaxPages != 0 {
		reqOpts = append(reqOpts, api.WithMaxPages(opts.MaxPages))
	}

	concat := []json.RawMessage{}
	pages := []json.RawMessage{}

	firstPage := true
	for raw, err := range client.DoPaginated(ctx, method, endpoint, reqOpts...) {
		if err != nil {
			return err
		}
		if opts.Slurp {
			pages = append(pages, raw)
			firstPage = false
			continue
		}
		// C11: reject --paginate against a non-array endpoint. The
		// pre-D3b behavior wrapped a single object in [obj] silently,
		// which fooled scripts into believing they were paginating
		// over a list (e.g., `api repos/o/r --paginate | jq '.[]'`
		// emitted exactly one element no matter what). Surface the
		// shape mismatch as soon as we see the first page; subsequent
		// pages are an array by construction.
		var arr []json.RawMessage
		if err := json.Unmarshal(raw, &arr); err != nil {
			if firstPage {
				return fmt.Errorf("api: --paginate requires a JSON array response; %s did not return one", endpoint)
			}
			// Defensive: a mid-stream non-array would be a server bug.
			// Keep the legacy behavior (pass through as a single item)
			// so we don't drop data; trust the first-page check above.
			concat = append(concat, raw)
			firstPage = false
			continue
		}
		concat = append(concat, arr...)
		firstPage = false
	}

	var assembled []byte
	if opts.Slurp {
		out, err := json.Marshal(pages)
		if err != nil {
			return fmt.Errorf("api: marshal --slurp pages: %w", err)
		}
		assembled = out
	} else {
		out, err := json.Marshal(concat)
		if err != nil {
			return fmt.Errorf("api: marshal pages: %w", err)
		}
		assembled = out
	}

	return emit(opts, http.StatusOK, http.Header{}, assembled)
}
