import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { api } from "./api";
import type { MemoryVisualNode } from "./memoryGraph";
import { primaryNavigationRoutes } from "./navigation";
import { isMemoryTaskActive, memoryTaskDisplayPercent } from "./memoryTaskView";
import type { EvolutionJob, MemoryFactGraph, MemoryRecallBundle, MemoryRecallItem, MemoryRecord, MemoryTaskRecord, Session, WorkspaceData } from "./types";

const AgentWorkspace = lazy(() => import("./AgentWorkspace").then((module) => ({ default: module.AgentWorkspace })));
const ApiManagementPage = lazy(() => import("./ApiManagementPage").then((module) => ({ default: module.ApiManagementPage })));
const ConversationWorkspace = lazy(() => import("./ConversationWorkspace").then((module) => ({ default: module.ConversationWorkspace })));
const EvolutionWorkspace = lazy(() => import("./EvolutionWorkspace").then((module) => ({ default: module.EvolutionWorkspace })));
const MemoryGraphCanvas = lazy(() => import("./MemoryGraphCanvas").then((module) => ({ default: module.MemoryGraphCanvas })));
const TraceExplorer = lazy(() => import("./TraceExplorer").then((module) => ({ default: module.TraceExplorer })));

type Route = "home" | "agents" | "apiKeys" | "conversations" | "traces" | "evolution" | "memory" | "settings";
type Locale = "zh" | "en";
type Theme = "system" | "light" | "dark";

const routePaths: Record<Route, string> = {
  home: "/",
  agents: "/agents",
  apiKeys: "/api-keys",
  conversations: "/conversations",
  traces: "/traces",
  evolution: "/evolution",
  memory: "/memory",
  settings: "/settings",
};

