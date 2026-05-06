package omegalocal

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type SQLiteRepository struct {
	Path string
}

type PagePilotRunListOptions struct {
	ID                 string
	ProjectID          string
	RepositoryTargetID string
	Status             string
	Limit              int
	Compact            bool
}

func NewSQLiteRepository(path string) *SQLiteRepository {
	return &SQLiteRepository{Path: path}
}

func (repo *SQLiteRepository) Initialize(ctx context.Context) error {
	if err := os.MkdirAll(filepath.Dir(repo.Path), 0o755); err != nil {
		return err
	}
	if err := repo.exec(ctx, `
PRAGMA journal_mode = WAL;
CREATE TABLE IF NOT EXISTS omega_migrations (
  version TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  applied_at TEXT NOT NULL
);
`); err != nil {
		return err
	}
	return repo.applyPendingSQLiteMigrations(ctx)
}

func (repo *SQLiteRepository) initializeBaselineSchema(ctx context.Context) error {
	return repo.exec(ctx, `
PRAGMA journal_mode = WAL;
CREATE TABLE IF NOT EXISTS omega_migrations (
  version TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  applied_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS omega_settings (
  key TEXT PRIMARY KEY,
  value_json TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS workspace_snapshots (
  id TEXT PRIMARY KEY,
  database_json TEXT NOT NULL,
  saved_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS projects (id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT NOT NULL, team TEXT NOT NULL, status TEXT NOT NULL, labels_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS repository_targets (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL,
  kind TEXT NOT NULL,
  label TEXT NOT NULL,
  owner TEXT,
  repo TEXT,
  path TEXT,
  url TEXT,
  default_branch TEXT,
  target_json TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_repository_targets_project ON repository_targets(project_id, kind, updated_at);
CREATE TABLE IF NOT EXISTS requirements (id TEXT PRIMARY KEY, project_id TEXT NOT NULL, repository_target_id TEXT, source TEXT NOT NULL, source_external_ref TEXT, title TEXT NOT NULL, raw_text TEXT NOT NULL, structured_json TEXT NOT NULL, acceptance_criteria_json TEXT NOT NULL, risks_json TEXT NOT NULL, status TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS work_items (id TEXT PRIMARY KEY, project_id TEXT NOT NULL, key TEXT NOT NULL, title TEXT NOT NULL, description TEXT NOT NULL, status TEXT NOT NULL, priority TEXT NOT NULL, assignee TEXT NOT NULL, labels_json TEXT NOT NULL, team TEXT NOT NULL, stage_id TEXT NOT NULL, target TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, record_json TEXT);
CREATE TABLE IF NOT EXISTS mission_control_states (run_id TEXT PRIMARY KEY, project_id TEXT NOT NULL, work_items_json TEXT NOT NULL, events_json TEXT NOT NULL, sync_intents_json TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS mission_events (id TEXT PRIMARY KEY, run_id TEXT NOT NULL, sequence INTEGER NOT NULL, event_json TEXT NOT NULL);
	CREATE TABLE IF NOT EXISTS sync_intents (id TEXT PRIMARY KEY, run_id TEXT NOT NULL, sequence INTEGER NOT NULL, intent_json TEXT NOT NULL);
	CREATE TABLE IF NOT EXISTS connections (provider_id TEXT PRIMARY KEY, status TEXT NOT NULL, granted_permissions_json TEXT NOT NULL, connected_as TEXT, updated_at TEXT NOT NULL);
	CREATE TABLE IF NOT EXISTS feishu_delivery_receipts (
	  dedupe_key TEXT PRIMARY KEY,
	  provider TEXT NOT NULL,
	  kind TEXT NOT NULL,
	  entity_id TEXT,
	  pipeline_id TEXT,
	  attempt_id TEXT,
	  work_item_id TEXT,
	  status TEXT NOT NULL,
	  route TEXT,
	  format TEXT,
	  task_guid TEXT,
	  task_id TEXT,
	  message_id TEXT,
	  payload_json TEXT NOT NULL,
	  created_at TEXT NOT NULL,
	  updated_at TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_feishu_delivery_receipts_status ON feishu_delivery_receipts(provider, kind, status, updated_at);
	CREATE TABLE IF NOT EXISTS ui_preferences (id TEXT PRIMARY KEY, active_nav TEXT NOT NULL, selected_provider_id TEXT NOT NULL, selected_work_item_id TEXT NOT NULL, inspector_open INTEGER NOT NULL, active_inspector_panel TEXT NOT NULL, runner_preset TEXT NOT NULL, status_filter TEXT NOT NULL, assignee_filter TEXT NOT NULL, sort_direction TEXT NOT NULL, collapsed_groups_json TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS pipelines (id TEXT PRIMARY KEY, work_item_id TEXT NOT NULL, run_id TEXT NOT NULL, status TEXT NOT NULL, run_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS attempts (
  id TEXT PRIMARY KEY,
  item_id TEXT NOT NULL,
  pipeline_id TEXT NOT NULL,
  repository_target_id TEXT,
  status TEXT NOT NULL,
  trigger TEXT NOT NULL,
  runner TEXT NOT NULL,
  current_stage_id TEXT,
  workspace_path TEXT,
  branch_name TEXT,
  pull_request_url TEXT,
  started_at TEXT NOT NULL,
  finished_at TEXT,
  duration_ms INTEGER,
  error_message TEXT,
  stdout_summary TEXT,
  stderr_summary TEXT,
  stages_json TEXT NOT NULL,
  events_json TEXT NOT NULL,
  feishu_failure_notified_at TEXT,
  feishu_failure_status TEXT,
  feishu_failure_message_id TEXT,
  feishu_failure_json TEXT NOT NULL DEFAULT '{}',
  record_json TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS checkpoints (
  id TEXT PRIMARY KEY,
  pipeline_id TEXT NOT NULL,
  attempt_id TEXT,
  stage_id TEXT NOT NULL,
  status TEXT NOT NULL,
  title TEXT NOT NULL,
  summary TEXT NOT NULL,
  decision_note TEXT,
  feishu_review_status TEXT,
  feishu_task_guid TEXT,
  feishu_task_id TEXT,
  feishu_message_id TEXT,
  feishu_review_json TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_pipelines_work_item ON pipelines(work_item_id, updated_at);
CREATE INDEX IF NOT EXISTS idx_attempts_repository_target ON attempts(repository_target_id, updated_at);
CREATE INDEX IF NOT EXISTS idx_attempts_pipeline_item ON attempts(pipeline_id, item_id, updated_at);
CREATE TABLE IF NOT EXISTS missions (id TEXT PRIMARY KEY, pipeline_id TEXT NOT NULL, work_item_id TEXT NOT NULL, title TEXT NOT NULL, status TEXT NOT NULL, mission_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS operations (id TEXT PRIMARY KEY, mission_id TEXT NOT NULL, stage_id TEXT NOT NULL, agent_id TEXT NOT NULL, status TEXT NOT NULL, prompt TEXT NOT NULL, required_proof_json TEXT NOT NULL, record_json TEXT, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS proof_records (id TEXT PRIMARY KEY, operation_id TEXT NOT NULL, label TEXT NOT NULL, value TEXT NOT NULL, source_path TEXT, created_at TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS idx_missions_pipeline_work_item ON missions(pipeline_id, work_item_id, updated_at);
CREATE INDEX IF NOT EXISTS idx_operations_mission ON operations(mission_id, updated_at);
CREATE INDEX IF NOT EXISTS idx_proof_records_operation ON proof_records(operation_id, created_at);
CREATE TABLE IF NOT EXISTS handoff_bundles (
  id TEXT PRIMARY KEY,
  attempt_id TEXT,
  pipeline_id TEXT,
  work_item_id TEXT,
  repository_target_id TEXT,
  source_path TEXT,
  bundle_json TEXT NOT NULL,
  summary_json TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_handoff_bundles_attempt ON handoff_bundles(attempt_id, updated_at);
CREATE INDEX IF NOT EXISTS idx_handoff_bundles_work_item ON handoff_bundles(work_item_id, updated_at);
CREATE TABLE IF NOT EXISTS operation_queue (
  id TEXT PRIMARY KEY,
  operation_id TEXT NOT NULL,
  pipeline_id TEXT,
  attempt_id TEXT,
  work_item_id TEXT,
  repository_target_id TEXT,
  stage_id TEXT,
  agent_id TEXT,
  status TEXT NOT NULL,
  priority INTEGER NOT NULL,
  not_before TEXT,
  locked_by TEXT,
  lock_expires_at TEXT,
  attempt_count INTEGER NOT NULL,
  queue_json TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_operation_queue_status ON operation_queue(status, priority, updated_at);
CREATE TABLE IF NOT EXISTS run_workpads (
  id TEXT PRIMARY KEY,
  attempt_id TEXT NOT NULL,
  pipeline_id TEXT NOT NULL,
  work_item_id TEXT NOT NULL,
  repository_target_id TEXT,
  status TEXT NOT NULL,
  workpad_json TEXT NOT NULL,
  record_json TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_run_workpads_attempt ON run_workpads(attempt_id);
CREATE INDEX IF NOT EXISTS idx_run_workpads_work_item ON run_workpads(work_item_id, updated_at);
CREATE TABLE IF NOT EXISTS workflow_templates (
  id TEXT PRIMARY KEY,
  scope TEXT NOT NULL,
  project_id TEXT NOT NULL,
  repository_target_id TEXT,
  template_id TEXT NOT NULL,
  source TEXT NOT NULL,
  version INTEGER NOT NULL,
  markdown TEXT NOT NULL,
  validation_json TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_workflow_templates_scope ON workflow_templates(project_id, scope, COALESCE(repository_target_id, ''), template_id);
CREATE TABLE IF NOT EXISTS agent_profiles (
  id TEXT PRIMARY KEY,
  scope TEXT NOT NULL,
  project_id TEXT NOT NULL,
  repository_target_id TEXT,
  version INTEGER NOT NULL,
  profile_json TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_profiles_project_scope ON agent_profiles(project_id, scope, COALESCE(repository_target_id, ''));
CREATE TABLE IF NOT EXISTS page_pilot_runs (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL,
  repository_target_id TEXT NOT NULL,
  status TEXT NOT NULL,
  runner TEXT,
  repository_path TEXT,
  branch_name TEXT,
  commit_sha TEXT,
  pull_request_url TEXT,
  changed_files_json TEXT NOT NULL,
  run_json TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_page_pilot_runs_scope ON page_pilot_runs(project_id, repository_target_id, updated_at);
CREATE TABLE IF NOT EXISTS runtime_logs (
  id TEXT PRIMARY KEY,
  level TEXT NOT NULL,
  event_type TEXT NOT NULL,
  message TEXT NOT NULL,
  entity_type TEXT,
  entity_id TEXT,
  project_id TEXT,
  repository_target_id TEXT,
  requirement_id TEXT,
  work_item_id TEXT,
  pipeline_id TEXT,
  attempt_id TEXT,
  stage_id TEXT,
  agent_id TEXT,
  request_id TEXT,
  details_json TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_runtime_logs_created_at ON runtime_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_runtime_logs_level ON runtime_logs(level, created_at);
CREATE INDEX IF NOT EXISTS idx_runtime_logs_pipeline ON runtime_logs(pipeline_id, created_at);
CREATE INDEX IF NOT EXISTS idx_runtime_logs_attempt ON runtime_logs(attempt_id, created_at);
`)
}

func (repo *SQLiteRepository) ensureRuntimeLogQueryExtensions(ctx context.Context) error {
	output, err := repo.query(ctx, `.mode json
PRAGMA table_info(runtime_logs);
`)
	if err != nil {
		return err
	}
	var columns []map[string]any
	if strings.TrimSpace(output) != "" {
		if err := json.Unmarshal([]byte(output), &columns); err != nil {
			return err
		}
	}
	hasRequirementID := false
	for _, column := range columns {
		if text(column, "name") == "requirement_id" {
			hasRequirementID = true
			break
		}
	}
	if !hasRequirementID {
		if err := repo.exec(ctx, "ALTER TABLE runtime_logs ADD COLUMN requirement_id TEXT;"); err != nil {
			return err
		}
	}
	return repo.exec(ctx, `
CREATE INDEX IF NOT EXISTS idx_runtime_logs_requirement ON runtime_logs(requirement_id, created_at);
CREATE INDEX IF NOT EXISTS idx_runtime_logs_work_item ON runtime_logs(work_item_id, created_at);
`)
}

