package omegalocal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (server *Server) jobSupervisorTick(response http.ResponseWriter, request *http.Request) {
	var options jobSupervisorTickOptions
	if err := json.NewDecoder(request.Body).Decode(&options); err != nil && !errors.Is(err, io.EOF) {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	summary, err := server.reconcileAttemptIntegrity(request.Context(), options)
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, summary)
}

func (server *Server) registerAttemptJob(attemptID string, cancel context.CancelFunc) {
	if strings.TrimSpace(attemptID) == "" || cancel == nil {
		return
	}
	server.jobMu.Lock()
	defer server.jobMu.Unlock()
	if server.attemptCancels == nil {
		server.attemptCancels = map[string]context.CancelFunc{}
	}
	server.attemptCancels[attemptID] = cancel
}

func (server *Server) unregisterAttemptJob(attemptID string) {
	server.jobMu.Lock()
	defer server.jobMu.Unlock()
	delete(server.attemptCancels, attemptID)
}

func (server *Server) cancelRegisteredAttemptJob(attemptID string) bool {
	server.jobMu.Lock()
	cancel := server.attemptCancels[attemptID]
	server.jobMu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	return true
}

type jobSupervisorTickOptions struct {
	StaleAfterSeconds         int  `json:"staleAfterSeconds"`
	AutoRunReady              bool `json:"autoRunReady"`
	AutoRetryFailed           bool `json:"autoRetryFailed"`
	AutoCleanupWorkspaces     bool `json:"autoCleanupWorkspaces"`
	MaxRetryAttempts          int  `json:"maxRetryAttempts"`
	RetryBackoffSeconds       int  `json:"retryBackoffSeconds"`
	WorkspaceRetentionSeconds int  `json:"workspaceRetentionSeconds"`
	Limit                     int  `json:"limit"`
}

func (server *Server) reconcileAttemptIntegrity(ctx context.Context, options jobSupervisorTickOptions) (map[string]any, error) {
	database, err := mustLoad(server, ctx)
	if err != nil {
		return nil, err
	}
	summary := server.reconcileAttemptIntegrityInDatabase(ctx, &database)
	mergeSupervisorSummary(summary, server.scanWorkflowContracts(ctx, &database))
	mergeSupervisorSummary(summary, server.markStalledAttempts(ctx, &database, options.StaleAfterSeconds))
	mergeSupervisorSummary(summary, server.scanWorkerHostLeases(ctx, &database))
	jobs := []map[string]any{}
	mergeSupervisorSummary(summary, server.scanRunnableWork(ctx, &database, options, &jobs))
	mergeSupervisorSummary(summary, server.scanRecoverableAttempts(ctx, &database, options, &jobs))
	mergeSupervisorSummary(summary, server.scanWorkspaceCleanup(ctx, &database, workspaceCleanupOptions{AutoCleanupWorkspaces: options.AutoCleanupWorkspaces, WorkspaceRetentionSeconds: options.WorkspaceRetentionSeconds, Limit: options.Limit}))
	if intValue(summary["changed"]) > 0 {
		if err := server.Repo.Save(ctx, database); err != nil {
			return nil, err
		}
	}
	for _, job := range jobs {
		server.startDevFlowCycleJob(text(job, "pipelineId"), text(job, "attemptId"), false, false, mapValue(job["lock"]))
	}
	return summary, nil
}

