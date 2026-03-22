-- Add agent_type column to cron_jobs for agent-type binding.
-- When set, the cron fires that specific agent type instead of the generic task path.
ALTER TABLE cron_jobs ADD COLUMN agent_type TEXT NOT NULL DEFAULT '';
