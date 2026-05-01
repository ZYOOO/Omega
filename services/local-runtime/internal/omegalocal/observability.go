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
		"generatedAt":           nowISO(),
		"attempts":              map[string]any{"total": 0, "terminal": 0, "active": 0, "successRate": 0},
		"failureReasons":        []map[string]any{},
		"slowStages":            []map[string]any{},
		"stageAverageDurations": []map[string]any{},
		"runnerUsage":           []map[string]any{},
		"checkpointWaitTimes":   map[string]any{"total": 0, "resolved": 0, "pending": 0, "averageWaitSeconds": 0, "maxWaitSeconds": 0, "byStage": []map[string]any{}},
		"pullRequests":          map[string]any{"created": 0, "merged": 0, "open": 0},
		"trends":                []map[string]any{},
		"waitingHumanQueue":     []map[string]any{},
		"activeRuns":            []map[string]any{},
		"recommendedActions":    []map[string]any{},
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
		"failureReasons":        observabilityFailureReasons(database),
		"slowStages":            observabilitySlowStages(database, 5),
		"stageAverageDurations": observabilityStageAverageDurations(database, 20),
		"runnerUsage":           observabilityRunnerUsage(database),
		"checkpointWaitTimes":   observabilityCheckpointWaitTimes(database),
		"pullRequests":          observabilityPullRequests(database),
		"trends":                observabilityTrends(database, 14),
		"waitingHumanQueue":     observabilityWaitingHumanQueue(database, 10),
		"activeRuns":            observabilityActiveRuns(database, 10),
		"recommendedActions":    observabilityRecommendedActions(pipelineStatus, checkpointStatus, operationStatus, workItemStatus, attemptStatus),
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

