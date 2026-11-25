# Contributing to tele

Thanks for wanting to make `tele` better.
This page says how to get a change in without surprises on either side.

## Code of Conduct

This project follows the [Contributor Covenant](../CODE_OF_CONDUCT.md).
By taking part, you agree to keep it.

## Before you start

Open an issue first for anything bigger than a small fix: a feature, a new key or setting, a change in how something behaves.
Say there that you would like to build it, and wait for an answer before you write the code.
`tele` has a design behind it, and a pull request that arrives without a talk first may be asked to change a lot, or to grow into something wider than you planned.

A small fix (a typo, an obvious bug in a few lines) can come straight as a pull request.

Issues labelled [`good first issue`](https://github.com/sorokin-vladimir/tele/labels/good%20first%20issue) are small, local, and say what "done" means.
They are a good place to start.

## Setup

You need Go 1.26 or newer.
A build from source runs without registering anything: it carries the shared app key described in [app-key.md](../docs/app-key.md).

The repo uses [mise](https://mise.jdx.dev/) for tools and tasks and [lefthook](https://github.com/evilmartians/lefthook) for git hooks:

```sh
git clone https://github.com/sorokin-vladimir/tele
cd tele

mise install        # the pinned Go toolchain and checking tools
mise run hooks      # git hooks via lefthook
```

More on building, Nix and gotd is in [development.md](../docs/development.md).

## Try it live

Tests are not enough.
Every change you can see in the window or on the command line has to be run by you before review:

```sh
mise run dev        # tele with the dev config, .config/tele/config.yml
mise run dev-e      # the same, with debug logging
```

Log in with your own account and go through what you changed, including the edge cases.
In the pull request, write what you checked.
A pull request that was never run is not reviewed.

## Using an AI assistant

You may use one.
You are responsible for the code all the same: every line in the pull request is yours, you should be able to explain any of it in review, and you run it live as above.
If an assistant wrote most of the code, say so in the pull request.

## Code

- New behaviour comes with tests, and the tests check behaviour, not how the code is built.
- No test talks to Telegram: the Telegram layer sits behind the `Client` interface, which tests replace.
- `internal/ui` never imports `internal/tg`. The window talks to the owner of the account through its calls and the updates it sends.
- A new setting is declared in `internal/config/registry.go`, says whose it is (host or client), and gets a `TestTakesHold_<key>` test where its effect is seen. The registry tests tell you what is missing.
- Before raising the gotd version, read [gotd-workarounds.md](../docs/gotd-workarounds.md).

## Drawing in the terminal

A theme can paint the whole screen, so every cell `tele` draws has to carry the background.
A cell that does not shows the terminal's own colour through, as a hole.

- Make styles with `theme.NewStyle`, never `lipgloss.NewStyle`.
- Pad a row with `theme.Pad` or `theme.PadTo`, never with plain spaces.
- A text cut short with `ansi.Truncate` gets an unstyled tail: pass a styled `…`.
- A new theme token needs an issue first.

`internal/ui/canvas_guard_test.go` refuses the first two at build time.
The rest is on you and on the review.

## Docs

The English docs are written one sentence per line, not wrapped at 80 columns.
A list item's next sentences go on their own lines, indented two spaces.

A change in behaviour comes with the change in the English docs.
You may update the Russian and Chinese translations too, but leave the commit in their first line as it is: it names a commit on `main`, which your change does not have until it is merged.
If you do not translate, that is fine: the maintainer does it after the merge.
See [translating.md](../docs/translating.md).

## CHANGELOG

A change a person would notice gets a line in [`CHANGELOG.md`](../CHANGELOG.md), under `[Unreleased]`, ending with the issue number: `(#123)`.
Releases happen while a pull request waits, so merge the latest `main` before it is merged and keep your line under `[Unreleased]`.

## Commits and merging

Commit subjects look like this:

```
fix: #123 Keep the cursor on screen after a resize
feat: #140 Forward messages between chats
docs: Describe the proxy settings
```

The subject is 50 characters at most, or 70 when it cannot be shorter.
Common types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`.

Pull requests are squash-merged, so the pull request's title becomes the commit: give it the same shape.
Keep one change to a pull request.
Unrelated fixes found on the way go in their own pull requests, however small.

## Checks

Run the full check before you push; the git hooks run it too:

```sh
mise run check      # go vet, golangci-lint (for every OS), go test, govulncheck, shellcheck, jactionlint, betterleaks
```

GitHub does not run the workflows on pull requests from forks, so the maintainer runs the checks on your branch by hand.
A pull request that fails `mise run check` waits until it passes.

## Review

Reviews are best effort, and big pull requests take longer.
The maintainer may push a small fix to your branch (a merge of `main`, a CHANGELOG line) instead of asking for it.

## License

By contributing, you agree that your contributions are licensed under the [GPL-3.0](../LICENSE) that covers the project.
