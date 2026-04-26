package omegalocal

import (
	"bytes"
	"context"
	"database/sql"
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

var sqliteMigrations = []struct {
	Version string
	Name    string
}{
	{Version: "20260424_001", Name: "bootstrap_go_local_service_schema"},
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
CREATE TABLE IF NOT EXISTS requirements (id TEXT PRIMARY KEY, project_id TEXT NOT NULL, repository_target_id TEXT, source TEXT NOT NULL, source_external_ref TEXT, title TEXT NOT NULL, raw_text TEXT NOT NULL, structured_json TEXT NOT NULL, acceptance_criteria_json TEXT NOT NULL, risks_json TEXT NOT NULL, status TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS work_items (id TEXT PRIMARY KEY, project_id TEXT NOT NULL, key TEXT NOT NULL, title TEXT NOT NULL, description TEXT NOT NULL, status TEXT NOT NULL, priority TEXT NOT NULL, assignee TEXT NOT NULL, labels_json TEXT NOT NULL, team TEXT NOT NULL, stage_id TEXT NOT NULL, target TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS mission_control_states (run_id TEXT PRIMARY KEY, project_id TEXT NOT NULL, work_items_json TEXT NOT NULL, events_json TEXT NOT NULL, sync_intents_json TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS mission_events (id TEXT PRIMARY KEY, run_id TEXT NOT NULL, sequence INTEGER NOT NULL, event_json TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS sync_intents (id TEXT PRIMARY KEY, run_id TEXT NOT NULL, sequence INTEGER NOT NULL, intent_json TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS connections (provider_id TEXT PRIMARY KEY, status TEXT NOT NULL, granted_permissions_json TEXT NOT NULL, connected_as TEXT, updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS ui_preferences (id TEXT PRIMARY KEY, active_nav TEXT NOT NULL, selected_provider_id TEXT NOT NULL, selected_work_item_id TEXT NOT NULL, inspector_open INTEGER NOT NULL, active_inspector_panel TEXT NOT NULL, runner_preset TEXT NOT NULL, status_filter TEXT NOT NULL, assignee_filter TEXT NOT NULL, sort_direction TEXT NOT NULL, collapsed_groups_json TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS pipelines (id TEXT PRIMARY KEY, work_item_id TEXT NOT NULL, run_id TEXT NOT NULL, status TEXT NOT NULL, run_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS attempts (id TEXT PRIMARY KEY, item_id TEXT NOT NULL, pipeline_id TEXT NOT NULL, repository_target_id TEXT, status TEXT NOT NULL, trigger TEXT NOT NULL, runner TEXT NOT NULL, current_stage_id TEXT, workspace_path TEXT, branch_name TEXT, pull_request_url TEXT, started_at TEXT NOT NULL, finished_at TEXT, duration_ms INTEGER, error_message TEXT, stdout_summary TEXT, stderr_summary TEXT, stages_json TEXT NOT NULL, events_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS checkpoints (id TEXT PRIMARY KEY, pipeline_id TEXT NOT NULL, stage_id TEXT NOT NULL, status TEXT NOT NULL, title TEXT NOT NULL, summary TEXT NOT NULL, decision_note TEXT, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS missions (id TEXT PRIMARY KEY, pipeline_id TEXT NOT NULL, work_item_id TEXT NOT NULL, title TEXT NOT NULL, status TEXT NOT NULL, mission_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS operations (id TEXT PRIMARY KEY, mission_id TEXT NOT NULL, stage_id TEXT NOT NULL, agent_id TEXT NOT NULL, status TEXT NOT NULL, prompt TEXT NOT NULL, required_proof_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS proof_records (id TEXT PRIMARY KEY, operation_id TEXT NOT NULL, label TEXT NOT NULL, value TEXT NOT NULL, source_path TEXT, created_at TEXT NOT NULL);
`); err != nil {
		return err
	}

	for _, migration := range sqliteMigrations {
		if err := repo.exec(ctx, fmt.Sprintf(
			"INSERT OR IGNORE INTO omega_migrations (version, name, applied_at) VALUES (%s, %s, %s);",
			sqlQuote(migration.Version),
			sqlQuote(migration.Name),
			sqlQuote(nowISO()),
		)); err != nil {
			return err
		}
	}
	return nil
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
		"DELETE FROM projects;",
		"DELETE FROM connections;",
		"DELETE FROM ui_preferences;",
		"DELETE FROM checkpoints;",
		"DELETE FROM attempts;",
		"DELETE FROM pipelines;",
		"DELETE FROM proof_records;",
		"DELETE FROM operations;",
		"DELETE FROM missions;",
		fmt.Sprintf("INSERT INTO workspace_snapshots (id, database_json, saved_at) VALUES ('default', %s, %s);", sqlQuote(string(raw)), sqlQuote(database.SavedAt)),
	}

	for _, project := range database.Tables.Projects {
		sqlText = append(sqlText, fmt.Sprintf(
			"INSERT OR REPLACE INTO projects VALUES (%s,%s,%s,%s,%s,%s,%s,%s);",
			q(project, "id"), q(project, "name"), q(project, "description"), q(project, "team"), q(project, "status"),
			jsonQ(project["labels"]), q(project, "createdAt"), q(project, "updatedAt"),
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
			"INSERT OR REPLACE INTO work_items VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s);",
			q(item, "id"), q(item, "projectId"), q(item, "key"), q(item, "title"), q(item, "description"), q(item, "status"),
			q(item, "priority"), q(item, "assignee"), jsonQ(item["labels"]), q(item, "team"), q(item, "stageId"), q(item, "target"),
			q(item, "createdAt"), q(item, "updatedAt"),
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
		sqlText = append(sqlText, fmt.Sprintf("INSERT OR REPLACE INTO attempts VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%d,%s,%s,%s,%s,%s,%s,%s);",
			q(attempt, "id"), q(attempt, "itemId"), q(attempt, "pipelineId"), nullableQ(attempt["repositoryTargetId"]), q(attempt, "status"),
			q(attempt, "trigger"), q(attempt, "runner"), nullableQ(attempt["currentStageId"]), nullableQ(attempt["workspacePath"]),
			nullableQ(attempt["branchName"]), nullableQ(attempt["pullRequestUrl"]), q(attempt, "startedAt"), nullableQ(attempt["finishedAt"]),
			intValue(attempt["durationMs"]), nullableQ(attempt["errorMessage"]), nullableQ(attempt["stdoutSummary"]), nullableQ(attempt["stderrSummary"]),
			jsonQ(attempt["stages"]), jsonQ(attempt["events"]), q(attempt, "createdAt"), q(attempt, "updatedAt")))
	}
	for _, checkpoint := range database.Tables.Checkpoints {
		sqlText = append(sqlText, fmt.Sprintf("INSERT OR REPLACE INTO checkpoints VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s);",
			q(checkpoint, "id"), q(checkpoint, "pipelineId"), q(checkpoint, "stageId"), q(checkpoint, "status"), q(checkpoint, "title"), q(checkpoint, "summary"),
			nullableQ(checkpoint["decisionNote"]), q(checkpoint, "createdAt"), q(checkpoint, "updatedAt")))
	}
	for _, mission := range database.Tables.Missions {
		sqlText = append(sqlText, fmt.Sprintf("INSERT OR REPLACE INTO missions VALUES (%s,%s,%s,%s,%s,%s,%s,%s);",
			q(mission, "id"), q(mission, "pipelineId"), q(mission, "workItemId"), q(mission, "title"), q(mission, "status"),
			jsonQ(mission["mission"]), q(mission, "createdAt"), q(mission, "updatedAt")))
	}
	for _, operation := range database.Tables.Operations {
		sqlText = append(sqlText, fmt.Sprintf("INSERT OR REPLACE INTO operations VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s);",
			q(operation, "id"), q(operation, "missionId"), q(operation, "stageId"), q(operation, "agentId"), q(operation, "status"),
			q(operation, "prompt"), jsonQ(operation["requiredProof"]), q(operation, "createdAt"), q(operation, "updatedAt")))
	}
	for _, proof := range database.Tables.ProofRecords {
		sqlText = append(sqlText, fmt.Sprintf("INSERT OR REPLACE INTO proof_records VALUES (%s,%s,%s,%s,%s,%s);",
			q(proof, "id"), q(proof, "operationId"), q(proof, "label"), q(proof, "value"), nullableQ(proof["sourcePath"]), q(proof, "createdAt")))
	}
	sqlText = append(sqlText, "COMMIT;")
	return repo.exec(ctx, strings.Join(sqlText, "\n"))
}

func (repo *SQLiteRepository) Load(ctx context.Context) (*WorkspaceDatabase, error) {
	if err := repo.Initialize(ctx); err != nil {
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
	var database WorkspaceDatabase
	if err := json.Unmarshal([]byte(raw), &database); err != nil {
		return nil, err
	}
	ensureTables(&database)
	return &database, nil
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
	if database.Tables.Attempts == nil {
		database.Tables.Attempts = []map[string]any{}
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
