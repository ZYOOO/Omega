package omegalocal

import (
	"sort"
	"strings"
	"time"
)

func emptyObservability() map[string]any {
	return map[string]any{
		"counts": map[string]any{
			"workItems":    0,
			"pipelines":    0,
			"checkpoints":  0,
			"missions":     0,
			"operations":   0,
			"proofRecords": 0,
			"events":       0,
			"runtimeLogs":  0,
			"attempts":     0,
		},
		"pipelineStatus":   map[string]int{},
		"checkpointStatus": map[string]int{},
		"operationStatus":  map[string]int{},
		"attemptStatus":    map[string]int{},
		"attention": map[string]any{
			"waitingHuman": 0,
			"failed":       0,
			"blocked":      0,
		},
		"recentErrors": []RuntimeLogRecord{},
		"dashboard":    emptyObservabilityDashboard(),
	}
}

func observabilitySummary(database WorkspaceDatabase) map[string]any {
	pipelineStatus := countBy(database.Tables.Pipelines, "status")
	checkpointStatus := countBy(database.Tables.Checkpoints, "status")
	operationStatus := countBy(database.Tables.Operations, "status")
	workItemStatus := countBy(database.Tables.WorkItems, "status")
	attemptStatus := countBy(database.Tables.Attempts, "status")

	return map[string]any{
		"counts": map[string]any{
			"workItems":    len(database.Tables.WorkItems),
			"pipelines":    len(database.Tables.Pipelines),
			"checkpoints":  len(database.Tables.Checkpoints),
			"missions":     len(database.Tables.Missions),
			"operations":   len(database.Tables.Operations),
			"proofRecords": len(database.Tables.ProofRecords),
			"events":       len(database.Tables.MissionEvents),
			"runtimeLogs":  0,
			"attempts":     len(database.Tables.Attempts),
		},
		"pipelineStatus":   pipelineStatus,
		"checkpointStatus": checkpointStatus,
		"operationStatus":  operationStatus,
		"workItemStatus":   workItemStatus,
		"attemptStatus":    attemptStatus,
		"attention": map[string]any{
			"waitingHuman": pipelineStatus["waiting-human"] + checkpointStatus["pending"] + attemptStatus["waiting-human"],
			"failed":       pipelineStatus["failed"] + operationStatus["failed"] + attemptStatus["failed"],
			"blocked":      workItemStatus["Blocked"],
		},
		"dashboard": observabilityDashboard(database, pipelineStatus, checkpointStatus, operationStatus, workItemStatus, attemptStatus),
	}
}

func countBy(records []map[string]any, key string) map[string]int {
	counts := map[string]int{}
	for _, record := range records {
		counts[text(record, key)]++
	}
	return counts
}

func emptyObservabilityDashboard() map[string]any {
	return map[string]any{
		"generatedAt":        nowISO(),
		"attempts":           map[string]any{"total": 0, "terminal": 0, "active": 0, "successRate": 0},
		"failureReasons":     []map[string]any{},
		"slowStages":         []map[string]any{},
		"waitingHumanQueue":  []map[string]any{},
		"activeRuns":         []map[string]any{},
		"recommendedActions": []map[string]any{},
	}
}

func observabilityDashboard(database WorkspaceDatabase, pipelineStatus map[string]int, checkpointStatus map[string]int, operationStatus map[string]int, workItemStatus map[string]int, attemptStatus map[string]int) map[string]any {
	terminalAttempts := 0
	for _, status := range []string{"done", "failed", "stalled", "canceled"} {
		terminalAttempts += attemptStatus[status]
	}
	successRate := 0.0
	if terminalAttempts > 0 {
		successRate = float64(attemptStatus["done"]) / float64(terminalAttempts)
	}
	return map[string]any{
		"generatedAt": nowISO(),
		"attempts": map[string]any{
			"total":       len(database.Tables.Attempts),
			"terminal":    terminalAttempts,
			"active":      attemptStatus["running"],
			"successRate": successRate,
			"status":      attemptStatus,
		},
		"failureReasons":     observabilityFailureReasons(database),
		"slowStages":         observabilitySlowStages(database, 5),
		"waitingHumanQueue":  observabilityWaitingHumanQueue(database, 10),
		"activeRuns":         observabilityActiveRuns(database, 10),
		"recommendedActions": observabilityRecommendedActions(pipelineStatus, checkpointStatus, operationStatus, workItemStatus, attemptStatus),
	}
}

