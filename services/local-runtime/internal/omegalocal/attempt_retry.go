package omegalocal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type attemptRetryPayload struct {
	AutoApproveHuman bool   `json:"autoApproveHuman"`
	AutoMerge        bool   `json:"autoMerge"`
	Reason           string `json:"reason"`
	Wait             bool   `json:"wait"`
}

type attemptRetryError struct {
	status  int
	message string
}

func (err attemptRetryError) Error() string {
	return err.message
}

func (server *Server) retryAttempt(response http.ResponseWriter, request *http.Request) {
	attemptID := strings.TrimSuffix(pathID(request.URL.Path), "/retry")
	var payload attemptRetryPayload
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil && !errors.Is(err, io.EOF) {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	database, err := mustLoad(server, request.Context())
	if err != nil {
		writeError(response, http.StatusNotFound, err)
		return
	}
	database, pipeline, attempt, err := server.prepareDevFlowAttemptRetry(request.Context(), database, attemptID, payload.Reason)
	if err != nil {
		if retryErr, ok := err.(attemptRetryError); ok {
			writeJSON(response, retryErr.status, map[string]any{"error": retryErr.message})
			return
		}
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	item := findWorkItem(database, text(pipeline, "workItemId"))
	target := findRepositoryTarget(database, text(item, "repositoryTargetId"))
	lock, lockErr := claimDevFlowWorkspaceLock(request.Context(), server, item, target, pipeline, attempt)
	if lockErr != nil {
		server.logError(request.Context(), "attempt.retry.workspace.lock_failed", lockErr.Error(), map[string]any{"pipelineId": text(pipeline, "id"), "attemptId": text(attempt, "id"), "retryOfAttemptId": attemptID, "workItemId": text(attempt, "itemId")})
		writeJSON(response, http.StatusConflict, map[string]any{"error": lockErr.Error()})
		return
	}
	if err := server.Repo.Save(request.Context(), database); err != nil {
		nextLock := cloneMap(lock)
		nextLock["status"] = "released"
		nextLock["runnerProcessState"] = "failed"
		nextLock["releasedAt"] = nowISO()
		nextLock["updatedAt"] = nowISO()
		_ = saveExecutionLock(context.Background(), server, nextLock)
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	server.logInfo(request.Context(), "attempt.retry.created", "Retry attempt created.", map[string]any{
		"entityType":       "attempt",
		"entityId":         text(attempt, "id"),
		"pipelineId":       text(pipeline, "id"),
		"attemptId":        text(attempt, "id"),
		"retryOfAttemptId": attemptID,
		"workItemId":       text(attempt, "itemId"),
	})

	if !payload.Wait {
		server.startDevFlowCycleJob(text(pipeline, "id"), text(attempt, "id"), payload.AutoApproveHuman, payload.AutoMerge, lock)
		writeJSON(response, http.StatusAccepted, map[string]any{
			"status":           "accepted",
			"pipeline":         pipeline,
			"attempt":          attempt,
			"retryOfAttemptId": attemptID,
		})
		return
	}

	runContext, cancelRun := context.WithTimeout(context.Background(), devFlowAttemptTimeout(findPipelineTemplate(text(pipeline, "templateId"))))
	defer cancelRun()
	lockProcessState := "completed"
	defer func() {
		nextLock := cloneMap(lock)
		nextLock["status"] = "released"
		nextLock["runnerProcessState"] = lockProcessState
		nextLock["releasedAt"] = nowISO()
		nextLock["updatedAt"] = nowISO()
		_ = saveExecutionLock(context.Background(), server, nextLock)
	}()
	result, err := server.executeDevFlowPRCycle(runContext, pipeline, item, target, text(attempt, "id"), payload.AutoApproveHuman, payload.AutoMerge)
	if err != nil {
		lockProcessState = "failed"
		_ = server.failDevFlowCycleJobWithResult(context.Background(), text(pipeline, "id"), text(attempt, "id"), err, result)
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	pipeline, attempt, err = server.completeDevFlowCycleJob(context.Background(), text(pipeline, "id"), text(attempt, "id"), result)
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	result["pipeline"] = pipeline
	result["attempt"] = attempt
	result["retryOfAttemptId"] = attemptID
	writeJSON(response, http.StatusOK, result)
}

func (server *Server) prepareDevFlowAttemptRetry(ctx context.Context, database WorkspaceDatabase, attemptID string, reason string) (WorkspaceDatabase, map[string]any, map[string]any, error) {
	if strings.TrimSpace(attemptID) == "" {
		return database, nil, nil, attemptRetryError{status: http.StatusBadRequest, message: "attempt id is required"}
	}
	attemptIndex := findByID(database.Tables.Attempts, attemptID)
	if attemptIndex < 0 {
		return database, nil, nil, attemptRetryError{status: http.StatusNotFound, message: "attempt not found"}
	}
	previousAttempt := cloneMap(database.Tables.Attempts[attemptIndex])
	if !retryableAttemptStatus(text(previousAttempt, "status")) {
		return database, nil, nil, attemptRetryError{status: http.StatusConflict, message: fmt.Sprintf("attempt status %q is not retryable", text(previousAttempt, "status"))}
	}
	pipelineIndex := findByID(database.Tables.Pipelines, text(previousAttempt, "pipelineId"))
	if pipelineIndex < 0 {
		return database, nil, nil, attemptRetryError{status: http.StatusNotFound, message: "pipeline not found"}
	}
	pipeline := cloneMap(database.Tables.Pipelines[pipelineIndex])
	if !isDevFlowPRTemplate(text(pipeline, "templateId")) {
		return database, nil, nil, attemptRetryError{status: http.StatusConflict, message: "pipeline is not using the devflow-pr template"}
	}
	if active := activeAttemptForPipeline(database, text(pipeline, "id"), attemptID); active != "" {
		return database, nil, nil, attemptRetryError{status: http.StatusConflict, message: "pipeline already has an active attempt: " + active}
	}
	item := findWorkItem(database, text(pipeline, "workItemId"))
	if item == nil {
		return database, nil, nil, attemptRetryError{status: http.StatusNotFound, message: "work item not found"}
	}
	stageItem, err := resolveWorkItemRepositoryTarget(database, item)
	if err != nil {
		return database, nil, nil, attemptRetryError{status: http.StatusBadRequest, message: err.Error()}
	}
	target := findRepositoryTarget(database, text(stageItem, "repositoryTargetId"))
	if target == nil {
		return database, nil, nil, attemptRetryError{status: http.StatusBadRequest, message: fmt.Sprintf("work item %s has no repository workspace", text(stageItem, "key"))}
	}
	profile := server.resolveAgentProfile(ctx, database, stageItem, target)
	preflight := server.preflightDevFlowRun(ctx, database, stageItem, target, profile)
	if !preflight.ok() {
		return database, nil, nil, attemptRetryError{status: http.StatusBadRequest, message: "DevFlow preflight failed: " + strings.Join(preflight.Errors, "; ")}
	}

	timestamp := nowISO()
	retryRootAttemptID := stringOr(text(previousAttempt, "retryRootAttemptId"), text(previousAttempt, "id"))
	retryReason := stringOr(reason, "Retry requested by operator.")
	retryIndex := nextRetryIndex(database, retryRootAttemptID)
	database, pipeline, retryAttempt := beginDevFlowAttempt(database, pipelineIndex, stageItem, pipeline, "retry")
	retryAttempt["retryOfAttemptId"] = text(previousAttempt, "id")
	retryAttempt["retryRootAttemptId"] = retryRootAttemptID
	retryAttempt["retryIndex"] = retryIndex
	retryAttempt["retryReason"] = retryReason
	retryAttempt["retryRequestedAt"] = timestamp
	retryEvents := arrayMaps(retryAttempt["events"])
	retryEvents = append(retryEvents, map[string]any{
		"type":      "attempt.retry.started",
		"message":   retryReason,
		"stageId":   text(retryAttempt, "currentStageId"),
		"createdAt": timestamp,
	})
	retryAttempt["events"] = retryEvents
	database.Tables.Attempts = appendOrReplace(database.Tables.Attempts, retryAttempt)
	upsertRunWorkpad(&database, text(retryAttempt, "id"))

	previousIndex := findByID(database.Tables.Attempts, attemptID)
	if previousIndex >= 0 {
		previousAttempt = cloneMap(database.Tables.Attempts[previousIndex])
		previousAttempt["retryAttemptId"] = text(retryAttempt, "id")
		previousAttempt["retryRequestedAt"] = timestamp
		previousAttempt["updatedAt"] = timestamp
		previousEvents := arrayMaps(previousAttempt["events"])
		previousEvents = append(previousEvents, map[string]any{
			"type":      "attempt.retry.requested",
			"message":   retryReason,
			"stageId":   text(previousAttempt, "currentStageId"),
			"createdAt": timestamp,
		})
		previousAttempt["events"] = previousEvents
		database.Tables.Attempts[previousIndex] = previousAttempt
		upsertRunWorkpad(&database, attemptID)
	}
	touch(&database)
	return database, pipeline, retryAttempt, nil
}

func (server *Server) prepareDevFlowHumanRequestedRework(ctx context.Context, database WorkspaceDatabase, checkpoint map[string]any, reason string) (WorkspaceDatabase, map[string]any, map[string]any, error) {
	if text(checkpoint, "stageId") != "human_review" {
		return database, nil, nil, attemptRetryError{status: http.StatusConflict, message: "checkpoint is not a human review gate"}
	}
	pipelineIndex := findByID(database.Tables.Pipelines, text(checkpoint, "pipelineId"))
	if pipelineIndex < 0 {
		return database, nil, nil, attemptRetryError{status: http.StatusNotFound, message: "pipeline not found"}
	}
	pipeline := cloneMap(database.Tables.Pipelines[pipelineIndex])
	if !isDevFlowPRTemplate(text(pipeline, "templateId")) {
		return database, nil, nil, attemptRetryError{status: http.StatusConflict, message: "pipeline is not using the devflow-pr template"}
	}
	previousAttemptIndex := attemptIndexForCheckpoint(database, checkpoint)
	if previousAttemptIndex < 0 {
		return database, nil, nil, attemptRetryError{status: http.StatusNotFound, message: "attempt not found"}
	}
	previousAttempt := cloneMap(database.Tables.Attempts[previousAttemptIndex])
	previousAttemptID := text(previousAttempt, "id")
	if active := activeAttemptForPipeline(database, text(pipeline, "id"), previousAttemptID); active != "" {
		return database, nil, nil, attemptRetryError{status: http.StatusConflict, message: "pipeline already has an active attempt: " + active}
	}
	item := findWorkItem(database, text(pipeline, "workItemId"))
	if item == nil {
		return database, nil, nil, attemptRetryError{status: http.StatusNotFound, message: "work item not found"}
	}
	stageItem, err := resolveWorkItemRepositoryTarget(database, item)
	if err != nil {
		return database, nil, nil, attemptRetryError{status: http.StatusBadRequest, message: err.Error()}
	}
	target := findRepositoryTarget(database, text(stageItem, "repositoryTargetId"))
	if target == nil {
		return database, nil, nil, attemptRetryError{status: http.StatusBadRequest, message: fmt.Sprintf("work item %s has no repository workspace", text(stageItem, "key"))}
	}
	profile := server.resolveAgentProfile(ctx, database, stageItem, target)
	preflight := server.preflightDevFlowRun(ctx, database, stageItem, target, profile)
	if !preflight.ok() {
		return database, nil, nil, attemptRetryError{status: http.StatusBadRequest, message: "DevFlow preflight failed: " + strings.Join(preflight.Errors, "; ")}
	}

	timestamp := nowISO()
	changeRequest := strings.TrimSpace(stringOr(reason, "Human requested changes."))
	retryRootAttemptID := stringOr(text(previousAttempt, "retryRootAttemptId"), previousAttemptID)
	retryIndex := nextRetryIndex(database, retryRootAttemptID)
	assessment := assessHumanRequestedRework(changeRequest, stageItem, previousAttempt, pipeline)
	entryStageID := stringOr(text(assessment, "entryStageId"), "rework")

	previousAttempt["status"] = "changes-requested"
	previousAttempt["statusReason"] = "Human requested changes."
	previousAttempt["failureReviewFeedback"] = truncateForProof(changeRequest, 1600)
	previousAttempt["reworkAssessment"] = assessment
	previousAttempt["finishedAt"] = timestamp
	previousAttempt["lastSeenAt"] = timestamp
	previousAttempt["durationMs"] = durationSinceMillis(text(previousAttempt, "startedAt"), timestamp)
	previousAttempt["updatedAt"] = timestamp
	previousEvents := arrayMaps(previousAttempt["events"])
	previousEvents = append(previousEvents, map[string]any{
		"type":      "attempt.human_changes_requested",
		"message":   changeRequest,
		"stageId":   text(checkpoint, "stageId"),
		"createdAt": timestamp,
	})
	previousAttempt["events"] = previousEvents
	database.Tables.Attempts[previousAttemptIndex] = previousAttempt

	run := mapValue(pipeline["run"])
	appendRunEvent(run, "human.rework.requested", "Human requested changes: "+changeRequest, "human_review", "human")
	appendRunEvent(run, "human.rework.assessed", fmt.Sprintf("Rework assessment selected %s: %s", text(assessment, "strategy"), text(assessment, "rationale")), entryStageID, "master")
	pipeline["run"] = run
	database.Tables.Pipelines[pipelineIndex] = pipeline

	database, pipeline, reworkAttempt := beginDevFlowAttemptFromStage(database, pipelineIndex, stageItem, pipeline, "human-request-changes", entryStageID)
	reworkReason := "Human requested changes: " + changeRequest
	reworkAttempt["retryOfAttemptId"] = previousAttemptID
	reworkAttempt["retryRootAttemptId"] = retryRootAttemptID
	reworkAttempt["retryIndex"] = retryIndex
	reworkAttempt["retryReason"] = reworkReason
	reworkAttempt["humanChangeRequest"] = changeRequest
	reworkAttempt["reworkAssessment"] = assessment
	reworkAttempt["retryRequestedAt"] = timestamp
	if branchName := text(previousAttempt, "branchName"); branchName != "" {
		reworkAttempt["branchName"] = branchName
	}
	if prURL := text(previousAttempt, "pullRequestUrl"); prURL != "" {
		reworkAttempt["pullRequestUrl"] = prURL
	}
	if workspacePath := text(previousAttempt, "workspacePath"); workspacePath != "" {
		reworkAttempt["workspacePath"] = workspacePath
	}
	reworkEvents := arrayMaps(reworkAttempt["events"])
	reworkEvents = append(reworkEvents, map[string]any{
		"type":      "attempt.human_rework.started",
		"message":   reworkReason,
		"stageId":   text(reworkAttempt, "currentStageId"),
		"createdAt": timestamp,
	})
	reworkEvents = append(reworkEvents, map[string]any{
		"type":      "attempt.rework_assessed",
		"message":   fmt.Sprintf("%s: %s", text(assessment, "strategy"), text(assessment, "rationale")),
		"stageId":   entryStageID,
		"createdAt": timestamp,
	})
	reworkAttempt["events"] = reworkEvents
	if text(assessment, "strategy") == reworkStrategyNeedsHumanInfo {
		reworkAttempt["status"] = "waiting-human"
		reworkAttempt["currentStageId"] = "human_review"
		reworkAttempt["statusReason"] = text(assessment, "rationale")
		reworkAttempt["failureReviewFeedback"] = changeRequest
		reworkAttempt["lastSeenAt"] = timestamp
		reworkAttempt["finishedAt"] = timestamp
		reworkAttempt["updatedAt"] = timestamp
		run = mapValue(pipeline["run"])
		stages := arrayMaps(run["stages"])
		for stageIndex, stage := range stages {
			if text(stage, "id") == "human_review" {
				stage["status"] = "needs-human"
				stage["startedAt"] = timestamp
				stage["notes"] = text(assessment, "rationale")
			} else if text(stage, "id") == "done" || text(stage, "id") == "merging" {
				stage["status"] = "waiting"
				delete(stage, "startedAt")
				delete(stage, "completedAt")
				delete(stage, "notes")
			}
			stages[stageIndex] = stage
		}
		appendRunEvent(run, "human.rework.needs_info", text(assessment, "rationale"), "human_review", "master")
		run["stages"] = stages
		pipeline["run"] = run
		pipeline["status"] = "waiting-human"
	}
	database.Tables.Attempts = appendOrReplace(database.Tables.Attempts, reworkAttempt)

	previousAttemptIndex = findByID(database.Tables.Attempts, previousAttemptID)
	if previousAttemptIndex >= 0 {
		previousAttempt = cloneMap(database.Tables.Attempts[previousAttemptIndex])
		previousAttempt["retryAttemptId"] = text(reworkAttempt, "id")
		previousAttempt["retryRequestedAt"] = timestamp
		previousAttempt["updatedAt"] = timestamp
		database.Tables.Attempts[previousAttemptIndex] = previousAttempt
		upsertRunWorkpad(&database, previousAttemptID)
	}

	run = mapValue(pipeline["run"])
	if text(assessment, "strategy") == reworkStrategyNeedsHumanInfo {
		appendRunEvent(run, "human.rework.waiting_info", text(assessment, "rationale"), text(reworkAttempt, "currentStageId"), "master")
	} else {
		appendRunEvent(run, "human.rework.started", reworkReason, text(reworkAttempt, "currentStageId"), "master")
	}
	pipeline["run"] = run
	pipeline["updatedAt"] = timestamp
	database.Tables.Pipelines[pipelineIndex] = pipeline
	upsertRunWorkpad(&database, text(reworkAttempt, "id"))
	touch(&database)
	return database, pipeline, reworkAttempt, nil
}

func retryableAttemptStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "stalled", "canceled":
		return true
	default:
		return false
	}
}

func activeAttemptForPipeline(database WorkspaceDatabase, pipelineID string, excludeAttemptID string) string {
	for _, attempt := range database.Tables.Attempts {
		if text(attempt, "pipelineId") != pipelineID || text(attempt, "id") == excludeAttemptID {
			continue
		}
		switch strings.ToLower(text(attempt, "status")) {
		case "running", "waiting-human":
			return text(attempt, "id")
		}
	}
	return ""
}

func nextRetryIndex(database WorkspaceDatabase, retryRootAttemptID string) int {
	next := 1
	for _, attempt := range database.Tables.Attempts {
		if text(attempt, "retryRootAttemptId") == retryRootAttemptID || text(attempt, "retryOfAttemptId") == retryRootAttemptID {
			next++
		}
	}
	return next
}
