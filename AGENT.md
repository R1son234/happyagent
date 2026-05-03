# Agent Guide

This file gives repository-specific guidance for automated coding agents working on `happyagent`.

## Project Scope

`happyagent` is a local Go agent runtime and CLI. The core responsibilities are:

- structured model loop execution
- local tool registration and safety boundaries
- MCP stdio integration
- on-demand skill activation
- profile-scoped runtime behavior
- session, run, trace, and eval workflows
- Career Copilot workspace and report generation

Keep changes focused on these boundaries. Avoid unrelated rewrites, broad formatting churn, and new abstractions without a clear maintenance benefit.

## Key Paths

```text
cmd/happyagent/        Main CLI.
cmd/happyagent-eval/   Eval runner.
cmd/mcpdemo/           Local MCP demo server.
internal/app/          Session application layer.
internal/career/       Career Copilot workspace, ingestion, reports, prompts.
internal/config/       Config defaults, JSON loading, environment overrides.
internal/engine/       Agent loop and action execution.
internal/eval/         Eval execution.
internal/llm/          Chat model interface and provider adapters.
internal/mcp/          MCP client, manager, resources, tool adapters.
internal/profile/      Profile loading and validation.
internal/runtime/      Runtime assembly.
internal/tools/        Built-in tools and path safety.
docs/                  Architecture and usage docs.
eval/                  Eval case definitions.
examples/career/       Synthetic Career Copilot fixtures.
profiles/              Runtime profile definitions.
skills/                Local skills.
```

## Local State

Do not commit generated local state:

- `happyagent.local.json`
- `.happyagent/`
- `bin/`
- `logs/`
- `.gocache/`
- `.gomodcache/`

Test fixtures belong under package-level `testdata/` directories or `examples/` when they are meant to document user-facing flows.

## Common Commands

```bash
make build
go test ./...
make eval-smoke
make eval-profiles
make eval-career
```

Build the MCP demo server when testing MCP paths:

```bash
go build -o ./bin/mcpdemo ./cmd/mcpdemo
```

Run the CLI:

```bash
./bin/happyagent --profile general-assistant "say hello in one sentence"
./bin/happyagent --interactive --profile general-assistant
./bin/happyagent career
```

Run Career Copilot batch analysis:

```bash
./bin/happyagent career analyze \
  --jd examples/career/real-world-anonymized/jd-marketing-growth.md \
  --resume examples/career/real-world-anonymized/resume-marketing-anonymized.md \
  --target examples/career/real-world-anonymized/target.md \
  --repo . \
  --out outputs/career-report.md \
  --json outputs/career-report.json \
  --trace-json logs/career/latest-trace.json
```

## Implementation Notes

- Prefer existing package boundaries and naming conventions.
- Use `rg` for repository search.
- Keep local tools rooted in the configured workspace directory.
- Preserve symlink escape checks in file tools.
- Keep shell execution argv-based and constrained by config.
- Keep skill content out of the initial system prompt; use `list_capabilities` and `activate_skill`.
- Keep profile behavior declarative in `profiles/<name>/profile.json`.
- Keep Career Copilot user material local under `.happyagent/career/`.
- Use package-level `testdata/` for synthetic documents used by tests.

## Documentation

Update docs when changing user-visible behavior:

- `README.md` for setup, primary commands, and feature overview.
- `docs/architecture.md` for runtime structure and data flow.
- `docs/demo.md` for copyable local checks.
- `docs/eval.md` for eval suite behavior.

Documentation should describe current behavior, commands, paths, and constraints. Remove stale planning notes when the corresponding behavior is already implemented or no longer relevant.

## Verification

Choose the narrowest check that covers the change:

- package tests for local code edits
- `go test ./...` for cross-package behavior
- `make eval-smoke` for runtime/tool regressions
- `make eval-profiles` for profile and schema behavior
- `make eval-career` for Career Copilot report behavior

If a check cannot run because model credentials or external services are unavailable, record the command and the reason.