func (server *Server) scanWorkerHostLeases(ctx context.Context, database *WorkspaceDatabase) map[string]any {
	summary := map[string]any{
		"changed":                0,
		"checkedWorkerLeases":    0,
		"orphanedWorkerAttempts": 0,
	}
	if database == nil {
		return summary
	}
	for index, attempt := range database.Tables.Attempts {
		if text(attempt, "status") != "running" {
			continue
		}
		summary["checkedWorkerLeases"] = intValue(summary["checkedWorkerLeases"]) + 1
		if server.hasRegisteredAttemptJob(text(attempt, "id")) {
			continue
		}
		item := findWorkItem(*database, text(attempt, "itemId"))
		target := findRepositoryTarget(*database, text(attempt, "repositoryTargetId"))
		if item != nil && target != nil {
			if lock := existingExecutionLock(ctx, server, devFlowExecutionScope(item, target)); lock != nil && text(lock, "attemptId") == text(attempt, "id") {
				continue
			}
		}
		nextAttempt := cloneMap(attempt)
		nextAttempt["status"] = "stalled"
		nextAttempt["statusReason"] = "No active local worker host lease for running attempt."
		nextAttempt["stalledAt"] = nowISO()
		nextAttempt["updatedAt"] = text(nextAttempt, "stalledAt")
		events := arrayMaps(nextAttempt["events"])
		events = append(events, map[string]any{
			"type":      "attempt.worker.orphaned",
			"message":   text(nextAttempt, "statusReason"),
			"stageId":   text(nextAttempt, "currentStageId"),
			"createdAt": text(nextAttempt, "stalledAt"),
		})
		nextAttempt["events"] = events
		database.Tables.Attempts[index] = nextAttempt
		server.markPipelineStalled(database, nextAttempt)
		summary["changed"] = intValue(summary["changed"]) + 1
		summary["orphanedWorkerAttempts"] = intValue(summary["orphanedWorkerAttempts"]) + 1
		server.logError(ctx, "job_supervisor.worker.orphaned", text(nextAttempt, "statusReason"), map[string]any{"attemptId": text(nextAttempt, "id"), "pipelineId": text(nextAttempt, "pipelineId"), "workItemId": text(nextAttempt, "itemId")})
	}
	return summary
}

func (server *Server) scanWorkflowContracts(ctx context.Context, database *WorkspaceDatabase) map[string]any {
	summary := map[string]any{
		"changed":                   0,
		"checkedWorkflowPipelines":  0,
		"workflowContractPipelines": 0,
		"workflowContractMissing":   0,
		"workflowContracts":         []map[string]any{},
		"workflowContractFailures":  []map[string]any{},
	}
	if database == nil {
		return summary
	}
	for index, pipeline := range database.Tables.Pipelines {
		if !isDevFlowPRTemplate(text(pipeline, "templateId")) {
			continue
		}
		summary["checkedWorkflowPipelines"] = intValue(summary["checkedWorkflowPipelines"]) + 1
		nextPipeline := cloneMap(pipeline)
		run := mapValue(nextPipeline["run"])
		workflow := mapValue(run["workflow"])
		template := findPipelineTemplate(text(pipeline, "templateId"))
		if template != nil {
			changed := false
			if text(workflow, "id") == "" {
				workflow["id"] = template.ID
				changed = true
			}
			if text(workflow, "source") == "" {
				workflow["source"] = template.Source
				changed = true
			}
			if len(arrayMaps(workflow["reviewRounds"])) == 0 {
				workflow["reviewRounds"] = template.ReviewRounds
				changed = true
			}
			if len(arrayMaps(workflow["transitions"])) == 0 {
				workflow["transitions"] = template.Transitions
				changed = true
			}
			if len(mapValue(workflow["runtime"])) == 0 {
				workflow["runtime"] = template.Runtime
				changed = true
			}
			if changed {
				run["workflow"] = workflow
				nextPipeline["run"] = run
				nextPipeline["updatedAt"] = nowISO()
				database.Tables.Pipelines[index] = nextPipeline
				summary["changed"] = intValue(summary["changed"]) + 1
				pipeline = nextPipeline
			}
		}
		contract := workflowContractSummary(pipeline)
		if text(contract, "source") == "" || intValue(contract["transitionCount"]) == 0 {
			summary["workflowContractMissing"] = intValue(summary["workflowContractMissing"]) + 1
			summary["workflowContractFailures"] = append(arrayMaps(summary["workflowContractFailures"]), contract)
			server.logError(ctx, "job_supervisor.workflow_contract.missing", "DevFlow pipeline is missing workflow contract metadata.", map[string]any{"pipelineId": text(pipeline, "id"), "workItemId": text(pipeline, "workItemId")})
			continue
		}
		summary["workflowContractPipelines"] = intValue(summary["workflowContractPipelines"]) + 1
		summary["workflowContracts"] = append(arrayMaps(summary["workflowContracts"]), contract)
	}
	return summary
}

