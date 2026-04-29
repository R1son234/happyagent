# Agent Platform Upgrade Plan

本文档用于规划 `happyagent` 从“本地 Agent Runtime”升级为“可展示生产化思路的 AI Agent 后端平台”。目标岗位是 AI Agent / LLM 应用 / Agent Platform 方向的后端开发，因此方案优先覆盖岗位 JD 中高频出现的能力：

- LLM observability / evals
- session state / tool dispatch / context engineering
- pluggable persistence
- RAG pipeline and vector search
- MCP registry and transports
- multi-agent / A2A-style discovery and delegation

本文只描述落地方案，不直接实现代码。

---

## 1. 总体目标

升级后的 `happyagent` 应能被描述为：

> A Go-based AI Agent runtime and backend platform with tool orchestration, MCP integration, skill loading, session persistence, observability, eval workflows, vector RAG, and multi-agent delegation demos.

具体要达到的面试展示效果：

1. 能跑一个真实 agent task。
2. 能查看本次 run 的 trace、token、tool call、latency、error category。
3. 能把 session / run 持久化到 SQLite。
4. 能对本地文档做 chunking、embedding、hybrid retrieval，并带 citation。
5. 能注册多个 MCP server，解决 tool/resource 命名冲突，并同时支持 stdio 与 HTTP/streamable transport。
6. 能展示一个 A2A-style multi-agent delegation demo，例如 planner agent 把子任务委托给 researcher / coder / reviewer profile，并记录标准化 parent-child run 关系。

---

## 2. 建议实施顺序

推荐按下面顺序做，原因是每一步都能形成独立可验收成果，并且后一阶段能复用前一阶段的基础设施。

| Phase | 名称 | 核心产出 | 面试价值 |
| --- | --- | --- | --- |
| 1 | OpenTelemetry + Prometheus | 标准 metrics/traces、eval 指标 | 对齐 LangSmith / LLM observability |
| 2 | Store 抽象 + SQLite | 可插拔持久化、SQLite run/session store | 对齐后端工程能力 |
| 3 | Vector RAG | embedding、chunking、hybrid search、citation | 对齐 RAG / context engineering |
| 4 | MCP Registry + Transport | server registry、resource disambiguation、HTTP transport 抽象 | 对齐 MCP 平台能力 |
| 5 | Multi-agent / A2A demo | agent card、delegation、sub-agent trace | 对齐 multi-agent orchestration |

---

## 3. Phase 1：OpenTelemetry + Prometheus

### 3.1 目标

把当前自定义的 `observe.Metrics` 和 `engine.RunTrace` 扩展为标准可观测能力：

- Prometheus metrics：用于聚合和 dashboard。
- OpenTelemetry traces：用于查看一次 run 的完整执行链路。
- Eval metrics：用于展示 agent 改动前后的质量变化。

### 3.2 当前基础

已有模块：

- `internal/observe`
  - event recorder
  - error classifier
  - basic metrics
- `internal/engine`
  - step record
  - run trace
  - token usage
  - tool call count
  - planning / execution duration
- `internal/eval`
  - suite result
  - case result
  - success rate
  - average steps / tool calls

这些数据已经足够支撑完整的标准化观测落地。

### 3.3 指标设计

建议新增 Prometheus metrics：

| Metric | Type | Labels | 来源 |
| --- | --- | --- | --- |
| `happyagent_runs_total` | Counter | `profile`, `status`, `error_category` | app/runtime |
| `happyagent_run_duration_seconds` | Histogram | `profile`, `status` | runtime |
| `happyagent_model_latency_seconds` | Histogram | `model`, `profile` | llm client / engine plan step |
| `happyagent_model_tokens_total` | Counter | `model`, `profile`, `kind=prompt|completion|total` | engine trace |
| `happyagent_tool_calls_total` | Counter | `profile`, `tool`, `status` | engine tool records |
| `happyagent_tool_latency_seconds` | Histogram | `profile`, `tool`, `status` | engine execute step |
| `happyagent_eval_cases_total` | Counter | `suite`, `profile`, `status` | eval runner |
| `happyagent_eval_success_rate` | Gauge | `suite`, `profile` | eval summary |

注意：

- `run_id`、`session_id` 不应作为 Prometheus label，避免 high cardinality。
- `run_id`、`session_id` 可以进入 trace attributes / structured logs。

### 3.4 Trace 设计

建议每个 run 形成一条 OpenTelemetry trace：

