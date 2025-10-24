# Repository Architecture Notes

## 1. High-Level Overview
- Purpose: Semaphore self-hosted agent CLI written in Go 1.23; registers to the Semaphore SaaS, pulls jobs, runs them via pluggable executors, and reports status/logs.
- Binaries: `main.go` builds a single `agent` CLI (see `make build`, `make build.windows`). Optional `serve` command spins up a lightweight local HTTP API (`pkg/server`) mainly for integration tests and hooks.
- Configuration: Defaults come from `pkg/config/config.go`; runtime values flow from CLI flags (`pflag`), env vars (`SEMAPHORE_AGENT_*`), or a YAML file (`config.example.yaml` as template).

## 2. Runtime Flow & Entry Points
- `main.go` bootstraps logging, loads config, then either `RunListener` (default) or HTTP server handlers depending on CLI args.
- `pkg/listener/listener.go` orchestrates lifecycle: registers the agent (`pkg/api`), long-polls for jobs, spawns `JobProcessor`.
- `pkg/listener/job_processor.go` prepares workspace, applies hooks (pre/post job), initializes event logging, selects executor, streams updates back.
- Graceful shutdown logic (signals, idle timeout) is centralized through `pkg/listener` and `shutdown_reason.go`.

## 3. Core Packages & Responsibilities
- `pkg/api`: HTTP client models for Semaphore endpoints (register agent, fetch job requests). Requires endpoint/token from config.
- `pkg/jobs`: Domain model for jobs (commands, files, secrets) with helper logic around panic recovery and resource locks.
- `pkg/executors`: Strategy interface plus implementations (`shell_executor`, `docker_compose_executor`, `kubernetes_executor`). Handles workspace setup, command execution, log streaming.
- `pkg/eventlogger`: Multiplexed logging backends (in-memory, file, HTTP). Default pipeline: formatter → `httpbackend` (streams to Semaphore) with file or stdout mirrors.
- `pkg/httputils`, `pkg/osinfo`, `pkg/random`, `pkg/retry`: shared utilities to keep domain packages focused.
- `pkg/kubernetes`, `pkg/docker`, `pkg/aws`: helper modules invoked by executors for cluster API interactions, Docker Compose templating, and AWS metadata respectively.
- `pkg/server`: Local HTTP server used for self-hosted coordination (`/jobs`, `/status` etc.), typically driven through `make serve` or tests.

## 4. Job Lifecycle (Happy Path)
1. Listener authenticates with `POST /agents/register` using token.
2. Long polling obtains `JobRequest` (see `pkg/api/job_request.go`).
3. Processor sets up workspace: downloads files (`pkg/listener.ParseFiles`), exports env vars, runs pre-job hook if configured.
4. Executor runs commands (selected from job payload). Shell executor runs directly on host; Docker Compose builds ephemeral services; Kubernetes executor schedules pods using supplied pod spec and allowed image list.
5. Event logs are appended via `pkg/eventlogger.Logger`, forwarded to Semaphore and optionally flushed to disk.
6. Post-job hook runs, artifacts/log uploads occur if enabled (`config.UploadJobLogs*`).
7. Processor reports status, acknowledges completion; listener loops for next job or exits based on disconnect flags.

## 5. Configuration & Secrets
- CLI flags defined in `RunListener`; env vars map 1:1 via `SEMAPHORE_AGENT_<FLAG>` (dashes become underscores).
- File injections support `--files path:dest` pairs; validation handled in `pkg/listener/files.go`.
- Hooks (`--shutdown-hook-path`, `--pre-job-hook-path`, `--post-job-hook-path`) execute via shell; when `--source-pre-job-hook` is set, the script runs within the current shell.
- Sensitive data (tokens, certs) must never be committed—use local overrides. Example config lives in repository solely for documentation.

## 6. Logging & Metrics
- Logging uses `logrus` with levels derived from `SEMAPHORE_AGENT_LOG_LEVEL`; defaults to info, writes to rotating file under `$TMPDIR/agent_log` unless overridden.
- Event logs leverage chunked HTTP uploads, with optional plain-text file backup (`pkg/eventlogger/filebackend`).
- StatsD support is provided via `gopkg.in/alexcesaro/statsd.v2` (see `pkg/listener/selfhostedapi` for references).

## 7. Testing Strategy
- Unit tests co-located with code (`*_test.go`). `test` target runs via `gotestsum` capturing JUnit reports.
- Integration/end-to-end tests live in `test/e2e` (Ruby scripts) with Docker Compose helpers under `test/e2e_support`. Trigger using `make e2e TEST=<script>`.
- Static analysis: `make lint` (revive). Security/regression suites via `make check.static`, `check.deps`, `check.docker` requiring `renderedtext/security-toolbox`.
- Benchmarks example: `make test.bench`, mostly targeting performance hotspots like event logging.

## 8. Build, Packaging & Release
- `Makefile` contains canonical targets for building cross-platform binaries, docker images (`docker.build`, `docker.push`), and release tagging (`release.minor/major/patch`).
- Docker images use `Dockerfile.self_hosted` for production, `Dockerfile.dev` for local dev, `Dockerfile.ecr` for AWS integration tests, and `Dockerfile.empty_ubuntu` for sandbox experiments.
- Release automation integrates with Semaphore pipelines; tagging triggers builds that publish to GitHub and Homebrew via scripts referenced in `README.md`.

## 9. Key Directories & Files
- `docs/`: supplemental guides (currently Kubernetes executor details).
- `examples/`: sample configurations/scripts for self-hosted deployments.
- `scripts/`: utility scripts for CI/CD tasks, log collection, or environment bootstrapping.
- `config.example.yaml`: baseline YAML demonstrating all config fields.
- `lint.toml`: revive configuration (style rules, ignores).
- `large_log.txt`: fixture for event logger performance tests.
- `run*/`: captured logs/traces from local executions—safe to ignore unless debugging historical failures.

## 10. Common Pitfalls & Tips
- Many commands assume Docker/Compose availability; ensure local environment mirrors production (see `empty.ubuntu.machine` target).
- Kubernetes executor requires `SEMAPHORE_KUBECONFIG` or in-cluster config plus allowed image regexes; missing configs lead to pod creation failures early in the job run.
- When modifying job processing, keep concurrency limits and interrupt handling in mind (`InterruptionGracePeriod`, `DisconnectAfterIdleTimeout`).
- Event logger writes large buffers; prefer streaming to avoid memory bloat. See recent commit `fix: improve eventlogger.GeneratePlainTextFile()` for context on large files.
- Before touching release scripts, review `CHANGELOG.md` and follow semantic versioning via `make tag.*` targets.

## 11. Getting Oriented Quickly
- Start debugging by running `make serve` + `make run JOB=test/job.json` using fixtures in `examples/` to reproduce flows.
- Trace a job end-to-end via `pkg/listener/listener.go:Run()` (entry point), `pkg/listener/job_processor.go:Process()`, and chosen executor implementation.
- For API schema updates, adjust `pkg/api` models first; downstream packages (`pkg/jobs`, `pkg/executors`) typically only consume typed structs.
- Use `pkg/eventlogger/test_helpers.go` to stub logging when writing tests around job execution.
- Architecture diagram suggestion: Listener ↔ Semaphore API ↔ Executor ↔ Workspace; keep this mental model handy.