const copy = {
  zh: {
    nav: { home: "首页", agents: "Agent", apiKeys: "API 管理", conversations: "对话", traces: "Trace", evolution: "Trace Farm", memory: "记忆", settings: "设置" },
    signIn: "使用 GitHub 登录",
    oauthFlowExpired: "登录流程已过期或从另一个地址发起。请重新登录，Catena 会自动使用正确的回调地址。",
    oauthUpstreamUnavailable: "GitHub 连接暂时超时，请重新登录。Catena 不会保留失败的授权流程。",
    oauthCancelled: "GitHub 授权已取消，你可以随时重新登录。",
    landingTitle: "让 Agent 的每次变化，都有证据。",
    landingBody: "汇聚不同 Agent 的 Trace，用内置 XiaoBaOS 持续提炼 agent.md、Skill、Role 与 Harness 优化。",
    landingNote: "跨 Agent 证据。持续提炼。可复用资产。",
    homeTitle: "今天的 Agent 状态",
    homeBody: "汇聚不同 Agent 的 Trace，由 XiaoBaOS 提炼可复用的 Agent 资产。",
    agentsTitle: "Agent",
    agentsBody: "这里只展示被观测和进化的目标 Agent；编排器与评测角色保留在 Trace 执行链中。",
    tracesTitle: "Trace",
    tracesBody: "Trace 是运行证据，不是日志堆。工具调用、结果与产物会保留同一条因果链。",
    memoryTitle: "记忆",
    memoryBody: "把 XiaoBaOS 的用户可见对话提炼为可追溯记忆，并通过语义、关系与时间三条路径召回。",
    memoryConnected: "记忆能力已就绪",
    memoryConnectedBody: "记忆按当前空间独立保存，并保留来源对话。",
    memoryTasks: "提炼任务",
    memoryTasksHint: "离开对话后任务仍会继续；这里保留最近状态和进度。",
    memoryTaskEmpty: "还没有提炼任务。请从一段对话中开始提炼。",
    memoryTaskPending: "等待处理",
    memoryTaskProcessing: "正在提炼",
    memoryTaskCompleted: "提炼完成",
    memoryTaskFailed: "提炼失败",
    memoryTaskUnknownSource: "未命名对话",
    semanticRecall: "语义召回",
    semanticRecallBody: "从事实、原始对话和主题中寻找相关内容。",
    graphRecall: "关系扩展",
    graphRecallBody: "沿事实之间的关系补充相关上下文。",
    temporalRecall: "时间扩展",
    temporalRecallBody: "恢复相邻轮次和事件顺序。",
    memoryUnavailable: "记忆能力尚未就绪",
    memoryUnavailableBody: "对话记录会继续保存；记忆服务配置完成后，即可从对话中提炼并召回长期记忆。",
    memoryQuery: "你想让 Agent 回忆什么？",
    memoryPlaceholder: "例如：上次发布为什么失败？",
    memorySearch: "召回",
    memorySearching: "正在召回",
    memoryResults: "召回结果",
    recentMemories: "最近记忆",
    memoryCount: "条长期记忆",
    noMemories: "还没有长期记忆。打开一段对话，选择“提炼为记忆”。",
    memoryLoadFailed: "无法读取记忆",
    memoryRecallFailed: "暂时无法召回，请稍后重试或联系管理员检查记忆配置。",
    memoryGraph: "记忆关系图",
    memoryGraphHint: "点击 Fact 查看它和实体、相关事实的真实关系。",
    memoryGraphFailed: "暂时无法读取这条记忆的关系图。",
    memoryGraphEmpty: "从一段对话提炼记忆后，这里会出现可探索的关系图。",
    memoryInspector: "当前节点",
    memorySelectHint: "选择图中的节点查看完整内容和来源。",
    memoryEntities: "实体",
    memoryRelations: "关系",
    relationType: "关系类型",
    confidence: "置信度",
    graphFact: "记忆事实",
    graphEntity: "实体",
    graphRelated: "相关事实",
    recallContext: "召回上下文",
    recallNotRun: "搜索后，这里会显示关系扩展、时间扩展与查询耗时。",
    graphExpanded: "关系扩展事实",
    temporalExpanded: "时间扩展事实",
    searchTime: "查询耗时",
    sourceConversation: "来源对话",
    noMemoryResults: "没有找到相关记忆。",
    recallFact: "Fact",
    recallConversation: "原始对话",
    recallTopic: "Topic",
    settingsTitle: "设置",
    settingsBody: "管理界面语言、显示主题与当前登录会话。Agent 和 LLM 接入统一在 API 管理中维护。",
    language: "语言",
    languageBody: "选择 Catena 的界面语言。",
    theme: "主题",
    themeBody: "跟随系统，或固定使用浅色与深色外观。",
    themeSystem: "跟随系统",
    themeLight: "浅色",
    themeDark: "深色",
    retry: "重试",
    account: "账户",
    accountSettings: "账户设置",
    switchAccount: "切换账户",
    signOut: "退出登录",
    loading: "正在读取 Catena 状态",
    loadFailed: "无法读取平台状态",
    blocked: "需要处理",
    agentMetric: "已观测 Agent",
    runtime: "Runtime",
    runs: "运行",
    issues: "问题",
    cases: "回归 Case",
    releases: "发布结论",
    recentRuns: "最近运行",
    noRuns: "还没有运行证据。接入 Agent 后，从一次 Explore 开始。",
    noRuntime: "XiaoBaOS 进化大脑尚未就绪。",
    observedAgents: "已观测 Agent",
    cloudRuntime: "XiaoBaOS 进化大脑",
    noAgents: "还没有接入 Agent。先到 Agent 页面输入名称并生成专属接入密钥。",
    traceUnavailable: "Trace 存储尚未配置",
    traceUnavailableBody: "为 Catena Server 配置 ClickHouse 后，Go 会直接接收和查询 OTLP Trace。",
    noTraces: "等待第一条 Trace",
    noTracesBody: "使用 Agent 的专属接入密钥作为 Bearer Token，把 OTLP/HTTP exporter 指向这个地址。",
    traceEndpoint: "OTLP Endpoint",
    spans: "Span",
    errors: "错误",
    duration: "耗时",
    close: "关闭",
    traceLoadFailed: "Trace 详情读取失败",
    input: "输入",
    output: "输出",
    noEvidence: "这个 Span 没有导出输入或输出证据。",
    metadataOnly: "这个 Runtime 只导出了工具名与状态，没有输入或输出正文。",
    statusReady: "可用",
    statusUnavailable: "不可用",
    latest: "最近更新",
  },
  en: {
    nav: { home: "Home", agents: "Agents", apiKeys: "API Management", conversations: "Conversations", traces: "Traces", evolution: "Trace Farm", memory: "Memory", settings: "Settings" },
    signIn: "Continue with GitHub",
    oauthFlowExpired: "This sign-in flow expired or started on another address. Restart it and Catena will use the canonical callback origin.",
    oauthUpstreamUnavailable: "GitHub temporarily timed out. Restart sign-in; Catena does not retain the failed authorization flow.",
    oauthCancelled: "GitHub authorization was cancelled. You can restart sign-in at any time.",
    landingTitle: "Evidence for every Agent change.",
    landingBody: "Unify Traces across Agents and let built-in XiaoBaOS continuously distill agent.md, Skill, Role, and Harness improvements.",
    landingNote: "Cross-Agent evidence. Continuous distillation. Reusable assets.",
    homeTitle: "Your Agent state today",
    homeBody: "Unify Traces across Agents and distill reusable Agent assets with XiaoBaOS.",
    agentsTitle: "Agents",
    agentsBody: "Only target Agents appear here. Orchestrators and evaluator roles stay in the Trace execution chain.",
    tracesTitle: "Traces",
    tracesBody: "A Trace is causal evidence, not a log pile. Tool calls, results, and artifacts stay linked.",
    memoryTitle: "Memory",
    memoryBody: "Distill user-visible XiaoBaOS Conversations into provenance-bearing memory and recall it through semantic, graph, and temporal paths.",
    memoryConnected: "Memory is ready",
    memoryConnectedBody: "Memory is isolated to the current space and keeps its source Conversation.",
    memoryTasks: "Distillation tasks",
    memoryTasksHint: "Tasks continue after you leave a Conversation. Recent status and progress stay visible here.",
    memoryTaskEmpty: "No distillation task yet. Start one from a Conversation.",
    memoryTaskPending: "Waiting",
    memoryTaskProcessing: "Distilling",
    memoryTaskCompleted: "Completed",
    memoryTaskFailed: "Failed",
    memoryTaskUnknownSource: "Untitled Conversation",
    semanticRecall: "Semantic recall",
    semanticRecallBody: "Find related facts, original conversations, and topics.",
    graphRecall: "Graph expansion",
    graphRecallBody: "Follow relationships between facts to recover context.",
    temporalRecall: "Temporal expansion",
    temporalRecallBody: "Recover adjacent turns and event order.",
    memoryUnavailable: "Memory is not ready",
    memoryUnavailableBody: "Conversations will keep syncing. Complete the memory setup to distill and recall long-term memory from them.",
    memoryQuery: "What should your Agent remember?",
    memoryPlaceholder: "Example: Why did the last release fail?",
    memorySearch: "Recall",
    memorySearching: "Recalling",
    memoryResults: "Recall results",
    recentMemories: "Recent memory",
    memoryCount: "long-term memories",
    noMemories: "No long-term memory yet. Open a Conversation and choose Distill to memory.",
    memoryLoadFailed: "Could not read memory",
    memoryRecallFailed: "Recall is temporarily unavailable. Retry later or ask an administrator to check the memory setup.",
    memoryGraph: "Memory graph",
    memoryGraphHint: "Select a Fact to inspect its real entities and related facts.",
    memoryGraphFailed: "This memory graph is temporarily unavailable.",
    memoryGraphEmpty: "Distill a Conversation and its explorable relationship graph will appear here.",
    memoryInspector: "Selected node",
    memorySelectHint: "Select a graph node to inspect its full content and provenance.",
    memoryEntities: "Entities",
    memoryRelations: "Relations",
    relationType: "Relation type",
    confidence: "Confidence",
    graphFact: "Memory fact",
    graphEntity: "Entity",
    graphRelated: "Related fact",
    recallContext: "Recall context",
    recallNotRun: "Run a search to see graph expansion, temporal expansion, and query time.",
    graphExpanded: "Graph-expanded facts",
    temporalExpanded: "Time-expanded facts",
    searchTime: "Query time",
    sourceConversation: "Source Conversation",
    noMemoryResults: "No related memory was found.",
    recallFact: "Fact",
    recallConversation: "Conversation",
    recallTopic: "Topic",
    settingsTitle: "Settings",
    settingsBody: "Manage language, appearance, and the current session. Agent and LLM connections live in API Management.",
    language: "Language",
    languageBody: "Choose the language used by Catena.",
    theme: "Theme",
    themeBody: "Follow the system or keep a fixed light or dark appearance.",
    themeSystem: "System",
    themeLight: "Light",
    themeDark: "Dark",
    retry: "Retry",
    account: "Account",
    accountSettings: "Account settings",
    switchAccount: "Switch account",
    signOut: "Sign out",
    loading: "Reading Catena state",
    loadFailed: "Could not load platform state",
    blocked: "Action needed",
    agentMetric: "Observed Agents",
    runtime: "Runtime",
    runs: "Runs",
    issues: "Issues",
    cases: "Regression cases",
    releases: "Release decisions",
    recentRuns: "Recent runs",
    noRuns: "No execution evidence yet. Connect an Agent and begin with Explore.",
    noRuntime: "The XiaoBaOS evolution brain is not ready.",
    observedAgents: "Observed Agents",
    cloudRuntime: "XiaoBaOS evolution brain",
    noAgents: "No Agent yet. Open Agents, name it, and generate its dedicated connection key.",
    traceUnavailable: "Trace storage is not configured",
    traceUnavailableBody: "Configure ClickHouse for Catena Server. Go then receives and queries OTLP Trace directly.",
    noTraces: "Waiting for the first Trace",
    noTracesBody: "Use the Agent's connection key as the Bearer Token and point the OTLP/HTTP exporter to this endpoint.",
    traceEndpoint: "OTLP Endpoint",
    spans: "Spans",
    errors: "Errors",
    duration: "Duration",
    close: "Close",
    traceLoadFailed: "Could not read Trace detail",
    input: "Input",
    output: "Output",
    noEvidence: "This Span exported no input or output evidence.",
    metadataOnly: "This Runtime exported the tool name and status, but no input or output content.",
    statusReady: "Ready",
    statusUnavailable: "Unavailable",
    latest: "Latest update",
  },
} as const;

