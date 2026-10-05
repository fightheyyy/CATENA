export type ExperimentCase = { case_id: string; title: string; description: string; task_count: number; trial_count: number; target: string; environment: string; limitation: string; evidence_summary?: string; source_trace_id?: string };
export type ExperimentTrial = { variant: string; task: string; valid: boolean; rewards: Record<string, number>; input_tokens: number; output_tokens: number; command_count: number; missing_rg_errors: number; policy_rejections: number; native_session_verified?: boolean; environment_verified?: boolean; attempt?: number; duration_seconds?: number };
export type Experiment = { experiment_id: string; request_id: string; case_id: string; state: string; harbor_job_id?: string; model: string; environment: string; trials: ExperimentTrial[]; tasks_unchanged: boolean; message?: string; summary?: string; created_at: string; updated_at: string; target_agent?: string; study?: string; attempts?: number; arms?: string[]; memory_context?: { id: string; title?: string; content: string; score: number }[] };
export const experimentArms = (experiment: Experiment) => experiment.study === "memory_skill" ? ["baseline", "memory", "candidate"] : ["baseline", "candidate"];
export const expectedTrials = (experiment: Experiment) => 3 * (experiment.attempts ?? 1) * experimentArms(experiment).length;
export const isExperimentActive = (experiment: Experiment) => ["queued", "running"].includes(experiment.state);
export function experimentArm(experiment: Experiment, arm: string) {
  const trials = experiment.trials.filter((trial) => trial.variant === arm);
  const valid = trials.filter((trial) => trial.valid);
  const sum = (key: "command_count" | "missing_rg_errors" | "input_tokens" | "output_tokens") => valid.reduce((n, trial) => n + (trial[key] ?? 0), 0);
  return { total: trials.length, valid: valid.length, passed: valid.filter((trial) => trial.rewards?.reward === 1).length,
    commands: sum("command_count"), errors: sum("missing_rg_errors"), input: sum("input_tokens"), output: sum("output_tokens"),
    seconds: valid.length && valid.every((t) => typeof t.duration_seconds === "number") ? valid.reduce((n, t) => n + t.duration_seconds!, 0) : null };
}
