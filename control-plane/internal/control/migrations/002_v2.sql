-- Preserve historic document keys and terminal rows. Never replay fixed Role jobs.
UPDATE spiral_evolution_jobs SET state='failed', current_stage='retired',
 document=jsonb_set(jsonb_set(jsonb_set(document,'{state}','"failed"'),'{current_stage}','"retired"'),'{error}','"Legacy execution retired by V2; create a new Analysis Job"'),
 worker_id='', lease_until=NULL, updated_at=now()
WHERE state IN ('queued','running') AND COALESCE(document->>'schema','') <> 'catena.analysis_job.v2';
