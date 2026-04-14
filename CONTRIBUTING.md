# Contributing

`hax` is a testnet4 wallet project.

The most useful contributions right now improve correctness, safety, tests, docs, and UX consistency.

## Local Setup

Prerequisites:

- Go 1.25.4 or newer
- A terminal with Unicode support

Common commands:

```bash
make build
make test
make build-release
DEBUG=1 ./hax
```

## Development Guidelines

- Keep changes small and focused.
- Prefer fixes with tests over broad rewrites.
- Do not add mainnet claims or production-readiness language unless the underlying work is done.
- Keep docs aligned with actual behavior.
- Do not commit generated binaries, local logs, or local agent metadata.

## Tests

Before opening a pull request, run:

```bash
make test
make build
```

If you changed UI behavior, add or update the closest relevant test in `internal/ui/models`.

If you changed wallet logic, add or update tests in `internal/wallet`.

## Pull Requests

Please include:

- What changed
- Why it changed
- How you verified it
- Any follow-up work that is still open

Keep pull requests narrowly scoped when possible.

## Security Reports

Do not file public issues for vulnerabilities. Use the process in [SECURITY.md](SECURITY.md).
