import { useCallback, useEffect, useSyncExternalStore } from "react";

export type BarenaLocale = "en" | "zh-CN";

export const BARENA_LOCALE_STORAGE_KEY = "barena.locale";

const LOCALE_CHANGE_EVENT = "barena:locale-change";

const enMessages = {
  Language: "Language",
  English: "English",
  Chinese: "中文",
  Search: "Search",
  "Open command bar": "Open command bar",
  "My Usage": "My Usage",
  Devices: "Devices",
  Configure: "Configure",
  Home: "Home",
  Trace: "Trace",
  Analytics: "Analytics",
  "Trace Explorer": "Trace Explorer",
  Traces: "Traces",
  "Online Evals": "Online Evals",
  Observe: "Observe",
  Test: "Test",
  Evolution: "Evolution",
  Explore: "Explore",
  "Explore Runs": "Explore Runs",
  "Test Scenarios": "Test Scenarios",
  Experiments: "Experiments",
  Annotations: "Annotations",
  Build: "Build",
  Prompts: "Prompts",
  Agents: "Agents",
  Workflows: "Workflows",
  Evaluators: "Evaluators",
  Datasets: "Datasets",
  Automations: "Automations",
  Settings: "Settings",
  "General Settings": "General Settings",
  "Appearance & Language": "Appearance & Language",
  "Agent Connection": "Agent Connection",
  "API Keys": "API Keys",
  "Model Providers": "Model Providers",
  Members: "Members",
  "Teams & Projects": "Teams & Projects",
  Theme: "Theme",
  Light: "Light",
  System: "System",
  Dark: "Dark",
  "Choose how Catena looks and speaks on this browser.":
    "Choose how Catena looks and speaks on this browser.",
  "Use Catena in your preferred interface language.":
    "Use Catena in your preferred interface language.",
  "Follow your system or choose a fixed appearance.":
    "Follow your system or choose a fixed appearance.",
  Support: "Support",
  Chat: "Chat",
  Documentation: "Documentation",
  "Status Page": "Status Page",
  "Feature Request": "Feature Request",
  "Report a Bug": "Report a Bug",
  Usage: "Usage",
  "You have used {current} {unit} out of {maximum} this month.":
    "You have used {current} {unit} out of {maximum} this month.",
  Legacy: "Legacy",
  Govern: "Govern",
  "AI Gateway": "AI Gateway",
  "AI Governance": "AI Governance",
  Beta: "Beta",
  Dashboard: "Dashboard",
  "Evolution control plane": "Evolution control plane",
  "Turn runtime evidence into a release decision":
    "Turn runtime evidence into a release decision",
  "Inspect an OpenTelemetry trace, retain the failure, review a fixed Case, then Replay it through the same TypeScript evaluation engine.":
    "Inspect an OpenTelemetry trace, retain the failure, review a fixed Case, then Replay it through the same TypeScript evaluation engine.",
  Refresh: "Refresh",
  "Refresh workbench": "Refresh workbench",
  "New issue": "New issue",
  "Control plane unavailable": "Control plane unavailable",
  Issues: "Issues",
  Cases: "Cases",
  Evaluations: "Evaluations",
  Compare: "Compare",
  "Release gates": "Release gates",
  "Loading retained evidence…": "Loading retained evidence…",
  Review: "Review",
  Replay: "Replay",
  Decide: "Decide",
  "Run + Trace": "Run + Trace",
  "Immutable Case": "Immutable Case",
  "Engine evidence": "Engine evidence",
  "Release gate": "Release gate",
  Issue: "Issue",
  Evidence: "Evidence",
  Severity: "Severity",
  Status: "Status",
  Action: "Action",
  "No retained issues": "No retained issues",
  "Open a Barena-linked trace or choose New issue to turn runtime evidence into a reviewable failure.":
    "Open a Barena-linked trace or choose New issue to turn runtime evidence into a reviewable failure.",
  "Review as Case": "Review as Case",
  Case: "Case",
  "Success contract": "Success contract",
  Provenance: "Provenance",
  "No immutable Cases": "No immutable Cases",
  "Review one Issue and define its deterministic artifact verifier.":
    "Review one Issue and define its deterministic artifact verifier.",
  "Replay Case": "Replay Case",
  "No Replay evaluations": "No Replay evaluations",
  "Replay an immutable Case. The TypeScript Engine's terminal decision will appear here unchanged.":
    "Replay an immutable Case. The TypeScript Engine's terminal decision will appear here unchanged.",
  "Replay in progress": "Replay in progress",
  "Replay failed": "Replay failed",
  "Replay ended without evaluation evidence":
    "Replay ended without evaluation evidence",
  "Replay unavailable: {reason}": "Replay unavailable: {reason}",
  Decision: "Decision",
  "Case / Run": "Case / Run",
  "Trace evidence": "Trace evidence",
  "Engine result": "Engine result",
  Created: "Created",
  "No release decisions": "No release decisions",
  "A Release is persisted only after a completed Replay produces a valid terminal Engine decision.":
    "A Release is persisted only after a completed Replay produces a valid terminal Engine decision.",
  Gate: "Gate",
  Lineage: "Lineage",
  Summary: "Summary",
  Evaluation: "Evaluation",
  Harness: "Harness",
  source: "source",
  replay: "replay",
  "open trace": "open trace",
  "Terminal Engine decision retained.": "Terminal Engine decision retained.",
  "Replay started": "Replay started",
  "{runId} is now producing release evidence.":
    "{runId} is now producing release evidence.",
  "Replay could not start": "Replay could not start",
  "Issue retained": "Issue retained",
  "The trace evidence is ready for review.":
    "The trace evidence is ready for review.",
  "Issue could not be created": "Issue could not be created",
  "Retain in Evolution": "Retain in Evolution",
  "Explore evidence retained": "Explore evidence retained",
  "Continue in Evolution to review an Issue and freeze a Replay Case.":
    "Continue in Evolution to review an Issue and freeze a Replay Case.",
  "Evidence could not be retained": "Evidence could not be retained",
  "Create issue from evidence": "Create issue from evidence",
  "Keep a concrete failure or boundary before it disappears into run history.":
    "Keep a concrete failure or boundary before it disappears into run history.",
  "Close issue form": "Close issue form",
  "Source Explore run": "Source Explore run",
  "Select a completed Explore run": "Select a completed Explore run",
  "Complete one Explore run before creating an issue.":
    "Complete one Explore run before creating an issue.",
  "Trace ID": "Trace ID",
  "Prefilled when opened from a Barena trace":
    "Prefilled when opened from a Barena trace",
  "Optional, but when supplied it must be retained by this Run.":
    "Optional, but when supplied it must be retained by this Run.",
  "What failed?": "What failed?",
  "Agent stopped before writing the requested artifact":
    "Agent stopped before writing the requested artifact",
  "Evidence and expected behavior": "Evidence and expected behavior",
  "Describe what the trace shows, and what should have happened.":
    "Describe what the trace shows, and what should have happened.",
  Low: "Low",
  Medium: "Medium",
  High: "High",
  Critical: "Critical",
  Unknown: "Unknown",
  Cancel: "Cancel",
  "Retaining…": "Retaining…",
  "Retain issue": "Retain issue",
  "Immutable Case created": "Immutable Case created",
  "Replay now uses the reviewed prompt and verifier.":
    "Replay now uses the reviewed prompt and verifier.",
  "Issue could not be promoted": "Issue could not be promoted",
  "Review before turning into a Case": "Review before turning into a Case",
  "Once created, revision 1 is immutable.":
    "Once created, revision 1 is immutable.",
  "Close case review": "Close case review",
  "Success criteria": "Success criteria",
  "The Agent completes the task and leaves verifiable evidence.":
    "The Agent completes the task and leaves verifiable evidence.",
  "Replay prompt": "Replay prompt",
  "Leave blank to reuse the original Explore objective.":
    "Leave blank to reuse the original Explore objective.",
  "This freezes the user request that will be replayed.":
    "This freezes the user request that will be replayed.",
  "Required artifact path": "Required artifact path",
  "Artifact must contain": "Artifact must contain",
  "Optional deterministic content check":
    "Optional deterministic content check",
  "Keep as issue": "Keep as issue",
  "Creating…": "Creating…",
  "Create immutable Case": "Create immutable Case",
  "Turn this finding into a Regression Case":
    "Turn this finding into a Regression Case",
  "Review and edit the proposed prompt and verifier before anything becomes immutable.":
    "Review and edit the proposed prompt and verifier before anything becomes immutable.",
  "Review proposed Case": "Review proposed Case",
  "Fix and review Case": "Fix and review Case",
  "Retaining finding…": "Retaining finding…",
  "Finding already promoted": "Finding already promoted",
  "Its immutable Case is ready for Replay.":
    "Its immutable Case is ready for Replay.",
  "Finding retained": "Finding retained",
  "Review the proposed prompt and verifier before creating the Case.":
    "Review the proposed prompt and verifier before creating the Case.",
  "Finding could not be retained": "Finding could not be retained",
  "Request failed": "Request failed",
  Open: "Open",
  Promoted: "Promoted",
  Cleared: "Cleared",
  Held: "Held",
  Rejected: "Rejected",
  "revision {revision}": "revision {revision}",
  "Switch language to {language}": "Switch language to {language}",
  "No compatible Explore runs": "No compatible Explore runs",
  "Complete the same Scenario against the same HTTP Agent at least twice, then retain both runs in Evolution.":
    "Complete the same Scenario against the same HTTP Agent at least twice, then retain both runs in Evolution.",
  "Evidence comparison, not a release decision":
    "Evidence comparison, not a release decision",
  "Compare two completed runs from the same Scenario and target Agent. Only Replay can produce a Release Gate.":
    "Compare two completed runs from the same Scenario and target Agent. Only Replay can produce a Release Gate.",
  "Run A": "Run A",
  "Run B": "Run B",
  "Compatible pair": "Compatible pair",
  "Judge verdict": "Judge verdict",
  "Criteria met": "Criteria met",
  "Source status": "Source status",
  Duration: "Duration",
  Completed: "Completed",
  "Run A Judge evidence": "Run A Judge evidence",
  "Run B Judge evidence": "Run B Judge evidence",
  "No Judge reasoning was retained.": "No Judge reasoning was retained.",
  "Unmet criteria": "Unmet criteria",
  "Open trace": "Open trace",
  "Open Trace": "Open Trace",
  "These services are required on this evaluation or workflow page but are not configured:":
    "These services are required on this evaluation or workflow page but are not configured:",
  "OTLP-native": "OTLP-native",
  "Your Agent evolution workspace": "Your Agent evolution workspace",
  "Observe deployed runtimes, explore unknown behavior, and turn concrete failures into auditable Replay and release decisions.":
    "Observe deployed runtimes, explore unknown behavior, and turn concrete failures into auditable Replay and release decisions.",
  "OTLP Trace": "OTLP Trace",
  "Explore Run": "Explore Run",
  "Release Gate": "Release Gate",
  "See what every Agent actually did": "See what every Agent actually did",
  "Collect runtime-native OpenTelemetry traces in one project, regardless of the Agent framework.":
    "Collect runtime-native OpenTelemetry traces in one project, regardless of the Agent framework.",
  "Open Trace Explorer": "Open Trace Explorer",
  "Test behavior with simulated users": "Test behavior with simulated users",
  "Drive an HTTP Agent through multi-turn scenarios and retain the Judge result with its execution evidence.":
    "Drive an HTTP Agent through multi-turn scenarios and retain the Judge result with its execution evidence.",
  "Run an Explore": "Run an Explore",
  "Turn failures into release evidence": "Turn failures into release evidence",
  "Review an Issue, freeze it as a deterministic Case, and Replay it before the next Agent release.":
    "Review an Issue, freeze it as a deterministic Case, and Replay it before the next Agent release.",
  "Open Evolution": "Open Evolution",
  "Cloud evaluator Runtime": "Cloud evaluator Runtime",
  "Checking Runtime": "Checking Runtime",
  Ready: "Ready",
  Blocked: "Blocked",
  Unavailable: "Unavailable",
  "Target Agents stay external": "Target Agents stay external",
  "Runs XiaoBaOS's evaluation and evolution roles in the Barena control plane. Your target Agents remain external and connect through HTTP or OTLP.":
    "Runs XiaoBaOS's evaluation and evolution roles in the Barena control plane. Your target Agents remain external and connect through HTTP or OTLP.",
  "Evolution Station": "Evolution Station",
  "Turn one real Trace into a reviewable change":
    "Turn one real Trace into a reviewable change",
  "Select retained runtime evidence. XiaoBaOS inspects the failure, proposes the smallest change, then reviews the proposal before any verification or release.":
    "Select retained runtime evidence. XiaoBaOS inspects the failure, proposes the smallest change, then reviews the proposal before any verification or release.",
  "Evolution history": "Evolution history",
  "Evolution jobs unavailable": "Evolution jobs unavailable",
  "Choose source evidence": "Choose source evidence",
  "Only completed Explore runs with a retained Trace are eligible.":
    "Only completed Explore runs with a retained Trace are eligible.",
  "Explore Run + Trace": "Explore Run + Trace",
  "Select retained evidence": "Select retained evidence",
  "Run Explore and retain its OTLP Trace before starting evolution.":
    "Run Explore and retain its OTLP Trace before starting evolution.",
  "Retained OTLP Trace": "Retained OTLP Trace",
  "Evolution focus (optional)": "Evolution focus (optional)",
  "For example: find why the Agent skipped clarification and propose the smallest safe fix.":
    "For example: find why the Agent skipped clarification and propose the smallest safe fix.",
  "Leave blank to let the retained task and Trace define the focus.":
    "Leave blank to let the retained task and Trace define the focus.",
  "Start evolution": "Start evolution",
  "Starting evolution…": "Starting evolution…",
  "Manage evaluation permission is required to start a job.":
    "Manage evaluation permission is required to start a job.",
  "Evolution started": "Evolution started",
  "The three-role review is now running against retained evidence.":
    "The three-role review is now running against retained evidence.",
  "Evolution could not start": "Evolution could not start",
  "Follow the evolution review": "Follow the evolution review",
  "Every stage keeps its output; later stages do not rewrite earlier evidence.":
    "Every stage keeps its output; later stages do not rewrite earlier evidence.",
  "No evolution job yet": "No evolution job yet",
  "Choose one Trace on the left. The current stage and all review outputs will appear here.":
    "Choose one Trace on the left. The current stage and all review outputs will appear here.",
  "Loading evolution job…": "Loading evolution job…",
  "Evolution job unavailable": "Evolution job unavailable",
  "Evolution stopped": "Evolution stopped",
  "Review the proposed outputs": "Review the proposed outputs",
  "These outputs are evidence and proposals. Verification still happens through Case Replay and Release Gate.":
    "These outputs are evidence and proposals. Verification still happens through Case Replay and Release Gate.",
  "Find the failure mode in Trace evidence":
    "Find the failure mode in Trace evidence",
  "Propose the smallest Role, Skill, Memory, or Harness change":
    "Propose the smallest Role, Skill, Memory, or Harness change",
  "Review the proposal and expose its risks":
    "Review the proposal and expose its risks",
  "Role outputs": "Role outputs",
  Finding: "Finding",
  "Case proposal": "Case proposal",
  "Suggested verifier": "Suggested verifier",
  "Human review is required before this proposal becomes an immutable Case.":
    "Human review is required before this proposal becomes an immutable Case.",
  "Review this proposal before turning it into an immutable Case.":
    "Review this proposal before turning it into an immutable Case.",
  Candidate: "Candidate",
  "Candidate details": "Candidate details",
  "Draft · Unverified": "Draft · Unverified",
  "Not applied, published, replayed, or cleared by Release Gate.":
    "Not applied, published, replayed, or cleared by Release Gate.",
  "Advisory only": "Advisory only",
  "ReviewerCat feedback is not a Release Gate decision. Promote the Case proposal, Replay it, then inspect the gate.":
    "ReviewerCat feedback is not a Release Gate decision. Promote the Case proposal, Replay it, then inspect the gate.",
  Source: "Source",
  Queued: "Queued",
  Running: "Running",
  Done: "Completed",
  Failed: "Failed",
  Role: "Role",
  Skill: "Skill",
  Memory: "Memory",
  "Proposal looks ready for verification":
    "Proposal looks ready for verification",
  "Revise before verification": "Revise before verification",
  "Review blocked by missing evidence": "Review blocked by missing evidence",
  "Proposal rejected by ReviewerCat": "Proposal rejected by ReviewerCat",
  "Agent evolution control plane": "Agent evolution control plane",
  "Observe. Evolve. Verify.": "Observe. Evolve. Verify.",
  "Use real OTLP Trace evidence to propose Agent improvements, then verify retained Cases with Barena Replay and Release Gate.":
    "Use real OTLP Trace evidence to propose Agent improvements, then verify retained Cases with Barena Replay and Release Gate.",
  "Agent Registry": "Agent Registry",
  "OTLP-discovered": "OTLP-discovered",
  "Agents are discovered from standard OpenTelemetry identity. Trace remains the execution evidence, not the asset registry.":
    "Agents are discovered from standard OpenTelemetry identity. Trace remains the execution evidence, not the asset registry.",
  "Connect Agent": "Connect Agent",
  "Last 30 days": "Last 30 days",
  Agent: "Agent",
  Runtime: "Runtime",
  Deployment: "Deployment",
  Activity: "Activity",
  "View traces": "View traces",
  "No Agents discovered": "No Agents discovered",
  "Send one OTLP Trace and Barena will discover the Agent automatically.":
    "Send one OTLP Trace and Barena will discover the Agent automatically.",
  "Open connection setup": "Open connection setup",
  "Observed through OTLP": "Observed through OTLP",
  "{count} traces": "{count} traces",
  "Last seen {time}": "Last seen {time}",
  "Not seen in recent sample": "Not seen in recent sample",
  "Service {service}": "Service {service}",
  "Version {version}": "Version {version}",
  "Environment {environment}": "Environment {environment}",
  "Instance {instance}": "Instance {instance}",
  "Advanced role prompts": "Advanced role prompts",
  "Regression Cases": "Regression Cases",
  "Runtime health": "Runtime health",
  "Agents observed": "Agents observed",
  "Trace volume": "Trace volume",
  Errors: "Errors",
  Models: "Models",
  "No trace activity yet": "No trace activity yet",
} as const;

