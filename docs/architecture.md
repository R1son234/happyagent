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
   - 负责组装配置、LLM client、Tool registry、MCP manager、skill loader 和 engine。

## 当前执行路径

1. `main.go` 读取配置。
   - 通过 `config.Load()` 固定读取仓库根目录的 `happyagent.local.json`。
2. `runtime.Builder` 创建 LLM client。
3. `runtime.Builder` 注册本地 tools。
4. `runtime.Builder` 连接配置中的 MCP server，并把远程 tool 注册进工具表。
5. `runtime.Run` 读取本地 skill catalog，并创建当前请求的 skill session。
6. 初始 prompt 保持简洁；模型如果需要当前 skill catalog 或 MCP resources，可调用 `list_capabilities`。
7. 模型按需调用 `activate_skill` 后，runtime 再加载该 skill 的正文并注入后续轮次的 system prompt。
8. `engine.Runner` 进入 loop。
9. `planStep` 向模型请求结构化动作。
10. `executeStep` 执行工具或返回最终答案。

## 当前限制

- MCP 当前只支持 stdio 子进程 transport，不支持 HTTP/streamable transport。
- skill 当前只支持本地目录加载，不支持动态脚本执行。
- 当前配置文件只支持 JSON，YAML 会在后续阶段再加。
