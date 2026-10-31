import type { AnalysisJob, Artifact } from "./types";

export function normalizeAnalysisJob(value: unknown): AnalysisJob {
 if (!value || typeof value !== "object" || !("job_id" in value) || typeof value.job_id !== "string") throw new Error("Analysis response is missing job_id");
 const row = value as Record<string, unknown>;
 return { ...row, job_id: value.job_id, state: String(row.state ?? "unknown"),
   source_trace_ids: Array.isArray(row.source_trace_ids) ? row.source_trace_ids.filter((v): v is string => typeof v === "string") : [],
   stages: Array.isArray(row.stages) ? row.stages as AnalysisJob["stages"] : [],
   artifacts: Array.isArray(row.artifacts) ? row.artifacts.filter((v): v is Artifact => !!v && typeof v === "object" && typeof v.id === "string" && v.job_id === value.job_id && ["rsi_candidate", "engineering_memory"].includes(v.type)) : [],
   created_at: String(row.created_at ?? ""), updated_at: String(row.updated_at ?? ""),
 } as AnalysisJob;
}
export function normalizeAnalysisJobs(value: unknown): AnalysisJob[] {
 if (!Array.isArray(value)) return [];
 return value.flatMap(v => {try {return [normalizeAnalysisJob(v)];} catch {return [];}});
}