func observabilityFailureReasons(database WorkspaceDatabase) []map[string]any {
	type bucket struct {
		reason          string
		count           int
		latestAttemptID string
		latestAt        string
	}
	buckets := map[string]*bucket{}
	for _, attempt := range database.Tables.Attempts {
		status := text(attempt, "status")
		if status != "failed" && status != "stalled" && status != "canceled" {
			continue
		}
		reason := failureReasonForAttempt(attempt)
		if reason == "" {
			reason = status
		}
		key := strings.ToLower(reason)
		current := buckets[key]
		if current == nil {
			current = &bucket{reason: reason}
			buckets[key] = current
		}
		current.count++
		updatedAt := stringOr(text(attempt, "updatedAt"), text(attempt, "finishedAt"))
		if updatedAt > current.latestAt {
			current.latestAt = updatedAt
			current.latestAttemptID = text(attempt, "id")
		}
	}
	output := make([]map[string]any, 0, len(buckets))
	for _, current := range buckets {
		output = append(output, map[string]any{
			"reason":          current.reason,
			"count":           current.count,
			"latestAttemptId": current.latestAttemptID,
			"latestAt":        current.latestAt,
		})
	}
	sort.SliceStable(output, func(i, j int) bool {
		left := intValue(output[i]["count"])
		right := intValue(output[j]["count"])
		if left == right {
			return text(output[i], "latestAt") > text(output[j], "latestAt")
		}
		return left > right
	})
	if len(output) > 10 {
		return output[:10]
	}
	return output
}

func failureReasonForAttempt(attempt map[string]any) string {
	for _, key := range []string{"errorMessage", "statusReason", "stderrSummary"} {
		if value := strings.TrimSpace(text(attempt, key)); value != "" {
			return truncateForProof(value, 160)
		}
	}
	events := arrayMaps(attempt["events"])
	for index := len(events) - 1; index >= 0; index-- {
		eventType := text(events[index], "type")
		if strings.Contains(eventType, "failed") || strings.Contains(eventType, "stalled") || strings.Contains(eventType, "canceled") {
			return truncateForProof(text(events[index], "message"), 160)
		}
	}
	return ""
}

func observabilitySlowStages(database WorkspaceDatabase, limit int) []map[string]any {
	output := []map[string]any{}
	for _, attempt := range database.Tables.Attempts {
		for _, stage := range arrayMaps(attempt["stages"]) {
			durationMs := stageDurationMillis(stage)
			if durationMs <= 0 {
				continue
			}
			output = append(output, map[string]any{
				"attemptId":   text(attempt, "id"),
				"pipelineId":  text(attempt, "pipelineId"),
				"workItemId":  text(attempt, "itemId"),
				"stageId":     text(stage, "id"),
				"title":       text(stage, "title"),
				"status":      text(stage, "status"),
				"startedAt":   text(stage, "startedAt"),
				"completedAt": text(stage, "completedAt"),
				"durationMs":  durationMs,
			})
		}
	}
	sort.SliceStable(output, func(i, j int) bool {
		return intValue(output[i]["durationMs"]) > intValue(output[j]["durationMs"])
	})
	if len(output) > limit {
		return output[:limit]
	}
	return output
}

func stageDurationMillis(stage map[string]any) int {
	if duration := intValue(stage["durationMs"]); duration > 0 {
		return duration
	}
	startedAt, completedAt := text(stage, "startedAt"), stringOr(text(stage, "completedAt"), text(stage, "finishedAt"))
	if startedAt == "" || completedAt == "" {
		return 0
	}
	started, err := time.Parse(time.RFC3339Nano, startedAt)
	if err != nil {
		return 0
	}
	completed, err := time.Parse(time.RFC3339Nano, completedAt)
	if err != nil || completed.Before(started) {
		return 0
	}
	return int(completed.Sub(started).Milliseconds())
}

