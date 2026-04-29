# Career Copilot Product Plan

本文档把 `happyagent` 的下一阶段主线从“继续堆 Agent 平台组件”调整为“做成一个专业、可用、能解决实际问题的 AI Agent 后端求职助手”。

目标不是削弱 runtime 的工程价值，而是给 runtime 找到一个清晰场景，让面试官和用户在 5 分钟内理解：

- 这个 Agent 解决什么问题。
- 它为什么需要 tool calling、profile、trace、eval、RAG。
- 它比普通 LLM prompt 包装更专业在哪里。
- 它如何帮助 AI Agent / LLM 后端求职者提升简历和面试表达。

---

## 1. Product Direction

### 1.1 产品定位

`happyagent` 的主产品场景定位为：

> Career Copilot for AI Agent Backend Engineers

它面向准备申请 AI Agent / LLM Platform / RAG / 后端开发岗位的人，输入目标 JD、个人简历草稿和项目仓库，输出一份有证据、有结构、可直接用于求职材料优化的分析报告。

### 1.2 用户问题

目标用户通常有这些问题：

- 不知道目标 JD 真正看重哪些能力。
- 不知道自己的项目和 JD 匹配在哪里。
- 简历 bullet 写得像功能清单，缺少工程价值。
- 面试时讲项目容易散，讲不出架构、权衡、验证和结果。
- 不知道下一步该补什么项目能力最能提高岗位匹配度。

### 1.3 产品承诺

Career Copilot 必须做到：

- 基于用户提供的 JD、简历和仓库文件分析，不编造经历。
- 每个项目判断都尽量给出仓库证据路径。
- 输出能直接服务求职动作，而不只是泛泛建议。
- 让用户知道“现在该怎么改简历、怎么讲项目、下一步补什么”。

---

## 2. Target Workflow

### 2.1 标准输入

用户准备三个输入：

```text
inputs/
  jd.md
  resume.md
  target.md
```

其中：

- `jd.md`：目标岗位 JD。
- `resume.md`：当前简历草稿。
- `target.md`：用户目标说明，例如“AI Agent 后端开发，偏 Go / Agent Runtime / MCP / RAG”。

项目仓库默认就是当前工作目录，也允许通过参数传入：

```bash
./bin/happyagent career analyze \
  --jd inputs/jd.md \
  --resume inputs/resume.md \
  --target inputs/target.md \
  --repo . \
  --out outputs/career-report.md
```

### 2.2 标准输出

输出报告必须包含：

1. JD 能力拆解
2. 用户当前简历匹配度
3. 仓库项目证据
4. 推荐简历 bullet
5. 面试讲述稿
6. 项目缺口和改进优先级
7. 风险提示：哪些内容不能写、哪些表述需要证据

### 2.3 用户体验目标

用户跑完命令后，应得到：

- 一份 Markdown 报告。
- 一份 JSON 结构化结果，便于 eval 和后续 UI 使用。
- 一个 run id，可查看 trace。
- 一个简短终端摘要，告诉用户报告路径、匹配度、最重要的三个建议。

---

## 3. CLI Product Shape

### 3.1 命令设计

新增面向用户的子命令：

```bash
happyagent career analyze
happyagent career rewrite-resume
happyagent career interview-brief
happyagent career gap-plan
```

推荐优先完成 `career analyze`，但它不是半成品入口；它必须覆盖完整报告生成闭环。

### 3.2 `career analyze`

命令：

```bash
happyagent career analyze \
  --jd inputs/jd.md \
  --resume inputs/resume.md \
  --target inputs/target.md \
  --repo . \
  --out outputs/career-report.md \
  --json outputs/career-report.json \
  --trace-json logs/career/latest-trace.json
```

行为：

1. 校验输入文件存在。
2. 使用 `career-copilot` profile。
3. 读取 JD、简历、目标说明。
4. 搜索并读取仓库中的关键文档和代码证据。
5. 生成结构化 career report。
6. 渲染 Markdown 报告。
7. 保存 JSON、Markdown、trace。
8. 在终端输出简洁摘要。

### 3.3 CLI 输出样式

终端输出示例：

```text
Career Copilot Report

Profile: career-copilot
Run: run-...
Session: session-...

Match: 78/100
Strong signals:
  - Agent runtime and tool orchestration
  - MCP integration and dynamic capability discovery
  - Eval and trace workflow

Top gaps:
  - Product-facing demo flow is still weak
  - RAG is lexical, not vector-based
  - Observability is custom, not OpenTelemetry-based

Report: outputs/career-report.md
Trace: logs/career/latest-trace.json
```

### 3.4 Supporting Commands

这些命令复用 `career analyze` 的结果，让产品更像专业工具：

```bash
happyagent career rewrite-resume --report outputs/career-report.json --out outputs/resume-bullets.md
happyagent career interview-brief --report outputs/career-report.json --out outputs/interview-brief.md
happyagent career gap-plan --report outputs/career-report.json --out outputs/project-gap-plan.md
```

