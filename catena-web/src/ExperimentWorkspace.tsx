import { useEffect, useRef, useState } from "react";
import { api } from "./api";
import { experimentArm, experimentArms, expectedTrials, isExperimentActive, type Experiment, type ExperimentCase } from "./experiments";

function analysisText(value?: string): string {
  if (!value) return "";
  try {
    const parsed = JSON.parse(value);
    return typeof parsed.summary === "string" ? parsed.summary : typeof parsed.comparison?.summary === "string" ? parsed.comparison.summary : "";
  } catch { return value; }
}

function TrialArtifact({ experimentID, index, label, zh }: { experimentID: string; index: number; label: string; zh: boolean }) {
  const [evidence, setEvidence] = useState<{ answer: string; calls: { command: string; exit_code: number | null; stdout: string; stderr: string }[] } | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  async function load() {
    setLoading(true); setError("");
    try { setEvidence(await api.experimentEvidence(experimentID, index)); }
    catch (cause) { setError(cause instanceof Error ? cause.message : "Unable to read artifacts"); }
    finally { setLoading(false); }
  }
  return <details className="experiment-scope"><summary>{label}</summary>
    {!evidence && <button type="button" disabled={loading} onClick={() => void load()}>{loading ? (zh ? "读取中…" : "Loading…") : (zh ? "读取工具日志和最终答案" : "Read tool logs and final answer")}</button>}
    {error && <p role="alert">{error}</p>}
    {evidence && <><p>{zh ? "日志经过长度限制和敏感信息遮盖。完整产物保存在执行端。" : "Logs are bounded and redacted. Full artifacts remain on the worker."}</p>
      {evidence.calls.map((call, i) => <div key={i}><strong>{i + 1}. {zh ? "退出码" : "Exit code"}: {call.exit_code ?? "—"}</strong><pre style={{ whiteSpace: "pre-wrap", overflowWrap: "anywhere" }}>{call.command}{"\n"}{call.stdout}{call.stderr}</pre></div>)}
      <strong>{zh ? "最终答案" : "Final answer"}</strong><pre style={{ whiteSpace: "pre-wrap", overflowWrap: "anywhere" }}>{evidence.answer}</pre>
    </>}
  </details>;
}

