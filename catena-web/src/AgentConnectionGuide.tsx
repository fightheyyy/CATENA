import { useEffect, useRef, useState } from "react";
import { api } from "./api";
import { agentConnectionConfig, type ConnectionShell } from "./agentConnection";
import { copyText } from "./clipboard";
import type { AgentSummary, ApiToken } from "./types";

const labels = {
  zh: {
    title: "配置 Agent 接入", shell: "终端类型", copy: "复制完整配置", copied: "配置已复制", copying: "正在复制",
    hidden: "预览使用密钥占位符；复制时会填入这个 Agent 的真实密钥。",
    run: "在运行 Agent 的终端中执行配置，重启支持 OTLP 的 Agent，再实际执行一次任务。仅复制配置不会产生数据。",
    capture: "Codex CLI 和 Claude Code 的完整执行过程需要安装 Runtime 采集插件。",
    docs: "查看采集接入说明", waiting: "等待首条 Trace 或对话", checking: "正在检测接入状态",
    connected: "已收到这个 Agent 的数据", timeout: "暂未收到数据。请检查接收地址、密钥和 Agent 的 OTLP 配置，然后重新检测。",
    failed: "检测失败，尚未确认接入状态", retry: "重新检测", open: "查看 Agent", close: "收起配置",
    copyFailed: "复制失败，请允许浏览器访问剪贴板后重试。",
  },
  en: {
    title: "Configure Agent ingestion", shell: "Terminal", copy: "Copy full configuration", copied: "Configuration copied", copying: "Copying",
    hidden: "The preview uses a placeholder. Copying inserts this Agent's actual credential.",
    run: "Run this configuration in the Agent's terminal, restart an OTLP-capable Agent, then perform a task. Copying alone does not send evidence.",
    capture: "Full Codex CLI and Claude Code execution capture requires the Runtime plugin.",
    docs: "Read capture setup", waiting: "Waiting for the first Trace or conversation", checking: "Checking ingestion status",
    connected: "Evidence received from this Agent", timeout: "No evidence received yet. Check the endpoint, key and Agent OTLP configuration, then check again.",
    failed: "Could not confirm ingestion status", retry: "Check again", open: "View Agent", close: "Close configuration",
    copyFailed: "Copy failed. Allow clipboard access and try again.",
  },
} as const;

export function AgentConnectionGuide({ agent, credential, locale, onRefresh, onOpen, onClose }: {
  agent: AgentSummary; credential: ApiToken; locale: "zh" | "en";
  onRefresh: () => Promise<void>; onOpen: (agentID: string) => void; onClose: () => void;
}) {
  const t = labels[locale];
  const heading = useRef<HTMLHeadingElement>(null);
  const [shell, setShell] = useState<ConnectionShell>(navigator.platform.startsWith("Win") ? "powershell" : "sh");
  const [state, setState] = useState<"checking" | "waiting" | "connected" | "timeout" | "failed">("checking");
  const [error, setError] = useState("");
  const [retry, setRetry] = useState(0);
  const [copying, setCopying] = useState(false);
  const [copied, setCopied] = useState(false);
  const preview = agentConnectionConfig(window.location.origin, agent.display_name, shell);

  useEffect(() => { heading.current?.focus(); }, []);

  useEffect(() => {
    const controller = new AbortController();
    const deadline = Date.now() + 120000;
    let timer = 0;
    setState("checking");
    const check = async () => {
      if (Date.now() >= deadline) { setState("timeout"); return; }
      if (document.hidden) { timer = window.setTimeout(() => void check(), 3000); return; }
      try {
        const result = await api.registeredAgent(agent.agent_id, controller.signal);
        if (controller.signal.aborted) return;
        if (result.connected) { setState("connected"); void onRefresh(); return; }
        setState("waiting");
        timer = window.setTimeout(() => void check(), 3000);
      } catch {
        if (!controller.signal.aborted) setState("failed");
      }
    };
    void check();
    return () => { controller.abort(); window.clearTimeout(timer); };
  }, [agent.agent_id, credential.id, retry, onRefresh]);

  const copyConfig = async () => {
    if (copying) return;
    setCopying(true); setCopied(false); setError("");
    try {
      const result = await api.revealApiToken(credential.id);
      if (!await copyText(agentConnectionConfig(window.location.origin, agent.display_name, shell, result.token))) {
        throw new Error(t.copyFailed);
      }
      setCopied(true);
    } catch (cause) { setError(cause instanceof Error ? cause.message : t.copyFailed); }
    finally { setCopying(false); }
  };

  return <section className="agent-connection-guide" aria-labelledby="connection-guide-title">
    <header><div><h2 id="connection-guide-title" ref={heading} tabIndex={-1}>{t.title} · {agent.display_name}</h2><p>{t.hidden}</p></div>
      <button className="text-button" type="button" onClick={onClose}>{t.close}</button></header>
    <div className="connection-config-actions"><label>{t.shell}<select value={shell} onChange={(event) => { setShell(event.target.value as ConnectionShell); setCopied(false); }}>
      <option value="sh">macOS / Linux (sh)</option><option value="powershell">Windows (PowerShell)</option>
    </select></label><button className="secondary-button" type="button" disabled={copying} onClick={() => void copyConfig()}>{copying ? t.copying : copied ? t.copied : t.copy}</button></div>
    <pre className="connection-config-preview"><code>{preview}</code></pre>
    {error ? <p className="inline-note error" role="alert">{error}</p> : null}
    <p>{t.run}</p><p>{t.capture} <a href="https://github.com/fightheyyy/CATENA/tree/main/tap#readme" target="_blank" rel="noreferrer">{t.docs}</a></p>
    <div className={`connection-check ${state}`} role="status"><strong>{t[state]}</strong>
      {state === "connected" ? <button className="primary-button compact" type="button" onClick={() => onOpen(agent.agent_id)}>{t.open}</button>
        : state === "failed" || state === "timeout" ? <button className="secondary-button" type="button" onClick={() => setRetry((value) => value + 1)}>{t.retry}</button> : null}
    </div>
  </section>;
}