它们不替代主流程，只是把报告拆成不同求职动作。

---

## 4. Output Contract

### 4.1 JSON Schema

当前 `career_report` 只校验字段存在。需要升级为稳定 schema：

```json
{
  "summary": {
    "target_role": "AI Agent Backend Engineer",
    "match_score": 78,
    "verdict": "Strong project fit with productization gaps"
  },
  "jd_analysis": {
    "required_capabilities": [
      {
        "name": "Agent runtime engineering",
        "importance": "high",
        "evidence_needed": "Tool loop, state management, traceability"
      }
    ]
  },
  "project_evidence": [
    {
      "claim": "Implements tool orchestration",
      "evidence": [
        {
          "path": "internal/engine/loop.go",
          "reason": "Executes model actions and tool calls"
        }
      ],
      "confidence": "high"
    }
  ],
  "resume_rewrite": {
    "bullets": [
      {
        "original": "...",
        "recommended": "...",
        "why": "Connects implementation to agent platform value"
      }
    ]
  },
  "interview_brief": {
    "project_pitch": "...",
    "architecture_talk_track": "...",
    "tradeoffs": ["..."],
    "questions_to_expect": ["..."]
  },
  "gap_plan": [
    {
      "priority": "P0",
      "item": "Make product demo flow reliable",
      "why_it_matters": "Converts runtime into usable product",
      "acceptance": "career analyze produces report, JSON, trace from fixed inputs"
    }
  ],
  "risk_flags": [
    {
      "statement": "Do not claim production-scale deployment",
      "reason": "No server deployment or real traffic evidence exists"
    }
  ]
}
```

### 4.2 Markdown Report Template

新增模板：

```text
docs/templates/career-report.md.tmpl
```

报告结构：

```md
# Career Copilot Report

## Executive Summary

## JD Capability Breakdown

## Project Evidence

## Resume Rewrite

## Interview Brief

## Gap Plan

## Risk Flags

## Appendix: Trace And Files Reviewed
```

模板渲染从 JSON 结果生成，避免 LLM 每次自由发挥导致格式漂移。

---

## 5. Documentation Cleanup

### 5.1 README 重写目标

README 必须从“功能清单”改成“产品入口 + 架构背书”。

推荐结构：

1. What is happyagent?
2. Why Career Copilot?
3. Quick Start
4. Run Career Analysis
5. Example Output
6. Architecture Overview
7. Core Capabilities
8. Eval And Trace
9. Roadmap

### 5.2 docs 调整

保留并重写：

- `docs/architecture.md`
  - 讲 runtime 如何支撑 Career Copilot。
  - 加执行链路图。
- `docs/demo.md`
  - 主 demo 改成 `career analyze`。
  - runtime/MCP demo 降为附录。
- `docs/eval.md`
  - 增加 career-specific eval。
- `docs/career-copilot-product-plan.md`
  - 本文作为当前主线实施计划。
- `docs/agent-platform-upgrade-plan.md`
  - 改为平台能力演进备忘，不作为当前主线。

新增：

- `docs/career-report-format.md`
  - 解释 JSON schema 和 Markdown report。
- `docs/interview-story.md`
  - 给面试讲项目用，包含 2 分钟版、5 分钟版、技术深挖版。

### 5.3 文档验收标准

- README 5 分钟内能让新人知道怎么跑主 demo。
- `docs/demo.md` 中的主命令可复制执行。
- `docs/architecture.md` 能解释为什么这个项目不是普通 prompt wrapper。
- `docs/interview-story.md` 能直接服务面试表达。

---

## 6. Implementation Plan

### 6.1 阶段 A：项目叙事和文档整理

完整交付：

- README 重写为 Career Copilot 主线。
- `docs/demo.md` 改成产品 demo。
- `docs/architecture.md` 补 Career Copilot 执行链路。
- 新增 `docs/career-report-format.md`。
- 新增 `docs/interview-story.md`。
- 调整 `docs/agent-platform-upgrade-plan.md` 为平台演进备忘。

验收：

- 文档中所有命令和路径真实存在。
- 没有互相冲突的项目定位。
- `rg -n "第一版|后续再|先留|fallback|skipped|以后再" docs README.md AGENT.md` 不出现半成品口径。

### 6.2 阶段 B：Career CLI 闭环

完整交付：

- 新增 `career analyze` 子命令。
- 支持 `--jd`、`--resume`、`--target`、`--repo`、`--out`、`--json`、`--trace-json`。
- 输入校验清晰。
- 输出 Markdown、JSON、trace。
- 终端摘要可读。

建议代码改动：

```text
cmd/happyagent/main.go
internal/career/
  command.go
  inputs.go
  report.go
  render.go
  schema.go
```

验收：

```bash
make test
make build
./bin/happyagent career analyze \
  --jd examples/career/jd-ai-agent-backend.md \
  --resume examples/career/resume-draft.md \
  --target examples/career/target.md \
  --repo . \
  --out outputs/career-report.md \
  --json outputs/career-report.json \
  --trace-json logs/career/latest-trace.json
```

