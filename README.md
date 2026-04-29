# happyagent

`happyagent` 是一个本地 Go Agent Runtime。当前实现已经覆盖 profile 宿主、session 应用层、run 持久化、基础 policy/memory/RAG、结构化 trace 和 eval 闭环。

## 当前范围

- Go 项目骨架
- JSON 配置加载和环境变量覆盖
- LLM 抽象接口
- 基于 `eino-ext` OpenAI ChatModel 的真实模型调用
- 本地工具
  - `file_read`
  - `file_search`
  - `search_docs`
  - `file_list`
  - `file_patch`
  - `file_write`
  - `file_delete`
  - `shell`
- MCP client
  - 当前支持通过子进程 stdio 连接 MCP server
  - 自动注册远程 tool
  - 支持按 URI 读取 resource
- Skill
  - 本地目录扫描与 catalog 暴露
  - `SKILL.md`
  - 通过 `activate_skill` 按需激活
  - 通过 `list_capabilities` 查看当前 skills 和 MCP resources
- Session / Store
  - 稳定 `session_id` / `run_id`
  - 本地 JSON 持久化
  - 支持 `show-session` / `show-run` / `replay-run`
- Observe / Policy
  - run events
  - 基础 metrics 汇总
  - 危险工具审批控制
- Memory / RAG / Validation
  - profile 驱动的短期 session memory，作为 runtime context 传入，不拼进 system prompt
  - 通过 `search_docs` 按需检索本地文档，不预注入 system prompt
  - profile 输出 schema 校验
- 激活结果会作为 tool observation 返回给模型
- 单 `Runner` + 内部 `plan step` / `execute step` 分层

## 运行

先复制一份本地配置：

```bash
cp config.example.json happyagent.local.json
```

然后把 `happyagent.local.json` 里的 `model` 和 `api_key` 改成你自己的配置。这个文件已经加入 `.gitignore`，不会被提交。

默认推荐先编译再运行，避免每次 `go run` 都重复走一次编译流程：

```bash
make build
./bin/happyagent
```

运行一个真实请求：

```bash
./bin/happyagent --profile general-assistant "say hello in one sentence"
```

开启 session 模式并继续同一个会话：

```bash
./bin/happyagent --session --profile general-assistant "summarize this repository"
./bin/happyagent --session-id <session_id> --profile general-assistant "now only focus on the tool system"
```

启动真正的交互式会话：

```bash
./bin/happyagent --interactive --profile general-assistant
```

继续一个已有会话的交互式模式：

```bash
./bin/happyagent --interactive --session-id <session_id> --profile general-assistant
```

交互模式下输入 `/exit` 或按 `Ctrl-D` 退出。

授权危险工具：

```bash
./bin/happyagent --profile general-assistant --approve-tools shell,file_patch "inspect the git status and update README if needed"
```

程序会固定读取当前工作目录下的 `happyagent.local.json`。

使用仓库内置的 demo MCP server：

1. 先构建主程序和 demo server：

```bash
make build
go build -o ./bin/mcpdemo ./cmd/mcpdemo
```

2. 在 `happyagent.local.json` 里加入一个 MCP server 配置：

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

3. 运行时就可以使用 `demo__repeat` 这个 MCP tool。MCP resource 读取能力已经在 runtime 里接好，但当前不再通过 skill frontmatter 自动注入。

也可以直接运行：

```bash
go run ./cmd/happyagent "say hello in one sentence"
```

导出单次运行的结构化 trace：

```bash
./bin/happyagent --profile general-assistant --trace-json logs/demo/run-trace.json "say hello in one sentence"
```

运行仓库内置 smoke eval：

```bash
make eval-smoke
```

运行 profile-aware eval：

```bash
make eval-profiles
```

## 配置

当前最小实现支持 JSON 配置文件，模板见 [config.example.json]，字段如下：