function currentRoute(): Route {
  const match = (Object.entries(routePaths) as [Route, string][]).find(([, path]) => path === window.location.pathname);
  return match?.[0] ?? "home";
}

function useRoute() {
  const [route, setRoute] = useState<Route>(currentRoute);
  useEffect(() => {
    const onPopState = () => setRoute(currentRoute());
    window.addEventListener("popstate", onPopState);
    return () => window.removeEventListener("popstate", onPopState);
  }, []);
  const navigate = useCallback((next: Route) => {
    window.history.pushState({}, "", routePaths[next]);
    setRoute(next);
    window.scrollTo({ top: 0, behavior: "instant" });
  }, []);
  return { route, navigate };
}

export function App() {
  const [locale, setLocale] = useState<Locale>(() => (localStorage.getItem("catena.locale") === "en" ? "en" : "zh"));
  const [theme, setTheme] = useState<Theme>(() => {
    const saved = localStorage.getItem("catena.theme");
    return saved === "light" || saved === "dark" ? saved : "system";
  });
  const [session, setSession] = useState<Session | null>(null);
  const [workspace, setWorkspace] = useState<WorkspaceData | null>(null);
  const [selectedEvolutionJobID, setSelectedEvolutionJobID] = useState("");
  const [selectedEvolutionAgentID, setSelectedEvolutionAgentID] = useState("");
  const [selectedTraceAgentID, setSelectedTraceAgentID] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const { route, navigate } = useRoute();
  const t = copy[locale];

  const changeLocale = () => {
    const next = locale === "zh" ? "en" : "zh";
    localStorage.setItem("catena.locale", next);
    setLocale(next);
  };

  const selectLocale = (next: Locale) => {
    localStorage.setItem("catena.locale", next);
    setLocale(next);
  };

  const selectTheme = (next: Theme) => {
    localStorage.setItem("catena.theme", next);
    setTheme(next);
  };

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
  }, [theme]);

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const nextSession = await api.session();
      setSession(nextSession);
      if (nextSession.authenticated) {
        setWorkspace(await api.workspace());
      } else {
        setWorkspace(null);
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Unknown request error");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const openEvolutionJob = useCallback((job: EvolutionJob) => {
    setWorkspace((current) => current ? {
      ...current,
      evolutionJobs: [job, ...current.evolutionJobs.filter((item) => item.job_id !== job.job_id)],
    } : current);
    setSelectedEvolutionJobID(job.job_id);
    if (job.source_agent_id) setSelectedEvolutionAgentID(job.source_agent_id);
    navigate("evolution");
  }, [navigate]);

  const analyzeAgent = useCallback((agentID: string) => {
    setSelectedEvolutionAgentID(agentID);
    setSelectedEvolutionJobID("");
    navigate("evolution");
  }, [navigate]);

  const selectEvolutionJob = useCallback((jobID: string) => {
    setSelectedEvolutionJobID(jobID);
  }, []);

  const removeEvolutionJob = useCallback((jobID: string) => {
    setWorkspace((current) => current ? {
      ...current,
      evolutionJobs: current.evolutionJobs.filter((item) => item.job_id !== jobID),
    } : current);
    setSelectedEvolutionJobID("");
  }, []);

  const openAgentTraces = useCallback((agentID: string) => {
    setSelectedTraceAgentID(agentID);
    navigate("traces");
  }, [navigate]);

  const navigateFromSidebar = useCallback((next: Route) => {
    if (next === "evolution") {
      setSelectedEvolutionJobID("");
      setSelectedEvolutionAgentID("");
    }
    navigate(next);
  }, [navigate]);

  if (loading && session === null) {
    return <LoadingPage label={t.loading} />;
  }

  if (error && session === null) {
    return <FailurePage title={t.loadFailed} detail={error} action={t.retry} onRetry={load} />;
  }

  if (!session?.authenticated) {
    return (
      <Landing
        locale={locale}
        onLocale={changeLocale}
        loginURL={session?.login_url ?? "/v1/auth/github"}
        oauthError={new URLSearchParams(window.location.search).get("auth_error") ?? ""}
      />
    );
  }

  const logout = async () => {
    await api.logout();
    window.location.assign("/");
  };

  const switchAccount = async () => {
    await api.logout();
    window.location.assign("/v1/auth/github");
  };

  return (
    <div className="app-shell">
      <Sidebar
        route={route}
        locale={locale}
        session={session}
        onNavigate={navigateFromSidebar}
        onSettings={() => navigate("settings")}
        onSwitchAccount={switchAccount}
        onLogout={logout}
      />
      <main className="main-canvas">
        {error ? <InlineError message={error} action={t.retry} onRetry={load} /> : null}
        {workspace ? (
          <RouteView
            route={route}
            locale={locale}
            workspace={workspace}
            session={session}
            selectedEvolutionJobID={selectedEvolutionJobID}
            selectedEvolutionAgentID={selectedEvolutionAgentID}
            selectedTraceAgentID={selectedTraceAgentID}
            onOpenEvolutionJob={openEvolutionJob}
            onSelectEvolutionJob={selectEvolutionJob}
            onDeleteEvolutionJob={removeEvolutionJob}
            onAnalyzeAgent={analyzeAgent}
            onOpenAgentTraces={openAgentTraces}
            onConnectAgent={() => navigate("apiKeys")}
            onOpenMemory={() => navigate("memory")}
            onRefresh={load}
            theme={theme}
            onLocale={selectLocale}
            onTheme={selectTheme}
            onLogout={logout}
          />
        ) : (
          <LoadingPanel label={t.loading} />
        )}
      </main>
    </div>
  );
}

