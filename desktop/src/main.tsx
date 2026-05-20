import React, { useEffect, useMemo, useState } from "react";
import Markdown from "react-markdown";
import { createRoot } from "react-dom/client";
import {
  Bot,
  CheckCircle2,
  ChevronRight,
  FileText,
  Folder,
  Loader2,
  Search,
  Send,
  Settings,
  Upload
} from "lucide-react";
import "./styles.css";

type FileNode = {
  name: string;
  path: string;
  kind: "file" | "directory";
  size?: number;
  modified?: string;
  children?: FileNode[];
};

type Preview = {
  path: string;
  name: string;
  kind: string;
  size: number;
  modified: string;
  content: string;
  truncated: boolean;
};

type WorkspaceStatus = {
  root: string;
  counts: Record<string, number>;
  index: { items: WorkspaceItem[] };
};

type WorkspaceItem = {
  id: string;
  type: string;
  title: string;
  path: string;
  tags?: string[];
  summary?: string;
};

type ChatMessage = {
  role: "user" | "assistant" | "system";
  content: string;
};

type SettingsPayload = {
  path: string;
  content: string;
  restart_required?: boolean;
};

function App() {
  const [tree, setTree] = useState<FileNode | null>(null);
  const [status, setStatus] = useState<WorkspaceStatus | null>(null);
  const [modelName, setModelName] = useState("");
  const [preview, setPreview] = useState<Preview | null>(null);
  const [query, setQuery] = useState("");
  const [input, setInput] = useState("");
  const [sessionId, setSessionId] = useState("");
  const [messages, setMessages] = useState<ChatMessage[]>([
    {
      role: "assistant",
      content: "把资料拖进来，或直接告诉我你想整理什么。我会基于当前资料库工作。"
    }
  ]);
  const [running, setRunning] = useState(false);
  const [runSteps, setRunSteps] = useState<string[]>([]);
  const [error, setError] = useState("");
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [settingsPath, setSettingsPath] = useState("");
  const [settingsContent, setSettingsContent] = useState("");
  const [settingsMessage, setSettingsMessage] = useState("");
  const [settingsSaving, setSettingsSaving] = useState(false);

  useEffect(() => {
    void loadWorkspace();
  }, []);

  async function loadWorkspace() {
    const [treeRes, statusRes, healthRes] = await Promise.all([
      fetch("/api/files/tree"),
      fetch("/api/workspace/status"),
      fetch("/api/health")
    ]);
    setTree(await treeRes.json());
    setStatus(await statusRes.json());
    const health = await healthRes.json();
    if (health.model) setModelName(health.model);
  }

  async function openFile(path: string) {
    if (!path) return;
    const res = await fetch(`/api/files/preview?path=${encodeURIComponent(path)}`);
    if (!res.ok) {
      setError("无法打开这个文件。");
      return;
    }
    setPreview(await res.json());
  }

  async function sendMessage() {
    const text = input.trim();
    if (!text || running) return;
    setInput("");
    setRunning(true);
    setError("");
    setRunSteps(["已接收请求", "正在读取资料库"]);
    setMessages((items) => [...items, { role: "user", content: text }]);
    try {
      const res = await fetch("/api/chat/runs", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          session_id: sessionId,
          profile: "career-copilot",
          input: text
        })
      });
      const data = await res.json();
      if (data.session_id) setSessionId(data.session_id);
      if (!res.ok) {
        throw new Error(data.error || "Agent 运行失败。");
      }
      setRunSteps((items) => [...items, "已生成回答", "已保存运行记录"]);
      setMessages((items) => [
        ...items,
        { role: "assistant", content: data.record?.output || "已完成。" }
      ]);
      await loadWorkspace();
    } catch (err) {
      const message = err instanceof Error ? err.message : "Agent 运行失败。";
      setError(message);
      setMessages((items) => [...items, { role: "system", content: message }]);
    } finally {
      setRunning(false);
    }
  }

  async function importDroppedFiles(files: FileList | null) {
    if (!files || files.length === 0) return;
    const form = new FormData();
    Array.from(files).forEach((file) => form.append("files", file));
    setRunning(true);
    setError("");
    setRunSteps(["正在上传文件", "正在导入资料库"]);
    try {
      const res = await fetch("/api/files/upload", {
        method: "POST",
        body: form
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || "文件导入失败。");
      const imported = data.items?.length || 0;
      const warningText = data.warnings?.length ? `，${data.warnings.length} 个文件需要检查` : "";
      setMessages((items) => [
        ...items,
        { role: "assistant", content: `已导入 ${imported} 个资料${warningText}。` }
      ]);
      setRunSteps(["文件已保存到 inbox", "资料库索引已更新"]);
      await loadWorkspace();
    } catch (err) {
      const message = err instanceof Error ? err.message : "文件导入失败。";
      setError(message);
    } finally {
      setRunning(false);
    }
  }

  async function openSettings() {
    setSettingsOpen(true);
    setSettingsMessage("正在读取配置...");
    try {
      const res = await fetch("/api/settings");
      const data: SettingsPayload & { error?: string } = await res.json();
      if (!res.ok) throw new Error(data.error || "无法读取配置。");
      setSettingsPath(data.path);
      setSettingsContent(data.content);
      setSettingsMessage("");
    } catch (err) {
      setSettingsMessage(err instanceof Error ? err.message : "无法读取配置。");
    }
  }

  async function saveSettings() {
    setSettingsSaving(true);
    setSettingsMessage("");
    try {
      JSON.parse(settingsContent);
    } catch (err) {
      setSettingsSaving(false);
      setSettingsMessage(err instanceof Error ? `JSON 格式错误：${err.message}` : "JSON 格式错误。");
      return;
    }
    try {
      const res = await fetch("/api/settings", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ content: settingsContent })
      });
      const data: SettingsPayload & { error?: string; model?: string } = await res.json();
      if (!res.ok) throw new Error(data.error || "保存配置失败。");
      setSettingsPath(data.path);
      setSettingsContent(data.content);
      if (data.model) setModelName(data.model);
      setSettingsMessage(data.restart_required ? "已保存。模型、工具和 MCP 等运行时配置需要重启桌面端后完全生效。" : "已保存。");
      await loadWorkspace();
    } catch (err) {
      setSettingsMessage(err instanceof Error ? err.message : "保存配置失败。");
    } finally {
      setSettingsSaving(false);
    }
  }

  const selectedItem = useMemo(() => {
    if (!preview || !status) return null;
    return status.index.items.find((item) => item.path === preview.path) || null;
  }, [preview, status]);

  const indexTextByPath = useMemo(() => buildIndexText(status?.index.items || []), [status]);

  return (
    <main className="desktop-frame">
      <header className="topbar">
        <div className="traffic">
          <i className="dot red" />
          <i className="dot yellow" />
          <i className="dot green" />
        </div>
        <div className="workspace-title">
          HappyAgent <span>/ {status?.root || "资料库"}</span>
        </div>
        <div className="top-actions">
          <div className="pill"><CheckCircle2 size={14} /> {modelName || "模型已连接"}</div>
          <button className="pill icon-button" onClick={openSettings} type="button"><Settings size={14} /> 设置</button>
        </div>
      </header>

      <section className="app-grid">
        <aside className="sidebar">
          <div className="search-panel">
            <label className="searchbox">
              <Search size={15} />
              <input
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder="搜索文件、标签、证据点"
              />
            </label>
          </div>
          <div className="tree">
            {tree ? (
              <TreeNode node={tree} query={query} indexTextByPath={indexTextByPath} onOpen={openFile} level={0} />
            ) : (
              <div className="muted">正在读取资料库...</div>
            )}
          </div>
        </aside>

        <section className="main">
          <div className="tabs">
            <button className="tab active" type="button">阅读</button>
            <button className="tab" type="button">结构</button>
            <button className="tab" type="button">图谱</button>
            <button className="tab" type="button">结果</button>
          </div>
          <article className="reader">
            {preview ? <PreviewPane preview={preview} item={selectedItem} /> : <EmptyReader />}
          </article>
        </section>

        <aside className="inspector">
          <section className="panel">
            <h2>文件属性</h2>
            <KeyValue label="类型" value={selectedItem?.type || preview?.kind || "-"} />
            <KeyValue label="路径" value={preview?.path || "-"} />
            <KeyValue label="大小" value={preview ? formatBytes(preview.size) : "-"} />
            <KeyValue label="来源" value="本地资料库" />
          </section>
          <section className="panel">
            <h2>Agent 建议</h2>
            <ActionButton text="生成问题清单" onClick={() => setInput("基于当前资料，生成接下来要准备的问题清单。")} />
            <ActionButton text="整理当前材料" onClick={() => setInput("请整理当前资料，并给出结构化摘要。")} />
          </section>
          <section className="panel">
            <h2>资料库状态</h2>
            <div className="status-grid">
              {Object.entries(status?.counts || {}).map(([key, value]) => (
                <span key={key}>{key}: {value}</span>
              ))}
            </div>
            {error && <div className="risk">{error}</div>}
          </section>
        </aside>

        <footer
          className="assistant-bar"
          onDragOver={(event) => event.preventDefault()}
          onDrop={(event) => {
            event.preventDefault();
            void importDroppedFiles(event.dataTransfer.files);
          }}
        >
          <section className="chat">
            <div className="messages">
              {messages.slice(-3).map((message, index) => (
                <div className={`bubble ${message.role}`} key={`${message.role}-${index}`}>
                  {message.content}
                </div>
              ))}
            </div>
	            <div className="composer">
	              <Upload size={16} />
	              <input
	                value={input}
	                onChange={(event) => setInput(event.target.value)}
	                onKeyDown={(event) => {
	                  if (event.key === "Enter" && event.metaKey) {
	                    event.preventDefault();
	                    void sendMessage();
	                  }
	                }}
	                placeholder="输入问题，Cmd+Enter 发送；也可以拖入文件..."
	              />
              <button className="send" onClick={sendMessage} disabled={running} type="button">
                {running ? <Loader2 className="spin" size={16} /> : <Send size={16} />}
                {running ? "运行中" : "发送"}
              </button>
            </div>
          </section>
          <section className="run">
            <h2>运行状态</h2>
            {(runSteps.length ? runSteps : ["等待用户输入", "资料库已就绪"]).map((step, index) => (
              <div className={`step ${running && index === runSteps.length - 1 ? "active" : ""}`} key={step}>
                <span className="mark" />
                {step}
              </div>
            ))}
          </section>
        </footer>
      </section>
      {settingsOpen && (
        <div className="modal-backdrop" role="presentation" onMouseDown={() => setSettingsOpen(false)}>
          <section className="settings-modal" role="dialog" aria-modal="true" aria-labelledby="settings-title" onMouseDown={(event) => event.stopPropagation()}>
            <header className="settings-header">
              <div>
                <h2 id="settings-title">编辑本地配置</h2>
                <p>{settingsPath || "happyagent.local.json"}</p>
              </div>
              <button className="modal-close" onClick={() => setSettingsOpen(false)} type="button">×</button>
            </header>
            <textarea
              className="settings-editor"
              spellCheck={false}
              value={settingsContent}
              onChange={(event) => setSettingsContent(event.target.value)}
            />
            {settingsMessage && <div className={settingsMessage.startsWith("已保存") ? "settings-note" : "settings-error"}>{settingsMessage}</div>}
            <footer className="settings-actions">
              <button className="secondary-button" onClick={() => setSettingsOpen(false)} type="button">取消</button>
              <button className="primary-button" onClick={saveSettings} disabled={settingsSaving || !settingsContent.trim()} type="button">
                {settingsSaving ? "保存中..." : "保存配置"}
              </button>
            </footer>
          </section>
        </div>
      )}
    </main>
  );
}

