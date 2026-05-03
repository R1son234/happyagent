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

## Single Run Trace

Any normal CLI run can export a trace:

```bash
./bin/happyagent \
  --trace-json logs/demo/skill-trace.json \
  "Inspect this repository and summarize it in Chinese."
```

Use this path when debugging one prompt before adding it to an eval suite.
