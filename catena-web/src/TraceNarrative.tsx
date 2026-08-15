import { useMemo } from "react";
import type { TraceDetail, TraceSpan } from "./types";
import {
  buildTraceNarrative,
  presentTraceEvidence,
  shortTraceID,
  traceSpanAttributeString,
  traceSpanState,
  traceSpanTokenUsage,
  traceSpanToolName,
  traceSpanToolType,
  type TraceEvidencePresentation,
  type TraceNarrativeNode,
  type TraceSpanSemanticKind,
} from "./traceView";

type Locale = "zh" | "en";

const copy = {
  zh: {
    turn: "本轮任务",
    userRequest: "用户请求",
    finalAnswer: "最终回答",
    waitingAnswer: "Runtime 没有记录最终回答",
    execution: "执行过程",
    executionHint: "按真实父子关系排列；选择一步可查看完整证据",
    selectedEvidence: "所选步骤",
    evidenceHint: "选择左侧步骤查看完整输入、输出与属性。",
    input: "输入",
    output: "输出",
    attributes: "诊断属性",
    noEvidence: "这一步没有上传正文证据。",
    model: "模型调用",
    tool: "工具调用",
    artifact: "文件变更",
    subagentThread: "Subagent 线程",
    subagentTurn: "Subagent 任务",
    retry: "模型重试",
    compact: "上下文压缩",
    check: "结果验证",
    run: "测试运行",
    error: "未匹配或异常事件",
    turnStep: "Turn",
    internal: "内部步骤",
    parallel: "并行工具",
    serial: "工具",
    calls: "次调用",
    callID: "Call ID",
    tokens: "Token",
    inputTokens: "输入",
    outputTokens: "输出",
    duration: "耗时",
    state: {
      ok: "完成",
      error: "失败",
      aborted: "已中止",
      incomplete: "未完成",
      retry: "正在重试",
    },
    hiddenContext: "条上下文已折叠",
    hiddenFields: "个次要字段已折叠",
    truncated: "导出内容已截断",
    roles: { user: "用户", assistant: "Agent", system: "系统", tool: "工具" },
  },
  en: {
    turn: "This turn",
    userRequest: "User request",
    finalAnswer: "Final answer",
    waitingAnswer: "The Runtime recorded no final answer",
    execution: "Execution",
    executionHint: "Ordered by real parentage; select a step for complete evidence",
    selectedEvidence: "Selected step",
    evidenceHint: "Select a step on the left to inspect its full input, output, and attributes.",
    input: "Input",
    output: "Output",
    attributes: "Diagnostic attributes",
    noEvidence: "This step exported no body evidence.",
    model: "Model call",
    tool: "Tool call",
    artifact: "File change",
    subagentThread: "Subagent thread",
    subagentTurn: "Subagent task",
    retry: "Model retry",
    compact: "Context compact",
    check: "Result check",
    run: "Test run",
    error: "Unmatched or failed event",
    turnStep: "Turn",
    internal: "Internal step",
    parallel: "Parallel tools",
    serial: "Tools",
    calls: "calls",
    callID: "Call ID",
    tokens: "Tokens",
    inputTokens: "input",
    outputTokens: "output",
    duration: "Duration",
    state: {
      ok: "Complete",
      error: "Failed",
      aborted: "Aborted",
      incomplete: "Incomplete",
      retry: "Retry",
    },
    hiddenContext: "context items folded",
    hiddenFields: "secondary fields folded",
    truncated: "Exported evidence was truncated",
    roles: { user: "User", assistant: "Agent", system: "System", tool: "Tool" },
  },
} as const;

