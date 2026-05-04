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
	loadedSavedAt := database.SavedAt
	summary := server.reconcileAttemptIntegrityInDatabase(ctx, &database)
	mergeSupervisorSummary(summary, server.scanWorkflowContracts(ctx, &database))
	mergeSupervisorSummary(summary, server.scanRemoteAttemptSignals(ctx, &database, options))
	if server.feishuReviewTaskBridgeEnabled(ctx) {
		if bridgeResult, _, err := server.tickFeishuReviewTaskBridge(ctx, "", options.Limit, false); err == nil {
			summary["feishuReviewTaskBridge"] = bridgeResult
		} else {
			summary["feishuReviewTaskBridge"] = map[string]any{"status": "failed", "error": err.Error()}
		}
	}
	mergeSupervisorSummary(summary, server.markStalledAttempts(ctx, &database, options.StaleAfterSeconds))
	mergeSupervisorSummary(summary, server.scanWorkerHostLeases(ctx, &database))
	jobs := []map[string]any{}
	mergeSupervisorSummary(summary, server.scanRunnableWork(ctx, &database, options, &jobs))
	mergeSupervisorSummary(summary, server.scanRecoverableAttempts(ctx, &database, options, &jobs))
	mergeSupervisorSummary(summary, server.scanWorkspaceCleanup(ctx, &database, workspaceCleanupOptions{AutoCleanupWorkspaces: options.AutoCleanupWorkspaces, WorkspaceRetentionSeconds: options.WorkspaceRetentionSeconds, Limit: options.Limit}))
	if intValue(summary["changed"]) > 0 {
		if currentSavedAt, err := server.Repo.WorkspaceSavedAt(ctx); err == nil && currentSavedAt != "" && loadedSavedAt != "" && currentSavedAt != loadedSavedAt {
			summary["saveSkippedStaleSnapshot"] = 1
			server.logRuntimeDiagnosticFile("DEBUG", "job_supervisor.save.skipped_stale_snapshot", "JobSupervisor skipped saving because the workspace changed during this tick.", map[string]any{"loadedSavedAt": loadedSavedAt, "currentSavedAt": currentSavedAt, "changed": intValue(summary["changed"])})
			return summary, nil
		}
		if err := server.Repo.Save(ctx, database); err != nil {
			return nil, err
		}
		for _, attempt := range database.Tables.Attempts {
			if boolValue(attempt["feishuFailureNotifyPending"]) {
				server.sendFeishuAttemptFailureIfConfigured(ctx, text(attempt, "pipelineId"), text(attempt, "id"))
			}
		}
		for _, pipelineID := range uniqueStrings(stringSlice(summary["feishuReviewPipelines"])) {
			server.sendFeishuReviewForPipelineIfConfigured(ctx, pipelineID)
		}
	}
	for _, job := range jobs {
		server.startDevFlowCycleJob(text(job, "pipelineId"), text(job, "attemptId"), false, false, mapValue(job["lock"]))
	}
	return summary, nil
}

