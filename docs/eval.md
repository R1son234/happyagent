# Eval Guide

`happyagent` includes deterministic eval suites for regression checks across the general runtime, profile loading, structured output validation, and Career Copilot reporting.

## Suites

| Suite | File | Purpose |
| --- | --- | --- |
| Smoke | `eval/smoke_cases.json` | Basic capability checks for skills, file tools, and repository inspection. |
| Profile | `eval/profile_cases.json` | Profile selection, scoped behavior, and structured output checks. |
| Career Copilot | `eval/career_cases.json` | Evidence-grounded career report generation, resume rewrite, hallucination controls, tool safety, Trace/Eval story, and interview preparation. |

Each case can define:

- fixed prompt
- profile
- timeout
- expected output substrings
- required tools
- maximum step count

Infra regression scenarios should cover:

- hook ordering and hook decisions for block, inject, force-continue, and record-only behavior
- policy priority, including deny-over-allow and ask/block observations for dangerous tools
- context compaction and prompt-too-long recovery without invalid tool-call pairing
- `agent_task` child runs with fresh context, summary-only observation, hidden recursive agent tools, and stored child trace
- async teammate mailbox append/list/mark-consumed behavior
- task DAG validation, dependency blocking, release, completion, and concurrent claim
- background/team completion notifications before model calls, including `shell` jobs started with `run_in_background`
- worktree slug validation, cwd root safety, `worktree_keep`, child-agent shell cwd override, and dirty-remove refusal
- MCP canonical naming with `mcp__server__tool`, server status in capabilities, and dangerous-by-default remote tools

Current non-goal coverage:

- Do not add eval expectations for model-facing `mcp_connect` or `mcp_disconnect`; MCP servers are connected from local config during runtime assembly.
- Do not add eval expectations for teammate idle-loop task-board scanning; task claims are explicit through task tools or Lead-delegated child runs with `task_id`.

## Run

Build the CLI and eval runner:

```bash
make build
make build-eval
```

Run smoke eval:

```bash
make eval-smoke
```

Run profile-aware eval:

```bash
make eval-profiles
```

Run Career Copilot eval:

```bash
make eval-career
```

The underlying eval runner can also be called directly:

```bash
./bin/happyagent-eval \
  -cases eval/smoke_cases.json \
  -output logs/eval/smoke-report.json \
  -trace-dir logs/eval/smoke-traces \
  -summary logs/eval/smoke-summary.md
```

## Outputs

Suite reports include:

- `passed_count`
- `failed_count`
- `success_rate`
- `average_steps`
- `average_tool_calls`
- `average_executed_tool_calls`
- `average_successful_tool_calls`
- `average_duration_millis`
- token totals
- `error_categories`

Markdown summaries include:

- pass/fail count and success rate
- average steps and tool-call counts
- attempted/executed/successful tool-call table
- per-case status table
- failure reasons and missing expectations

Per-case traces include:

- profile
- input prompt
- final output
- step actions
- tool-call status
- observations
- planning and execution duration
- token usage
- attempted, executed, and successful tool-call counts
- error category
- hook decisions
- compaction events
- recovery attempts
- transcript path when prompt-too-long recovery writes one

## Single Run Trace

Any normal CLI run can export a trace:

```bash
./bin/happyagent \
  --trace-json logs/demo/skill-trace.json \
  "Inspect this repository and summarize it in Chinese."
```

Use this path when debugging one prompt before adding it to an eval suite.
