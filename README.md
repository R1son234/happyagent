# happyagent

`happyagent` 是一个按最小可行路径实现的本地 Agent Runtime。当前代码重点是先把项目骨架、配置加载、本地工具注册、单 Runner ReAct loop 和 runtime 装配层搭起来。

## 当前范围

- Go 项目骨架
- JSON 配置加载和环境变量覆盖
- LLM 抽象接口
- 基于 `eino-ext` OpenAI ChatModel 的真实模型调用
- 本地工具
  - `file_read`
  - `file_list`
  - `file_write`
  - `file_delete`
  - `shell`
- 单 `Runner` + 内部 `plan step` / `execute step` 分层

## 运行

先复制一份本地配置：

```bash
cp config.example.json happyagent.local.json
```

然后把 `happyagent.local.json` 里的 `model` 和 `api_key` 改成你自己的配置。这个文件已经加入 `.gitignore`，不会被提交。

直接检查初始化：

```bash
go run ./cmd/happyagent
```

运行一个真实请求：

```bash
go run ./cmd/happyagent -prompt "say hello in one sentence"
```

使用配置文件：

```bash
go run ./cmd/happyagent -config happyagent.local.json -prompt "list files"
```

## 配置

当前最小实现支持 JSON 配置文件，模板见 [config.example.json](/Users/r1son/Desktop/projects/happyagent/config.example.json:1)，字段如下：

```json
{
  "llm": {
    "model": "gpt-4o-mini",
    "api_key": "your-api-key",
    "base_url": ""
  },
  "engine": {
    "loop_max_steps": 8,
    "run_timeout_seconds": 60,
    "system_prompt": "You are a local coding agent."
  },
  "tools": {
    "root_dir": ".",
    "shell_enabled": true,
    "write_enabled": true,
    "delete_enabled": false
  }
}
```

环境变量覆盖：

- `HAPPYAGENT_LLM_MODEL`
- `HAPPYAGENT_LLM_API_KEY`
- `HAPPYAGENT_LLM_BASE_URL`
- `HAPPYAGENT_SYSTEM_PROMPT`
- `HAPPYAGENT_LOOP_MAX_STEPS`
- `HAPPYAGENT_RUN_TIMEOUT_SECONDS`
- `HAPPYAGENT_ROOT_DIR`
- `HAPPYAGENT_SHELL_ENABLED`
- `HAPPYAGENT_WRITE_ENABLED`
- `HAPPYAGENT_DELETE_ENABLED`

## 下一步

- 让 engine 从 prompt JSON action 平滑演进到原生 tool-calling loop
- 增加 MCP 和 Skill 模块
- 补单元测试和集成测试
