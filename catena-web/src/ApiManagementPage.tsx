import { useEffect, useMemo, useRef, useState } from "react";
import { api } from "./api";
import { registeredAgentSummaries } from "./agentConnection";
import { copyText } from "./clipboard";
import { AgentConnectionGuide } from "./AgentConnectionGuide";
import type { AgentSummary, ApiToken, EvolutionModelSettings, WorkspaceData } from "./types";

type Locale = "zh" | "en";

const apiCopy = {
  zh: {
    title: "连接",
    body: "把日常使用的 Agent 连接到这里，让经历开始积累。",
    llmTitle: "分析模型",
    llmBody: "连接你自己的模型，用于分析 Trace、提炼产出。API Key 保存后不再展示。",
    llmProvider: "Provider",
    llmBaseURL: "Base URL",
    llmModel: "Model",
    llmAPIKey: "API Key",
    llmProviderPlaceholder: "例如：openai",
    llmBaseURLPlaceholder: "https://api.example.com/v1",
    llmModelPlaceholder: "例如：gpt-5.5",
    llmAPIKeyPlaceholder: "输入你自己的 API Key",
    llmAPIKeySaved: "已安全保存；留空不会修改",
    llmSave: "保存配置",
    llmSaving: "正在保存",
    llmSaved: "模型配置已保存，将用于下一次分析。",
    llmConfigured: "已配置",
    llmMissing: "未配置",
    llmClear: "清除配置",
    llmConfirmClear: "确认清除",
    llmCleared: "LLM 配置已清除。",
    createTitle: "接入新 Agent",
    agentName: "Agent 名称",
    placeholder: "例如：我的 Codex",
    create: "生成接入密钥",
    creating: "正在生成",
    created: "密钥已生成，可从对应行复制。",
    endpoints: "接收地址",
    otlp: "OTLP Trace",
    keys: "Agent 接入",
    keysBody: "撤销密钥只会停止后续上传，Agent 与历史数据会继续保留。",
    empty: "还没有接入 Agent。",
    connected: "接入正常",
    waiting: "等待首条数据",
    paused: "接入已停用",
    retained: "历史数据保留",
    noData: "尚未上传数据",
    noKey: "密钥已撤销",
    copy: "复制",
    copied: "已复制",
    copyOtlp: "复制 OTLP Trace 接收地址",
    revoke: "撤销密钥",
    confirmRevoke: "确认撤销",
    recreate: "重新生成",
  },
  en: {
    title: "Connections",
    body: "Connect the Agents you work with and give their history a home.",
    llmTitle: "Analysis model",
    llmBody: "Connect your own model to analyze Traces and create reusable outputs. API keys are never shown after saving.",
    llmProvider: "Provider",
    llmBaseURL: "Base URL",
    llmModel: "Model",
    llmAPIKey: "API key",
    llmProviderPlaceholder: "For example: openai",
    llmBaseURLPlaceholder: "https://api.example.com/v1",
    llmModelPlaceholder: "For example: gpt-5.5",
    llmAPIKeyPlaceholder: "Enter your own API key",
    llmAPIKeySaved: "Stored securely; leave blank to keep it",
    llmSave: "Save configuration",
    llmSaving: "Saving",
    llmSaved: "Model configuration saved for the next analysis.",
    llmConfigured: "Configured",
    llmMissing: "Not configured",
    llmClear: "Clear configuration",
    llmConfirmClear: "Confirm clear",
    llmCleared: "LLM configuration cleared.",
    createTitle: "Connect a new Agent",
    agentName: "Agent name",
    placeholder: "For example: My Codex",
    create: "Generate ingest key",
    creating: "Generating",
    created: "Key generated. Copy it from the corresponding row.",
    endpoints: "Ingest endpoint",
    otlp: "OTLP Trace",
    keys: "Agent connections",
    keysBody: "Revoking a key stops future ingestion. The Agent and its historical data remain available.",
    empty: "No Agent connected yet.",
    connected: "Ingest active",
    waiting: "Waiting for first data",
    paused: "Ingest paused",
    retained: "Historical data retained",
    noData: "No data uploaded yet",
    noKey: "Key revoked",
    copy: "Copy",
    copied: "Copied",
    copyOtlp: "Copy OTLP Trace ingest endpoint",
    revoke: "Revoke key",
    confirmRevoke: "Confirm revoke",
    recreate: "Regenerate",
  },
} as const;

