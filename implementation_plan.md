# Implementation Plan

## 1. 文档目标

本文档用于指导 `happyagent` 项目的首版实现。目标是按最小可用能力逐步落地一个类似 openclaw 的 Agent Runtime，并严格遵守 [AGENT.md](/Users/r1son/Desktop/projects/happyagent/AGENT.md:1) 中的约束：

- 先规划，再实现。
- 先验证，再交付。
- 保持最小改动面和最少魔法。
- 以清晰接口隔离各能力边界，避免过早耦合。

本文档既描述系统架构设计，也描述首轮代码改动计划。后续开发默认以本文档为基线推进；如果实施过程中发现前提不成立，需要先更新本文档，再调整实现。

## 2. 项目目标与非目标

### 2.1 项目目标

本项目第一阶段目标如下：

1. 实现基础 LLM 调用能力，底层使用 `eino`。
2. 实现本地 Tool 能力和 ReAct 风格的 Loop Engine。
3. 实现 MCP Client 能力，支持 MCP `tool` 和 `resource`。
4. 实现本地 Skill 发现、加载与按需激活能力。

### 2.2 非目标

以下内容不纳入第一版实现范围：

- 多 Agent 协作。
- 分布式任务调度。
- Web UI。
- 长期记忆、向量数据库、RAG 平台集成。
- 动态执行不受控的第三方脚本型 Skill。
- 自定义 MCP Server 实现。
- 与 openclaw 的完全协议兼容。

约束原因：先保证核心链路可运行、可验证，再逐步扩展复杂能力。

## 3. 总体设计原则

### 3.1 设计原则

- 最小闭环优先：先跑通 `User Input -> LLM -> Tool -> Observation -> Final Answer`。
- 接口先行：先定义 `llm`、`tool`、`engine`、`mcp`、`skill` 等抽象接口，再补具体实现。
- 单向依赖：上层依赖下层抽象，下层不反向依赖运行时实现。
- 显式状态：循环步骤、工具调用、资源读取、Skill 激活都必须可观测。
- 安全默认保守：shell/file/mcp/resource 的能力默认受限，不隐式放权。
- 方便验证：每层都提供最小单测点与集成验证入口。

### 3.2 推荐技术栈

- 语言：Go
- LLM 调用：`eino`
- 配置：环境变量 + 本地配置文件
- 日志：优先标准库或轻量日志抽象
- 测试：Go `testing`

选择 Go 的原因：

- 适合做 CLI 和 Agent Runtime。
- 与 `eino` 配套实现更直接。
- 类型系统有利于收敛工具协议和执行状态。

## 4. 总体架构

### 4.1 分层结构

建议采用以下分层：

1. `cmd`
   - 程序入口。
   - 负责装配配置、初始化运行时、启动一次执行。
2. `internal/config`
   - 配置加载、校验、默认值。
3. `internal/llm`
   - 基于 `eino` 的模型封装。
4. `internal/tools`
   - 本地工具定义、参数校验、执行器、注册中心。
5. `internal/engine`
   - ReAct Loop Engine，负责编排模型和工具。
6. `internal/mcp`
   - MCP Client 管理、Tool 映射、Resource 读取。
7. `internal/skills`
   - Skill 扫描、解析、校验、激活输出。
8. `internal/runtime`
   - 组合装配层，把 LLM、Tools、MCP、Skills 组装为一个可运行 Agent。

### 4.2 模块关系

依赖方向建议如下：

`cmd -> runtime -> engine -> llm/tools`

`runtime -> mcp`

`runtime -> skills`

`skills -> tools`

`mcp -> tools`

约束：

- `engine` 不直接依赖 `mcp` 和 `skills` 的具体实现，只依赖统一的工具注册中心和上下文输入。
- `tools` 不依赖 `engine`。
- `llm` 不关心 Tool/MCP/Skill 细节，只处理消息与模型交互。

## 5. 目录规划

建议首轮落盘目录如下：

