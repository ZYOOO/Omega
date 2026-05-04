package omegalocal

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func (repo *SQLiteRepository) LoadWorkspaceSession(ctx context.Context) (*WorkspaceDatabase, error) {
	if err := repo.Initialize(ctx); err != nil {
		return nil, err
	}
	savedAt, err := repo.WorkspaceSavedAt(ctx)
	if err != nil {
		return nil, err
	}
	projects, err := repo.listSessionProjects(ctx)
	if err != nil {
		return nil, err
	}
	requirements, err := repo.listSessionRequirements(ctx)
	if err != nil {
		return nil, err
	}
	workItems, err := repo.listSessionWorkItems(ctx)
	if err != nil {
		return nil, err
	}
	states, err := repo.listSessionMissionControlStates(ctx)
	if err != nil {
		return nil, err
	}
	connections, err := repo.listSessionConnections(ctx)
	if err != nil {
		return nil, err
	}
	uiPreferences, err := repo.listSessionUIPreferences(ctx)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(savedAt) == "" && len(projects) == 0 && len(workItems) == 0 {
		return nil, sql.ErrNoRows
	}
	database := &WorkspaceDatabase{
		SchemaVersion: 1,
		SavedAt:       savedAt,
		Tables: WorkspaceTables{
			Projects:             projects,
			Requirements:         requirements,
			WorkItems:            workItems,
			MissionControlStates: states,
			Connections:          connections,
			UIPreferences:        uiPreferences,
		},
	}
	ensureTables(database)
	return database, nil
}

func (repo *SQLiteRepository) listSessionProjects(ctx context.Context) ([]map[string]any, error) {
	output, err := repo.query(ctx, `.mode json
SELECT
  id,
  name,
  description,
  team,
  status,
  labels_json AS labelsJson,
  created_at AS createdAt,
  updated_at AS updatedAt
FROM projects
ORDER BY created_at ASC, id ASC;
`)
	if err != nil {
		return nil, err
	}
	projects, err := decodeStructuredRows(output, map[string]string{"labels": "labelsJson"})
	if err != nil {
		return nil, err
	}
	targetOutput, err := repo.query(ctx, `.mode json
SELECT project_id AS projectId, target_json AS targetJson
FROM repository_targets
ORDER BY project_id ASC, label ASC;
`)
	if err != nil {
		return nil, err
	}
	targetRows, err := decodeStructuredRows(targetOutput, nil)
	if err != nil {
		return nil, err
	}
	targetsByProject := map[string][]any{}
	for _, row := range targetRows {
		var target map[string]any
		if err := json.Unmarshal([]byte(text(row, "targetJson")), &target); err != nil {
			return nil, err
		}
		if target != nil {
			targetsByProject[text(row, "projectId")] = append(targetsByProject[text(row, "projectId")], target)
		}
	}
	for _, project := range projects {
		project["repositoryTargets"] = targetsByProject[text(project, "id")]
		if project["repositoryTargets"] == nil {
			project["repositoryTargets"] = []any{}
		}
	}
	return projects, nil
}

func (repo *SQLiteRepository) listSessionRequirements(ctx context.Context) ([]map[string]any, error) {
	output, err := repo.query(ctx, `.mode json
SELECT
  id,
  project_id AS projectId,
  repository_target_id AS repositoryTargetId,
  source,
  source_external_ref AS sourceExternalRef,
  title,
  raw_text AS rawText,
  structured_json AS structuredJson,
  acceptance_criteria_json AS acceptanceCriteriaJson,
  risks_json AS risksJson,
  status,
  created_at AS createdAt,
  updated_at AS updatedAt
FROM requirements
ORDER BY updated_at DESC, created_at DESC, id DESC;
`)
	if err != nil {
		return nil, err
	}
	return decodeStructuredRows(output, map[string]string{
		"structured":         "structuredJson",
		"acceptanceCriteria": "acceptanceCriteriaJson",
		"risks":              "risksJson",
	})
}