func (server *Server) scanRecoverableAttempts(ctx context.Context, database *WorkspaceDatabase, options jobSupervisorTickOptions, jobs *[]map[string]any) map[string]any {
	summary := map[string]any{
		"changed":                 0,
		"checkedRecoveryAttempts": 0,
		"retryableAttempts":       0,
		"acceptedRetryAttempts":   0,
		"retryBackoff":            0,
		"retryLimitReached":       0,
		"retrySkippedActive":      0,
		"recoveryFailures":        []map[string]any{},
		"recoverableAttempts":     []map[string]any{},
		"acceptedRetryRuns":       []map[string]any{},
	}
	if database == nil {
		return summary
	}
	limit := options.Limit
	if limit <= 0 {
		limit = 10
	}
	for _, attempt := range database.Tables.Attempts {
		if intValue(summary["checkedRecoveryAttempts"]) >= limit {
			break
		}
		if !supervisorRecoverableAttemptStatus(text(attempt, "status")) {
			continue
		}
		summary["checkedRecoveryAttempts"] = intValue(summary["checkedRecoveryAttempts"]) + 1
		if text(attempt, "retryAttemptId") != "" {
			summary["retrySkippedActive"] = intValue(summary["retrySkippedActive"]) + 1
			continue
		}
		pipelineID := text(attempt, "pipelineId")
		maxRetries, backoffSeconds := supervisorRetryPolicyForAttempt(*database, attempt, options)
		if active := activeAttemptForPipeline(*database, pipelineID, text(attempt, "id")); active != "" {
			summary["retrySkippedActive"] = intValue(summary["retrySkippedActive"]) + 1
			continue
		}
		retryRootAttemptID := stringOr(text(attempt, "retryRootAttemptId"), text(attempt, "id"))
		nextIndex := nextRetryIndex(*database, retryRootAttemptID)
		record := map[string]any{
			"attemptId":           text(attempt, "id"),
			"pipelineId":          pipelineID,
			"workItemId":          text(attempt, "itemId"),
			"status":              text(attempt, "status"),
			"retryRootAttemptId":  retryRootAttemptID,
			"nextRetryIndex":      nextIndex,
			"reason":              supervisorAttemptFailureReason(attempt),
			"maxRetryAttempts":    maxRetries,
			"retryBackoffSeconds": backoffSeconds,
		}
		if nextIndex > maxRetries {
			summary["retryLimitReached"] = intValue(summary["retryLimitReached"]) + 1
			summary["recoverableAttempts"] = append(arrayMaps(summary["recoverableAttempts"]), withSupervisorDecision(record, "retry-limit-reached"))
			continue
		}
		if !retryBackoffElapsed(attempt, backoffSeconds) {
			summary["retryBackoff"] = intValue(summary["retryBackoff"]) + 1
			summary["recoverableAttempts"] = append(arrayMaps(summary["recoverableAttempts"]), withSupervisorDecision(record, "waiting-backoff"))
			continue
		}
		summary["retryableAttempts"] = intValue(summary["retryableAttempts"]) + 1
		summary["recoverableAttempts"] = append(arrayMaps(summary["recoverableAttempts"]), withSupervisorDecision(record, "retryable"))
		if !options.AutoRetryFailed {
			continue
		}
		reason := fmt.Sprintf("JobSupervisor retry #%d after %s: %s", nextIndex, text(attempt, "status"), stringOr(supervisorAttemptFailureReason(attempt), "recoverable attempt"))
		updated, pipeline, retryAttempt, err := server.prepareDevFlowAttemptRetry(ctx, *database, text(attempt, "id"), reason)
		if err != nil {
			summary["recoveryFailures"] = append(arrayMaps(summary["recoveryFailures"]), map[string]any{"attemptId": text(attempt, "id"), "error": err.Error()})
			server.logError(ctx, "job_supervisor.retry.failed", err.Error(), map[string]any{"attemptId": text(attempt, "id"), "pipelineId": pipelineID, "workItemId": text(attempt, "itemId")})
			continue
		}
		item := findWorkItem(updated, text(retryAttempt, "itemId"))
		target := findRepositoryTarget(updated, text(item, "repositoryTargetId"))
		lock, lockErr := claimDevFlowWorkspaceLock(ctx, server, item, target, pipeline, retryAttempt)
		if lockErr != nil {
			summary["recoveryFailures"] = append(arrayMaps(summary["recoveryFailures"]), map[string]any{"attemptId": text(attempt, "id"), "error": lockErr.Error()})
			server.logError(ctx, "job_supervisor.retry.workspace.lock_failed", lockErr.Error(), map[string]any{"attemptId": text(retryAttempt, "id"), "retryOfAttemptId": text(attempt, "id"), "pipelineId": pipelineID, "workItemId": text(attempt, "itemId")})
			continue
		}
		*database = updated
		summary["changed"] = intValue(summary["changed"]) + 1
		summary["acceptedRetryAttempts"] = intValue(summary["acceptedRetryAttempts"]) + 1
		accepted := map[string]any{"workItemId": text(retryAttempt, "itemId"), "pipelineId": text(pipeline, "id"), "attemptId": text(retryAttempt, "id"), "retryOfAttemptId": text(attempt, "id"), "retryIndex": intValue(retryAttempt["retryIndex"]), "lock": lock}
		summary["acceptedRetryRuns"] = append(arrayMaps(summary["acceptedRetryRuns"]), accepted)
		*jobs = append(*jobs, accepted)
		server.logInfo(ctx, "job_supervisor.retry.accepted", "Recoverable attempt accepted for retry.", map[string]any{"attemptId": text(retryAttempt, "id"), "retryOfAttemptId": text(attempt, "id"), "pipelineId": text(pipeline, "id"), "workItemId": text(retryAttempt, "itemId"), "retryIndex": intValue(retryAttempt["retryIndex"])})
	}
	return summary
}