function AccountMenu({
  locale,
  session,
  onSettings,
  onSwitchAccount,
  onLogout,
}: {
  locale: Locale;
  session: Session;
  onSettings: () => void;
  onSwitchAccount: () => Promise<void>;
  onLogout: () => Promise<void>;
}) {
  const t = copy[locale];
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const menuRef = useRef<HTMLDivElement>(null);
  const user = session.user;
  const displayName = user?.display_name || user?.login || (locale === "zh" ? "本地账户" : "Local account");
  const initial = displayName.trim().slice(0, 1).toUpperCase() || "C";

  useEffect(() => {
    if (!open) return;
    const closeOnOutside = (event: MouseEvent) => {
      if (!menuRef.current?.contains(event.target as Node)) setOpen(false);
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") setOpen(false);
    };
    document.addEventListener("mousedown", closeOnOutside);
    document.addEventListener("keydown", closeOnEscape);
    return () => {
      document.removeEventListener("mousedown", closeOnOutside);
      document.removeEventListener("keydown", closeOnEscape);
    };
  }, [open]);

  const runAccountAction = async (action: () => Promise<void>) => {
    setBusy(true);
    setError("");
    try {
      await action();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : (locale === "zh" ? "账户操作失败" : "Account action failed"));
      setBusy(false);
    }
  };

  return (
    <div className="account-menu" ref={menuRef}>
      <button
        className="account-trigger"
        type="button"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label={`${t.account}: ${displayName}`}
        onClick={() => {
          setError("");
          setOpen((value) => !value);
        }}
      >
        <span className="account-avatar">
          {user?.avatar_url ? <img src={user.avatar_url} alt="" referrerPolicy="no-referrer" /> : initial}
        </span>
        <span className="account-trigger-compact">{t.account}</span>
        <span className="account-trigger-name">{displayName}</span>
        <span className="account-chevron" aria-hidden="true">⌄</span>
      </button>
      {open ? (
        <div className="account-popover" role="menu">
          <div className="account-identity">
            <span className="account-avatar large">
              {user?.avatar_url ? <img src={user.avatar_url} alt="" referrerPolicy="no-referrer" /> : initial}
            </span>
            <span><strong>{displayName}</strong>{user?.login ? <small>@{user.login}</small> : null}</span>
          </div>
          <div className="account-actions">
            <button type="button" role="menuitem" onClick={() => { setOpen(false); onSettings(); }}>{t.accountSettings}</button>
            {session.mode === "github" ? (
              <button type="button" role="menuitem" disabled={busy} onClick={() => void runAccountAction(onSwitchAccount)}>{t.switchAccount}</button>
            ) : null}
            {session.mode === "github" ? (
              <button className="danger" type="button" role="menuitem" disabled={busy} onClick={() => void runAccountAction(onLogout)}>{t.signOut}</button>
            ) : null}
          </div>
          {error ? <p className="account-error" role="alert">{error}</p> : null}
        </div>
      ) : null}
    </div>
  );
}

function Landing({ locale, onLocale, loginURL, oauthError }: { locale: Locale; onLocale: () => void; loginURL: string; oauthError: string }) {
  const t = copy[locale];
  const oauthErrorMessage = oauthError === "upstream"
    ? t.oauthUpstreamUnavailable
    : oauthError === "cancelled"
      ? t.oauthCancelled
      : oauthError
        ? t.oauthFlowExpired
        : "";
  return (
    <main className="landing">
      <header className="landing-nav">
        <Brand />
        <button className="text-button" type="button" onClick={onLocale}>
          {locale === "zh" ? "EN" : "中文"}
        </button>
      </header>
      <section className="landing-hero">
        <div className="landing-copy">
          <h1>{t.landingTitle}</h1>
          <p>{t.landingBody}</p>
          {oauthErrorMessage ? <p className="landing-auth-error" role="alert">{oauthErrorMessage}</p> : null}
          <a className="primary-button" href={loginURL}>{t.signIn}</a>
        </div>
        <div className="landing-mark" aria-hidden="true">
          <img src="/catena-mark.svg" alt="" />
        </div>
      </section>
      <footer className="landing-footer">
        <span>{t.landingNote}</span>
        <a href="https://github.com/fightheyyy/CATENA">GitHub</a>
      </footer>
    </main>
  );
}

function Brand() {
  return (
    <a className="brand" href="/" aria-label="Catena home">
      <img src="/catena-mark.svg" alt="" />
      <span>CATENA</span>
    </a>
  );
}

