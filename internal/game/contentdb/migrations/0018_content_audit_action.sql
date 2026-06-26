-- P09 lane-F: record an explicit audit action per content_audit_log row so
-- consumers can distinguish publish from rollback without inferring it from the
-- version row. Existing rows back-fill to 'publish' (the only pre-existing
-- mutating action); rollback sets 'rollback'.
ALTER TABLE content_audit_log
  ADD COLUMN IF NOT EXISTS action text NOT NULL DEFAULT 'publish'
  CHECK (action IN ('publish', 'rollback'));

CREATE INDEX IF NOT EXISTS content_audit_log_action_idx
  ON content_audit_log(action);