export type BarenaMessageKey = keyof typeof enMessages;

const zhCNMessages: Record<BarenaMessageKey, string> = {
  Language: "语言",
  English: "English",
  Chinese: "中文",
  Search: "搜索",
  "Open command bar": "打开命令面板",
  "My Usage": "我的用量",
  Devices: "设备",
  Configure: "接入配置",
  Home: "首页",
  Trace: "Trace",
  Analytics: "分析",
  "Trace Explorer": "Trace 浏览器",
  Traces: "Trace 列表",
  "Online Evals": "在线评测",
  Observe: "观测",
  Test: "测试",
  Evolution: "进化台",
  Explore: "探索",
  "Explore Runs": "探索记录",
  "Test Scenarios": "测试场景",
  Experiments: "实验",
  Annotations: "标注",
  Build: "构建",
  Prompts: "提示词",
  Agents: "Agent",
  Workflows: "工作流",
  Evaluators: "评测器",
  Datasets: "数据集",
  Automations: "自动化",
  Settings: "设置",
  "General Settings": "常规设置",
  "Appearance & Language": "外观与语言",
  "Agent Connection": "Agent 接入",
  "API Keys": "API 密钥",
  "Model Providers": "模型供应商",
  Members: "成员",
  "Teams & Projects": "团队与项目",
  Theme: "主题",
  Light: "浅色",
  System: "跟随系统",
  Dark: "深色",
  "Choose how Catena looks and speaks on this browser.":
    "设置当前浏览器中的界面语言与主题。",
  "Use Catena in your preferred interface language.":
    "选择 Catena 的界面语言。",
  "Follow your system or choose a fixed appearance.":
    "跟随系统主题，或固定使用浅色与深色外观。",
  Support: "支持",
  Chat: "在线交流",
  Documentation: "文档",
  "Status Page": "服务状态",
  "Feature Request": "功能建议",
  "Report a Bug": "报告问题",
  Usage: "用量",
  "You have used {current} {unit} out of {maximum} this month.":
    "本月已使用 {current} {unit}，总额度为 {maximum}。",
  Legacy: "旧版",
  Govern: "治理",
  "AI Gateway": "AI 网关",
  "AI Governance": "AI 治理",
  Beta: "测试版",
  Dashboard: "工作台",
  "Evolution control plane": "持续进化控制台",
  "Turn runtime evidence into a release decision":
    "把 Runtime 执行证据转化为发布决策",
  "Inspect an OpenTelemetry trace, retain the failure, review a fixed Case, then Replay it through the same TypeScript evaluation engine.":
    "检查 OpenTelemetry Trace，沉淀失败问题，评审为固定 Case，再通过同一套 TypeScript 评测引擎执行 Replay。",
  Refresh: "刷新",
  "Refresh workbench": "刷新工作台",
  "New issue": "新建问题",
  "Control plane unavailable": "控制平面不可用",
  Issues: "问题",
  Cases: "用例",
  Evaluations: "评测",
  Compare: "对比",
  "Release gates": "发布门禁",
  "Loading retained evidence…": "正在加载已留存证据…",
  Review: "评审",
  Replay: "回归",
  Decide: "决策",
  "Run + Trace": "运行 + Trace",
  "Immutable Case": "不可变用例",
  "Engine evidence": "引擎证据",
  "Release gate": "发布门禁",
  Issue: "问题",
  Evidence: "证据",
  Severity: "严重程度",
  Status: "状态",
  Action: "操作",
  "No retained issues": "暂无留存问题",
  "Open a Barena-linked trace or choose New issue to turn runtime evidence into a reviewable failure.":
    "打开关联 Barena 的 Trace，或选择“新建问题”，把运行证据沉淀为可评审的失败记录。",
  "Review as Case": "评审为用例",
  Case: "用例",
  "Success contract": "成功契约",
  Provenance: "来源",
  "No immutable Cases": "暂无不可变用例",
  "Review one Issue and define its deterministic artifact verifier.":
    "评审一个问题，并为它定义确定性的产物验证器。",
  "Replay Case": "回归此用例",
  "No Replay evaluations": "暂无回归评测",
  "Replay an immutable Case. The TypeScript Engine's terminal decision will appear here unchanged.":
    "运行一个不可变用例；TypeScript 引擎的最终结论会原样保存在这里。",
  "Replay in progress": "正在执行回归",
  "Replay failed": "回归执行失败",
  "Replay ended without evaluation evidence": "回归结束，但没有生成评测证据",
  "Replay unavailable: {reason}": "无法回归：{reason}",
  Decision: "结论",
  "Case / Run": "用例 / 运行",
  "Trace evidence": "Trace 证据",
  "Engine result": "引擎结果",
  Created: "创建时间",
  "No release decisions": "暂无发布决策",
  "A Release is persisted only after a completed Replay produces a valid terminal Engine decision.":
    "只有完整的 Replay 产生有效的引擎最终结论后，系统才会保存发布决策。",
  Gate: "门禁",
  Lineage: "链路来源",
  Summary: "摘要",
  Evaluation: "评测",
  Harness: "Harness",
  source: "来源",
  replay: "回归",
  "open trace": "打开 Trace",
  "Terminal Engine decision retained.": "已保留引擎最终结论。",
  "Replay started": "回归已开始",
  "{runId} is now producing release evidence.": "{runId} 正在生成发布证据。",
  "Replay could not start": "无法启动回归",
  "Issue retained": "问题已留存",
  "The trace evidence is ready for review.": "Trace 证据已可以评审。",
  "Issue could not be created": "无法创建问题",
  "Retain in Evolution": "留存到进化工作台",
  "Explore evidence retained": "Explore 证据已留存",
  "Continue in Evolution to review an Issue and freeze a Replay Case.":
    "前往进化工作台审查 Issue，并冻结为可回放的 Case。",
  "Evidence could not be retained": "无法留存证据",
  "Create issue from evidence": "从证据创建问题",
  "Keep a concrete failure or boundary before it disappears into run history.":
    "在具体失败或能力边界淹没于运行历史之前，把它沉淀下来。",
  "Close issue form": "关闭问题表单",
  "Source Explore run": "来源 Explore 运行",
  "Select a completed Explore run": "选择一个已完成的 Explore 运行",
  "Complete one Explore run before creating an issue.":
    "创建问题前，请先完成一次 Explore 运行。",
  "Trace ID": "Trace ID",
  "Prefilled when opened from a Barena trace":
    "从 Barena Trace 打开时会自动填写",
  "Optional, but when supplied it must be retained by this Run.":
    "可选；填写后，该 Trace 必须属于本次运行。",
  "What failed?": "哪里失败了？",
  "Agent stopped before writing the requested artifact":
    "Agent 在写入所需产物前停止了",
  "Evidence and expected behavior": "证据与预期行为",
  "Describe what the trace shows, and what should have happened.":
    "描述 Trace 显示了什么，以及预期应该发生什么。",
  Low: "低",
  Medium: "中",
  High: "高",
  Critical: "严重",
  Unknown: "未知",
  Cancel: "取消",
  "Retaining…": "正在留存…",
  "Retain issue": "留存问题",
  "Immutable Case created": "不可变用例已创建",
  "Replay now uses the reviewed prompt and verifier.":
    "Replay 将使用已评审的提示词和验证器。",
  "Issue could not be promoted": "无法将问题提升为用例",
  "Review before turning into a Case": "评审后转为用例",
  "Once created, revision 1 is immutable.": "创建后，版本 1 不可修改。",
  "Close case review": "关闭用例评审",
  "Success criteria": "成功标准",
  "The Agent completes the task and leaves verifiable evidence.":
    "Agent 完成任务并留下可验证的证据。",
  "Replay prompt": "回归提示词",
  "Leave blank to reuse the original Explore objective.":
    "留空则复用原始 Explore 目标。",
  "This freezes the user request that will be replayed.":
    "该内容会固化为后续回归的用户请求。",
  "Required artifact path": "必需产物路径",
  "Artifact must contain": "产物必须包含",
  "Optional deterministic content check": "可选的确定性内容检查",
  "Keep as issue": "保留为问题",
  "Creating…": "正在创建…",
  "Create immutable Case": "创建不可变用例",
  "Turn this finding into a Regression Case": "将该发现沉淀为回归用例",
  "Review and edit the proposed prompt and verifier before anything becomes immutable.":
    "在内容固化前，先审核并修改建议的任务提示与验证器。",
  "Review proposed Case": "审核建议用例",
  "Fix and review Case": "修正并审核用例",
  "Retaining finding…": "正在沉淀发现…",
  "Finding already promoted": "该发现已转为用例",
  "Its immutable Case is ready for Replay.": "对应的不可变用例已可执行回放。",
  "Finding retained": "发现已沉淀",
  "Review the proposed prompt and verifier before creating the Case.":
    "创建用例前，请审核建议的任务提示与验证器。",
  "Finding could not be retained": "无法沉淀该发现",
  "Request failed": "请求失败",
  Open: "待处理",
  Promoted: "已转用例",
  Cleared: "可发布",
  Held: "暂缓",
  Rejected: "拒绝发布",
  "revision {revision}": "版本 {revision}",
  "Switch language to {language}": "切换语言为{language}",
  "No compatible Explore runs": "暂无可对比的 Explore 运行",
  "Complete the same Scenario against the same HTTP Agent at least twice, then retain both runs in Evolution.":
    "请对同一个 HTTP Agent 完成至少两次相同 Scenario，并将两次运行都留存到持续进化工作台。",
  "Evidence comparison, not a release decision": "这是证据对照，不是发布决策",
  "Compare two completed runs from the same Scenario and target Agent. Only Replay can produce a Release Gate.":
    "仅对比同一 Scenario、同一目标 Agent 的两次完整运行；只有 Replay 才能产生发布门禁结论。",
  "Run A": "运行 A",
  "Run B": "运行 B",
  "Compatible pair": "可比运行",
  "Judge verdict": "Judge 结论",
  "Criteria met": "满足标准",
  "Source status": "来源状态",
  Duration: "耗时",
  Completed: "完成时间",
  "Run A Judge evidence": "运行 A 的 Judge 证据",
  "Run B Judge evidence": "运行 B 的 Judge 证据",
  "No Judge reasoning was retained.": "本次运行未留存 Judge 推理说明。",
  "Unmet criteria": "未满足标准",
  "Open trace": "打开 Trace",
  "Open Trace": "打开 Trace",
  "These services are required on this evaluation or workflow page but are not configured:":
    "当前评测或工作流页面依赖以下服务，但尚未配置：",
  "OTLP-native": "原生 OTLP",
  "Your Agent evolution workspace": "你的 Agent 持续进化工作台",
  "Observe deployed runtimes, explore unknown behavior, and turn concrete failures into auditable Replay and release decisions.":
    "统一观测已部署的 Runtime，探索未知行为边界，并把具体失败沉淀为可审计的 Replay 与发布决策。",
  "OTLP Trace": "OTLP Trace",
  "Explore Run": "Explore 运行",
  "Release Gate": "发布门禁",
  "See what every Agent actually did": "看清每个 Agent 实际做了什么",
  "Collect runtime-native OpenTelemetry traces in one project, regardless of the Agent framework.":
    "无论 Agent 使用什么框架，都把 Runtime 原生 OpenTelemetry Trace 汇集到同一个项目。",
  "Open Trace Explorer": "打开 Trace 浏览器",
  "Test behavior with simulated users": "用模拟用户测试真实行为",
  "Drive an HTTP Agent through multi-turn scenarios and retain the Judge result with its execution evidence.":
    "通过多轮场景驱动 HTTP Agent，并把 Judge 结论与真实执行证据一并留存。",
  "Run an Explore": "开始 Explore",
  "Turn failures into release evidence": "把失败转化为发布证据",
  "Review an Issue, freeze it as a deterministic Case, and Replay it before the next Agent release.":
    "评审 Issue，固化为确定性 Case，并在下一次 Agent 发布前完成 Replay。",
  "Open Evolution": "打开持续进化",
  "Cloud evaluator Runtime": "云端评测 Runtime",
  "Checking Runtime": "正在检查 Runtime",
  Ready: "就绪",
  Blocked: "受阻",
  Unavailable: "不可用",
  "Target Agents stay external": "被测 Agent 保持外置",
  "Runs XiaoBaOS's evaluation and evolution roles in the Barena control plane. Your target Agents remain external and connect through HTTP or OTLP.":
    "在 Barena 控制平面中运行 XiaoBaOS 的评测与进化角色；被测 Agent 保持外置，通过 HTTP 或 OTLP 接入。",
  "Evolution Station": "进化台",
  "Turn one real Trace into a reviewable change":
    "把一条真实 Trace 转化为可评审的改进方案",
  "Select retained runtime evidence. XiaoBaOS inspects the failure, proposes the smallest change, then reviews the proposal before any verification or release.":
    "选择已留存的 Runtime 证据；XiaoBaOS 会定位失败、提出最小改动，并在验证或发布前审查方案。",
  "Evolution history": "进化记录",
  "Evolution jobs unavailable": "无法获取进化任务",
  "Choose source evidence": "选择来源证据",
  "Only completed Explore runs with a retained Trace are eligible.":
    "只有已完成且留存 Trace 的 Explore 运行可以使用。",
  "Explore Run + Trace": "Explore 运行 + Trace",
  "Select retained evidence": "选择已留存证据",
  "Run Explore and retain its OTLP Trace before starting evolution.":
    "请先完成 Explore 并留存其 OTLP Trace，再启动进化。",
  "Retained OTLP Trace": "已留存的 OTLP Trace",
  "Evolution focus (optional)": "进化重点（可选）",
  "For example: find why the Agent skipped clarification and propose the smallest safe fix.":
    "例如：定位 Agent 为什么跳过澄清，并提出最小且安全的修复。",
  "Leave blank to let the retained task and Trace define the focus.":
    "留空则由已留存任务与 Trace 决定分析重点。",
  "Start evolution": "启动进化",
  "Starting evolution…": "正在启动进化…",
  "Manage evaluation permission is required to start a job.":
    "启动任务需要评测管理权限。",
  "Evolution started": "进化任务已启动",
  "The three-role review is now running against retained evidence.":
    "三个角色正在基于已留存证据执行分析与评审。",
  "Evolution could not start": "无法启动进化任务",
  "Follow the evolution review": "跟踪进化评审",
  "Every stage keeps its output; later stages do not rewrite earlier evidence.":
    "每个阶段都会保留产出；后续阶段不会改写先前证据。",
  "No evolution job yet": "暂无进化任务",
  "Choose one Trace on the left. The current stage and all review outputs will appear here.":
    "从左侧选择一条 Trace；当前阶段与全部评审产出会显示在这里。",
  "Loading evolution job…": "正在加载进化任务…",
  "Evolution job unavailable": "无法获取进化任务",
  "Evolution stopped": "进化任务已停止",
  "Review the proposed outputs": "审阅进化产出",
  "These outputs are evidence and proposals. Verification still happens through Case Replay and Release Gate.":
    "这些内容是分析证据与改进建议；验证仍需通过 Case Replay 与发布门禁完成。",
  "Find the failure mode in Trace evidence": "从 Trace 证据中定位失败模式",
  "Propose the smallest Role, Skill, Memory, or Harness change":
    "提出最小的 Role、Skill、Memory 或 Harness 改动",
  "Review the proposal and expose its risks": "审查改进方案并揭示风险",
  "Role outputs": "角色运行详情",
  Finding: "问题发现",
  "Case proposal": "Case 建议",
  "Suggested verifier": "建议验证器",
  "Human review is required before this proposal becomes an immutable Case.":
    "该建议必须经过人工评审，才能转为不可变 Case。",
  "Review this proposal before turning it into an immutable Case.":
    "请先评审该建议，再将其转为不可变 Case。",
  Candidate: "改进候选",
  "Candidate details": "查看候选内容",
  "Draft · Unverified": "草稿 · 未验证",
  "Not applied, published, replayed, or cleared by Release Gate.":
    "尚未应用、发布、Replay，也未通过发布门禁。",
  "Advisory only": "仅供评审参考",
  "ReviewerCat feedback is not a Release Gate decision. Promote the Case proposal, Replay it, then inspect the gate.":
    "ReviewerCat 的意见不是发布门禁结论；请评审 Case 建议、执行 Replay，再查看门禁结果。",
  Source: "来源",
  Queued: "排队中",
  Running: "运行中",
  Done: "已完成",
  Failed: "失败",
  Role: "角色",
  Skill: "技能",
  Memory: "记忆",
  "Proposal looks ready for verification": "方案可以进入验证",
  "Revise before verification": "验证前需要修改",
  "Review blocked by missing evidence": "缺少证据，无法完成评审",
  "Proposal rejected by ReviewerCat": "ReviewerCat 拒绝了该方案",
  "Agent evolution control plane": "Agent 持续进化控制平面",
  "Observe. Evolve. Verify.": "观测、进化、验证",
  "Use real OTLP Trace evidence to propose Agent improvements, then verify retained Cases with Barena Replay and Release Gate.":
    "基于真实 OTLP Trace 提出 Agent 改进方案，再通过 Barena Replay 与发布门禁验证已沉淀的 Case。",
  "Agent Registry": "Agent 资产",
  "OTLP-discovered": "OTLP 自动发现",
  "Agents are discovered from standard OpenTelemetry identity. Trace remains the execution evidence, not the asset registry.":
    "平台通过标准 OpenTelemetry 身份自动发现 Agent；Trace 仍然负责呈现单次执行证据，而不是重复管理资产。",
  "Connect Agent": "接入 Agent",
  "Last 30 days": "最近 30 天",
  Agent: "Agent",
  Runtime: "运行时",
  Deployment: "部署",
  Activity: "活动",
  "View traces": "查看 Trace",
  "No Agents discovered": "尚未发现 Agent",
  "Send one OTLP Trace and Barena will discover the Agent automatically.":
    "发送一条 OTLP Trace，Barena 就会自动发现对应 Agent。",
  "Open connection setup": "打开接入配置",
  "Observed through OTLP": "已通过 OTLP 观测",
  "{count} traces": "{count} 条 Trace",
  "Last seen {time}": "最近活跃于 {time}",
  "Not seen in recent sample": "近期样本中暂无活动",
  "Service {service}": "服务 {service}",
  "Version {version}": "版本 {version}",
  "Environment {environment}": "环境 {environment}",
  "Instance {instance}": "实例 {instance}",
  "Advanced role prompts": "高级 Role Prompt 配置",
  "Regression Cases": "回归用例",
  "Runtime health": "运行健康度",
  "Agents observed": "已观测 Agent",
  "Trace volume": "Trace 数量",
  Errors: "错误",
  Models: "模型",
  "No trace activity yet": "尚无 Trace 活动",
};