func supervisorRetryPolicyForAttempt(database WorkspaceDatabase, attempt map[string]any, options jobSupervisorTickOptions) (int, int) {
	maxRetries := options.MaxRetryAttempts
	backoffSeconds := options.RetryBackoffSeconds
	pipeline := pipelineByID(database, text(attempt, "pipelineId"))
	runtime := workflowRuntimeFromPipeline(pipeline)
	if maxRetries <= 0 {
		maxRetries = intValue(runtime["maxRetryAttempts"])
	}
	if maxRetries <= 0 {
		maxRetries = 2
	}
	if backoffSeconds <= 0 {
		backoffSeconds = intValue(runtime["retryBackoffSeconds"])
	}
	if backoffSeconds < 0 {
		backoffSeconds = 0
	}
	return maxRetries, backoffSeconds
}

func (server *Server) reconcileAttemptIntegrityInDatabase(ctx context.Context, database *WorkspaceDatabase) map[string]any {
	summary := map[string]any{
		"status":              "ok",
		"changed":             0,
		"linkedCheckpoints":   0,
		"backfilledAttempts":  0,
		"checkedCheckpoints":  0,
		"pendingCheckpoints":  0,
		"unresolvedGateLinks": 0,
		"checkedAttempts":     0,
		"stalledAttempts":     0,
	}
	if database == nil {
		return summary
	}
	for index, checkpoint := range database.Tables.Checkpoints {
		summary["checkedCheckpoints"] = intValue(summary["checkedCheckpoints"]) + 1
		if text(checkpoint, "status") != "pending" {
			continue
		}
		summary["pendingCheckpoints"] = intValue(summary["pendingCheckpoints"]) + 1
		pipelineID := text(checkpoint, "pipelineId")
		pipelineIndex := findByID(database.Tables.Pipelines, pipelineID)
		if pipelineIndex < 0 {
			summary["unresolvedGateLinks"] = intValue(summary["unresolvedGateLinks"]) + 1
			continue
		}
		pipeline := cloneMap(database.Tables.Pipelines[pipelineIndex])
		attemptID := text(checkpoint, "attemptId")
		attemptIndex := -1
		if attemptID != "" {
			attemptIndex = findByID(database.Tables.Attempts, attemptID)
		}
		if attemptIndex < 0 {
			attemptIndex = latestAttemptIndexForPipeline(*database, pipelineID)
		}
		if attemptIndex < 0 && isDevFlowPRTemplate(text(pipeline, "templateId")) {
			if attempt, ok := server.backfillAttemptForCheckpoint(ctx, database, pipeline, checkpoint); ok {
				database.Tables.Attempts = appendOrReplace(database.Tables.Attempts, attempt)
				upsertRunWorkpad(database, text(attempt, "id"))
				attemptIndex = findByID(database.Tables.Attempts, text(attempt, "id"))
				summary["backfilledAttempts"] = intValue(summary["backfilledAttempts"]) + 1
			}
		}
		if attemptIndex < 0 {
			summary["unresolvedGateLinks"] = intValue(summary["unresolvedGateLinks"]) + 1
			continue
		}
		nextAttemptID := text(database.Tables.Attempts[attemptIndex], "id")
		if text(checkpoint, "attemptId") == nextAttemptID {
			continue
		}
		nextCheckpoint := cloneMap(checkpoint)
		nextCheckpoint["attemptId"] = nextAttemptID
		nextCheckpoint["updatedAt"] = nowISO()
		database.Tables.Checkpoints[index] = nextCheckpoint
		summary["linkedCheckpoints"] = intValue(summary["linkedCheckpoints"]) + 1
		summary["changed"] = intValue(summary["changed"]) + 1
	}
	return summary
}

