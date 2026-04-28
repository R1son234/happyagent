# Eval Guide

## Scope

仓库内置了一套最小 smoke eval，用于两类场景：

1. 回归检查工具链和执行闭环是否仍然可用。
2. 为面试展示提供固定任务集、trace 产物和量化指标。

## Suite

默认任务集位于 `eval/smoke_cases.json`，当前覆盖：

- `skill-summary`
- `search-read-readme`
- `repo-list-summary`

每个 case 都包含：

- 固定 prompt
- 可选 profile
- 超时时间
- 期望输出关键字
- 必须经过的工具
- 最大步数

## Run

先准备本地配置并构建评测程序：

```bash
cp config.example.json happyagent.local.json
make build-eval
```

运行 smoke suite：

```bash
./bin/happyagent-eval \
  -cases eval/smoke_cases.json \
  -output logs/eval/smoke-report.json \
  -trace-dir logs/eval/smoke-traces
```

也可以直接使用：

```bash
make eval-smoke
```

运行 profile-aware suite：

```bash
make eval-profiles
```

## Outputs

评测会产生两类结果：

1. suite 级汇总报告
   - `passed_count`
   - `failed_count`
   - `success_rate`
   - `average_steps`
   - `average_tool_calls`
   - `average_duration_millis`
   - token 汇总
   - `error_categories`

2. case 级 trace
   - profile
   - 输入 prompt
   - 最终输出
   - 每一步 actions
   - observation
   - 单步 planning / execution 耗时
   - token usage
   - tool call 统计
   - 错误分类

## Single Run Trace

单次运行可通过 `--trace-json` 导出结构化 trace：

```bash
./bin/happyagent --trace-json logs/demo/skill-trace.json "Inspect this repository and summarize it in Chinese."
```

trace 文件可直接用于面试演示或后续分析。
