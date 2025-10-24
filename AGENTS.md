# Repository Guidelines

## Onboarding & Architecture Reference
- Start by reading `DOCUMENTATION.md` for a walkthrough of architecture, component ownership, and common triage paths before tackling changes.
- Keep the document handy when planning fixes; it maps package responsibilities, runtime flow, and gotchas you should re-check during reviews.

## Project Structure & Module Organization
- Root CLI entry point is `main.go`; domain logic lives under `pkg/` (e.g. `executors/`, `listener/`, `kubernetes/`) with packages named for their subsystem.
- Runtime assets such as sample configs reside in `config.example.yaml`, release docs live in `docs/`, and Docker/Vagrant files support local environments; built binaries land in `build/`.
- Tests and fixtures sit in `pkg/*/*_test.go` for unit coverage and Ruby E2E helpers in `test/` (see `test/e2e` and `test/e2e_support`).

## Build, Test, and Development Commands
- `make build` compiles a Linux CLI at `build/agent`; use `make build.windows` for a Windows artifact or `make docker.build.dev` for a dev image.
- `make test` runs `gotestsum` across `./...` with a JUnit report; run `go test ./pkg/listener -run TestName` to iterate on a single package.
- `make run JOB=<job-file>` executes the agent locally; `make serve` starts the API shim with a static auth token for manual workflows.
- `make lint` executes `revive` with `lint.toml`; the `check.*` targets pull the security-toolbox for deeper audits when needed.

## Coding Style & Naming Conventions
- Target Go 1.23; always format with `gofmt` (or `go fmt ./...`) before pushing and organize imports with `goimports`.
- Exported identifiers follow Go’s PascalCase, package-private helpers use camelCase, and test doubles mirror the production type names with `Fake` or `Stub` suffixes.
- Keep configuration structs in `pkg/config` aligned with `config.example.yaml`; when introducing flags, add them to `pkg/server` command wiring.

## Testing Guidelines
- Co-locate `*_test.go` files with their packages and favor table-driven tests plus `testify` assertions already in use.
- Aim to cover new branches and failure paths, especially around `pkg/executors` and `pkg/listener` which guard remote job execution.
- For smoke verification against remote services, add or extend Ruby scripts in `test/e2e` and run with `make e2e TEST=<script>`; share new scripts in review.

## Commit & Pull Request Guidelines
- Follow the existing Conventional Commit style (`feat:`, `fix:`, `chore:`) and reference the PR number or issue in parentheses, e.g. `fix: harden retry backoff (#250)`.
- Each PR should describe motivation, key changes, rollout considerations, and include `make test` / `make lint` output; attach logs or screenshots when touching user-facing behavior.
- Update `CHANGELOG.md` when shipping externally visible changes and call out any config or API migrations in the PR checklist.

## Security & Configuration Tips
- Use `config.example.yaml` as a baseline; never commit real tokens or certificates—store overrides in local `.yaml` files ignored by git.
- Rotate test certificates in `server.crt` / `server.key` if reused and prefer the supplied Docker targets (`docker.build`, `empty.ubuntu.machine`) for sandboxed validation.
