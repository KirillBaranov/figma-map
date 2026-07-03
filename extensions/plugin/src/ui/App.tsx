import React, { useEffect, useMemo, useRef, useState } from "react";

type RequestType =
  | "get_document"
  | "get_selection"
  | "get_node"
  | "get_styles"
  | "get_metadata"
  | "get_design_context"
  | "get_variable_defs"
  | "get_screenshot"
  | "set_node_visibility"
  | "set_text_content"
  | "set_text_properties"
  | "set_node_properties"
  | "set_solid_fill"
  | "set_gradient_fill"
  | "set_effects"
  | "set_stroke_properties"
  | "set_auto_layout"
  | "create_frame"
  | "create_text"
  | "create_shape"
  | "create_image"
  | "duplicate_nodes"
  | "reparent_nodes"
  | "group_nodes"
  | "ungroup_node"
  | "set_selection"
  | "scroll_and_zoom_into_view"
  | "delete_nodes";

type ServerRequest = {
  type: RequestType;
  requestId: string;
  nodeIds?: string[];
  params?: Record<string, unknown>;
};

type PluginResponse = {
  type: RequestType;
  requestId: string;
  data?: unknown;
  error?: string;
};

type PluginStatus = {
  fileName: string;
  fileKey: string;
  selectionCount: number;
  selectedNodeIds: string[];
  selectedNodeNames: string[];
  version: string;
};

type LogEntry = {
  id: number;
  direction: "in" | "out";
  label: string;
  timestamp: number;
};

const WS_BASE_URL = "ws://localhost:1994/ws";
const REPO_URL = "https://github.com/KirillBaranov/figma-map";
const MAX_LOG_ENTRIES = 40;

function formatRelativeTime(timestampMs: number, now: number): string {
  const seconds = Math.max(0, Math.round((now - timestampMs) / 1000));
  if (seconds < 2) return "just now";
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.round(seconds / 60);
  return `${minutes}m ago`;
}

function formatDuration(ms: number): string {
  const totalSeconds = Math.max(0, Math.round(ms / 1000));
  const h = Math.floor(totalSeconds / 3600);
  const m = Math.floor((totalSeconds % 3600) / 60);
  const s = totalSeconds % 60;
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m ${s}s`;
  return `${s}s`;
}

function buildAgentPrompt(status: PluginStatus): string {
  if (status.selectedNodeIds.length === 0) return "";
  const primaryId = status.selectedNodeIds[0];
  const primaryName = status.selectedNodeNames[0] || "selection";
  const lines = [
    `Build "${primaryName}" (node ${primaryId}) from Figma file "${status.fileName}".`
  ];
  if (status.selectedNodeIds.length > 1) {
    lines.push(`Also selected: ${status.selectedNodeIds.slice(1).join(", ")}.`);
  }
  lines.push(
    `Run \`figma-map build plan ${primaryId} --json\`, implement it per the figma-map build loop, ` +
      `then \`figma-map verify reconcile ${primaryId} --story <storyId>\` until it reports match: true.`
  );
  return lines.join(" ");
}

// The plugin UI iframe is sandboxed without clipboard-write permission, so
// navigator.clipboard.writeText silently rejects. document.execCommand("copy")
// on a temporary textarea is the workaround Figma's own plugin samples use.
function copyToClipboard(text: string): boolean {
  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.style.position = "fixed";
  textarea.style.top = "-1000px";
  textarea.style.opacity = "0";
  document.body.appendChild(textarea);
  textarea.focus();
  textarea.select();
  let ok = false;
  try {
    ok = document.execCommand("copy");
  } catch {
    ok = false;
  }
  document.body.removeChild(textarea);
  return ok;
}

