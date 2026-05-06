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
	states = missionControlStatesWithCanonicalWorkItems(states, workItems)
	workflowTemplates, err := repo.ListWorkflowTemplates(ctx, nil)
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
			WorkflowTemplates:    workflowTemplates,
			Connections:          connections,
			UIPreferences:        uiPreferences,
		},
	}
	ensureTables(database)
	return database, nil
}

func (repo *SQLiteRepository) LoadSupervisorExecutionState(ctx context.Context) (*WorkspaceDatabase, error) {
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
	states = missionControlStatesWithCanonicalWorkItems(states, workItems)
	workflowTemplates, err := repo.ListWorkflowTemplates(ctx, nil)
	if err != nil {
		return nil, err
	}
	pipelines, err := repo.ListPipelines(ctx, nil)
	if err != nil {
		return nil, err
	}
	attempts, err := repo.ListAttempts(ctx, nil)
	if err != nil {
		return nil, err
	}
	checkpoints, err := repo.ListCheckpoints(ctx, nil)
	if err != nil {
		return nil, err
	}
	runWorkpads, err := repo.ListRunWorkpads(ctx, nil)
	if err != nil {
		return nil, err
	}
	missions, err := repo.ListMissions(ctx, map[string]string{"limit": "500"})
	if err != nil {
		return nil, err
	}
	operations, err := repo.ListOperations(ctx, map[string]string{"limit": "500"})
	if err != nil {
		return nil, err
	}
	proofRecords, err := repo.ListProofRecords(ctx, map[string]string{"limit": "500"})
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(savedAt) == "" && len(workItems) == 0 && len(pipelines) == 0 && len(attempts) == 0 {
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
			WorkflowTemplates:    workflowTemplates,
			Pipelines:            pipelines,
			Attempts:             attempts,
			Checkpoints:          checkpoints,
			RunWorkpads:          runWorkpads,
			Missions:             missions,
			Operations:           operations,
			ProofRecords:         proofRecords,
		},
	}
	ensureTables(database)
	return database, nil
}

func (repo *SQLiteRepository) LoadJobSupervisorState(ctx context.Context) (*WorkspaceDatabase, error) {
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
	states = missionControlStatesWithCanonicalWorkItems(states, workItems)
	workflowTemplates, err := repo.ListWorkflowTemplates(ctx, nil)
	if err != nil {
		return nil, err
	}
	pipelines, err := repo.ListPipelines(ctx, nil)
	if err != nil {
		return nil, err
	}
	attempts, err := repo.ListJobSupervisorAttempts(ctx)
	if err != nil {
		return nil, err
	}
	checkpoints, err := repo.ListCheckpoints(ctx, nil)
	if err != nil {
		return nil, err
	}
	proofRecords, err := repo.ListProofRecords(ctx, map[string]string{"limit": "500"})
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(savedAt) == "" && len(workItems) == 0 && len(pipelines) == 0 && len(attempts) == 0 {
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
			WorkflowTemplates:    workflowTemplates,
			Pipelines:            pipelines,
			Attempts:             attempts,
			Checkpoints:          checkpoints,
			ProofRecords:         proofRecords,
		},
	}
	ensureTables(database)
	return database, nil
}

