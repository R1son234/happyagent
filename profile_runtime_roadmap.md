# Profile And Runtime Roadmap

本文档用于承接 `implementation_plan.md` 中第 `19`、`20` 章的落地执行，把后续开发拆成两个阶段：

1. `Phase A`：先建立 `Profile` 骨架
2. `Phase B`：继续补强 `Runtime` 能力

整体目标不是先做一个写死的 `Career Copilot`，而是先把 `happyagent` 变成：

- 底层：可复用的 Agent Runtime
- 上层：可配置的 Agent Profile 宿主
- 首个重点场景：`career-copilot`

---

## 1. 总体执行原则

- 先定结构边界，再堆能力。
- `Profile` 先做最小闭环，不在第一阶段引入过多业务逻辑。
- `Runtime` 增强阶段优先补工程化闭环，而不是先做大量场景功能。
- 每个阶段都必须有可运行结果、可演示入口和明确验收标准。
- 所有新增能力优先复用现有 `runtime / engine / tools / mcp / skills / eval` 分层。

---

## 2. Phase A：Profile 骨架

### 2.1 阶段目标

本阶段目标是让 `happyagent` 从“只有一个默认 agent”升级为“可承载多个 agent profile 的 runtime”。

完成后，项目至少应支持：

- 定义多个 profile
- 通过 CLI 选择 profile
- 不同 profile 使用不同的 `system prompt`
- 不同 profile 使用不同的 `tools` 子集
- 不同 profile 使用不同的 `skills` 子集

这一阶段不要求：

- 完整 Memory 策略落地
- 完整 RAG 落地
- 专用业务 tool 全部落地
- Session 模式宿主与可选服务化适配

### 2.2 目标交付物

- 新增 `internal/profile` 模块
- 新增 `profiles/general-assistant/profile.json`
- 新增 `profiles/career-copilot/profile.json`
- CLI 支持 `--profile`
- Runtime 支持按 profile 过滤 prompt / tools / skills
- 至少一组 profile 相关测试

### 2.3 建议目录结构

```text
internal/
  profile/
    types.go
    loader.go
    validator.go

profiles/
  general-assistant/
    profile.json
  career-copilot/
    profile.json
```

如后续需要，也可为 profile 增加补充说明文件：

```text
profiles/
  career-copilot/
    profile.json
    README.md
```

### 2.4 任务拆分

#### A1. 设计 Profile 数据结构

目标：

- 明确 profile 的最小字段集合
- 保证后续扩展 `memory_strategy / output_schema / eval_suite` 时不需要重构

建议字段：

- `name`
- `description`
- `system_prompt`
- `enabled_tools`
- `enabled_skills`
- `memory_strategy`
- `output_schema`
- `eval_suite`

第一版必须真正生效的字段：

- `name`
- `system_prompt`
- `enabled_tools`
- `enabled_skills`

其余字段第一版可先只完成解析和保留。

建议任务：

- 在 `internal/profile/types.go` 定义 `Profile`
- 为未来运行时使用定义 `ResolvedProfile`
- 预留 profile 校验错误类型

验收标准：

- 代码层有统一 profile struct
- 后续 runtime 可以直接消费该 struct

#### A2. 实现 Profile Loader

目标：

- 从 `profiles/` 目录扫描并加载 profile
- 支持按名称查找指定 profile

建议任务：

- 在 `internal/profile/loader.go` 中实现：
- `LoadAll(dir string) ([]Profile, error)`
- `LoadByName(dir, name string) (Profile, error)`
- 读取 `profile.json`
- 处理目录不存在、文件不存在、JSON 非法等错误

验收标准：

- 能加载 `general-assistant`
- 能加载 `career-copilot`
- profile 不存在时错误信息明确

#### A3. 实现 Profile 校验

目标：

- 防止错误配置直接进入运行时

建议校验项：

- `name` 不能为空
- `system_prompt` 不能为空
- `enabled_tools` 不允许重复
- `enabled_skills` 不允许重复
- profile 目录名与 `name` 建议一致

第二阶段再做的校验：

- `enabled_tools` 是否真的都已注册
- `enabled_skills` 是否都存在

建议任务：

- 在 `internal/profile/validator.go` 实现基础校验
- 定义清晰错误信息

验收标准：

- 非法 profile 配置会在加载阶段失败

#### A4. 创建两个最小 Profile

目标：

- 明确“通用 profile”和“求职场景 profile”的边界

`general-assistant` 建议：

- 面向通用仓库/文件任务
- tool 范围较广
- 不绑定求职场景语义

`career-copilot` 建议：

- 面向简历、JD、项目表述优化
- 默认只开放必要 tools
- prompt 强调基于证据分析，不夸大经历

建议任务：

- 创建 `profiles/general-assistant/profile.json`
- 创建 `profiles/career-copilot/profile.json`

