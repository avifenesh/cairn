-- 007_subagents: Index for querying subagent tasks by parent.
-- The parent_id is stored in the metadata JSON column via task.SubmitRequest.ParentID.
-- This index enables efficient lookups like GET /v1/subagents?parentTaskId=xyz.
CREATE INDEX IF NOT EXISTS idx_tasks_parent_id
    ON tasks(json_extract(metadata, '$.parent_id'))
    WHERE json_extract(metadata, '$.parent_id') IS NOT NULL
      AND json_extract(metadata, '$.parent_id') != '';
