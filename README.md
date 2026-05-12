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
./bin/happyagent
```

By default the CLI now opens the local Career Copilot workspace at `career-workspace/`. Put your prepared material into `career-workspace/inbox/`, then talk to it in natural language. Inbox scans copy recognized material into the typed workspace library and keep the original inbox files in place:

- `我把简历和 JD 放进 inbox 了，帮我分析一下`
- `帮我针对当前岗位优化简历`
- `帮我生成面试准备材料`
- `我刚面完，帮我复盘一下`

The interactive workspace keeps material local. Advanced commands are still available in the prompt:

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
career-workspace/
  inbox/
  resume/
  jd/
  experiences/
  prepare/
  my-interviews/
  outputs/
  record/
  workspace.json
  index.json
```

`record/` stores import logs, generated process artifacts, and unclassified material. It is an operation trail, not the main QA library.

Examples:

```text
/add jd ./examples/career/real-world-anonymized/jd-marketing-growth.md
/add resume ./examples/career/real-world-anonymized/resume-marketing-anonymized.md
/add prepare "市场营销项目准备：活动复盘、用户增长案例、内容策略证据口径"
/export jd-match
```

The workspace also accepts natural-language references to local `.md`, `.txt`, `.docx`, and `.pdf` files. Markdown and text are extracted directly. DOCX and PDF ingestion use the repository's document extraction path and preserve the original file with extracted text in the workspace.

Batch analysis is available through `career analyze`:

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

## Profiles

Profiles live under `profiles/<name>/profile.json`. A profile can configure:

- `system_prompt`
- visible `tools`
- visible `skills`
- `memory_strategy`
- `output_schema`
- `eval_suite`

`memory_strategy` can keep recent turns verbatim and, when enabled, add a deterministic structured summary of older turns:

```json
{
  "enabled": true,
  "max_turns": 6,
  "max_chars": 2000,
  "summary_enabled": true,
  "summary_max_chars": 4000,
  "summary_source_turns": 20
}
```

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

Large tool results can be offloaded to local files instead of staying in the model context. Configure this under `engine`:

```json
{
  "offload_enabled": true,
  "offload_min_bytes": 12000,
  "offload_dir": ".happyagent/offload"
}
```

When a non-final tool result reaches the threshold, happyagent writes the full output under `.happyagent/offload/<run-id>/` and returns a compact `file_read`-compatible reference in the observation. Offload files are local state under `.happyagent/` and are ignored by Git.

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
- `offloaded`: large result saved under `.happyagent/offload/<run-id>/`

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