```json
{
  "llm": {
    "model": "gpt-4o-mini",
    "api_key": "your-api-key",
    "base_url": ""
  },
  "engine": {
    "loop_max_steps": 8,
    "max_observation_bytes": 8192,
    "run_timeout_seconds": 60,
    "system_prompt": "You are a local coding agent."
  },
  "tools": {
    "root_dir": ".",
    "shell_enabled": true,
    "shell_allowed_commands": ["cat", "echo", "find", "git", "go", "grep", "head", "ls", "make", "pwd", "printf", "rg", "sed", "tail", "wc"],
    "write_enabled": true,
    "write_max_bytes": 32768,
    "write_require_overwrite": true,
    "delete_enabled": false,
    "delete_require_confirmation": true
  },
  "mcp": {
    "connect_timeout_seconds": 15,
    "max_listed_resources": 100,
    "max_resource_bytes": 8192,
    "servers": []
  },
  "skills": {
    "dir": "skills"
  }
}
```

环境变量覆盖：

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

## Profile

当前 runtime 支持从 `profiles/` 目录加载多个 profile。每个 profile 使用独立的 `profile.json`，可以覆盖当前 run 的：

- `system_prompt`
- 可见 `tools`
- 可见 `skills`
- `memory_strategy`
- `output_schema`
- `eval_suite`

仓库内置两个最小 profile：

- `general-assistant`
- `career-copilot`

默认 CLI profile 是 `general-assistant`。示例：

```bash
./bin/happyagent --profile general-assistant "summarize this repository"
./bin/happyagent --profile career-copilot "read README.md and produce a grounded career report"
```

`career-copilot` 当前要求最终输出满足 `career_report` schema，也就是返回 JSON，并至少包含：

- `summary`
- `match_report`
- `rewrite_plan`
- `project_gap_analysis`

如果模型给出的最终答案不满足 schema，runtime 会把校验错误回送给模型并继续重试，直到完成或达到步数上限。

## Session And Trace

每次 CLI 运行现在都会落地到 `.happyagent/store/`，包含：

- `sessions/<session_id>.json`
- `runs/<run_id>.json`

可用命令：

```bash
./bin/happyagent --show-session <session_id>
./bin/happyagent --show-run <run_id>
./bin/happyagent --replay-run <run_id>
./bin/happyagent --metrics
```

`--metrics` 会同时输出 JSON 汇总和 Prometheus 文本格式指标。

trace 里当前会同时区分三类 tool call：

- attempted：模型发起了 tool call
- executed：runtime 实际执行了 tool
- successful：tool 成功执行并返回结果

文件与目录工具的 root dir 边界不仅会校验 `../`，也会拒绝通过 symlink 穿透到 root 外部。

## Skill

本地 skill 目录默认是 `skills/`。运行时不会再把 skill catalog、MCP resources 或 tool 列表塞进 system prompt；模型如果需要这些运行时能力信息，应调用 `list_capabilities`。需要某个 skill 时，再调用 `activate_skill` 加载它的详细说明。skill 目录使用通用的 `SKILL.md` 格式。

推荐格式：

```text
skills/<skill-name>/
└── SKILL.md
```

`SKILL.md` 示例：

```md
---
name: file-inspector
description: Inspect local files with the built-in file tools.
---

你当前扮演一个文件检查助手。
```

仓库里已经放了一个 demo skill，见 [skills/file-inspector/SKILL.md](/Users/r1son/Desktop/projects/happyagent/skills/file-inspector/SKILL.md:1)。
运行时默认会把 `activate_skill` 和 `list_capabilities` 暴露给模型。未激活前，system prompt 保持简洁；激活后，skill 内容会通过 tool observation 返回给模型，而不是直接改写 system prompt。

## MCP

当前 MCP 支持的配置方式是 stdio 子进程：

```json
{
  "mcp": {
    "connect_timeout_seconds": 15,
    "servers": [
      {
        "name": "demo",
        "command": "/path/to/mcp-server",
        "args": ["serve"],
        "env": {
          "TOKEN": "value"
        },
        "enabled": true
      }
    ]
  }
}
```

注册后，远程 tool 会以 `<serverName>__<toolName>` 的名字进入 agent 可用工具集合。
仓库内置的 demo server 位于 [cmd/mcpdemo/main.go](/Users/r1son/Desktop/projects/happyagent/cmd/mcpdemo/main.go:1)。

## 下一步

- 补更细粒度的 memory / RAG 策略
- 补更强的 policy 审批宿主
- 扩展更多真实业务 eval case