func (repo *SQLiteRepository) listSessionWorkItems(ctx context.Context) ([]map[string]any, error) {
	output, err := repo.query(ctx, `.mode json
SELECT
  id,
  project_id AS projectId,
  key,
  title,
  description,
  status,
  priority,
  assignee,
  labels_json AS labelsJson,
  team,
  stage_id AS stageId,
  target,
  created_at AS createdAt,
  updated_at AS updatedAt,
  record_json AS recordJson
FROM work_items
ORDER BY updated_at DESC, created_at DESC, id DESC;
`)
	if err != nil {
		return nil, err
	}
	rows, err := decodeStructuredRows(output, map[string]string{"labels": "labelsJson"})
	if err != nil {
		return nil, err
	}
	workItems := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		var record map[string]any
		if raw := strings.TrimSpace(text(row, "recordJson")); raw != "" {
			_ = json.Unmarshal([]byte(raw), &record)
		}
		if record == nil {
			record = cloneMap(row)
		}
		delete(record, "recordJson")
		workItems = append(workItems, record)
	}
	return workItems, nil
}

func (repo *SQLiteRepository) listSessionMissionControlStates(ctx context.Context) ([]map[string]any, error) {
	output, err := repo.query(ctx, `.mode json
SELECT
  run_id AS runId,
  project_id AS projectId,
  work_items_json AS workItemsJson,
  events_json AS eventsJson,
  sync_intents_json AS syncIntentsJson,
  updated_at AS updatedAt
FROM mission_control_states
ORDER BY updated_at DESC, run_id DESC;
`)
	if err != nil {
		return nil, err
	}
	return decodeStructuredRows(output, map[string]string{
		"workItems":   "workItemsJson",
		"events":      "eventsJson",
		"syncIntents": "syncIntentsJson",
	})
}

func (repo *SQLiteRepository) listSessionConnections(ctx context.Context) ([]map[string]any, error) {
	output, err := repo.query(ctx, `.mode json
SELECT
  provider_id AS providerId,
  status,
  granted_permissions_json AS grantedPermissionsJson,
  connected_as AS connectedAs,
  updated_at AS updatedAt
FROM connections
ORDER BY provider_id ASC;
`)
	if err != nil {
		return nil, err
	}
	return decodeStructuredRows(output, map[string]string{"grantedPermissions": "grantedPermissionsJson"})
}

func (repo *SQLiteRepository) listSessionUIPreferences(ctx context.Context) ([]map[string]any, error) {
	output, err := repo.query(ctx, `.mode json
SELECT
  id,
  active_nav AS activeNav,
  selected_provider_id AS selectedProviderId,
  selected_work_item_id AS selectedWorkItemId,
  inspector_open AS inspectorOpen,
  active_inspector_panel AS activeInspectorPanel,
  runner_preset AS runnerPreset,
  status_filter AS statusFilter,
  assignee_filter AS assigneeFilter,
  sort_direction AS sortDirection,
  collapsed_groups_json AS collapsedGroupsJson
FROM ui_preferences
ORDER BY id ASC;
`)
	if err != nil {
		return nil, err
	}
	rows, err := decodeStructuredRows(output, map[string]string{"collapsedGroups": "collapsedGroupsJson"})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		row["inspectorOpen"] = boolValue(row["inspectorOpen"])
	}
	return rows, nil
}

func (repo *SQLiteRepository) ListPipelines(ctx context.Context, filters map[string]string) ([]map[string]any, error) {
	if err := repo.Initialize(ctx); err != nil {
		return nil, err
	}
	where := sqlFilterClauses(filters, map[string]string{
		"id":         "id",
		"workItemId": "work_item_id",
		"runId":      "run_id",
		"status":     "status",
	})
	output, err := repo.query(ctx, fmt.Sprintf(`.mode json
SELECT id, work_item_id AS workItemId, run_id AS runId, status, run_json AS runJson, created_at AS createdAt, updated_at AS updatedAt
FROM pipelines
WHERE %s
ORDER BY updated_at DESC, created_at DESC, id DESC
%s;
`, strings.Join(where, " AND "), sqlLimitClause(filters)))
	if err != nil {
		return nil, err
	}
	return decodeStructuredRows(output, map[string]string{"run": "runJson"})
}

