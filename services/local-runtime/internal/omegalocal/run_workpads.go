package omegalocal

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

func upsertRunWorkpad(database *WorkspaceDatabase, attemptID string) map[string]any {
	if database == nil || attemptID == "" {
		return nil
	}
	attemptIndex := findByID(database.Tables.Attempts, attemptID)
	if attemptIndex < 0 {
		return nil
	}
	attempt := cloneMap(database.Tables.Attempts[attemptIndex])
	pipelineIndex := findByID(database.Tables.Pipelines, text(attempt, "pipelineId"))
	var pipeline map[string]any
	if pipelineIndex >= 0 {
		pipeline = cloneMap(database.Tables.Pipelines[pipelineIndex])
	}
	item := findWorkItem(*database, text(attempt, "itemId"))
	requirement := workpadRequirement(*database, item)
	timestamp := nowISO()
	recordID := fmt.Sprintf("%s:workpad", attemptID)
	createdAt := timestamp
	if existingIndex := findByID(database.Tables.RunWorkpads, recordID); existingIndex >= 0 {
		createdAt = stringOr(database.Tables.RunWorkpads[existingIndex]["createdAt"], timestamp)
	}
	workpad := map[string]any{
		"plan":               workpadPlan(pipeline, attempt),
		"acceptanceCriteria": workpadAcceptanceCriteria(item, requirement),
		"validation":         workpadValidation(*database, pipeline, attempt),
		"notes":              workpadNotes(attempt),
		"blockers":           workpadBlockers(*database, pipeline, attempt),
		"pr":                 workpadPullRequest(attempt),
		"reviewFeedback":     workpadReviewFeedback(*database, pipeline, attempt),
		"retryReason":        workpadRetryReason(attempt),
		"reworkAssessment":   mapValue(attempt["reworkAssessment"]),
		"updatedBy":          "runtime",
	}
	record := map[string]any{
		"id":                 recordID,
		"attemptId":          attemptID,
		"pipelineId":         text(attempt, "pipelineId"),
		"workItemId":         text(attempt, "itemId"),
		"repositoryTargetId": text(attempt, "repositoryTargetId"),
		"status":             text(attempt, "status"),
		"workpad":            workpad,
		"createdAt":          createdAt,
		"updatedAt":          timestamp,
	}
	database.Tables.RunWorkpads = appendOrReplace(database.Tables.RunWorkpads, record)
	return record
}

func upsertLatestRunWorkpadForPipeline(database *WorkspaceDatabase, pipelineID string) map[string]any {
	if database == nil || pipelineID == "" {
		return nil
	}
	if attemptIndex := latestAttemptIndexForPipeline(*database, pipelineID); attemptIndex >= 0 {
		return upsertRunWorkpad(database, text(database.Tables.Attempts[attemptIndex], "id"))
	}
	return nil
}

func workpadRequirement(database WorkspaceDatabase, item map[string]any) map[string]any {
	if item == nil {
		return nil
	}
	requirementID := text(item, "requirementId")
	if requirementID == "" {
		return nil
	}
	if index := findByID(database.Tables.Requirements, requirementID); index >= 0 {
		return database.Tables.Requirements[index]
	}
	return nil
}

func workpadPlan(pipeline map[string]any, attempt map[string]any) map[string]any {
	stages := []map[string]any{}
	if pipeline != nil {
		for _, stage := range arrayMaps(mapValue(pipeline["run"])["stages"]) {
			stages = append(stages, map[string]any{
				"id":      text(stage, "id"),
				"title":   text(stage, "title"),
				"status":  text(stage, "status"),
				"agents":  stringSlice(stage["agentIds"]),
				"started": text(stage, "startedAt"),
				"done":    text(stage, "completedAt"),
			})
		}
	}
	return map[string]any{
		"pipelineStatus": stringOr(text(pipeline, "status"), "unknown"),
		"currentStageId": text(attempt, "currentStageId"),
		"stageCount":     len(stages),
		"stages":         stages,
	}
}

func workpadAcceptanceCriteria(item map[string]any, requirement map[string]any) []string {
	values := stringSlice(nil)
	if requirement != nil {
		values = append(values, stringSlice(requirement["acceptanceCriteria"])...)
	}
	if len(values) == 0 && item != nil {
		values = append(values, stringSlice(item["acceptanceCriteria"])...)
	}
	if len(values) == 0 {
		values = append(values, "Human reviewer can verify the requested change against the original requirement.")
	}
	return compactStringList(values)
}

func workpadValidation(database WorkspaceDatabase, pipeline map[string]any, attempt map[string]any) map[string]any {
	proofs := workpadProofSummaries(database, text(pipeline, "id"), "test")
	status := "pending"
	if text(attempt, "status") == "done" {
		status = "passed"
	}
	if text(attempt, "status") == "failed" || text(attempt, "status") == "stalled" || text(attempt, "status") == "canceled" {
		status = "needs-attention"
	}
	return map[string]any{
		"status":     status,
		"proofCount": len(proofs),
		"proofs":     proofs,
	}
}

func workpadNotes(attempt map[string]any) []string {
	notes := []string{}
	for _, event := range arrayMaps(attempt["events"]) {
		message := strings.TrimSpace(text(event, "message"))
		if message == "" {
			continue
		}
		notes = append(notes, message)
	}
	return compactStringList(notes)
}