func observabilityWaitingHumanQueue(database WorkspaceDatabase, limit int) []map[string]any {
	output := []map[string]any{}
	for _, checkpoint := range database.Tables.Checkpoints {
		if text(checkpoint, "status") != "pending" {
			continue
		}
		pipeline := findPipeline(database, text(checkpoint, "pipelineId"))
		itemID := ""
		if pipeline != nil {
			itemID = text(pipeline, "workItemId")
		}
		createdAt := stringOr(text(checkpoint, "createdAt"), text(checkpoint, "updatedAt"))
		output = append(output, map[string]any{
			"checkpointId": text(checkpoint, "id"),
			"pipelineId":   text(checkpoint, "pipelineId"),
			"attemptId":    text(checkpoint, "attemptId"),
			"workItemId":   itemID,
			"stageId":      text(checkpoint, "stageId"),
			"title":        text(checkpoint, "title"),
			"createdAt":    createdAt,
			"updatedAt":    text(checkpoint, "updatedAt"),
			"ageSeconds":   ageSeconds(createdAt),
		})
	}
	sort.SliceStable(output, func(i, j int) bool {
		return intValue(output[i]["ageSeconds"]) > intValue(output[j]["ageSeconds"])
	})
	if len(output) > limit {
		return output[:limit]
	}
	return output
}

func observabilityActiveRuns(database WorkspaceDatabase, limit int) []map[string]any {
	output := []map[string]any{}
	for _, attempt := range database.Tables.Attempts {
		status := text(attempt, "status")
		if status != "running" && status != "waiting-human" {
			continue
		}
		lastSeenAt := stringOr(text(attempt, "lastSeenAt"), text(attempt, "updatedAt"))
		output = append(output, map[string]any{
			"attemptId":          text(attempt, "id"),
			"status":             status,
			"pipelineId":         text(attempt, "pipelineId"),
			"workItemId":         text(attempt, "itemId"),
			"repositoryTargetId": text(attempt, "repositoryTargetId"),
			"stageId":            text(attempt, "currentStageId"),
			"lastSeenAt":         lastSeenAt,
			"lastSeenAgeSeconds": ageSeconds(lastSeenAt),
		})
	}
	sort.SliceStable(output, func(i, j int) bool {
		return intValue(output[i]["lastSeenAgeSeconds"]) > intValue(output[j]["lastSeenAgeSeconds"])
	})
	if len(output) > limit {
		return output[:limit]
	}
	return output
}

func observabilityRecommendedActions(pipelineStatus map[string]int, checkpointStatus map[string]int, operationStatus map[string]int, workItemStatus map[string]int, attemptStatus map[string]int) []map[string]any {
	actions := []map[string]any{}
	if checkpointStatus["pending"] > 0 || attemptStatus["waiting-human"] > 0 {
		actions = append(actions, map[string]any{"type": "review", "label": "Review pending human gates", "count": checkpointStatus["pending"] + attemptStatus["waiting-human"]})
	}
	if attemptStatus["stalled"] > 0 {
		actions = append(actions, map[string]any{"type": "retry", "label": "Inspect stalled attempts and retry or cancel", "count": attemptStatus["stalled"]})
	}
	if attemptStatus["failed"] > 0 || pipelineStatus["failed"] > 0 || operationStatus["failed"] > 0 {
		actions = append(actions, map[string]any{"type": "failure", "label": "Inspect failed runs", "count": attemptStatus["failed"] + pipelineStatus["failed"] + operationStatus["failed"]})
	}
	if workItemStatus["Blocked"] > 0 {
		actions = append(actions, map[string]any{"type": "blocked", "label": "Triage blocked work items", "count": workItemStatus["Blocked"]})
	}
	return actions
}

func findPipeline(database WorkspaceDatabase, pipelineID string) map[string]any {
	for _, pipeline := range database.Tables.Pipelines {
		if text(pipeline, "id") == pipelineID {
			return pipeline
		}
	}
	return nil
}

func ageSeconds(timestamp string) int {
	if timestamp == "" {
		return 0
	}
	parsed, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil {
		return 0
	}
	age := time.Since(parsed)
	if age < 0 {
		return 0
	}
	return int(age.Seconds())
}