func (repo *SQLiteRepository) ListAttempts(ctx context.Context, filters map[string]string) ([]map[string]any, error) {
	if err := repo.Initialize(ctx); err != nil {
		return nil, err
	}
	where := sqlFilterClauses(filters, map[string]string{
		"id":                 "id",
		"itemId":             "item_id",
		"workItemId":         "item_id",
		"pipelineId":         "pipeline_id",
		"repositoryTargetId": "repository_target_id",
		"status":             "status",
	})
	output, err := repo.query(ctx, fmt.Sprintf(`.mode json
SELECT
  id,
  item_id AS itemId,
  pipeline_id AS pipelineId,
  repository_target_id AS repositoryTargetId,
  status,
  trigger,
  runner,
  current_stage_id AS currentStageId,
  workspace_path AS workspacePath,
  branch_name AS branchName,
  pull_request_url AS pullRequestUrl,
  started_at AS startedAt,
  finished_at AS finishedAt,
  duration_ms AS durationMs,
  error_message AS errorMessage,
  stdout_summary AS stdoutSummary,
  stderr_summary AS stderrSummary,
  stages_json AS stagesJson,
  events_json AS eventsJson,
  created_at AS createdAt,
  updated_at AS updatedAt
FROM attempts
WHERE %s
ORDER BY started_at DESC, created_at DESC, id DESC
%s;
`, strings.Join(where, " AND "), sqlLimitClause(filters)))
	if err != nil {
		return nil, err
	}
	return decodeStructuredRows(output, map[string]string{"stages": "stagesJson", "events": "eventsJson"})
}

func (repo *SQLiteRepository) ListCheckpoints(ctx context.Context, filters map[string]string) ([]map[string]any, error) {
	if err := repo.Initialize(ctx); err != nil {
		return nil, err
	}
	where := sqlFilterClauses(filters, map[string]string{
		"id":         "id",
		"pipelineId": "pipeline_id",
		"stageId":    "stage_id",
		"status":     "status",
	})
	output, err := repo.query(ctx, fmt.Sprintf(`.mode json
SELECT
  id,
  pipeline_id AS pipelineId,
  stage_id AS stageId,
  status,
  title,
  summary,
  decision_note AS decisionNote,
  created_at AS createdAt,
  updated_at AS updatedAt
FROM checkpoints
WHERE %s
ORDER BY updated_at DESC, created_at DESC, id DESC
%s;
`, strings.Join(where, " AND "), sqlLimitClause(filters)))
	if err != nil {
		return nil, err
	}
	return decodeStructuredRows(output, nil)
}

func (repo *SQLiteRepository) ListOperations(ctx context.Context, filters map[string]string) ([]map[string]any, error) {
	if err := repo.Initialize(ctx); err != nil {
		return nil, err
	}
	where := sqlFilterClauses(filters, map[string]string{
		"id":        "id",
		"missionId": "mission_id",
		"stageId":   "stage_id",
		"agentId":   "agent_id",
		"status":    "status",
	})
	if pipelineID := strings.TrimSpace(filters["pipelineId"]); pipelineID != "" {
		pattern := sqlLikePattern(pipelineID)
		where = append(where, fmt.Sprintf("(id LIKE %s OR mission_id LIKE %s)", pattern, pattern))
	}
	if workItemID := strings.TrimSpace(filters["workItemId"]); workItemID != "" {
		pattern := sqlLikePattern(workItemID)
		where = append(where, fmt.Sprintf("(id LIKE %s OR mission_id LIKE %s OR prompt LIKE %s)", pattern, pattern, pattern))
	}
	output, err := repo.query(ctx, fmt.Sprintf(`.mode json
SELECT
  id,
  mission_id AS missionId,
  stage_id AS stageId,
  agent_id AS agentId,
  status,
  prompt,
  required_proof_json AS requiredProofJson,
  created_at AS createdAt,
  updated_at AS updatedAt
FROM operations
WHERE %s
ORDER BY updated_at DESC, created_at DESC, id DESC
%s;
`, strings.Join(where, " AND "), sqlLimitClause(filters)))
	if err != nil {
		return nil, err
	}
	return decodeStructuredRows(output, map[string]string{"requiredProof": "requiredProofJson"})
}