```text
run
  plan_step_1
    llm.generate
  execute_step_1
    tool.file_search
  plan_step_2
    llm.generate
  execute_step_2
    tool.file_read
  final_answer
```

建议 span attributes：

- run span
  - `happyagent.run_id`
  - `happyagent.session_id`
  - `happyagent.profile`
  - `happyagent.status`
  - `happyagent.error_category`
  - `llm.model`
- plan step span
  - `happyagent.step_index`
  - `llm.prompt_tokens`
  - `llm.completion_tokens`
  - `llm.total_tokens`
- tool span
  - `happyagent.tool_name`
  - `happyagent.tool_status`
  - `happyagent.observation_bytes`

### 3.5 建议代码结构

```text
internal/telemetry/
  config.go
  metrics.go
  tracing.go
  noop.go

internal/observe/
  observe.go
  classify.go
```

`internal/observe` 保持业务事件语义，`internal/telemetry` 负责标准导出。

建议新增配置：

```json
{
  "telemetry": {
    "enabled": true,
    "service_name": "happyagent",
    "prometheus_enabled": true,
    "prometheus_addr": ":9090",
    "otlp_enabled": true,
    "otlp_endpoint": "http://localhost:4318"
  }
}
```

本阶段交付必须同时包含 Prometheus HTTP endpoint 和 OTLP exporter。stdout trace exporter 只作为本地调试选项，不能替代 OTLP collector 接入。

### 3.6 实施任务

1. 新增 `config.TelemetryConfig`。
2. 新增 `internal/telemetry`。
3. 在 `cmd/happyagent` 初始化 telemetry provider。
4. 在 `runtime.Run` 建 run span。
5. 在 `engine.planStep` 建 model span，记录 model latency 和 token usage。
6. 在 `engine.executeToolCall` 建 tool span，记录 tool status 和 latency。
7. 在 `eval.RunSuite` 记录 eval metrics。
8. 在 README / docs 增加 metrics 使用说明。

### 3.7 验收标准

- `make test` 通过。
- 运行一次 agent 后，Prometheus endpoint 能看到 run、token、tool 指标。
- trace 中能看到 run -> step -> model/tool 的层级。
- eval 运行后能看到 suite success rate。

### 3.8 简历表达

```text
Added OpenTelemetry and Prometheus instrumentation for an AI agent runtime, tracking run latency, model token usage, tool call status, error categories, and eval pass rates.
```

---

## 4. Phase 2：SQLite Store 抽象

### 4.1 目标

把当前 JSON 文件 store 升级为可插拔存储层，并实现 SQLite 后端。

目标不是追求复杂数据库能力，而是展示后端工程基本功：

- interface boundary
- migration
- transactional append turn
- queryable run/session history
- future-ready Redis/cache extension

### 4.2 当前基础

当前 `internal/store.Store` 直接操作本地 JSON：

- `SaveSession`
- `SaveRun`
- `GetSession`
- `GetRun`
- `ListRuns`
- `ListAllRuns`

`internal/app.Application` 直接依赖 `*store.Store`。

### 4.3 目标接口

建议把 store 抽象为 interface：

```go
type Store interface {
    SaveSession(ctx context.Context, record SessionRecord) error
    SaveRun(ctx context.Context, record RunRecord) error
    GetSession(ctx context.Context, id string) (SessionRecord, error)
    GetRun(ctx context.Context, id string) (RunRecord, error)
    ListRuns(ctx context.Context, sessionID string) ([]RunRecord, error)
    ListAllRuns(ctx context.Context) ([]RunRecord, error)
}
```

目录结构直接按可插拔 store 目标拆分：

```text
internal/store/
  types.go
  store.go              # interface + common errors
  json_store.go         # 原实现迁移过来
  sqlite_store.go       # 新实现
  migrations.go
```

### 4.4 SQLite schema

建议 schema：

```sql
CREATE TABLE sessions (
  id TEXT PRIMARY KEY,
  profile TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE runs (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL REFERENCES sessions(id),
  profile TEXT NOT NULL,
  input TEXT NOT NULL,
  output TEXT NOT NULL,
  status TEXT NOT NULL,
  error_category TEXT,
  error_message TEXT,
  termination_reason TEXT,
  system_prompt TEXT,
  started_at TEXT NOT NULL,
  finished_at TEXT NOT NULL,
  trace_json TEXT NOT NULL,
  steps_json TEXT NOT NULL,
  events_json TEXT NOT NULL
);

CREATE INDEX idx_runs_session_started_at
ON runs(session_id, started_at);
```