func observabilityStageAverageDurations(database WorkspaceDatabase, limit int) []map[string]any {
	type bucket struct {
		stageID       string
		title         string
		count         int
		totalDuration int
		maxDuration   int
		latestAt      string
	}
	buckets := map[string]*bucket{}
	for _, attempt := range database.Tables.Attempts {
		for _, stage := range arrayMaps(attempt["stages"]) {
			durationMs := stageDurationMillis(stage)
			if durationMs <= 0 {
				continue
			}
			stageID := text(stage, "id")
			if stageID == "" {
				stageID = text(stage, "stageId")
			}
			if stageID == "" {
				stageID = "unknown"
			}
			current := buckets[stageID]
			if current == nil {
				current = &bucket{stageID: stageID, title: stringOr(text(stage, "title"), stageID)}
				buckets[stageID] = current
			}
			current.count++
			current.totalDuration += durationMs
			if durationMs > current.maxDuration {
				current.maxDuration = durationMs
			}
			latestAt := stringOr(text(stage, "completedAt"), stringOr(text(stage, "finishedAt"), text(attempt, "updatedAt")))
			if latestAt > current.latestAt {
				current.latestAt = latestAt
			}
		}
	}
	output := make([]map[string]any, 0, len(buckets))
	for _, current := range buckets {
		average := 0
		if current.count > 0 {
			average = current.totalDuration / current.count
		}
		output = append(output, map[string]any{
			"stageId":           current.stageID,
			"title":             current.title,
			"count":             current.count,
			"averageDurationMs": average,
			"maxDurationMs":     current.maxDuration,
			"latestAt":          current.latestAt,
		})
	}
	sort.SliceStable(output, func(i, j int) bool {
		left := intValue(output[i]["averageDurationMs"])
		right := intValue(output[j]["averageDurationMs"])
		if left == right {
			return text(output[i], "latestAt") > text(output[j], "latestAt")
		}
		return left > right
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

func observabilityRunnerUsage(database WorkspaceDatabase) []map[string]any {
	type bucket struct {
		runner        string
		count         int
		successCount  int
		failureCount  int
		activeCount   int
		totalDuration int
		durationCount int
		latestAt      string
	}
	buckets := map[string]*bucket{}
	for _, attempt := range database.Tables.Attempts {
		runner := stringOr(text(attempt, "runner"), "unknown")
		current := buckets[runner]
		if current == nil {
			current = &bucket{runner: runner}
			buckets[runner] = current
		}
		current.count++
		switch text(attempt, "status") {
		case "done":
			current.successCount++
		case "failed", "stalled", "canceled":
			current.failureCount++
		case "running", "waiting-human":
			current.activeCount++
		}
		if duration := attemptDurationMillis(attempt); duration > 0 {
			current.totalDuration += duration
			current.durationCount++
		}
		latestAt := stringOr(text(attempt, "updatedAt"), stringOr(text(attempt, "finishedAt"), text(attempt, "startedAt")))
		if latestAt > current.latestAt {
			current.latestAt = latestAt
		}
	}
	output := make([]map[string]any, 0, len(buckets))
	for _, current := range buckets {
		average := 0
		if current.durationCount > 0 {
			average = current.totalDuration / current.durationCount
		}
		output = append(output, map[string]any{
			"runner":            current.runner,
			"count":             current.count,
			"successCount":      current.successCount,
			"failureCount":      current.failureCount,
			"activeCount":       current.activeCount,
			"averageDurationMs": average,
			"latestAt":          current.latestAt,
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
	return output
}

func observabilityCheckpointWaitTimes(database WorkspaceDatabase) map[string]any {
	total := checkpointWaitBucket{stageID: "all"}
	byStage := map[string]*checkpointWaitBucket{}
	for _, checkpoint := range database.Tables.Checkpoints {
		createdAt := stringOr(text(checkpoint, "createdAt"), text(checkpoint, "updatedAt"))
		resolvedAt := ""
		if status := text(checkpoint, "status"); status == "approved" || status == "rejected" || status == "canceled" {
			resolvedAt = text(checkpoint, "updatedAt")
		}
		waitSeconds := durationSeconds(createdAt, resolvedAt)
		stageID := stringOr(text(checkpoint, "stageId"), "unknown")
		current := byStage[stageID]
		if current == nil {
			current = &checkpointWaitBucket{stageID: stageID}
			byStage[stageID] = current
		}
		for _, target := range []*checkpointWaitBucket{&total, current} {
			target.count++
			if resolvedAt == "" {
				target.pending++
			} else {
				target.resolved++
			}
			target.totalSeconds += waitSeconds
			if waitSeconds > target.maxSeconds {
				target.maxSeconds = waitSeconds
			}
		}
	}
	stageRows := make([]map[string]any, 0, len(byStage))
	for _, current := range byStage {
		stageRows = append(stageRows, checkpointWaitBucketMap(*current))
	}
	sort.SliceStable(stageRows, func(i, j int) bool {
		left := intValue(stageRows[i]["averageWaitSeconds"])
		right := intValue(stageRows[j]["averageWaitSeconds"])
		if left == right {
			return intValue(stageRows[i]["count"]) > intValue(stageRows[j]["count"])
		}
		return left > right
	})
	result := checkpointWaitBucketMap(total)
	result["byStage"] = stageRows
	return result
}

type checkpointWaitBucket struct {
	stageID      string
	count        int
	resolved     int
	pending      int
	totalSeconds int
	maxSeconds   int
}

func checkpointWaitBucketMap(current checkpointWaitBucket) map[string]any {
	average := 0
	if current.count > 0 {
		average = current.totalSeconds / current.count
	}
	return map[string]any{
		"stageId":            current.stageID,
		"total":              current.count,
		"count":              current.count,
		"resolved":           current.resolved,
		"pending":            current.pending,
		"averageWaitSeconds": average,
		"maxWaitSeconds":     current.maxSeconds,
	}
}

func observabilityPullRequests(database WorkspaceDatabase) map[string]any {
	created := map[string]string{}
	merged := map[string]string{}
	for _, attempt := range database.Tables.Attempts {
		prURL := text(attempt, "pullRequestUrl")
		if prURL == "" {
			continue
		}
		created[prURL] = stringOr(text(attempt, "createdAt"), text(attempt, "startedAt"))
		if status := text(attempt, "status"); status == "done" || status == "merged" || status == "delivered" {
			merged[prURL] = stringOr(text(attempt, "finishedAt"), text(attempt, "updatedAt"))
		}
	}
	for _, proof := range database.Tables.ProofRecords {
		value := text(proof, "value")
		if !strings.Contains(value, "/pull/") {
			continue
		}
		created[value] = stringOr(text(proof, "createdAt"), created[value])
		label := strings.ToLower(text(proof, "label"))
		if strings.Contains(label, "merge") || strings.Contains(label, "merged") {
			merged[value] = text(proof, "createdAt")
		}
	}
	return map[string]any{
		"created": len(created),
		"merged":  len(merged),
		"open":    len(created) - len(merged),
	}
}

func observabilityTrends(database WorkspaceDatabase, limit int) []map[string]any {
	buckets := map[string]map[string]any{}
	prCreatedByDay := map[string]struct{}{}
	prMergedByDay := map[string]struct{}{}
	for _, attempt := range database.Tables.Attempts {
		startBucket := trendBucket(buckets, dayBucket(stringOr(text(attempt, "startedAt"), text(attempt, "createdAt"))))
		startBucket["attemptsStarted"] = intValue(startBucket["attemptsStarted"]) + 1
		if prURL := text(attempt, "pullRequestUrl"); prURL != "" {
			prCreatedByDay[dayBucket(stringOr(text(attempt, "createdAt"), text(attempt, "startedAt")))+"|"+prURL] = struct{}{}
		}
		if finished := dayBucket(stringOr(text(attempt, "finishedAt"), text(attempt, "updatedAt"))); finished != "" && terminalAttemptStatus(text(attempt, "status")) {
			finishBucket := trendBucket(buckets, finished)
			finishBucket["attemptsCompleted"] = intValue(finishBucket["attemptsCompleted"]) + 1
			if text(attempt, "status") == "done" {
				finishBucket["attemptsDone"] = intValue(finishBucket["attemptsDone"]) + 1
			}
			if text(attempt, "status") == "failed" {
				finishBucket["attemptsFailed"] = intValue(finishBucket["attemptsFailed"]) + 1
			}
			if text(attempt, "pullRequestUrl") != "" && text(attempt, "status") == "done" {
				prMergedByDay[finished+"|"+text(attempt, "pullRequestUrl")] = struct{}{}
			}
		}
	}
	for _, proof := range database.Tables.ProofRecords {
		prURL := text(proof, "value")
		if !strings.Contains(prURL, "/pull/") {
			continue
		}
		day := dayBucket(text(proof, "createdAt"))
		label := strings.ToLower(text(proof, "label"))
		if strings.Contains(label, "merge") || strings.Contains(label, "merged") {
			prMergedByDay[day+"|"+prURL] = struct{}{}
		} else {
			prCreatedByDay[day+"|"+prURL] = struct{}{}
		}
	}
	for key := range prCreatedByDay {
		day := strings.SplitN(key, "|", 2)[0]
		if day == "" {
			continue
		}
		row := trendBucket(buckets, day)
		row["pullRequestsCreated"] = intValue(row["pullRequestsCreated"]) + 1
	}
	for key := range prMergedByDay {
		day := strings.SplitN(key, "|", 2)[0]
		if day == "" {
			continue
		}
		row := trendBucket(buckets, day)
		row["pullRequestsMerged"] = intValue(row["pullRequestsMerged"]) + 1
	}
	for _, checkpoint := range database.Tables.Checkpoints {
		createdBucket := trendBucket(buckets, dayBucket(stringOr(text(checkpoint, "createdAt"), text(checkpoint, "updatedAt"))))
		createdBucket["checkpointsCreated"] = intValue(createdBucket["checkpointsCreated"]) + 1
		status := text(checkpoint, "status")
		if status == "approved" || status == "rejected" || status == "canceled" {
			resolvedBucket := trendBucket(buckets, dayBucket(text(checkpoint, "updatedAt")))
			resolvedBucket["checkpointsResolved"] = intValue(resolvedBucket["checkpointsResolved"]) + 1
		}
	}
	rows := make([]map[string]any, 0, len(buckets))
	for _, row := range buckets {
		if text(row, "date") != "" {
			rows = append(rows, row)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return text(rows[i], "date") > text(rows[j], "date")
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return text(rows[i], "date") < text(rows[j], "date")
	})
	return rows
}

func terminalAttemptStatus(status string) bool {
	return status == "done" || status == "failed" || status == "stalled" || status == "canceled"
}

func attemptDurationMillis(attempt map[string]any) int {
	if duration := intValue(attempt["durationMs"]); duration > 0 {
		return duration
	}
	startedAt := text(attempt, "startedAt")
	finishedAt := stringOr(text(attempt, "finishedAt"), text(attempt, "updatedAt"))
	if startedAt == "" || finishedAt == "" {
		return 0
	}
	started, err := time.Parse(time.RFC3339Nano, startedAt)
	if err != nil {
		return 0
	}
	finished, err := time.Parse(time.RFC3339Nano, finishedAt)
	if err != nil || finished.Before(started) {
		return 0
	}
	return int(finished.Sub(started).Milliseconds())
}

func durationSeconds(startedAt string, finishedAt string) int {
	if startedAt == "" {
		return 0
	}
	started, err := time.Parse(time.RFC3339Nano, startedAt)
	if err != nil {
		return 0
	}
	var finished time.Time
	if finishedAt == "" {
		finished = time.Now()
	} else {
		finished, err = time.Parse(time.RFC3339Nano, finishedAt)
		if err != nil {
			return 0
		}
	}
	if finished.Before(started) {
		return 0
	}
	return int(finished.Sub(started).Seconds())
}

func dayBucket(timestamp string) string {
	if timestamp == "" {
		return ""
	}
	parsed, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil {
		return ""
	}
	return parsed.UTC().Format("2006-01-02")
}

func trendBucket(buckets map[string]map[string]any, date string) map[string]any {
	if date == "" {
		return map[string]any{}
	}
	current := buckets[date]
	if current == nil {
		current = map[string]any{
			"date":                date,
			"attemptsStarted":     0,
			"attemptsCompleted":   0,
			"attemptsDone":        0,
			"attemptsFailed":      0,
			"pullRequestsCreated": 0,
			"pullRequestsMerged":  0,
			"checkpointsCreated":  0,
			"checkpointsResolved": 0,
		}
		buckets[date] = current
	}
	return current
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
