# happyagent

`happyagent` is a local Go agent runtime and CLI application. It combines model-driven tool use, MCP integration, on-demand skills, persistent run history, structured traces, eval suites, and a Career Copilot workspace for evidence-grounded job-search material analysis.

## Features

- Agent loop with structured `plan` and `execute` steps.
- Profile-aware runtime with scoped prompts, tools, skills, memory strategy, output schema, and eval suite configuration.
- Local tools for file read/search/list/patch/write/delete, controlled shell execution, capability discovery, and final answers.
- MCP stdio client that registers remote tools and reads MCP resources with output bounds.
- On-demand skill loading through `list_capabilities` and `activate_skill`.
- Session and run persistence under `.happyagent/store/`.
- JSON run traces with step timing, tool-call status, token usage, and error categories.
- Eval runner for smoke, profile, and Career Copilot scenarios.
- Career Copilot CLI for maintaining a local interview library of resumes, JDs, public interview experience, project preparation, real interview records, and operation records.

## Repository Layout

```text
cmd/
  happyagent/        Main CLI entrypoint.
  happyagent-eval/   Eval runner.
  mcpdemo/           Small local MCP server for integration checks.
internal/
  app/               Session-oriented application layer.
  career/            Career Copilot workspace, prompts, ingestion, and generated records.
  config/            JSON config loading and environment overrides.
  engine/            Agent loop and action execution.
  eval/              Eval case runner.
  llm/               Chat model interface and Eino/OpenAI adapter.
  mcp/               MCP client, manager, tools, and resources.
  runtime/           Runtime assembly for profiles, tools, MCP, skills, and engine.
  tools/             Built-in local tools and safety boundaries.
docs/                Architecture, usage, and eval documentation.
examples/career/    Synthetic inputs for Career Copilot evals.
profiles/           Runtime profile definitions.
skills/             Local skills available to the runtime.
```

Generated local state is ignored by Git:

- `happyagent.local.json`
- `.happyagent/`
- `bin/`
- `logs/`
- `.gocache/`
- `.gomodcache/`

## Setup

Copy the config template:

```bash
cp config.example.json happyagent.local.json
```

Edit `happyagent.local.json` with your model configuration:

```json
{
  "llm": {
    "model": "gpt-4o-mini",
    "api_key": "your-api-key",
    "base_url": ""
  }
}
```

Build the CLI:

```bash
make build
```

Run a basic request:

```bash
./bin/happyagent --profile general-assistant "say hello in one sentence"
```

Start an interactive session:

```bash
./bin/happyagent --interactive --profile general-assistant
```

Continue an existing session:

```bash
./bin/happyagent --interactive --session-id <session_id> --profile general-assistant
```

Exit interactive mode with `/exit` or `Ctrl-D`.

## Career Copilot

Start the Career Copilot workspace:

```bash
./bin/happyagent career
```

The command creates `.happyagent/career/` and keeps all workspace material local. In the interactive prompt:

- `/status` shows workspace counts and active pointers.
- `/add <type>` archives material.
- `/export <kind>` generates Markdown material and saves it back into the relevant workspace area.
- `/help` lists available commands.
- `/exit` exits the workspace.

Supported material types:

- `jd`
- `resume`
- `prepare`
- `experiences`
- `my-interviews`
- `record`

The workspace creates this user-facing layout:

```text
.happyagent/career/
  resume/
  jd/
  experiences/
  prepare/
  my-interviews/
  record/
  workspace.json
  index.json
```

`record/` stores import logs, migration notes, generated process artifacts, and unclassified material. It is an operation trail, not the main QA library.

Examples:

```text
/add jd ./examples/career/real-world-anonymized/jd-marketing-growth.md
/add resume ./examples/career/real-world-anonymized/resume-marketing-anonymized.md
/add prepare "市场营销项目准备：活动复盘、用户增长案例、内容策略证据口径"
/export jd-match
```

The workspace also accepts natural-language references to local `.md`, `.txt`, `.docx`, and `.pdf` files. Markdown and text are extracted directly. DOCX and PDF ingestion use the repository's document extraction path and preserve the original file with extracted text in the workspace.

When an existing workspace still has legacy directories such as `jds/`, `resumes/`, `projects/`, `interview_experience/`, `interview_records/`, `review_notes/`, `reports/`, `exports/`, `search_sources/`, or `inbox/`, Career Copilot migrates recognizable content into the new layout and writes a migration note under `record/migrations/`. Unclassified legacy material is preserved under `record/unclassified/`.