trace / steps / events 使用 JSON 存储，同时为高频查询字段建独立列。这样既保留完整结构化数据，又能支持状态、时间、profile、error category 等后端查询。

### 4.5 配置设计

```json
{
  "store": {
    "driver": "sqlite",
    "path": ".happyagent/happyagent.db"
  }
}
```

driver 可选：

- `sqlite`
- `json`

默认使用 `sqlite`。`json` 只作为迁移兼容和测试 fixture 使用，不作为主路径。

### 4.6 事务设计

当前 `AppendUserTurn` 保存 run 后再保存 session。SQLite 版建议做事务：

1. 读取 session。
2. 读取历史 runs。
3. 执行 runtime。
4. `INSERT runs`。
5. `UPDATE sessions.updated_at`。

如果保留 `session.RunIDs` 字段，需要单独维护，但 SQLite 版更建议从 runs 表反查，不再冗余存 `RunIDs`。

### 4.7 实施任务

1. 抽 `store.Store` interface。
2. 把当前文件实现改名为 `JSONStore`。
3. `app.Application` 改依赖 interface。
4. 新增 `SQLiteStore`。
5. 新增 migration 初始化。
6. `cmd/happyagent` 根据配置选择 store driver。
7. 增加 SQLite store tests。
8. 保留原 JSON store tests，防止兼容层退化。

### 4.8 验收标准

- `make test` 通过。
- JSON store 仍能通过测试。
- SQLite store 能创建 session、append run、查询历史。
- CLI 能通过配置切换 store driver。

### 4.9 简历表达

```text
Refactored local JSON persistence into a pluggable store interface and implemented a SQLite backend with schema migrations and transactional run/session persistence.
```

---

## 5. Phase 3：Vector RAG

### 5.1 目标

把当前 `search_docs` 从“本地文本搜索工具”升级为可展示的 RAG pipeline：

- document ingestion
- chunking
- embedding
- vector store
- hybrid retrieval
- citation

本阶段直接使用 SQLite 作为本地向量存储，并集成 `sqlite-vec`。如果构建环境需要 CGO 或扩展加载，安装和配置步骤必须写入 README，并纳入验收流程；不要用纯 FTS 替代向量检索。

### 5.2 当前基础

已有：

- `internal/rag/rag.go`
- `tools/search_docs_tool.go`
- docs search 不再预注入 prompt，而是由模型按需调用工具。

这已经符合 context engineering 的方向。

### 5.3 目标能力

新增两个 CLI / tool 入口：

1. ingestion：

```bash
./bin/happyagent-rag index docs README.md profiles
```

或集成到主程序：

```bash
./bin/happyagent --rag-index docs README.md
```

2. retrieval tool：

```json
{
  "query": "How does happyagent load MCP resources?",
  "top_k": 5,
  "hybrid": true
}
```

返回：

```text
[1] docs/architecture.md:12-18 score=0.82
...

Answer context:
...
```

### 5.4 数据模型

建议 SQLite tables：

```sql
CREATE TABLE documents (
  id TEXT PRIMARY KEY,
  path TEXT NOT NULL,
  content_hash TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE chunks (
  id TEXT PRIMARY KEY,
  document_id TEXT NOT NULL REFERENCES documents(id),
  path TEXT NOT NULL,
  start_line INTEGER,
  end_line INTEGER,
  content TEXT NOT NULL,
  token_count INTEGER,
  content_hash TEXT NOT NULL
);

CREATE VIRTUAL TABLE chunks_fts
USING fts5(content, path, content='chunks', content_rowid='rowid');
```

如果使用 `sqlite-vec`：

```sql
CREATE VIRTUAL TABLE chunk_embeddings
USING vec0(
  chunk_id TEXT PRIMARY KEY,
  embedding float[1536]
);
```

维度不要写死在业务层，由 embedding provider 返回。

### 5.5 Embedding provider 抽象

建议新增：

```text
internal/embedding/
  provider.go
  openai.go
  fake.go
```

接口：

```go
type Provider interface {
    EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error)
    EmbedQuery(ctx context.Context, text string) ([]float32, error)
    Dimensions() int
}
```

测试使用 fake embedding，保证不依赖外部 API。

### 5.6 Chunking 策略

支持 Markdown / text：

- 以 heading 和段落为优先边界。
- 当文档没有明确结构时，使用固定 token 窗口切分。
- 记录 `path`、`start_line`、`end_line`。
- chunk size 建议 500-1000 tokens。
- overlap 建议 80-150 tokens。

建议新增：