function TreeNode({
  node,
  query,
  indexTextByPath,
  onOpen,
  level
}: {
  node: FileNode;
  query: string;
  indexTextByPath: Map<string, string>;
  onOpen: (path: string) => void;
  level: number;
}) {
  const [open, setOpen] = useState(level < 2);
  const normalizedQuery = query.trim().toLowerCase();
  const searching = normalizedQuery !== "";
  const visibleChildren = (node.children || []).filter((child) => matchesNode(child, normalizedQuery, indexTextByPath));
  if (level > 0 && searching && !matchesNode(node, normalizedQuery, indexTextByPath)) {
    return null;
  }
  if (node.kind === "directory") {
    return (
      <div className="tree-section">
        {level > 0 && (
          <button className="tree-head" onClick={() => setOpen(!open)} type="button" style={{ paddingLeft: 8 + level * 12 }}>
            <ChevronRight className={open ? "chevron open" : "chevron"} size={14} />
            <Folder size={15} /> {node.name}
          </button>
        )}
        {(open || level === 0 || searching) && visibleChildren.map((child) => (
          <TreeNode node={child} query={query} indexTextByPath={indexTextByPath} onOpen={onOpen} level={level + 1} key={child.path || child.name} />
        ))}
      </div>
    );
  }
  return (
    <button className="tree-item" onClick={() => onOpen(node.path)} type="button" style={{ paddingLeft: 14 + level * 12 }}>
      <FileText size={14} />
      <span className="name">{node.name}</span>
    </button>
  );
}