验收标准：

- 两个 profile 都能被加载
- 两个 profile 的 prompt 和 tool 集明显不同

#### A5. Runtime 接入 Profile

目标：

- 运行时能根据 profile 构造本次执行上下文

建议任务：

- 为 `runtime.Builder` 或 `runtime.Run` 增加 profile 输入
- 在运行前加载 profile
- 使用 profile 的 `system_prompt`
- 使用 profile 过滤可见 tools
- 使用 profile 过滤可激活 skills

注意点：

- 过滤应该作用于当前 run，而不是全局破坏 registry
- 尽量在运行时构造“本次执行可见工具集”

验收标准：

- 使用 `general-assistant` 时能看到通用工具集
- 使用 `career-copilot` 时只能看到允许的 tools/skills

#### A6. CLI 增加 `--profile`

目标：

- 让 profile 选择能力可直接演示

建议任务：

- 在 `cmd/happyagent/main.go` 增加 `--profile`
- 默认值设为 `general-assistant`
- profile 不存在时直接失败并给出清晰错误

建议演示命令：

```bash
./bin/happyagent --profile general-assistant "summarize this repository"
./bin/happyagent --profile career-copilot "analyze my resume against this JD"
```

验收标准：

- 不同 profile 可被命令行切换
- 默认 profile 行为稳定

#### A7. 测试与最小 Demo

目标：

- 确保 profile 不是只有配置，没有验证

建议任务：

- 为 `internal/profile` 增加单测：
- 加载成功
- 配置缺失失败
- 重复 tool / skill 失败
- 为 runtime 增加一组 profile 过滤测试

最小 demo：

- `general-assistant`：总结仓库 README
- `career-copilot`：分析 `resume` 与 `ai.txt` 的匹配方向

验收标准：

- `make check` 通过
- 至少一个 case 能体现 profile 差异

### 2.5 Phase A 里程碑

#### Milestone A1

- 定义 `Profile` struct
- loader 可加载 profile.json

#### Milestone A2

- 两个 profile 文件可用
- CLI 支持 `--profile`

#### Milestone A3

- runtime 可按 profile 过滤 prompt / tools / skills
- 有基础测试和 demo

### 2.6 Phase A 最低交付标准

- 项目存在独立 `internal/profile` 模块
- CLI 可通过 `--profile` 切换 agent 身份
- `general-assistant` 和 `career-copilot` 均可运行
- profile 已对 tools 和 prompt 生效
- 有至少一组测试覆盖 profile 加载和过滤逻辑

---

## 3. Phase B：Runtime 增强

### 3.1 阶段目标

在 Phase A 已建立 profile 骨架的前提下，继续把 Runtime 补成“更像一个成熟 AI 工程项目”的形态。

本阶段主线对应 `implementation_plan.md` 第 `19` 章，重点包括：

- 工程化硬度
- AI 核心能力闭环
- 评测与结果证明

### 3.2 建议执行顺序

建议顺序如下：

1. session 宿主与 trace 持久化
2. metrics / logging / 错误分类
3. policy / approval
4. memory
5. RAG
6. eval 扩展与对比实验
7. Career Copilot 专项增强

### 3.3 任务拆分

#### B1. Run 持久化与 Trace 存储

目标：

- 每次运行都具备稳定 `run_id`
- trace 不只在内存里，而是可回查、可复用、可展示

建议任务：

- 新增 `internal/store`
- 设计 `RunRecord`、`StepRecord`、`TraceRecord`
- 落地本地持久化方案：
- 第一版建议 `SQLite` 或本地 JSON 文件
- 记录：
- `run_id`
- `profile`
- 输入内容
- 输出内容
- steps
- token usage
- 终止原因
- 错误分类

验收标准：

- 一次 run 结束后可按 `run_id` 查回完整记录

#### B2. Session 宿主与应用层

目标：

- 让 runtime 从单次 CLI 执行演进为可持续多轮对话的 session 宿主
- 先沉淀稳定的 application 层边界，而不是一开始就绑定 HTTP 形态

建议任务：

- 新增 `internal/app`
- 设计核心应用对象：
- `Session`
- `Run`
- `Trace`
- 定义最小应用层接口：
- `CreateSession`
- `AppendUserTurn`
- `GetSession`
- `GetRun`
- `GetTrace`
- `ReplayRun`
- CLI 改为基于 session 驱动：
- 首次输入创建 `session`
- 后续输入在同一 `session` 中追加 turn
- 每次 turn 内部生成稳定 `run_id`
- 如后续需要，再增加 `cmd/happyagentd` 作为可选 HTTP 适配层

验收标准：

- CLI 可以进入持续 session 模式并完成多轮对话
- 每次 turn 都可按 `run_id` 查询 trace
- session 与 run 的边界在存储和运行时中清晰可见

#### B3. Metrics、Logging、错误分类

