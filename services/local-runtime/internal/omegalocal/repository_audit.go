package omegalocal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func repositoryTargetRecordsFromProjects(projects []map[string]any) []map[string]any {
	records := []map[string]any{}
	for _, project := range projects {
		projectID := text(project, "id")
		for _, target := range arrayMaps(project["repositoryTargets"]) {
			targetID := text(target, "id")
			if projectID == "" || targetID == "" {
				continue
			}
			kind := firstNonEmpty(text(target, "kind"), "local")
			label := text(target, "path")
			if kind == "github" {
				label = firstNonEmpty(text(target, "label"), repositoryTargetLabel(target))
			}
			timestamp := firstNonEmpty(text(target, "updatedAt"), text(project, "updatedAt"), nowISO())
			records = append(records, map[string]any{
				"id":            targetID,
				"projectId":     projectID,
				"kind":          kind,
				"label":         firstNonEmpty(label, targetID),
				"owner":         text(target, "owner"),
				"repo":          text(target, "repo"),
				"path":          text(target, "path"),
				"url":           text(target, "url"),
				"defaultBranch": text(target, "defaultBranch"),
				"target":        target,
				"createdAt":     firstNonEmpty(text(target, "createdAt"), text(project, "createdAt"), timestamp),
				"updatedAt":     timestamp,
			})
		}
	}
	return records
}

func handoffBundleRecordsFromDatabase(database WorkspaceDatabase) []map[string]any {
	records := []map[string]any{}
	for _, proof := range database.Tables.ProofRecords {
		sourcePath := text(proof, "sourcePath")
		if !strings.EqualFold(filepath.Base(sourcePath), "handoff-bundle.json") && !strings.Contains(strings.ToLower(text(proof, "label")), "handoff") {
			continue
		}
		operation := findRecordByID(database.Tables.Operations, text(proof, "operationId"))
		mission := findRecordByID(database.Tables.Missions, text(operation, "missionId"))
		pipelineID := firstNonEmpty(text(mission, "pipelineId"), pipelineIDFromOperationID(text(proof, "operationId")))
		attempt := latestAttemptForPipeline(database, pipelineID)
		bundle := readJSONMapIfPresent(sourcePath)
		if len(bundle) == 0 {
			bundle = map[string]any{
				"sourcePath": sourcePath,
				"label":      text(proof, "label"),
				"value":      text(proof, "value"),
			}
		}
		timestamp := firstNonEmpty(text(proof, "createdAt"), text(attempt, "updatedAt"), nowISO())
		records = append(records, map[string]any{
			"id":                 firstNonEmpty(text(proof, "id"), text(proof, "operationId")+":handoff"),
			"attemptId":          text(attempt, "id"),
			"pipelineId":         pipelineID,
			"workItemId":         firstNonEmpty(text(mission, "workItemId"), text(attempt, "itemId")),
			"repositoryTargetId": text(attempt, "repositoryTargetId"),
			"sourcePath":         sourcePath,
			"bundle":             bundle,
			"summary": map[string]any{
				"label":        text(proof, "label"),
				"value":        text(proof, "value"),
				"sourcePath":   sourcePath,
				"changedFiles": arrayValues(bundle["changedFiles"]),
				"pullRequest":  firstNonEmpty(text(bundle, "pullRequestUrl"), text(attempt, "pullRequestUrl")),
			},
			"createdAt": timestamp,
			"updatedAt": timestamp,
		})
	}
	return records
}

func operationQueueRecordsFromDatabase(database WorkspaceDatabase) []map[string]any {
	records := []map[string]any{}
	for _, operation := range database.Tables.Operations {
		operationID := text(operation, "id")
		if operationID == "" {
			continue
		}
		mission := findRecordByID(database.Tables.Missions, text(operation, "missionId"))
		pipelineID := text(mission, "pipelineId")
		attempt := latestAttemptForPipeline(database, pipelineID)
		status := normalizeOperationQueueStatus(text(operation, "status"))
		priority := operationQueuePriority(status)
		timestamp := firstNonEmpty(text(operation, "updatedAt"), text(operation, "createdAt"), nowISO())
		records = append(records, map[string]any{
			"id":                 "queue:" + operationID,
			"operationId":        operationID,
			"pipelineId":         pipelineID,
			"attemptId":          text(attempt, "id"),
			"workItemId":         firstNonEmpty(text(mission, "workItemId"), text(attempt, "itemId")),
			"repositoryTargetId": text(attempt, "repositoryTargetId"),
			"stageId":            text(operation, "stageId"),
			"agentId":            text(operation, "agentId"),
			"status":             status,
			"priority":           priority,
			"notBefore":          text(operation, "notBefore"),
			"lockedBy":           text(operation, "lockedBy"),
			"lockExpiresAt":      text(operation, "lockExpiresAt"),
			"attemptCount":       intValue(operation["attemptCount"]),
			"queue": map[string]any{
				"prompt":        text(operation, "prompt"),
				"requiredProof": operation["requiredProof"],
				"sourceStatus":  text(operation, "status"),
			},
			"createdAt": firstNonEmpty(text(operation, "createdAt"), timestamp),
			"updatedAt": timestamp,
		})
	}
	return records
}