```text
happyagent/
├── AGENT.md
├── implementation_plan.md
├── README.md
├── go.mod
├── cmd/
│   └── happyagent/
│       └── main.go
├── docs/
│   └── architecture.md
├── examples/
│   ├── config.example.yaml
│   └── skills/
│       └── file-inspector/
│           ├── skill.yaml
│           └── prompt.md
├── internal/
│   ├── config/
│   │   ├── config.go
│   │   └── loader.go
│   ├── llm/
│   │   ├── client.go
│   │   ├── eino_client.go
│   │   └── types.go
│   ├── tools/
│   │   ├── tool.go
│   │   ├── registry.go
│   │   ├── validator.go
│   │   ├── shell_tool.go
│   │   ├── file_read_tool.go
│   │   ├── file_write_tool.go
│   │   ├── file_list_tool.go
│   │   └── file_delete_tool.go
│   ├── engine/
│   │   ├── runner.go
│   │   ├── loop.go
│   │   ├── prompt_builder.go
│   │   ├── action_parser.go
│   │   └── types.go
│   ├── mcp/
│   │   ├── manager.go
│   │   ├── client.go
│   │   ├── tool_adapter.go
│   │   └── resource_provider.go
│   ├── skills/
│   │   ├── loader.go
│   │   ├── parser.go
│   │   ├── injector.go
│   │   └── types.go
│   └── runtime/
│       ├── runtime.go
│       └── builder.go
└── tests/
    └── integration/
```

说明：

- 目录先按职责拆分，不做过早细化。
- 第一版只维护 `internal` 级别 API，不对外承诺稳定公共包。

## 6. 核心运行链路

### 6.1 执行流程

目标链路如下：

1. CLI 读取配置。
2. 初始化 LLM Client。
3. 初始化本地 Tool Registry。
4. 初始化 MCP 并将远程 Tool/Resource 映射进运行时。
5. 读取本地 Skill catalog，并建立本次运行的 skill session。
6. 初始 system prompt 保持简洁；模型如需运行时能力信息，可调用 `list_capabilities`。
7. 模型按需调用 `activate_skill` 后，runtime 加载 skill 正文，并通过 tool observation 返回给模型。
8. Engine 启动 ReAct Loop。
9. LLM 返回下一步动作：
   - 直接回答。
   - 请求调用工具。
10. Engine 执行工具并记录 observation。
11. 继续循环直到：
   - 得到 `final_answer`
   - 遇到不可恢复错误
   - 达到最大步数

### 6.2 最小运行模型

第一版不做独立进程或独立模块级别的 `Planner/Executor` 分离，统一由一个 `Runner` 驱动主循环。

但为了降低后续演进成本，`Runner` 内部设计应显式区分两个阶段职责：

- `plan step`
  - 基于当前消息、工具描述、历史 observation 决定下一步动作。
- `execute step`
  - 执行动作、收集结果、生成 observation、判断是否继续循环。

也就是说，第一版采用“单 Runner 实现，内部逻辑分层”的方案，而不是“两个完全独立组件协作”的方案。

这样做的原因：

- 首版重点是先跑通主链路，而不是先做复杂调度框架。
- 当前 MCP、Tool Schema、Skill 激活链路已经带来足够多的系统复杂度。
- 先在单 Runner 内部固化 `Action`、`Observation`、`StepRecord` 等协议，后续再拆分 `Planner` 和 `Executor` 时改动会更小。

后续如需演进为独立 `Planner/Executor`，优先通过接口抽取完成，而不是推倒现有循环实现。

建议核心方法：

```go
type Runner interface {
    Run(ctx context.Context, input RunInput) (RunResult, error)
}
```

`RunInput` 至少包含：

- 用户输入
- system prompt
- 会话限制项
- 可用工具集合
- 可用资源快照

`RunResult` 至少包含：

- 最终回答
- 执行步数
- 工具调用记录
- 错误信息

## 7. 核心模块设计

### 7.1 LLM 模块

#### 设计目标

- 屏蔽 `eino` 细节，给上层统一的聊天接口。
- 支持最小消息模型。
- 支持超时、重试、基础参数透传。

#### 建议接口