export function TraceNarrative({
  detail,
  locale,
  selectedSpanID,
  onSelectSpan,
}: {
  detail: TraceDetail;
  locale: Locale;
  selectedSpanID: string;
  onSelectSpan: (spanID: string) => void;
}) {
  const t = copy[locale];
  const narrative = useMemo(() => buildTraceNarrative(detail.spans), [detail.spans]);
  const selectedNode = useMemo(
    () => findNarrativeNode(narrative.roots, selectedSpanID) ?? narrative.primaryTurn ?? narrative.roots[0],
    [narrative, selectedSpanID],
  );
  const primaryTurn = narrative.primaryTurn;
  const flowRoots = primaryTurn
    ? [...primaryTurn.children, ...narrative.roots.filter((node) => node !== primaryTurn)]
    : narrative.roots;
  const state = primaryTurn ? traceSpanState(primaryTurn.span) : detail.summary.error_count > 0 ? "error" : "ok";

  return (
    <section className="trace-narrative" aria-label={t.execution}>
      <article className={`trace-turn-brief state-${safeState(state)}`}>
        <header>
          <div><span>{t.turn}</span><code>{primaryTurn ? shortTraceID(traceSpanAttributeString(primaryTurn.span, "agent.turn.id") || primaryTurn.span.span_id) : shortTraceID(detail.summary.trace_id)}</code></div>
          <StateBadge state={state} locale={locale} />
        </header>
        <div className="trace-turn-story">
          <section>
            <h3>{t.userRequest}</h3>
            <p>{primaryTurn?.span.input || "—"}</p>
          </section>
          <section>
            <h3>{t.finalAnswer}</h3>
            <p className={primaryTurn?.span.output ? "" : "muted"}>{primaryTurn?.span.output || t.waitingAnswer}</p>
          </section>
        </div>
      </article>

      <div className="trace-narrative-layout">
        <section className="trace-causal-flow">
          <header>
            <h3>{t.execution}</h3>
            <p>{t.executionHint}</p>
          </header>
          <div className="trace-causal-list">
            {flowRoots.map((node) => (
              <NarrativeNodeCard
                key={node.span.span_id}
                node={node}
                locale={locale}
                selectedSpanID={selectedNode?.span.span_id ?? ""}
                onSelectSpan={onSelectSpan}
              />
            ))}
          </div>
        </section>
        <aside className="trace-narrative-inspector" aria-live="polite">
          <header><span>{t.selectedEvidence}</span>{selectedNode ? <code>{shortTraceID(selectedNode.span.span_id)}</code> : null}</header>
          {selectedNode ? <NarrativeInspector node={selectedNode} locale={locale} /> : <p>{t.evidenceHint}</p>}
        </aside>
      </div>
    </section>
  );
}

function NarrativeNodeCard({
  node,
  locale,
  selectedSpanID,
  onSelectSpan,
}: {
  node: TraceNarrativeNode;
  locale: Locale;
  selectedSpanID: string;
  onSelectSpan: (spanID: string) => void;
}) {
  const t = copy[locale];
  const { span, kind } = node;
  const selected = span.span_id === selectedSpanID;
  const state = traceSpanState(span);
  const toolChildren = node.children.filter((child) => child.kind === "tool" || child.kind === "artifact");
  const otherChildren = node.children.filter((child) => child.kind !== "tool" && child.kind !== "artifact");
  const preview = narrativeStepPreview(span, kind, locale);
  const toolName = traceSpanToolName(span);
  const toolType = traceSpanToolType(span);
  const callID = traceSpanAttributeString(span, "gen_ai.tool.call.id", "tool.call.id");
  const usage = traceSpanTokenUsage(span);
  const duration = formatDuration(new Date(span.end_time).getTime() - new Date(span.start_time).getTime());

  return (
    <article className={`trace-narrative-node kind-${kind} state-${safeState(state)}`}>
      <button
        type="button"
        className={selected ? "trace-narrative-step selected" : "trace-narrative-step"}
        aria-pressed={selected}
        onClick={() => onSelectSpan(span.span_id)}
      >
        <span className="trace-narrative-step-head">
          <span className="trace-narrative-kind">{stepLabel(node, locale)}</span>
          <StateBadge state={state} locale={locale} />
        </span>
        <strong>{toolName || stepTitle(node, locale)}</strong>
        {preview ? <span className="trace-narrative-preview">{preview}</span> : null}
        <span className="trace-narrative-facts">
          {kind === "model" && usage.total > 0 ? <span>{t.tokens} {usage.total.toLocaleString()}</span> : null}
          {toolType ? <span>{toolType}</span> : null}
          {callID ? <span>{t.callID} {shortTraceID(callID)}</span> : null}
          <span>{duration}</span>
        </span>
        {(kind === "tool" || kind === "artifact") && (span.input || span.output) ? (
          <span className="trace-tool-glance">
            {span.input ? <span><b>{t.input}</b>{evidencePreview(presentTraceEvidence(span.input, kind, "input"))}</span> : null}
            {span.output ? <span><b>{t.output}</b>{evidencePreview(presentTraceEvidence(span.output, kind, "output"))}</span> : null}
          </span>
        ) : null}
      </button>

      {toolChildren.length > 0 ? (
        <section className={toolChildren.length > 1 ? "trace-tool-branch parallel" : "trace-tool-branch"}>
          <header><span>{toolChildren.length > 1 ? t.parallel : t.serial}</span><b>{toolChildren.length} {t.calls}</b></header>
          <div className="trace-tool-branch-grid">
            {toolChildren.map((child) => (
              <NarrativeNodeCard
                key={child.span.span_id}
                node={child}
                locale={locale}
                selectedSpanID={selectedSpanID}
                onSelectSpan={onSelectSpan}
              />
            ))}
          </div>
        </section>
      ) : null}
      {otherChildren.length > 0 ? (
        <div className="trace-narrative-children">
          {otherChildren.map((child) => (
            <NarrativeNodeCard
              key={child.span.span_id}
              node={child}
              locale={locale}
              selectedSpanID={selectedSpanID}
              onSelectSpan={onSelectSpan}
            />
          ))}
        </div>
      ) : null}
    </article>
  );
}