func (repo *SQLiteRepository) LoadRepositoryTargetDeleteState(ctx context.Context) (*WorkspaceDatabase, error) {
	if err := repo.Initialize(ctx); err != nil {
		return nil, err
	}
	session, err := repo.LoadWorkspaceSession(ctx)
	if err != nil {
		return nil, err
	}
	pipelines, err := repo.ListPipelines(ctx, nil)
	if err != nil {
		return nil, err
	}
	attempts, err := repo.ListAttempts(ctx, nil)
	if err != nil {
		return nil, err
	}
	checkpoints, err := repo.ListCheckpoints(ctx, nil)
	if err != nil {
		return nil, err
	}
	runWorkpads, err := repo.ListRunWorkpads(ctx, nil)
	if err != nil {
		return nil, err
	}
	missions, err := repo.ListMissions(ctx, nil)
	if err != nil {
		return nil, err
	}
	database := *session
	database.Tables.Pipelines = pipelines
	database.Tables.Attempts = attempts
	database.Tables.Checkpoints = checkpoints
	database.Tables.RunWorkpads = runWorkpads
	database.Tables.Missions = missions
	database.Tables.Operations = nil
	database.Tables.ProofRecords = nil
	ensureTables(&database)
	return &database, nil
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
		targets := targetsByProject[text(project, "id")]
		project["repositoryTargets"] = targets
		if project["repositoryTargets"] == nil {
			project["repositoryTargets"] = []any{}
		}
		if text(project, "defaultRepositoryTargetId") == "" && len(targets) > 0 {
			project["defaultRepositoryTargetId"] = text(mapValue(targets[0]), "id")
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

func missionControlStatesWithCanonicalWorkItems(states []map[string]any, workItems []map[string]any) []map[string]any {
	if len(states) == 0 {
		return states
	}
	canonicalItems := make([]any, 0, len(workItems))
	for _, item := range workItems {
		canonicalItems = append(canonicalItems, cloneMap(item))
	}
	next := make([]map[string]any, 0, len(states))
	for _, state := range states {
		record := cloneMap(state)
		record["workItems"] = canonicalItems
		next = append(next, record)
	}
	return next
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
	pipelines, err := decodeStructuredRows(output, map[string]string{"run": "runJson"})
	if err != nil {
		return nil, err
	}
	for _, pipeline := range pipelines {
		if text(pipeline, "templateId") != "" {
			continue
		}
		run := mapValue(pipeline["run"])
		templateID := firstNonEmpty(
			text(mapValue(run["workflow"]), "id"),
			text(mapValue(run["orchestrator"]), "templateId"),
		)
		if templateID != "" {
			pipeline["templateId"] = templateID
		}
	}
	return pipelines, nil
}

func (repo *SQLiteRepository) ListAttempts(ctx context.Context, filters map[string]string) ([]map[string]any, error) {
	if err := repo.Initialize(ctx); err != nil {
		return nil, err
	}
	where := sqlFilterClauses(filters, map[string]string{
		"id":                      "id",
		"itemId":                  "item_id",
		"workItemId":              "item_id",
		"pipelineId":              "pipeline_id",
		"repositoryTargetId":      "repository_target_id",
		"status":                  "status",
		"feishuFailureStatus":     "feishu_failure_status",
		"feishuFailureMessageId":  "feishu_failure_message_id",
		"feishuFailureNotifiedAt": "feishu_failure_notified_at",
	})
	if truthyFilter(filters["compact"]) {
		return repo.listCompactAttempts(ctx, where, filters)
	}
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
  feishu_failure_notified_at AS feishuFailureNotifiedAt,
  feishu_failure_status AS feishuFailureStatus,
  feishu_failure_message_id AS feishuFailureMessageId,
  feishu_failure_json AS feishuFailureJson,
  record_json AS recordJson,
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
	rows, err := decodeStructuredRows(output, map[string]string{"stages": "stagesJson", "events": "eventsJson", "feishuFailure": "feishuFailureJson"})
	if err != nil {
		return nil, err
	}
	attempts := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		var record map[string]any
		if raw := strings.TrimSpace(text(row, "recordJson")); raw != "" {
			_ = json.Unmarshal([]byte(raw), &record)
		}
		if record == nil {
			record = cloneMap(row)
		}
		for key, value := range row {
			if key == "recordJson" {
				continue
			}
			record[key] = value
		}
		delete(record, "recordJson")
		attempts = append(attempts, record)
	}
	return attempts, nil
}

func (repo *SQLiteRepository) ListJobSupervisorAttempts(ctx context.Context) ([]map[string]any, error) {
	statuses := []string{"running", "waiting-human", "stalled", "failed", "canceled"}
	attempts := []map[string]any{}
	seen := map[string]bool{}
	for _, status := range statuses {
		records, err := repo.ListAttempts(ctx, map[string]string{"status": status, "limit": "80"})
		if err != nil {
			return nil, err
		}
		for _, record := range records {
			id := text(record, "id")
			if id != "" && seen[id] {
				continue
			}
			if id != "" {
				seen[id] = true
			}
			attempts = append(attempts, record)
		}
	}
	return attempts, nil
}

func (repo *SQLiteRepository) listCompactAttempts(ctx context.Context, where []string, filters map[string]string) ([]map[string]any, error) {
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
  feishu_failure_notified_at AS feishuFailureNotifiedAt,
  feishu_failure_status AS feishuFailureStatus,
  feishu_failure_message_id AS feishuFailureMessageId,
  feishu_failure_json AS feishuFailureJson,
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
	return decodeStructuredRows(output, map[string]string{"feishuFailure": "feishuFailureJson"})
}

func (repo *SQLiteRepository) ListCheckpoints(ctx context.Context, filters map[string]string) ([]map[string]any, error) {
	if err := repo.Initialize(ctx); err != nil {
		return nil, err
	}
	where := sqlFilterClauses(filters, map[string]string{
		"id":                 "id",
		"pipelineId":         "pipeline_id",
		"attemptId":          "attempt_id",
		"stageId":            "stage_id",
		"status":             "status",
		"feishuReviewStatus": "feishu_review_status",
		"feishuTaskGuid":     "feishu_task_guid",
		"feishuTaskId":       "feishu_task_id",
	})
	output, err := repo.query(ctx, fmt.Sprintf(`.mode json
SELECT
  id,
  pipeline_id AS pipelineId,
  attempt_id AS attemptId,
  stage_id AS stageId,
  status,
  title,
  summary,
  decision_note AS decisionNote,
  feishu_review_status AS feishuReviewStatus,
  feishu_task_guid AS feishuTaskGuid,
  feishu_task_id AS feishuTaskId,
  feishu_message_id AS feishuMessageId,
  feishu_review_json AS feishuReviewJson,
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
	return decodeStructuredRows(output, map[string]string{"feishuReview": "feishuReviewJson"})
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
	if truthyFilter(filters["compact"]) {
		return repo.listCompactOperations(ctx, where, filters)
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
  record_json AS recordJson,
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
	rows, err := decodeStructuredRows(output, map[string]string{"requiredProof": "requiredProofJson"})
	if err != nil {
		return nil, err
	}
	operations := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		var record map[string]any
		if raw := strings.TrimSpace(text(row, "recordJson")); raw != "" {
			_ = json.Unmarshal([]byte(raw), &record)
		}
		if record == nil {
			record = cloneMap(row)
		}
		for key, value := range row {
			if key == "recordJson" {
				continue
			}
			record[key] = value
		}
		delete(record, "recordJson")
		operations = append(operations, record)
	}
	return operations, nil
}

func (repo *SQLiteRepository) listCompactOperations(ctx context.Context, where []string, filters map[string]string) ([]map[string]any, error) {
	output, err := repo.query(ctx, fmt.Sprintf(`.mode json
WITH selected_operations AS (
  SELECT
    id,
    mission_id,
    stage_id,
    agent_id,
    status,
    prompt,
    required_proof_json,
    CASE WHEN json_valid(record_json) THEN record_json ELSE '{}' END AS safe_record_json,
    created_at,
    updated_at
  FROM operations
  WHERE %s
  ORDER BY updated_at DESC, created_at DESC, id DESC
  %s
)
SELECT
  id,
  mission_id AS missionId,
  stage_id AS stageId,
  agent_id AS agentId,
  status,
  substr(prompt, 1, 512) AS prompt,
  COALESCE(
    json_extract(safe_record_json, '$.summary'),
    json_extract(safe_record_json, '$.runnerProcess.summary')
  ) AS summary,
  required_proof_json AS requiredProofJson,
  json_object(
    'runner', json_extract(safe_record_json, '$.runnerProcess.runner'),
    'provider', json_extract(safe_record_json, '$.runnerProcess.provider'),
    'model', json_extract(safe_record_json, '$.runnerProcess.model'),
    'effort', json_extract(safe_record_json, '$.runnerProcess.effort'),
    'status', COALESCE(json_extract(safe_record_json, '$.runnerProcess.status'), status),
    'exitCode', json_extract(safe_record_json, '$.runnerProcess.exitCode'),
    'durationMs', json_extract(safe_record_json, '$.runnerProcess.durationMs'),
    'startedAt', json_extract(safe_record_json, '$.runnerProcess.startedAt'),
    'finishedAt', json_extract(safe_record_json, '$.runnerProcess.finishedAt'),
    'promptTokens', COALESCE(
      json_extract(safe_record_json, '$.runnerProcess.promptTokens'),
      json_extract(safe_record_json, '$.runnerProcess.inputTokens'),
      json_extract(safe_record_json, '$.runnerProcess.usage.promptTokens'),
      json_extract(safe_record_json, '$.runnerProcess.usage.inputTokens')
    ),
    'completionTokens', COALESCE(
      json_extract(safe_record_json, '$.runnerProcess.completionTokens'),
      json_extract(safe_record_json, '$.runnerProcess.outputTokens'),
      json_extract(safe_record_json, '$.runnerProcess.usage.completionTokens'),
      json_extract(safe_record_json, '$.runnerProcess.usage.outputTokens')
    ),
    'totalTokens', COALESCE(
      json_extract(safe_record_json, '$.runnerProcess.totalTokens'),
      json_extract(safe_record_json, '$.runnerProcess.usage.totalTokens')
    )
  ) AS runnerProcessJson,
  created_at AS createdAt,
  updated_at AS updatedAt
FROM selected_operations;
`, strings.Join(where, " AND "), sqlLimitClause(filters)))
	if err != nil {
		return nil, err
	}
	return decodeStructuredRows(output, map[string]string{
		"requiredProof": "requiredProofJson",
		"runnerProcess": "runnerProcessJson",
	})
}

func truthyFilter(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (repo *SQLiteRepository) ListMissions(ctx context.Context, filters map[string]string) ([]map[string]any, error) {
	if err := repo.Initialize(ctx); err != nil {
		return nil, err
	}
	where := sqlFilterClauses(filters, map[string]string{
		"id":         "id",
		"pipelineId": "pipeline_id",
		"workItemId": "work_item_id",
		"status":     "status",
	})
	output, err := repo.query(ctx, fmt.Sprintf(`.mode json
SELECT
  id,
  pipeline_id AS pipelineId,
  work_item_id AS workItemId,
  title,
  status,
  mission_json AS missionJson,
  created_at AS createdAt,
  updated_at AS updatedAt
FROM missions
WHERE %s
ORDER BY updated_at DESC, created_at DESC, id DESC
%s;
`, strings.Join(where, " AND "), sqlLimitClause(filters)))
	if err != nil {
		return nil, err
	}
	return decodeStructuredRows(output, map[string]string{"mission": "missionJson"})
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

func (repo *SQLiteRepository) ListWorkflowTemplates(ctx context.Context, filters map[string]string) ([]map[string]any, error) {
	if err := repo.Initialize(ctx); err != nil {
		return nil, err
	}
	where := sqlFilterClauses(filters, map[string]string{
		"id":                 "id",
		"projectId":          "project_id",
		"repositoryTargetId": "repository_target_id",
		"templateId":         "template_id",
		"scope":              "scope",
	})
	output, err := repo.query(ctx, fmt.Sprintf(`.mode json
SELECT
  id,
  scope,
  project_id AS projectId,
  repository_target_id AS repositoryTargetId,
  template_id AS templateId,
  source,
  version,
  markdown,
  validation_json AS validationJson,
  created_at AS createdAt,
  updated_at AS updatedAt
FROM workflow_templates
WHERE %s
ORDER BY updated_at DESC, id DESC;
`, strings.Join(where, " AND ")))
	if err != nil {
		return nil, err
	}
	records, err := decodeStructuredRows(output, map[string]string{"validation": "validationJson"})
	if err != nil {
		return nil, err
	}
	for _, record := range records {
		record["workflowMarkdown"] = text(record, "markdown")
	}
	return records, nil
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
	if truthyFilter(filters["compact"]) {
		output, err := repo.query(ctx, fmt.Sprintf(`.mode json
SELECT
  id,
  attempt_id AS attemptId,
  pipeline_id AS pipelineId,
  work_item_id AS workItemId,
  repository_target_id AS repositoryTargetId,
  status,
  '{}' AS workpadJson,
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
	output, err := repo.query(ctx, fmt.Sprintf(`.mode json
SELECT
  id,
  attempt_id AS attemptId,
  pipeline_id AS pipelineId,
  work_item_id AS workItemId,
  repository_target_id AS repositoryTargetId,
  status,
  workpad_json AS workpadJson,
  record_json AS recordJson,
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
	rows, err := decodeStructuredRows(output, map[string]string{"workpad": "workpadJson"})
	if err != nil {
		return nil, err
	}
	workpads := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		var record map[string]any
		if raw := strings.TrimSpace(text(row, "recordJson")); raw != "" {
			_ = json.Unmarshal([]byte(raw), &record)
		}
		if record == nil {
			record = cloneMap(row)
		}
		for key, value := range row {
			if key == "recordJson" {
				continue
			}
			record[key] = value
		}
		delete(record, "recordJson")
		workpads = append(workpads, record)
	}
	return workpads, nil
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
