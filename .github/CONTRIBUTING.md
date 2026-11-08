# Contributing to tele

Thanks for your interest in improving `tele` — a terminal-native Telegram client.
This document explains how to set up a development environment, the conventions
the project follows, and how to get your changes merged.

## Code of Conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md). By
participating, you are expected to uphold it.

## Ways to contribute

- **Report a bug** — open an issue using the [Bug report](ISSUE_TEMPLATE/bug_report.md) template.
- **Request a feature** — open an issue using the [Feature request](ISSUE_TEMPLATE/feature_request.md) template.
- **Submit a fix or feature** — see the workflow below.
- **Improve docs** — README, keybindings reference, and inline docs are all fair game.

For anything non-trivial, please open an issue first so we can agree on the
approach before you spend time on a pull request.

## Development setup

Requires **Go 1.26+** and your own [Telegram API credentials](https://my.telegram.org).

This repo uses [mise](https://mise.jdx.dev/) for tooling and tasks, and
[lefthook](https://github.com/evilmartians/lefthook) for git hooks.

```sh
git clone https://github.com/sorokin-vladimir/tele
cd tele

mise install        # installs the pinned Go toolchain and checking tools
mise run hooks      # installs git hooks via lefthook
```

Run the app against a local dev config:

```sh
mise run dev        # go run ./cmd/tele/ --config .config/tele/config.yml
mise run dev-e      # same, with debug logging
```

Build a binary (your API credentials are injected at build time):

```sh
go build \
  -ldflags "-X main.buildAPIID=YOUR_API_ID -X main.buildAPIHash=YOUR_API_HASH" \
  -o tele ./cmd/tele/
```

## Before you open a PR

Run the full check locally — this is also what the git hooks enforce on push:

```sh
mise run check      # go vet ./... + golangci-lint run ./... + go test ./... + govulncheck ./...
```

Individual tasks are available too:

| Task             | Command                       |
| ---------------- | ----------------------------- |
| `mise run test`  | `go test ./...`               |
| `mise run lint`  | `golangci-lint run ./...`     |
| `mise run vet`   | `go vet ./...`                |
| `mise run vuln`  | `govulncheck ./...`           |
| `mise run tidy`  | `go mod tidy`                 |

Code is formatted with `gofmt` (run automatically on commit via lefthook).

New behavior should come with tests. The project values test-driven changes —
add or update tests alongside the code they cover.

## Commit messages

Commits follow a [Conventional Commits](https://www.conventionalcommits.org/)
style, referencing the related issue where one exists:

```
fix: #123 Avoid flooding notify when run app after long idle
feat: #140 Forward messages between chats
docs: Update changelog
chore: release v1.3.1
```

Common types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`.

## Pull request process

1. Fork the repo and create a branch from `main`.
2. Make your change, including tests and doc updates where relevant.
3. Run `mise run check` and make sure it passes.
4. Update [`CHANGELOG.md`](../CHANGELOG.md) under the unreleased section if the
   change is user-facing.
5. Open the PR and fill in the template. Link the issue it resolves.

Maintainers review PRs on a best-effort basis. Keep PRs focused — one logical
change per PR is much easier to review than a large mixed one.

## License

By contributing, you agree that your contributions will be licensed under the
[GPL-3.0](../LICENSE) license that covers the project.