func (server *Server) scanRemoteAttemptSignals(ctx context.Context, database *WorkspaceDatabase, options jobSupervisorTickOptions) map[string]any {
	summary := map[string]any{
		"changed":                  0,
		"checkedRemoteSignals":     0,
		"refreshedRemoteHeartbeat": 0,
		"polledPullRequests":       0,
		"blockedRemoteChecks":      0,
		"remoteSignalFailures":     []map[string]any{},
		"remoteSignalObservations": []map[string]any{},
	}
	if database == nil {
		return summary
	}
	limit := options.Limit
	if limit <= 0 {
		limit = 10
	}
	for index, attempt := range database.Tables.Attempts {
		if intValue(summary["checkedRemoteSignals"]) >= limit {
			break
		}
		status := text(attempt, "status")
		if status != "running" && status != "waiting-human" {
			continue
		}
		summary["checkedRemoteSignals"] = intValue(summary["checkedRemoteSignals"]) + 1
		nextAttempt := cloneMap(attempt)
		changed := false
		worker := mapValue(nextAttempt["workerHost"])
		if text(worker, "kind") != "" && text(worker, "kind") != "local-runtime" {
			workerSeen := firstNonEmpty(text(worker, "lastSeenAt"), text(worker, "updatedAt"))
			if workerSeen != "" && workerSeen > text(nextAttempt, "lastSeenAt") {
				nextAttempt["lastSeenAt"] = workerSeen
				nextAttempt["updatedAt"] = nowISO()
				changed = true
				summary["refreshedRemoteHeartbeat"] = intValue(summary["refreshedRemoteHeartbeat"]) + 1
			}
		}
		prURL := text(nextAttempt, "pullRequestUrl")
		repositoryPath := text(nextAttempt, "repositoryPath")
		if repositoryPath == "" && text(nextAttempt, "workspacePath") != "" {
			repositoryPath = filepath.Join(text(nextAttempt, "workspacePath"), "repo")
		}
		if prURL != "" && repositoryPath != "" {
			pipeline := pipelineByID(*database, text(nextAttempt, "pipelineId"))
			repoSlug := githubRepoSlugForRepositoryTargetID(*database, text(nextAttempt, "repositoryTargetId"))
			checks, rawChecks := githubPullRequestChecks(ctx, repositoryPath, prURL, repoSlug)
			if len(checks) > 0 || strings.TrimSpace(rawChecks) != "" {
				summary["polledPullRequests"] = intValue(summary["polledPullRequests"]) + 1
				checkSummary := githubCheckSummaryWithRequired(checks, workflowRuntimeRequiredChecks(pipeline))
				nextAttempt["lastSeenAt"] = nowISO()
				nextAttempt["remoteSignals"] = map[string]any{
					"pullRequestUrl": prURL,
					"checks":         checks,
					"checkSummary":   checkSummary,
					"observedAt":     nowISO(),
				}
				if intValue(checkSummary["failed"]) > 0 || intValue(checkSummary["missingRequired"]) > 0 {
					summary["blockedRemoteChecks"] = intValue(summary["blockedRemoteChecks"]) + 1
					server.logInfo(ctx, "job_supervisor.remote_signals.blocked", "Remote pull request checks require attention.", map[string]any{"attemptId": text(nextAttempt, "id"), "pipelineId": text(nextAttempt, "pipelineId"), "pullRequestUrl": prURL, "checkSummary": checkSummary})
				} else {
					server.logRuntimeDiagnosticFile("DEBUG", "job_supervisor.remote_signals.polled", "Remote pull request checks polled by JobSupervisor.", map[string]any{"attemptId": text(nextAttempt, "id"), "pipelineId": text(nextAttempt, "pipelineId"), "pullRequestUrl": prURL, "checkSummary": checkSummary})
				}
				summary["remoteSignalObservations"] = append(arrayMaps(summary["remoteSignalObservations"]), map[string]any{
					"attemptId":    text(nextAttempt, "id"),
					"pipelineId":   text(nextAttempt, "pipelineId"),
					"checkSummary": checkSummary,
				})
				changed = true
			}
		}
		if changed {
			nextAttempt["updatedAt"] = nowISO()
			database.Tables.Attempts[index] = nextAttempt
			summary["changed"] = intValue(summary["changed"]) + 1
		}
	}
	return summary
}

func githubRepoSlugForRepositoryTargetID(database WorkspaceDatabase, targetID string) string {
	target := findRepositoryTarget(database, targetID)
	if target == nil {
		return ""
	}
	owner := strings.TrimSpace(text(target, "owner"))
	repo := strings.TrimSpace(text(target, "repo"))
	if owner != "" && repo != "" {
		return owner + "/" + repo
	}
	if text(target, "kind") == "github" {
		label := strings.TrimSpace(repositoryTargetLabel(target))
		if strings.Count(label, "/") == 1 && !strings.HasPrefix(label, "/") {
			return label
		}
	}
	return ""
}

