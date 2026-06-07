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
- `run_in_background` uses the same shell allowlist and policy gate before the job is started; completion is reported later through the background notification hook.
- Keep shell enabled only for profiles that need it.

## MCP Tools

- Remote MCP tools are dangerous by default.
- A server can mark specific remote tools as non-dangerous with `mcp.servers[].safe_tools`.
- Safe tool names can use either the remote tool name, such as `repeat`, or the canonical runtime name, such as `mcp__helper__repeat`.
- Runtime MCP tool names are always `mcp__<server>__<tool>`; the older `<server>__<tool>` spelling is not registered.
- MCP servers are connected from local config during runtime assembly. Model-initiated `mcp_connect` / `mcp_disconnect` is not exposed, so the model cannot dynamically add or remove MCP servers during a run.
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
- Career Copilot background tasks that classify or generate from saved material pass source paths and hashes in the user prompt, then require the model to read declared files through `file_read`. Those runs suppress session history and memory so a repair turn cannot reintroduce prior material through runtime context.
- Source-bound background runs allow `file_read` only for the declared source paths and block undeclared paths before tool execution. This preserves evidence traceability while keeping full document bodies out of generated user prompts.
- Career Copilot natural-language turns use an LLM semantic decision task for business intent, material classification, file candidate selection, and requested outputs. That decision cannot bypass code gates: returned paths must match predeclared candidates, material and output kinds must be known enums, low-confidence or confirmation-required decisions are not auto-executed, and generated reports still require source reads from declared files.

## Dangerous Tool Policy

The policy engine returns `allow`, `deny`, `ask`, or `passthrough`. Explicit deny wins over allow. Built-in write, patch, delete, shell, worktree, and unconfigured MCP tools should be treated as operations that can change local state or expose sensitive context. Local defaults can be configured with `tools.approved_tools`; per-run `--approve-tools` values are merged with those defaults.

Shell policy receives argv and workdir details, so commands such as destructive root removal, `sudo`, reboot/shutdown, `mkfs`, and path escapes are denied before execution. Child-agent dangerous actions use the same policy hook and cannot bypass lead approvals.

## Agents And Tasks

- Durable tasks are local JSON under `.happyagent/tasks/`; claims are atomic in-process and dependency cycles are rejected.
- Child agent traces and mailbox entries are stored under `.happyagent/agents/`.
- `agent_task` and async teammate runs get fresh model context and do not see recursive agent tools unless profile tool scope explicitly exposes them.
- Child-agent dangerous actions that require approval append a `permission_request` to the Lead inbox instead of bypassing policy.
- Child runs with `task_id` claim the task before running and complete it only after successful child execution.
- `agent_check_inbox` marks returned lead messages consumed only after listing them, so mailbox state remains auditable.
- Teammates do not run an autonomous idle loop that scans `.happyagent/tasks/` and self-claims work. Task ownership changes only through explicit task tools or Lead-delegated child runs with `task_id`.

## Worktrees

- Worktree paths must stay under `.happyagent/worktrees/`.
- `agent_task` and `agent_spawn` may bind a child run to a `worktree_path`; child shell calls default to that cwd while keeping the same shell allowlist and policy checks.
- `worktree_keep` validates and records the path for manual review without deleting it.
- `worktree_remove` refuses dirty worktrees unless `discard_dirty` is true.
- Worktree tools are marked dangerous and should be exposed only to profiles that need isolated concurrent filesystem work.