func (server *Server) scanRunnableWork(ctx context.Context, database *WorkspaceDatabase, options jobSupervisorTickOptions, jobs *[]map[string]any) map[string]any {
	summary := map[string]any{
		"changed":             0,
		"checkedReadyItems":   0,
		"runnableItems":       0,
		"preflightFailed":     0,
		"acceptedReadyRuns":   0,
		"skippedActiveRuns":   0,
		"runnableWorkItems":   []map[string]any{},
		"preflightFailures":   []map[string]any{},
		"acceptedRunAttempts": []map[string]any{},
	}
	if database == nil {
		return summary
	}
	limit := options.Limit
	if limit <= 0 {
		limit = 10
	}
	template := findPipelineTemplate("devflow-pr")
	if template == nil {
		return summary
	}
	for _, item := range database.Tables.WorkItems {
		if intValue(summary["checkedReadyItems"]) >= limit {
			break
		}
		if text(item, "status") != "Ready" || text(item, "repositoryTargetId") == "" {
			continue
		}
		summary["checkedReadyItems"] = intValue(summary["checkedReadyItems"]) + 1
		stageItem, err := resolveWorkItemRepositoryTarget(*database, item)
		if err != nil {
			summary["preflightFailed"] = intValue(summary["preflightFailed"]) + 1
			summary["preflightFailures"] = append(arrayMaps(summary["preflightFailures"]), map[string]any{"workItemId": text(item, "id"), "error": err.Error()})
			continue
		}
		target := findRepositoryTarget(*database, text(stageItem, "repositoryTargetId"))
		profile := server.resolveAgentProfile(ctx, *database, stageItem, target)
		pipeline, pipelineIndex := pipelineForSupervisorWorkItem(*database, stageItem, template, profile)
		if active := activeAttemptForPipeline(*database, text(pipeline, "id"), ""); active != "" {
			summary["skippedActiveRuns"] = intValue(summary["skippedActiveRuns"]) + 1
			continue
		}
		preflight := server.preflightDevFlowRun(ctx, *database, stageItem, target, profile)
		if !preflight.ok() {
			summary["preflightFailed"] = intValue(summary["preflightFailed"]) + 1
			summary["preflightFailures"] = append(arrayMaps(summary["preflightFailures"]), map[string]any{"workItemId": text(item, "id"), "pipelineId": text(pipeline, "id"), "preflight": preflight})
			server.logError(ctx, "job_supervisor.preflight.failed", strings.Join(preflight.Errors, "; "), map[string]any{"workItemId": text(item, "id"), "pipelineId": text(pipeline, "id"), "repositoryTargetId": text(stageItem, "repositoryTargetId")})
			continue
		}
		if lock := existingExecutionLock(ctx, server, devFlowExecutionScope(stageItem, target)); lock != nil {
			summary["skippedActiveRuns"] = intValue(summary["skippedActiveRuns"]) + 1
			summary["runnableWorkItems"] = append(arrayMaps(summary["runnableWorkItems"]), map[string]any{"workItemId": text(item, "id"), "pipelineId": text(pipeline, "id"), "key": text(item, "key"), "decision": "workspace-locked", "lock": lock})
			continue
		}
		summary["runnableItems"] = intValue(summary["runnableItems"]) + 1
		summary["runnableWorkItems"] = append(arrayMaps(summary["runnableWorkItems"]), map[string]any{"workItemId": text(item, "id"), "pipelineId": text(pipeline, "id"), "key": text(item, "key")})
		if !options.AutoRunReady {
			continue
		}
		if pipelineIndex < 0 {
			database.Tables.Pipelines = append(database.Tables.Pipelines, pipeline)
			pipelineIndex = len(database.Tables.Pipelines) - 1
			summary["changed"] = intValue(summary["changed"]) + 1
		}
		var attempt map[string]any
		*database, pipeline, attempt = beginDevFlowAttempt(*database, pipelineIndex, stageItem, pipeline, "job-supervisor")
		lock, lockErr := claimDevFlowWorkspaceLock(ctx, server, stageItem, target, pipeline, attempt)
		if lockErr != nil {
			summary["recoveryFailures"] = append(arrayMaps(summary["recoveryFailures"]), map[string]any{"workItemId": text(stageItem, "id"), "pipelineId": text(pipeline, "id"), "error": lockErr.Error()})
			server.logError(ctx, "job_supervisor.workspace.lock_failed", lockErr.Error(), map[string]any{"workItemId": text(stageItem, "id"), "pipelineId": text(pipeline, "id"), "attemptId": text(attempt, "id"), "repositoryTargetId": text(stageItem, "repositoryTargetId")})
			continue
		}
		summary["changed"] = intValue(summary["changed"]) + 1
		summary["acceptedReadyRuns"] = intValue(summary["acceptedReadyRuns"]) + 1
		record := map[string]any{"workItemId": text(stageItem, "id"), "pipelineId": text(pipeline, "id"), "attemptId": text(attempt, "id"), "lock": lock}
		summary["acceptedRunAttempts"] = append(arrayMaps(summary["acceptedRunAttempts"]), record)
		*jobs = append(*jobs, record)
		server.logInfo(ctx, "job_supervisor.run.accepted", "Ready work item accepted by JobSupervisor.", map[string]any{"workItemId": text(stageItem, "id"), "pipelineId": text(pipeline, "id"), "attemptId": text(attempt, "id"), "repositoryTargetId": text(stageItem, "repositoryTargetId")})
	}
	return summary
}