```go
type Client interface {
    Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
}

type ChatRequest struct {
    Messages    []Message
    Temperature *float64
    MaxTokens   *int
    Tools       []ToolSpec
}

type ChatResponse struct {
    Message Message
    Usage   TokenUsage
}
```

#### 改动细节

- `types.go`
  - 定义 `Message`、`ChatRequest`、`ChatResponse`、`TokenUsage`。
- `client.go`
  - 定义抽象接口。
- `eino_client.go`
  - 基于 `eino` 实现 `Client`。
  - 处理模型名、认证、超时、错误包装。

#### 验证要求

- 能发起最小单轮对话。
- 错误信息包含模型配置上下文，但不暴露敏感 token。
- 支持无 Tool 和带 Tool 描述两种请求。

### 7.2 Tools 模块

#### 设计目标

- 提供统一 Tool 抽象。
- 支持本地工具注册和执行。
- 参数校验显式化。

#### 建议接口

```go
type Tool interface {
    Definition() Definition
    Execute(ctx context.Context, call Call) (Result, error)
}
```

`Definition` 建议包含：

- 名称
- 描述
- 参数 Schema
- 是否危险

`Call` 建议包含：

- 工具名
- 参数原始 JSON

`Result` 建议包含：

- 文本输出
- 结构化输出
- 错误类型

#### 首批内置工具

1. `shell`
   - 支持限定工作目录。
   - 支持命令白名单或最小权限策略。
   - 不支持复杂 shell 特性穿透。
2. `file_read`
   - 读取单文件内容。
3. `file_write`
   - 写入或覆盖单文件。
4. `file_list`
   - 列目录。
5. `file_delete`
   - 删除单文件。

#### 改动细节

- `tool.go`
  - 定义 Tool 抽象和通用类型。
- `registry.go`
  - 管理工具注册、查询、执行。
- `validator.go`
  - 负责参数格式验证。
- 各工具文件
  - 各自实现参数结构、执行逻辑和错误包装。

#### 验证要求

- 每个 Tool 至少一组成功用例和一组失败用例。
- `shell` 必须验证工作目录限制。
- 文件工具必须验证路径边界和错误路径。

### 7.3 Engine 模块

#### 设计目标

- 实现最小 ReAct Loop。
- 让模型输出结构化动作，避免脆弱字符串解析。
- 把每一步执行状态显式化，便于后续追踪。
- 在单 `Runner` 内部预留 `Planner/Executor` 职责拆分边界。

#### 推荐循环状态

```text
Prepare -> AskModel -> ParseAction -> ExecuteTool -> AppendObservation -> Stop
```

其中逻辑职责建议按两段组织：

- Planner 职责（首版仍在 `Runner` 内部）
  - 构建 prompt
  - 请求模型
  - 解析 Action
- Executor 职责（首版仍在 `Runner` 内部）
  - 执行 Tool 或 Finalize
  - 生成 Observation
  - 写入 StepRecord
  - 判断是否终止或继续

建议在代码层至少预留以下内部方法或等价结构：

```go
func (r *runner) planStep(ctx context.Context, state LoopState) (Action, error)
func (r *runner) executeStep(ctx context.Context, state LoopState, action Action) (StepResult, error)
```

注意：这里的 `planStep` 和 `executeStep` 只是职责边界，不意味着第一版就要引入独立的 `Planner`、`Executor` 模块或额外抽象层。

#### 动作模型

建议模型输出固定结构：

```json
{
  "type": "tool_call",
  "tool_name": "file_read",
  "arguments": {
    "path": "README.md"
  }
}
```

或：

```json
{
  "type": "final_answer",
  "content": "..."
}
```

#### 改动细节

- `types.go`
  - 定义 `RunInput`、`RunResult`、`StepRecord`、`Action`、`Observation`、`LoopState`。
- `prompt_builder.go`
  - 生成 system prompt、tool 列表、输出格式约束。
- `action_parser.go`
  - 解析模型输出为结构化 Action。
- `loop.go`
  - 实现循环状态推进。
  - 组织 `plan step` 和 `execute step` 两段内部逻辑。
