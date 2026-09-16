# Contributing Guide

We welcome all contributions!

This repo is the official GrowthBook SDK for Go — a client library for
evaluating feature flags and running experiments in Go applications.

## Requirements

- **Go 1.22+**, as declared in `go.mod`. This constrains which language and
  standard library features you can use — see
  [Language version](#language-version).
- **Python 3** — only if you touch the conformance corpus (`cases.json`).
- **golangci-lint** — optional locally, but CI runs it:
  ```sh
  go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
  ```
  This resolves to v1.x, matching CI. Don't use v2 (what Homebrew ships) —
  it can't read this repo's v1-format `.golangci.yml`.

Optionally, `flake.nix` provides the whole toolchain via `nix develop`, or
automatically on `cd` if you use [direnv](https://direnv.net/) (`direnv
allow`). Neither is required.

## Getting started

Fork the repo, or clone directly if you have write access:

```sh
git clone git@github.com:growthbook/growthbook-golang.git
cd growthbook-golang
go mod download
go build ./...
go test ./...
```

Dependencies are declared in `go.mod` and fetched on demand — there's no
install step, no `vendor/`, and no generated code. `./...` means "this package
and everything under it". Get a green baseline before you start writing code.

> `go.mod` pins `toolchain go1.22.7`. If your Go is older, that toolchain is
> downloaded automatically on first build; a newer Go is used as-is.

## Writing code

### Layout

Go keeps a package's files flat in one directory, so the root looks busier than
it is — everything there is one package, `growthbook`.

| Path | Contents |
| --- | --- |
| `client*.go` | The `Client` type and its functional options |
| `evaluator.go`, `feature*.go`, `experiment*.go` | Feature and experiment evaluation — the core |
| `datasource*.go` | Feature loading: polling, SSE, custom sources |
| `sticky_bucket.go`, `contextual_bandit.go` | Sticky bucketing; contextual bandits |
| `tracking*.go`, `event_logger.go`, `plugin.go` | Exposure tracking and event forwarding |
| `internal/condition/` | Targeting conditions (`$in`, `$elemMatch`, version compare, …) |
| `internal/value/` | The dynamic JSON value type used during evaluation |
| `cases.json`, `cases_test.go` | Cross-SDK conformance corpus |

Anything under `internal/` is invisible outside the module — a Go language
rule, not a convention — and can be refactored freely. The root package is the
public API.

### Public API

Every exported (capitalized) identifier in the root package is public API,
consumed via
[pkg.go.dev](https://pkg.go.dev/github.com/growthbook/growthbook-golang).
Adding is cheap; changing or removing is a breaking change and needs an issue
first.

Configuration uses functional options — `WithClientKey`, `WithAttributes`, and
friends. New configuration should follow that pattern rather than adding struct
fields or constructor arguments, both of which break callers.

### SDK parity

GrowthBook's SDKs must make identical decisions given identical inputs, and the
JavaScript SDK is the reference implementation. Match it. When you deliberately
diverge, say so in the PR and record it in `CHANGELOG.md` — see the "Deliberate
divergences" entries under v0.5.0 for the expected level of detail. Silent
divergence is the thing to avoid.

### Concurrency

`Client` is safe for concurrent use, and children created by `WithAttributes`
and the other `With*` methods share state with their parent (caches, data
sources, tracking buffers). If you add mutable state, decide explicitly whether
it's shared or cloned, and guard it.

### Logging

Use `log/slog` through the client's logger, and take a `context.Context` on
public methods. Never write to stdout or `log` from library code.

### Language version

`go.mod` declares `go 1.22` and pins `toolchain go1.22.7`, so 1.22 is the
floor. Be aware that CI's version matrix does not enforce it: because of the
toolchain directive, the job labelled "Go 1.21" downloads go1.22.7 and builds
with that, so nothing in CI actually compiles against 1.21.

pkg.go.dev shows the version each symbol was added in. Raising the minimum is a
decision for an issue, not a PR side effect.

## Testing

Tests live beside the code they cover, in `_test.go` files. `testify`'s
`require` is the house assertion library, and `test_utils.go` holds shared
helpers — including `testLogger`, which prints captured logs only when a test
fails.

```sh
go test ./...                              # everything
go test -v -race -timeout=15m ./...        # what CI runs
go test ./internal/condition/              # one package
go test -run TestClientEvalFeatures ./...  # one test (the argument is a regex)
go test -coverprofile=coverage.out ./...   # coverage
```

**Always run with `-race` before opening a PR.** This SDK is concurrent by
design — background data sources, shared caches, cloned clients — and the race
detector catches the class of bug that's hardest to spot by reading. CI runs
it, so you'll find out either way.

Add tests with your change: a bug fix should come with a test that fails before
it and passes after.

### Conformance corpus

`cases.json` is a shared corpus mirrored from the [JS
SDK](https://github.com/growthbook/growthbook/blob/main/packages/sdk-js/test/cases.json)
and run by `cases_test.go`. It's how every GrowthBook SDK proves it evaluates
features, conditions, hashing, and bucketing identically. **If a corpus case
fails, assume your change is wrong, not the corpus.**

CI checks the corpus for drift two ways: the PR gate compares against the JS
revision pinned as `PINNED_JS_CASES_URL` in
`.github/workflows/corpus-freshness.yml`, while a weekly audit compares against
live JS `main`. Run either locally:

```sh
python3 tests/scripts/check_corpus_freshness.py --js-source <pinned-url>  # the PR gate
python3 tests/scripts/check_corpus_freshness.py                          # live JS main
```

When you sync `cases.json` from upstream, bump `PINNED_JS_CASES_URL` in
`.github/workflows/corpus-freshness.yml` in the same commit. Intentional
differences belong in `tests/scripts/corpus_skiplist.json`.

### Benchmarks

```sh
go test -bench=. -benchmem -run=^$ -benchtime=2s .
```

`-run=^$` matches no regular tests, so only benchmarks run. If you touch the
eval path, record before/after numbers in `docs/benchmarks/baseline.md`
following the format at the top of that file.

### Live tracking test

`tracking_plugin_live_test.go` sends real events to the GrowthBook ingestor and
is build-tagged out of normal runs. You probably don't need it:

```sh
GB_CLIENT_KEY=sdk-XXXX go test -tags live_tracking_test -run TestLiveTrackingPlugin -v
```

## Code quality

```sh
gofmt -l .                       # lists files needing formatting
go vet ./...                     # standard correctness checks; fast, catches real bugs
golangci-lint run --timeout=5m   # the linter CI runs (config: .golangci.yml)
```

Most editors run `gofmt` on save. **Format only the files you touched** — a
couple of files in the repo aren't gofmt-clean, and `gofmt -w .` would sweep
them into your diff as unrelated noise.

`errcheck` is deliberately disabled in `.golangci.yml`, because some unchecked
errors here are intentional. Don't re-enable it as a drive-by change.

## Opening pull requests

Open an issue first for API changes, new configuration, or behavior changes —
cheaper than discovering at review time that the design needs to change. Bug
reports and feature requests have
[templates](https://github.com/growthbook/growthbook-golang/issues/new/choose).
For a typo or an obvious fix, just open the PR.

1. Branch from `main`. Naming is loose; existing branches use both
   `feat/thing` and `yourname/thing`.
2. Keep the diff focused — send unrelated refactors and reformatting
   separately.
3. Check your work:
   ```sh
   gofmt -l .          # your files shouldn't be listed
   go vet ./...
   go build ./...
   go test -race ./...
   ```
4. Commit and push. Messages are plain imperative summaries ("Fix data race on
   shared sticky bucket assignments cache"); conventional-commit prefixes like
   `fix:` appear too and are fine, but nothing is enforced.
5. Open the PR against `main` and fill in the template: what changed,
   dependencies, how to test, and any issues it closes. Delete the Screenshots
   section — it comes from the main GrowthBook repo and rarely applies to an
   SDK PR. Call out parity implications explicitly.

CI (`.github/workflows/ci.yml`) runs lint, tests with `-race` across the Go
version matrix, a `gosec` scan (advisory, non-blocking), and builds across
Linux, macOS, and Windows. Touching `cases.json` or `tests/scripts/` also triggers the
corpus freshness check. The most common failure is the race detector, which reproduces
locally.

Push follow-up commits rather than force-pushing where you can; it keeps review
comments anchored to the code they were written about.

## Releasing

Maintainers handle releases. `CHANGELOG.md` is updated at release time, and
pushing a `v*` tag triggers `.github/workflows/release.yml`, which re-runs the
tests and publishes a GitHub release:

```sh
git tag v0.5.2
git push origin v0.5.2
```

Go modules are served straight from the Git tag, so a published version is
immutable — which is why breaking changes get the scrutiny they do.
Contributors don't need to touch `CHANGELOG.md` or version numbers; what helps
is a PR description a maintainer can turn into a changelog entry without
guessing.

## Getting help

- [Slack community](https://slack.growthbook.io?ref=contributing) — we're happy
  to help you get set up
- [Open an issue](https://github.com/growthbook/growthbook-golang/issues)
- [Go SDK docs](https://docs.growthbook.io/lib/go) ·
  [GoDoc](https://pkg.go.dev/github.com/growthbook/growthbook-golang)

Found a security vulnerability? Email security@growthbook.io — **don't file a
public issue**. See the [security
policy](https://github.com/growthbook/growthbook/blob/main/SECURITY.md).

Contributors are expected to follow the [Code of
Conduct](https://github.com/growthbook/growthbook/blob/main/CODE_OF_CONDUCT.md).