func (repo *SQLiteRepository) ensureWorkItemRecordJSON(ctx context.Context) error {
	output, err := repo.query(ctx, `.mode json
PRAGMA table_info(work_items);
`)
	if err != nil {
		return err
	}
	var columns []map[string]any
	if strings.TrimSpace(output) != "" {
		if err := json.Unmarshal([]byte(output), &columns); err != nil {
			return err
		}
	}
	hasRecordJSON := false
	for _, column := range columns {
		if text(column, "name") == "record_json" {
			hasRecordJSON = true
			break
		}
	}
	if !hasRecordJSON {
		if err := repo.exec(ctx, "ALTER TABLE work_items ADD COLUMN record_json TEXT;"); err != nil {
			return err
		}
	}
	return repo.exec(ctx, `
UPDATE work_items
SET record_json = COALESCE(
  (
    SELECT j.value
    FROM workspace_snapshots ws, json_each(ws.database_json, '$.tables.workItems') j
    WHERE json_extract(j.value, '$.id') = work_items.id
    LIMIT 1
  ),
  json_object(
    'id', id,
    'projectId', project_id,
    'key', key,
    'title', title,
    'description', description,
    'status', status,
    'priority', priority,
    'assignee', assignee,
    'labels', json(labels_json),
    'team', team,
    'stageId', stage_id,
    'target', target,
    'createdAt', created_at,
    'updatedAt', updated_at
  )
)
WHERE record_json IS NULL OR TRIM(record_json) = '';
`)
}

func (repo *SQLiteRepository) ensureCheckpointFeishuReviewJSON(ctx context.Context) error {
	output, err := repo.query(ctx, `.mode json
PRAGMA table_info(checkpoints);
`)
	if err != nil {
		return err
	}
	var columns []map[string]any
	if strings.TrimSpace(output) != "" {
		if err := json.Unmarshal([]byte(output), &columns); err != nil {
			return err
		}
	}
	hasFeishuReviewJSON := false
	for _, column := range columns {
		if text(column, "name") == "feishu_review_json" {
			hasFeishuReviewJSON = true
			break
		}
	}
	if !hasFeishuReviewJSON {
		if err := repo.exec(ctx, "ALTER TABLE checkpoints ADD COLUMN feishu_review_json TEXT NOT NULL DEFAULT '{}';"); err != nil {
			return err
		}
	}
	return repo.exec(ctx, `
UPDATE checkpoints
SET feishu_review_json = COALESCE(
  (
    SELECT json_extract(j.value, '$.feishuReview')
    FROM workspace_snapshots ws, json_each(ws.database_json, '$.tables.checkpoints') j
    WHERE json_extract(j.value, '$.id') = checkpoints.id
    LIMIT 1
  ),
  '{}'
)
WHERE feishu_review_json IS NULL OR TRIM(feishu_review_json) = '' OR TRIM(feishu_review_json) = '{}';
`)
}

func (repo *SQLiteRepository) ensureCheckpointCoreFacts(ctx context.Context) error {
	output, err := repo.query(ctx, `.mode json
PRAGMA table_info(checkpoints);
`)
	if err != nil {
		return err
	}
	var columns []map[string]any
	if strings.TrimSpace(output) != "" {
		if err := json.Unmarshal([]byte(output), &columns); err != nil {
			return err
		}
	}
	known := map[string]bool{}
	for _, column := range columns {
		known[text(column, "name")] = true
	}
	for _, statement := range []struct {
		column string
		sql    string
	}{
		{column: "attempt_id", sql: "ALTER TABLE checkpoints ADD COLUMN attempt_id TEXT;"},
		{column: "feishu_review_status", sql: "ALTER TABLE checkpoints ADD COLUMN feishu_review_status TEXT;"},
		{column: "feishu_task_guid", sql: "ALTER TABLE checkpoints ADD COLUMN feishu_task_guid TEXT;"},
		{column: "feishu_task_id", sql: "ALTER TABLE checkpoints ADD COLUMN feishu_task_id TEXT;"},
		{column: "feishu_message_id", sql: "ALTER TABLE checkpoints ADD COLUMN feishu_message_id TEXT;"},
	} {
		if known[statement.column] {
			continue
		}
		if err := repo.exec(ctx, statement.sql); err != nil {
			return err
		}
	}
	return repo.exec(ctx, `
UPDATE checkpoints
SET
  attempt_id = COALESCE(
    NULLIF(attempt_id, ''),
    (
      SELECT json_extract(j.value, '$.attemptId')
      FROM workspace_snapshots ws, json_each(ws.database_json, '$.tables.checkpoints') j
      WHERE json_extract(j.value, '$.id') = checkpoints.id
      LIMIT 1
    )
  ),
  feishu_review_status = COALESCE(NULLIF(feishu_review_status, ''), json_extract(feishu_review_json, '$.status')),
  feishu_task_guid = COALESCE(NULLIF(feishu_task_guid, ''), json_extract(feishu_review_json, '$.taskGuid')),
  feishu_task_id = COALESCE(NULLIF(feishu_task_id, ''), json_extract(feishu_review_json, '$.taskId')),
  feishu_message_id = COALESCE(NULLIF(feishu_message_id, ''), json_extract(feishu_review_json, '$.messageId'));
CREATE INDEX IF NOT EXISTS idx_checkpoints_pipeline_stage_status ON checkpoints(pipeline_id, stage_id, status, updated_at);
CREATE INDEX IF NOT EXISTS idx_checkpoints_attempt ON checkpoints(attempt_id, updated_at);
CREATE INDEX IF NOT EXISTS idx_checkpoints_feishu_task ON checkpoints(feishu_task_guid, feishu_task_id);
`)
}

func (repo *SQLiteRepository) ensureAttemptDeliveryFacts(ctx context.Context) error {
	output, err := repo.query(ctx, `.mode json
PRAGMA table_info(attempts);
`)
	if err != nil {
		return err
	}
	var columns []map[string]any
	if strings.TrimSpace(output) != "" {
		if err := json.Unmarshal([]byte(output), &columns); err != nil {
			return err
		}
	}
	known := map[string]bool{}
	for _, column := range columns {
		known[text(column, "name")] = true
	}
	for _, statement := range []struct {
		column string
		sql    string
	}{
		{column: "feishu_failure_notified_at", sql: "ALTER TABLE attempts ADD COLUMN feishu_failure_notified_at TEXT;"},
		{column: "feishu_failure_status", sql: "ALTER TABLE attempts ADD COLUMN feishu_failure_status TEXT;"},
		{column: "feishu_failure_message_id", sql: "ALTER TABLE attempts ADD COLUMN feishu_failure_message_id TEXT;"},
		{column: "feishu_failure_json", sql: "ALTER TABLE attempts ADD COLUMN feishu_failure_json TEXT NOT NULL DEFAULT '{}';"},
		{column: "record_json", sql: "ALTER TABLE attempts ADD COLUMN record_json TEXT;"},
	} {
		if known[statement.column] {
			continue
		}
		if err := repo.exec(ctx, statement.sql); err != nil {
			return err
		}
	}
	return repo.exec(ctx, `
UPDATE attempts
SET
  feishu_failure_notified_at = COALESCE(
    NULLIF(feishu_failure_notified_at, ''),
    (
      SELECT json_extract(j.value, '$.feishuFailureNotifiedAt')
      FROM workspace_snapshots ws, json_each(ws.database_json, '$.tables.attempts') j
      WHERE json_extract(j.value, '$.id') = attempts.id
      LIMIT 1
    )
  ),
  feishu_failure_json = COALESCE(
    NULLIF(NULLIF(feishu_failure_json, ''), '{}'),
    (
      SELECT json_extract(j.value, '$.feishuFailure')
      FROM workspace_snapshots ws, json_each(ws.database_json, '$.tables.attempts') j
      WHERE json_extract(j.value, '$.id') = attempts.id
      LIMIT 1
    ),
    '{}'
  ),
  feishu_failure_status = COALESCE(NULLIF(feishu_failure_status, ''), json_extract(feishu_failure_json, '$.status')),
  feishu_failure_message_id = COALESCE(NULLIF(feishu_failure_message_id, ''), json_extract(feishu_failure_json, '$.messageId')),
  record_json = COALESCE(
    NULLIF(record_json, ''),
    (
      SELECT j.value
      FROM workspace_snapshots ws, json_each(ws.database_json, '$.tables.attempts') j
      WHERE json_extract(j.value, '$.id') = attempts.id
      LIMIT 1
    )
  );
CREATE INDEX IF NOT EXISTS idx_attempts_feishu_failure ON attempts(feishu_failure_notified_at, feishu_failure_status, updated_at);
`)
}

func (repo *SQLiteRepository) ensureRunWorkpadRecordJSON(ctx context.Context) error {
	output, err := repo.query(ctx, `.mode json
PRAGMA table_info(run_workpads);
`)
	if err != nil {
		return err
	}
	var columns []map[string]any
	if strings.TrimSpace(output) != "" {
		if err := json.Unmarshal([]byte(output), &columns); err != nil {
			return err
		}
	}
	hasRecordJSON := false
	for _, column := range columns {
		if text(column, "name") == "record_json" {
			hasRecordJSON = true
			break
		}
	}
	if !hasRecordJSON {
		if err := repo.exec(ctx, "ALTER TABLE run_workpads ADD COLUMN record_json TEXT;"); err != nil {
			return err
		}
	}
	return repo.exec(ctx, `
UPDATE run_workpads
SET record_json = COALESCE(
  NULLIF(record_json, ''),
  (
    SELECT j.value
    FROM workspace_snapshots ws, json_each(ws.database_json, '$.tables.runWorkpads') j
    WHERE json_extract(j.value, '$.id') = run_workpads.id
    LIMIT 1
  ),
  json_object(
    'id', id,
    'attemptId', attempt_id,
    'pipelineId', pipeline_id,
    'workItemId', work_item_id,
    'repositoryTargetId', repository_target_id,
    'status', status,
    'workpad', json(workpad_json),
    'createdAt', created_at,
    'updatedAt', updated_at
  )
);
`)
}

func (repo *SQLiteRepository) ensureOperationRecordJSON(ctx context.Context) error {
	output, err := repo.query(ctx, `.mode json
PRAGMA table_info(operations);
`)
	if err != nil {
		return err
	}
	var columns []map[string]any
	if strings.TrimSpace(output) != "" {
		if err := json.Unmarshal([]byte(output), &columns); err != nil {
			return err
		}
	}
	hasRecordJSON := false
	for _, column := range columns {
		if text(column, "name") == "record_json" {
			hasRecordJSON = true
			break
		}
	}
	if !hasRecordJSON {
		if err := repo.exec(ctx, "ALTER TABLE operations ADD COLUMN record_json TEXT;"); err != nil {
			return err
		}
	}
	return repo.exec(ctx, `
UPDATE operations
SET record_json = COALESCE(
  NULLIF(record_json, ''),
  (
    SELECT j.value
    FROM workspace_snapshots ws, json_each(ws.database_json, '$.tables.operations') j
    WHERE json_extract(j.value, '$.id') = operations.id
    LIMIT 1
  )
);
	`)
}

func (repo *SQLiteRepository) ensureFeishuDeliveryReceipts(ctx context.Context) error {
	if err := repo.exec(ctx, `
CREATE TABLE IF NOT EXISTS feishu_delivery_receipts (
  dedupe_key TEXT PRIMARY KEY,
  provider TEXT NOT NULL,
  kind TEXT NOT NULL,
  entity_id TEXT,
  pipeline_id TEXT,
  attempt_id TEXT,
  work_item_id TEXT,
  status TEXT NOT NULL,
  route TEXT,
  format TEXT,
  task_guid TEXT,
  task_id TEXT,
  message_id TEXT,
  payload_json TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_feishu_delivery_receipts_status ON feishu_delivery_receipts(provider, kind, status, updated_at);
`); err != nil {
		return err
	}
	return repo.exec(ctx, `
INSERT OR IGNORE INTO feishu_delivery_receipts (
  dedupe_key, provider, kind, entity_id, pipeline_id, attempt_id, work_item_id,
  status, route, format, task_guid, task_id, message_id, payload_json, created_at, updated_at
)
SELECT
  'human_review:' || id,
  'feishu',
  'human_review',
  id,
  pipeline_id,
  attempt_id,
  NULL,
  COALESCE(NULLIF(feishu_review_status, ''), json_extract(feishu_review_json, '$.status'), 'sent'),
  json_extract(feishu_review_json, '$.route'),
  json_extract(feishu_review_json, '$.format'),
  COALESCE(NULLIF(feishu_task_guid, ''), json_extract(feishu_review_json, '$.taskGuid')),
  COALESCE(NULLIF(feishu_task_id, ''), json_extract(feishu_review_json, '$.taskId')),
  COALESCE(NULLIF(feishu_message_id, ''), json_extract(feishu_review_json, '$.messageId')),
  feishu_review_json,
  COALESCE(NULLIF(created_at, ''), updated_at),
  COALESCE(NULLIF(updated_at, ''), created_at)
FROM checkpoints
WHERE
  COALESCE(NULLIF(feishu_review_status, ''), json_extract(feishu_review_json, '$.status')) = 'sent'
  OR COALESCE(NULLIF(feishu_task_guid, ''), NULLIF(feishu_task_id, ''), NULLIF(feishu_message_id, ''),
              json_extract(feishu_review_json, '$.taskGuid'), json_extract(feishu_review_json, '$.taskId'),
              json_extract(feishu_review_json, '$.messageId')) IS NOT NULL;

INSERT OR IGNORE INTO feishu_delivery_receipts (
  dedupe_key, provider, kind, entity_id, pipeline_id, attempt_id, work_item_id,
  status, route, format, task_guid, task_id, message_id, payload_json, created_at, updated_at
)
SELECT
  'human_review:' || json_extract(details_json, '$.checkpointId'),
  'feishu',
  'human_review',
  json_extract(details_json, '$.checkpointId'),
  json_extract(details_json, '$.pipelineId'),
  json_extract(details_json, '$.attemptId'),
  json_extract(details_json, '$.workItemId'),
  json_extract(details_json, '$.status'),
  json_extract(details_json, '$.route'),
  json_extract(details_json, '$.format'),
  json_extract(details_json, '$.taskGuid'),
  json_extract(details_json, '$.taskId'),
  json_extract(details_json, '$.messageId'),
  details_json,
  created_at,
  created_at
FROM runtime_logs
WHERE event_type = 'feishu.review.synced'
  AND json_extract(details_json, '$.checkpointId') IS NOT NULL
  AND json_extract(details_json, '$.status') = 'sent';
`)
}