func pipelineForSupervisorWorkItem(database WorkspaceDatabase, item map[string]any, template *PipelineTemplate, profile ProjectAgentProfile) (map[string]any, int) {
	for index, pipeline := range database.Tables.Pipelines {
		if text(pipeline, "workItemId") == text(item, "id") && isDevFlowPRTemplate(text(pipeline, "templateId")) {
			return cloneMap(pipeline), index
		}
	}
	pipeline := makePipelineWithTemplate(item, template)
	return attachAgentProfileToPipeline(pipeline, profile), -1
}

func supervisorRecoverableAttemptStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "stalled":
		return true
	default:
		return false
	}
}

func supervisorAttemptFailureReason(attempt map[string]any) string {
	for _, key := range []string{"errorMessage", "statusReason", "stderrSummary"} {
		if value := strings.TrimSpace(text(attempt, key)); value != "" {
			return truncateForProof(value, 160)
		}
	}
	return text(attempt, "status")
}

func withSupervisorDecision(record map[string]any, decision string) map[string]any {
	next := cloneMap(record)
	next["decision"] = decision
	return next
}

func retryBackoffElapsed(attempt map[string]any, backoffSeconds int) bool {
	if backoffSeconds <= 0 {
		return true
	}
	timestamp := stringOr(text(attempt, "retryRequestedAt"), stringOr(text(attempt, "updatedAt"), stringOr(text(attempt, "finishedAt"), text(attempt, "stalledAt"))))
	if timestamp == "" {
		return true
	}
	parsed, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil {
		return true
	}
	return time.Since(parsed) >= time.Duration(backoffSeconds)*time.Second
}

