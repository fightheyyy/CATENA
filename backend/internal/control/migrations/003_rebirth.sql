-- Preserve completed jobs, provider checkpoints, artifacts and old Memory tables.
UPDATE spiral_evolution_jobs SET state='failed', current_stage='retired',
 document=jsonb_set(jsonb_set(jsonb_set(document,'{state}','"failed"'),'{current_stage}','"retired"'),'{error}','"Memory retired by Operation Rebirth; export historical data before removal"'),
 worker_id='', lease_until=NULL, updated_at=now()
WHERE state IN ('queued','running') AND document->>'pipeline'='memory';