func (repo *SQLiteRepository) Save(ctx context.Context, database WorkspaceDatabase) error {
	if err := repo.Initialize(ctx); err != nil {
		return err
	}
	ensureTables(&database)
	if database.SavedAt == "" {
		database.SavedAt = nowISO()
	}
	raw, err := json.Marshal(database)
	if err != nil {
		return err
	}

	sqlText := []string{
		"BEGIN;",
		"DELETE FROM workspace_snapshots;",
		"DELETE FROM sync_intents;",
		"DELETE FROM mission_events;",
		"DELETE FROM mission_control_states;",
		"DELETE FROM work_items;",
		"DELETE FROM requirements;",
		"DELETE FROM repository_targets;",
		"DELETE FROM projects;",
		"DELETE FROM connections;",
		"DELETE FROM ui_preferences;",
		"DELETE FROM checkpoints;",
		"DELETE FROM attempts;",
		"DELETE FROM pipelines;",
		"DELETE FROM proof_records;",
		"DELETE FROM operations;",
		"DELETE FROM handoff_bundles;",
		"DELETE FROM operation_queue;",
		"DELETE FROM missions;",
		"DELETE FROM run_workpads;",
		"DELETE FROM workflow_templates;",
		fmt.Sprintf("INSERT INTO workspace_snapshots (id, database_json, saved_at) VALUES ('default', %s, %s);", sqlQuote(string(raw)), sqlQuote(database.SavedAt)),
	}

	for _, project := range database.Tables.Projects {
		sqlText = append(sqlText, fmt.Sprintf(
			"INSERT OR REPLACE INTO projects VALUES (%s,%s,%s,%s,%s,%s,%s,%s);",
			q(project, "id"), q(project, "name"), q(project, "description"), q(project, "team"), q(project, "status"),
			jsonQ(project["labels"]), q(project, "createdAt"), q(project, "updatedAt"),
		))
	}
	for _, target := range repositoryTargetRecordsFromProjects(database.Tables.Projects) {
		sqlText = append(sqlText, fmt.Sprintf(
			"INSERT OR REPLACE INTO repository_targets VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s);",
			q(target, "id"), q(target, "projectId"), q(target, "kind"), q(target, "label"),
			nullableQ(target["owner"]), nullableQ(target["repo"]), nullableQ(target["path"]), nullableQ(target["url"]),
			nullableQ(target["defaultBranch"]), jsonQ(target["target"]), q(target, "createdAt"), q(target, "updatedAt"),
		))
	}
	for _, requirement := range database.Tables.Requirements {
		sqlText = append(sqlText, fmt.Sprintf(
			"INSERT OR REPLACE INTO requirements VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s);",
			q(requirement, "id"), q(requirement, "projectId"), nullableQ(requirement["repositoryTargetId"]), q(requirement, "source"),
			nullableQ(requirement["sourceExternalRef"]), q(requirement, "title"), q(requirement, "rawText"), jsonQ(requirement["structured"]),
			jsonQ(requirement["acceptanceCriteria"]), jsonQ(requirement["risks"]), q(requirement, "status"),
			q(requirement, "createdAt"), q(requirement, "updatedAt"),
		))
	}
	for _, item := range database.Tables.WorkItems {
		sqlText = append(sqlText, fmt.Sprintf(
			`INSERT OR REPLACE INTO work_items (
  id, project_id, key, title, description, status, priority, assignee, labels_json, team, stage_id, target, created_at, updated_at, record_json
) VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s);`,
			q(item, "id"), q(item, "projectId"), q(item, "key"), q(item, "title"), q(item, "description"), q(item, "status"),
			q(item, "priority"), q(item, "assignee"), jsonQ(item["labels"]), q(item, "team"), q(item, "stageId"), q(item, "target"),
			q(item, "createdAt"), q(item, "updatedAt"), jsonQ(item),
		))
	}
	for _, state := range database.Tables.MissionControlStates {
		sqlText = append(sqlText, fmt.Sprintf(
			"INSERT OR REPLACE INTO mission_control_states VALUES (%s,%s,%s,%s,%s,%s);",
			q(state, "runId"), q(state, "projectId"), jsonQ(state["workItems"]), jsonQ(state["events"]), jsonQ(state["syncIntents"]), q(state, "updatedAt"),
		))
	}
	for _, record := range database.Tables.MissionEvents {
		sqlText = append(sqlText, fmt.Sprintf("INSERT OR REPLACE INTO mission_events VALUES (%s,%s,%d,%s);",
			q(record, "id"), q(record, "runId"), intValue(record["sequence"]), jsonQ(record["event"])))
	}
	for _, record := range database.Tables.SyncIntents {
		sqlText = append(sqlText, fmt.Sprintf("INSERT OR REPLACE INTO sync_intents VALUES (%s,%s,%d,%s);",
			q(record, "id"), q(record, "runId"), intValue(record["sequence"]), jsonQ(record["intent"])))
	}
	for _, connection := range database.Tables.Connections {
		sqlText = append(sqlText, fmt.Sprintf("INSERT OR REPLACE INTO connections VALUES (%s,%s,%s,%s,%s);",
			q(connection, "providerId"), q(connection, "status"), jsonQ(connection["grantedPermissions"]), nullableQ(connection["connectedAs"]), q(connection, "updatedAt")))
	}
	for _, ui := range database.Tables.UIPreferences {
		sqlText = append(sqlText, fmt.Sprintf("INSERT OR REPLACE INTO ui_preferences VALUES (%s,%s,%s,%s,%d,%s,%s,%s,%s,%s,%s);",
			q(ui, "id"), q(ui, "activeNav"), q(ui, "selectedProviderId"), q(ui, "selectedWorkItemId"), boolInt(ui["inspectorOpen"]),
			q(ui, "activeInspectorPanel"), q(ui, "runnerPreset"), q(ui, "statusFilter"), q(ui, "assigneeFilter"), q(ui, "sortDirection"), jsonQ(ui["collapsedGroups"])))
	}
	for _, pipeline := range database.Tables.Pipelines {
		sqlText = append(sqlText, fmt.Sprintf("INSERT OR REPLACE INTO pipelines VALUES (%s,%s,%s,%s,%s,%s,%s);",
			q(pipeline, "id"), q(pipeline, "workItemId"), q(pipeline, "runId"), q(pipeline, "status"), jsonQ(pipeline["run"]), q(pipeline, "createdAt"), q(pipeline, "updatedAt")))
	}
	for _, attempt := range database.Tables.Attempts {
		sqlText = append(sqlText, fmt.Sprintf(`INSERT OR REPLACE INTO attempts (
  id, item_id, pipeline_id, repository_target_id, status, trigger, runner, current_stage_id,
  workspace_path, branch_name, pull_request_url, started_at, finished_at, duration_ms,
  error_message, stdout_summary, stderr_summary, stages_json, events_json,
  feishu_failure_notified_at, feishu_failure_status, feishu_failure_message_id, feishu_failure_json,
  record_json, created_at, updated_at
) VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%d,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s);`,
			q(attempt, "id"), q(attempt, "itemId"), q(attempt, "pipelineId"), nullableQ(attempt["repositoryTargetId"]), q(attempt, "status"),
			q(attempt, "trigger"), q(attempt, "runner"), nullableQ(attempt["currentStageId"]), nullableQ(attempt["workspacePath"]),
			nullableQ(attempt["branchName"]), nullableQ(attempt["pullRequestUrl"]), q(attempt, "startedAt"), nullableQ(attempt["finishedAt"]),
			intValue(attempt["durationMs"]), nullableQ(attempt["errorMessage"]), nullableQ(attempt["stdoutSummary"]), nullableQ(attempt["stderrSummary"]),
			jsonQ(attempt["stages"]), jsonQ(attempt["events"]), nullableQ(attempt["feishuFailureNotifiedAt"]), nullableQ(attemptFeishuFailureStatus(attempt)),
			nullableQ(attemptFeishuFailureMessageID(attempt)), attemptFeishuFailureQ(attempt), jsonQ(attempt), q(attempt, "createdAt"), q(attempt, "updatedAt")))
	}
	for _, checkpoint := range database.Tables.Checkpoints {
		sqlText = append(sqlText, fmt.Sprintf(`INSERT OR REPLACE INTO checkpoints (
  id, pipeline_id, attempt_id, stage_id, status, title, summary, decision_note,
  feishu_review_status, feishu_task_guid, feishu_task_id, feishu_message_id, feishu_review_json,
  created_at, updated_at
) VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s);`,
			q(checkpoint, "id"), q(checkpoint, "pipelineId"), nullableQ(checkpoint["attemptId"]), q(checkpoint, "stageId"), q(checkpoint, "status"),
			q(checkpoint, "title"), q(checkpoint, "summary"), nullableQ(checkpoint["decisionNote"]), nullableQ(checkpointFeishuReviewStatus(checkpoint)), nullableQ(checkpointFeishuTaskGuid(checkpoint)),
			nullableQ(checkpointFeishuTaskID(checkpoint)), nullableQ(checkpointFeishuMessageID(checkpoint)), checkpointFeishuReviewQ(checkpoint),
			q(checkpoint, "createdAt"), q(checkpoint, "updatedAt")))
	}
	for _, mission := range database.Tables.Missions {
		sqlText = append(sqlText, fmt.Sprintf("INSERT OR REPLACE INTO missions VALUES (%s,%s,%s,%s,%s,%s,%s,%s);",
			q(mission, "id"), q(mission, "pipelineId"), q(mission, "workItemId"), q(mission, "title"), q(mission, "status"),
			jsonQ(mission["mission"]), q(mission, "createdAt"), q(mission, "updatedAt")))
	}
	for _, operation := range database.Tables.Operations {
		sqlText = append(sqlText, fmt.Sprintf(`INSERT OR REPLACE INTO operations (
  id, mission_id, stage_id, agent_id, status, prompt, required_proof_json, record_json, created_at, updated_at
) VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s);`,
			q(operation, "id"), q(operation, "missionId"), q(operation, "stageId"), q(operation, "agentId"), q(operation, "status"),
			q(operation, "prompt"), jsonQ(operation["requiredProof"]), operationRecordJSONQ(operation), operationTimestampQ(operation, "createdAt"), operationTimestampQ(operation, "updatedAt")))
	}
	for _, proof := range database.Tables.ProofRecords {
		sqlText = append(sqlText, fmt.Sprintf("INSERT OR REPLACE INTO proof_records VALUES (%s,%s,%s,%s,%s,%s);",
			q(proof, "id"), q(proof, "operationId"), q(proof, "label"), q(proof, "value"), nullableQ(proof["sourcePath"]), q(proof, "createdAt")))
	}
	for _, bundle := range handoffBundleRecordsFromDatabase(database) {
		sqlText = append(sqlText, fmt.Sprintf(
			"INSERT OR REPLACE INTO handoff_bundles VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s);",
			q(bundle, "id"), nullableQ(bundle["attemptId"]), nullableQ(bundle["pipelineId"]), nullableQ(bundle["workItemId"]),
			nullableQ(bundle["repositoryTargetId"]), nullableQ(bundle["sourcePath"]), jsonQ(bundle["bundle"]), jsonQ(bundle["summary"]),
			q(bundle, "createdAt"), q(bundle, "updatedAt"),
		))
	}
	for _, queueItem := range operationQueueRecordsFromDatabase(database) {
		sqlText = append(sqlText, fmt.Sprintf(
			"INSERT OR REPLACE INTO operation_queue VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%d,%s,%s,%s,%d,%s,%s,%s);",
			q(queueItem, "id"), q(queueItem, "operationId"), nullableQ(queueItem["pipelineId"]), nullableQ(queueItem["attemptId"]),
			nullableQ(queueItem["workItemId"]), nullableQ(queueItem["repositoryTargetId"]), nullableQ(queueItem["stageId"]),
			nullableQ(queueItem["agentId"]), q(queueItem, "status"), intValue(queueItem["priority"]),
			nullableQ(queueItem["notBefore"]), nullableQ(queueItem["lockedBy"]), nullableQ(queueItem["lockExpiresAt"]),
			intValue(queueItem["attemptCount"]), jsonQ(queueItem["queue"]), q(queueItem, "createdAt"), q(queueItem, "updatedAt"),
		))
	}
	for _, workpad := range database.Tables.RunWorkpads {
		sqlText = append(sqlText, fmt.Sprintf(`INSERT OR REPLACE INTO run_workpads (
  id, attempt_id, pipeline_id, work_item_id, repository_target_id, status, workpad_json, record_json, created_at, updated_at
) VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s);`,
			q(workpad, "id"), q(workpad, "attemptId"), q(workpad, "pipelineId"), q(workpad, "workItemId"), nullableQ(workpad["repositoryTargetId"]),
			q(workpad, "status"), jsonQ(workpad["workpad"]), jsonQ(workpad), q(workpad, "createdAt"), q(workpad, "updatedAt")))
	}
	for _, workflowTemplate := range database.Tables.WorkflowTemplates {
		sqlText = append(sqlText, fmt.Sprintf("INSERT OR REPLACE INTO workflow_templates VALUES (%s,%s,%s,%s,%s,%s,%d,%s,%s,%s,%s);",
			q(workflowTemplate, "id"), q(workflowTemplate, "scope"), q(workflowTemplate, "projectId"), nullableQ(workflowTemplate["repositoryTargetId"]),
			q(workflowTemplate, "templateId"), q(workflowTemplate, "source"), intValue(workflowTemplate["version"]), q(workflowTemplate, "markdown"),
			jsonQ(workflowTemplate["validation"]), q(workflowTemplate, "createdAt"), q(workflowTemplate, "updatedAt")))
	}
	sqlText = append(sqlText, "COMMIT;")
	return repo.exec(ctx, strings.Join(sqlText, "\n"))
}

