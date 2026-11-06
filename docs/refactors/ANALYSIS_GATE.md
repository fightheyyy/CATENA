# Analysis first, human-started improvement

Implemented two controlled tasks on the existing Codex App Server Runtime:

- `pipeline=analyze`: trace-analysis Skill, no tools, no Case or Harbor requirement.
  Persists Problem / Evidence references / hypothesis / uncertainty as an unverified
  `analysis_report`. Completing analysis never starts improvement.
- A user can keep the report, or explicitly submit `pipeline=improve` with the owned
  completed `analysis_job_id`. The new Job freezes the original Evidence and report;
  its candidate provenance retains source ID, full report and report hash. Foreign,
  incomplete or missing analysis references are rejected. The existing candidate
  evaluation and final Human Review remain separate.

The UI defaults to analysis, displays the report and exposes the manual improvement
form only after completion. No new queue, approval table, framework or dependency.
Historical jobs remain readable. Old direct Trace → improve requests now fail.

Simple real smoke: `analysis-job-1793935150510-1ee91d7dcc723b78`, using the retained
real Trace and existing CLIProxyAPI gpt-5.6-luna. Completed with **5 problems, zero
registered tools, zero tool calls, zero Harbor trials and zero automatic improvement
jobs**. Report was read back from PostgreSQL. The temporary test connection was
removed. [Evidence summary](evidence/analysis-human-gate/real-analysis.json).

Only small boundary tests were added; existing caller fixtures were updated.
Go tests and the PostgreSQL/private-Python fixture passed. Targeted Python checks,
existing React unit/browser tests and TypeScript/build passed. No new candidate
tuning, paid optimization or effectiveness study was performed. The report is
hypothesis-bearing evidence, not a verified root-cause or reusable-memory assertion.