func workflowContractSummary(pipeline map[string]any) map[string]any {
	run := mapValue(pipeline["run"])
	workflow := mapValue(run["workflow"])
	runtime := mapValue(workflow["runtime"])
	stages := arrayMaps(run["stages"])
	humanGateCount := 0
	for _, stage := range stages {
		if boolValue(stage["humanGate"]) {
			humanGateCount++
		}
	}
	return map[string]any{
		"pipelineId":             text(pipeline, "id"),
		"workItemId":             text(pipeline, "workItemId"),
		"workflowId":             text(workflow, "id"),
		"source":                 text(workflow, "source"),
		"stageCount":             len(stages),
		"transitionCount":        len(arrayMaps(workflow["transitions"])),
		"reviewRoundCount":       len(arrayMaps(workflow["reviewRounds"])),
		"humanGateCount":         humanGateCount,
		"maxReviewCycles":        intValue(runtime["maxReviewCycles"]),
		"runnerHeartbeatSeconds": intValue(runtime["runnerHeartbeatSeconds"]),
		"attemptTimeoutMinutes":  intValue(runtime["attemptTimeoutMinutes"]),
		"maxRetryAttempts":       intValue(runtime["maxRetryAttempts"]),
		"retryBackoffSeconds":    intValue(runtime["retryBackoffSeconds"]),
	}
}

func pipelineByID(database WorkspaceDatabase, pipelineID string) map[string]any {
	for _, pipeline := range database.Tables.Pipelines {
		if text(pipeline, "id") == pipelineID {
			return pipeline
		}
	}
	return nil
}

func workflowRuntimeFromPipeline(pipeline map[string]any) map[string]any {
	if pipeline == nil {
		return map[string]any{}
	}
	run := mapValue(pipeline["run"])
	workflow := mapValue(run["workflow"])
	return mapValue(workflow["runtime"])
}

func (server *Server) markStalledAttempts(ctx context.Context, database *WorkspaceDatabase, staleAfterSeconds int) map[string]any {
	if staleAfterSeconds <= 0 {
		staleAfterSeconds = 15 * 60
	}
	threshold := time.Duration(staleAfterSeconds) * time.Second
	summary := map[string]any{
		"changed":         0,
		"checkedAttempts": 0,
		"stalledAttempts": 0,
	}
	if database == nil {
		return summary
	}
	now := time.Now().UTC()
	for index, attempt := range database.Tables.Attempts {
		summary["checkedAttempts"] = intValue(summary["checkedAttempts"]) + 1
		if text(attempt, "status") != "running" {
			continue
		}
		lastSeenAt := stringOr(text(attempt, "lastSeenAt"), stringOr(text(attempt, "updatedAt"), text(attempt, "startedAt")))
		lastSeen, err := time.Parse(time.RFC3339Nano, lastSeenAt)
		if err != nil || now.Sub(lastSeen) <= threshold {
			continue
		}
		nextAttempt := cloneMap(attempt)
		nextAttempt["status"] = "stalled"
		nextAttempt["statusReason"] = fmt.Sprintf("No heartbeat for %d seconds.", int(now.Sub(lastSeen).Seconds()))
		nextAttempt["stalledAt"] = nowISO()
		nextAttempt["updatedAt"] = text(nextAttempt, "stalledAt")
		events := arrayMaps(nextAttempt["events"])
		events = append(events, map[string]any{
			"type":      "attempt.stalled",
			"message":   text(nextAttempt, "statusReason"),
			"stageId":   text(nextAttempt, "currentStageId"),
			"createdAt": text(nextAttempt, "stalledAt"),
		})
		nextAttempt["events"] = events
		database.Tables.Attempts[index] = nextAttempt
		server.markPipelineStalled(database, nextAttempt)
		summary["changed"] = intValue(summary["changed"]) + 1
		summary["stalledAttempts"] = intValue(summary["stalledAttempts"]) + 1
		server.logError(ctx, "job_supervisor.attempt.stalled", text(nextAttempt, "statusReason"), map[string]any{
			"pipelineId": text(nextAttempt, "pipelineId"),
			"attemptId":  text(nextAttempt, "id"),
			"workItemId": text(nextAttempt, "itemId"),
			"stageId":    text(nextAttempt, "currentStageId"),
			"lastSeenAt": lastSeenAt,
		})
	}
	return summary
}