func (repo *SQLiteRepository) SaveSupervisorExecutionState(ctx context.Context, database WorkspaceDatabase) error {
	if err := repo.Initialize(ctx); err != nil {
		return err
	}
	ensureTables(&database)
	sqlText := []string{"BEGIN;"}
	for _, item := range database.Tables.WorkItems {
		sqlText = append(sqlText, fmt.Sprintf("INSERT OR REPLACE INTO work_items VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s);",
			q(item, "id"), q(item, "projectId"), q(item, "key"), q(item, "title"), q(item, "description"), q(item, "status"),
			q(item, "priority"), q(item, "assignee"), jsonQ(item["labels"]), q(item, "team"), q(item, "stageId"), q(item, "target"),
			q(item, "createdAt"), q(item, "updatedAt"), jsonQ(item),
		))
	}
	for _, pipeline := range database.Tables.Pipelines {
		sqlText = append(sqlText, fmt.Sprintf("INSERT OR REPLACE INTO pipelines VALUES (%s,%s,%s,%s,%s,%s,%s);",
			q(pipeline, "id"), q(pipeline, "workItemId"), q(pipeline, "runId"), q(pipeline, "status"), jsonQ(pipeline["run"]), q(pipeline, "createdAt"), q(pipeline, "updatedAt")))
	}
	for _, attempt := range database.Tables.Attempts {
		sqlText = append(sqlText, fmt.Sprintf(`INSERT OR REPLACE INTO attempts (
  id, item_id, pipeline_id, repository_target_id, status, trigger, runner, current_stage_id,
  workspace_path, branch_name, pull_request_url, started_at, finished_at, duration_ms,
  error_message, stdout_summary, stderr_summary, stages_json, events_json,
  feishu_failure_notified_at, feishu_failure_status, feishu_failure_message_id, feishu_failure_json,
  record_json, created_at, updated_at
) VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%d,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s);`,
			q(attempt, "id"), q(attempt, "itemId"), q(attempt, "pipelineId"), nullableQ(attempt["repositoryTargetId"]), q(attempt, "status"),
			q(attempt, "trigger"), q(attempt, "runner"), nullableQ(attempt["currentStageId"]), nullableQ(attempt["workspacePath"]),
			nullableQ(attempt["branchName"]), nullableQ(attempt["pullRequestUrl"]), q(attempt, "startedAt"), nullableQ(attempt["finishedAt"]),
			intValue(attempt["durationMs"]), nullableQ(attempt["errorMessage"]), nullableQ(attempt["stdoutSummary"]), nullableQ(attempt["stderrSummary"]),
			jsonQ(attempt["stages"]), jsonQ(attempt["events"]), nullableQ(attempt["feishuFailureNotifiedAt"]), nullableQ(attemptFeishuFailureStatus(attempt)),
			nullableQ(attemptFeishuFailureMessageID(attempt)), attemptFeishuFailureQ(attempt), jsonQ(attempt), q(attempt, "createdAt"), q(attempt, "updatedAt")))
	}
	for _, checkpoint := range database.Tables.Checkpoints {
		sqlText = append(sqlText, fmt.Sprintf(`INSERT OR REPLACE INTO checkpoints (
  id, pipeline_id, attempt_id, stage_id, status, title, summary, decision_note,
  feishu_review_status, feishu_task_guid, feishu_task_id, feishu_message_id, feishu_review_json,
  created_at, updated_at
) VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s);`,
			q(checkpoint, "id"), q(checkpoint, "pipelineId"), nullableQ(checkpoint["attemptId"]), q(checkpoint, "stageId"), q(checkpoint, "status"),
			q(checkpoint, "title"), q(checkpoint, "summary"), nullableQ(checkpoint["decisionNote"]), nullableQ(checkpointFeishuReviewStatus(checkpoint)), nullableQ(checkpointFeishuTaskGuid(checkpoint)),
			nullableQ(checkpointFeishuTaskID(checkpoint)), nullableQ(checkpointFeishuMessageID(checkpoint)), checkpointFeishuReviewQ(checkpoint),
			q(checkpoint, "createdAt"), q(checkpoint, "updatedAt")))
	}
	for _, mission := range database.Tables.Missions {
		sqlText = append(sqlText, fmt.Sprintf("INSERT OR REPLACE INTO missions VALUES (%s,%s,%s,%s,%s,%s,%s,%s);",
			q(mission, "id"), q(mission, "pipelineId"), q(mission, "workItemId"), q(mission, "title"), q(mission, "status"),
			jsonQ(mission["mission"]), q(mission, "createdAt"), q(mission, "updatedAt")))
	}
	for _, operation := range database.Tables.Operations {
		sqlText = append(sqlText, fmt.Sprintf(`INSERT OR REPLACE INTO operations (
  id, mission_id, stage_id, agent_id, status, prompt, required_proof_json, record_json, created_at, updated_at
) VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s);`,
			q(operation, "id"), q(operation, "missionId"), q(operation, "stageId"), q(operation, "agentId"), q(operation, "status"),
			q(operation, "prompt"), jsonQ(operation["requiredProof"]), operationRecordJSONQ(operation), operationTimestampQ(operation, "createdAt"), operationTimestampQ(operation, "updatedAt")))
	}
	for _, proof := range database.Tables.ProofRecords {
		sqlText = append(sqlText, fmt.Sprintf("INSERT OR REPLACE INTO proof_records VALUES (%s,%s,%s,%s,%s,%s);",
			q(proof, "id"), q(proof, "operationId"), q(proof, "label"), q(proof, "value"), nullableQ(proof["sourcePath"]), q(proof, "createdAt")))
	}
	for _, workpad := range database.Tables.RunWorkpads {
		sqlText = append(sqlText, fmt.Sprintf(`INSERT OR REPLACE INTO run_workpads (
  id, attempt_id, pipeline_id, work_item_id, repository_target_id, status, workpad_json, record_json, created_at, updated_at
) VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s);`,
			q(workpad, "id"), q(workpad, "attemptId"), q(workpad, "pipelineId"), q(workpad, "workItemId"), nullableQ(workpad["repositoryTargetId"]),
			q(workpad, "status"), jsonQ(workpad["workpad"]), jsonQ(workpad), q(workpad, "createdAt"), q(workpad, "updatedAt")))
	}
	sqlText = append(sqlText, repo.compatSnapshotMirrorSQL(ctx, database))
	sqlText = append(sqlText, "COMMIT;")
	return repo.exec(ctx, strings.Join(sqlText, "\n"))
}

func (repo *SQLiteRepository) UpsertRunWorkpad(ctx context.Context, record map[string]any) error {
	if err := repo.Initialize(ctx); err != nil {
		return err
	}
	if strings.TrimSpace(text(record, "id")) == "" {
		return errors.New("run workpad id is required")
	}
	if strings.TrimSpace(text(record, "createdAt")) == "" {
		record["createdAt"] = nowISO()
	}
	if strings.TrimSpace(text(record, "updatedAt")) == "" {
		record["updatedAt"] = nowISO()
	}
	sqlText := []string{
		"BEGIN;",
		fmt.Sprintf(`INSERT OR REPLACE INTO run_workpads (
  id, attempt_id, pipeline_id, work_item_id, repository_target_id, status, workpad_json, record_json, created_at, updated_at
) VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s);`,
			q(record, "id"), q(record, "attemptId"), q(record, "pipelineId"), q(record, "workItemId"), nullableQ(record["repositoryTargetId"]),
			q(record, "status"), jsonQ(record["workpad"]), jsonQ(record), q(record, "createdAt"), q(record, "updatedAt")),
		repo.compatSnapshotRunWorkpadSQL(ctx, record),
		"COMMIT;",
	}
	return repo.exec(ctx, strings.Join(sqlText, "\n"))
}

func (repo *SQLiteRepository) SaveWorkItemsState(ctx context.Context, database WorkspaceDatabase) error {
	if err := repo.Initialize(ctx); err != nil {
		return err
	}
	ensureTables(&database)
	sqlText := []string{"BEGIN;"}
	for _, project := range database.Tables.Projects {
		sqlText = append(sqlText, fmt.Sprintf("INSERT OR REPLACE INTO projects VALUES (%s,%s,%s,%s,%s,%s,%s,%s);",
			q(project, "id"), q(project, "name"), q(project, "description"), q(project, "team"), q(project, "status"), jsonQ(project["labels"]), q(project, "createdAt"), q(project, "updatedAt")))
		for _, target := range arrayMaps(project["repositoryTargets"]) {
			targetID := text(target, "id")
			if targetID == "" {
				continue
			}
			sqlText = append(sqlText, fmt.Sprintf(`INSERT OR REPLACE INTO repository_targets (
  id, project_id, kind, label, owner, repo, path, url, default_branch, target_json, created_at, updated_at
) VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s);`,
				sqlQuote(targetID), q(project, "id"), q(target, "kind"), sqlQuote(repositoryTargetLabel(target)), nullableQ(target["owner"]),
				nullableQ(target["repo"]), nullableQ(target["path"]), nullableQ(target["url"]), nullableQ(target["defaultBranch"]),
				jsonQ(target), sqlQuote(stringOr(text(target, "createdAt"), text(project, "createdAt"))), sqlQuote(stringOr(text(target, "updatedAt"), text(project, "updatedAt")))))
		}
	}
	for _, requirement := range database.Tables.Requirements {
		sqlText = append(sqlText, fmt.Sprintf("INSERT OR REPLACE INTO requirements VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s);",
			q(requirement, "id"), q(requirement, "projectId"), nullableQ(requirement["repositoryTargetId"]), q(requirement, "source"), nullableQ(requirement["sourceExternalRef"]),
			q(requirement, "title"), q(requirement, "rawText"), jsonQ(requirement["structured"]), jsonQ(requirement["acceptanceCriteria"]),
			jsonQ(requirement["risks"]), q(requirement, "status"), q(requirement, "createdAt"), q(requirement, "updatedAt")))
	}
	for _, item := range database.Tables.WorkItems {
		sqlText = append(sqlText, fmt.Sprintf("INSERT OR REPLACE INTO work_items VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s);",
			q(item, "id"), q(item, "projectId"), q(item, "key"), q(item, "title"), q(item, "description"), q(item, "status"),
			q(item, "priority"), q(item, "assignee"), jsonQ(item["labels"]), q(item, "team"), q(item, "stageId"), q(item, "target"),
			q(item, "createdAt"), q(item, "updatedAt"), jsonQ(item),
		))
	}
	for _, state := range database.Tables.MissionControlStates {
		sqlText = append(sqlText, fmt.Sprintf(
			"INSERT OR REPLACE INTO mission_control_states VALUES (%s,%s,%s,%s,%s,%s);",
			q(state, "runId"), q(state, "projectId"), jsonQ(state["workItems"]), jsonQ(state["events"]), jsonQ(state["syncIntents"]), q(state, "updatedAt"),
		))
	}
	sqlText = append(sqlText, repo.compatSnapshotWorkItemsSQL(ctx, database))
	sqlText = append(sqlText, "COMMIT;")
	return repo.exec(ctx, strings.Join(sqlText, "\n"))
}