const messages: Record<BarenaLocale, Record<BarenaMessageKey, string>> = {
  en: enMessages,
  "zh-CN": zhCNMessages,
};

export function resolveBarenaLocale(
  storedLocale: string | null,
  browserLanguages: readonly string[],
): BarenaLocale {
  if (storedLocale === "en" || storedLocale === "zh-CN") {
    return storedLocale;
  }
  return browserLanguages[0]?.toLowerCase().startsWith("zh") ? "zh-CN" : "en";
}

export function translateBarenaMessage(
  locale: BarenaLocale,
  key: BarenaMessageKey,
  variables: Record<string, string | number> = {},
): string {
  return Object.entries(variables).reduce(
    (message, [name, value]) => message.replaceAll(`{${name}}`, String(value)),
    messages[locale][key],
  );
}

function getLocaleSnapshot(): BarenaLocale {
  if (typeof window === "undefined") return "en";
  return resolveBarenaLocale(
    window.localStorage.getItem(BARENA_LOCALE_STORAGE_KEY),
    window.navigator.languages?.length
      ? window.navigator.languages
      : [window.navigator.language],
  );
}

function subscribeToLocaleChange(onStoreChange: () => void) {
  if (typeof window === "undefined") return () => undefined;

  const handleStorage = (event: StorageEvent) => {
    if (event.key === BARENA_LOCALE_STORAGE_KEY) onStoreChange();
  };
  window.addEventListener("storage", handleStorage);
  window.addEventListener(LOCALE_CHANGE_EVENT, onStoreChange);

  return () => {
    window.removeEventListener("storage", handleStorage);
    window.removeEventListener(LOCALE_CHANGE_EVENT, onStoreChange);
  };
}

export function useBarenaI18n() {
  const locale = useSyncExternalStore(
    subscribeToLocaleChange,
    getLocaleSnapshot,
    (): BarenaLocale => "en",
  );

  useEffect(() => {
    document.documentElement.lang = locale;
  }, [locale]);

  const setLocale = useCallback((nextLocale: BarenaLocale) => {
    window.localStorage.setItem(BARENA_LOCALE_STORAGE_KEY, nextLocale);
    window.dispatchEvent(new Event(LOCALE_CHANGE_EVENT));
  }, []);

  const t = useCallback(
    (key: BarenaMessageKey, variables?: Record<string, string | number>) =>
      translateBarenaMessage(locale, key, variables),
    [locale],
  );

  return { locale, setLocale, t };
}