export default function App() {
  const [tab, setTab] = useState<"status" | "settings">("status");
  const [connected, setConnected] = useState(false);
  const [connectedAt, setConnectedAt] = useState<number | null>(null);
  const [status, setStatus] = useState<PluginStatus>({
    fileName: "Unknown file",
    fileKey: "",
    selectionCount: 0,
    selectedNodeIds: [],
    selectedNodeNames: [],
    version: ""
  });
  const [requestCount, setRequestCount] = useState(0);
  const [lastActivityAt, setLastActivityAt] = useState<number | null>(null);
  const [now, setNow] = useState(() => Date.now());
  const [copiedKey, setCopiedKey] = useState(false);
  const [copiedPrompt, setCopiedPrompt] = useState(false);
  const [debugOpen, setDebugOpen] = useState(false);
  const [copiedLog, setCopiedLog] = useState(false);
  const [log, setLog] = useState<LogEntry[]>([]);
  const socketRef = useRef<WebSocket | null>(null);
  const reconnectTimer = useRef<number | null>(null);
  const connectRef = useRef<(() => void) | null>(null);
  const logIdRef = useRef(0);

  const statusLabel = useMemo(
    () => (connected ? "Connected" : "Disconnected"),
    [connected]
  );

  const prompt = useMemo(() => buildAgentPrompt(status), [status]);

  const pushLog = (entry: Omit<LogEntry, "id">) => {
    logIdRef.current += 1;
    setLog((prev) => [{ ...entry, id: logIdRef.current }, ...prev].slice(0, MAX_LOG_ENTRIES));
  };

  // Tick once a second so "x ago" / uptime labels stay fresh without extra work elsewhere.
  useEffect(() => {
    const interval = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(interval);
  }, []);

  useEffect(() => {
    const handleMessage = (event: MessageEvent) => {
      const msg = event.data?.pluginMessage;
      if (!msg) return;

      if (msg.type === "plugin-status") {
        setStatus(msg.payload);
        return;
      }

      if (!("requestId" in msg)) {
        return;
      }

      pushLog({ direction: "out", label: msg.type ?? "response", timestamp: Date.now() });

      if (!socketRef.current || socketRef.current.readyState !== WebSocket.OPEN) {
        return;
      }
      socketRef.current.send(JSON.stringify(msg));
    };

    window.addEventListener("message", handleMessage);
    return () => {
      window.removeEventListener("message", handleMessage);
    };
  }, []);

  const handleCopyFileKey = () => {
    if (!status.fileKey) return;
    if (copyToClipboard(status.fileKey)) {
      setCopiedKey(true);
      window.setTimeout(() => setCopiedKey(false), 1200);
    }
  };

  const handleCopyPrompt = () => {
    if (!prompt) return;
    if (copyToClipboard(prompt)) {
      setCopiedPrompt(true);
      window.setTimeout(() => setCopiedPrompt(false), 1200);
    }
  };

  const handleReconnect = () => {
    if (reconnectTimer.current !== null) {
      window.clearTimeout(reconnectTimer.current);
      reconnectTimer.current = null;
    }
    connectRef.current?.();
  };

  const handleCopyLog = () => {
    if (log.length === 0) return;
    const text = [...log]
      .reverse()
      .map(
        (entry) =>
          `[${new Date(entry.timestamp).toISOString()}] ${entry.direction === "in" ? "IN " : "OUT"} ${entry.label}`
      )
      .join("\n");
    if (copyToClipboard(text)) {
      setCopiedLog(true);
      window.setTimeout(() => setCopiedLog(false), 1200);
    }
  };

  // Connect/reconnect WebSocket when fileKey changes
  useEffect(() => {
    if (!status.fileKey) return;

    let disposed = false;

    const connect = () => {
      if (disposed) return;

      if (socketRef.current) {
        const previousSocket = socketRef.current;
        previousSocket.onopen = null;
        previousSocket.onclose = null;
        previousSocket.onerror = null;
        previousSocket.onmessage = null;
        previousSocket.close();
      }

      const wsUrl = `${WS_BASE_URL}?fileKey=${encodeURIComponent(status.fileKey)}&fileName=${encodeURIComponent(status.fileName)}`;
      const ws = new WebSocket(wsUrl);
      socketRef.current = ws;

      ws.onopen = () => {
        setConnected(true);
        setConnectedAt(Date.now());
        parent.postMessage({ pluginMessage: { type: "ui-ready" } }, "*");
      };

      ws.onclose = () => {
        if (disposed || socketRef.current !== ws) return;
        setConnected(false);
        setConnectedAt(null);
        if (reconnectTimer.current === null) {
          reconnectTimer.current = window.setTimeout(() => {
            reconnectTimer.current = null;
            connect();
          }, 1500);
        }
      };

      ws.onerror = () => {
        if (disposed || socketRef.current !== ws) return;
        setConnected(false);
        setConnectedAt(null);
      };

      ws.onmessage = (event) => {
        if (disposed || socketRef.current !== ws) return;
        const payload = JSON.parse(event.data) as ServerRequest;
        setRequestCount((count) => count + 1);
        setLastActivityAt(Date.now());
        pushLog({ direction: "in", label: payload.type, timestamp: Date.now() });
        parent.postMessage({ pluginMessage: { type: "server-request", payload } }, "*");
      };
    };

    connectRef.current = connect;
    connect();

    return () => {
      disposed = true;
      if (reconnectTimer.current !== null) {
        window.clearTimeout(reconnectTimer.current);
        reconnectTimer.current = null;
      }
      if (socketRef.current) {
        const ws = socketRef.current;
        ws.onopen = null;
        ws.onclose = null;
        ws.onerror = null;
        ws.onmessage = null;
        ws.close();
        socketRef.current = null;
      }
    };
  }, [status.fileKey, status.fileName]);

  return (
    <div className="container">
      <div className={`status-banner ${connected ? "connected" : "disconnected"}`}>
        <span className={`dot ${connected ? "pulse" : ""}`} />
        <span className="badge-text">{statusLabel}</span>
        {connected ? (
          <span className="status-meta">
            {requestCount} req{requestCount === 1 ? "" : "s"}
          </span>
        ) : (
          <button type="button" className="reconnect-button" onClick={handleReconnect}>
            Reconnect
          </button>
        )}
      </div>

      <div className="tabs">
        <button
          type="button"
          className={`tab ${tab === "status" ? "active" : ""}`}
          onClick={() => setTab("status")}
        >
          Status
        </button>
        <button
          type="button"
          className={`tab ${tab === "settings" ? "active" : ""}`}
          onClick={() => setTab("settings")}
        >
          Settings
        </button>
      </div>

      {tab === "status" && (
        <>
          <div className="info-section">
            <button
              type="button"
              className="info-row info-row-button"
              onClick={handleCopyFileKey}
              disabled={!status.fileKey}
              title={status.fileKey ? "Click to copy file key" : undefined}
            >
              <span className="info-icon" aria-hidden="true">
                <svg viewBox="0 0 16 16" width="14" height="14" fill="none">
                  <path
                    d="M4 1.5h5l3 3v9a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V2.5a1 1 0 0 1 1-1Z"
                    stroke="currentColor"
                    strokeWidth="1.2"
                    strokeLinejoin="round"
                  />
                  <path d="M9 1.5V4h3" stroke="currentColor" strokeWidth="1.2" strokeLinejoin="round" />
                </svg>
              </span>
              <span className="info-label">File</span>
              <span className="info-value" title={status.fileName}>
                {status.fileName}
              </span>
              {status.fileKey && (
                <span className="copy-hint">{copiedKey ? "Copied" : "Copy key"}</span>
              )}
            </button>
            <div className="info-row">
              <span className="info-icon" aria-hidden="true">
                <svg viewBox="0 0 16 16" width="14" height="14" fill="none">
                  <path
                    d="M2 2.5 13 7l-4.2 1.7L7 13 2 2.5Z"
                    stroke="currentColor"
                    strokeWidth="1.2"
                    strokeLinejoin="round"
                    strokeLinecap="round"
                  />
                </svg>
              </span>
              <span className="info-label">Selection</span>
              <span className="info-value">
                {status.selectionCount} node{status.selectionCount === 1 ? "" : "s"}
              </span>
            </div>
            <div className="info-row">
              <span className="info-icon" aria-hidden="true">
                <svg viewBox="0 0 16 16" width="14" height="14" fill="none">
                  <circle cx="8" cy="8" r="6" stroke="currentColor" strokeWidth="1.2" />
                  <path d="M8 5v3.2L10 10" stroke="currentColor" strokeWidth="1.2" strokeLinecap="round" />
                </svg>
              </span>
              <span className="info-label">Activity</span>
              <span className="info-value">
                {lastActivityAt ? formatRelativeTime(lastActivityAt, now) : "idle"}
              </span>
            </div>
          </div>

          <div className="prompt-section">
            <div className="prompt-header">
              <span>Prompt for agent</span>
              {prompt && (
                <button type="button" className="prompt-copy" onClick={handleCopyPrompt}>
                  {copiedPrompt ? "Copied" : "Copy"}
                </button>
              )}
            </div>
            <textarea
              className="prompt-text"
              readOnly
              value={prompt || "Select a frame or node in Figma to generate a prompt."}
              rows={4}
              onClick={(event) => (event.target as HTMLTextAreaElement).select()}
            />
          </div>
        </>
      )}

      {tab === "settings" && (
        <div className="settings-section">
          <div className="info-section">
            <div className="info-row">
              <span className="info-label">Version</span>
              <span className="info-value">{status.version || "—"}</span>
            </div>
            <div className="info-row">
              <span className="info-label">Server</span>
              <span className="info-value">{WS_BASE_URL}</span>
            </div>
            <div className="info-row">
              <span className="info-label">Uptime</span>
              <span className="info-value">
                {connectedAt ? formatDuration(now - connectedAt) : "—"}
              </span>
            </div>
          </div>

          <button type="button" className="debug-toggle" onClick={() => setDebugOpen((open) => !open)}>
            {debugOpen ? "Hide debug log" : "Show debug log"}
          </button>

          {debugOpen && (
            <div className="debug-panel">
              <div className="debug-panel-header">
                <span>{log.length} entries</span>
                <div className="debug-panel-actions">
                  <button type="button" className="debug-clear" onClick={handleCopyLog} disabled={log.length === 0}>
                    {copiedLog ? "Copied" : "Copy all"}
                  </button>
                  <button type="button" className="debug-clear" onClick={() => setLog([])} disabled={log.length === 0}>
                    Clear
                  </button>
                </div>
              </div>
              <div className="debug-log">
                {log.length === 0 && <div className="debug-empty">No traffic yet.</div>}
                {log.map((entry) => (
                  <div key={entry.id} className={`debug-entry ${entry.direction}`}>
                    <span className="debug-arrow">{entry.direction === "in" ? "↓" : "↑"}</span>
                    <span className="debug-label">{entry.label}</span>
                    <span className="debug-time">{formatRelativeTime(entry.timestamp, now)}</span>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      )}

      <div className="footer">
        <a href={REPO_URL} target="_blank" rel="noopener noreferrer" className="footer-link">
          Figma MAP
        </a>
      </div>
    </div>
  );
}