func (repo *SQLiteRepository) DeleteWorkItemState(ctx context.Context, database WorkspaceDatabase, itemID string, requirementID string) error {
	if err := repo.Initialize(ctx); err != nil {
		return err
	}
	ensureTables(&database)
	sqlText := []string{
		"BEGIN;",
		fmt.Sprintf("DELETE FROM work_items WHERE id = %s;", sqlQuote(itemID)),
	}
	if strings.TrimSpace(requirementID) != "" {
		sqlText = append(sqlText, fmt.Sprintf("DELETE FROM requirements WHERE id = %s;", sqlQuote(requirementID)))
	}
	for _, state := range database.Tables.MissionControlStates {
		sqlText = append(sqlText, fmt.Sprintf(
			"INSERT OR REPLACE INTO mission_control_states VALUES (%s,%s,%s,%s,%s,%s);",
			q(state, "runId"), q(state, "projectId"), jsonQ(state["workItems"]), jsonQ(state["events"]), jsonQ(state["syncIntents"]), q(state, "updatedAt"),
		))
	}
	sqlText = append(sqlText, repo.compatSnapshotWorkItemsSQL(ctx, database))
	sqlText = append(sqlText, "COMMIT;")
	return repo.exec(ctx, strings.Join(sqlText, "\n"))
}

func (repo *SQLiteRepository) DeleteRepositoryTargetState(ctx context.Context, database WorkspaceDatabase, targetID string, result repositoryTargetDeleteResult) error {
	if err := repo.Initialize(ctx); err != nil {
		return err
	}
	ensureTables(&database)
	sqlText := []string{
		"BEGIN;",
		fmt.Sprintf("DELETE FROM repository_targets WHERE id = %s;", sqlQuote(targetID)),
		fmt.Sprintf("DELETE FROM workflow_templates WHERE repository_target_id = %s;", sqlQuote(targetID)),
		fmt.Sprintf("DELETE FROM agent_profiles WHERE repository_target_id = %s;", sqlQuote(targetID)),
	}
	for _, project := range database.Tables.Projects {
		sqlText = append(sqlText, fmt.Sprintf("INSERT OR REPLACE INTO projects VALUES (%s,%s,%s,%s,%s,%s,%s,%s);",
			q(project, "id"), q(project, "name"), q(project, "description"), q(project, "team"), q(project, "status"), jsonQ(project["labels"]), q(project, "createdAt"), q(project, "updatedAt")))
	}
	sqlText = append(sqlText, deleteWhereSQL("requirements",
		fmt.Sprintf("repository_target_id = %s", sqlQuote(targetID)),
		sqlInClause("id", result.RequirementIDs),
	))
	sqlText = append(sqlText, deleteWhereSQL("work_items", sqlInClause("id", result.WorkItemIDs)))
	sqlText = append(sqlText, deleteWhereSQL("pipelines", sqlInClause("id", result.PipelineIDs)))
	sqlText = append(sqlText, deleteWhereSQL("attempts",
		fmt.Sprintf("repository_target_id = %s", sqlQuote(targetID)),
		sqlInClause("id", result.AttemptIDs),
	))
	sqlText = append(sqlText, deleteWhereSQL("run_workpads",
		fmt.Sprintf("repository_target_id = %s", sqlQuote(targetID)),
		sqlInClause("attempt_id", result.AttemptIDs),
		sqlInClause("pipeline_id", result.PipelineIDs),
		sqlInClause("work_item_id", result.WorkItemIDs),
	))
	sqlText = append(sqlText, deleteWhereSQL("checkpoints",
		sqlInClause("pipeline_id", result.PipelineIDs),
		sqlInClause("attempt_id", result.AttemptIDs),
	))
	sqlText = append(sqlText, deleteWhereSQL("missions",
		sqlInClause("id", result.MissionIDs),
		sqlInClause("pipeline_id", result.PipelineIDs),
		sqlInClause("work_item_id", result.WorkItemIDs),
	))
	proofDeleteClauses := []string{sqlInClause("operation_id", result.OperationIDs)}
	if missionClause := sqlInClause("mission_id", result.MissionIDs); missionClause != "" {
		proofDeleteClauses = append(proofDeleteClauses, fmt.Sprintf("operation_id IN (SELECT id FROM operations WHERE %s)", missionClause))
	}
	sqlText = append(sqlText, deleteWhereSQL("proof_records", proofDeleteClauses...))
	sqlText = append(sqlText, deleteWhereSQL("operations",
		sqlInClause("id", result.OperationIDs),
		sqlInClause("mission_id", result.MissionIDs),
	))
	sqlText = append(sqlText, deleteWhereSQL("handoff_bundles",
		fmt.Sprintf("repository_target_id = %s", sqlQuote(targetID)),
		sqlInClause("attempt_id", result.AttemptIDs),
		sqlInClause("pipeline_id", result.PipelineIDs),
		sqlInClause("work_item_id", result.WorkItemIDs),
	))
	sqlText = append(sqlText, deleteWhereSQL("operation_queue",
		fmt.Sprintf("repository_target_id = %s", sqlQuote(targetID)),
		sqlInClause("attempt_id", result.AttemptIDs),
		sqlInClause("pipeline_id", result.PipelineIDs),
		sqlInClause("work_item_id", result.WorkItemIDs),
	))
	for _, state := range database.Tables.MissionControlStates {
		sqlText = append(sqlText, fmt.Sprintf(
			"INSERT OR REPLACE INTO mission_control_states VALUES (%s,%s,%s,%s,%s,%s);",
			q(state, "runId"), q(state, "projectId"), jsonQ(state["workItems"]), jsonQ(state["events"]), jsonQ(state["syncIntents"]), q(state, "updatedAt"),
		))
	}
	// Repository workspace deletion is a normalized-table operation. Do not mirror
	// into the legacy full workspace snapshot here; old snapshots can be hundreds
	// of MB and make a simple delete look hung.
	sqlText = append(sqlText, "COMMIT;")
	return repo.exec(ctx, strings.Join(sqlText, "\n"))
}

func (repo *SQLiteRepository) UpsertWorkflowTemplate(ctx context.Context, record map[string]any) error {
	if err := repo.Initialize(ctx); err != nil {
		return err
	}
	if strings.TrimSpace(text(record, "id")) == "" {
		return errors.New("workflow template id is required")
	}
	if strings.TrimSpace(text(record, "createdAt")) == "" {
		record["createdAt"] = nowISO()
	}
	if strings.TrimSpace(text(record, "updatedAt")) == "" {
		record["updatedAt"] = nowISO()
	}
	if intValue(record["version"]) <= 0 {
		record["version"] = 1
	}
	return repo.exec(ctx, fmt.Sprintf("INSERT OR REPLACE INTO workflow_templates VALUES (%s,%s,%s,%s,%s,%s,%d,%s,%s,%s,%s);",
		q(record, "id"), q(record, "scope"), q(record, "projectId"), nullableQ(record["repositoryTargetId"]),
		q(record, "templateId"), q(record, "source"), intValue(record["version"]), q(record, "markdown"),
		jsonQ(record["validation"]), q(record, "createdAt"), q(record, "updatedAt")))
}

func (repo *SQLiteRepository) UpsertConnection(ctx context.Context, connection map[string]any) error {
	if err := repo.Initialize(ctx); err != nil {
		return err
	}
	if strings.TrimSpace(text(connection, "providerId")) == "" {
		return errors.New("connection providerId is required")
	}
	if strings.TrimSpace(text(connection, "status")) == "" {
		connection["status"] = "connected"
	}
	if strings.TrimSpace(text(connection, "updatedAt")) == "" {
		connection["updatedAt"] = nowISO()
	}
	sqlText := []string{
		"BEGIN;",
		fmt.Sprintf("INSERT OR REPLACE INTO connections VALUES (%s,%s,%s,%s,%s);",
			q(connection, "providerId"), q(connection, "status"), jsonQ(connection["grantedPermissions"]), nullableQ(connection["connectedAs"]), q(connection, "updatedAt")),
		repo.compatSnapshotConnectionSQL(ctx, connection),
		"COMMIT;",
	}
	return repo.exec(ctx, strings.Join(sqlText, "\n"))
}

func (repo *SQLiteRepository) compatSnapshotMirrorSQL(ctx context.Context, executionState WorkspaceDatabase) string {
	output, err := repo.query(ctx, "SELECT database_json FROM workspace_snapshots WHERE id = 'default';")
	if err != nil {
		return ""
	}
	raw := strings.TrimSpace(output)
	if raw == "" {
		return ""
	}
	var snapshot WorkspaceDatabase
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		return ""
	}
	ensureTables(&snapshot)
	snapshot.Tables.WorkItems = executionState.Tables.WorkItems
	snapshot.Tables.Pipelines = executionState.Tables.Pipelines
	snapshot.Tables.Attempts = executionState.Tables.Attempts
	snapshot.Tables.Checkpoints = executionState.Tables.Checkpoints
	snapshot.Tables.RunWorkpads = executionState.Tables.RunWorkpads
	snapshot.Tables.Missions = nil
	snapshot.Tables.Operations = nil
	snapshot.Tables.ProofRecords = nil
	snapshot.SavedAt = nowISO()
	nextRaw, err := json.Marshal(snapshot)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("UPDATE workspace_snapshots SET database_json = %s, saved_at = %s WHERE id = 'default';", sqlQuote(string(nextRaw)), sqlQuote(snapshot.SavedAt))
}

func (repo *SQLiteRepository) compatSnapshotConnectionSQL(ctx context.Context, connection map[string]any) string {
	output, err := repo.query(ctx, "SELECT database_json FROM workspace_snapshots WHERE id = 'default';")
	if err != nil {
		return ""
	}
	raw := strings.TrimSpace(output)
	if raw == "" {
		return ""
	}
	var snapshot WorkspaceDatabase
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		return ""
	}
	ensureTables(&snapshot)
	snapshot.Tables.Connections = appendOrReplaceByKey(snapshot.Tables.Connections, connection, "providerId")
	snapshot.SavedAt = nowISO()
	nextRaw, err := json.Marshal(snapshot)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("UPDATE workspace_snapshots SET database_json = %s, saved_at = %s WHERE id = 'default';", sqlQuote(string(nextRaw)), sqlQuote(snapshot.SavedAt))
}

func (repo *SQLiteRepository) compatSnapshotRepositoryTargetDeleteSQL(ctx context.Context, database WorkspaceDatabase) string {
	output, err := repo.query(ctx, "SELECT database_json FROM workspace_snapshots WHERE id = 'default';")
	if err != nil {
		return ""
	}
	raw := strings.TrimSpace(output)
	if raw == "" {
		return ""
	}
	var snapshot WorkspaceDatabase
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		return ""
	}
	ensureTables(&snapshot)
	snapshot.Tables.Projects = database.Tables.Projects
	snapshot.Tables.Requirements = database.Tables.Requirements
	snapshot.Tables.WorkItems = database.Tables.WorkItems
	snapshot.Tables.MissionControlStates = database.Tables.MissionControlStates
	snapshot.Tables.Pipelines = database.Tables.Pipelines
	snapshot.Tables.Attempts = database.Tables.Attempts
	snapshot.Tables.Checkpoints = database.Tables.Checkpoints
	snapshot.Tables.RunWorkpads = database.Tables.RunWorkpads
	snapshot.Tables.Missions = database.Tables.Missions
	snapshot.Tables.Operations = database.Tables.Operations
	snapshot.Tables.ProofRecords = database.Tables.ProofRecords
	snapshot.SavedAt = nowISO()
	nextRaw, err := json.Marshal(snapshot)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("UPDATE workspace_snapshots SET database_json = %s, saved_at = %s WHERE id = 'default';", sqlQuote(string(nextRaw)), sqlQuote(snapshot.SavedAt))
}