function Sidebar({
  route,
  locale,
  session,
  onNavigate,
  onSettings,
  onSwitchAccount,
  onLogout,
}: {
  route: Route;
  locale: Locale;
  session: Session;
  onNavigate: (route: Route) => void;
  onSettings: () => void;
  onSwitchAccount: () => Promise<void>;
  onLogout: () => Promise<void>;
}) {
  const t = copy[locale];
  return (
    <aside className="sidebar">
      <Brand />
      <nav aria-label="Product">
        {primaryNavigationRoutes.map((item) => (
          <button
            className={route === item ? "nav-item active" : "nav-item"}
            key={item}
            type="button"
            onClick={() => onNavigate(item)}
          >
            {t.nav[item]}
          </button>
        ))}
      </nav>
      <div className="sidebar-footer">
        <div className="sidebar-utilities">
          <button className={route === "apiKeys" ? "nav-item active" : "nav-item"} type="button" onClick={() => onNavigate("apiKeys")}>{t.nav.apiKeys}</button>
          <button className={route === "settings" ? "nav-item active" : "nav-item"} type="button" onClick={() => onNavigate("settings")}>{t.nav.settings}</button>
        </div>
        <AccountMenu
          locale={locale}
          session={session}
          onSettings={onSettings}
          onSwitchAccount={onSwitchAccount}
          onLogout={onLogout}
        />
      </div>
    </aside>
  );
}

function RouteView({
  route,
  locale,
  workspace,
  session,
  selectedEvolutionJobID,
  selectedEvolutionAgentID,
  selectedTraceAgentID,
  onOpenEvolutionJob,
  onSelectEvolutionJob,
  onDeleteEvolutionJob,
  onAnalyzeAgent,
  onOpenAgentTraces,
  onConnectAgent,
  onOpenMemory,
  onRefresh,
  theme,
  onLocale,
  onTheme,
  onLogout,
}: {
  route: Route;
  locale: Locale;
  workspace: WorkspaceData;
  session: Session;
  selectedEvolutionJobID: string;
  selectedEvolutionAgentID: string;
  selectedTraceAgentID: string;
  onOpenEvolutionJob: (job: EvolutionJob) => void;
  onSelectEvolutionJob: (jobID: string) => void;
  onDeleteEvolutionJob: (jobID: string) => void;
  onAnalyzeAgent: (agentID: string) => void;
  onOpenAgentTraces: (agentID: string) => void;
  onConnectAgent: () => void;
  onOpenMemory: () => void;
  onRefresh: () => Promise<void>;
  theme: Theme;
  onLocale: (locale: Locale) => void;
  onTheme: (theme: Theme) => void;
  onLogout: () => void;
}) {
  let content: React.ReactNode;
  if (route === "agents") content = <AgentWorkspace locale={locale} workspace={workspace} onAnalyze={onAnalyzeAgent} onOpenTraces={onOpenAgentTraces} onConnect={onConnectAgent} />;
  else if (route === "apiKeys") content = <ApiManagementPage locale={locale} workspace={workspace} onRefresh={onRefresh} />;
  else if (route === "conversations") content = <ConversationWorkspace locale={locale} memoryReady={workspace.system.memory_store === "available"} onOpenMemory={onOpenMemory} />;
  else if (route === "traces") content = <TraceExplorer locale={locale} workspace={workspace} initialAgentID={selectedTraceAgentID} />;
  else if (route === "evolution") content = (
    <EvolutionWorkspace
      locale={locale}
      jobs={workspace.evolutionJobs}
      agents={workspace.agents}
      initialJobID={selectedEvolutionJobID}
      initialAgentID={selectedEvolutionAgentID}
      onJobStarted={onOpenEvolutionJob}
      onJobSelected={onSelectEvolutionJob}
      onJobDeleted={onDeleteEvolutionJob}
    />
  );
  else if (route === "memory") content = <Memory locale={locale} workspace={workspace} />;
  else if (route === "settings") content = <Settings locale={locale} session={session} theme={theme} onLocale={onLocale} onTheme={onTheme} onLogout={onLogout} />;
  else content = <Home locale={locale} workspace={workspace} />;

  return <Suspense fallback={<LoadingPanel label={copy[locale].loading} />}>{content}</Suspense>;
}

function PageHeader({ title, body }: { title: string; body: string }) {
  return <header className="page-header"><h1>{title}</h1><p>{body}</p></header>;
}