- `runner.go`
  - 对外暴露统一入口。

#### 关键控制项

- `max_steps`
- `max_tool_calls`
- `max_observation_bytes`
- `tool_timeout`

#### 验证要求

- 正常闭环：模型调用工具后输出最终答案。
- 解析失败：能给出明确错误。
- 工具失败：观察信息进入下一轮，而不是直接崩溃。
- 达到最大步数：有明确终止原因。

### 7.4 MCP 模块

#### 设计目标

- 以 Client 方式接入外部 MCP Server。
- 把 MCP Tool 映射为内部 Tool。
- 把 MCP Resource 暴露为可读取上下文源。

#### 首版范围

- 只实现 MCP Client，不实现 MCP Server。
- 先支持静态配置的 MCP Server 列表。
- 先支持同步请求场景。

#### 建议接口

```go
type Manager interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    ListTools(ctx context.Context) ([]tools.Definition, error)
    CallTool(ctx context.Context, name string, args json.RawMessage) (tools.Result, error)
    ReadResource(ctx context.Context, uri string) (ResourceContent, error)
}
```

#### 改动细节

- `client.go`
  - 封装单个 MCP 连接。
- `manager.go`
  - 管理多个 MCP Client。
- `tool_adapter.go`
  - 把 MCP Tool 转成内部 `tools.Tool` 适配器。
- `resource_provider.go`
  - 提供 resource 读取接口。

#### 关键问题

- 同名 Tool 冲突需要处理。
- Resource 读取不能无限制注入上下文。
- MCP 断连需要可感知错误。

#### 验证要求

- 接入一个最小 MCP server demo。
- 能列出远程 Tool。
- 能调用远程 Tool。
- 能读取 Resource 内容。

### 7.5 Skills 模块

#### 设计目标

- 支持从本地目录加载 Skill。
- Skill 通过声明式配置增强 Agent 行为。
- Skill 不直接执行任意代码。

#### Skill 文件建议

每个 Skill 至少包含：

```text
skill.yaml
prompt.md
```

`skill.yaml` 建议字段：

- `name`
- `description`
- `version`
- `tools`
- `resources`
- `input_schema`
- `constraints`

#### Skill 工作方式

1. 扫描 Skill 目录。
2. 解析元数据。
3. 读取 `prompt.md` 或 `SKILL.md` 正文。
4. 校验引用的 Tool/Resource 是否存在。
5. 通过 `list_capabilities` 暴露 skill catalog。
6. 模型调用 `activate_skill` 时，把 skill 正文作为 tool observation 返回。

#### 改动细节

- `types.go`
  - 定义 Skill 结构。
- `parser.go`
  - 解析 `skill.yaml` 和 `prompt.md`。
- `loader.go`
  - 扫描目录、校验 Skill。
- `injector.go`
  - 将 Skill 配置转换为运行时 catalog、激活输出和能力限制。

#### 验证要求

- 能加载一个 demo skill。
- 引用不存在 Tool 时给出清晰错误。
- Skill 激活后 tool observation 明确可见。

### 7.6 Runtime 模块

#### 设计目标

- 作为系统装配层，避免 `main.go` 变成大杂烩。
- 聚合配置、LLM、Tools、MCP、Skills、Engine。

#### 改动细节

- `builder.go`
  - 基于配置构建各模块实例。
- `runtime.go`
  - 提供 `Run` 入口。

#### 验证要求

- 单次运行流程可直接从 runtime 发起。
- 构建失败时错误来源明确。

## 8. 配置设计

### 8.1 配置来源

第一版支持两类配置来源：

1. 环境变量
2. 本地配置文件，例如 `config.yaml`

环境变量优先级高于配置文件。

### 8.2 配置结构建议

```yaml
llm:
  provider: openai
  model: gpt-4o-mini
  api_key_env: OPENAI_API_KEY
  base_url: ""
  timeout_seconds: 60

engine:
  max_steps: 8
  max_tool_calls: 8
  max_observation_bytes: 8192

tools:
  shell:
    enabled: true
    working_dir: .
  file:
    enabled: true
    root_dir: .

mcp:
  servers: []

skills:
  dir: ./examples/skills
  default: ""
```