```text
internal/rag/
  chunker.go
  indexer.go
  retriever.go
  sqlite_store.go
  citation.go
```

### 5.7 Hybrid retrieval

默认 scoring：

```text
final_score = 0.65 * vector_score + 0.35 * keyword_score
```

同时实现一个可配置 reranker 接口，并提供 lexical reranker：

- simple lexical rerank
- LLM rerank 和 cross-encoder rerank 保留为接口实现位，但本阶段至少交付 lexical rerank 的真实实现和测试。

### 5.8 Citation 设计

`search_docs` 输出必须包含 citation：

```json
{
  "results": [
    {
      "path": "docs/architecture.md",
      "start_line": 23,
      "end_line": 35,
      "score": 0.82,
      "content": "..."
    }
  ]
}
```

最终答案如果引用 RAG 内容，profile prompt 应要求带文件路径。

### 5.9 实施任务

1. 新增 embedding provider interface。
2. 新增 fake embedding 测试实现。
3. 实现 markdown/text chunker。
4. 实现 SQLite RAG store。
5. 接入 `sqlite-vec`，并把本地安装/加载方式写入 README。
6. 实现 hybrid retriever。
7. 替换或新增 `rag_search` tool。
8. 为 ingestion / retrieval 增加 tests。
9. 更新 README，提供可复现实验。

### 5.10 验收标准

- 能 index 仓库 docs。
- 能通过 query 找回相关 chunk。
- 返回结果带 citation。
- 无外部 API 的 fake embedding 测试可稳定通过。
- 配置真实 embedding provider 后可执行端到端 RAG demo。

### 5.11 简历表达

```text
Built a local vector RAG pipeline with document chunking, embeddings, SQLite-based vector storage, hybrid retrieval, and citation-aware tool outputs for agent context engineering.
```

---

## 6. Phase 4：MCP Registry + 更完整 MCP Transport

### 6.1 目标

把当前 MCP 接入从“配置里列 server，然后启动时注册 tool”升级为更像平台能力的 MCP registry：

- server registry
- tool/resource namespace
- resource disambiguation
- server health/status
- stdio + HTTP/streamable transport 抽象

### 6.2 当前基础

已有：

- `internal/mcp/client.go`
- `internal/mcp/manager.go`
- `internal/mcp/tool_adapter.go`
- `mcp_read_resource`
- 自动注册远程 tool
- resource preview

当前问题：

- resource 以 URI 为唯一 key，多 server 同 URI 会冲突。
- MCP server 只能来自配置，缺少 registry 状态模型。
- 只支持 stdio。

### 6.3 Registry 数据模型

建议引入：

```go
type ServerRecord struct {
    ID        string
    Name      string
    Transport string // stdio, http
    Command   string
    Args      []string
    URL       string
    Env       map[string]string
    Enabled   bool
    Status    string // unknown, connecting, ready, failed
}

type ToolRecord struct {
    ID         string // serverName__toolName
    ServerName string
    RemoteName string
    SchemaJSON string
}

type ResourceRecord struct {
    ID         string // serverName::uri or stable hash
    ServerName string
    URI        string
    Name       string
    Description string
}
```

### 6.4 Resource disambiguation

建议 `list_capabilities` 返回：

```json
{
  "mcp_resources": [
    {
      "id": "demo::demo://project-summary",
      "server_name": "demo",
      "uri": "demo://project-summary",
      "name": "project-summary"
    }
  ]
}
```

`mcp_read_resource` 保留旧参数用于单 server 或 URI 唯一场景：

```json
{
  "uri": "demo://project-summary"
}
```

同时新增：

```json
{
  "resource_id": "demo::demo://project-summary"
}
```

当 URI 不唯一时，旧 `uri` 参数返回明确错误，提示使用 `resource_id`。

### 6.5 Transport 抽象

建议新增：

```go
type TransportFactory interface {
    Connect(ctx context.Context, server ServerRecord) (*Client, error)
}
```

实现：

```text
internal/mcp/
  transport.go
  stdio_transport.go
  http_transport.go
```

本阶段同时交付 stdio 和 HTTP/streamable transport。transport interface 是为了隔离实现，不是为了推迟 HTTP 支持。

### 6.6 HTTP / streamable transport

目标：

- 支持配置 HTTP MCP server URL。
- 支持鉴权 header，并支持从环境变量展开敏感值。
- 连接失败时 registry status 可见。

配置示例：

