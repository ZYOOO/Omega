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
	database, err := server.Repo.Load(request.Context())
	if err != nil {
		writeJSON(response, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	attemptIndex := findByID(database.Tables.Attempts, attemptID)
	if attemptIndex < 0 {
		writeJSON(response, http.StatusNotFound, map[string]any{"error": "attempt not found"})
		return
	}

	attempt := cloneMap(database.Tables.Attempts[attemptIndex])
	pipeline := map[string]any(nil)
	if pipelineID := text(attempt, "pipelineId"); pipelineID != "" {
		if index := findByID(database.Tables.Pipelines, pipelineID); index >= 0 {
			pipeline = cloneMap(database.Tables.Pipelines[index])
		}
	}

	items, err := server.buildAttemptTimelineItems(request.Context(), *database, attempt, pipeline)
	if err != nil {
		writeJSON(response, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if limit > 0 && len(items) > limit {
		items = items[len(items)-limit:]
	}
	writeJSON(response, http.StatusOK, AttemptTimelineResponse{
		Attempt:     attempt,
		Pipeline:    pipeline,
		Items:       items,
		GeneratedAt: nowISO(),
	})
}

func intValueFromString(value string) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return parsed
}

func (server *Server) buildAttemptTimelineItems(ctx context.Context, database WorkspaceDatabase, attempt map[string]any, pipeline map[string]any) ([]AttemptTimelineItem, error) {
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

	logs, err := timelineRuntimeLogs(ctx, server, attemptID, pipelineID)
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

func timelineRuntimeLogs(ctx context.Context, server *Server, attemptID string, pipelineID string) ([]RuntimeLogRecord, error) {
	records := []RuntimeLogRecord{}
	seen := map[string]bool{}
	for _, filters := range []map[string]string{{"attemptId": attemptID}, {"pipelineId": pipelineID}} {
		for key, value := range filters {
			if strings.TrimSpace(value) == "" {
				delete(filters, key)
			}
		}
		if len(filters) == 0 {
			continue
		}
		logs, err := server.Repo.ListRuntimeLogs(ctx, filters, 200)
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
