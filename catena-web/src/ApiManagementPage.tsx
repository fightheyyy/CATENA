import { useEffect, useMemo, useRef, useState } from "react";
import { api } from "./api";
import { registeredAgentSummaries } from "./agentConnection";
import { copyText } from "./clipboard";
import type { AgentSummary, ApiToken, EvolutionModelSettings, WorkspaceData } from "./types";

type Locale = "zh" | "en";

const apiCopy = {
  zh: {
    title: "API 管理",
    body: "每个 Agent 使用一个独立密钥上传 Trace 或对话。密钥会自动决定数据归属。",
    llmTitle: "LLM 配置",
    llmBody: "Trace Farm 使用你的模型运行 Inspector、Evolution 与 Reviewer。Catena 不提供公共模型，也不会在保存后展示 API Key。",
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
    llmSaved: "LLM 配置已保存。下一次 Trace Farm 任务会立即使用。",
    llmConfigured: "已配置",
    llmMissing: "未配置",
    llmClear: "清除配置",
    llmConfirmClear: "确认清除",
    llmCleared: "LLM 配置已清除。",
    createTitle: "创建 Agent 密钥",
    agentName: "Agent 名称",
    placeholder: "例如：大狗",
    create: "生成密钥",
    creating: "正在生成",
    created: "密钥已生成，可从对应行复制。",
    endpoints: "接收地址",
    otlp: "OTLP Trace",
    conversation: "XiaoBaOS 对话",
    keys: "Agent 密钥",
    empty: "还没有 Agent 密钥。",
    connected: "已连接",
    waiting: "等待数据",
    noKey: "没有密钥",
    copy: "复制",
    copied: "已复制",
    copyOtlp: "复制 OTLP Trace 接收地址",
    copyConversation: "复制 XiaoBaOS 对话接收地址",
    revoke: "删除",
    confirmRevoke: "确认删除",
    recreate: "生成密钥",
  },
  en: {
    title: "API Management",
    body: "Each Agent uses one dedicated key to upload Trace or conversation data. The key determines data ownership.",
    llmTitle: "LLM configuration",
    llmBody: "Trace Farm runs Inspector, Evolution, and Reviewer with your model. Catena does not provide a shared model and never reveals the API key after saving.",
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
    llmSaved: "LLM configuration saved. The next Trace Farm job will use it.",
    llmConfigured: "Configured",
    llmMissing: "Not configured",
    llmClear: "Clear configuration",
    llmConfirmClear: "Confirm clear",
    llmCleared: "LLM configuration cleared.",
    createTitle: "Create an Agent key",
    agentName: "Agent name",
    placeholder: "For example: Big Dog",
    create: "Generate key",
    creating: "Generating",
    created: "Key generated. Copy it from the corresponding row.",
    endpoints: "Ingest endpoints",
    otlp: "OTLP Trace",
    conversation: "XiaoBaOS conversation",
    keys: "Agent keys",
    empty: "No Agent key yet.",
    connected: "Connected",
    waiting: "Waiting for data",
    noKey: "No key",
    copy: "Copy",
    copied: "Copied",
    copyOtlp: "Copy OTLP Trace ingest endpoint",
    copyConversation: "Copy XiaoBaOS conversation ingest endpoint",
    revoke: "Delete",
    confirmRevoke: "Confirm delete",
    recreate: "Generate key",
  },
} as const;

type LocalAgent = {
  summary: AgentSummary;
  credential: ApiToken;
};

