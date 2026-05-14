# Security

`happyagent` is local-first, but it can still read files, write files, start shell commands, and connect to MCP servers. Treat profiles and config as the active safety boundary.

## File Boundaries

- File tools resolve paths through a configured root directory.
- Symlink escapes outside the root are rejected.
- Write and delete tools have separate enable flags.
- File writes are bounded by `tools.write_max_bytes`.

## Shell Tools

- Shell execution is controlled by `tools.shell_enabled`.
- When enabled, commands must be present in `tools.shell_allowed_commands`.
- Shell calls are marked dangerous and require approval unless the run explicitly approves the tool.
- Keep shell enabled only for profiles that need it.

## MCP Tools

- Remote MCP tools are dangerous by default.
- A server can mark specific remote tools as non-dangerous with `mcp.servers[].safe_tools`.
- Safe tool names can use either the remote tool name, such as `repeat`, or the runtime-qualified name, such as `helper__repeat`.
- MCP servers inherit the parent process environment plus configured `env` entries, so only enable servers you trust.

Example:

```json
{
  "mcp": {
    "servers": [
      {
        "name": "helper",
        "command": "./bin/mcpdemo",
        "args": [],
        "enabled": true,
        "safe_tools": ["repeat"]
      }
    ]
  }
}
```

## Persistence And Logs

- Session and run state is stored under `.happyagent/store/`.
- Career Copilot workspace material is stored under `career-workspace/` by default.
- JSON metadata writes use temporary files and atomic replacement.
- Run logs can contain prompts, tool observations, model output, and user-provided material. Do not commit `logs/`, `.happyagent/`, or local workspace directories.

## Dangerous Tool Policy

The policy engine allows non-dangerous tools, blocks explicitly denied tools, and requires approval for dangerous tools that are not in the approved list. Built-in write, patch, delete, shell, and unconfigured MCP tools should be treated as operations that can change local state or expose sensitive context. Local defaults can be configured with `tools.approved_tools`; per-run `--approve-tools` values are merged with those defaults.
