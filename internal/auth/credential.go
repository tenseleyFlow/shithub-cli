// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// CredentialRequest is the parsed input git sends a credential helper.
// Protocol: a sequence of `key=value\n` lines terminated by a blank line.
// The fields we care about are `protocol`, `host`, optionally `path`;
// the `username` field is unset on the way in (we choose it).
type CredentialRequest struct {
	Protocol string
	Host     string
	Path     string
	// Raw captures any additional keys git sends so we don't accidentally
	// drop forward-compat metadata.
	Raw map[string]string
}

// ReadCredentialRequest reads a git credential-protocol request from r.
// Returns an error only on I/O failure; an empty request (no headers,
// just an immediate blank line) is legal and yields a zero-value struct.
func ReadCredentialRequest(r io.Reader) (CredentialRequest, error) {
	req := CredentialRequest{Raw: map[string]string{}}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			break
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := line[:eq]
		val := line[eq+1:]
		req.Raw[key] = val
		switch key {
		case "protocol":
			req.Protocol = val
		case "host":
			req.Host = val
		case "path":
			req.Path = val
		}
	}
	if err := scanner.Err(); err != nil {
		return req, err
	}
	return req, nil
}

// WriteCredentialResponse emits the helper response: username + password
// lines, terminating blank line. git accepts this as "credentials found".
//
// An empty username or token causes nothing to be written — git falls
// through to its other credential helpers, matching gh's behavior.
func WriteCredentialResponse(w io.Writer, username, token string) error {
	if username == "" || token == "" {
		return nil
	}
	if _, err := fmt.Fprintf(w, "protocol=https\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "username=%s\n", username); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "password=%s\n", token); err != nil {
		return err
	}
	// Trailing blank line per the credential-helper protocol.
	_, err := fmt.Fprintln(w)
	return err
}
