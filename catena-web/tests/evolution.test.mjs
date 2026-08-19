import assert from "node:assert/strict";
import test from "node:test";
import {
  agentAssetArchive,
  agentAssets,
  agentAssetDownloadURL,
  agentAssetFiles,
  agentAssetFilename,
  agentAssetPath,
  agentAssetText,
  evolutionTraceCounts,
  evolutionStages,
  normalizeEvolutionJob,
  normalizeEvolutionJobs,
} from "../src/evolution.ts";

const completedJob = {
  schema: "catena.evolution_job.v1",
  job_id: "job-1",
  source_kind: "trace",
  source_trace_id: "trace-1",
  state: "completed",
  current_stage: "complete",
  stages: [
    {
      name: "inspector",
      role: "inspector-cat",
      state: "completed",
      raw_output: JSON.stringify({
        finding: { title: "Tool result was ignored", summary: "The final answer contradicted retained output.", severity: "high", evidence: ["tool.result=failed"] },
        case_proposal: { candidate_id: "case-1", kind: "case", title: "Honor failed tool result", replay_prompt: "Run the tool once.", success_criteria: "The answer reports failure.", verifier: { kind: "artifact_assertions" }, source_trace_id: "trace-1", evidence_pack_sha256: "sha256:abc" },
      }),
    },
    {
      name: "evolution",
      role: "evolution-cat",
      state: "completed",
      raw_output: { candidate: { candidate_id: "skill-1", kind: "skill", title: "Check tool outcome", summary: "Read the result before responding.", content: { instruction: "Check result status." }, source_trace_id: "trace-1", evidence_pack_sha256: "sha256:abc" } },
    },
    {
      name: "reviewer",
      role: "reviewer-cat",
      state: "completed",
      raw_output: { review: { verdict: "pass", summary: "Grounded in the retained Trace." } },
    },
  ],
  evidence_pack: { schema: "catena.evolution_evidence_pack.v1", sha256: "sha256:abc", included_span_count: 4 },
  boundary: { target_agent_executed_by_catena: false, creates_release: false, release_authority: "barena", candidate_status: "draft/unverified", review_scope: "proposal_only" },
  created_at: "2026-08-05T10:00:00Z",
  updated_at: "2026-08-05T10:01:00Z",
};

test("normalizes current stage outputs into structured draft proposals", () => {
  const job = normalizeEvolutionJob(completedJob);

  assert.equal(job.source_kind, "trace");
  assert.equal(job.source_run_id, undefined);
  assert.equal(job.finding?.title, "Tool result was ignored");
  assert.equal(job.case_proposal?.candidate_id, "case-1");
  assert.equal(job.case_proposal?.kind, "case");
  assert.equal(job.case_proposal?.status, "draft/unverified");
  assert.equal(job.case_proposal?.source_trace_id, "trace-1");
  assert.equal(job.case_proposal?.evidence_pack_sha256, "sha256:abc");
  assert.equal(job.candidates.length, 1);
  assert.equal(job.candidates[0].kind, "skill");
  assert.equal(job.candidates[0].status, "draft/unverified");
  assert.equal(job.candidates[0].source_trace_id, "trace-1");
  assert.equal(job.review?.scope, "proposal_only");
  assert.equal(job.review?.candidate_status, "draft/unverified");
  assert.equal(job.evidence_pack?.sha256, "sha256:abc");
  assert.deepEqual(job.boundary, {
    target_agent_executed_by_catena: false,
    creates_release: false,
    release_authority: "barena",
    candidate_status: "draft/unverified",
    review_scope: "proposal_only",
  });
});

test("accepts plural candidate and compatibility field names without duplicating records", () => {
  const job = normalizeEvolutionJob({
    id: "legacy-job",
    trace_id: "legacy-trace",
    run_id: "run-7",
    status: "running",
    stage: "reviewer",
    candidates: [
      { id: "memory-1", type: "memory", title: "Remember constraint", description: "Retain the explicit user constraint." },
      { id: "case-2", kind: "case", title: "Repeat constraint", prompt: "Ask the same request.", expected_behavior: "The constraint remains active." },
    ],
    candidate: { id: "memory-1", type: "memory", title: "Remember constraint", description: "Retain the explicit user constraint." },
    stage_outputs: [{ stage: "reviewer", actor: "reviewer-cat", status: "running" }],
    reviewer_output: { decision: "blocked", reason: "Replay evidence is missing." },
  });

  assert.equal(job.source_run_id, "run-7");
  assert.equal(job.candidates.length, 1);
  assert.equal(job.candidates[0].kind, "memory");
  assert.equal(job.case_proposal?.title, "Repeat constraint");
  assert.equal(job.review?.verdict, "blocked");
  assert.equal(job.review?.candidate_status, "draft/unverified");
});