type LocalAgent = {
  summary: AgentSummary;
  credential: ApiToken;
};

export function ApiManagementPage({ locale, workspace, onRefresh, onOpenAgent }: { locale: Locale; workspace: WorkspaceData; onRefresh: () => Promise<void>; onOpenAgent: (agentID: string) => void }) {
  const t = apiCopy[locale];
  const nameField = useRef<HTMLInputElement>(null);
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [localAgent, setLocalAgent] = useState<LocalAgent | null>(null);
  const [setupAgentID, setSetupAgentID] = useState("");
  const [credentialBusy, setCredentialBusy] = useState(false);
  const [localCredentials, setLocalCredentials] = useState<Record<string, ApiToken>>({});
  const [copiedID, setCopiedID] = useState("");
  const [copiedEndpoint, setCopiedEndpoint] = useState(false);
  const [confirmID, setConfirmID] = useState("");
  const [revokedAgentIDs, setRevokedAgentIDs] = useState<Set<string>>(() => new Set());
  const [llm, setLLM] = useState<EvolutionModelSettings>({ provider: "", base_url: "", model: "", api_key_configured: false, configured: false });
  const [llmProvider, setLLMProvider] = useState("openai");
  const [llmBaseURL, setLLMBaseURL] = useState("");
  const [llmModel, setLLMModel] = useState("");
  const [llmAPIKey, setLLMAPIKey] = useState("");
  const [llmBusy, setLLMBusy] = useState(false);
  const [llmMessage, setLLMMessage] = useState("");
  const [llmError, setLLMError] = useState("");
  const [confirmLLMClear, setConfirmLLMClear] = useState(false);
  const otlpEndpoint = `${window.location.origin}/v1/otlp/v1/traces`;

  useEffect(() => {
    let active = true;
    api.llmConfig().then((value) => {
      if (!active) return;
      setLLM(value);
      setLLMProvider(value.provider || "openai");
      setLLMBaseURL(value.base_url || "");
      setLLMModel(value.model || "");
    }).catch((cause) => {
      if (active) setLLMError(cause instanceof Error ? cause.message : "Request failed");
    });
    return () => { active = false; };
  }, []);

  const saveLLM = async () => {
    if (!llmProvider.trim() || !llmBaseURL.trim() || !llmModel.trim() || (!llm.api_key_configured && !llmAPIKey.trim()) || llmBusy) return;
    setLLMBusy(true);
    setLLMError("");
    setLLMMessage("");
    try {
      const value = await api.saveLLMConfig({
        provider: llmProvider.trim(),
        base_url: llmBaseURL.trim(),
        model: llmModel.trim(),
        api_key: llmAPIKey,
      });
      setLLM(value);
      setLLMAPIKey("");
      setLLMMessage(t.llmSaved);
    } catch (cause) {
      setLLMError(cause instanceof Error ? cause.message : "Request failed");
    } finally {
      setLLMBusy(false);
    }
  };

  const clearLLM = async () => {
    if (!confirmLLMClear) {
      setConfirmLLMClear(true);
      return;
    }
    setLLMBusy(true);
    setLLMError("");
    try {
      await api.deleteLLMConfig();
      setLLM({ provider: "", base_url: "", model: "", api_key_configured: false, configured: false });
      setLLMProvider("openai");
      setLLMBaseURL("");
      setLLMModel("");
      setLLMAPIKey("");
      setLLMMessage(t.llmCleared);
      setConfirmLLMClear(false);
    } catch (cause) {
      setLLMError(cause instanceof Error ? cause.message : "Request failed");
    } finally {
      setLLMBusy(false);
    }
  };

  const copyEndpoint = async () => {
    const ok = await copyText(otlpEndpoint);
    setCopiedEndpoint(ok);
    if (!ok) setError(locale === "zh" ? "复制失败，请允许剪贴板访问后重试。" : "Copy failed. Allow clipboard access and retry.");
  };

  const agents = useMemo(() => {
    const current = registeredAgentSummaries(workspace.agents);
    if (!localAgent || current.some((agent) => agent.agent_id === localAgent.summary.agent_id)) return current;
    return [localAgent.summary, ...current];
  }, [localAgent, workspace.agents]);

  const credentialFor = (agent: AgentSummary) => {
    if (revokedAgentIDs.has(agent.agent_id)) return undefined;
    return localCredentials[agent.agent_id] ?? (localAgent?.summary.agent_id === agent.agent_id ? localAgent.credential : agent.credential);
  };
  const setupAgent = agents.find((agent) => agent.agent_id === setupAgentID);
  const setupCredential = setupAgent ? credentialFor(setupAgent) : undefined;

  const createAgent = async () => {
    if (!name.trim() || busy) return;
    setBusy(true);
    setError("");
    setMessage("");
    try {
      const result = await api.createAgent(name.trim());
      const summary: AgentSummary = {
        agent_id: result.agent.agent_id,
        display_name: result.agent.display_name,
        identity_source: "registered",
        runtime_kind: result.agent.runtime_kind,
        registered: true,
        connected: false,
        conversation_count: 0,
        credential: result.api_token,
        trace_count: 0,
        span_count: 0,
        error_count: 0,
        last_seen_at: "",
      };
      setLocalAgent({ summary, credential: result.api_token });
      setSetupAgentID(summary.agent_id);
      setName("");
      setMessage(t.created);
      void onRefresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Request failed");
    } finally {
      setBusy(false);
    }
  };

  const copyKey = async (agent: AgentSummary, credential: ApiToken) => {
    if (credentialBusy) return;
    setCredentialBusy(true);
    setError("");
    try {
      const result = await api.revealApiToken(credential.id);
      const copied = await copyText(result.token);
      setCopiedID(copied ? agent.agent_id : "");
      if (!copied) setError(locale === "zh" ? "复制失败，请允许剪贴板访问后重试。" : "Copy failed. Allow clipboard access and retry.");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Request failed");
    } finally { setCredentialBusy(false); }
  };

  const createKey = async (agent: AgentSummary) => {
    if (credentialBusy) return;
    setCredentialBusy(true);
    setError("");
    try {
      const result = await api.createAgentConnectionKey(agent.agent_id);
      setLocalCredentials((current) => ({ ...current, [agent.agent_id]: result.api_token }));
      setRevokedAgentIDs((current) => { const next = new Set(current); next.delete(agent.agent_id); return next; });
      setSetupAgentID(agent.agent_id);
      void onRefresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Request failed");
    } finally { setCredentialBusy(false); }
  };

  const revokeKey = async (agent: AgentSummary, credential: ApiToken) => {
    if (credentialBusy) return;
    if (confirmID !== credential.id) {
      setConfirmID(credential.id);
      return;
    }
    setError("");
    setCredentialBusy(true);
    try {
      await api.deleteApiToken(credential.id);
      setConfirmID("");
      setRevokedAgentIDs((current) => new Set(current).add(agent.agent_id));
      setCopiedID((current) => current === agent.agent_id ? "" : current);
      void onRefresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Request failed");
    } finally { setCredentialBusy(false); }
  };

  return (
    <section className="page api-page">
      <header className="page-header"><h1>{t.title}</h1></header>

      <section className="api-create-section">
        <h2>{t.createTitle}</h2>
        <form className="key-form" onSubmit={(event) => { event.preventDefault(); void createAgent(); }}>
          <label><span>{t.agentName}</span><input ref={nameField} value={name} maxLength={80} autoComplete="off" placeholder={t.placeholder} onChange={(event) => setName(event.target.value)} /></label>
          <button className="primary-button compact" type="submit" disabled={!name.trim() || busy}>{busy ? t.creating : t.create}</button>
          <p className={error ? "key-feedback error" : "key-feedback"} role={error ? "alert" : "status"}>{error || message || " "}</p>
        </form>
      </section>

      {setupAgent && setupCredential ? <AgentConnectionGuide key={setupAgent.agent_id} agent={setupAgent} credential={setupCredential} locale={locale} onRefresh={onRefresh} onOpen={onOpenAgent} onClose={() => setSetupAgentID("")} /> : null}

      <section className="api-endpoints">
        <h2>{t.endpoints}</h2>
        <dl>
          <div>
            <dt>{t.otlp}</dt>
            <dd>
              <EndpointAddress value={otlpEndpoint} />
              <button className="endpoint-copy-button" type="button" aria-label={t.copyOtlp} onClick={() => void copyEndpoint()}>
                {copiedEndpoint ? t.copied : t.copy}
              </button>
            </dd>
          </div>
        </dl>
      </section>

      <section className="api-key-section">
        <header><div><h2>{t.keys}</h2><p>{t.keysBody}</p></div><span>{agents.length}</span></header>
        {agents.length === 0 ? <div className="empty-state"><span>{t.empty}</span></div> : <div className="token-list">
          {agents.map((agent) => {
            const credential = credentialFor(agent);
            return <article className="token-row" key={agent.agent_id}>
              <div className="token-identity">
                <strong>{agent.display_name}</strong>
                <code>{credential?.masked_token ?? t.noKey}</code>
                <span className={credential ? "token-state active" : "token-state paused"}>
                  <b>{credential ? (agent.connected ? t.connected : t.waiting) : t.paused}</b>
                  {!credential ? <small>{agent.connected ? t.retained : t.noData}</small> : null}
                </span>
              </div>
              <div className="token-actions">
                {credential ? <>
                  <button className="text-button" type="button" onClick={() => setSetupAgentID(agent.agent_id)}>{locale === "zh" ? "接入配置" : "Setup"}</button>
                  <button className="text-button" type="button" disabled={credentialBusy} onClick={() => void copyKey(agent, credential)}>{copiedID === agent.agent_id ? t.copied : t.copy}</button>
                  <button className="text-button danger" type="button" disabled={credentialBusy} onClick={() => void revokeKey(agent, credential)}>{confirmID === credential.id ? t.confirmRevoke : t.revoke}</button>
                  {confirmID === credential.id ? <button className="text-button" type="button" disabled={credentialBusy} onClick={() => setConfirmID("")}>{locale === "zh" ? "取消" : "Cancel"}</button> : null}
                </> : <button className="text-button" type="button" disabled={credentialBusy} onClick={() => void createKey(agent)}>{t.recreate}</button>}
              </div>
            </article>;
          })}
        </div>}
      </section>

      <section className="llm-config-section">
        <header>
          <div><h2>{t.llmTitle}</h2><p>{t.llmBody}</p></div>
          <span className={llm.configured ? "config-state ready" : "config-state"}>{llm.configured ? t.llmConfigured : t.llmMissing}</span>
        </header>
        <form className="llm-form" onSubmit={(event) => { event.preventDefault(); void saveLLM(); }}>
          <label><span>{t.llmProvider}</span><input list="llm-provider-options" value={llmProvider} maxLength={128} autoComplete="off" placeholder={t.llmProviderPlaceholder} onChange={(event) => setLLMProvider(event.target.value)} /></label>
          <datalist id="llm-provider-options"><option value="openai" /><option value="anthropic" /><option value="google" /></datalist>
          <label className="llm-base-url-field"><span>{t.llmBaseURL}</span><input type="url" value={llmBaseURL} maxLength={1000} autoComplete="url" placeholder={t.llmBaseURLPlaceholder} onChange={(event) => setLLMBaseURL(event.target.value)} /></label>
          <label><span>{t.llmModel}</span><input value={llmModel} maxLength={240} autoComplete="off" placeholder={t.llmModelPlaceholder} onChange={(event) => setLLMModel(event.target.value)} /></label>
          <label><span>{t.llmAPIKey}</span><input type="password" value={llmAPIKey} maxLength={16384} autoComplete="new-password" placeholder={llm.api_key_configured ? t.llmAPIKeySaved : t.llmAPIKeyPlaceholder} onChange={(event) => setLLMAPIKey(event.target.value)} /></label>
          <div className="llm-form-actions">
            <button className="primary-button compact" type="submit" disabled={llmBusy || !llmProvider.trim() || !llmBaseURL.trim() || !llmModel.trim() || (!llm.api_key_configured && !llmAPIKey.trim())}>{llmBusy ? t.llmSaving : t.llmSave}</button>
            {llm.configured ? <button className={confirmLLMClear ? "text-button danger" : "text-button"} type="button" disabled={llmBusy} onClick={() => void clearLLM()}>{confirmLLMClear ? t.llmConfirmClear : t.llmClear}</button> : null}
          </div>
          <p className={llmError ? "llm-feedback error" : "llm-feedback"} role={llmError ? "alert" : "status"}>{llmError || llmMessage || " "}</p>
        </form>
      </section>

    </section>
  );
}

function EndpointAddress({ value }: { value: string }) {
  const origin = new URL(value).origin;
  return <code>{origin}<wbr />{value.slice(origin.length)}</code>;
}
