# Contributing to shithub-cli

shithub-cli is open to contributions from anyone. The project is
pre-1.0; until v1.0.0 ships, minor versions may break flags or
output shapes.

By contributing, you certify that the work is yours to give and
that you're licensing it to the project under the same terms as
the rest of the codebase (AGPLv3). The mechanism is the
[Developer Certificate of Origin (DCO) v1.1](https://developercertificate.org/) —
sign off your commits with `-s`:

```sh
git commit -s -m "your message"
```

A CI check rejects PRs whose commits aren't signed off.

## Code of conduct

Be civil. Harassment isn't tolerated.

## Reporting bugs

For non-security bugs: open an issue on this repo.

For security issues: see `SECURITY.md` once it lands. Until then,
email mfwolffe@outlook.com. Do **not** file public issues for
security findings.

## Development setup

```sh
git clone https://github.com/tenseleyFlow/shithub-cli.git
cd shithub-cli
make install-tools   # gofumpt, goimports, golangci-lint, goreleaser
make build            # produces bin/shithub
make test
```

Requires Go 1.26+.

## Submitting a change

1. **Branch.** From `trunk`, create a feature branch.
2. **Commit.** Terse, imperative, single-line messages unless the
   change needs elaboration. One concern per commit. Avoid
   `git add -A`; stage by file.
3. **Test.** Add tests proportional to the scope. Unit tests use
   `internal/api/fakeapi` and `internal/testing/fakeconfig`.
   Integration tests run against a real shithub instance via
   docker-compose.
4. **Lint.** `make ci` runs everything CI runs. PRs must be green.
5. **Push + PR.** Open against `trunk`. The PR template prompts
   for a summary, test plan, and reviewer notes.

## Code style

- `gofumpt` + `goimports` (run via `make fmt`).
- `golangci-lint` per the in-repo config.
- SPDX header (`// SPDX-License-Identifier: AGPL-3.0-or-later`) on
  every `.go` file. Enforced by `scripts/check-spdx.sh` in CI.
- Comment style: lead with the why. Don't restate the code.
- No `fmt.Println` in command code; use `iostreams.IOStreams`.
- No direct `net/http` in command code; use `internal/api.Client`.

## Sprint planning

Implementation work follows the `.docs/sprints/CXX-*.md` files.
`.docs/` is gitignored (private planning material). If you want
to propose a new feature, open an issue first describing the
scope so we can fit it into the sprint roadmap.