function Home({ locale, workspace }: { locale: Locale; workspace: WorkspaceData }) {
  const t = copy[locale];
  const latestRun = workspace.runs[0];
  return (
    <section className="page">
      <PageHeader title={t.homeTitle} body={t.homeBody} />
      <div className="metric-grid">
        <Metric label={t.agentMetric} value={String(workspace.agents.length)} />
        <Metric label={t.runs} value={String(workspace.runs.length)} />
        <Metric label={t.issues} value={String(workspace.issues.length)} />
        <Metric label={t.cases} value={String(workspace.cases.length)} />
        <Metric label={t.releases} value={String(workspace.releases.length)} />
      </div>
      <section className="focus-block">
        <div>
          <p className="section-label">{t.latest}</p>
          <h2>{latestRun ? `${runOperationLabel(latestRun.operation, locale)} · ${runStateLabel(latestRun.state, locale)}` : t.noRuns}</h2>
        </div>
        {latestRun ? <Time value={latestRun.updated_at} locale={locale} /> : null}
      </section>
      <RunList title={t.recentRuns} empty={t.noRuns} runs={workspace.runs.slice(0, 6)} locale={locale} />
    </section>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return <div className="metric"><strong>{value}</strong><span>{label}</span></div>;
}

function Memory({ locale, workspace }: { locale: Locale; workspace: WorkspaceData }) {
  const t = copy[locale];
  const ready = workspace.system.memory_store === "available";
  const [query, setQuery] = useState("");
  const [busy, setBusy] = useState(false);
  const [loadingRecent, setLoadingRecent] = useState(false);
  const [message, setMessage] = useState("");
  const [recent, setRecent] = useState<MemoryRecord[]>([]);
  const [total, setTotal] = useState(0);
  const [results, setResults] = useState<Array<{ kind: string; item: MemoryRecallItem }> | null>(null);
  const [bundle, setBundle] = useState<MemoryRecallBundle | null>(null);
  const [graph, setGraph] = useState<MemoryFactGraph | null>(null);
  const [graphLoading, setGraphLoading] = useState(false);
  const [graphError, setGraphError] = useState("");
  const [selectedNode, setSelectedNode] = useState<MemoryVisualNode | null>(null);
  const [tasks, setTasks] = useState<MemoryTaskRecord[]>([]);
  const [taskError, setTaskError] = useState("");

  const loadTasks = useCallback(async () => {
    try {
      const response = await api.memoryTasks(12);
      const refreshed = await Promise.all(response.tasks.map(async (task) => {
        if (!isMemoryTaskActive(task)) return task;
        try {
          return { ...task, ...await api.memoryTask(task.task_id) };
        } catch {
          return task;
        }
      }));
      setTasks(refreshed);
      setTaskError("");
      return refreshed;
    } catch {
      setTaskError(t.memoryLoadFailed);
      return [];
    }
  }, [t.memoryLoadFailed]);

  const loadGraph = useCallback(async (factID: string) => {
    if (!/^\d+$/.test(factID)) return;
    setGraphLoading(true);
    setGraphError("");
    try {
      const nextGraph = await api.memoryGraph(factID);
      setGraph(nextGraph);
      setSelectedNode({
        id: `fact:${nextGraph.fact_id}`,
        kind: "fact",
        eyebrow: `FACT ${nextGraph.fact_id}`,
        title: nextGraph.content,
        content: nextGraph.content,
        factId: String(nextGraph.fact_id),
        position: { x: 0, y: 0 },
      });
    } catch {
      setGraphError(t.memoryGraphFailed);
    } finally {
      setGraphLoading(false);
    }
  }, [t.memoryGraphFailed]);

  useEffect(() => {
    if (!ready) return;
    let active = true;
    setLoadingRecent(true);
    void api.memories(30).then((response) => {
      if (!active) return;
      setRecent(response.memories);
      setTotal(response.total);
      const defaultFact = [...response.memories].reverse().find((item) => /^\d+$/.test(item.id));
      if (defaultFact) void loadGraph(defaultFact.id);
    }).catch(() => {
      if (!active) return;
      setMessage(t.memoryLoadFailed);
    }).finally(() => {
      if (active) setLoadingRecent(false);
    });
    return () => { active = false; };
  }, [ready, t.memoryLoadFailed, loadGraph]);

  useEffect(() => {
    if (!ready) return;
    let stopped = false;
    let timer = 0;
    const poll = async () => {
      const latest = await loadTasks();
      if (!stopped && latest.some(isMemoryTaskActive)) {
        timer = window.setTimeout(poll, 1800);
      }
    };
    void poll();
    return () => {
      stopped = true;
      window.clearTimeout(timer);
    };
  }, [ready, loadTasks]);

  const visibleEntries: Array<{ kind: string; item: MemoryRecallItem; createdAt?: string }> = results === null
    ? recent.map((item) => ({
      kind: t.recallFact,
      item: { id: item.id, content: item.content, score: 0, metadata: item.metadata },
      createdAt: item.created_at,
    }))
    : results.map((item) => ({ ...item }));

  const visibleFacts = visibleEntries.filter(({ item }) => /^\d+$/.test(item.id) && (results === null || results.some((result) => result.item === item && result.kind === t.recallFact)));
  const graphCatalog: MemoryRecord[] = [
    ...recent,
    ...(bundle?.facts ?? []).map((item) => ({ id: item.id, content: item.content, metadata: item.metadata })),
  ].filter((item, index, items) => items.findIndex((candidate) => candidate.id === item.id) === index);
  const selectedRecord = graph && graphCatalog.find((item) => item.id === String(graph.fact_id));
  const sourceConversation = typeof selectedRecord?.metadata?.conversation_id === "string" ? selectedRecord.metadata.conversation_id : "";
  const nodeLabel = selectedNode?.kind === "entity" ? t.graphEntity : selectedNode?.kind === "related" ? t.graphRelated : t.graphFact;

  return (
    <section className="page memory-page">
      <header className="memory-page-header">
        <div><p className="section-label">XIAOBAOS CONVERSATION MEMORY</p><h1>{t.memoryTitle}</h1><p>{t.memoryBody}</p></div>
        {ready ? <div className="memory-ready-summary"><Status value="available" locale={locale} /><strong>{total}</strong><span>{t.memoryCount}</span></div> : null}
      </header>
      {!ready ? (
        <section className="memory-unavailable">
          <Status value="unavailable" locale={locale} />
          <div><h2>{t.memoryUnavailable}</h2><p>{t.memoryUnavailableBody}</p></div>
        </section>
      ) : (
        <>
          <MemoryTaskCenter tasks={tasks} locale={locale} error={taskError} />
          <form className="memory-search" onSubmit={async (event) => {
            event.preventDefault();
            if (!query.trim()) return;
            setBusy(true);
            setMessage("");
            try {
              const bundle = await api.searchMemories(query.trim());
              setBundle(bundle);
              setResults([
                ...bundle.facts.map((item) => ({ kind: t.recallFact, item })),
                ...bundle.conversations.map((item) => ({ kind: t.recallConversation, item })),
                ...bundle.topics.map((item) => ({ kind: t.recallTopic, item })),
              ]);
              const firstFact = bundle.facts.find((item) => /^\d+$/.test(item.id));
              if (firstFact) await loadGraph(firstFact.id);
            } catch {
              setResults(null);
              setBundle(null);
              setMessage(t.memoryRecallFailed);
            } finally {
              setBusy(false);
            }
          }}>
            <label><span>{t.memoryQuery}</span><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder={t.memoryPlaceholder} /></label>
            <button className="primary-button compact" type="submit" disabled={busy || !query.trim()}>{busy ? t.memorySearching : t.memorySearch}</button>
          </form>
          {message ? <InlineNote tone="error">{message}</InlineNote> : null}
          <div className="memory-graph-workspace">
            <section className="memory-graph-panel">
              <header><div><h2>{t.memoryGraph}</h2><p>{t.memoryGraphHint}</p></div>{graph ? <dl><div><dt>{t.memoryEntities}</dt><dd>{graph.total_entities}</dd></div><div><dt>{t.memoryRelations}</dt><dd>{graph.total_relations}</dd></div></dl> : null}</header>
              {graphLoading && !graph ? <LoadingPanel label={t.loading} /> : graph ? (
                <Suspense fallback={<LoadingPanel label={t.loading} />}>
                  <MemoryGraphCanvas graph={graph} facts={graphCatalog} locale={locale} selectedNodeID={selectedNode?.id ?? `fact:${graph.fact_id}`} onSelect={setSelectedNode} onOpenFact={(factID: string) => { void loadGraph(factID); }} />
                </Suspense>
              ) : loadingRecent ? <LoadingPanel label={t.loading} /> : <EmptyState title={t.memoryGraphEmpty} />}
              {graphError ? <p className="memory-graph-error" role="alert">{graphError}</p> : null}
            </section>
            <aside className="memory-inspector">
              <p className="section-label">{t.memoryInspector}</p>
              {selectedNode ? (
                <>
                  <span className={`memory-node-kind ${selectedNode.kind}`}>{nodeLabel}</span>
                  <h2>{selectedNode.title}</h2>
                  {selectedNode.title !== selectedNode.content ? <p>{selectedNode.content}</p> : null}
                  <dl>
                    {selectedNode.relation ? <MemoryContextFact label={t.relationType} value={selectedNode.relation.type} /> : null}
                    {selectedNode.relation ? <MemoryContextFact label={t.confidence} value={`${Math.round(selectedNode.relation.confidence * 100)}%`} /> : null}
                    {selectedNode.entity?.type && selectedNode.entity.type !== "unknown" ? <MemoryContextFact label="Type" value={selectedNode.entity.type} /> : null}
                    {sourceConversation && selectedNode.kind === "fact" ? <MemoryContextFact label={t.sourceConversation} value={sourceConversation} /> : null}
                    {bundle && selectedNode.kind === "fact" ? <MemoryContextFact label={t.searchTime} value={typeof bundle.search_time_ms === "number" ? `${Math.round(bundle.search_time_ms)} ms` : "-"} /> : null}
                  </dl>
                </>
              ) : <p>{t.memorySelectHint}</p>}
            </aside>
          </div>
          <section className="memory-index">
            <header><div><h2>{results === null ? t.recentMemories : t.memoryResults}</h2><span>{results === null ? `${total} ${t.memoryCount}` : `${visibleEntries.length}`}</span></div></header>
            {loadingRecent && results === null ? <LoadingPanel label={t.loading} /> : visibleFacts.length === 0 ? <EmptyState title={results === null ? t.noMemories : t.noMemoryResults} /> : (
              <div className="memory-index-list">
                {visibleFacts.slice(0, 12).map(({ kind, item, createdAt }, index) => (
                  <MemoryEntry key={`${kind}-${item.id}-${index}`} kind={kind} item={item} createdAt={createdAt} locale={locale} active={graph?.fact_id === Number(item.id)} onSelect={() => { void loadGraph(item.id); }} />
                ))}
              </div>
            )}
          </section>
        </>
      )}
    </section>
  );
}

function MemoryTaskCenter({ tasks, locale, error }: { tasks: MemoryTaskRecord[]; locale: Locale; error: string }) {
  const t = copy[locale];
  const activeCount = tasks.filter(isMemoryTaskActive).length;
  const statusLabel = (status: MemoryTaskRecord["status"]) => ({
    pending: t.memoryTaskPending,
    processing: t.memoryTaskProcessing,
    completed: t.memoryTaskCompleted,
    failed: t.memoryTaskFailed,
  })[status];

  return (
    <section className="memory-task-center" aria-live="polite">
      <header>
        <div><h2>{t.memoryTasks}</h2><p>{t.memoryTasksHint}</p></div>
        {activeCount > 0 ? <strong>{activeCount} {t.memoryTaskProcessing}</strong> : null}
      </header>
      {error ? <p className="memory-task-error" role="alert">{error}</p> : tasks.length === 0 ? (
        <p className="memory-task-empty">{t.memoryTaskEmpty}</p>
      ) : (
        <div className="memory-task-list">
          {tasks.slice(0, 5).map((task) => {
            const percent = memoryTaskDisplayPercent(task);
            return (
              <article className={`memory-task-row ${task.status}`} key={task.task_id}>
                <i aria-hidden="true" />
                <div className="memory-task-copy">
                  <strong>{task.source_conversation_title || task.source_conversation_id || t.memoryTaskUnknownSource}</strong>
                  <span>{task.agent_name || task.agent_id}{task.current_step ? ` · ${task.current_step}` : task.message ? ` · ${task.message}` : ""}</span>
                </div>
                <div className="memory-task-state">
                  <strong>{statusLabel(task.status)}</strong>
                  <span>{percent}%</span>
                </div>
                <div className="memory-task-meter" aria-label={`${statusLabel(task.status)} ${percent}%`}><i style={{ width: `${percent}%` }} /></div>
              </article>
            );
          })}
        </div>
      )}
    </section>
  );
}

function MemoryEntry({
  kind,
  item,
  createdAt,
  locale,
  active,
  onSelect,
}: {
  kind: string;
  item: MemoryRecallItem;
  createdAt?: string;
  locale: Locale;
  active: boolean;
  onSelect: () => void;
}) {
  const t = copy[locale];
  const conversationID = typeof item.metadata?.conversation_id === "string" ? item.metadata.conversation_id : "";
  const agent = typeof item.metadata?.agent_id === "string" ? item.metadata.agent_id : "";
  const score = item.score > 0 ? `${Math.round(item.score * 100)}%` : "";
  return (
    <button className={`memory-result ${active ? "active" : ""}`} type="button" onClick={onSelect}>
      <span className="memory-result-header"><span>{kind}</span>{score ? <strong>{score}</strong> : createdAt ? <Time value={createdAt} locale={locale} /> : null}</span>
      <span className="memory-result-title">{item.title || item.content}</span>
      {agent || conversationID ? (
        <span className="memory-result-footer">
          {agent ? <span>{agent}</span> : null}
          {conversationID ? <code><b>{t.sourceConversation}</b>{conversationID}</code> : null}
        </span>
      ) : null}
    </button>
  );
}

function MemoryContextFact({ label, value }: { label: string; value: string }) {
  return <div><dt>{label}</dt><dd>{value}</dd></div>;
}

function RunList({ title, empty, runs, locale }: { title: string; empty: string; runs: WorkspaceData["runs"]; locale: Locale }) {
  return (
    <RecordSection title={title} empty={empty}>
      {runs.map((run) => <RecordRow key={run.run_id} title={runOperationLabel(run.operation, locale)} meta={`${runOriginLabel(run.origin, locale)} · ${runStateLabel(run.state, locale)}`} time={run.updated_at} locale={locale} />)}
    </RecordSection>
  );
}

function runOperationLabel(value: string, locale: Locale) {
  const labels: Record<string, [string, string]> = {
    explore: ["探索", "Explore"],
    replay: ["回归", "Replay"],
    compare: ["对比", "Compare"],
  };
  return labels[value]?.[locale === "zh" ? 0 : 1] ?? value;
}

function runStateLabel(value: string, locale: Locale) {
  const labels: Record<string, [string, string]> = {
    queued: ["等待中", "Queued"],
    running: ["运行中", "Running"],
    completed: ["已完成", "Completed"],
    failed: ["失败", "Failed"],
    cancelled: ["已取消", "Cancelled"],
  };
  return labels[value]?.[locale === "zh" ? 0 : 1] ?? value;
}

function runOriginLabel(value: string, locale: Locale) {
  const labels: Record<string, [string, string]> = {
    platform: ["平台", "Platform"],
    edge: ["端侧", "Edge"],
    local: ["本地", "Local"],
  };
  return labels[value]?.[locale === "zh" ? 0 : 1] ?? value;
}

function RecordSection({ title, empty, children }: { title: string; empty: string; children: React.ReactNode }) {
  const count = useMemo(() => Array.isArray(children) ? children.length : children ? 1 : 0, [children]);
  return <section className="record-section"><h2>{title}</h2>{count ? <div className="record-list">{children}</div> : <EmptyState title={empty} />}</section>;
}

function RecordRow({ title, meta, time, locale }: { title: string; meta: string; time: string; locale: Locale }) {
  return <article className="record-row"><div><strong>{title}</strong><span>{meta}</span></div><Time value={time} locale={locale} /></article>;
}

function Time({ value, locale }: { value: string; locale: Locale }) {
  const formatted = new Intl.DateTimeFormat(locale === "zh" ? "zh-CN" : "en", { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" }).format(new Date(value));
  return <time dateTime={value}>{formatted}</time>;
}

function Status({ value, locale }: { value: string; locale: Locale }) {
  const ready = value === "ready" || value === "ok" || value === "available";
  return <span className={ready ? "status ready" : "status blocked"}>{ready ? copy[locale].statusReady : copy[locale].statusUnavailable}</span>;
}

function Settings({ locale, session, theme, onLocale, onTheme, onLogout }: { locale: Locale; session: Session; theme: Theme; onLocale: (locale: Locale) => void; onTheme: (theme: Theme) => void; onLogout: () => void }) {
  const t = copy[locale];
  return (
    <section className="page settings-page">
      <PageHeader title={t.settingsTitle} body={t.settingsBody} />
      <section className="settings-section preference-section">
        <div><h2>{t.language}</h2><p>{t.languageBody}</p></div>
        <div className="segmented-control" role="group" aria-label={t.language}>
          <button className={locale === "zh" ? "active" : ""} type="button" onClick={() => onLocale("zh")}>中文</button>
          <button className={locale === "en" ? "active" : ""} type="button" onClick={() => onLocale("en")}>English</button>
        </div>
      </section>
      <section className="settings-section preference-section">
        <div><h2>{t.theme}</h2><p>{t.themeBody}</p></div>
        <div className="segmented-control" role="group" aria-label={t.theme}>
          <button className={theme === "system" ? "active" : ""} type="button" onClick={() => onTheme("system")}>{t.themeSystem}</button>
          <button className={theme === "light" ? "active" : ""} type="button" onClick={() => onTheme("light")}>{t.themeLight}</button>
          <button className={theme === "dark" ? "active" : ""} type="button" onClick={() => onTheme("dark")}>{t.themeDark}</button>
        </div>
      </section>
      <section className="settings-section">
        <h2>{locale === "zh" ? "账户" : "Account"}</h2>
        <p>{locale === "zh" ? "这里显示当前登录身份。Agent 接入密钥和 LLM 配置统一在 API 管理页面维护。" : "This is your current identity. Agent credentials and LLM configuration live in API Management."}</p>
        {session.user ? <InlineNote>{session.user.display_name}</InlineNote> : null}
        {session.mode === "github" ? <button className="text-button danger" type="button" onClick={onLogout}>{t.signOut}</button> : null}
      </section>
    </section>
  );
}

function EmptyState({ title }: { title: string }) {
  return <div className="empty-state"><span>{title}</span></div>;
}

function InlineNote({ children, tone = "neutral" }: { children: React.ReactNode; tone?: "neutral" | "error" }) {
  return <p className={`inline-note ${tone}`}>{children}</p>;
}

function InlineError({ message, action, onRetry }: { message: string; action: string; onRetry: () => void }) {
  return <div className="inline-error"><span>{message}</span><button type="button" className="text-button" onClick={onRetry}>{action}</button></div>;
}

function LoadingPage({ label }: { label: string }) {
  return <main className="loading-page"><Brand /><div className="loading-lines" aria-label={label}><i /><i /><i /></div><p>{label}</p></main>;
}

function LoadingPanel({ label }: { label: string }) {
  return <section className="page"><div className="loading-lines" aria-label={label}><i /><i /><i /></div></section>;
}

function FailurePage({ title, detail, action, onRetry }: { title: string; detail: string; action: string; onRetry: () => void }) {
  return <main className="failure-page"><Brand /><h1>{title}</h1><p>{detail}</p><button className="primary-button" type="button" onClick={onRetry}>{action}</button></main>;
}