export function ExperimentWorkspace({ locale }: { locale: "zh" | "en" }) {
  const zh = locale === "zh";
  const [cases, setCases] = useState<ExperimentCase[]>([]);
  const [experiments, setExperiments] = useState<Experiment[]>([]);
  const [selectedID, setSelectedID] = useState(() => new URLSearchParams(window.location.search).get("experiment") ?? "");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [version, setVersion] = useState(0);
  const [includeMemory, setIncludeMemory] = useState(false);
  const [targetAgent, setTargetAgent] = useState("codex");
  const requestID = useRef<string | null>(null);
  const submitLock = useRef(false);
  const mounted = useRef(true);
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; }; }, []);
  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    async function load() {
      try {
        const [catalog, history] = await Promise.all([api.experimentCases(controller.signal), api.experiments(controller.signal)]);
        if (controller.signal.aborted) return;
        setCases(catalog.cases); setExperiments(history.experiments); setError("");
        if (history.experiments.some(isExperimentActive)) timer = setTimeout(() => void load(), 5000);
      } catch (cause) {
        if (controller.signal.aborted) return;
        setError(cause instanceof Error ? cause.message : "Unable to load experiments");
        timer = setTimeout(() => void load(), 10000);
      } finally { if (!controller.signal.aborted) setLoading(false); }
    }
    void load();
    return () => { controller.abort(); clearTimeout(timer); };
  }, [version]);
  function select(id: string) {
    setSelectedID(id);
    const url = new URL(window.location.href); url.searchParams.set("experiment", id);
    window.history.replaceState({}, "", url);
  }
  async function start(caseID: string) {
    if (submitLock.current) return;
    submitLock.current = true; setSubmitting(true); setError("");
    requestID.current ??= crypto.randomUUID();
    try {
      const experiment = await api.createExperiment(caseID, requestID.current, includeMemory, targetAgent);
      if (!mounted.current) return;
      requestID.current = null;
      setExperiments((current) => [experiment, ...current.filter((item) => item.experiment_id !== experiment.experiment_id)]);
      select(experiment.experiment_id); setVersion((n) => n + 1);
    } catch (cause) {
      if (mounted.current) setError(cause instanceof Error ? cause.message : "Unable to start experiment");
    } finally { submitLock.current = false; if (mounted.current) setSubmitting(false); }
  }
  const selected = experiments.find((item) => item.experiment_id === selectedID) ?? experiments[0];
  const analysis = analysisText(selected?.summary);
  const active = experiments.some(isExperimentActive);
  const labels: Record<string, string> = zh ? { queued: "等待确认", running: "正在运行", completed: "验证完成", invalid: "证据无效", failed: "执行失败", blocked: "无法执行" } : { queued: "Pending", running: "Running", completed: "Verified", invalid: "Invalid evidence", failed: "Failed", blocked: "Blocked" };
  const baseline = selected && experimentArm(selected, "baseline");
  const candidate = selected && experimentArm(selected, "candidate");
  const arms = selected ? experimentArms(selected) : [];
  const total = selected ? expectedTrials(selected) : 6;
  const perArm = selected ? 3 * (selected.attempts ?? 1) : 3;
  const armLabel = (arm: string) => arm === "baseline" ? (zh ? "原始 Agent" : "Baseline") : arm === "memory" ? (zh ? "仅记忆" : "Memory only") : (zh ? "仅 Skill" : "Skill only");
  return <section className="page experiment-page">
    <header className="page-header"><span className="proactive-eyebrow">{zh ? "工作空间 / 验证" : "Workspace / Evaluation"}</span><h1>{zh ? "Case 实验" : "Case experiments"}</h1><p>{zh ? "从真实问题出发，用对照实验验证 Skill。" : "Verify Skills with controlled experiments based on real problems."}</p></header>
    {error && <div role="alert" className="experiment-notice">{error} <button type="button" onClick={() => setVersion((n) => n + 1)}>{zh ? "刷新" : "Refresh"}</button></div>}
    {loading && <p role="status">{zh ? "正在读取实验记录…" : "Loading experiments…"}</p>}
    <div className="experiment-workbench">
    <aside className="experiment-sidebar">
    <div className="experiment-catalog">{cases.map((item) => <article className="experiment-card" key={item.case_id}>
      <div className="experiment-eyebrow"><span className="experiment-dot" />{zh ? "可运行案例" : "Ready to run"}</div>
      <h2>{zh ? item.title : "Recover when a search tool is missing"}</h2>
      <p>{zh ? item.description : "Three frozen search tasks, compared with and without the search Skill."}</p>
      <label>{zh ? "被测 Agent" : "Target Agent"}<select value={targetAgent} disabled={submitting || active} onChange={(event) => { setTargetAgent(event.target.value); setIncludeMemory(false); requestID.current = null; }}><option value="codex">Codex CLI</option><option value="sdk">{zh ? "SDK 搜索 Agent（接入测试）" : "SDK search Agent (integration test)"}</option></select></label>
      <dl className="experiment-facts"><div><dt>{zh ? "被测 Agent" : "Target"}</dt><dd>{targetAgent === "codex" ? "Codex CLI" : (zh ? "SDK 搜索 Agent" : "SDK search Agent")}</dd></div><div><dt>{zh ? "实验环境" : "Environment"}</dt><dd>{item.environment}</dd></div><div><dt>{zh ? "试验规模" : "Trials"}</dt><dd>{includeMemory ? (zh ? "3 题 × 3 组 × 2 次，共 18 次" : "3 tasks × 3 arms × 2 attempts, 18 trials") : (zh ? "3 题 × 2 组，共 6 次" : "3 tasks × 2 arms, 6 trials")}</dd></div></dl>
      {targetAgent === "sdk" && <label><input type="checkbox" checked={includeMemory} disabled={submitting || active} onChange={(event) => { setIncludeMemory(event.target.checked); requestID.current = null; }} />{zh ? "加入历史记忆组，并重复两次" : "Add historical memory arm and repeat twice"}</label>}
      <details className="experiment-scope"><summary>{zh ? "案例范围与限制" : "Scope and limitations"}</summary><p className="experiment-muted">{zh ? item.limitation : "Reconstructed Linux/PowerShell fixtures, not a replay of the original Windows sessions. Results apply only to the selected Agent and these tasks."}</p></details>
      {item.evidence_summary && <details className="experiment-scope"><summary>{zh ? "真实问题的来源" : "Source evidence"}</summary><p>{zh ? item.evidence_summary : "Five independently reviewed Codex sessions contain missing-rg tool errors followed by native PowerShell recovery. This does not establish overall task failure."}</p>{item.source_trace_id && <a href={`/traces?trace=${encodeURIComponent(item.source_trace_id)}`}>{zh ? "查看来源 Trace" : "Open source Trace"}</a>}</details>}
      <button type="button" className="primary-button" disabled={submitting || active} onClick={() => void start(item.case_id)}>{submitting ? (zh ? "正在提交…" : "Submitting…") : active ? (zh ? "等待当前实验完成" : "Experiment in progress") : (zh ? "发起对照实验" : "Run comparison")}</button>
      <small>{zh ? "使用「连接」页保存的模型；各组使用同一配置。记忆组冻结召回内容，Skill 组只加载 Skill。运行会消耗模型额度。" : "All arms use the saved model. Memory is frozen at submission; Skill is a separate intervention. Runs consume model usage."}</small>
    </article>)}</div>
      <div className="experiment-history"><h2>{zh ? "实验记录" : "History"}<span>{experiments.length}</span></h2>{!loading && experiments.length === 0 && <p className="experiment-muted">{zh ? "还没有实验记录" : "No experiments yet"}</p>}
        {experiments.map((item) => <button type="button" key={item.experiment_id} aria-pressed={selected?.experiment_id === item.experiment_id} onClick={() => select(item.experiment_id)}><strong>{labels[item.state] ?? item.state}</strong><span>{new Date(item.created_at).toLocaleString(zh ? "zh-CN" : "en-US")}</span><span>{item.model} · {item.trials.length}/{expectedTrials(item)}</span></button>)}
      </div>
      </aside>
      {!selected && !loading && <article className="experiment-result experiment-empty"><span className="experiment-empty-symbol">↗</span><h2>{zh ? "让结论有证据" : "Put evidence behind the conclusion"}</h2><p>{zh ? "选择左侧案例发起实验。这里会展示原始 Agent 与加载 Skill 后的表现、成本和验证证据。" : "Run a case to compare baseline and Skill performance, cost and evidence here."}</p><div className="experiment-empty-steps"><span>01 · {zh ? "冻结任务" : "Freeze tasks"}</span><span>02 · {zh ? "对照运行" : "Compare runs"}</span><span>03 · {zh ? "验证结论" : "Verify results"}</span></div></article>}
      {selected && baseline && candidate && <article className="experiment-result" aria-label={zh ? "实验结果" : "Experiment result"}>
        <div className="experiment-result-heading"><div><span className={`experiment-status ${selected.state}`}>{labels[selected.state] ?? selected.state}</span><h2>{zh ? "对照实验结果" : "Comparison results"}</h2></div><button type="button" onClick={() => setVersion((n) => n + 1)}>{zh ? "刷新" : "Refresh"}</button></div>
        <p className="experiment-result-meta">{selected.target_agent === "codex" ? "Codex CLI" : "SDK 搜索 Agent"} · {selected.model} · {selected.environment} · {new Date(selected.created_at).toLocaleString(zh ? "zh-CN" : "en-US")}</p>
        {selected.target_agent === "codex" && selected.trials.length > 0 && <p className="experiment-muted">{zh ? "原生会话与缺少 rg 的环境已核对：" : "Native session and missing-rg environment verified: "}{selected.trials.filter((trial) => trial.native_session_verified && trial.environment_verified).length}/{selected.trials.length}</p>}
        <div className="experiment-highlights"><div><span>{zh ? "有效试验" : "Valid trials"}</span><strong>{arms.reduce((n, arm) => n + experimentArm(selected, arm).valid, 0)}<small> / {total}</small></strong></div><div><span>{zh ? "各组答案通过" : "Answers passed per arm"}</span><strong>{arms.map((arm) => experimentArm(selected, arm).passed).join(" / ")}</strong></div><div><span>{zh ? "各组缺失工具错误" : "Missing-tool errors per arm"}</span><strong>{arms.map((arm) => { const result = experimentArm(selected, arm); return result.valid ? result.errors : "—"; }).join(" / ")}</strong></div></div>
        {isExperimentActive(selected) && <div role="status"><progress max={total} value={selected.trials.length} /><p>{zh ? `已收集 ${selected.trials.length}/${total} 次结果。离开页面后执行仍会继续。` : `${selected.trials.length}/${total} results collected. Execution continues after leaving this page.`}</p></div>}
        {selected.message && <p className="experiment-notice">{selected.message}</p>}
        <div className="experiment-table-scroll"><table><caption>{zh ? "有效试验的对照结果" : "Valid trial comparison"}</caption><thead><tr><th>{zh ? "指标" : "Metric"}</th>{arms.map((arm) => <th key={arm}>{armLabel(arm)}</th>)}</tr></thead><tbody>
          <tr><th>{zh ? "有效试验" : "Valid trials"}</th>{arms.map((arm) => <td key={arm}>{experimentArm(selected, arm).valid}/{perArm}</td>)}</tr>
          <tr><th>{zh ? "答案通过" : "Answers passed"}</th>{arms.map((arm) => { const result = experimentArm(selected, arm); return <td key={arm}>{result.valid ? `${result.passed}/${result.valid}` : "—"}</td>; })}</tr>
          {([[zh ? "缺失工具错误" : "Missing-tool errors", "errors"], [zh ? "工具调用" : "Tool calls", "commands"], [zh ? "输入令牌" : "Input tokens", "input"], [zh ? "输出令牌" : "Output tokens", "output"], [zh ? "累计执行秒数" : "Total execution seconds", "seconds"]] as const).map(([label, key]) => <tr key={key}><th>{label}</th>{arms.map((arm) => { const result = experimentArm(selected, arm); return <td key={arm}>{result.valid && result[key] !== null ? result[key]!.toLocaleString() : "—"}</td>; })}</tr>)}
        </tbody></table></div>
        {selected.state === "completed" && <p className="experiment-verdict">{arms.every((arm) => experimentArm(selected, arm).passed === baseline.passed) ? (zh ? "各组答案通过率持平，不能据此声称记忆或 Skill 提升了成功率。" : "Answer pass rates are equal; this does not establish a success-rate improvement.") : (zh ? "本次观察到组间通过率差异；样本很小，需要重复验证。" : "A difference was observed in this small sample; repeat trials are needed.")}</p>}
        <p className="experiment-muted">{zh ? `每组每题运行 ${selected.attempts ?? 1} 次。样本较小，不作统计显著性结论；运行异常与证据无效不能计为成功。耗时包括模型和工具调用，不包括容器构建。` : `${selected.attempts ?? 1} attempt(s) per task per arm. Small sample; no statistical significance established. Duration excludes container builds.`}</p>
        {selected.memory_context?.length ? <details><summary>{zh ? "本次冻结的记忆上下文" : "Frozen memory context"}</summary>{selected.memory_context.map((item) => <div key={item.id}><strong>{item.title ?? item.id}</strong><p>{item.content}</p><small>{zh ? "召回分数" : "Retrieval score"}: {item.score.toFixed(3)} · {item.id}</small></div>)}</details> : null}
        {analysis && <div className="experiment-verdict"><strong>{zh ? "平台 Agent 分析" : "Platform Agent analysis"}</strong><p>{analysis}</p></div>}
        {selected.trials.length > 0 && <details><summary>{zh ? "原始执行日志" : "Execution logs"}</summary>{selected.trials.map((trial, i) => <TrialArtifact key={`${selected.experiment_id}-${i}`} experimentID={selected.experiment_id} index={i} label={`${armLabel(trial.variant)} · ${trial.task} · ${trial.attempt ?? 1}`} zh={zh} />)}</details>}
        <details><summary>{zh ? "逐次验证证据" : "Trial evidence"}</summary><div className="experiment-table-scroll"><table><thead><tr><th>{zh ? "任务" : "Task"}</th><th>{zh ? "组别 / 次数" : "Arm / attempt"}</th><th>{zh ? "有效" : "Valid"}</th><th>{zh ? "奖励" : "Reward"}</th><th>{zh ? "执行秒数" : "Seconds"}</th></tr></thead><tbody>{selected.trials.map((trial, i) => <tr key={`${trial.variant}-${trial.task}-${i}`}><td>{trial.task}</td><td>{armLabel(trial.variant)} / {trial.attempt ?? 1}</td><td>{trial.valid ? "✓" : "—"}</td><td>{trial.valid ? String(trial.rewards?.reward ?? "—") : "—"}</td><td>{trial.duration_seconds ?? "—"}</td></tr>)}</tbody></table></div><p>{zh ? "任务文件保持冻结：" : "Task files unchanged: "}{selected.tasks_unchanged ? "✓" : (zh ? "尚未确认" : "Not confirmed")}</p><p className="experiment-muted">{selected.experiment_id}{selected.harbor_job_id && ` · Harbor ${selected.harbor_job_id}`}</p></details>
      </article>}
    </div>
  </section>;
}
