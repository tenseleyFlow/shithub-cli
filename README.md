# shithub-cli

`shithub` — the command-line client for [shithub.sh](https://shithub.sh).

A 1:1 reverse engineering of GitHub's [`gh` CLI](https://cli.github.com/),
targeting [shithub](https://github.com/tenseleyFlow/shithub) as the
backend. AGPLv3, no Copilot, no AI features.

> **Status**: pre-1.0. Sprint roadmap drives feature delivery; see
> commits tagged `C00`..`C28`. Until v1.0.0, flags and output shapes may
> change between minor releases — wire the response shapes you depend
> on under explicit `--json <fields>` rather than relying on the human
> formatter.

## Install

### v0.x.0 (current pre-release): from source

```sh
git clone https://github.com/tenseleyFlow/shithub-cli.git
cd shithub-cli
make build
./bin/shithub --version
```

Or, from any Go toolchain:

```sh
go install github.com/tenseleyFlow/shithub-cli/cmd/shithub@latest
shithub --version
```

Requires Go 1.26+. The `go install` path recovers the module version
and VCS metadata from Go's build info, so `shithub --version` reports
the tagged version even without the Makefile.

### v0.1.0+ (after C25 lands): from package managers

```sh
# macOS / Linux
brew install tenseleyFlow/tap/shithub

# Windows
scoop bucket add tenseleyFlow https://github.com/tenseleyFlow/scoop-bucket
scoop install shithub

# Debian / Ubuntu
# (apt repo + .deb release artifact, coming in C25)
```

## Quick start

```sh
# Authenticate. Use the browser flow (recommended)…
shithub auth login --hostname shithub.sh --web

# …or paste a PAT minted at https://shithub.sh/settings/tokens.
shithub auth login --hostname shithub.sh --with-token < token.txt

# Verify.
shithub auth status

# Create a repo + clone it.
shithub repo create my-project --public --add-readme --license MIT
shithub repo clone mfwolffe/my-project
cd my-project

# Open a PR.
git checkout -b feat/something
# ...edit...
git commit -am "feat: something"
git push -u origin feat/something
shithub pr create --fill

# Daily-driver verbs.
shithub pr list --state open
shithub pr view 1
shithub pr checkout 1
shithub issue create --title "bug: …" --body "…"
shithub repo view --web
```

`shithub <command> --help` shows every flag and example. Verbs broadly
mirror `gh`'s — a script that worked against `gh` should mostly work
against `shithub` with the binary name swapped.

## Differences from `gh`

- **Backend**: targets `shithub.sh` by default, not `github.com`. The
  hostname can be overridden per-invocation with `--hostname` or
  globally via `SHITHUB_HOST`.
- **Auth tokens**: `SHITHUB_TOKEN` / `SHITHUB_ENTERPRISE_TOKEN` /
  optionally `GITHUB_TOKEN` (opt in with
  `SHITHUB_ACCEPT_GITHUB_TOKEN=1`). See `shithub help environment` for
  the full precedence chain.
- **Deferred verbs**: a handful of `gh` commands (gists, releases,
  rulesets, projects, mention-based search) aren't yet wired
  server-side and surface as friendly "not yet supported" deferred
  stubs. `shithub <verb> --help` documents which features ship today.
- **AGPLv3** vs gh's MIT — server, CLI, and any extensions are
  AGPL-licensed.

## Where to file issues

- **CLI bugs / feature requests**: this repo's issues.
- **Server bugs**: [tenseleyFlow/shithub](https://github.com/tenseleyFlow/shithub) issues.
- **`gh`-compat divergences** (something `gh` does that `shithub`
  doesn't, or vice versa): either repo — we'll move it.

## License

[AGPL-3.0-or-later](./LICENSE). Matches the
[shithub](https://github.com/tenseleyFlow/shithub) server license.

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md). The sprint plans in
`.docs/sprints/` (gitignored locally, mirrored to the public spec
repo) are the source of truth for in-flight work.
