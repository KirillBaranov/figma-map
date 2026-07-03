# Contributing to figma-map

Thanks for your interest in improving figma-map! This document covers the
development workflow and expectations for contributions.

## Getting started

```bash
git clone https://github.com/KirillBaranov/figma-map.git
cd figma-map
make build      # builds ./figma-map
make test       # unit tests with the race detector
make lint       # golangci-lint
```

Unit tests are offline (they use fixtures in `testdata/`); you do not need a
running Storybook, Figma bridge, or API key to run them.

## Development workflow

1. **Open an issue first** for anything non-trivial, so we can agree on the
   approach before you invest time.
2. **Branch** from `main`.
3. **Keep changes focused** — one logical change per pull request.
4. **Add tests** for new behaviour and bug fixes. A bug fix should come with a
   test that fails before the fix and passes after.
5. **Run the checks** locally before pushing:
   ```bash
   gofmt -w .
   make vet test lint
   ```
6. **Open a pull request** and fill in the template.

## Code style

- Standard Go style — code must be `gofmt`-clean and pass `go vet` and
  `golangci-lint run`.
- Prefer the existing patterns: small packages, interfaces at the seams
  (`figma.Source`, `matcher.Matcher`), table-driven tests.
- Comments are in English and explain *why*, not *what*.
- No `panic` for ordinary errors — return wrapped errors with context.

## Commit messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add Figma REST backend
fix: handle float screenshot dimensions from the bridge
docs: clarify bind review step
```

The release changelog is generated from commit prefixes, so `feat:`/`fix:`
matter; `docs:`, `test:`, and `chore:` are excluded from release notes.

## Testing against live services

The end-to-end flow (`scan` → `bind` → `map`) needs:

- a running Storybook (`storybook` URL in config),
- a running figma-bridge backend with a Figma file connected,
- an API key in the environment.

`figma-map doctor` verifies all four. Live runs cost API tokens, so keep them
deliberate and prefer offline unit tests for logic changes.

## Reporting bugs and requesting features

Use the issue templates. For bugs, include the command you ran, what you
expected, what happened, and `figma-map doctor` output where relevant.

## License

By contributing, you agree that your contributions are licensed under the
[MIT License](LICENSE).
