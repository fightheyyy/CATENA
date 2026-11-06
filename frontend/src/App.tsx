import { lazy, Suspense, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import type { CSSProperties } from "react";
import { api } from "./api";
import { primaryNavigationRoutes, readNavigation, navigationURL, type Route, type EvidenceSelection } from "./navigation";
import { useWorkspace } from "./useWorkspace";
import { Icon, type IconName } from "./Icons";
import type { AnalysisJob, Session, WorkspaceData } from "./types";

const AgentWorkspace = lazy(() => import("./AgentWorkspace").then((module) => ({ default: module.AgentWorkspace })));
const ApiManagementPage = lazy(() => import("./ApiManagementPage").then((module) => ({ default: module.ApiManagementPage })));
const WorkbenchWorkspace = lazy(() => import("./WorkbenchWorkspace").then((module) => ({ default: module.WorkbenchWorkspace })));
const AnalysisWorkspace = lazy(() => import("./AnalysisWorkspace").then((module) => ({ default: module.AnalysisWorkspace })));
const TraceExplorer = lazy(() => import("./TraceExplorer").then((module) => ({ default: module.TraceExplorer })));

type Locale = "zh" | "en";
type Theme = "system" | "light" | "dark";

const copy = {
  zh: {
    nav: { home: "工作台", assistant: "工作台", cases: "实验", agents: "Agent", apiKeys: "连接", conversations: "对话", traces: "经历", evolution: "产出", settings: "设置" },
    signIn: "使用 GitHub 登录",
    oauthFlowExpired: "登录流程已过期或从另一个地址发起。请重新登录，Catena 会自动使用正确的回调地址。",
    oauthUpstreamUnavailable: "GitHub 连接暂时超时，请重新登录。Catena 不会保留失败的授权流程。",
    oauthCancelled: "GitHub 授权已取消，你可以随时重新登录。",
    landingTitle: "从真实运行，找到可修复的问题。",
    landingBody: "回看 Codex 和 Claude Code 的运行证据，提炼可审查的 Skill 候选。",
    landingNote: "原始运行留在端侧；Catena 保存可追溯的分析证据。",
    homeTitle: "今天的 Agent 状态",
    homeBody: "通过 Codex 分析 Trace 并生成改进候选，经 Harbor 独立评测和人工审查。",
    agentsTitle: "Agent",
    agentsBody: "这里只展示被观测和进化的目标 Agent；编排器与评测角色保留在 Trace 执行链中。",
    tracesTitle: "Trace",
    tracesBody: "Trace 是运行证据，不是日志堆。工具调用、结果与产物会保留同一条因果链。",
    settingsTitle: "设置",
    settingsBody: "让 Catena 更符合你的阅读习惯。",
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
    noRuntime: "Catena Engine尚未就绪。",
    observedAgents: "已观测 Agent",
    cloudRuntime: "Catena Engine",
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
    nav: { home: "Workspace", assistant: "Workspace", cases: "Experiments", agents: "Agents", apiKeys: "Connections", conversations: "Conversations", traces: "History", evolution: "Outputs", settings: "Settings" },
    signIn: "Continue with GitHub",
    oauthFlowExpired: "This sign-in flow expired or started on another address. Restart it and Catena will use the canonical callback origin.",
    oauthUpstreamUnavailable: "GitHub temporarily timed out. Restart sign-in; Catena does not retain the failed authorization flow.",
    oauthCancelled: "GitHub authorization was cancelled. You can restart sign-in at any time.",
    landingTitle: "Find fixable patterns in real Agent runs.",
    landingBody: "Review Codex and Claude Code evidence, then draft Skills grounded in the observed behavior.",
    landingNote: "Your Agents run locally; Catena keeps the analysis traceable.",
    homeTitle: "Your Agent state today",
    homeBody: "Analyze traces with Codex, compare candidates in Harbor, and retain human decisions.",
    agentsTitle: "Agents",
    agentsBody: "Only target Agents appear here. Orchestrators and evaluator roles stay in the Trace execution chain.",
    tracesTitle: "Traces",
    tracesBody: "A Trace is causal evidence, not a log pile. Tool calls, results, and artifacts stay linked.",
    settingsTitle: "Settings",
    settingsBody: "Make Catena feel comfortable to read and use.",
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
    noRuntime: "The Catena Engine is not ready.",
    observedAgents: "Observed Agents",
    cloudRuntime: "Catena Engine",
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

function useRoute() {
  const [selection, setSelection] = useState(() => readNavigation(window.location));
  useEffect(() => {
    const onPopState = () => setSelection(readNavigation(window.location));
    window.addEventListener("popstate", onPopState);
    return () => window.removeEventListener("popstate", onPopState);
  }, []);
  const navigate = useCallback((next: Route, evidence: EvidenceSelection = {}) => {
    const url = navigationURL(next, evidence);
    if (url === window.location.pathname + window.location.search) return;
    window.history.pushState({}, "", url);
    setSelection(readNavigation(window.location));
    window.scrollTo({ top: 0, behavior: "instant" });
  }, []);
  return { route: selection.route, selection, navigate };
}

export function App() {
  const [locale, setLocale] = useState<Locale>(() => (localStorage.getItem("catena.locale") === "en" ? "en" : "zh"));
  const [theme, setTheme] = useState<Theme>(() => {
    const saved = localStorage.getItem("catena.theme");
    return saved === "light" || saved === "dark" ? saved : "system";
  });
  const [session, setSession] = useState<Session | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const { route, selection, navigate } = useRoute();
  const workspaceState = useWorkspace(route, session?.authenticated ? `${session.mode}:${session.user?.id ?? "local"}` : "");
  const workspace = workspaceState.data;
  const { updateJobs, removeJob } = workspaceState;
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
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Unknown request error");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const openAnalysisJob = useCallback((job: AnalysisJob) => {
    updateJobs([job]);
    navigate("home", { agentID: job.source_agent_id, jobID: job.job_id });
  }, [navigate, updateJobs]);

  const analyzeAgent = useCallback((agentID: string) => {
    navigate("home", { agentID });
  }, [navigate]);

  const selectAnalysisJob = useCallback((jobID: string) => {
    navigate(selection.route === "home" ? "home" : "evolution", { agentID: selection.agentID, jobID });
  }, [navigate, selection.agentID, selection.route]);

  const removeAnalysisJob = useCallback((jobID: string) => {
    removeJob(jobID);
    navigate("evolution", { agentID: selection.agentID });
  }, [navigate, removeJob, selection.agentID]);

  const openAgentTraces = useCallback((agentID: string, traceID?: string) => {
    navigate("traces", { agentID, traceID });
  }, [navigate]);

  const navigateFromSidebar = useCallback((next: Route) => {
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
      <a className="skip-link" href="#workspace-content">{locale === "zh" ? "跳转至内容" : "Skip to content"}</a>
      <Sidebar
        route={route}
        locale={locale}
        session={session}
        onNavigate={navigateFromSidebar}
        onSettings={() => navigate("settings")}
        onSwitchAccount={switchAccount}
        onLogout={logout}
        onRefresh={["home", "agents", "apiKeys", "traces", "evolution"].includes(route) ? workspaceState.refresh : undefined}
        refreshing={workspaceState.loading}
      />
      <main className="main-canvas" id="workspace-content" tabIndex={-1}>
        {error ? <InlineError message={error} action={t.retry} onRetry={load} /> : null}
        {workspaceState.error ? <InlineError message={workspaceState.error} action={t.retry} onRetry={workspaceState.refresh} /> : null}
        {workspaceState.ready || route === "settings" ? (
          <RouteView
            route={route}
            locale={locale}
            workspace={workspace}
            session={session}
            onNavigate={navigateFromSidebar}
            selectedAnalysisJobID={selection.jobID}
            selectedAnalysisAgentID={selection.agentID}
            selectedTraceAgentID={selection.agentID}
            selectedTraceID={selection.traceID}
            selectedAgentID={selection.agentID}
            refreshVersion={workspaceState.updatedAt}
            onOpenAnalysisJob={openAnalysisJob}
            onSelectAnalysisJob={selectAnalysisJob}
            onDeleteAnalysisJob={removeAnalysisJob}
            onJobsUpdated={updateJobs}
            onAnalyzeAgent={analyzeAgent}
            onOpenAgentTraces={openAgentTraces}
            onConnectAgent={() => navigate("apiKeys")}
            onOpenAgent={(agentID) => navigate("agents", { agentID })}
            onRefresh={workspaceState.refresh}
            theme={theme}
            onLocale={selectLocale}
            onTheme={selectTheme}
            onLogout={logout}
          />
        ) : workspaceState.loading ? (
          <LoadingPanel label={t.loading} />
        ) : null}
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
  const [popoverPosition, setPopoverPosition] = useState({ left: 0, bottom: 0 });
  const menuRef = useRef<HTMLDivElement>(null);
  const user = session.user;
  const displayName = user?.display_name || user?.login || (locale === "zh" ? "本地账户" : "Local account");
  const initial = displayName.trim().slice(0, 1).toUpperCase() || "C";

  useLayoutEffect(() => {
    if (!open) return;
    const positionPopover = () => {
      const bounds = menuRef.current?.getBoundingClientRect();
      if (bounds) setPopoverPosition({ left: bounds.right + 10, bottom: window.innerHeight - bounds.bottom });
    };
    positionPopover();
    const closeOnOutside = (event: MouseEvent) => {
      if (!menuRef.current?.contains(event.target as Node)) setOpen(false);
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") setOpen(false);
    };
    document.addEventListener("mousedown", closeOnOutside);
    document.addEventListener("keydown", closeOnEscape);
    window.addEventListener("resize", positionPopover);
    window.addEventListener("scroll", positionPopover, true);
    return () => {
      document.removeEventListener("mousedown", closeOnOutside);
      document.removeEventListener("keydown", closeOnEscape);
      window.removeEventListener("resize", positionPopover);
      window.removeEventListener("scroll", positionPopover, true);
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
        <div className="account-popover" role="menu" style={{ "--account-popover-left": `${popoverPosition.left}px`, "--account-popover-bottom": `${popoverPosition.bottom}px` } as CSSProperties}>
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
          <h1>{locale === "zh" ? t.landingTitle.split("，").map((line, index) => <span key={line}>{line}{index === 0 ? "，" : ""}</span>) : t.landingTitle}</h1>
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
      <span>catena</span>
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
  onRefresh,
  refreshing,
}: {
  route: Route;
  locale: Locale;
  session: Session;
  onNavigate: (route: Route) => void;
  onSettings: () => void;
  onSwitchAccount: () => Promise<void>;
  onLogout: () => Promise<void>;
  onRefresh?: () => Promise<void>;
  refreshing: boolean;
}) {
  const t = copy[locale];
  const icons: Record<Route, IconName> = { home: "home", assistant: "assistant", traces: "history", evolution: "outputs", agents: "agents", apiKeys: "plug", settings: "settings" };
  return (
    <aside className="sidebar">
      <div className="sidebar-brand"><Brand />{onRefresh && <button className="text-button icon-button" type="button" aria-label={locale === "zh" ? "刷新" : "Refresh"} title={locale === "zh" ? "刷新" : "Refresh"} disabled={refreshing} onClick={() => void onRefresh()}><Icon name="refresh" /></button>}</div>
      <nav aria-label={locale === "zh" ? "工作空间" : "Workspace"}>
        {primaryNavigationRoutes.map((item) => (
          <button
            className={route === item ? "nav-item active" : "nav-item"}
            key={item}
            type="button"
            aria-current={route === item ? "page" : undefined}
            onClick={() => onNavigate(item)}
          >
            <Icon name={icons[item]} /><span>{t.nav[item]}</span>
          </button>
        ))}
      </nav>
      <div className="sidebar-footer">
        <div className="sidebar-utilities">
          {(["agents", "apiKeys", "settings"] as const).map((item) => <button key={item} className={route === item ? "nav-item active" : "nav-item"} type="button" aria-current={route === item ? "page" : undefined} onClick={() => onNavigate(item)}><Icon name={icons[item]} /><span>{t.nav[item]}</span></button>)}
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
  selectedAnalysisJobID,
  selectedAnalysisAgentID,
  selectedTraceAgentID,
  selectedTraceID,
  selectedAgentID,
  refreshVersion,
  onOpenAnalysisJob,
  onSelectAnalysisJob,
  onDeleteAnalysisJob,
  onJobsUpdated,
  onAnalyzeAgent,
  onOpenAgentTraces,
  onConnectAgent,
  onOpenAgent,
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
  onNavigate: (route: Route) => void;
  selectedAnalysisJobID: string;
  selectedAnalysisAgentID: string;
  selectedTraceAgentID: string;
  selectedTraceID: string;
  selectedAgentID: string;
  refreshVersion: string;
  onOpenAnalysisJob: (job: AnalysisJob) => void;
  onSelectAnalysisJob: (jobID: string) => void;
  onDeleteAnalysisJob: (jobID: string) => void;
  onJobsUpdated: (jobs: AnalysisJob[]) => void;
  onAnalyzeAgent: (agentID: string) => void;
  onOpenAgentTraces: (agentID: string, traceID?: string) => void;
  onConnectAgent: () => void;
  onOpenAgent: (agentID: string) => void;
  onRefresh: () => Promise<void>;
  theme: Theme;
  onLocale: (locale: Locale) => void;
  onTheme: (theme: Theme) => void;
  onLogout: () => void;
}) {
  let content: React.ReactNode;
  if (route === "home" && !selectedAnalysisJobID) content = <WorkbenchWorkspace key={session.user?.id ?? "local"} locale={locale} workspace={workspace} ownerID={session.user?.id ?? "local"} initialAgentID={selectedAgentID} onOpenTrace={onOpenAgentTraces} onOpenJob={onOpenAnalysisJob} onConnectAgent={onConnectAgent} onRefresh={onRefresh} />;
  else if (route === "agents") content = <AgentWorkspace locale={locale} workspace={workspace} initialAgentID={selectedAgentID} onSelectAgent={onOpenAgent} onAnalyze={onAnalyzeAgent} onOpenTraces={onOpenAgentTraces} onConnect={onConnectAgent} />;
  else if (route === "apiKeys") content = <ApiManagementPage locale={locale} workspace={workspace} onRefresh={onRefresh} onOpenAgent={onOpenAgent} />;
  else if (route === "traces") content = <TraceExplorer locale={locale} workspace={workspace} initialAgentID={selectedTraceAgentID} initialTraceID={selectedTraceID} onSelectTrace={onOpenAgentTraces} refreshVersion={refreshVersion} />;
  else if (route === "evolution" || route === "home") content = (
    <AnalysisWorkspace
      locale={locale}
      jobs={workspace.analysisJobs}
      agents={workspace.agents}
      initialJobID={selectedAnalysisJobID}
      analysisOnly={route === "home"}
      onAnalyze={() => onAnalyzeAgent(selectedAnalysisAgentID)}
      onJobSelected={onSelectAnalysisJob}
      onJobDeleted={onDeleteAnalysisJob}
      onJobsUpdated={onJobsUpdated}
      refreshVersion={refreshVersion}
    />
  );
  else if (route === "settings") content = <Settings locale={locale} session={session} theme={theme} onLocale={onLocale} onTheme={onTheme} onLogout={onLogout} />;
  else content = null;

  return <Suspense fallback={<LoadingPanel label={copy[locale].loading} />}>{content}</Suspense>;
}

function PageHeader({ title }: { title: string }) {
  return <header className="page-header"><h1>{title}</h1></header>;
}

function Settings({ locale, session, theme, onLocale, onTheme, onLogout }: { locale: Locale; session: Session; theme: Theme; onLocale: (locale: Locale) => void; onTheme: (theme: Theme) => void; onLogout: () => void }) {
  const t = copy[locale];
  return (
    <section className="page settings-page">
      <PageHeader title={t.settingsTitle} />
      <section className="settings-section preference-section">
        <h2>{t.language}</h2>
        <div className="segmented-control" role="group" aria-label={t.language}>
          <button className={locale === "zh" ? "active" : ""} type="button" onClick={() => onLocale("zh")}>中文</button>
          <button className={locale === "en" ? "active" : ""} type="button" onClick={() => onLocale("en")}>English</button>
        </div>
      </section>
      <section className="settings-section preference-section">
        <h2>{t.theme}</h2>
        <div className="segmented-control" role="group" aria-label={t.theme}>
          <button className={theme === "system" ? "active" : ""} type="button" onClick={() => onTheme("system")}>{t.themeSystem}</button>
          <button className={theme === "light" ? "active" : ""} type="button" onClick={() => onTheme("light")}>{t.themeLight}</button>
          <button className={theme === "dark" ? "active" : ""} type="button" onClick={() => onTheme("dark")}>{t.themeDark}</button>
        </div>
      </section>
      <section className="settings-section">
        <h2>{locale === "zh" ? "账户" : "Account"}</h2>
        <p>{session.user?.display_name || (locale === "zh" ? "本地账户" : "Local account")}</p>
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
