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

## Web Tools

- Web tools are disabled by default with `web.enabled: false`.
- `web_search` uses the configured SearXNG service when present; otherwise it uses zero-config direct HTML search.
- The direct backend tries Baidu first and falls back to Bing when Baidu returns a challenge page or no parseable results. It is best-effort and may break if search result HTML changes or search sites block automated requests.
- `web_fetch` only supports HTTP/HTTPS GET requests and returns bounded text previews.
- Localhost, loopback, private networks, link-local addresses, multicast, unspecified addresses, and metadata IPs are blocked unless `web.allow_private_networks` is explicitly true.
- URL redirects are re-checked before the final page is fetched.
- URLs containing values that look like tokens, API keys, secrets, or passwords are rejected.
- `web.blocked_domains` can deny exact domains and subdomains; wildcard rules like `*.example.com` match subdomains only.
- `web.max_fetch_bytes` limits returned content size. Large observations may still be offloaded by the engine if offload is enabled.

## Persistence And Logs

- Session and run state is stored under `.happyagent/store/`.
- Career Copilot workspace material is stored under `career-workspace/` by default.
- JSON metadata writes use temporary files and atomic replacement.
- Run logs can contain prompts, tool observations, model output, and user-provided material. Do not commit `logs/`, `.happyagent/`, or local workspace directories.

## Dangerous Tool Policy

The policy engine allows non-dangerous tools, blocks explicitly denied tools, and requires approval for dangerous tools that are not in the approved list. Built-in write, patch, delete, shell, and unconfigured MCP tools should be treated as operations that can change local state or expose sensitive context. Local defaults can be configured with `tools.approved_tools`; per-run `--approve-tools` values are merged with those defaults.