test("keeps malformed legacy jobs from hiding valid jobs", () => {
  const jobs = normalizeEvolutionJobs([{ state: "completed" }, completedJob]);
  assert.deepEqual(jobs.map((job) => job.job_id), ["job-1"]);
});

test("normalizes Agent Trace Set provenance for direct Agent assets", () => {
  const job = normalizeEvolutionJob({
    schema: "spiral.evolution_job.v1",
    job_id: "job-agent-window",
    source_kind: "agent_trace_set",
    source_agent_id: "codex-desktop",
    source_trace_ids: ["trace-1", "trace-2", "trace-3"],
    window_start: "2026-08-04T12:00:00Z",
    window_end: "2026-08-05T12:00:00Z",
    evidence_pack: { included_trace_count: 3, total_trace_count: 29 },
    state: "completed",
    stages: [],
    candidate: {
      candidate_id: "skill-window",
      kind: "skill",
      title: "Check tool outcomes",
      source_agent_id: "codex-desktop",
      source_trace_ids: ["trace-1", "trace-2", "trace-3"],
      content: {
        root: "skills/check-tool-outcomes",
        files: [{ path: "skills/check-tool-outcomes/SKILL.md", content: "---\nname: check-tool-outcomes\ndescription: Check tool outcomes.\n---\n\n# Check tool outcomes" }],
      },
    },
  });

  assert.equal(job.source_kind, "agent_trace_set");
  assert.equal(job.source_agent_id, "codex-desktop");
  assert.deepEqual(job.source_trace_ids, ["trace-1", "trace-2", "trace-3"]);
  assert.equal(job.source_trace_id, undefined);
  assert.equal(job.window_start, "2026-08-04T12:00:00Z");
  assert.equal(job.window_end, "2026-08-05T12:00:00Z");
  assert.deepEqual(evolutionTraceCounts(job), { frozen: 3, matched: 29 });
  assert.deepEqual(job.candidates[0].source_trace_ids, ["trace-1", "trace-2", "trace-3"]);
  assert.equal(job.candidates[0].source_agent_id, "codex-desktop");

  assert.deepEqual(agentAssets(job).map((candidate) => candidate.kind), ["skill"]);
});

test("fills the fixed role timeline with explicit not-reported states", () => {
  const job = normalizeEvolutionJob({
    job_id: "job-minimal",
    source_trace_id: "trace-minimal",
    state: "running",
    stages: [{ name: "inspector", role: "inspector-cat", state: "completed" }],
  });
  assert.deepEqual(evolutionStages(job).map((stage) => [stage.name, stage.state]), [
    ["inspector", "completed"],
    ["evolution", "not_reported"],
    ["reviewer", "not_reported"],
  ]);
});

