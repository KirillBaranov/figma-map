# Install e2e test

Builds a "fake release" (CLI + backend + plugin, packaged exactly like a
real GitHub release) at two versions, then runs `install.sh` through the
full `bridge up` → `doctor` → `update` → `uninstall` cycle against it
inside a small matrix of Linux Docker images — catching distro-specific
shell/libc differences (`dash` vs `ash`, `curl` vs `wget`, glibc vs musl)
that a single dev machine won't.

## Run locally

```bash
make e2e-install
```

Requires `docker`, `go`, `bun`, and `node`/`npm` on `PATH`. Runs against
`ubuntu:24.04`, `debian:12`, and `alpine:3.20`.

**Alpine is expected to partially skip.** `bun build --compile`'s Linux
target assumes glibc; Alpine is musl, so the fetched backend binary may
fail to exec there (`fork/exec ...: no such file or directory` is the
tell). `assert-install.sh`'s `partial` mode treats that as a diagnosed
`SKIP`, not a failure — install, the on-disk paths, `update`, and
`uninstall` are still asserted fully on Alpine; only the "does the backend
actually start" checks are soft-failed.

## Files

- `build-fixtures.sh <version>` — builds one fixture version
  (`test/e2e/fixtures/v<version>/`, gitignored). Run twice with two
  different versions before the assertions (one to install, one to
  `update` into).
- `assert-install.sh <label> [full|partial]` — runs *inside* a container:
  install → verify on-disk paths → `bridge up` + `/ping` → `doctor`
  (bridge-reachable check only — plugin-connected legitimately fails with
  no real Figma attached) → `update` against the second fixture → `uninstall`.
- `run-e2e-local.sh` — starts the fixture HTTP server (as a container, so
  it's reachable from Docker-in-a-VM setups like Colima, not just native
  Linux) and runs `assert-install.sh` in each matrix image.

## Why the human/agent-run installer path is exercised faithfully

These fixtures and `FIGMA_MAP_BASE_URL` (an env override read by
`install.sh`/`install.ps1`/`internal/release.BaseURL`) exist solely so the
*real* download/checksum/extract code runs against local files instead of
real GitHub — nothing about the install/update/uninstall logic itself is
mocked. See [docs/onboarding-flow.md](../../docs/onboarding-flow.md) for
the design this test is checking.