func (server *Server) markPipelineStalled(database *WorkspaceDatabase, attempt map[string]any) {
	pipelineIndex := findByID(database.Tables.Pipelines, text(attempt, "pipelineId"))
	if pipelineIndex >= 0 {
		pipeline := cloneMap(database.Tables.Pipelines[pipelineIndex])
		run := mapValue(pipeline["run"])
		appendRunEvent(run, "attempt.stalled", text(attempt, "statusReason"), text(attempt, "currentStageId"), "job-supervisor")
		pipeline["run"] = run
		pipeline["status"] = "stalled"
		pipeline["updatedAt"] = nowISO()
		database.Tables.Pipelines[pipelineIndex] = pipeline
	}
	if item := findWorkItem(*database, text(attempt, "itemId")); item != nil {
		*database = updateWorkItem(*database, text(item, "id"), map[string]any{"status": "Blocked"})
	}
}

func mergeSupervisorSummary(target map[string]any, source map[string]any) {
	for key, value := range source {
		if _, ok := value.(int); ok {
			target[key] = intValue(target[key]) + intValue(value)
			continue
		}
		if _, exists := target[key]; !exists {
			target[key] = value
		}
	}
}

func (server *Server) backfillAttemptForCheckpoint(ctx context.Context, database *WorkspaceDatabase, pipeline map[string]any, checkpoint map[string]any) (map[string]any, bool) {
	item := findWorkItem(*database, text(pipeline, "workItemId"))
	if item == nil {
		return nil, false
	}
	attempt := makeAttemptRecord(item, pipeline, "supervisor-repair", "devflow-pr", text(checkpoint, "stageId"))
	attempt["id"] = fmt.Sprintf("%s:attempt:recovered:%d", text(pipeline, "id"), len(database.Tables.Attempts)+1)
	attempt["status"] = "waiting-human"
	attempt["currentStageId"] = text(checkpoint, "stageId")
	attempt["recoveredAt"] = nowISO()
	if delivery := recoverAttemptDeliveryFromProof(*database, text(pipeline, "id")); delivery != nil {
		attempt["workspacePath"] = text(delivery, "workspacePath")
		attempt["branchName"] = text(delivery, "branchName")
		attempt["pullRequestUrl"] = text(delivery, "pullRequestUrl")
	}
	events := arrayMaps(attempt["events"])
	events = append(events, map[string]any{
		"type":      "attempt.recovered",
		"message":   "Attempt record recovered by JobSupervisor integrity scan.",
		"stageId":   text(checkpoint, "stageId"),
		"createdAt": nowISO(),
	})
	attempt["events"] = events
	server.logError(ctx, "job_supervisor.attempt.backfilled", "Missing attempt was recovered for a pending checkpoint.", map[string]any{
		"pipelineId": text(pipeline, "id"),
		"attemptId":  text(attempt, "id"),
		"workItemId": text(item, "id"),
		"stageId":    text(checkpoint, "stageId"),
	})
	return attempt, true
}

func attemptIndexForCheckpoint(database WorkspaceDatabase, checkpoint map[string]any) int {
	if attemptID := text(checkpoint, "attemptId"); attemptID != "" {
		if index := findByID(database.Tables.Attempts, attemptID); index >= 0 {
			return index
		}
	}
	return latestAttemptIndexForPipeline(database, text(checkpoint, "pipelineId"))
}

func recoverAttemptDeliveryFromProof(database WorkspaceDatabase, pipelineID string) map[string]any {
	for _, proof := range database.Tables.ProofRecords {
		sourcePath := text(proof, "sourcePath")
		if filepath.Base(sourcePath) != "handoff-bundle.json" {
			continue
		}
		raw, err := os.ReadFile(sourcePath)
		if err != nil {
			continue
		}
		var bundle map[string]any
		if json.Unmarshal(raw, &bundle) != nil {
			continue
		}
		if text(bundle, "pipelineId") == pipelineID {
			return bundle
		}
	}
	return nil
}