Batch analysis is available through `career analyze`:

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

Run the anonymized real-world demo used for resume and interview evidence:

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

The repository also includes checked-in example outputs at `outputs/demo-career-report.md`, `outputs/demo-career-report.json`, and `outputs/demo-career-trace.example.json`.

Report transforms can be generated from the structured report:

```bash
./bin/happyagent career rewrite-resume --report outputs/career-report.json --out outputs/resume-bullets.md
./bin/happyagent career interview-brief --report outputs/career-report.json --out outputs/interview-brief.md
./bin/happyagent career gap-plan --report outputs/career-report.json --out outputs/project-gap-plan.md
```

## Profiles

Profiles live under `profiles/<name>/profile.json`. A profile can configure:

- `system_prompt`
- visible `tools`
- visible `skills`
- `memory_strategy`
- `output_schema`
- `eval_suite`

Included profiles:

- `general-assistant`
- `career-copilot`

Examples:

```bash
./bin/happyagent --profile general-assistant "summarize this repository"
./bin/happyagent --profile career-copilot "read README.md and produce a grounded career report"
```

When a profile declares `output_schema`, the runtime validates the final answer. Validation errors are sent back to the model as observations until the output is valid or the run reaches the step limit.

## Tools And Safety

The runtime exposes built-in tools through the same action protocol used for MCP tools. Local file tools are restricted to the configured root directory and reject symlink escapes outside that root.

Dangerous tools require explicit approval. For example:

```bash
./bin/happyagent \
  --profile general-assistant \
  --approve-tools shell,file_patch \
  "inspect git status and update README if needed"
```

The `shell` tool only runs configured allowed commands and executes via argv rather than string interpolation.

## MCP

Build the included MCP demo server:

```bash
make build
go build -o ./bin/mcpdemo ./cmd/mcpdemo
```

Add a server to `happyagent.local.json`:

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

Remote tools are registered as `<serverName>__<toolName>`. MCP resources are available through `list_capabilities` and `mcp_read_resource`, with byte and list-size limits from config.

## Trace And Store

Each CLI run writes local state to `.happyagent/store/`:

```text
sessions/<session_id>.json
runs/<run_id>.json
```

Useful inspection commands:

```bash
./bin/happyagent --show-session <session_id>
./bin/happyagent --show-run <run_id>
./bin/happyagent --replay-run <run_id>
./bin/happyagent --metrics
```

Export a run trace:

```bash
./bin/happyagent \
  --profile general-assistant \
  --trace-json logs/demo/run-trace.json \
  "say hello in one sentence"
```

Tool calls in traces are classified as:

- `attempted`: requested by the model
- `executed`: run by the runtime
- `successful`: completed successfully

## Eval

Run unit tests:

```bash
go test ./...
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

Eval reports, traces, and Markdown summaries are written under `logs/eval/`, including `logs/eval/career-summary.md`.

## Configuration

The runtime reads `happyagent.local.json` from the current working directory. The template is [config.example.json](config.example.json).

Supported environment overrides:

- `HAPPYAGENT_LLM_MODEL`
- `HAPPYAGENT_LLM_API_KEY`
- `HAPPYAGENT_LLM_BASE_URL`
- `HAPPYAGENT_SYSTEM_PROMPT`
- `HAPPYAGENT_LOOP_MAX_STEPS`
- `HAPPYAGENT_MAX_OBSERVATION_BYTES`
- `HAPPYAGENT_RUN_TIMEOUT_SECONDS`
- `HAPPYAGENT_ROOT_DIR`
- `HAPPYAGENT_SHELL_ENABLED`
- `HAPPYAGENT_SHELL_ALLOWED_COMMANDS`
- `HAPPYAGENT_WRITE_ENABLED`
- `HAPPYAGENT_WRITE_MAX_BYTES`
- `HAPPYAGENT_WRITE_REQUIRE_OVERWRITE`
- `HAPPYAGENT_DELETE_ENABLED`
- `HAPPYAGENT_DELETE_REQUIRE_CONFIRMATION`
- `HAPPYAGENT_MCP_CONNECT_TIMEOUT_SECONDS`
- `HAPPYAGENT_MCP_MAX_LISTED_RESOURCES`
- `HAPPYAGENT_MCP_MAX_RESOURCE_BYTES`
- `HAPPYAGENT_SKILLS_DIR`

## Documentation

- [Architecture](docs/architecture.md)
- [Usage Guide](docs/demo.md)
- [Eval Guide](docs/eval.md)
