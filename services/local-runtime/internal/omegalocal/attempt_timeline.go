package omegalocal

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (server *Server) attemptTimeline(response http.ResponseWriter, request *http.Request) {
	attemptID := strings.TrimSuffix(pathID(request.URL.Path), "/timeline")
	limit := intValueFromString(request.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 120
	}
	if limit > 240 {
		limit = 240
	}
	database, attempt, pipeline, err := server.loadAttemptTimelineContext(request.Context(), attemptID, limit)
	if err != nil {
		writeJSON(response, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if attempt == nil {
		writeJSON(response, http.StatusNotFound, map[string]any{"error": "attempt not found"})
		return
	}

	items, err := server.buildAttemptTimelineItems(request.Context(), database, attempt, pipeline, limit)
	if err != nil {
		writeJSON(response, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if limit > 0 && len(items) > limit {
		items = items[len(items)-limit:]
	}
	writeJSON(response, http.StatusOK, AttemptTimelineResponse{
		Attempt:     compactAttemptTimelineRecord(attempt),
		Pipeline:    compactPipelineTimelineRecord(pipeline),
		Items:       items,
		GeneratedAt: nowISO(),
	})
}

func (server *Server) loadAttemptTimelineContext(ctx context.Context, attemptID string, limit int) (WorkspaceDatabase, map[string]any, map[string]any, error) {
	attempts, err := server.Repo.ListAttempts(ctx, map[string]string{"id": attemptID, "limit": "1"})
	if err != nil {
		return WorkspaceDatabase{}, nil, nil, err
	}
	if len(attempts) == 0 {
		return WorkspaceDatabase{}, nil, nil, nil
	}
	attempt := cloneMap(attempts[0])
	pipelineID := text(attempt, "pipelineId")
	workItemID := text(attempt, "itemId")
	database := WorkspaceDatabase{SchemaVersion: 1, SavedAt: nowISO()}
	database.Tables.Attempts = []map[string]any{attempt}

	var pipeline map[string]any
	if pipelineID != "" {
		pipelines, err := server.Repo.ListPipelines(ctx, map[string]string{"id": pipelineID, "limit": "1"})
		if err != nil {
			return WorkspaceDatabase{}, nil, nil, err
		}
		if len(pipelines) > 0 {
			pipeline = cloneMap(pipelines[0])
			database.Tables.Pipelines = []map[string]any{pipeline}
		}
	}

	if pipelineID != "" || workItemID != "" {
		readLimit := timelineRecordReadLimit(limit)
		operationFilters := map[string]string{"limit": readLimit}
		proofFilters := map[string]string{"limit": readLimit}
		if pipelineID != "" {
			operationFilters["pipelineId"] = pipelineID
			proofFilters["pipelineId"] = pipelineID
		} else {
			operationFilters["workItemId"] = workItemID
			proofFilters["workItemId"] = workItemID
		}
		operations, err := server.Repo.ListOperations(ctx, operationFilters)
		if err != nil {
			return WorkspaceDatabase{}, nil, nil, err
		}
		proofs, err := server.Repo.ListProofRecords(ctx, proofFilters)
		if err != nil {
			return WorkspaceDatabase{}, nil, nil, err
		}
		database.Tables.Operations = operations
		database.Tables.ProofRecords = proofs
	}

	checkpoints := []map[string]any{}
	checkpointLimit := timelineCheckpointReadLimit(limit)
	if pipelineID != "" {
		rows, err := server.Repo.ListCheckpoints(ctx, map[string]string{"pipelineId": pipelineID, "limit": checkpointLimit})
		if err != nil {
			return WorkspaceDatabase{}, nil, nil, err
		}
		checkpoints = appendUniqueTimelineMaps(checkpoints, rows)
	}
	if attemptID != "" {
		rows, err := server.Repo.ListCheckpoints(ctx, map[string]string{"attemptId": attemptID, "limit": checkpointLimit})
		if err != nil {
			return WorkspaceDatabase{}, nil, nil, err
		}
		checkpoints = appendUniqueTimelineMaps(checkpoints, rows)
	}
	database.Tables.Checkpoints = checkpoints
	return database, attempt, pipeline, nil
}

func timelineRecordReadLimit(limit int) string {
	readLimit := limit * 2
	if readLimit < 80 {
		readLimit = 80
	}
	if readLimit > 160 {
		readLimit = 160
	}
	return strconv.Itoa(readLimit)
}

func timelineCheckpointReadLimit(limit int) string {
	readLimit := limit
	if readLimit < 40 {
		readLimit = 40
	}
	if readLimit > 120 {
		readLimit = 120
	}
	return strconv.Itoa(readLimit)
}

func appendUniqueTimelineMaps(current []map[string]any, next []map[string]any) []map[string]any {
	seen := map[string]bool{}
	for _, record := range current {
		if id := text(record, "id"); id != "" {
			seen[id] = true
		}
	}
	for _, record := range next {
		id := text(record, "id")
		if id != "" && seen[id] {
			continue
		}
		if id != "" {
			seen[id] = true
		}
		current = append(current, record)
	}
	return current
}

func compactAttemptTimelineRecord(attempt map[string]any) map[string]any {
	return compactDetails(attempt,
		"id",
		"pipelineId",
		"itemId",
		"repositoryTargetId",
		"status",
		"currentStageId",
		"runner",
		"trigger",
		"branchName",
		"pullRequestUrl",
		"workspacePath",
		"retryOfAttemptId",
		"retryRootAttemptId",
		"retryIndex",
		"durationMs",
		"createdAt",
		"startedAt",
		"finishedAt",
		"updatedAt",
	)
}

func compactPipelineTimelineRecord(pipeline map[string]any) map[string]any {
	if pipeline == nil {
		return nil
	}
	return compactDetails(pipeline,
		"id",
		"workItemId",
		"templateId",
		"runId",
		"status",
		"createdAt",
		"updatedAt",
	)
}

func intValueFromString(value string) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return parsed
}

func (server *Server) buildAttemptTimelineItems(ctx context.Context, database WorkspaceDatabase, attempt map[string]any, pipeline map[string]any, limit int) ([]AttemptTimelineItem, error) {
	attemptID := text(attempt, "id")
	pipelineID := text(attempt, "pipelineId")
	fallbackTime := firstNonEmpty(text(attempt, "startedAt"), text(attempt, "createdAt"), text(attempt, "updatedAt"), nowISO())
	builder := attemptTimelineBuilder{items: []AttemptTimelineItem{}, seen: map[string]bool{}, fallbackTime: fallbackTime}

	for index, event := range arrayMaps(attempt["events"]) {
		builder.add(AttemptTimelineItem{
			ID:        fmt.Sprintf("attempt-event:%s:%d", attemptID, index),
			Time:      firstNonEmpty(text(event, "createdAt"), text(event, "timestamp"), fallbackTime),
			Source:    "attempt",
			Level:     timelineLevel(text(event, "type"), text(event, "status")),
			EventType: firstNonEmpty(text(event, "type"), "attempt.event"),
			Message:   firstNonEmpty(text(event, "message"), text(event, "type")),
			StageID:   text(event, "stageId"),
			AgentID:   text(event, "agentId"),
			Details:   compactDetails(event, "id", "type", "message", "createdAt", "timestamp", "stageId", "agentId"),
		})
	}

	for index, stage := range arrayMaps(attempt["stages"]) {
		status := text(stage, "status")
		stageTime := firstNonEmpty(text(stage, "completedAt"), text(stage, "startedAt"), text(attempt, "updatedAt"), fallbackTime)
		builder.add(AttemptTimelineItem{
			ID:        fmt.Sprintf("attempt-stage:%s:%s:%d", attemptID, text(stage, "id"), index),
			Time:      stageTime,
			Source:    "stage",
			Level:     timelineLevel("stage."+status, status),
			EventType: "stage." + firstNonEmpty(status, "observed"),
			Message:   fmt.Sprintf("%s is %s.", firstNonEmpty(text(stage, "title"), text(stage, "id"), "Stage"), firstNonEmpty(status, "observed")),
			StageID:   text(stage, "id"),
			Details:   compactDetails(stage, "id", "title", "status", "startedAt", "completedAt", "agentIds", "evidence"),
		})
	}

	if pipeline != nil {
		run := mapValue(pipeline["run"])
		for index, event := range arrayMaps(run["events"]) {
			builder.add(AttemptTimelineItem{
				ID:        fmt.Sprintf("pipeline-event:%s:%d", text(pipeline, "id"), index),
				Time:      firstNonEmpty(text(event, "timestamp"), text(event, "createdAt"), fallbackTime),
				Source:    "pipeline",
				Level:     timelineLevel(text(event, "type"), text(event, "status")),
				EventType: firstNonEmpty(text(event, "type"), "pipeline.event"),
				Message:   firstNonEmpty(text(event, "message"), text(event, "type")),
				StageID:   text(event, "stageId"),
				AgentID:   text(event, "agentId"),
				Details:   compactDetails(event, "id", "type", "message", "timestamp", "createdAt", "stageId", "agentId"),
			})
		}
		for index, stage := range arrayMaps(run["stages"]) {
			status := text(stage, "status")
			builder.add(AttemptTimelineItem{
				ID:        fmt.Sprintf("pipeline-stage:%s:%s:%d", text(pipeline, "id"), text(stage, "id"), index),
				Time:      firstNonEmpty(text(stage, "completedAt"), text(stage, "startedAt"), text(pipeline, "updatedAt"), fallbackTime),
				Source:    "stage",
				Level:     timelineLevel("stage."+status, status),
				EventType: "stage." + firstNonEmpty(status, "observed"),
				Message:   fmt.Sprintf("%s is %s.", firstNonEmpty(text(stage, "title"), text(stage, "id"), "Stage"), firstNonEmpty(status, "observed")),
				StageID:   text(stage, "id"),
				AgentID:   text(stage, "agentId"),
				Details:   compactDetails(stage, "id", "title", "status", "startedAt", "completedAt", "agentIds", "evidence", "notes"),
			})
		}
	}

	operationIDs := map[string]bool{}
	for _, operation := range database.Tables.Operations {
		if !operationBelongsToAttempt(operation, attempt, pipelineID) {
			continue
		}
		operationID := text(operation, "id")
		operationIDs[operationID] = true
		status := text(operation, "status")
		builder.add(AttemptTimelineItem{
			ID:          "operation:" + operationID,
			Time:        firstNonEmpty(text(operation, "updatedAt"), text(operation, "createdAt"), fallbackTime),
			Source:      "operation",
			Level:       timelineLevel("operation."+status, status),
			EventType:   "operation." + firstNonEmpty(status, "observed"),
			Message:     firstNonEmpty(text(operation, "summary"), text(operation, "prompt"), fmt.Sprintf("Operation %s is %s.", operationID, status)),
			StageID:     text(operation, "stageId"),
			AgentID:     text(operation, "agentId"),
			OperationID: operationID,
			Details:     compactDetails(operation, "id", "missionId", "stageId", "agentId", "status", "requiredProof", "runnerProcess"),
		})
	}

	for _, proof := range database.Tables.ProofRecords {
		operationID := text(proof, "operationId")
		if !operationIDs[operationID] && pipelineID != "" && !strings.Contains(operationID, pipelineID) {
			continue
		}
		builder.add(AttemptTimelineItem{
			ID:          "proof:" + text(proof, "id"),
			Time:        firstNonEmpty(text(proof, "createdAt"), fallbackTime),
			Source:      "proof",
			Level:       timelineLevel("proof.collected", text(proof, "status")),
			EventType:   "proof.collected",
			Message:     firstNonEmpty(text(proof, "label"), text(proof, "value"), text(proof, "sourcePath"), "Proof collected."),
			OperationID: operationID,
			ProofID:     text(proof, "id"),
			Details:     compactDetails(proof, "id", "operationId", "label", "value", "sourcePath", "sourceUrl", "status"),
		})
	}

	for _, checkpoint := range database.Tables.Checkpoints {
		if text(checkpoint, "attemptId") != attemptID && text(checkpoint, "pipelineId") != pipelineID {
			continue
		}
		status := text(checkpoint, "status")
		builder.add(AttemptTimelineItem{
			ID:           "checkpoint:" + text(checkpoint, "id"),
			Time:         firstNonEmpty(text(checkpoint, "updatedAt"), text(checkpoint, "createdAt"), fallbackTime),
			Source:       "checkpoint",
			Level:        timelineLevel("checkpoint."+status, status),
			EventType:    "checkpoint." + firstNonEmpty(status, "observed"),
			Message:      firstNonEmpty(text(checkpoint, "decisionNote"), text(checkpoint, "summary"), text(checkpoint, "title")),
			StageID:      text(checkpoint, "stageId"),
			CheckpointID: text(checkpoint, "id"),
			Details:      compactDetails(checkpoint, "id", "pipelineId", "attemptId", "stageId", "status", "title", "summary"),
		})
	}

	logs, err := timelineRuntimeLogs(ctx, server, attemptID, pipelineID, limit)
	if err != nil {
		return nil, err
	}
	for _, log := range logs {
		builder.add(AttemptTimelineItem{
			ID:           "runtime-log:" + log.ID,
			Time:         firstNonEmpty(log.CreatedAt, fallbackTime),
			Source:       "runtime-log",
			Level:        firstNonEmpty(log.Level, "INFO"),
			EventType:    firstNonEmpty(log.EventType, "runtime.log"),
			Message:      firstNonEmpty(log.Message, log.EventType),
			StageID:      log.StageID,
			AgentID:      log.AgentID,
			RuntimeLogID: log.ID,
			Details:      log.Details,
		})
	}

	builder.sort()
	return builder.items, nil
}

type attemptTimelineBuilder struct {
	items        []AttemptTimelineItem
	seen         map[string]bool
	fallbackTime string
}

func (builder *attemptTimelineBuilder) add(item AttemptTimelineItem) {
	if strings.TrimSpace(item.ID) == "" || builder.seen[item.ID] {
		return
	}
	item.Time = firstNonEmpty(item.Time, builder.fallbackTime, nowISO())
	item.Source = firstNonEmpty(item.Source, "runtime")
	item.Level = firstNonEmpty(strings.ToUpper(item.Level), "INFO")
	item.EventType = firstNonEmpty(item.EventType, "runtime.event")
	item.Message = firstNonEmpty(item.Message, item.EventType)
	builder.seen[item.ID] = true
	builder.items = append(builder.items, item)
}

func (builder *attemptTimelineBuilder) sort() {
	sort.SliceStable(builder.items, func(left, right int) bool {
		return timelineTimeLess(builder.items[left].Time, builder.items[right].Time)
	})
}

func operationBelongsToAttempt(operation map[string]any, attempt map[string]any, pipelineID string) bool {
	attemptID := text(attempt, "id")
	workItemID := text(attempt, "itemId")
	operationID := text(operation, "id")
	missionID := text(operation, "missionId")
	prompt := text(operation, "prompt")
	return (attemptID != "" && (strings.Contains(operationID, attemptID) || strings.Contains(missionID, attemptID) || strings.Contains(prompt, attemptID))) ||
		(pipelineID != "" && (strings.Contains(operationID, pipelineID) || strings.Contains(missionID, pipelineID) || strings.Contains(prompt, pipelineID))) ||
		(workItemID != "" && strings.Contains(missionID, workItemID))
}

func timelineRuntimeLogs(ctx context.Context, server *Server, attemptID string, pipelineID string, limit int) ([]RuntimeLogRecord, error) {
	records := []RuntimeLogRecord{}
	seen := map[string]bool{}
	readLimit := limit
	if readLimit <= 0 {
		readLimit = 120
	}
	if readLimit < 80 {
		readLimit = 80
	}
	if readLimit > 160 {
		readLimit = 160
	}
	for _, filters := range []map[string]string{{"attemptId": attemptID}, {"pipelineId": pipelineID}} {
		for key, value := range filters {
			if strings.TrimSpace(value) == "" {
				delete(filters, key)
			}
		}
		if len(filters) == 0 {
			continue
		}
		logs, err := server.Repo.ListRuntimeLogs(ctx, filters, readLimit)
		if err != nil {
			return nil, err
		}
		for _, log := range logs {
			if seen[log.ID] {
				continue
			}
			seen[log.ID] = true
			records = append(records, log)
		}
	}
	return records, nil
}

func compactDetails(record map[string]any, keys ...string) map[string]any {
	output := map[string]any{}
	for _, key := range keys {
		if value, ok := record[key]; ok && value != nil && strings.TrimSpace(fmt.Sprint(value)) != "" {
			output[key] = value
		}
	}
	if len(output) == 0 {
		return nil
	}
	return output
}

func timelineLevel(eventType string, status string) string {
	text := strings.ToLower(eventType + " " + status)
	switch {
	case strings.Contains(text, "failed"), strings.Contains(text, "error"), strings.Contains(text, "stalled"), strings.Contains(text, "rejected"), strings.Contains(text, "blocked"):
		return "ERROR"
	case strings.Contains(text, "waiting"), strings.Contains(text, "pending"), strings.Contains(text, "human"):
		return "INFO"
	default:
		return "INFO"
	}
}

func timelineTimeLess(left string, right string) bool {
	leftTime, leftErr := time.Parse(time.RFC3339Nano, left)
	rightTime, rightErr := time.Parse(time.RFC3339Nano, right)
	if leftErr == nil && rightErr == nil {
		return leftTime.Before(rightTime)
	}
	return left < right
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