function NarrativeInspector({ node, locale }: { node: TraceNarrativeNode; locale: Locale }) {
  const t = copy[locale];
  const { span, kind } = node;
  const state = traceSpanState(span);
  const usage = traceSpanTokenUsage(span);
  const diagnosticAttributes = Object.fromEntries(Object.entries(span.attributes).filter(([key]) => ![
    "input.value",
    "output.value",
    "gen_ai.tool.call.arguments",
    "gen_ai.tool.call.result",
    "tool.call.arguments",
    "tool.call.result",
  ].includes(key)));
  return (
    <div className="trace-narrative-inspector-body">
      <div className="trace-narrative-inspector-title">
        <div><span>{stepLabel(node, locale)}</span><strong>{traceSpanToolName(span) || stepTitle(node, locale)}</strong></div>
        <StateBadge state={state} locale={locale} />
      </div>
      <dl className="trace-narrative-inspector-facts">
        <div><dt>{t.duration}</dt><dd>{formatDuration(new Date(span.end_time).getTime() - new Date(span.start_time).getTime())}</dd></div>
        {usage.total > 0 ? <div><dt>{t.tokens}</dt><dd>{usage.total.toLocaleString()} · {t.inputTokens} {usage.input.toLocaleString()} · {t.outputTokens} {usage.output.toLocaleString()}</dd></div> : null}
        {traceSpanAttributeString(span, "gen_ai.tool.call.id") ? <div><dt>{t.callID}</dt><dd><code>{traceSpanAttributeString(span, "gen_ai.tool.call.id")}</code></dd></div> : null}
      </dl>
      {span.status_code === 2 && span.status_message ? <p className="trace-span-error">{span.status_message}</p> : null}
      <div className="trace-narrative-evidence">
        {span.input ? <NarrativeEvidenceBlock title={t.input} presentation={presentTraceEvidence(span.input, kind, "input")} locale={locale} /> : null}
        {span.output ? <NarrativeEvidenceBlock title={t.output} presentation={presentTraceEvidence(span.output, kind, "output")} locale={locale} /> : null}
        {!span.input && !span.output ? <p className="trace-no-evidence">{t.noEvidence}</p> : null}
      </div>
      {Object.keys(diagnosticAttributes).length > 0 ? (
        <details className="trace-attributes"><summary>{t.attributes}</summary><pre>{JSON.stringify(diagnosticAttributes, null, 2)}</pre></details>
      ) : null}
    </div>
  );
}

function NarrativeEvidenceBlock({
  title,
  presentation,
  locale,
}: {
  title: string;
  presentation: TraceEvidencePresentation;
  locale: Locale;
}) {
  const t = copy[locale];
  return (
    <section className={`trace-evidence-block ${presentation.kind}`}>
      <h4>{title}</h4>
      {presentation.kind === "messages" ? (
        <div className="trace-evidence-messages">
          {presentation.messages.map((message, index) => (
            <article className={`trace-evidence-message ${message.role}`} key={`${message.role}-${index}`}>
              <span>{t.roles[message.role]}</span><p>{message.text}</p>
            </article>
          ))}
        </div>
      ) : null}
      {presentation.kind === "fields" ? (
        <dl className="trace-evidence-fields">
          {presentation.fields.map((field) => <div key={field.key}><dt>{field.key}</dt><dd className={field.code ? "code" : ""}>{field.value}</dd></div>)}
        </dl>
      ) : null}
      {presentation.kind === "terminal" ? <pre className="trace-evidence-terminal">{presentation.text}</pre> : null}
      {presentation.kind === "text" ? <p className="trace-evidence-text">{presentation.text}</p> : null}
      {presentation.hiddenContextCount > 0 ? <p className="trace-evidence-folded"><strong>{presentation.hiddenContextCount}</strong> {t.hiddenContext}</p> : null}
      {presentation.hiddenFieldCount > 0 ? <p className="trace-evidence-folded"><strong>{presentation.hiddenFieldCount}</strong> {t.hiddenFields}</p> : null}
      {presentation.truncated ? <p className="trace-evidence-warning">{t.truncated}</p> : null}
    </section>
  );
}

