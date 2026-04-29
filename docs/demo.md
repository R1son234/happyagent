# Demo Guide

本文件用于两类场景：

1. 本地自测 `happyagent` 是否能稳定跑通。
2. 面试或简历补充材料中，快速展示项目的工程边界和可用能力。

## 准备

1. 复制配置模板：

```bash
cp config.example.json happyagent.local.json
```

2. 填入可用的模型配置：

- `llm.model`
- `llm.api_key`
- `llm.base_url`，如使用兼容网关则填写对应地址

3. 构建主程序：

```bash
make build
```

## Demo 1：Skill 按需激活

该 demo 用于展示两点：

- system prompt 保持简洁，不预注入 skill 正文
- 模型通过 `activate_skill` 的 tool observation 获取 skill 详细说明

执行：

```bash
./bin/happyagent "Inspect this repository. If a skill is useful, call it first, then summarize the project structure in Chinese."
```

观察点：

- run log 里是否先出现 `activate_skill`
- 下一轮请求里是否带上了 skill observation
- 最终回答是否引用了 skill 中的约束

## Demo 2：搜索定位与精确改码

该 demo 用于展示 tool runtime 的稳定性：

- `file_search` 先定位代码片段，再读取局部内容
- `file_read` 支持按行范围读取，并对大文件自动截断
- `file_list` 支持分页查看目录内容
- `file_patch` 支持小范围精确替换，避免整文件重写

执行：

```bash
./bin/happyagent "Search for a short typo or wording issue in README.md, read the relevant lines, patch only the matching snippet, then summarize the change."
```

如需单独验证大文件保护，可让模型读取较大的日志或生成文件，预期结果应包含：

- `[file_read truncated ...]`
- `[file_read showing first ... and last ...]`

## Demo 3：MCP Tool 接入

先构建 demo MCP server：

```bash
mkdir -p bin
GOCACHE=$PWD/.gocache go build -o ./bin/mcpdemo ./cmd/mcpdemo
```

然后在 `happyagent.local.json` 中加入：

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

执行：

```bash
./bin/happyagent "Use list_capabilities to inspect available MCP resources, call mcp_read_resource on one URI with max_bytes 128, then call the demo repeat tool with hello and explain what happened."
```

MCP resource 读取默认也会应用输出边界控制；较大 resource 会返回截断提示，而不是直接把全文塞进 observation。
`list_capabilities` 返回的 MCP resource 列表也会按配置限制数量，避免一次返回过多条目。

## Demo Trace

演示时可同时导出结构化 trace：

```bash
./bin/happyagent --trace-json logs/demo/demo-trace.json "Search README.md for the shell tool notes, read the matching lines, then answer with one Chinese sentence that includes argv."
```

trace 中会包含：

- 总步数
- 总耗时
- attempted / executed / successful tool call 统计
- token usage 汇总
- 每一步 actions、observation 和耗时

## Demo Eval

仓库内置 smoke eval 可直接作为稳定演示入口：

```bash
make eval-smoke
```

默认会输出：

- suite 汇总报告 `logs/eval/smoke-report.json`
- case 级 trace 目录 `logs/eval/smoke-traces/`

## 面试讲法

如需用于 AI 研发岗位介绍，建议重点突出以下三点：

1. 项目实现的是本地 agent runtime，而不仅是 prompt demo，覆盖工具注册、执行循环、MCP 接入和 skill 激活。
2. 高风险能力做了边界控制，包括 root dir 限制、文件读取截断、二进制文件保护和受控命令执行。
3. 能力暴露被设计为 runtime 协议问题，而不是把所有信息预塞进 system prompt，便于观测、调试和扩展。