### 8.3 改动细节

- `config.go`
  - 定义配置结构体。
- `loader.go`
  - 负责读取、合并、校验配置。

### 8.4 验证要求

- 缺少关键配置时错误清晰。
- 环境变量覆盖生效。
- 默认值合理可运行。

## 9. Prompt 设计

### 9.1 基本策略

Engine 的 prompt 分三部分：

1. 固定运行时约束
2. 当前可用 Tool 描述和输出协议
3. Tool 使用规范与输出格式规范

### 9.2 关键要求

- 模型必须按固定 JSON Action 输出。
- 当无需调用工具时输出 `final_answer`。
- 不允许编造未执行过的工具结果。
- Tool 失败时要基于 observation 继续推理。

### 9.3 风险控制

- 强制结构化输出，减少自由文本解析错误。
- Tool 描述中明确参数和边界。
- 对 observation 内容做截断，避免上下文无限膨胀。

## 10. 安全设计

### 10.1 Shell Tool

- 默认关闭高风险命令。
- 限制工作目录。
- 限制执行时间。
- 输出大小限制。

### 10.2 File Tools

- 仅允许访问配置指定根目录下的路径。
- 路径必须做归一化和越界校验。
- 删除操作必须显式启用。

### 10.3 MCP

- 仅连接显式配置的 server。
- Resource 注入前做大小限制。
- Tool 调用错误不应泄露敏感连接信息。

### 10.4 日志

- 不记录 API Key。
- 不直接记录完整敏感资源内容。
- 记录工具调用摘要和错误上下文。

## 11. 实施阶段

### 阶段 0：仓库初始化与架构落盘

目标：

- 建立 Go 项目骨架。
- 建立目录结构和核心接口。
- 补充 README 与架构文档。

改动：

- 新增 `go.mod`
- 新增 `cmd/happyagent/main.go`
- 新增 `internal/...` 基础目录和空实现
- 新增 `README.md`
- 新增 `docs/architecture.md`

验证：

- `go build ./...`

### 阶段 1：LLM 最小实现

目标：

- 跑通最小单轮模型调用。

改动：

- 实现 `internal/llm`
- 实现 `internal/config` 中的 LLM 配置

验证：

- 最小集成调用通过

### 阶段 2：本地 Tools 和 Registry

目标：

- 本地 Tool 可注册、查询、执行。

改动：

- 实现 `internal/tools`
- 添加 shell 和 file 系列工具

验证：

- Tool 单测通过

### 阶段 3：Loop Engine

目标：

- 跑通 ReAct 闭环。

改动：

- 实现 `internal/engine`
- 打通 `llm + tools`

验证：

- demo task 能完成工具调用闭环

### 阶段 4：MCP 集成

目标：

- 接入 MCP Tool 和 Resource。

改动：

- 实现 `internal/mcp`
- 在 runtime 中装配 MCP 适配层

验证：

- MCP demo server 联调成功

### 阶段 5：Skill 集成

目标：

- Skill 可加载、可激活、可限制能力范围。

改动：

- 实现 `internal/skills`
- 添加 demo skill

验证：

- 指定 skill 后 Agent 行为可观察变化

### 阶段 6：文档和样例补全

目标：

- 提供最小可运行示例。

改动：

- 补 README
- 补配置样例
- 补运行命令

验证：

- 新用户可按文档跑通 demo

## 12. 测试与验证策略

### 12.1 单元测试优先级

- `config`
  - 配置解析与覆盖
- `tools`
  - 参数校验、路径校验、shell 边界
- `engine`
  - action 解析、循环停止条件
- `skills`
  - skill 解析与引用校验
- `mcp`
  - tool/resource 适配逻辑

### 12.2 集成测试优先级

1. `LLM -> final_answer`
2. `LLM -> tool_call -> final_answer`
3. `LLM -> MCP tool -> final_answer`
4. `Skill + Tool + final_answer`

