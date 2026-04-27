# happyagent

`happyagent` 是一个按最小可行路径实现的本地 Agent Runtime。当前代码重点是先把项目骨架、配置加载、本地工具注册、单 Runner ReAct loop 和 runtime 装配层搭起来。

## 当前范围

- Go 项目骨架
- JSON 配置加载和环境变量覆盖
- LLM 抽象接口
- 基于 `eino-ext` OpenAI ChatModel 的真实模型调用
- 本地工具
  - `file_read`
  - `file_search`
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
./bin/happyagent "say hello in one sentence"
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
./bin/happyagent --trace-json logs/demo/run-trace.json "say hello in one sentence"
```

运行仓库内置 smoke eval：

```bash
make eval-smoke
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

- 补 MCP 和 skill 的集成测试
- 补受控审批流和更细粒度审计