func (repo *SQLiteRepository) ListProofRecords(ctx context.Context, filters map[string]string) ([]map[string]any, error) {
	if err := repo.Initialize(ctx); err != nil {
		return nil, err
	}
	where := sqlFilterClauses(filters, map[string]string{
		"id":          "id",
		"operationId": "operation_id",
		"label":       "label",
	})
	if pipelineID := strings.TrimSpace(filters["pipelineId"]); pipelineID != "" {
		where = append(where, fmt.Sprintf("operation_id LIKE %s", sqlLikePattern(pipelineID)))
	}
	if workItemID := strings.TrimSpace(filters["workItemId"]); workItemID != "" {
		where = append(where, fmt.Sprintf("operation_id LIKE %s", sqlLikePattern(workItemID)))
	}
	output, err := repo.query(ctx, fmt.Sprintf(`.mode json
SELECT
  id,
  operation_id AS operationId,
  label,
  value,
  source_path AS sourcePath,
  created_at AS createdAt
FROM proof_records
WHERE %s
ORDER BY created_at DESC, id DESC
%s;
`, strings.Join(where, " AND "), sqlLimitClause(filters)))
	if err != nil {
		return nil, err
	}
	return decodeStructuredRows(output, nil)
}

func (repo *SQLiteRepository) ListRunWorkpads(ctx context.Context, filters map[string]string) ([]map[string]any, error) {
	if err := repo.Initialize(ctx); err != nil {
		return nil, err
	}
	where := sqlFilterClauses(filters, map[string]string{
		"id":                 "id",
		"attemptId":          "attempt_id",
		"pipelineId":         "pipeline_id",
		"workItemId":         "work_item_id",
		"repositoryTargetId": "repository_target_id",
		"status":             "status",
	})
	output, err := repo.query(ctx, fmt.Sprintf(`.mode json
SELECT
  id,
  attempt_id AS attemptId,
  pipeline_id AS pipelineId,
  work_item_id AS workItemId,
  repository_target_id AS repositoryTargetId,
  status,
  workpad_json AS workpadJson,
  created_at AS createdAt,
  updated_at AS updatedAt
FROM run_workpads
WHERE %s
ORDER BY updated_at DESC, created_at DESC, id DESC
%s;
`, strings.Join(where, " AND "), sqlLimitClause(filters)))
	if err != nil {
		return nil, err
	}
	return decodeStructuredRows(output, map[string]string{"workpad": "workpadJson"})
}

func tableListFilters(query map[string][]string) map[string]string {
	filters := map[string]string{}
	for key, values := range query {
		if len(values) == 0 {
			continue
		}
		value := strings.TrimSpace(values[0])
		if value == "" {
			continue
		}
		filters[key] = value
	}
	return filters
}

func sqlLimitClause(filters map[string]string) string {
	limit, err := strconv.Atoi(strings.TrimSpace(filters["limit"]))
	if err != nil || limit <= 0 {
		return ""
	}
	if limit > 1000 {
		limit = 1000
	}
	return fmt.Sprintf("LIMIT %d", limit)
}

func sqlLikePattern(value string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
	return sqlQuote("%"+escaped+"%") + " ESCAPE '\\'"
}