### 12.3 最低交付标准

- 项目可编译。
- 至少一个 demo task 可跑通。
- 有最小配置样例和运行文档。
- 关键错误路径有说明或测试覆盖。

## 13. 风险与回退方案

### 13.1 主要风险

- `eino` 接入细节可能与预期不一致。
- MCP 协议实现复杂度高于首版预估。
- Tool 输出格式如果不稳定，会影响 ReAct 解析。
- Skill 设计如果过早泛化，容易引入复杂耦合。

### 13.2 风险控制策略

- 每层都先做最小接口和最小实现。
- MCP 先只支持最小可验证能力。
- Skill 先只做声明式 prompt 暴露 + capability 控制。
- 尽量用结构化输出而不是自由文本协议。

### 13.3 回退方式

- MCP 未稳定前，可先只保留本地 Tool 运行链路。
- Skill 未稳定前，可退化为固定 system prompt。
- 结构化 Tool Call 不稳定时，可先收紧 prompt 和 parser，而不是扩展更多能力。

## 14. 首轮改动清单

后续编码时，优先按以下顺序创建文件：

1. `go.mod`
2. `cmd/happyagent/main.go`
3. `internal/config/config.go`
4. `internal/config/loader.go`
5. `internal/llm/types.go`
6. `internal/llm/client.go`
7. `internal/llm/eino_client.go`
8. `internal/tools/tool.go`
9. `internal/tools/registry.go`
10. `internal/tools/file_read_tool.go`
11. `internal/tools/file_write_tool.go`
12. `internal/tools/file_list_tool.go`
13. `internal/tools/file_delete_tool.go`
14. `internal/tools/shell_tool.go`
15. `internal/engine/types.go`
16. `internal/engine/prompt_builder.go`
17. `internal/engine/action_parser.go`
18. `internal/engine/loop.go`
19. `internal/engine/runner.go`
20. `internal/runtime/builder.go`
21. `internal/runtime/runtime.go`
22. `README.md`
23. `docs/architecture.md`

MCP 和 Skill 文件建议在本地主链路稳定后再创建，避免空壳代码过早扩散。

## 15. 下一步执行建议

建议按以下顺序进入编码：

1. 先完成阶段 0，建项目骨架和接口。
2. 立刻验证 `go build ./...`。
3. 再实现 LLM 最小调用。
4. 然后补本地 Tools 和 Loop Engine。
5. 主链路稳定后再引入 MCP 和 Skill。

这样可以始终保持系统处于“可编译、可验证、可回退”的状态，符合 `AGENT.md` 的工作方式。

## 16. 第二阶段执行计划：补齐成熟 Agent 的关键能力

本节用于指导 `happyagent` 从“可运行原型”演进到“可用于简历展示、可稳定演示、具备工程可信度”的第二阶段实现。目标不是盲目扩功能，而是围绕 coding agent 的核心闭环，优先补齐五类最关键能力：

1. 精确改码能力
2. 搜索与定位能力
3. 上下文压缩与分页能力
4. 权限与风险控制
5. Eval、Trace 与 Demo 闭环

第二阶段默认遵守以下约束：

- 不引入 Web UI、多 Agent、RAG 或长期记忆。
- 不为了“未来平台化”提前引入过度抽象。
- 每项能力都必须配套测试、文档和最小 demo。
- 每一阶段结束后都保持 `go test` 和 demo 路径可验证。

### 16.1 能力一：精确改码能力

#### 目标

- 让 Agent 从“整文件覆盖”升级为“定位后小范围改动”。
- 降低误改概率，提高对真实代码库任务的适配度。
- 让修改结果天然具备 diff 和审查语义。

#### 首轮范围

- 新增 `file_patch` 或 `text_edit` 工具。
- 支持按精确片段替换，最少支持：
  - 指定文件
  - 查找文本
  - 替换文本
  - 预期替换次数
- 可选支持统一 diff patch，但不要求首轮就支持完整 patch grammar。
- 工具返回结果中包含变更摘要，例如命中次数、写入文件、是否创建备份。