```json
{
  "mcp": {
    "servers": [
      {
        "name": "remote-docs",
        "transport": "http",
        "url": "http://localhost:8081/mcp",
        "headers": {
          "Authorization": "Bearer ${TOKEN}"
        },
        "enabled": true
      }
    ]
  }
}
```

优先使用当前 MCP SDK 的 HTTP/streamable transport；如果 SDK 能力不足，则在本仓库实现最小可用 HTTP transport，并用本地 demo server 验证 list tools、call tool、list resources、read resource 四条路径。

### 6.7 Registry operations

CLI 可增加：

```bash
./bin/happyagent mcp list
./bin/happyagent mcp inspect demo
./bin/happyagent mcp refresh demo
```

同时提供 server API：

- `GET /mcp/servers`
- `GET /mcp/servers/{name}/tools`
- `GET /mcp/servers/{name}/resources`

### 6.8 实施任务

1. 扩展 MCP config，增加 `transport`, `url`, `headers`。
2. 引入 `ServerRecord` / `ToolRecord` / `ResourceRecord`。
3. resource map 从 `map[uri]ResourceInfo` 改为 server-scoped registry。
4. `list_capabilities` 返回 resource id。
5. `mcp_read_resource` 支持 `resource_id`。
6. 抽 transport factory。
7. 保持 stdio 行为不退化。
8. 实现 HTTP/streamable transport。
9. 增加同 URI 多 server 测试。

### 6.9 验收标准

- 多个 MCP server 有相同 URI 时不会覆盖。
- `list_capabilities` 能显示 server-scoped resources。
- `mcp_read_resource` 可用 resource id 精确读取。
- stdio MCP demo 仍然通过。
- HTTP transport 有可运行 demo，测试不能以 skipped 作为验收。

### 6.10 简历表达

```text
Extended MCP integration into a server registry with namespaced tool/resource discovery, resource disambiguation, and transport abstraction for stdio and HTTP MCP servers.
```

---

## 7. Phase 5：Multi-agent / A2A Demo

### 7.1 目标

交付一个 A2A-style multi-agent orchestration demo。范围聚焦在本项目可控的 Agent Card、Task Delegation 和 Traceability，不实现跨组织安全协商等企业级扩展，但核心协议对象必须真实落地：

- agent card discovery
- sub-agent delegation
- profile-scoped tools
- parent run trace 包含 child run trace

### 7.2 Demo 场景

建议做一个适合求职项目展示的场景：

```text
用户输入：帮我评估 happyagent 是否适合放到 AI Agent 后端简历里。

planner agent:
  - 拆分任务
  - 委托 researcher agent 搜集 JD/关键词
  - 委托 repo analyst agent 分析本仓库能力
  - 委托 resume writer agent 生成简历 bullet
  - 汇总最终建议
```

### 7.3 Agent Card

新增：

```text
agents/
  researcher/agent.json
  repo-analyst/agent.json
  resume-writer/agent.json
```

示例：

```json
{
  "name": "repo-analyst",
  "description": "Analyze repository architecture and implementation details.",
  "profile": "general-assistant",
  "capabilities": ["repo_read", "architecture_summary"],
  "input_schema": "text",
  "output_schema": "analysis_report"
}
```

### 7.4 Delegation tool

新增工具：

```text
delegate_task
```

输入：

```json
{
  "agent_name": "repo-analyst",
  "task": "Summarize the runtime architecture and production-readiness gaps.",
  "max_steps": 5
}
```

输出：

```json
{
  "agent_name": "repo-analyst",
  "status": "completed",
  "output": "...",
  "child_run_id": "run-..."
}
```

### 7.5 Runtime 设计

建议新增：

```text
internal/agent/
  card.go
  loader.go
  registry.go
  delegation.go
```

`delegate_task` 不直接绕过 runtime，而是调用同一个 `runtime.Run`：

- 使用目标 agent card 指定的 profile。
- 继承 session id 或创建 child session。
- 限制 max steps。
- child run trace 记录 parent run id。

### 7.6 Trace 设计

Run trace 增加：

```go
ParentRunID string
ChildRunIDs []string
```

同时在 events 中记录：

```json
{
  "type": "delegate_task",
  "parent_run_id": "run-a",
  "child_run_id": "run-b",
  "agent_name": "repo-analyst"
}
```

trace 字段和 event 都要落地：trace 字段用于结构化查询，event 用于时间线展示。

### 7.7 A2A 对齐点

本阶段交付 A2A-style 的核心对象和调用语义：