func (repo *SQLiteRepository) compatSnapshotWorkItemsSQL(ctx context.Context, sessionState WorkspaceDatabase) string {
	output, err := repo.query(ctx, "SELECT database_json FROM workspace_snapshots WHERE id = 'default';")
	if err != nil {
		return ""
	}
	raw := strings.TrimSpace(output)
	if raw == "" {
		return ""
	}
	var snapshot WorkspaceDatabase
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		return ""
	}
	ensureTables(&snapshot)
	snapshot.Tables.Projects = sessionState.Tables.Projects
	snapshot.Tables.Requirements = sessionState.Tables.Requirements
	snapshot.Tables.WorkItems = sessionState.Tables.WorkItems
	snapshot.Tables.MissionControlStates = sessionState.Tables.MissionControlStates
	snapshot.SavedAt = nowISO()
	nextRaw, err := json.Marshal(snapshot)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("UPDATE workspace_snapshots SET database_json = %s, saved_at = %s WHERE id = 'default';", sqlQuote(string(nextRaw)), sqlQuote(snapshot.SavedAt))
}

func (repo *SQLiteRepository) compatSnapshotRunWorkpadSQL(ctx context.Context, record map[string]any) string {
	output, err := repo.query(ctx, "SELECT database_json FROM workspace_snapshots WHERE id = 'default';")
	if err != nil {
		return ""
	}
	raw := strings.TrimSpace(output)
	if raw == "" {
		return ""
	}
	var snapshot WorkspaceDatabase
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		return ""
	}
	ensureTables(&snapshot)
	snapshot.Tables.RunWorkpads = appendOrReplace(snapshot.Tables.RunWorkpads, record)
	snapshot.SavedAt = nowISO()
	nextRaw, err := json.Marshal(snapshot)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("UPDATE workspace_snapshots SET database_json = %s, saved_at = %s WHERE id = 'default';", sqlQuote(string(nextRaw)), sqlQuote(snapshot.SavedAt))
}

func (repo *SQLiteRepository) Load(ctx context.Context) (*WorkspaceDatabase, error) {
	if err := repo.Initialize(ctx); err != nil {
		return nil, err
	}
	database, err := repo.LoadSupervisorExecutionState(ctx)
	if err == nil {
		if connections, err := repo.listSessionConnections(ctx); err == nil {
			database.Tables.Connections = connections
		}
		if uiPreferences, err := repo.listSessionUIPreferences(ctx); err == nil {
			database.Tables.UIPreferences = uiPreferences
		}
		return database, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	output, err := repo.query(ctx, "SELECT database_json FROM workspace_snapshots WHERE id = 'default';")
	if err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(output)
	if raw == "" {
		return nil, sql.ErrNoRows
	}
	var snapshot WorkspaceDatabase
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		return nil, err
	}
	ensureTables(&snapshot)
	return &snapshot, nil
}

func (repo *SQLiteRepository) WorkspaceSavedAt(ctx context.Context) (string, error) {
	if err := repo.Initialize(ctx); err != nil {
		return "", err
	}
	output, err := repo.query(ctx, "SELECT saved_at FROM workspace_snapshots WHERE id = 'default';")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func (repo *SQLiteRepository) Migrations(ctx context.Context) ([]map[string]any, error) {
	if err := repo.Initialize(ctx); err != nil {
		return nil, err
	}
	output, err := repo.query(ctx, `.mode json
SELECT version, name, applied_at AS appliedAt FROM omega_migrations ORDER BY version;
`)
	if err != nil {
		return nil, err
	}
	var migrations []map[string]any
	if err := json.Unmarshal([]byte(output), &migrations); err != nil {
		return nil, err
	}
	return migrations, nil
}

func (repo *SQLiteRepository) GetSetting(ctx context.Context, key string) (map[string]any, error) {
	if err := repo.Initialize(ctx); err != nil {
		return nil, err
	}
	output, err := repo.query(ctx, fmt.Sprintf("SELECT value_json FROM omega_settings WHERE key = %s;", sqlQuote(key)))
	if err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(output)
	if raw == "" {
		return nil, sql.ErrNoRows
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, err
	}
	return value, nil
}

func (repo *SQLiteRepository) SetSetting(ctx context.Context, key string, value map[string]any) error {
	if err := repo.Initialize(ctx); err != nil {
		return err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return repo.exec(ctx, fmt.Sprintf(
		"INSERT OR REPLACE INTO omega_settings (key, value_json, updated_at) VALUES (%s, %s, %s);",
		sqlQuote(key),
		sqlQuote(string(raw)),
		sqlQuote(nowISO()),
	))
}

func (repo *SQLiteRepository) DeleteSetting(ctx context.Context, key string) error {
	if err := repo.Initialize(ctx); err != nil {
		return err
	}
	return repo.exec(ctx, fmt.Sprintf("DELETE FROM omega_settings WHERE key = %s;", sqlQuote(key)))
}

func (repo *SQLiteRepository) ListSettings(ctx context.Context, prefix string) ([]map[string]any, error) {
	if err := repo.Initialize(ctx); err != nil {
		return nil, err
	}
	output, err := repo.query(ctx, fmt.Sprintf(`.mode json
SELECT key, value_json AS valueJson, updated_at AS updatedAt FROM omega_settings WHERE key LIKE %s ORDER BY updated_at DESC;
`, sqlQuote(prefix+"%")))
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(output) == "" {
		return []map[string]any{}, nil
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(output), &rows); err != nil {
		return nil, err
	}
	settings := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		var value map[string]any
		if err := json.Unmarshal([]byte(text(row, "valueJson")), &value); err != nil {
			return nil, err
		}
		if value != nil {
			settings = append(settings, value)
		}
	}
	return settings, nil
}

func (repo *SQLiteRepository) DeleteSettingsByRepositoryTarget(ctx context.Context, prefix string, repositoryTargetID string) ([]map[string]any, error) {
	if err := repo.Initialize(ctx); err != nil {
		return nil, err
	}
	output, err := repo.query(ctx, fmt.Sprintf(`.mode json
SELECT key, value_json AS valueJson FROM omega_settings WHERE key LIKE %s;
`, sqlQuote(prefix+"%")))
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(output) == "" {
		return []map[string]any{}, nil
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(output), &rows); err != nil {
		return nil, err
	}
	deleted := []map[string]any{}
	for _, row := range rows {
		var value map[string]any
		if err := json.Unmarshal([]byte(text(row, "valueJson")), &value); err != nil {
			return nil, err
		}
		if text(value, "repositoryTargetId") != repositoryTargetID {
			continue
		}
		if err := repo.DeleteSetting(ctx, text(row, "key")); err != nil {
			return nil, err
		}
		deleted = append(deleted, value)
	}
	return deleted, nil
}

func (repo *SQLiteRepository) GetAgentProfile(ctx context.Context, projectID string, repositoryTargetID string) (map[string]any, error) {
	if err := repo.Initialize(ctx); err != nil {
		return nil, err
	}
	scope := "project"
	targetSQL := "repository_target_id IS NULL"
	if strings.TrimSpace(repositoryTargetID) != "" {
		scope = "repository"
		targetSQL = "repository_target_id = " + sqlQuote(repositoryTargetID)
	}
	output, err := repo.query(ctx, fmt.Sprintf(
		"SELECT profile_json FROM agent_profiles WHERE project_id = %s AND scope = %s AND %s ORDER BY version DESC LIMIT 1;",
		sqlQuote(stringOr(projectID, "project_omega")),
		sqlQuote(scope),
		targetSQL,
	))
	if err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(output)
	if raw == "" {
		return nil, sql.ErrNoRows
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, err
	}
	return value, nil
}

func (repo *SQLiteRepository) SetAgentProfile(ctx context.Context, profile ProjectAgentProfile) error {
	if err := repo.Initialize(ctx); err != nil {
		return err
	}
	profile = normalizeAgentProfile(profile)
	if profile.UpdatedAt == "" {
		profile.UpdatedAt = nowISO()
	}
	scope := "project"
	if strings.TrimSpace(profile.RepositoryTargetID) != "" {
		scope = "repository"
	}
	id := agentProfileRecordID(profile.ProjectID, profile.RepositoryTargetID)
	raw, err := json.Marshal(profile)
	if err != nil {
		return err
	}
	target := "NULL"
	if profile.RepositoryTargetID != "" {
		target = sqlQuote(profile.RepositoryTargetID)
	}
	now := nowISO()
	return repo.exec(ctx, fmt.Sprintf(`
INSERT INTO agent_profiles (id, scope, project_id, repository_target_id, version, profile_json, created_at, updated_at)
VALUES (%s, %s, %s, %s, COALESCE((SELECT version FROM agent_profiles WHERE id = %s), 0) + 1, %s, COALESCE((SELECT created_at FROM agent_profiles WHERE id = %s), %s), %s)
ON CONFLICT(id) DO UPDATE SET
  scope = excluded.scope,
  project_id = excluded.project_id,
  repository_target_id = excluded.repository_target_id,
  version = excluded.version,
  profile_json = excluded.profile_json,
  updated_at = excluded.updated_at;
`,
		sqlQuote(id),
		sqlQuote(scope),
		sqlQuote(profile.ProjectID),
		target,
		sqlQuote(id),
		sqlQuote(string(raw)),
		sqlQuote(id),
		sqlQuote(now),
		sqlQuote(profile.UpdatedAt),
	))
}

func (repo *SQLiteRepository) SetPagePilotRun(ctx context.Context, run map[string]any) error {
	if err := repo.Initialize(ctx); err != nil {
		return err
	}
	if text(run, "id") == "" {
		return errors.New("page pilot run id is required")
	}
	if text(run, "projectId") == "" {
		run["projectId"] = "project_omega"
	}
	if text(run, "createdAt") == "" {
		run["createdAt"] = nowISO()
	}
	if text(run, "updatedAt") == "" {
		run["updatedAt"] = nowISO()
	}
	raw, err := json.Marshal(run)
	if err != nil {
		return err
	}
	return repo.exec(ctx, fmt.Sprintf(`
INSERT OR REPLACE INTO page_pilot_runs (
  id, project_id, repository_target_id, status, runner, repository_path, branch_name, commit_sha,
  pull_request_url, changed_files_json, run_json, created_at, updated_at
) VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s);
`,
		q(run, "id"),
		q(run, "projectId"),
		q(run, "repositoryTargetId"),
		q(run, "status"),
		nullableQ(run["runner"]),
		nullableQ(run["repositoryPath"]),
		nullableQ(run["branchName"]),
		nullableQ(run["commitSha"]),
		nullableQ(run["pullRequestUrl"]),
		jsonQ(run["changedFiles"]),
		sqlQuote(string(raw)),
		q(run, "createdAt"),
		q(run, "updatedAt"),
	))
}

func (repo *SQLiteRepository) GetPagePilotRun(ctx context.Context, id string) (map[string]any, error) {
	if err := repo.Initialize(ctx); err != nil {
		return nil, err
	}
	output, err := repo.query(ctx, fmt.Sprintf("SELECT run_json FROM page_pilot_runs WHERE id = %s;", sqlQuote(id)))
	if err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(output)
	if raw == "" {
		return nil, sql.ErrNoRows
	}
	var run map[string]any
	if err := json.Unmarshal([]byte(raw), &run); err != nil {
		return nil, err
	}
	return run, nil
}

func (repo *SQLiteRepository) ListPagePilotRuns(ctx context.Context, options PagePilotRunListOptions) ([]map[string]any, error) {
	if err := repo.Initialize(ctx); err != nil {
		return nil, err
	}
	where := []string{}
	if strings.TrimSpace(options.ID) != "" {
		where = append(where, fmt.Sprintf("id = %s", sqlQuote(strings.TrimSpace(options.ID))))
	}
	if strings.TrimSpace(options.ProjectID) != "" {
		where = append(where, fmt.Sprintf("project_id = %s", sqlQuote(strings.TrimSpace(options.ProjectID))))
	}
	if strings.TrimSpace(options.RepositoryTargetID) != "" {
		where = append(where, fmt.Sprintf("repository_target_id = %s", sqlQuote(strings.TrimSpace(options.RepositoryTargetID))))
	}
	if strings.TrimSpace(options.Status) != "" {
		where = append(where, fmt.Sprintf("status = %s", sqlQuote(strings.TrimSpace(options.Status))))
	}
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = "WHERE " + strings.Join(where, " AND ")
	}
	limitSQL := ""
	if options.Limit > 0 {
		if options.Limit > 100 {
			options.Limit = 100
		}
		limitSQL = fmt.Sprintf("LIMIT %d", options.Limit)
	}
	if options.Compact {
		return repo.listPagePilotRunsCompact(ctx, whereSQL, limitSQL)
	}
	output, err := repo.query(ctx, fmt.Sprintf(`.mode json
SELECT run_json AS runJson FROM page_pilot_runs %s ORDER BY updated_at DESC, created_at DESC %s;
`, whereSQL, limitSQL))
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(output) == "" {
		return []map[string]any{}, nil
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(output), &rows); err != nil {
		return nil, err
	}
	runs := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		var run map[string]any
		if err := json.Unmarshal([]byte(text(row, "runJson")), &run); err != nil {
			return nil, err
		}
		if run != nil {
			runs = append(runs, run)
		}
	}
	return runs, nil
}

