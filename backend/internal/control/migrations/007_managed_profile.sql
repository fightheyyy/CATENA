ALTER TABLE managed_agents ADD COLUMN skills JSONB NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(skills)='array');
ALTER TABLE managed_agents ADD COLUMN knowledge_refs JSONB NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(knowledge_refs)='array');
ALTER TABLE managed_agents DROP CONSTRAINT managed_agents_tool_policy_check;
ALTER TABLE managed_agents ADD CONSTRAINT managed_agents_tool_policy_check CHECK (tool_policy IN ('sandbox-write','read-only'));