#### 改动建议

- `internal/tools/file_patch_tool.go`
  - 实现最小精确替换能力。
- `internal/tools/tool.go`
  - 如有必要，为 `Result` 增加结构化字段，例如 `Meta` 或 `ChangedFiles`。
- `internal/tools/file_write_tool.go`
  - 保留整文件写入能力，但在文档中降级为次优路径。
- `README.md` / `docs/demo.md`
  - 增加精确改码示例。

#### 风险点

- 替换文本不唯一时容易误改。
- 统一 diff 首轮直接上会明显增加复杂度和 parser 风险。

#### 验证要求

- 成功用例：单文件单片段替换。
- 成功用例：多次匹配但指定 `expected_replacements=1` 时明确失败。
- 失败用例：目标文本不存在。
- 失败用例：目标路径越界。

#### 交付标准

- Agent 可以在不重写整个文件的情况下完成常见小改动任务。
- 工具输出足够清晰，便于模型在下一轮基于 observation 继续行动。

### 16.2 能力二：搜索与定位能力

#### 目标

- 让 Agent 能先定位再读取，而不是盲读整个仓库。
- 降低无效上下文消耗，提高代码任务命中率。

#### 首轮范围

- 新增 `file_search` 工具，优先使用 `rg`。
- 支持：
  - 文本模式搜索
  - 文件名过滤
  - 返回行号
  - 返回最大命中数
- 视复杂度补一个 `file_read_range` 或增强 `file_read` 支持行号区间读取。

#### 改动建议

- `internal/tools/file_search_tool.go`
  - 调用 `rg`，统一结果格式。
- `internal/tools/file_read_tool.go`
  - 支持可选 `start_line` / `end_line`，或单独实现 `file_read_range_tool.go`。
- `internal/runtime/builder.go`
  - 默认注册搜索工具。

#### 风险点

- 命中过多时 observation 过长。
- `rg` 在某些环境中缺失，需要降级到 `grep` 或给出明确错误。

#### 验证要求

- 成功用例：按关键字命中文件和行号。
- 成功用例：限制最大返回条数。
- 失败用例：搜索根目录越界。
- 失败用例：命中为空时返回稳定结果，而不是工具报错。

#### 交付标准

- Agent 能以“search -> read range -> patch”的路径完成基础代码任务。
- README 和 demo 中体现这条推荐工作流。

### 16.3 能力三：上下文压缩与分页能力

#### 目标

- 控制 observation 体积，避免大文件、大目录和长运行历史拖垮模型上下文。
- 让 Agent 在真实仓库上运行时保持稳定成本和可预期表现。

#### 首轮范围

- 为 `file_read`、`file_list`、MCP resource 读取加入统一的输出上限。
- 引入目录结果和大文件结果的分页/摘要语义。
- 在 engine 层加入 observation 截断和历史压缩策略。

#### 改动建议

- `internal/tools/file_read_tool.go`
  - 补齐行级分页或 offset/limit 读取能力。
- `internal/tools/file_list_tool.go`
  - 支持 `limit`、`offset` 或目录条目摘要。
- `internal/engine/loop.go`
  - 在写入 observation 前做统一字节上限处理。
- `internal/engine/types.go`
  - 为 `RunInput` 增加 `MaxObservationBytes` 等控制项。
- `internal/config/config.go`
  - 暴露上下文限制配置。

#### 风险点

- 压缩过度会让模型拿不到足够信息。
- 不同 Tool 各自截断会导致行为不一致。

#### 验证要求

- 大文件读取时能返回稳定的头尾预览。
- 大目录 listing 时不会一次返回全部内容。
- 多轮运行时 observation 超限会被明确截断，而不是静默爆上下文。

#### 交付标准

- 默认配置下，任意单个 Tool observation 不超过约定阈值。
- 长运行任务的日志和结果仍可追踪。

### 16.4 能力四：权限与风险控制

#### 目标

- 把“能执行工具”升级为“受控地执行工具”。
- 让项目具备更强的工程可信度和演示安全感。

#### 首轮范围