func (repo *SQLiteRepository) listPagePilotRunsCompact(ctx context.Context, whereSQL string, limitSQL string) ([]map[string]any, error) {
	output, err := repo.query(ctx, fmt.Sprintf(`.mode json
SELECT
  id,
  project_id AS projectId,
  repository_target_id AS repositoryTargetId,
  status,
  runner,
  repository_path AS repositoryPath,
  branch_name AS branchName,
  commit_sha AS commitSha,
  pull_request_url AS pullRequestUrl,
  changed_files_json AS changedFilesJson,
  json_extract(run_json, '$.workItemId') AS workItemId,
  json_extract(run_json, '$.pipelineId') AS pipelineId,
  json_extract(run_json, '$.updatedAt') AS runUpdatedAt,
  created_at AS createdAt,
  updated_at AS updatedAt
FROM page_pilot_runs
%s
ORDER BY updated_at DESC, created_at DESC
%s;
`, whereSQL, limitSQL))
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(output) == "" {
		return []map[string]any{}, nil
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(output), &rows); err != nil {
		return nil, err
	}
	runs := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		run := map[string]any{}
		for _, key := range []string{"id", "projectId", "repositoryTargetId", "status", "runner", "repositoryPath", "branchName", "commitSha", "pullRequestUrl", "workItemId", "pipelineId", "createdAt", "updatedAt"} {
			value := row[key]
			if value != nil && fmt.Sprint(value) != "" {
				run[key] = value
			}
		}
		if text(row, "runUpdatedAt") != "" {
			run["updatedAt"] = text(row, "runUpdatedAt")
		}
		var changedFiles []any
		if raw := text(row, "changedFilesJson"); raw != "" {
			_ = json.Unmarshal([]byte(raw), &changedFiles)
		}
		run["changedFiles"] = changedFiles
		runs = append(runs, run)
	}
	return runs, nil
}

func (repo *SQLiteRepository) DeletePagePilotRunsByRepositoryTarget(ctx context.Context, repositoryTargetID string) ([]map[string]any, error) {
	if err := repo.Initialize(ctx); err != nil {
		return nil, err
	}
	output, err := repo.query(ctx, fmt.Sprintf(`.mode json
SELECT run_json AS runJson FROM page_pilot_runs WHERE repository_target_id = %s ORDER BY updated_at DESC, created_at DESC;
`, sqlQuote(repositoryTargetID)))
	if err != nil {
		return nil, err
	}
	deleted := []map[string]any{}
	if strings.TrimSpace(output) != "" {
		var rows []map[string]any
		if err := json.Unmarshal([]byte(output), &rows); err != nil {
			return nil, err
		}
		for _, row := range rows {
			var run map[string]any
			if err := json.Unmarshal([]byte(text(row, "runJson")), &run); err != nil {
				return nil, err
			}
			if run != nil {
				deleted = append(deleted, run)
			}
		}
	}
	if err := repo.exec(ctx, fmt.Sprintf("DELETE FROM page_pilot_runs WHERE repository_target_id = %s;", sqlQuote(repositoryTargetID))); err != nil {
		return nil, err
	}
	return deleted, nil
}

func (repo *SQLiteRepository) GetFeishuDeliveryReceipt(ctx context.Context, dedupeKey string) (map[string]any, error) {
	if err := repo.Initialize(ctx); err != nil {
		return nil, err
	}
	dedupeKey = strings.TrimSpace(dedupeKey)
	if dedupeKey == "" {
		return nil, sql.ErrNoRows
	}
	output, err := repo.query(ctx, fmt.Sprintf(`.mode json
SELECT
  dedupe_key AS dedupeKey,
  provider,
  kind,
  entity_id AS entityId,
  pipeline_id AS pipelineId,
  attempt_id AS attemptId,
  work_item_id AS workItemId,
  status,
  route,
  format,
  task_guid AS taskGuid,
  task_id AS taskId,
  message_id AS messageId,
  payload_json AS payloadJson,
  created_at AS createdAt,
  updated_at AS updatedAt
FROM feishu_delivery_receipts
WHERE dedupe_key = %s
LIMIT 1;
`, sqlQuote(dedupeKey)))
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(output) == "" {
		return nil, sql.ErrNoRows
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(output), &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, sql.ErrNoRows
	}
	receipt := rows[0]
	if raw := text(receipt, "payloadJson"); raw != "" {
		var payload map[string]any
		if err := json.Unmarshal([]byte(raw), &payload); err == nil && payload != nil {
			receipt["payload"] = payload
		}
	}
	return receipt, nil
}

func (repo *SQLiteRepository) UpsertFeishuDeliveryReceipt(ctx context.Context, receipt map[string]any) error {
	if err := repo.Initialize(ctx); err != nil {
		return err
	}
	dedupeKey := text(receipt, "dedupeKey")
	if dedupeKey == "" {
		return errors.New("feishu delivery dedupe key is required")
	}
	timestamp := nowISO()
	if text(receipt, "createdAt") == "" {
		receipt["createdAt"] = timestamp
	}
	receipt["updatedAt"] = timestamp
	payload := receipt["payload"]
	if payload == nil {
		payload = receipt
	}
	return repo.exec(ctx, fmt.Sprintf(`
INSERT INTO feishu_delivery_receipts (
  dedupe_key, provider, kind, entity_id, pipeline_id, attempt_id, work_item_id,
  status, route, format, task_guid, task_id, message_id, payload_json, created_at, updated_at
) VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s)
ON CONFLICT(dedupe_key) DO UPDATE SET
  provider = excluded.provider,
  kind = excluded.kind,
  entity_id = excluded.entity_id,
  pipeline_id = excluded.pipeline_id,
  attempt_id = excluded.attempt_id,
  work_item_id = excluded.work_item_id,
  status = excluded.status,
  route = excluded.route,
  format = excluded.format,
  task_guid = COALESCE(excluded.task_guid, feishu_delivery_receipts.task_guid),
  task_id = COALESCE(excluded.task_id, feishu_delivery_receipts.task_id),
  message_id = COALESCE(excluded.message_id, feishu_delivery_receipts.message_id),
  payload_json = excluded.payload_json,
  updated_at = excluded.updated_at;
`,
		sqlQuote(dedupeKey),
		sqlQuote(stringOr(text(receipt, "provider"), "feishu")),
		sqlQuote(stringOr(text(receipt, "kind"), "human_review")),
		nullableQ(receipt["entityId"]),
		nullableQ(receipt["pipelineId"]),
		nullableQ(receipt["attemptId"]),
		nullableQ(receipt["workItemId"]),
		sqlQuote(stringOr(text(receipt, "status"), "sent")),
		nullableQ(receipt["route"]),
		nullableQ(receipt["format"]),
		nullableQ(receipt["taskGuid"]),
		nullableQ(receipt["taskId"]),
		nullableQ(receipt["messageId"]),
		jsonQ(payload),
		q(receipt, "createdAt"),
		q(receipt, "updatedAt"),
	))
}

func (repo *SQLiteRepository) AppendRuntimeLog(ctx context.Context, record RuntimeLogRecord) error {
	if err := repo.Initialize(ctx); err != nil {
		return err
	}
	if strings.TrimSpace(record.ID) == "" {
		record.ID = fmt.Sprintf("log_%d", time.Now().UnixNano())
	}
	if strings.TrimSpace(record.CreatedAt) == "" {
		record.CreatedAt = nowISO()
	}
	if strings.TrimSpace(record.Level) == "" {
		record.Level = "INFO"
	}
	if strings.TrimSpace(record.EventType) == "" {
		record.EventType = "runtime.event"
	}
	details, err := json.Marshal(record.Details)
	if err != nil {
		return err
	}
	return repo.exec(ctx, fmt.Sprintf(`
INSERT OR REPLACE INTO runtime_logs (
  id, level, event_type, message, entity_type, entity_id, project_id, repository_target_id,
  requirement_id, work_item_id, pipeline_id, attempt_id, stage_id, agent_id, request_id, details_json, created_at
) VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s);
`,
		sqlQuote(record.ID),
		sqlQuote(strings.ToUpper(record.Level)),
		sqlQuote(record.EventType),
		sqlQuote(record.Message),
		nullableQ(record.EntityType),
		nullableQ(record.EntityID),
		nullableQ(record.ProjectID),
		nullableQ(record.RepositoryTargetID),
		nullableQ(record.RequirementID),
		nullableQ(record.WorkItemID),
		nullableQ(record.PipelineID),
		nullableQ(record.AttemptID),
		nullableQ(record.StageID),
		nullableQ(record.AgentID),
		nullableQ(record.RequestID),
		sqlQuote(string(details)),
		sqlQuote(record.CreatedAt),
	))
}

func (repo *SQLiteRepository) compactNoisyRuntimeLogs(ctx context.Context) error {
	return repo.exec(ctx, `
DELETE FROM runtime_logs
WHERE event_type = 'api.request'
  AND level = 'DEBUG'
  AND id NOT IN (
    SELECT id FROM runtime_logs
    WHERE event_type = 'api.request'
      AND level = 'DEBUG'
    ORDER BY created_at DESC, id DESC
    LIMIT 1000
  );
DELETE FROM runtime_logs
WHERE event_type = 'job_supervisor.tick.completed'
  AND level = 'DEBUG'
  AND id NOT IN (
    SELECT id FROM runtime_logs
    WHERE event_type = 'job_supervisor.tick.completed'
      AND level = 'DEBUG'
    ORDER BY created_at DESC, id DESC
    LIMIT 200
  );
DELETE FROM runtime_logs
WHERE event_type = 'job_supervisor.remote_signals.polled'
  AND level = 'INFO'
  AND id NOT IN (
    SELECT id FROM runtime_logs
    WHERE event_type = 'job_supervisor.remote_signals.polled'
      AND level = 'INFO'
    ORDER BY created_at DESC, id DESC
    LIMIT 200
  );
`)
}