function PreviewPane({ preview, item }: { preview: Preview; item: WorkspaceItem | null }) {
  return (
    <>
      <div className="doc-kicker">{preview.kind} · {preview.path}</div>
      <h1>{item?.title || preview.name}</h1>
      <div className="doc-meta">
        {(item?.tags || [preview.kind]).slice(0, 5).map((tag) => (
          <span className="meta-tag" key={tag}>{tag}</span>
        ))}
      </div>
      {preview.content ? (
        preview.kind === "markdown" ? (
          <div className="preview-markdown"><Markdown>{preview.content}</Markdown></div>
        ) : preview.kind === "json" ? (
          <pre className="preview-json">{formatJSON(preview.content)}</pre>
        ) : (
          <pre className="preview-text">{preview.content}</pre>
        )
      ) : (
        <div className="notice">
          这个文件类型暂不支持直接内嵌预览。你仍然可以让 Agent 基于已提取的资料或索引继续工作。
        </div>
      )}
    </>
  );
}

function EmptyReader() {
  return (
    <div className="empty-reader">
      <Bot size={42} />
      <h1>打开一个文件，或直接问 HappyAgent</h1>
      <p>左侧是本地资料库。选择文件后会在这里预览，底部可以输入自然语言请求。</p>
    </div>
  );
}