目标：

- 增强系统可观测性

建议任务：

- 新增 `internal/observe`
- 增加结构化日志
- 增加 Prometheus metrics
- 统一错误类型：
- model
- tool validation
- tool execution
- timeout
- policy denial
- mcp transport

验收标准：

- 至少能看见 run 成功率、耗时、tool 调用数
- trace 中有明确失败原因

#### B4. Policy / Approval

目标：

- 建立高风险能力的控制边界

建议任务：

- 新增 `internal/policy`
- 为 tool 增加 risk level
- 支持：
- `allow`
- `deny`
- `require_approval`
- 首批接入工具：
- `shell`
- `file_write`
- `file_patch`
- `file_delete`

验收标准：

- 危险工具在未授权时不会直接执行
- trace 中能看到拒绝原因

#### B5. Memory

目标：

- 补齐多轮任务场景下的上下文管理能力

建议任务：

- 新增 `internal/memory`
- 先做短期 session memory
- 再做 summary memory
- 再做上下文压缩
- 支持按 profile 选择记忆策略

验收标准：

- 多轮任务场景下历史信息可复用
- 长上下文场景不会无限膨胀

#### B6. RAG

目标：

- 补齐文档检索增强能力

建议任务：

- 新增 `internal/rag`
- 支持本地文档切分和索引
- 支持 query -> retrieve -> inject
- 支持来源引用

第一版建议：

- 先做轻量实现
- 先服务 Career Copilot 的 JD / 简历 / 项目文档分析

验收标准：

- 至少一个文档问答场景可给出带引用回答

#### B7. Eval 扩展与对比实验

目标：

- 让项目具备量化结果，而不只是功能说明

建议任务：

- 扩展 `eval/smoke_cases.json`
- 新增 profile 维度的 case
- 为 `career-copilot` 增加专用 case
- 输出：
- 成功率
- 平均步数
- 平均时延
- token 消耗
- 失败原因分布
- 设计对比实验：
- profile 差异
- memory 开关
- RAG 开关

验收标准：

- 同一套 case 可重复执行并输出稳定报告

#### B8. Career Copilot 专项增强

目标：

- 在 runtime 完整度提升后，把 `career-copilot` 做成强场景 demo

建议任务：

- 为 `career-copilot` 增加结构化输出 schema
- 增加：
- `match_report`
- `rewrite_plan`
- `project_gap_analysis`
- 如有必要，再增加专用 tools：
- `jd_extract`
- `resume_rewrite`
- `evidence_linker`

注意：

- 专用业务能力应建立在 profile 层和 runtime 层之上
- 避免直接把这些逻辑写死到 runtime 主流程

验收标准：

- 可稳定演示“读取 JD + 读取简历/项目 + 输出结构化建议”的完整闭环

### 3.4 Phase B 里程碑

#### Milestone B1

- `session_id`
- `run_id`
- trace 持久化
- `internal/app`

#### Milestone B2

- metrics
- logging
- 错误分类
- policy

#### Milestone B3

- memory
- RAG
- 更完整的上下文治理

#### Milestone B4

- eval 扩展
- Career Copilot 场景报告
- README / demo 材料完善

### 3.5 Phase B 最低交付标准

- runtime 具备持久化 trace 和 session 模式宿主能力
- 至少一类高风险工具具备审批或拒绝策略
- 至少一种 memory 策略真正生效
- 至少一个 RAG 场景可运行
- 至少一套可复现 eval case 能输出量化指标
- `career-copilot` 可作为固定 demo 稳定展示

---

## 4. 推荐开发节奏

如果按 6 到 8 周节奏推进，建议如下：

### 第 1 周

- 完成 `Profile` struct、loader、validator
- 建立两个 profile 文件

### 第 2 周

- CLI `--profile`
- runtime profile 过滤
- profile 测试与最小 demo

### 第 3 到 4 周

- `session_id`
- `run_id`
- trace 持久化
- `internal/app`

### 第 5 周

- metrics / logging / 错误分类
- policy / approval

### 第 6 周

- memory
- 上下文压缩

### 第 7 周

- RAG
- `career-copilot` 结构化输出

### 第 8 周

- eval 扩展
- 对比实验
- README / demo / 简历素材整理

---

## 5. 当前最值得先做的 5 件事

如果只看下一步，我建议先做：

1. 新增 `internal/profile/types.go`
2. 新增 `internal/profile/loader.go`
3. 新增 `profiles/general-assistant/profile.json`
4. 新增 `profiles/career-copilot/profile.json`
5. 在 CLI 接入 `--profile`

原因：

- 这 5 件事最小，但能把结构先立住
- 做完后，后续 Runtime 增强不会再围绕“唯一默认 agent”写死
- 这也是把项目从原型推进到“通用 runtime + 垂直 profile”路线的真正起点
