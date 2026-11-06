import assert from "node:assert/strict";
import test from "node:test";
import {normalizeAnalysisJob, normalizeAnalysisJobs} from "../src/analysis.ts";

test("shared analysis contract retains artifacts and ignores malformed/foreign job envelopes", () => {
 const job=normalizeAnalysisJob({job_id:"job",state:"completed",pipeline:"memory",artifacts:[
  {id:"memory",job_id:"job",type:"engineering_memory",validation_status:"extracted",payload:{result:{memory_count:1}}},
  {id:"candidate",job_id:"job",type:"improvement_candidate",validation_status:"review_required"},
  {id:"foreign",job_id:"other",type:"rsi_candidate"}
 ]});
 assert.equal(job.artifacts.length,2);
 assert.equal(job.artifacts[0].validation_status,"extracted");
 assert.equal(job.artifacts[1].validation_status,"review_required");
 assert.deepEqual(job.source_trace_ids,[]);
 assert.equal(normalizeAnalysisJobs([{},job]).length,1);
});