export function ApiManagementPage({ locale, workspace, onRefresh }: { locale: Locale; workspace: WorkspaceData; onRefresh: () => Promise<void> }) {
  const t = apiCopy[locale];
  const nameField = useRef<HTMLInputElement>(null);
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [localAgent, setLocalAgent] = useState<LocalAgent | null>(null);
  const [localCredentials, setLocalCredentials] = useState<Record<string, ApiToken>>({});
  const [copiedID, setCopiedID] = useState("");
  const [copiedEndpoint, setCopiedEndpoint] = useState<"otlp" | "conversation" | "">("");
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
  const conversationEndpoint = `${window.location.origin}/v1/ingest/conversations`;

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

  const copyEndpoint = async (kind: "otlp" | "conversation", value: string) => {
    const ok = await copyText(value);
    setCopiedEndpoint(ok ? kind : "");
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
      setName("");
      setMessage(t.created);
      void onRefresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Request failed");
    } finally {
      setBusy(false);
      nameField.current?.focus();
    }
  };

  const copyKey = async (agent: AgentSummary, credential: ApiToken) => {
    setError("");
    try {
      const result = await api.revealApiToken(credential.id);
      const copied = await copyText(result.token);
      setCopiedID(copied ? agent.agent_id : "");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Request failed");
    }
  };

  const createKey = async (agent: AgentSummary) => {
    setError("");
    try {
      const result = await api.createAgentConnectionKey(agent.agent_id);
      setLocalCredentials((current) => ({ ...current, [agent.agent_id]: result.api_token }));
      setRevokedAgentIDs((current) => { const next = new Set(current); next.delete(agent.agent_id); return next; });
      void onRefresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Request failed");
    }
  };

  const revokeKey = async (agent: AgentSummary, credential: ApiToken) => {
    if (confirmID !== credential.id) {
      setConfirmID(credential.id);
      return;
    }
    setError("");
    try {
      await api.deleteApiToken(credential.id);
      setConfirmID("");
      setRevokedAgentIDs((current) => new Set(current).add(agent.agent_id));
      setCopiedID((current) => current === agent.agent_id ? "" : current);
      void onRefresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Request failed");
    }
  };

  return (
    <section className="page api-page">
      <header className="page-header"><h1>{t.title}</h1><p>{t.body}</p></header>

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

      <section className="api-create-section">
        <h2>{t.createTitle}</h2>
        <form className="key-form" onSubmit={(event) => { event.preventDefault(); void createAgent(); }}>
          <label><span>{t.agentName}</span><input ref={nameField} value={name} maxLength={80} autoComplete="off" placeholder={t.placeholder} onChange={(event) => setName(event.target.value)} /></label>
          <button className="primary-button compact" type="submit" disabled={!name.trim() || busy}>{busy ? t.creating : t.create}</button>
          <p className={error ? "key-feedback error" : "key-feedback"} role={error ? "alert" : "status"}>{error || message || " "}</p>
        </form>
      </section>

      <section className="api-endpoints">
        <h2>{t.endpoints}</h2>
        <dl>
          <div>
            <dt>{t.otlp}</dt>
            <dd>
              <code>{otlpEndpoint}</code>
              <button className="endpoint-copy-button" type="button" aria-label={t.copyOtlp} onClick={() => void copyEndpoint("otlp", otlpEndpoint)}>
                {copiedEndpoint === "otlp" ? t.copied : t.copy}
              </button>
            </dd>
          </div>
          <div>
            <dt>{t.conversation}</dt>
            <dd>
              <code>{conversationEndpoint}</code>
              <button className="endpoint-copy-button" type="button" aria-label={t.copyConversation} onClick={() => void copyEndpoint("conversation", conversationEndpoint)}>
                {copiedEndpoint === "conversation" ? t.copied : t.copy}
              </button>
            </dd>
          </div>
        </dl>
      </section>

      <section className="api-key-section">
        <header><h2>{t.keys}</h2><span>{agents.length}</span></header>
        {agents.length === 0 ? <div className="empty-state"><span>{t.empty}</span></div> : <div className="token-list">
          {agents.map((agent) => {
            const credential = credentialFor(agent);
            return <article className="token-row" key={agent.agent_id}>
              <div className="token-identity">
                <strong>{agent.display_name}</strong>
                <code>{credential?.masked_token ?? t.noKey}</code>
                <span>{agent.connected ? t.connected : t.waiting}</span>
              </div>
              <div className="token-actions">
                {credential ? <>
                  <button className="text-button" type="button" onClick={() => void copyKey(agent, credential)}>{copiedID === agent.agent_id ? t.copied : t.copy}</button>
                  <button className="text-button danger" type="button" onClick={() => void revokeKey(agent, credential)}>{confirmID === credential.id ? t.confirmRevoke : t.revoke}</button>
                </> : <button className="text-button" type="button" onClick={() => void createKey(agent)}>{t.recreate}</button>}
              </div>
            </article>;
          })}
        </div>}
      </section>
    </section>
  );
}