function KeyValue({ label, value }: { label: string; value: string }) {
  return (
    <div className="kv">
      <span>{label}</span>
      <span title={value}>{value}</span>
    </div>
  );
}

function ActionButton({ text, onClick }: { text: string; onClick: () => void }) {
  return (
    <button className="suggestion" onClick={onClick} type="button">
      {text} <span>›</span>
    </button>
  );
}

function formatBytes(value: number) {
  if (!Number.isFinite(value)) return "-";
  if (value < 1024) return `${value} B`;
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`;
  return `${(value / 1024 / 1024).toFixed(1)} MB`;
}

function formatJSON(raw: string): string {
  try { return JSON.stringify(JSON.parse(raw), null, 2); }
  catch { return raw; }
}

function buildIndexText(items: WorkspaceItem[]) {
  const byPath = new Map<string, string>();
  items.forEach((item) => {
    byPath.set(item.path, [
      item.title,
      item.path,
      item.type,
      item.summary,
      ...(item.tags || [])
    ].filter(Boolean).join(" ").toLowerCase());
  });
  return byPath;
}

function matchesNode(node: FileNode, query: string, indexTextByPath: Map<string, string>): boolean {
  if (!query) return true;
  const ownText = [node.name, node.path, indexTextByPath.get(node.path)].filter(Boolean).join(" ").toLowerCase();
  if (ownText.includes(query)) return true;
  return (node.children || []).some((child) => matchesNode(child, query, indexTextByPath));
}

createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
