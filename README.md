# shithub-cli

`shithub` — the command-line client for [shithub.sh](https://shithub.sh).

A 1:1 reverse engineering of GitHub's [`gh` CLI](https://cli.github.com/),
targeting [shithub](https://github.com/tenseleyFlow/shithub) as the
backend. AGPLv3, no Copilot, no AI features.

> Status: pre-1.0. Sprint roadmap drives feature delivery; see commits
> tagged `C00`..`C28`. Until v1.0.0, flags and output shapes may change
> between minor releases.

## Install

shithub-cli is in active development; binaries are not yet published.

Build from source:

```sh
git clone https://github.com/tenseleyFlow/shithub-cli.git
cd shithub-cli
make build
./bin/shithub --version
```

Requires Go 1.26+.

Once C25 (Distribution) ships, the canonical install paths are:

```sh
# macOS / Linux
brew install tenseleyFlow/tap/shithub

# Windows
scoop bucket add tenseleyFlow https://github.com/tenseleyFlow/scoop-bucket
scoop install shithub

# Debian / Ubuntu
# (apt package, .deb release artifact)
```

## Quick start

```sh
# Authenticate (paste a PAT minted at https://shithub.sh/settings/tokens)
shithub auth login --hostname shithub.sh --with-token < token.txt

# Verify
shithub auth status

# Clone a repo
shithub repo clone owner/repo

# Open a pull request
cd repo
git checkout -b my-fix
# ...edit...
git commit -am "fix: thing"
git push -u origin my-fix
shithub pr create --fill
```

A full getting-started guide ships with C26 (Documentation).

## License

[AGPL-3.0-or-later](./LICENSE). Matches the
[shithub](https://github.com/tenseleyFlow/shithub) server license.

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md).
