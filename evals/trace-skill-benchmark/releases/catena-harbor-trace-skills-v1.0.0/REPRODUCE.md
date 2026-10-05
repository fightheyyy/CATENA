# Catena Harbor dataset and result freeze: catena-harbor-trace-skills-v1.0.0

This snapshot contains 28 Harbor tasks, 7 corresponding Skills, and all 168 valid
Codex with/without results (84 per arm, 3 repeats per task). Original failed
provider/setup attempts are retained locally and excluded from effect metrics.

## Verify the snapshot

The release-manifest.json records SHA-256 for each dataset and result file.
selected-evidence-sha256.json records the original selected Harbor artifact hashes.
Native sessions and connection configurations remain private; the ZIP contains
only anonymous fixtures, Skill text, aggregate/individual metrics, and hashes.

## Reproduce

1. Use a Windows/PowerShell host with Docker Linux containers, Harbor
   0.23.0, and Codex CLI 0.160.0.
   dependency-versions.json lists the original Python environment. vendor/harbor
   contains the exact installed Harbor source; set PYTHONPATH to the vendor
   directory to reuse it with its matching installed dependencies.
2. Restore the saved image with docker image load, then confirm its image ID is
   sha256:7d9497b876b57bd6889909bf05dbd4041d0944f3615149e1790ed4b6d47eb92a. The image tar is stored locally under
   .local/trace-skill-benchmark/images/7d9497b876b57bd6889909bf05dbd4041d0944f3615149e1790ed4b6d47eb92a.tar, with the hash in the manifest.
3. Configure your own proxy at port 8317 and set CATENA_EVAL_KEY_FILE to your key file.
4. From the release root, run:
   python evals/trace-skill-benchmark/run_study.py --mode oracle --run oracle-new --repeats 1 --concurrency 1
   python evals/trace-skill-benchmark/run_study.py --mode ab --run ab-new --repeats 3 --concurrency 1
5. Keep this frozen release unchanged. New observations and task revisions need a
   new version. New stochastic model runs can produce different results.

All tasks and Skills retain the original pre-experiment hashes. The resumed cohort
is explicitly marked; timing across concurrent and serial cohorts is descriptive.
See evals/trace-skill-benchmark/results/ab-v2.md for outcomes and negative results.