func (server *Server) listRepositoryTargets(response http.ResponseWriter, request *http.Request) {
	records, err := server.Repo.ListRepositoryTargets(request.Context(), request.URL.Query().Get("projectId"))
	if errorsIsNoRows(err) {
		writeJSON(response, http.StatusOK, []map[string]any{})
		return
	}
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, records)
}

func (server *Server) listHandoffBundles(response http.ResponseWriter, request *http.Request) {
	records, err := server.Repo.ListHandoffBundles(request.Context(), map[string]string{
		"attemptId":          request.URL.Query().Get("attemptId"),
		"pipelineId":         request.URL.Query().Get("pipelineId"),
		"workItemId":         request.URL.Query().Get("workItemId"),
		"repositoryTargetId": request.URL.Query().Get("repositoryTargetId"),
	})
	if errorsIsNoRows(err) {
		writeJSON(response, http.StatusOK, []map[string]any{})
		return
	}
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, records)
}

func (server *Server) listOperationQueue(response http.ResponseWriter, request *http.Request) {
	records, err := server.Repo.ListOperationQueue(request.Context(), map[string]string{
		"status":             request.URL.Query().Get("status"),
		"pipelineId":         request.URL.Query().Get("pipelineId"),
		"attemptId":          request.URL.Query().Get("attemptId"),
		"workItemId":         request.URL.Query().Get("workItemId"),
		"repositoryTargetId": request.URL.Query().Get("repositoryTargetId"),
	})
	if errorsIsNoRows(err) {
		writeJSON(response, http.StatusOK, []map[string]any{})
		return
	}
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, records)
}

func (server *Server) proofRecordPreview(response http.ResponseWriter, request *http.Request) {
	proofID := strings.TrimSuffix(pathID(request.URL.Path), "/preview")
	database, err := mustLoad(server, request.Context())
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	proof := findRecordByID(database.Tables.ProofRecords, proofID)
	if len(proof) == 0 {
		writeJSON(response, http.StatusNotFound, map[string]any{"error": "proof record not found"})
		return
	}
	sourcePath := text(proof, "sourcePath")
	if strings.TrimSpace(sourcePath) == "" {
		writeJSON(response, http.StatusOK, map[string]any{"proof": proof, "content": "", "available": false})
		return
	}
	preview, err := previewLocalTextFile(sourcePath, 128*1024)
	if err != nil {
		writeJSON(response, http.StatusOK, map[string]any{"proof": proof, "sourcePath": sourcePath, "available": false, "error": err.Error()})
		return
	}
	preview["proof"] = proof
	writeJSON(response, http.StatusOK, preview)
}

func (repo *SQLiteRepository) ListRepositoryTargets(ctx context.Context, projectID string) ([]map[string]any, error) {
	if err := repo.Initialize(ctx); err != nil {
		return nil, err
	}
	where := "1=1"
	if strings.TrimSpace(projectID) != "" {
		where = "project_id = " + sqlQuote(projectID)
	}
	output, err := repo.query(ctx, fmt.Sprintf(`.mode json
SELECT target_json AS targetJson FROM repository_targets WHERE %s ORDER BY project_id, label;
`, where))
	if err != nil {
		return nil, err
	}
	return decodeJSONRows(output, "targetJson")
}

func (repo *SQLiteRepository) ListHandoffBundles(ctx context.Context, filters map[string]string) ([]map[string]any, error) {
	if err := repo.Initialize(ctx); err != nil {
		return nil, err
	}
	where := sqlFilterClauses(filters, map[string]string{
		"attemptId":          "attempt_id",
		"pipelineId":         "pipeline_id",
		"workItemId":         "work_item_id",
		"repositoryTargetId": "repository_target_id",
	})
	output, err := repo.query(ctx, fmt.Sprintf(`.mode json
SELECT
  id,
  attempt_id AS attemptId,
  pipeline_id AS pipelineId,
  work_item_id AS workItemId,
  repository_target_id AS repositoryTargetId,
  source_path AS sourcePath,
  bundle_json AS bundleJson,
  summary_json AS summaryJson,
  created_at AS createdAt,
  updated_at AS updatedAt
FROM handoff_bundles
WHERE %s
ORDER BY updated_at DESC, id DESC;
`, strings.Join(where, " AND ")))
	if err != nil {
		return nil, err
	}
	return decodeStructuredRows(output, map[string]string{"bundle": "bundleJson", "summary": "summaryJson"})
}

