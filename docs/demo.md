# Usage Guide

This guide contains copyable local checks for the main runtime paths. All commands assume the repository root as the working directory.

## Prepare

```bash
cp config.example.json happyagent.local.json
make build
```

Set `llm.model`, `llm.api_key`, and optionally `llm.base_url` in `happyagent.local.json`.

## Skill Activation

Verify that skills are discovered through runtime capabilities and loaded only when requested:

```bash
./bin/happyagent \
  "Inspect this repository. If a skill is useful, call it first, then summarize the project structure in Chinese."
```

Expected signals:

- The run includes `list_capabilities` or `activate_skill`.
- Skill content is returned as a tool observation.
- The final answer reflects the activated skill's instructions.

## File Tools

Verify search, read, and patch orchestration:

```bash
./bin/happyagent \
  --approve-tools file_patch \
  "Search for a small wording issue in README.md, read the relevant lines, patch only the matching snippet, then summarize the change."
```

Expected signals:

- `file_search` locates candidate lines.
- `file_read` reads a scoped range.
- `file_patch` changes only the selected snippet.

Large file reads should return truncation markers instead of sending the entire file into the model context.

## MCP Integration

Build the local MCP demo server:

```bash
go build -o ./bin/mcpdemo ./cmd/mcpdemo
```

Add a server entry to `happyagent.local.json`:

```json
{
  "mcp": {
    "connect_timeout_seconds": 15,
    "servers": [
      {
        "name": "demo",
        "command": "./bin/mcpdemo",
        "args": [],
        "env": {},
        "enabled": true
      }
    ]
  }
}
```

Run a tool and resource check:

```bash
./bin/happyagent \
  "Use list_capabilities to inspect available MCP resources, call mcp_read_resource on one URI with max_bytes 128, then call the demo repeat tool with hello and explain what happened."
```

Expected signals:

- MCP resources appear in `list_capabilities`.
- `mcp_read_resource` returns bounded resource content.
- The remote tool is callable as `demo__repeat`.

## Career Workspace

Start the interactive workspace:

```bash
./bin/happyagent
```

Put sample materials into the visible inbox:

```bash
mkdir -p career-workspace/inbox
cp ./examples/career/real-world-anonymized/jd-marketing-growth.md career-workspace/inbox/
cp ./examples/career/real-world-anonymized/resume-marketing-anonymized.md career-workspace/inbox/
```

Then ask in natural language:

```text
我把简历和 JD 放进 inbox 了，帮我分析一下
```

Run the batch flow:

```bash
./bin/happyagent career analyze \
  --jd examples/career/real-world-anonymized/jd-marketing-growth.md \
  --resume examples/career/real-world-anonymized/resume-marketing-anonymized.md \
  --target examples/career/real-world-anonymized/target.md \
  --repo . \
  --out career-workspace/outputs/latest-report.md \
  --json career-workspace/outputs/latest-report.json \
  --trace-json logs/career/latest-trace.json
```

Run the anonymized real-world resume demo:

```bash
./bin/happyagent career analyze \
  --jd examples/career/real-world-anonymized/jd-marketing-growth.md \
  --resume examples/career/real-world-anonymized/resume-marketing-anonymized.md \
  --target examples/career/real-world-anonymized/target.md \
  --repo . \
  --out outputs/demo-career-report.md \
  --json outputs/demo-career-report.json \
  --trace-json logs/career/demo-trace.json
```

Expected outputs:

- Markdown report at `career-workspace/outputs/latest-report.md`.
- Structured JSON report at `career-workspace/outputs/latest-report.json`.
- Runtime trace at `logs/career/latest-trace.json`.
- For the anonymized demo, checked-in example reports are available at `outputs/demo-career-report.md` and `outputs/demo-career-report.json`.

## Trace

Export a single-run trace:

```bash
./bin/happyagent \
  --trace-json logs/demo/run-trace.json \
  "Search README.md for the shell tool notes, read the matching lines, then answer with one Chinese sentence that includes argv."
```

Trace files include:

- Step count and duration.
- Attempted, executed, and successful tool-call counts.
- Per-step actions and observations.
- Token usage when reported by the model provider.

## Eval

Run the smoke suite:

```bash
make eval-smoke
```

Run profile and Career Copilot suites:

```bash
make eval-profiles
make eval-career
```

Eval reports and per-case traces are written under `logs/eval/`.
Markdown summaries are also written, for example `logs/eval/career-summary.md`.