function StateBadge({ state, locale }: { state: string; locale: Locale }) {
  const t = copy[locale];
  const normalized = safeState(state);
  const label = t.state[normalized as keyof typeof t.state] ?? state;
  return <span className={`trace-node-state state-${normalized}`}>{label}</span>;
}

function stepLabel(node: TraceNarrativeNode, locale: Locale) {
  const t = copy[locale];
  if (node.kind === "model") {
    const index = Number(traceSpanAttributeString(node.span, "catena.model.step.index"));
    return Number.isFinite(index) ? `${t.model} ${index + 1}` : t.model;
  }
  if (node.kind === "tool") return t.tool;
  if (node.kind === "artifact") return t.artifact;
  if (node.kind === "subagent") return node.span.name.endsWith(".thread") ? t.subagentThread : t.subagentTurn;
  if (node.kind === "retry") return t.retry;
  if (node.kind === "compact") return t.compact;
  if (node.kind === "check") return t.check;
  if (node.kind === "run") return t.run;
  if (node.kind === "error") return t.error;
  if (node.kind === "turn") return t.turnStep;
  return t.internal;
}

function stepTitle(node: TraceNarrativeNode, locale: Locale) {
  const { span, kind } = node;
  if (kind === "model") return span.model || traceSpanAttributeString(span, "gen_ai.request.model", "gen_ai.response.model") || stepLabel(node, locale);
  if (kind === "subagent") return traceSpanAttributeString(span, "agent.subagent.thread.id", "agent.turn.id") || stepLabel(node, locale);
  if (kind === "compact") {
    const evidence = span.input ? presentTraceEvidence(span.input, kind, "input") : undefined;
    return evidence ? evidencePreview(evidence) : stepLabel(node, locale);
  }
  return span.name || stepLabel(node, locale);
}

function narrativeStepPreview(span: TraceSpan, kind: TraceSpanSemanticKind, locale: Locale) {
  if (kind === "model" && span.output) {
    const parsed = parseRecord(span.output);
    const content = typeof parsed?.content === "string" ? parsed.content : "";
    const calls = Array.isArray(parsed?.tool_calls) ? parsed.tool_calls.length : 0;
    if (content && calls) return `${content} · ${calls} ${copy[locale].calls}`;
    if (content) return content;
    if (calls) return `${calls} ${copy[locale].calls}`;
  }
  if ((kind === "subagent" || kind === "turn") && (span.input || span.output)) return span.input || span.output || "";
  if (kind === "retry") return span.status_message || traceSpanAttributeString(span, "http.status_code", "error.type") || span.name;
  return "";
}

function evidencePreview(presentation: TraceEvidencePresentation) {
  const value = presentation.fields.length > 0
    ? presentation.fields.slice(0, 2).map((field) => `${field.key}: ${field.value}`).join(" · ")
    : presentation.messages.at(-1)?.text || presentation.text;
  return truncate(value.replace(/\s+/g, " ").trim(), 220);
}

function findNarrativeNode(nodes: TraceNarrativeNode[], spanID: string): TraceNarrativeNode | undefined {
  for (const node of nodes) {
    if (node.span.span_id === spanID) return node;
    const child = findNarrativeNode(node.children, spanID);
    if (child) return child;
  }
  return undefined;
}

function parseRecord(value: string): Record<string, unknown> | undefined {
  try {
    const parsed = JSON.parse(value);
    return typeof parsed === "object" && parsed !== null && !Array.isArray(parsed) ? parsed : undefined;
  } catch {
    return undefined;
  }
}

function truncate(value: string, limit: number) {
  return value.length > limit ? `${value.slice(0, limit)}…` : value;
}

function safeState(value: string) {
  const state = value.trim().toLocaleLowerCase();
  return ["ok", "error", "aborted", "incomplete", "retry"].includes(state) ? state : state || "ok";
}

function formatDuration(value: number) {
  if (!Number.isFinite(value) || value <= 0) return "0 ms";
  if (value < 1_000) return `${Math.max(1, Math.round(value))} ms`;
  if (value < 60_000) return `${(value / 1_000).toFixed(value < 10_000 ? 2 : 1)} s`;
  return `${(value / 60_000).toFixed(1)} min`;
}