func workflowRuntimeRequiredChecks(pipeline map[string]any) []string {
	runtime := workflowRuntimeFromPipeline(pipeline)
	values := []string{}
	for _, value := range arrayValues(runtime["requiredChecks"]) {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			values = append(values, strings.TrimSpace(text))
		}
	}
	return values
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
		if server.refreshDatabaseWhenAttemptChanged(ctx, database, attempt) {
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
		nextAttempt["feishuFailureNotifyPending"] = true
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
		"manualRecoveryRequired":  0,
		"retryBackoff":            0,
		"retryLimitReached":       0,
		"retrySkippedActive":      0,
		"recoveryClassCounts":     map[string]any{},
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
		recoveryPolicy := supervisorRecoveryPolicyForAttempt(attempt)
		classCounts := mapValue(summary["recoveryClassCounts"])
		classCounts[recoveryPolicy.Class] = intValue(classCounts[recoveryPolicy.Class]) + 1
		summary["recoveryClassCounts"] = classCounts
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
			"recoveryPolicy":      supervisorRecoveryPolicyMap(recoveryPolicy),
		}
		if plan, status := buildAttemptActionPlan(*database, text(attempt, "id")); status == http.StatusOK {
			record["actionPlan"] = supervisorActionPlanSummary(plan)
		}
		if !recoveryPolicy.AutoRetry {
			summary["manualRecoveryRequired"] = intValue(summary["manualRecoveryRequired"]) + 1
			summary["recoverableAttempts"] = append(arrayMaps(summary["recoverableAttempts"]), withSupervisorDecision(record, recoveryPolicy.Action))
			server.logInfo(ctx, "job_supervisor.recovery.manual_action", recoveryPolicy.Label, map[string]any{
				"attemptId":      text(attempt, "id"),
				"pipelineId":     pipelineID,
				"workItemId":     text(attempt, "itemId"),
				"failureClass":   recoveryPolicy.Class,
				"recoveryAction": recoveryPolicy.Action,
			})
			continue
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
		reason := fmt.Sprintf("JobSupervisor retry #%d after %s [%s/%s]: %s", nextIndex, text(attempt, "status"), recoveryPolicy.Class, recoveryPolicy.Action, stringOr(supervisorAttemptFailureReason(attempt), "recoverable attempt"))
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
		accepted["recoveryPolicy"] = supervisorRecoveryPolicyMap(recoveryPolicy)
		if plan, status := buildAttemptActionPlan(updated, text(retryAttempt, "id")); status == http.StatusOK {
			accepted["actionPlan"] = supervisorActionPlanSummary(plan)
		}
		summary["acceptedRetryRuns"] = append(arrayMaps(summary["acceptedRetryRuns"]), accepted)
		*jobs = append(*jobs, accepted)
		server.logInfo(ctx, "job_supervisor.retry.accepted", "Recoverable attempt accepted for retry.", map[string]any{"attemptId": text(retryAttempt, "id"), "retryOfAttemptId": text(attempt, "id"), "pipelineId": text(pipeline, "id"), "workItemId": text(retryAttempt, "itemId"), "retryIndex": intValue(retryAttempt["retryIndex"]), "failureClass": recoveryPolicy.Class, "recoveryAction": recoveryPolicy.Action})
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
		"status":                        "ok",
		"changed":                       0,
		"linkedCheckpoints":             0,
		"backfilledAttempts":            0,
		"checkedCheckpoints":            0,
		"pendingCheckpoints":            0,
		"reconciledCheckpointDecisions": 0,
		"unresolvedGateLinks":           0,
		"checkedAttempts":               0,
		"stalledAttempts":               0,
		"recoveredHumanGates":           0,
		"recoveredProofHumanGates":      0,
		"feishuReviewPipelines":         []any{},
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
		if status, note := reconciledHumanReviewCheckpointDecision(database.Tables.Attempts[attemptIndex], pipeline); status != "" {
			nextCheckpoint := cloneMap(checkpoint)
			nextCheckpoint["status"] = status
			if text(nextCheckpoint, "decisionNote") == "" && note != "" {
				nextCheckpoint["decisionNote"] = note
			}
			if nextAttemptID := text(database.Tables.Attempts[attemptIndex], "id"); nextAttemptID != "" {
				nextCheckpoint["attemptId"] = nextAttemptID
			}
			nextCheckpoint["updatedAt"] = nowISO()
			database.Tables.Checkpoints[index] = nextCheckpoint
			summary["reconciledCheckpointDecisions"] = intValue(summary["reconciledCheckpointDecisions"]) + 1
			summary["changed"] = intValue(summary["changed"]) + 1
			continue
		}
		if server.recoverOrphanedHumanReviewAttempt(ctx, database, pipelineIndex, attemptIndex, checkpoint) {
			summary["recoveredHumanGates"] = intValue(summary["recoveredHumanGates"]) + 1
			summary["feishuReviewPipelines"] = append(arrayValues(summary["feishuReviewPipelines"]), pipelineID)
			summary["changed"] = intValue(summary["changed"]) + 1
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
	proofRecovered, reviewPipelines := server.recoverProofBackedHumanReviewAttempts(ctx, database)
	if proofRecovered > 0 {
		summary["recoveredProofHumanGates"] = proofRecovered
		summary["recoveredHumanGates"] = intValue(summary["recoveredHumanGates"]) + proofRecovered
		summary["changed"] = intValue(summary["changed"]) + proofRecovered
		for _, pipelineID := range reviewPipelines {
			summary["feishuReviewPipelines"] = append(arrayValues(summary["feishuReviewPipelines"]), pipelineID)
		}
	}
	return summary
}

func reconciledHumanReviewCheckpointDecision(attempt map[string]any, pipeline map[string]any) (string, string) {
	if text(attempt, "status") == "done" && text(pipeline, "status") == "done" && pipelineHasRunEvent(pipeline, "gate.approved") {
		return "approved", latestPipelineEventMessage(pipeline, "gate.approved", "approved by human")
	}
	if pipelineHasRunEvent(pipeline, "gate.rejected") {
		return "rejected", latestPipelineEventMessage(pipeline, "gate.rejected", "changes requested")
	}
	return "", ""
}

func pipelineHasRunEvent(pipeline map[string]any, eventType string) bool {
	return latestPipelineEventMessage(pipeline, eventType, "") != ""
}

func latestPipelineEventMessage(pipeline map[string]any, eventType string, fallback string) string {
	events := arrayMaps(mapValue(pipeline["run"])["events"])
	for index := len(events) - 1; index >= 0; index-- {
		if text(events[index], "type") != eventType {
			continue
		}
		if message := text(events[index], "message"); message != "" {
			return message
		}
		return fallback
	}
	return ""
}

func (server *Server) recoverOrphanedHumanReviewAttempt(ctx context.Context, database *WorkspaceDatabase, pipelineIndex int, attemptIndex int, checkpoint map[string]any) bool {
	if database == nil || pipelineIndex < 0 || attemptIndex < 0 || text(checkpoint, "stageId") != "human_review" {
		return false
	}
	attempt := database.Tables.Attempts[attemptIndex]
	if text(attempt, "status") != "stalled" || !attemptHasWorkerOrphanMark(attempt) {
		return false
	}
	nextAttempt := cloneMap(attempt)
	nextAttempt["status"] = "waiting-human"
	nextAttempt["currentStageId"] = "human_review"
	nextAttempt["recoveredAt"] = nowISO()
	nextAttempt["updatedAt"] = text(nextAttempt, "recoveredAt")
	delete(nextAttempt, "statusReason")
	delete(nextAttempt, "stalledAt")
	delete(nextAttempt, "feishuFailureNotifyPending")
	events := arrayMaps(nextAttempt["events"])
	events = append(events, map[string]any{
		"type":      "attempt.worker.orphaned.recovered",
		"message":   "Pending Human Review checkpoint recovered from an orphaned worker mark.",
		"stageId":   "human_review",
		"createdAt": text(nextAttempt, "recoveredAt"),
	})
	nextAttempt["events"] = events
	database.Tables.Attempts[attemptIndex] = nextAttempt

	pipeline := cloneMap(database.Tables.Pipelines[pipelineIndex])
	pipeline = markDevFlowStageProgress(pipeline, "human_review", "needs-human", "Waiting for explicit human approval.")
	pipeline["status"] = "waiting-human"
	run := mapValue(pipeline["run"])
	appendRunEvent(run, "attempt.worker.orphaned.recovered", "Human Review checkpoint restored after stale supervisor state was detected.", "human_review", "job-supervisor")
	pipeline["run"] = run
	pipeline["updatedAt"] = nowISO()
	database.Tables.Pipelines[pipelineIndex] = pipeline

	if item := findWorkItem(*database, text(nextAttempt, "itemId")); item != nil {
		*database = updateWorkItem(*database, text(item, "id"), map[string]any{"status": "In Review", "stageId": "human_review"})
	}
	server.logInfo(ctx, "job_supervisor.human_review.recovered", "Pending Human Review checkpoint recovered from stale worker orphan state.", map[string]any{
		"attemptId":    text(nextAttempt, "id"),
		"pipelineId":   text(nextAttempt, "pipelineId"),
		"workItemId":   text(nextAttempt, "itemId"),
		"checkpointId": text(checkpoint, "id"),
	})
	return true
}

func (server *Server) recoverProofBackedHumanReviewAttempts(ctx context.Context, database *WorkspaceDatabase) (int, []string) {
	if database == nil {
		return 0, nil
	}
	recovered := 0
	reviewPipelines := []string{}
	for attemptIndex, attempt := range database.Tables.Attempts {
		if text(attempt, "status") != "stalled" || !attemptHasWorkerOrphanMark(attempt) {
			continue
		}
		pipelineID := text(attempt, "pipelineId")
		if pipelineHasPendingHumanReviewCheckpoint(*database, pipelineID) {
			continue
		}
		pipelineIndex := findByID(database.Tables.Pipelines, pipelineID)
		if pipelineIndex < 0 {
			continue
		}
		proofDir := proofDirForSupervisorRecovery(*database, attempt)
		if proofDir == "" || !fileExists(filepath.Join(proofDir, "human-review-request.md")) || !fileExists(filepath.Join(proofDir, "handoff-bundle.json")) {
			continue
		}
		var handoff map[string]any
		if raw, err := os.ReadFile(filepath.Join(proofDir, "handoff-bundle.json")); err == nil {
			_ = json.Unmarshal(raw, &handoff)
		}
		if text(handoff, "pipelineId") != "" && text(handoff, "pipelineId") != pipelineID {
			continue
		}
		var reviewPacket map[string]any
		if raw, err := os.ReadFile(filepath.Join(proofDir, "attempt-review-packet.json")); err == nil {
			_ = json.Unmarshal(raw, &reviewPacket)
		}

		pipeline := cloneMap(database.Tables.Pipelines[pipelineIndex])
		pipeline = markDevFlowStageProgress(pipeline, "code_review_round_2", "passed", "Review approved the diff against the requirement.")
		pipeline = markDevFlowStageProgress(pipeline, "human_review", "needs-human", "Waiting for explicit human approval before delivery.")
		run := mapValue(pipeline["run"])
		appendRunEvent(run, "attempt.worker.orphaned.recovered", "Human Review proof was recovered after a stale worker orphan mark.", "human_review", "job-supervisor")
		appendRunEvent(run, "checkpoint.requested", "Human review is required before delivery.", "human_review", "human")
		pipeline["run"] = run
		pipeline["status"] = "waiting-human"
		pipeline["updatedAt"] = nowISO()
		database.Tables.Pipelines[pipelineIndex] = pipeline

		nextAttempt := cloneMap(attempt)
		nextAttempt["status"] = "waiting-human"
		nextAttempt["currentStageId"] = "human_review"
		nextAttempt["workspacePath"] = firstNonEmpty(text(nextAttempt, "workspacePath"), strings.TrimSuffix(proofDir, string(filepath.Separator)+filepath.Join(".omega", "proof")), text(handoff, "workspacePath"))
		nextAttempt["branchName"] = firstNonEmpty(text(nextAttempt, "branchName"), text(handoff, "branchName"), text(reviewPacket, "branchName"))
		nextAttempt["pullRequestUrl"] = firstNonEmpty(text(nextAttempt, "pullRequestUrl"), text(handoff, "pullRequestUrl"), text(reviewPacket, "pullRequestUrl"))
		if len(reviewPacket) > 0 {
			nextAttempt["reviewPacket"] = reviewPacket
		} else if packet := mapValue(handoff["reviewPacket"]); len(packet) > 0 {
			nextAttempt["reviewPacket"] = packet
		}
		if proofFiles, err := collectFiles(proofDir); err == nil {
			nextAttempt["proofFiles"] = proofFiles
		}
		nextAttempt["stages"] = attemptStageSnapshot(pipeline)
		nextAttempt["recoveredAt"] = nowISO()
		nextAttempt["lastSeenAt"] = text(nextAttempt, "recoveredAt")
		nextAttempt["updatedAt"] = text(nextAttempt, "recoveredAt")
		delete(nextAttempt, "statusReason")
		delete(nextAttempt, "stalledAt")
		delete(nextAttempt, "feishuFailureNotifyPending")
		events := arrayMaps(nextAttempt["events"])
		events = append(events, map[string]any{
			"type":      "attempt.worker.orphaned.recovered",
			"message":   "Human Review proof and handoff bundle recovered after stale worker orphan state.",
			"stageId":   "human_review",
			"createdAt": text(nextAttempt, "recoveredAt"),
		})
		nextAttempt["events"] = events
		database.Tables.Attempts[attemptIndex] = nextAttempt

		upsertPendingCheckpoint(database, pipeline)
		if item := findWorkItem(*database, text(nextAttempt, "itemId")); item != nil {
			*database = updateWorkItem(*database, text(item, "id"), map[string]any{"status": "In Review", "stageId": "human_review"})
		}
		upsertRunWorkpad(database, text(nextAttempt, "id"))
		server.logInfo(ctx, "job_supervisor.human_review.proof_recovered", "Human Review checkpoint recovered from proof files after stale worker orphan state.", map[string]any{
			"attemptId":  text(nextAttempt, "id"),
			"pipelineId": pipelineID,
			"workItemId": text(nextAttempt, "itemId"),
			"proofDir":   proofDir,
		})
		recovered++
		reviewPipelines = append(reviewPipelines, pipelineID)
	}
	return recovered, reviewPipelines
}

func attemptHasWorkerOrphanMark(attempt map[string]any) bool {
	if strings.Contains(text(attempt, "statusReason"), "No active local worker host lease") {
		return true
	}
	for _, event := range arrayMaps(attempt["events"]) {
		if text(event, "type") == "attempt.worker.orphaned" {
			return true
		}
		if strings.Contains(text(event, "message"), "No active local worker host lease") {
			return true
		}
	}
	return false
}

func pipelineHasPendingHumanReviewCheckpoint(database WorkspaceDatabase, pipelineID string) bool {
	for _, checkpoint := range database.Tables.Checkpoints {
		if text(checkpoint, "pipelineId") == pipelineID && text(checkpoint, "stageId") == "human_review" && text(checkpoint, "status") == "pending" {
			return true
		}
	}
	return false
}

func proofDirForSupervisorRecovery(database WorkspaceDatabase, attempt map[string]any) string {
	if workspace := text(attempt, "workspacePath"); workspace != "" {
		proofDir := filepath.Join(workspace, ".omega", "proof")
		if fileExists(proofDir) {
			return proofDir
		}
	}
	pipelineID := text(attempt, "pipelineId")
	for index := len(database.Tables.ProofRecords) - 1; index >= 0; index-- {
		proof := database.Tables.ProofRecords[index]
		if !strings.Contains(text(proof, "operationId"), pipelineID) {
			continue
		}
		sourcePath := text(proof, "sourcePath")
		if sourcePath == "" {
			continue
		}
		proofDir := filepath.Dir(sourcePath)
		if fileExists(filepath.Join(proofDir, "human-review-request.md")) {
			return proofDir
		}
	}
	for index := len(database.Tables.Attempts) - 1; index >= 0; index-- {
		other := database.Tables.Attempts[index]
		if text(other, "pipelineId") != pipelineID || text(other, "workspacePath") == "" {
			continue
		}
		proofDir := filepath.Join(text(other, "workspacePath"), ".omega", "proof")
		if fileExists(proofDir) {
			return proofDir
		}
	}
	return ""
}

func fileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
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

func supervisorActionPlanSummary(plan map[string]any) map[string]any {
	currentState := mapValue(plan["currentState"])
	currentAction := mapValue(plan["currentAction"])
	retry := mapValue(plan["retry"])
	return map[string]any{
		"executionMode":      text(plan, "executionMode"),
		"currentStateId":     text(currentState, "id"),
		"currentStateTitle":  text(currentState, "title"),
		"currentStateStatus": text(currentState, "status"),
		"currentActionId":    text(currentAction, "id"),
		"currentActionType":  text(currentAction, "type"),
		"currentActionAgent": text(currentAction, "agent"),
		"currentActionState": text(currentAction, "status"),
		"actionCount":        len(arrayMaps(plan["actions"])),
		"transitionCount":    len(arrayMaps(plan["transitions"])),
		"retryAvailable":     boolValue(retry["available"]),
		"retryAction":        text(retry, "action"),
		"retryReason":        text(retry, "reason"),
	}
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
		if server.refreshDatabaseWhenAttemptChanged(ctx, database, attempt) {
			continue
		}
		nextAttempt := cloneMap(attempt)
		nextAttempt["status"] = "stalled"
		nextAttempt["statusReason"] = fmt.Sprintf("No heartbeat for %d seconds.", int(now.Sub(lastSeen).Seconds()))
		nextAttempt["feishuFailureNotifyPending"] = true
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

func (server *Server) refreshDatabaseWhenAttemptChanged(ctx context.Context, database *WorkspaceDatabase, attempt map[string]any) bool {
	if database == nil || text(attempt, "id") == "" {
		return false
	}
	fresh, err := mustLoad(server, ctx)
	if err != nil {
		return false
	}
	index := findByID(fresh.Tables.Attempts, text(attempt, "id"))
	if index < 0 {
		return false
	}
	freshAttempt := fresh.Tables.Attempts[index]
	if text(freshAttempt, "status") == text(attempt, "status") && text(freshAttempt, "updatedAt") == text(attempt, "updatedAt") {
		return false
	}
	*database = fresh
	return true
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
