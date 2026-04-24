# Architecture

当前架构分为五层：

1. `cmd/happyagent`
   - 进程入口，负责读取配置并启动 runtime。
2. `internal/config`
   - 默认值、JSON 配置加载、环境变量覆盖、基础校验。
3. `internal/llm`
   - 定义统一聊天接口；当前直接通过 `eino-ext` 的 OpenAI ChatModel 接真实模型。
4. `internal/tools`
   - 定义工具协议、注册中心、路径隔离和本地文件/命令工具。
5. `internal/engine`
   - 使用单 `Runner` 驱动 loop，并在内部区分 `plan step` 和 `execute step`。
6. `internal/runtime`
   - 负责组装配置、LLM client、Tool registry 和 engine。

## 当前执行路径

1. `main.go` 读取配置。
   - 当未显式传入 `-config` 时，默认尝试读取仓库根目录的 `happyagent.local.json`。
2. `runtime.Builder` 创建 LLM client。
3. `runtime.Builder` 注册本地 tools。
4. `engine.Runner` 进入 loop。
5. `planStep` 向模型请求结构化动作。
6. `executeStep` 执行工具或返回最终答案。

## 当前限制

- runtime 尚未把 Tool 定义传给 engine，用于 prompt 注入的接线还需要补。
- MCP、Skill、测试目录还未开始实现。
- 当前配置文件只支持 JSON，YAML 会在后续阶段再加。
