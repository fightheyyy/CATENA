import type {
  EvolutionCandidate,
  EvolutionCandidateKind,
  EvolutionBoundary,
  EvolutionCaseProposal,
  EvolutionFinding,
  EvolutionJob,
  EvolutionReview,
  EvolutionStage,
} from "./types";

const DRAFT_STATUS = "draft/unverified";

const stageDefinitions = [
  { name: "inspector", role: "inspector-cat" },
  { name: "evolution", role: "evolution-cat" },
  { name: "reviewer", role: "reviewer-cat" },
] as const;

function record(value: unknown): Record<string, unknown> | null {
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? value as Record<string, unknown>
    : null;
}

function stringValue(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function stringArray(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.map(stringValue).filter(Boolean);
}

function firstValue(source: Record<string, unknown>, ...keys: string[]): unknown {
  for (const key of keys) {
    if (source[key] !== undefined && source[key] !== null) return source[key];
  }
  return undefined;
}

function firstRecord(source: Record<string, unknown>, ...keys: string[]): Record<string, unknown> | null {
  return record(firstValue(source, ...keys));
}

function candidateKind(value: unknown): EvolutionCandidateKind {
  const kind = stringValue(value).toLowerCase();
  if (kind === "agent_md" || kind === "memory" || kind === "skill" || kind === "role" || kind === "harness" || kind === "case") {
    return kind;
  }
  return "unknown";
}

function normalizeStage(value: unknown, index: number): EvolutionStage | null {
  const source = record(value);
  if (!source) return null;
  const fallback = stageDefinitions[index];
  const role = stringValue(firstValue(source, "role", "actor")) || fallback?.role || "";
  const name = stringValue(firstValue(source, "name", "stage")) || fallback?.name || role.replace(/-cat$/, "");
  if (!name && !role) return null;
  return {
    name,
    role,
    state: stringValue(firstValue(source, "state", "status")) || "not_reported",
    raw_output: firstValue(source, "raw_output", "output", "result"),
    error: stringValue(source.error) || undefined,
    started_at: stringValue(source.started_at) || undefined,
    finished_at: stringValue(source.finished_at) || undefined,
  };
}

function normalizeFinding(value: unknown): EvolutionFinding | undefined {
  const source = record(value);
  if (!source) return undefined;
  const title = stringValue(source.title);
  const summary = stringValue(source.summary);
  const evidence = stringArray(firstValue(source, "evidence", "evidence_items"));
  if (!title && !summary && evidence.length === 0) return undefined;
  return {
    title,
    summary,
    severity: stringValue(source.severity) || "unknown",
    evidence,
  };
}

function normalizeCaseProposal(value: unknown): EvolutionCaseProposal | undefined {
  const source = record(value);
  if (!source) return undefined;
  const title = stringValue(source.title);
  const replayPrompt = stringValue(firstValue(source, "replay_prompt", "prompt"));
  const successCriteria = stringValue(firstValue(source, "success_criteria", "expected_behavior"));
  if (!title && !replayPrompt && !successCriteria) return undefined;
  return {
    candidate_id: stringValue(firstValue(source, "candidate_id", "id")) || undefined,
    kind: stringValue(source.kind) === "case" ? "case" : undefined,
    title,
    replay_prompt: replayPrompt,
    success_criteria: successCriteria,
    verifier: firstValue(source, "verifier", "verification"),
    requires_human_review: typeof source.requires_human_review === "boolean"
      ? source.requires_human_review
      : undefined,
    status: stringValue(firstValue(source, "status", "candidate_status")) || DRAFT_STATUS,
    source_trace_id: stringValue(source.source_trace_id) || undefined,
    source_trace_ids: stringArray(source.source_trace_ids),
    source_agent_id: stringValue(source.source_agent_id) || undefined,
    source_run_id: stringValue(source.source_run_id) || undefined,
    evidence_pack_sha256: stringValue(firstValue(source, "evidence_pack_sha256", "evidence_digest")) || undefined,
  };
}

function normalizeCandidate(value: unknown, fallbackStatus: string): EvolutionCandidate | undefined {
  const source = record(value);
  if (!source) return undefined;
  const kind = candidateKind(firstValue(source, "kind", "type"));
  const title = stringValue(source.title);
  const summary = stringValue(firstValue(source, "summary", "description"));
  if (!title && !summary && kind === "unknown") return undefined;
  return {
    candidate_id: stringValue(firstValue(source, "candidate_id", "id")),
    kind,
    title,
    summary,
    content: firstValue(source, "content", "proposal", "spec"),
    status: stringValue(firstValue(source, "status", "candidate_status")) || fallbackStatus || DRAFT_STATUS,
    source_trace_id: stringValue(source.source_trace_id) || undefined,
    source_trace_ids: stringArray(source.source_trace_ids),
    source_agent_id: stringValue(source.source_agent_id) || undefined,
    source_run_id: stringValue(source.source_run_id) || undefined,
    evidence_pack_sha256: stringValue(firstValue(source, "evidence_pack_sha256", "evidence_digest")) || undefined,
  };
}

function normalizeBoundary(value: unknown): EvolutionBoundary | undefined {
  const source = record(value);
  if (!source) return undefined;
  const hasBoundaryField = [
    "target_agent_executed_by_catena",
    "creates_release",
    "release_authority",
    "candidate_status",
    "review_scope",
  ].some((key) => source[key] !== undefined);
  if (!hasBoundaryField) return undefined;
  return {
    target_agent_executed_by_catena: source.target_agent_executed_by_catena === true,
    creates_release: source.creates_release === true,
    release_authority: stringValue(source.release_authority),
    candidate_status: stringValue(source.candidate_status) || DRAFT_STATUS,
    review_scope: stringValue(source.review_scope) || "proposal_only",
  };
}

function normalizeReview(value: unknown): EvolutionReview | undefined {
  const source = record(value);
  if (!source) return undefined;
  const verdict = stringValue(firstValue(source, "verdict", "decision"));
  const summary = stringValue(firstValue(source, "summary", "reason"));
  if (!verdict && !summary) return undefined;
  return {
    verdict: verdict || "not_reported",
    summary,
    scope: stringValue(source.scope) || "proposal_only",
    candidate_status: stringValue(source.candidate_status) || DRAFT_STATUS,
  };
}

function stageEmbeddedOutput(stages: EvolutionStage[], stageName: string, key: string): unknown {
  const stage = stages.find((item) => item.name === stageName || item.role === `${stageName}-cat`);
  const raw = stage?.raw_output;
  if (typeof raw === "string") {
    try {
      return record(JSON.parse(raw))?.[key];
    } catch {
      return undefined;
    }
  }
  return record(raw)?.[key];
}

export function normalizeEvolutionJob(value: unknown): EvolutionJob {
  const source = record(value);
  const jobID = source ? stringValue(firstValue(source, "job_id", "id")) : "";
  if (!source || !jobID) throw new Error("Evolution Job response is missing job_id");

  const rawStages = Array.isArray(firstValue(source, "stages", "stage_outputs"))
    ? firstValue(source, "stages", "stage_outputs") as unknown[]
    : [];
  const stages = rawStages.map(normalizeStage).filter((item): item is EvolutionStage => item !== null);
  const reviewSource = firstValue(source, "review", "reviewer_output")
    ?? stageEmbeddedOutput(stages, "reviewer", "review");
  const review = normalizeReview(reviewSource);
  const boundary = normalizeBoundary(source.boundary);
  const fallbackStatus = review?.candidate_status || boundary?.candidate_status || stringValue(source.candidate_status) || DRAFT_STATUS;

  const candidateValues: unknown[] = [];
  if (Array.isArray(source.candidates)) candidateValues.push(...source.candidates);
  if (source.candidate !== undefined) candidateValues.push(source.candidate);
  const embeddedCandidate = stageEmbeddedOutput(stages, "evolution", "candidate");
  if (embeddedCandidate !== undefined) candidateValues.push(embeddedCandidate);
  const candidates = candidateValues
    .map((candidate) => normalizeCandidate(candidate, fallbackStatus))
    .filter((candidate): candidate is EvolutionCandidate => candidate !== undefined && candidate.kind !== "case")
    .filter((candidate, index, all) => {
      const identity = candidate.candidate_id || `${candidate.kind}:${candidate.title}:${candidate.summary}`;
      return all.findIndex((item) => (item.candidate_id || `${item.kind}:${item.title}:${item.summary}`) === identity) === index;
    });

  const caseFromCandidates = candidateValues.find((candidate) => candidateKind(record(candidate)?.kind) === "case");
  const caseSource = firstValue(source, "case_proposal", "case_candidate")
    ?? caseFromCandidates
    ?? stageEmbeddedOutput(stages, "inspector", "case_proposal");
  const findingSource = firstValue(source, "finding", "inspector_finding")
    ?? stageEmbeddedOutput(stages, "inspector", "finding");

  return {
    schema: stringValue(source.schema) || undefined,
    job_id: jobID,
    source_kind: stringValue(source.source_kind) === "agent_trace_set"
      ? "agent_trace_set"
      : stringValue(source.source_kind) === "run_trace"
        ? "run_trace"
        : stringValue(source.source_kind) === "trace"
          ? "trace"
          : undefined,
    source_run_id: stringValue(firstValue(source, "source_run_id", "run_id")) || undefined,
    source_trace_id: stringValue(firstValue(source, "source_trace_id", "trace_id")) || undefined,
    source_trace_ids: stringArray(source.source_trace_ids),
    source_agent_id: stringValue(source.source_agent_id) || undefined,
    window_start: stringValue(source.window_start) || undefined,
    window_end: stringValue(source.window_end) || undefined,
    objective: stringValue(source.objective) || undefined,
    state: stringValue(firstValue(source, "state", "status")) || "not_reported",
    current_stage: stringValue(firstValue(source, "current_stage", "stage")) || undefined,
    stages,
    finding: normalizeFinding(findingSource),
    case_proposal: normalizeCaseProposal(caseSource),
    candidates,
    review,
    error: stringValue(source.error) || undefined,
    evidence_pack: firstRecord(source, "evidence_pack", "evidence") ?? undefined,
    boundary,
    created_at: stringValue(source.created_at),
    updated_at: stringValue(source.updated_at),
  };
}

export function normalizeEvolutionJobs(value: unknown): EvolutionJob[] {
  if (!Array.isArray(value)) return [];
  const jobs: EvolutionJob[] = [];
  for (const item of value) {
    try {
      jobs.push(normalizeEvolutionJob(item));
    } catch {
      // One malformed legacy record must not hide the rest of the workspace.
    }
  }
  return jobs;
}

export function evolutionStages(job: EvolutionJob): EvolutionStage[] {
  const known = stageDefinitions.map((definition) => {
    const actual = job.stages.find((stage) => stage.name === definition.name || stage.role === definition.role);
    return actual ?? {
      ...definition,
      state: job.state === "queued" ? "queued" : "not_reported",
    };
  });
  const additional = job.stages.filter((stage) => !stageDefinitions.some(
    (definition) => stage.name === definition.name || stage.role === definition.role,
  ));
  return [...known, ...additional];
}

export function isEvolutionJobTerminal(job: EvolutionJob): boolean {
  return job.state === "completed" || job.state === "failed";
}

export function evolutionTraceCounts(job: EvolutionJob) {
  const included = job.evidence_pack?.included_trace_count;
  const total = job.evidence_pack?.total_trace_count;
  const frozen = job.source_trace_ids.length
    || (typeof included === "number" && Number.isFinite(included) ? Math.max(0, Math.trunc(included)) : 0)
    || (job.source_trace_id ? 1 : 0);
  const matched = typeof total === "number" && Number.isFinite(total)
    ? Math.max(frozen, Math.trunc(total))
    : frozen;
  return { frozen, matched };
}

export function prettyJSON(value: unknown): string {
  if (value === undefined || value === null || value === "") return "";
  if (typeof value === "string") {
    try {
      return JSON.stringify(JSON.parse(value), null, 2);
    } catch {
      return value;
    }
  }
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

export function agentAssets(job: EvolutionJob): EvolutionCandidate[] {
  return job.candidates.filter((candidate) => (
    (candidate.kind === "agent_md" || candidate.kind === "skill" || candidate.kind === "role")
    && isUsableAgentAsset(candidate)
  ));
}

export type AgentAssetFile = {
  path: string;
  content: string;
};

export function agentAssetFiles(candidate: EvolutionCandidate): AgentAssetFile[] {
  const content = record(candidate.content);
  if (content) {
    const files = Array.isArray(content.files) ? content.files.flatMap((value) => {
      const file = record(value);
      const path = stringValue(file?.path);
      const body = stringValue(file?.content);
      return path && body ? [{ path, content: body }] : [];
    }) : [];
    if (files.length) return files;

    const path = stringValue(content.path);
    const markdown = stringValue(content.markdown);
    const defaultPath = candidate.kind === "agent_md" ? "agent.md"
      : candidate.kind === "skill" ? "SKILL.md"
        : candidate.kind === "role" ? "role.json"
          : "agent-asset.json";
    if (markdown) return [{ path: path || defaultPath, content: markdown }];
  }
  return [];
}

export function isUsableAgentAsset(candidate: EvolutionCandidate): boolean {
  const files = agentAssetFiles(candidate);
  if (candidate.kind === "agent_md") {
    return files.length === 1 && files[0].path === "agent.md";
  }
  const root = stringValue(record(candidate.content)?.root).replace(/\/$/, "");
  if (candidate.kind === "skill") {
    if (!root) return files.length === 1 && /^skills\/[a-z0-9][a-z0-9-]*\/SKILL\.md$/.test(files[0]?.path || "");
    return /^skills\/[a-z0-9][a-z0-9-]*$/.test(root) && files.some((file) => file.path === `${root}/SKILL.md`);
  }
  if (candidate.kind === "role") {
    if (!/^roles\/[a-z0-9][a-z0-9-]*$/.test(root)) return false;
    return files.some((file) => file.path === `${root}/role.json`)
      && files.some((file) => file.path.startsWith(`${root}/prompts/`) && file.path.endsWith(".md"));
  }
  return false;
}

export function agentAssetText(candidate: EvolutionCandidate): string {
  const files = agentAssetFiles(candidate);
  if (files.length === 1) return files[0].content;
  if (files.length > 1) return files.map((file) => `===== ${file.path} =====\n${file.content}`).join("\n\n");
  if (typeof candidate.content === "object" && candidate.content !== null) {
    const markdown = (candidate.content as Record<string, unknown>).markdown;
    if (typeof markdown === "string") return markdown;
  }
  return prettyJSON(candidate.content);
}

export function agentAssetFilename(candidate: EvolutionCandidate): string {
  const files = agentAssetFiles(candidate);
  if (files.length) return files[0].path.split("/").at(-1) || files[0].path;
  if (typeof candidate.content === "object" && candidate.content !== null) {
    const path = (candidate.content as Record<string, unknown>).path;
    if (typeof path === "string" && path.trim()) {
      const filename = path.trim().split(/[\\/]/).filter(Boolean).at(-1);
      if (filename) return filename;
    }
  }
  if (candidate.kind === "agent_md") return "agent.md";
  if (candidate.kind === "skill") return "SKILL.md";
  if (candidate.kind === "role") return "role.json";
  return "agent-asset.json";
}

export function agentAssetPath(candidate: EvolutionCandidate): string {
  if (typeof candidate.content === "object" && candidate.content !== null) {
    const root = (candidate.content as Record<string, unknown>).root;
    if (typeof root === "string" && root.trim()) return root.trim();
    const path = (candidate.content as Record<string, unknown>).path;
    if (typeof path === "string" && path.trim()) return path.trim();
  }
  return agentAssetFilename(candidate);
}

export function agentAssetDownloadURL(filename: string, content: string): string {
  const type = filename.endsWith(".md") ? "text/markdown"
    : filename.endsWith(".json") ? "application/json"
      : "text/plain";
  return `data:${type};charset=utf-8,${encodeURIComponent(content)}`;
}