- Agent Card：描述 agent identity、capabilities、input/output。
- Discovery：`list_agents` 工具返回可用 agent cards。
- Task delegation：`delegate_task`。
- Traceability：parent/child run relation。

Agent Card 和 Task object 的字段命名应尽量贴近 A2A 语义，方便面试时解释协议映射关系。

### 7.8 实施任务

1. 新增 agent card schema。
2. 新增 agent loader。
3. 新增 `list_agents` tool。
4. 新增 `delegate_task` tool。
5. runtime 支持 child run metadata。
6. 新增 demo profiles / agents。
7. 新增 multi-agent eval case。

### 7.9 验收标准

- planner 能发现可用 sub-agents。
- planner 能委托至少一个 sub-agent。
- child run 被持久化。
- trace / event 中能看到 parent-child 关系。
- demo 输出质量可通过 eval case 检查。

### 7.10 简历表达

```text
Built a lightweight multi-agent delegation layer with agent-card discovery, profile-scoped sub-agents, delegated task execution, and parent-child run tracing.
```

---

## 8. Cross-cutting Design Decisions

### 8.1 配置优先，硬编码靠后

新增能力都应能通过配置开关控制：

- telemetry enabled
- store driver
- rag provider
- mcp transport
- agent registry dir

这能让项目在本地 demo 和测试环境中保持轻量。

### 8.2 Interface first, implementation second

推荐抽象：

- `store.Store`
- `embedding.Provider`
- `rag.VectorStore`
- `mcp.TransportFactory`
- `agent.Registry`
- `telemetry.Recorder`

但不要过度抽象。每个 interface 必须至少有一个真实实现和一个测试实现。

### 8.3 测试策略

每个 phase 至少包含：

- unit tests
- one integration-style test
- docs demo command

涉及外部 API 的地方必须有 fake provider：

- fake LLM
- fake embedding
- fake MCP server

### 8.4 面试展示路径

最终建议准备一个固定 demo：

1. `make test`
2. `make build`
3. 启动 demo MCP server
4. 启动 Prometheus metrics endpoint
5. index docs 到 SQLite vector store
6. 运行一个 profile-aware agent task
7. 展示：
   - SQLite 中的 run/session
   - Prometheus metrics
   - trace JSON / OTel trace
   - RAG citation
   - MCP tools/resources
   - multi-agent child run

---

## 9. Suggested Milestones

### Milestone 1：Observability Baseline

范围：

- telemetry config
- Prometheus metrics
- basic OTel spans
- eval metrics

验收：

- `make test`
- `curl localhost:9090/metrics`
- 单次 run trace 可观察

### Milestone 2：SQLite Runtime Store

范围：

- store interface
- JSONStore compatibility
- SQLiteStore
- migrations

验收：

- SQLite 能保存和读取 session/run
- CLI 可切换 store driver

### Milestone 3：Citation RAG

范围：

- chunker
- embedding provider abstraction
- SQLite vector store
- hybrid retrieval
- citation output

验收：

- index docs
- `rag_search` 返回 citation
- fake embedding tests 稳定

### Milestone 4：MCP Platform Layer

范围：

- MCP server registry
- resource id
- transport abstraction
- stdio regression
- HTTP transport demo

验收：

- 同 URI resource 不冲突
- list/read resource 精确可控

### Milestone 5：Multi-agent Demo

范围：

- agent cards
- list agents
- delegate task
- child run trace
- demo eval

验收：

- planner -> sub-agent -> final answer 闭环可跑

---

## 10. Final Resume Positioning

完成上述能力后，项目可以在简历里定位为：

```text
happyagent: A Go-based AI Agent Runtime and Platform for local autonomous workflows, featuring tool orchestration, MCP integration, dynamic skill loading, profile-scoped capabilities, SQLite persistence, OpenTelemetry/Prometheus observability, vector RAG, eval workflows, and multi-agent delegation demos.
```

推荐 bullet：

```text
- Designed and implemented an agent runtime with structured tool calling, policy-gated dangerous tools, session memory, profile-scoped capabilities, and dynamic skill activation.
- Added production-style observability with OpenTelemetry and Prometheus, tracking model latency, token usage, tool execution status, run failures, and eval pass rates.
- Built a SQLite-backed persistence layer and vector RAG pipeline with chunking, embeddings, hybrid retrieval, and citation-aware outputs.
- Extended MCP integration with server-scoped tool/resource registry, resource disambiguation, and transport abstraction for stdio/HTTP MCP servers.
- Prototyped multi-agent delegation using agent-card discovery, sub-agent task execution, and parent-child run tracing.
```