产物必须存在且非空：

- `outputs/career-report.md`
- `outputs/career-report.json`
- `logs/career/latest-trace.json`

### 6.3 阶段 C：Report Contract 和渲染

完整交付：

- 扩展 `validator.ValidateOutput("career_report")`。
- 定义 Go struct 表达 career report。
- LLM 输出 JSON。
- 程序校验 JSON。
- 程序渲染 Markdown。
- 报告附带 files reviewed 和 evidence 列表。

建议代码改动：

```text
internal/validator/validator.go
internal/career/report.go
internal/career/render.go
docs/templates/career-report.md.tmpl
```

验收：

- 缺字段会失败并要求模型修正。
- 非 JSON 输出会失败并要求模型修正。
- Markdown 由程序渲染，不依赖模型自由格式。

### 6.4 阶段 D：Career Eval

完整交付：

- 新增 `eval/career_cases.json`。
- 覆盖固定 JD、简历、仓库分析任务。
- 检查输出包含关键字段。
- 检查至少使用 `file_read` / `file_search` / `search_docs` 中的证据工具。
- 检查不能出现夸大表述。

建议测试项：

- 报告必须包含 match score。
- 报告必须包含至少 3 条 project evidence。
- 报告必须包含至少 3 条 resume bullet。
- 报告必须包含 risk flags。

验收：

```bash
make eval-career
```

输出：

- `logs/eval/career-report.json`
- `logs/eval/career-traces/`

### 6.5 阶段 E：CLI 体验打磨

完整交付：

- `happyagent doctor`：检查配置、API key、profile、skills、可写目录。
- `happyagent sessions list`：列出 session。
- `happyagent runs show <run_id>`：显示 run 摘要。
- `happyagent trace show <run_id>`：显示 trace summary。
- 错误信息人类可读，不暴露一长串内部栈式文本作为主要输出。

验收：

```bash
./bin/happyagent doctor
./bin/happyagent sessions list
./bin/happyagent runs show <run_id>
./bin/happyagent trace show <run_id>
```

每个命令都能输出结构化、可读的表格或摘要。

---

## 7. Data And Examples

新增示例目录：

```text
examples/career/
  jd-ai-agent-backend.md
  resume-draft.md
  target.md
  expected-report-outline.md
```

要求：

- 示例 JD 贴近 AI Agent 后端岗位。
- 简历草稿不要虚构过多经历，用当前项目和真实技能表达。
- expected outline 用于说明报告应长什么样，不作为硬编码答案。

---

## 8. Runtime Capabilities Used By Product

Career Copilot 必须真实使用现有 runtime 能力：

| Runtime capability | Product value |
| --- | --- |
| profile | 固定 career-copilot 的角色、工具范围和输出 schema |
| tools | 读取 JD、简历、仓库证据 |
| search_docs | 搜索项目文档，形成 evidence |
| session | 保留用户求职目标和连续修改上下文 |
| trace | 解释报告如何产生，便于 debug 和面试展示 |
| eval | 防止报告格式和质量退化 |
| policy | 避免默认开放危险工具 |

这样项目叙事会更顺：

> 我不是为了炫技做 runtime，而是为了让 Career Copilot 能可靠地读取证据、执行工具、保留上下文、验证输出、追踪决策过程。

---

## 9. Acceptance Checklist

完成本主线后，必须满足：

- [ ] `make test` 通过。
- [ ] `make build` 通过。
- [ ] `make eval-career` 通过。
- [ ] README 的主 demo 是 Career Copilot。
- [ ] `career analyze` 能生成 Markdown、JSON、trace。
- [ ] 报告包含 JD 拆解、项目证据、简历 bullet、面试讲述稿、gap plan、risk flags。
- [ ] 报告中的能力判断能追溯到文件路径或用户输入。
- [ ] CLI 输出清晰，不需要读源码才能知道发生了什么。
- [ ] docs 中没有把平台能力作为当前主线喧宾夺主。
- [ ] 面试讲述文档能解释产品场景、架构设计、验证方式和下一步演进。

---

## 10. Resume Positioning

完成后，简历可以写：

```text
happyagent Career Copilot: Built a Go-based AI Agent application that analyzes job descriptions, resumes, and project repositories to generate evidence-backed resume rewrites, interview briefs, and project gap plans.
```

技术 bullet：

```text
- Designed a profile-driven agent runtime with tool calling, session persistence, structured output validation, trace export, and eval workflows.
- Built a Career Copilot workflow that grounds JD/resume analysis in repository evidence and renders validated JSON into user-facing Markdown reports.
- Implemented CLI product flows for career analysis, trace inspection, session history, and reproducible demo/eval runs.
```

面试讲述核心：

```text
这个项目最开始是 Agent Runtime。后来我把它产品化成 Career Copilot，因为 Agent 平台能力只有落到真实任务上才有说服力。它会读取 JD、简历和项目仓库，用工具收集证据，输出结构化报告，并用 trace/eval 保证可调试和可回归。
```