test("exposes usable Agent assets and Runtime-bound DSH Plugin packages", () => {
  const codex = normalizeEvolutionJob({
    job_id: "job-assets",
    source_kind: "agent_trace_set",
    source_agent_id: "codex",
    source_trace_ids: ["trace-1", "trace-2"],
    state: "completed",
    stages: [],
    candidates: [
      { kind: "agent_md", title: "Operating rules", summary: "Portable instructions", content: { root: "agent.md", files: [{ path: "agent.md", content: "# Rules\n\nCheck tool results." }] } },
      { kind: "skill", title: "Check results", summary: "Reusable skill", content: { root: "skills/check-results", files: [{ path: "skills/check-results/SKILL.md", content: "---\nname: check-results\ndescription: Check results.\n---\n\n# Check results" }, { path: "skills/check-results/scripts/check.sh", content: "#!/bin/sh\necho check" }] } },
      { kind: "role", title: "Reviewer", summary: "Reusable role", content: { root: "roles/reviewer", files: [{ path: "roles/reviewer/role.json", content: "{\"name\":\"reviewer\",\"promptFile\":\"reviewer.md\"}" }, { path: "roles/reviewer/prompts/reviewer.md", content: "# Reviewer" }] } },
      { kind: "dsh_plugin", title: "Wrong Runtime", summary: "Must stay hidden", source_runtime_kind: "codex", content: { root: "dsh-plugins/evidence-guard", files: [{ path: "dsh-plugins/evidence-guard/package.json", content: "{}" }, { path: "dsh-plugins/evidence-guard/cordis.patch.yml", content: "- id: system-prompt\n  disabled: false" }] } },
      { kind: "harness", title: "Loop guard", summary: "Runtime change", content: { change: "limit loop" } },
      { kind: "memory", title: "User prefers short answers", summary: "Legacy memory" },
      { kind: "case", title: "Legacy case", summary: "Legacy case" },
    ],
  });
  assert.deepEqual(agentAssets(codex).map((candidate) => candidate.kind), ["agent_md", "skill", "role"]);
  assert.equal(agentAssetText(agentAssets(codex)[0]), "# Rules\n\nCheck tool results.");
  assert.equal(agentAssetFiles(agentAssets(codex)[1]).length, 2);
  assert.equal(agentAssetPath(agentAssets(codex)[2]), "roles/reviewer");

  const dsh = normalizeEvolutionJob({
    job_id: "job-dsh-assets",
    source_kind: "agent_trace_set",
    source_agent_id: "dsh-local",
    source_runtime_kind: "dsh",
    source_trace_ids: ["trace-a", "trace-b"],
    state: "completed",
    stages: [],
    candidates: [{
      kind: "dsh_plugin",
      title: "Evidence guard",
      summary: "Ground every claim",
      source_runtime_kind: "dsh",
      content: {
        root: "dsh-plugins/evidence-guard",
        files: [
          { path: "dsh-plugins/evidence-guard/package.json", content: "{\"name\":\"dsh-plugin-evidence-guard\",\"version\":\"0.1.0\",\"private\":true,\"dsh\":{\"bundle\":{\"patch\":\"./cordis.patch.yml\"}}}" },
          { path: "dsh-plugins/evidence-guard/cordis.patch.yml", content: "- id: system-prompt\n  config:\n    persona: Ground claims in retained evidence.\n" },
        ],
      },
    }],
  });
  const plugins = agentAssets(dsh);
  assert.deepEqual(plugins.map((candidate) => candidate.kind), ["dsh_plugin"]);
  assert.equal(agentAssetFilename(plugins[0]), "package.json");
  const archive = agentAssetArchive(plugins[0]);
  assert.equal(archive?.filename, "evidence-guard.tar");
  assert.equal((archive?.bytes.length || 1) % 512, 0);
  assert.match(new TextDecoder().decode(archive?.bytes.slice(0, 100)), /dsh-plugins\/evidence-guard\/package\.json/);

  const wrongRowCandidate = {
    ...dsh.candidates[0],
    content: {
      root: "dsh-plugins/evidence-guard",
      files: [
        { path: "dsh-plugins/evidence-guard/package.json", content: "{\"name\":\"dsh-plugin-evidence-guard\",\"version\":\"0.1.0\",\"private\":true,\"dsh\":{\"bundle\":{\"patch\":\"./cordis.patch.yml\"}}}" },
        { path: "dsh-plugins/evidence-guard/cordis.patch.yml", content: "- id: agent-runtime-id\n  config:\n    persona: Unsafe row.\n" },
      ],
    },
  };
  const wrongRow = normalizeEvolutionJob({
    ...dsh,
    job_id: "job-dsh-wrong-row",
    candidates: [wrongRowCandidate],
    candidate: wrongRowCandidate,
  });
  assert.deepEqual(agentAssets(wrongRow), []);
});

test("gives every deployable Agent asset a stable download filename", () => {
  assert.equal(agentAssetFilename({ kind: "agent_md", content: { path: "config/AGENTS.md", markdown: "# Rules" } }), "AGENTS.md");
  assert.equal(agentAssetFilename({ kind: "agent_md", content: { markdown: "# Rules" } }), "agent.md");
  assert.equal(agentAssetFilename({ kind: "skill", content: {} }), "SKILL.md");
  assert.equal(agentAssetFilename({ kind: "role", content: {} }), "role.json");
  assert.equal(agentAssetFilename({ kind: "dsh_plugin", content: {} }), "cordis.patch.yml");
  assert.equal(agentAssetFilename({ kind: "skill", content: { path: "skills/check-results/SKILL.md", markdown: "# Check" } }), "SKILL.md");
  assert.equal(agentAssetPath({ kind: "skill", content: { path: "skills/check-results/SKILL.md", markdown: "# Check" } }), "skills/check-results/SKILL.md");
  assert.equal(agentAssetText({ kind: "skill", content: { path: "skills/check-results/SKILL.md", markdown: "# Check" } }), "# Check");
  assert.equal(agentAssetFilename({ kind: "role", content: { root: "roles/reviewer", files: [{ path: "roles/reviewer/role.json", content: "{}" }] } }), "role.json");
  assert.match(agentAssetDownloadURL("agent.md", "# A\nB"), /^data:text\/markdown;charset=utf-8,/);
  assert.match(agentAssetDownloadURL("skill.json", '{"ok":true}'), /^data:application\/json;charset=utf-8,/);
  assert.equal(decodeURIComponent(agentAssetDownloadURL("skill.json", '{"ok":true}').split(",", 2)[1]), '{"ok":true}');
});