func (repo *SQLiteRepository) ListOperationQueue(ctx context.Context, filters map[string]string) ([]map[string]any, error) {
	if err := repo.Initialize(ctx); err != nil {
		return nil, err
	}
	where := sqlFilterClauses(filters, map[string]string{
		"status":             "status",
		"pipelineId":         "pipeline_id",
		"attemptId":          "attempt_id",
		"workItemId":         "work_item_id",
		"repositoryTargetId": "repository_target_id",
	})
	output, err := repo.query(ctx, fmt.Sprintf(`.mode json
SELECT
  id,
  operation_id AS operationId,
  pipeline_id AS pipelineId,
  attempt_id AS attemptId,
  work_item_id AS workItemId,
  repository_target_id AS repositoryTargetId,
  stage_id AS stageId,
  agent_id AS agentId,
  status,
  priority,
  not_before AS notBefore,
  locked_by AS lockedBy,
  lock_expires_at AS lockExpiresAt,
  attempt_count AS attemptCount,
  queue_json AS queueJson,
  created_at AS createdAt,
  updated_at AS updatedAt
FROM operation_queue
WHERE %s
ORDER BY priority ASC, updated_at DESC, id DESC;
`, strings.Join(where, " AND ")))
	if err != nil {
		return nil, err
	}
	return decodeStructuredRows(output, map[string]string{"queue": "queueJson"})
}

func sqlFilterClauses(filters map[string]string, columns map[string]string) []string {
	where := []string{"1=1"}
	for key, column := range columns {
		if value := strings.TrimSpace(filters[key]); value != "" {
			where = append(where, fmt.Sprintf("%s = %s", column, sqlQuote(value)))
		}
	}
	return where
}

func decodeJSONRows(output string, jsonColumn string) ([]map[string]any, error) {
	if strings.TrimSpace(output) == "" {
		return []map[string]any{}, nil
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(output), &rows); err != nil {
		return nil, err
	}
	records := []map[string]any{}
	for _, row := range rows {
		var record map[string]any
		if err := json.Unmarshal([]byte(text(row, jsonColumn)), &record); err != nil {
			return nil, err
		}
		if record != nil {
			records = append(records, record)
		}
	}
	return records, nil
}

func decodeStructuredRows(output string, jsonColumns map[string]string) ([]map[string]any, error) {
	if strings.TrimSpace(output) == "" {
		return []map[string]any{}, nil
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(output), &rows); err != nil {
		return nil, err
	}
	for index, row := range rows {
		for targetKey, sourceKey := range jsonColumns {
			var value any
			_ = json.Unmarshal([]byte(text(row, sourceKey)), &value)
			row[targetKey] = value
			delete(row, sourceKey)
		}
		rows[index] = row
	}
	return rows, nil
}

func previewLocalTextFile(path string, limit int64) (map[string]any, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("proof source is a directory")
	}
	if limit <= 0 {
		limit = 128 * 1024
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	truncated := false
	if int64(len(raw)) > limit {
		raw = raw[:limit]
		truncated = true
	}
	content := string(raw)
	return map[string]any{
		"available":   true,
		"sourcePath":  path,
		"fileName":    filepath.Base(path),
		"extension":   strings.TrimPrefix(filepath.Ext(path), "."),
		"sizeBytes":   info.Size(),
		"content":     content,
		"truncated":   truncated,
		"previewType": proofPreviewType(path),
	}, nil
}

func readJSONMapIfPresent(path string) map[string]any {
	if strings.TrimSpace(path) == "" {
		return map[string]any{}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return map[string]any{}
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return map[string]any{}
	}
	if value == nil {
		return map[string]any{}
	}
	return value
}

func proofPreviewType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return "json"
	case ".md", ".markdown":
		return "markdown"
	case ".patch", ".diff":
		return "diff"
	default:
		return "text"
	}
}

func findRecordByID(records []map[string]any, id string) map[string]any {
	for _, record := range records {
		if text(record, "id") == id {
			return record
		}
	}
	return map[string]any{}
}

func latestAttemptForPipeline(database WorkspaceDatabase, pipelineID string) map[string]any {
	attempts := []map[string]any{}
	for _, attempt := range database.Tables.Attempts {
		if text(attempt, "pipelineId") == pipelineID {
			attempts = append(attempts, attempt)
		}
	}
	sort.SliceStable(attempts, func(left, right int) bool {
		return firstNonEmpty(text(attempts[left], "updatedAt"), text(attempts[left], "createdAt")) >
			firstNonEmpty(text(attempts[right], "updatedAt"), text(attempts[right], "createdAt"))
	})
	if len(attempts) == 0 {
		return map[string]any{}
	}
	return attempts[0]
}

func pipelineIDFromOperationID(operationID string) string {
	if strings.TrimSpace(operationID) == "" {
		return ""
	}
	if index := strings.Index(operationID, ":"); index > 0 {
		return operationID[:index]
	}
	return ""
}

func normalizeOperationQueueStatus(status string) string {
	normalized := strings.ToLower(strings.TrimSpace(status))
	switch normalized {
	case "ready", "pending", "waiting", "queued":
		return "queued"
	case "running", "in_progress", "in-progress":
		return "running"
	case "passed", "done", "completed", "succeeded", "success":
		return "done"
	case "failed", "error", "stalled":
		return "failed"
	case "canceled", "cancelled":
		return "canceled"
	default:
		if normalized == "" {
			return "queued"
		}
		return normalized
	}
}

func operationQueuePriority(status string) int {
	switch status {
	case "running":
		return 10
	case "queued":
		return 20
	case "failed":
		return 30
	case "canceled":
		return 80
	default:
		return 90
	}
}
