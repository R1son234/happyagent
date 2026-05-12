# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Test Commands

```bash
make build          # Build happyagent CLI to bin/happyagent
make build-eval     # Build eval runner to bin/happyagent-eval
make test           # Run all unit tests
make check          # Build check (compile only)
make eval-smoke     # Run smoke eval suite
make eval-profiles  # Run profile eval suite
make eval-career   # Run career copilot eval suite
```

Run a single test: `go test ./internal/engine/... -v`

## Architecture

The runtime is organized in layers:

```
cmd/happyagent          # CLI entrypoint: flags, subcommands, config loading
  └── internal/app      # Session management, persistence
       └── internal/runtime  # Runtime assembly: profile, tools, MCP, skills, engine
            ├── internal/profile   # Profile loading from profiles/<name>/profile.json
            ├── internal/llm        # Chat model interface (Eino/OpenAI adapter)
            ├── internal/tools      # Local tool registry (file, shell, capability tools)
            ├── internal/mcp        # MCP stdio client, remote tool registration
            ├── internal/skills     # On-demand skill loading
            ├── internal/engine     # Agent loop: plan/execute, tool execution, trace
            ├── internal/memory     # Turn-based memory with configurable limits
            └── internal/store      # Session/run JSON persistence under .happyagent/
```

Career Copilot (`internal/career`) is an application layer on top of the shared app/runtime stack with its own workspace, prompts, and report schema.

## Core Design Philosophy (from AGENTS.md)

The agent follows: **Plan → Execute → Verify → Learn**

Three core principles:
- **简洁优先**: Prefer the simplest, most direct solution. Minimize changes.
- **根因导向**: Fix root causes, don't patch symptoms. Verify before concluding.
- **最小影响**: Only modify what's necessary. Don't introduce unrelated refactoring.

Before making changes, at minimum understand:
- What problem is being solved
- What files are affected
- How to verify the result
- What risks exist and rollback options

## Change Planning Requirement

For any logic change, behavior change, data structure change, config format change, persistence change, agent loop change, MCP/skill integration change, memory/compression change, or tool protocol change, write a concrete implementation spec under `spec/` and wait for user confirmation before editing implementation code.

The spec must cover goal, non-goals, current context, affected files, implementation steps, behavior changes, compatibility and risks, validation plan, and acceptance criteria.

Only skip the `spec/` document when the user explicitly asks for direct implementation, or when the change is purely documentation, spelling, formatting, or comments with no runtime behavior impact.

## Agent Loop Behavior

1. Model receives a compact prompt with available tools/skills
2. Model calls `list_capabilities` to discover skill catalog
3. Model calls `activate_skill` to load skill instructions as observation
4. Engine enters loop: model emits structured actions (`tool_call` or `final_answer`)
5. Tool calls are validated, executed, and observations returned to model
6. Loop continues until `final_answer` or step limit reached
7. Trace output writes per-step actions, timing, token usage, tool-call status

## Profile System

Profiles in `profiles/<name>/profile.json` configure:
- `system_prompt`: Scoped instruction set
- `enabled_tools`: Subset of available tools
- `enabled_skills`: On-demand skill catalog subset
- `memory_strategy`: Turn count and char limits
- `output_schema`: Optional structured output validation
- `eval_suite`: Associated eval cases

Key profiles:
- `general-assistant`: Full tool access (file, shell, MCP resources)
- `career-copilot`: Restricted to evidence-first career workflows

## Tool Safety

- File tools restricted to configured root directory, reject symlink escapes
- Shell only runs configured allowed commands, executes via argv (not string interpolation)
- Dangerous tools (`shell`, `file_patch`) require explicit `--approve-tools` flag
- Write operations can require overwrite confirmation

## Career Copilot Workspace

The workspace at `career-workspace/` organizes job-search material:
- `inbox/` - intake folder for new material
- `resume/`, `jd/`, `experiences/`, `prepare/`, `my-interviews/` - typed libraries
- `record/` - operation trail (import logs, generated artifacts)
- `outputs/` - generated reports

Workspace commands in interactive mode: `/status`, `/add <type>`, `/export <kind>`, `/help`, `/exit`

## Session State

- Sessions: `.happyagent/store/sessions/<session_id>.json`
- Runs: `.happyagent/store/runs/<run_id>.json`
- Inspect: `./bin/happyagent --show-session <id>`, `--show-run <id>`, `--replay-run <id>`
- Metrics: `./bin/happyagent --metrics`

## Eval System

Eval cases defined in JSON with prompts, expected output substrings, required tools, and max steps. Reports written to `logs/eval/` with trace directories and Markdown summaries.