- 为 Tool 增加风险等级或危险标记扩展。
- 为 shell 引入最小白名单或命令类别控制。
- 为删除、覆盖写入、潜在破坏性操作增加显式开关。
- 支持 `dry_run` 模式。

#### 改动建议

- `internal/tools/tool.go`
  - 增加 `RiskLevel`、`RequiresApproval` 或等价字段。
- `internal/tools/shell_tool.go`
  - 支持 `argv` 模式下的命令校验。
  - 增加命令白名单或禁用前缀列表。
- `internal/tools/file_write_tool.go`
  - 支持 `dry_run` 结果预览。
- `internal/tools/file_delete_tool.go`
  - 输出更明确的危险提示和执行摘要。
- `internal/runtime/runtime.go`
  - 把配置中的权限策略统一透传给工具层。

#### 风险点

- 限制过严会影响工具可用性。
- 限制过松则无法体现工程边界价值。

#### 验证要求

- 禁止命令被清晰拒绝，并说明原因。
- `dry_run` 模式下不落盘，但返回拟执行结果。
- 危险操作默认关闭；配置显式打开后才允许执行。

#### 交付标准

- 项目可以明确说明“默认安全边界是什么、如何放权、如何审计”。
- demo 时不会因为危险命令或误删文件而影响观感。

### 16.5 能力五：Eval、Trace 与 Demo 闭环

#### 目标

- 让项目从“功能存在”升级为“能力可验证、结果可解释、效果可展示”。
- 提供简历和面试中最关键的证据链。

#### 首轮范围

- 固定 10 到 20 条本地任务作为 eval 集。
- 记录每次运行的：
  - 步数
  - 工具调用次数
  - 失败原因
  - 耗时
- 形成至少三条稳定 demo 路径：
  - skill 激活
  - search/read/patch
  - MCP 接入

#### 改动建议

- `internal/runlog/runlog.go`
  - 增加结构化统计输出。
- `cmd/happyagent/main.go`
  - 如有必要，增加 `--trace-json` 或 `--eval-case` 参数。
- `tests/integration/`
  - 补固定任务集或最小假模型集成用例。
- `docs/demo.md`
  - 明确标准演示步骤、预期现象和失败排查。
- 新增 `docs/evaluation.md`
  - 记录任务集、指标口径和当前结果。

#### 风险点

- 如果评测任务定义太随意，数据没有说服力。
- 如果 demo 依赖太多外部条件，演示稳定性仍然不足。

#### 验证要求

- 至少一条本地 demo 无需修改代码即可复现。
- eval 任务能重复执行，并输出稳定统计结果。
- 失败案例有可归因的 trace，而不是只剩一句报错。

#### 交付标准

- README 中可以引用 demo 和评测文档。
- 项目对外可清楚回答“能做什么、如何验证、当前效果如何”。

## 17. 第二阶段实施顺序

第二阶段建议严格按以下顺序推进：

1. 先做搜索与定位能力。
2. 再做精确改码能力。
3. 然后补上下文压缩与分页。
4. 再补权限与风险控制。
5. 最后收口到 eval、trace 和 demo。

这样排序的原因：

- 没有搜索能力，精确改码工具的价值会被严重削弱。
- 没有精确改码能力，coding agent 很难体现“真正可用”。
- 上下文管理和权限控制决定项目是否能稳定演示，而不是只在小样例上成功。
- eval 和 demo 最后做，才能反映当前实现的真实能力，而不是提前包装。

## 18. 第二阶段最低交付标准

第二阶段完成后，项目至少满足以下条件：

- 具备 `search -> read range -> patch -> verify` 的基础工作流。
- 任意单个 observation 都有明确的大小边界。
- shell、write、delete 等高风险能力有清晰限制策略。
- 至少三条 demo 路径可按文档稳定复现。
- 至少一组固定任务集可重复执行并给出统计结果。

只有达到这组标准，`happyagent` 才能从“有潜力的 Agent Runtime 原型”升级到“适合写进简历、并能经得起面试追问的 AI 工程项目”。
