// SPDX-License-Identifier: AGPL-3.0-or-later

// Package shared houses the cross-subcommand helpers for `shithub
// search`: range parsing for numeric qualifiers, qualifier composition
// (so `--label bug --state open` lowers into `label:bug state:open`),
// and shared flag binding.
package shared

import (
	"fmt"
	"strconv"
	"strings"
)

// Range represents a gh-compatible numeric range used for qualifiers
// like `stars`, `forks`, `size`, `reactions`. The forms accepted:
//
//	>N    open upper bound, exclusive low      (Lower=N+1, Upper unset)
//	>=N   open upper bound, inclusive low      (Lower=N, Upper unset)
//	<N    open lower bound, exclusive high     (Upper=N-1)
//	<=N   open lower bound, inclusive high     (Upper=N)
//	N     exact match                          (Lower=N, Upper=N)
//	N..M  inclusive range                      (Lower=N, Upper=M)
//	*..N  unbounded low, inclusive high        (Upper=N)
//	N..*  inclusive low, unbounded high        (Lower=N)
//
// String renders the canonical form so callers can drop it into a
// qualifier without re-parsing.
type Range struct {
	HasLower bool
	HasUpper bool
	Lower    int
	Upper    int
}

// ParseRange converts a user-supplied range string into Range. Returns
// an error when the form is unrecognized — the CLI surfaces these
// verbatim so the user can fix their flag.
func ParseRange(s string) (Range, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Range{}, fmt.Errorf("range: empty")
	}

	switch {
	case strings.HasPrefix(s, ">="):
		n, err := atoi(s[2:])
		if err != nil {
			return Range{}, err
		}
		return Range{HasLower: true, Lower: n}, nil
	case strings.HasPrefix(s, "<="):
		n, err := atoi(s[2:])
		if err != nil {
			return Range{}, err
		}
		return Range{HasUpper: true, Upper: n}, nil
	case strings.HasPrefix(s, ">"):
		n, err := atoi(s[1:])
		if err != nil {
			return Range{}, err
		}
		return Range{HasLower: true, Lower: n + 1}, nil
	case strings.HasPrefix(s, "<"):
		n, err := atoi(s[1:])
		if err != nil {
			return Range{}, err
		}
		return Range{HasUpper: true, Upper: n - 1}, nil
	}

	if lo, hi, ok := strings.Cut(s, ".."); ok {
		r := Range{}
		if lo != "*" {
			n, err := atoi(lo)
			if err != nil {
				return Range{}, err
			}
			r.HasLower = true
			r.Lower = n
		}
		if hi != "*" {
			n, err := atoi(hi)
			if err != nil {
				return Range{}, err
			}
			r.HasUpper = true
			r.Upper = n
		}
		if !r.HasLower && !r.HasUpper {
			return Range{}, fmt.Errorf("range: %q has no bounds", s)
		}
		return r, nil
	}

	n, err := atoi(s)
	if err != nil {
		return Range{}, err
	}
	return Range{HasLower: true, HasUpper: true, Lower: n, Upper: n}, nil
}

// String renders Range as the canonical qualifier value. Empty Range
// yields the empty string so callers can skip it without a branch.
func (r Range) String() string {
	switch {
	case !r.HasLower && !r.HasUpper:
		return ""
	case r.HasLower && r.HasUpper && r.Lower == r.Upper:
		return strconv.Itoa(r.Lower)
	case r.HasLower && r.HasUpper:
		return fmt.Sprintf("%d..%d", r.Lower, r.Upper)
	case r.HasLower:
		return fmt.Sprintf(">=%d", r.Lower)
	default:
		return fmt.Sprintf("<=%d", r.Upper)
	}
}

func atoi(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("range: %q is not an integer", s)
	}
	return n, nil
}