func (repo *SQLiteRepository) ListRuntimeLogs(ctx context.Context, filters map[string]string, limit int) ([]RuntimeLogRecord, error) {
	page, err := repo.ListRuntimeLogsPage(ctx, filters, limit)
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

func (repo *SQLiteRepository) ListRuntimeLogsPage(ctx context.Context, filters map[string]string, limit int) (RuntimeLogPage, error) {
	if err := repo.Initialize(ctx); err != nil {
		return RuntimeLogPage{}, err
	}
	maxLimit := 500
	if filters["export"] == "1" {
		maxLimit = 5000
	}
	if limit <= 0 || limit > maxLimit {
		limit = 100
		if filters["export"] == "1" {
			limit = maxLimit
		}
	}
	where := []string{"1=1"}
	for key, column := range map[string]string{
		"level":              "level",
		"eventType":          "event_type",
		"entityType":         "entity_type",
		"entityId":           "entity_id",
		"projectId":          "project_id",
		"repositoryTargetId": "repository_target_id",
		"workItemId":         "work_item_id",
		"pipelineId":         "pipeline_id",
		"attemptId":          "attempt_id",
		"stageId":            "stage_id",
		"agentId":            "agent_id",
		"requestId":          "request_id",
	} {
		value := strings.TrimSpace(filters[key])
		if value != "" {
			where = append(where, fmt.Sprintf("%s = %s", column, sqlQuote(value)))
		}
	}
	if requirementID := strings.TrimSpace(filters["requirementId"]); requirementID != "" {
		if scope := repo.runtimeLogRequirementScope(ctx, requirementID); scope != "" {
			where = append(where, scope)
		}
	}
	if value := strings.TrimSpace(filters["createdAfter"]); value != "" {
		where = append(where, fmt.Sprintf("created_at >= %s", sqlQuote(value)))
	}
	if value := strings.TrimSpace(filters["createdBefore"]); value != "" {
		where = append(where, fmt.Sprintf("created_at <= %s", sqlQuote(value)))
	}
	if value := strings.TrimSpace(filters["query"]); value != "" {
		like := sqlQuote("%" + strings.ToLower(value) + "%")
		where = append(where, fmt.Sprintf("(LOWER(event_type) LIKE %s OR LOWER(message) LIKE %s OR LOWER(details_json) LIKE %s OR LOWER(level) LIKE %s)", like, like, like, like))
	}
	if cursor := decodeRuntimeLogCursor(filters["cursor"]); cursor.CreatedAt != "" && cursor.ID != "" {
		where = append(where, fmt.Sprintf("(created_at < %s OR (created_at = %s AND id < %s))", sqlQuote(cursor.CreatedAt), sqlQuote(cursor.CreatedAt), sqlQuote(cursor.ID)))
	}
	queryLimit := limit + 1
	output, err := repo.query(ctx, fmt.Sprintf(`.mode json
SELECT
  id,
  level,
  event_type AS eventType,
  message,
  entity_type AS entityType,
  entity_id AS entityId,
  project_id AS projectId,
  repository_target_id AS repositoryTargetId,
  requirement_id AS requirementId,
  work_item_id AS workItemId,
  pipeline_id AS pipelineId,
  attempt_id AS attemptId,
  stage_id AS stageId,
  agent_id AS agentId,
  request_id AS requestId,
  details_json AS detailsJson,
  created_at AS createdAt
FROM runtime_logs
WHERE %s
ORDER BY created_at DESC, id DESC
LIMIT %d;
`, strings.Join(where, " AND "), queryLimit))
	if err != nil {
		return RuntimeLogPage{}, err
	}
	if strings.TrimSpace(output) == "" {
		return RuntimeLogPage{Items: []RuntimeLogRecord{}, Limit: limit, HasMore: false}, nil
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(output), &rows); err != nil {
		return RuntimeLogPage{}, err
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	logs := make([]RuntimeLogRecord, 0, len(rows))
	for _, row := range rows {
		record := RuntimeLogRecord{
			ID:                 text(row, "id"),
			Level:              text(row, "level"),
			EventType:          text(row, "eventType"),
			Message:            text(row, "message"),
			EntityType:         text(row, "entityType"),
			EntityID:           text(row, "entityId"),
			ProjectID:          text(row, "projectId"),
			RepositoryTargetID: text(row, "repositoryTargetId"),
			RequirementID:      text(row, "requirementId"),
			WorkItemID:         text(row, "workItemId"),
			PipelineID:         text(row, "pipelineId"),
			AttemptID:          text(row, "attemptId"),
			StageID:            text(row, "stageId"),
			AgentID:            text(row, "agentId"),
			RequestID:          text(row, "requestId"),
			CreatedAt:          text(row, "createdAt"),
			Details:            map[string]any{},
		}
		if raw := text(row, "detailsJson"); raw != "" {
			_ = json.Unmarshal([]byte(raw), &record.Details)
		}
		logs = append(logs, record)
	}
	nextCursor := ""
	if hasMore && len(logs) > 0 {
		last := logs[len(logs)-1]
		nextCursor = encodeRuntimeLogCursor(runtimeLogCursor{CreatedAt: last.CreatedAt, ID: last.ID})
	}
	return RuntimeLogPage{Items: logs, Limit: limit, NextCursor: nextCursor, HasMore: hasMore}, nil
}

type runtimeLogCursor struct {
	CreatedAt string `json:"createdAt"`
	ID        string `json:"id"`
}

func encodeRuntimeLogCursor(cursor runtimeLogCursor) string {
	raw, err := json.Marshal(cursor)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeRuntimeLogCursor(value string) runtimeLogCursor {
	if strings.TrimSpace(value) == "" {
		return runtimeLogCursor{}
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return runtimeLogCursor{}
	}
	var cursor runtimeLogCursor
	_ = json.Unmarshal(raw, &cursor)
	return cursor
}

func (repo *SQLiteRepository) runtimeLogRequirementScope(ctx context.Context, requirementID string) string {
	terms := []string{fmt.Sprintf("requirement_id = %s", sqlQuote(requirementID))}
	database, err := repo.LoadSupervisorExecutionState(ctx)
	if err == nil && database != nil {
		workItemIDs := map[string]bool{}
		pipelineIDs := map[string]bool{}
		attemptIDs := map[string]bool{}
		for _, item := range database.Tables.WorkItems {
			if text(item, "requirementId") == requirementID {
				workItemIDs[text(item, "id")] = true
			}
		}
		for _, pipeline := range database.Tables.Pipelines {
			if workItemIDs[text(pipeline, "workItemId")] {
				pipelineIDs[text(pipeline, "id")] = true
			}
		}
		for _, attempt := range database.Tables.Attempts {
			if workItemIDs[text(attempt, "itemId")] || pipelineIDs[text(attempt, "pipelineId")] {
				attemptIDs[text(attempt, "id")] = true
			}
		}
		if clause := sqlInClause("work_item_id", workItemIDs); clause != "" {
			terms = append(terms, clause)
		}
		if clause := sqlInClause("pipeline_id", pipelineIDs); clause != "" {
			terms = append(terms, clause)
		}
		if clause := sqlInClause("attempt_id", attemptIDs); clause != "" {
			terms = append(terms, clause)
		}
	}
	terms = append(terms, fmt.Sprintf("details_json LIKE %s", sqlQuote("%"+requirementID+"%")))
	return "(" + strings.Join(terms, " OR ") + ")"
}

func sqlInClause(column string, values map[string]bool) string {
	quoted := []string{}
	for value := range values {
		if strings.TrimSpace(value) != "" {
			quoted = append(quoted, sqlQuote(value))
		}
	}
	if len(quoted) == 0 {
		return ""
	}
	return fmt.Sprintf("%s IN (%s)", column, strings.Join(quoted, ","))
}

func deleteWhereSQL(table string, clauses ...string) string {
	filtered := []string{}
	for _, clause := range clauses {
		if strings.TrimSpace(clause) != "" {
			filtered = append(filtered, clause)
		}
	}
	if len(filtered) == 0 {
		return ""
	}
	return fmt.Sprintf("DELETE FROM %s WHERE %s;", table, strings.Join(filtered, " OR "))
}

func (repo *SQLiteRepository) exec(ctx context.Context, input string) error {
	_, err := repo.run(ctx, input)
	return err
}

func (repo *SQLiteRepository) query(ctx context.Context, input string) (string, error) {
	return repo.run(ctx, input)
}

func (repo *SQLiteRepository) run(ctx context.Context, input string) (string, error) {
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		cmd := exec.CommandContext(ctx, "sqlite3", repo.Path)
		cmd.Stdin = strings.NewReader(".timeout 5000\n" + input)
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			if stderr.Len() > 0 {
				lastErr = errors.New(strings.TrimSpace(stderr.String()))
			} else {
				lastErr = err
			}
			if isSQLiteBusy(lastErr) && ctx.Err() == nil {
				time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
				continue
			}
			return "", lastErr
		}
		return stdout.String(), nil
	}
	return "", lastErr
}

func isSQLiteBusy(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is locked") || strings.Contains(message, "database is busy")
}

func ensureTables(database *WorkspaceDatabase) {
	if database.SavedAt == "" {
		database.SavedAt = nowISO()
	}
	linked := normalizeRequirementLinks(*database)
	linked = normalizePipelineExecutionMetadata(linked)
	database.Tables.Requirements = linked.Tables.Requirements
	database.Tables.WorkItems = linked.Tables.WorkItems
	database.Tables.MissionControlStates = linked.Tables.MissionControlStates
	database.Tables.Pipelines = linked.Tables.Pipelines
	database.Tables.Checkpoints = dedupeCheckpointRecords(linked.Tables.Checkpoints)
	if database.Tables.Attempts == nil {
		database.Tables.Attempts = []map[string]any{}
	}
	if database.Tables.RunWorkpads == nil {
		database.Tables.RunWorkpads = []map[string]any{}
	}
	if database.Tables.WorkflowTemplates == nil {
		database.Tables.WorkflowTemplates = []map[string]any{}
	}
}

func dedupeCheckpointRecords(records []map[string]any) []map[string]any {
	if len(records) <= 1 {
		return records
	}
	positions := map[string]int{}
	output := []map[string]any{}
	for _, record := range records {
		id := text(record, "id")
		if id == "" {
			output = append(output, record)
			continue
		}
		if index, ok := positions[id]; ok {
			output[index] = preferredCheckpointRecord(output[index], record)
			continue
		}
		positions[id] = len(output)
		output = append(output, record)
	}
	return output
}

func preferredCheckpointRecord(left map[string]any, right map[string]any) map[string]any {
	leftRank := checkpointStatusRank(text(left, "status"))
	rightRank := checkpointStatusRank(text(right, "status"))
	if rightRank > leftRank {
		return right
	}
	if rightRank < leftRank {
		return left
	}
	if text(right, "updatedAt") > text(left, "updatedAt") {
		return right
	}
	return left
}

func checkpointStatusRank(status string) int {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "approved", "rejected", "done", "passed":
		return 3
	case "running", "needs-human", "waiting-human":
		return 2
	case "pending":
		return 1
	default:
		return 0
	}
}

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func sqlQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func nullableQ(value any) string {
	if value == nil {
		return "NULL"
	}
	if text, ok := value.(string); ok && text == "" {
		return "NULL"
	}
	return sqlQuote(fmt.Sprint(value))
}

func q(record map[string]any, key string) string {
	value := record[key]
	if value == nil {
		return sqlQuote("")
	}
	return sqlQuote(fmt.Sprint(value))
}

func jsonQ(value any) string {
	raw, _ := json.Marshal(value)
	if string(raw) == "null" {
		return sqlQuote("[]")
	}
	return sqlQuote(string(raw))
}

func checkpointFeishuReviewQ(checkpoint map[string]any) string {
	review := mapValue(checkpoint["feishuReview"])
	if len(review) == 0 {
		return sqlQuote("{}")
	}
	raw, _ := json.Marshal(review)
	return sqlQuote(string(raw))
}

func checkpointFeishuReviewStatus(checkpoint map[string]any) string {
	review := mapValue(checkpoint["feishuReview"])
	return firstNonEmpty(text(checkpoint, "feishuReviewStatus"), text(review, "status"))
}

func checkpointFeishuTaskGuid(checkpoint map[string]any) string {
	review := mapValue(checkpoint["feishuReview"])
	return firstNonEmpty(text(checkpoint, "feishuTaskGuid"), text(review, "taskGuid"))
}

func checkpointFeishuTaskID(checkpoint map[string]any) string {
	review := mapValue(checkpoint["feishuReview"])
	return firstNonEmpty(text(checkpoint, "feishuTaskId"), text(review, "taskId"))
}

func checkpointFeishuMessageID(checkpoint map[string]any) string {
	review := mapValue(checkpoint["feishuReview"])
	return firstNonEmpty(text(checkpoint, "feishuMessageId"), text(review, "messageId"))
}

func attemptFeishuFailureQ(attempt map[string]any) string {
	failure := mapValue(attempt["feishuFailure"])
	if len(failure) == 0 {
		return sqlQuote("{}")
	}
	raw, _ := json.Marshal(failure)
	return sqlQuote(string(raw))
}

func attemptFeishuFailureStatus(attempt map[string]any) string {
	failure := mapValue(attempt["feishuFailure"])
	return firstNonEmpty(text(attempt, "feishuFailureStatus"), text(failure, "status"))
}

func attemptFeishuFailureMessageID(attempt map[string]any) string {
	failure := mapValue(attempt["feishuFailure"])
	return firstNonEmpty(text(attempt, "feishuFailureMessageId"), text(failure, "messageId"))
}

func operationRecordJSONQ(operation map[string]any) string {
	record := cloneMap(operation)
	record["createdAt"] = operationTimestampValue(operation, "createdAt")
	record["updatedAt"] = operationTimestampValue(operation, "updatedAt")
	delete(record, "recordJson")
	return jsonQ(record)
}

func operationTimestampQ(operation map[string]any, key string) string {
	return sqlQuote(operationTimestampValue(operation, key))
}

func operationTimestampValue(operation map[string]any, key string) string {
	value := text(operation, key)
	if isReasonableTimestamp(value) {
		return value
	}
	return nowISO()
}

func isReasonableTimestamp(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || len(trimmed) > 128 {
		return false
	}
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return false
	}
	if _, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
		return true
	}
	return false
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func boolInt(value any) int {
	if value == true {
		return 1
	}
	return 0
}