func workpadBlockers(database WorkspaceDatabase, pipeline map[string]any, attempt map[string]any) []string {
	blockers := []string{}
	for _, value := range []string{text(attempt, "failureReason"), text(attempt, "statusReason"), text(attempt, "errorMessage"), text(attempt, "failureDetail")} {
		if strings.TrimSpace(value) != "" {
			blockers = append(blockers, value)
		}
	}
	if status := text(pipeline, "status"); status == "failed" || status == "stalled" || status == "canceled" {
		blockers = append(blockers, "Pipeline is "+status+".")
	}
	for _, checkpoint := range database.Tables.Checkpoints {
		if text(checkpoint, "pipelineId") != text(attempt, "pipelineId") {
			continue
		}
		if status := text(checkpoint, "status"); status == "rejected" || status == "canceled" || status == "pending" {
			blockers = append(blockers, strings.TrimSpace(text(checkpoint, "title")+" "+text(checkpoint, "decisionNote")))
		}
	}
	return compactStringList(blockers)
}

func workpadPullRequest(attempt map[string]any) map[string]any {
	return map[string]any{
		"url":       text(attempt, "pullRequestUrl"),
		"branch":    text(attempt, "branchName"),
		"workspace": text(attempt, "workspacePath"),
	}
}

func workpadReviewFeedback(database WorkspaceDatabase, pipeline map[string]any, attempt map[string]any) []string {
	feedback := []string{}
	if value := strings.TrimSpace(text(attempt, "humanChangeRequest")); value != "" {
		feedback = append(feedback, value)
	}
	if value := strings.TrimSpace(text(attempt, "failureReviewFeedback")); value != "" {
		feedback = append(feedback, value)
	}
	if value := latestHumanChangeRequestFromPipeline(pipeline); value != "" {
		feedback = append(feedback, value)
	}
	for _, proof := range workpadProofSummaries(database, text(pipeline, "id"), "review") {
		feedback = append(feedback, stringOr(proof["label"], stringOr(proof["path"], "")))
	}
	for _, operation := range database.Tables.Operations {
		if !strings.Contains(text(operation, "stageId"), "review") {
			continue
		}
		if summary := strings.TrimSpace(text(operation, "summary")); summary != "" {
			feedback = append(feedback, summary)
		}
	}
	return compactStringList(feedback)
}

func workpadRetryReason(attempt map[string]any) string {
	if attempt == nil {
		return ""
	}
	for _, value := range []string{text(attempt, "retryReason"), text(attempt, "humanChangeRequest"), text(attempt, "failureReason"), text(attempt, "failureReviewFeedback"), text(attempt, "statusReason"), text(attempt, "errorMessage"), text(attempt, "failureDetail"), text(attempt, "stderrSummary")} {
		if strings.TrimSpace(value) != "" {
			return truncateForProof(value, 1200)
		}
	}
	if status := text(attempt, "status"); status == "failed" || status == "stalled" || status == "canceled" {
		return "Attempt is " + status + " and needs a retry decision."
	}
	return ""
}

func workpadProofSummaries(database WorkspaceDatabase, pipelineID string, kind string) []map[string]any {
	operationIDs := map[string]bool{}
	for _, operation := range database.Tables.Operations {
		operationID := text(operation, "id")
		if pipelineID != "" && !strings.Contains(operationID, pipelineID) {
			continue
		}
		if kind != "" {
			haystack := strings.ToLower(text(operation, "stageId") + " " + text(operation, "agentId") + " " + text(operation, "summary"))
			if !strings.Contains(haystack, strings.ToLower(kind)) {
				continue
			}
		}
		operationIDs[operationID] = true
	}
	proofs := []map[string]any{}
	for _, proof := range database.Tables.ProofRecords {
		if len(operationIDs) > 0 && !operationIDs[text(proof, "operationId")] {
			continue
		}
		if kind != "" {
			haystack := strings.ToLower(text(proof, "label") + " " + text(proof, "value") + " " + text(proof, "sourcePath"))
			if !strings.Contains(haystack, strings.ToLower(kind)) {
				continue
			}
		}
		proofs = append(proofs, map[string]any{
			"id":    text(proof, "id"),
			"label": text(proof, "label"),
			"value": text(proof, "value"),
			"path":  filepath.Base(text(proof, "sourcePath")),
		})
	}
	return proofs
}

func filterRunWorkpads(records []map[string]any, query url.Values) []map[string]any {
	filters := map[string]string{
		"attemptId":          query.Get("attemptId"),
		"pipelineId":         query.Get("pipelineId"),
		"workItemId":         query.Get("workItemId"),
		"repositoryTargetId": query.Get("repositoryTargetId"),
		"status":             query.Get("status"),
	}
	filtered := []map[string]any{}
	for _, record := range records {
		matched := true
		for key, value := range filters {
			if value == "" {
				continue
			}
			if text(record, key) != value {
				matched = false
				break
			}
		}
		if matched {
			filtered = append(filtered, record)
		}
	}
	return filtered
}

func compactStringList(values []string) []string {
	output := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		output = append(output, value)
	}
	return output
}
