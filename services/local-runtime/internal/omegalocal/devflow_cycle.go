package omegalocal

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func beginDevFlowAttempt(database WorkspaceDatabase, pipelineIndex int, item map[string]any, pipeline map[string]any, trigger string) (WorkspaceDatabase, map[string]any, map[string]any) {
	return beginDevFlowAttemptFromStage(database, pipelineIndex, item, pipeline, trigger, "todo")
}

func beginDevFlowAttemptFromStage(database WorkspaceDatabase, pipelineIndex int, item map[string]any, pipeline map[string]any, trigger string, entryStageID string) (WorkspaceDatabase, map[string]any, map[string]any) {
	pipeline = resetDevFlowPipelineForAttemptFromStage(pipeline, entryStageID)
	pipeline["status"] = "running"
	pipeline["updatedAt"] = nowISO()
	database.Tables.Pipelines[pipelineIndex] = pipeline
	database = updateWorkItem(database, text(item, "id"), map[string]any{"status": "In Review"})
	attempt := makeAttemptRecord(item, pipeline, trigger, "devflow-pr", entryStageID)
	runtime := workflowRuntimeFromPipeline(pipeline)
	attempt["continuation"] = map[string]any{
		"maxTurns":    intOrDefault(runtime["maxContinuationTurns"], 1),
		"currentTurn": 1,
		"strategy":    "same-workspace-same-branch",
		"source":      "workflow-runtime",
	}
	database.Tables.Attempts = appendOrReplace(database.Tables.Attempts, attempt)
	upsertRunWorkpad(&database, text(attempt, "id"))
	touch(&database)
	return database, pipeline, attempt
}

func resetDevFlowPipelineForAttempt(pipeline map[string]any) map[string]any {
	return resetDevFlowPipelineForAttemptFromStage(pipeline, "todo")
}

func resetDevFlowPipelineForAttemptFromStage(pipeline map[string]any, entryStageID string) map[string]any {
	entryStageID = stringOr(entryStageID, "todo")
	next := cloneMap(pipeline)
	run := mapValue(next["run"])
	stages := arrayMaps(run["stages"])
	entryFound := false
	for _, stage := range stages {
		if text(stage, "id") == entryStageID {
			entryFound = true
			break
		}
	}
	if !entryFound {
		entryStageID = "todo"
	}
	beforeEntry := true
	for index, stage := range stages {
		stageID := text(stage, "id")
		if stageID == entryStageID {
			stage["status"] = "running"
			stage["startedAt"] = nowISO()
			beforeEntry = false
		} else if beforeEntry && entryStageID != "todo" {
			stage["status"] = "passed"
			stage["completedAt"] = nowISO()
			stage["notes"] = "Skipped by rework assessment."
		} else if index == 0 && entryStageID == "todo" {
			stage["status"] = "running"
			stage["startedAt"] = nowISO()
		} else {
			stage["status"] = "waiting"
			delete(stage, "startedAt")
			delete(stage, "completedAt")
			delete(stage, "notes")
		}
		stage["evidence"] = []any{}
	}
	run["stages"] = stages
	events := arrayMaps(run["events"])
	events = append(events, map[string]any{
		"id":        fmt.Sprintf("event_%d", time.Now().UnixNano()),
		"type":      "attempt.reset",
		"message":   "Pipeline stages reset for a new attempt.",
		"timestamp": nowISO(),
		"stageId":   entryStageID,
		"agentId":   "master",
	})
	run["events"] = events
	next["run"] = run
	next["updatedAt"] = nowISO()
	return next
}

func (server *Server) startDevFlowCycleJob(pipelineID string, attemptID string, autoApproveHuman bool, autoMerge bool, lock map[string]any) {
	server.logInfo(context.Background(), "devflow.job.started", "DevFlow background job started.", map[string]any{
		"entityType": "attempt",
		"entityId":   attemptID,
		"pipelineId": pipelineID,
		"attemptId":  attemptID,
	})
	go func() {
		releaseLock := func(state string) {
			if lock == nil {
				return
			}
			nextLock := cloneMap(lock)
			nextLock["status"] = "released"
			nextLock["runnerProcessState"] = state
			nextLock["releasedAt"] = nowISO()
			nextLock["updatedAt"] = nowISO()
			_ = saveExecutionLock(context.Background(), server, nextLock)
		}

		database, err := mustLoad(server, context.Background())
		if err != nil {
			server.logError(context.Background(), "devflow.job.load_failed", err.Error(), map[string]any{"pipelineId": pipelineID, "attemptId": attemptID})
			_ = server.failDevFlowCycleJob(context.Background(), pipelineID, attemptID, err)
			releaseLock("failed")
			return
		}
		pipelineIndex := findByID(database.Tables.Pipelines, pipelineID)
		if pipelineIndex < 0 {
			_ = server.failDevFlowCycleJob(context.Background(), pipelineID, attemptID, fmt.Errorf("pipeline not found"))
			releaseLock("failed")
			return
		}
		pipeline := cloneMap(database.Tables.Pipelines[pipelineIndex])
		template := findPipelineTemplate(text(pipeline, "templateId"))
		runContext, cancelRun := context.WithTimeout(context.Background(), devFlowAttemptTimeout(template))
		defer cancelRun()
		server.registerAttemptJob(attemptID, cancelRun)
		defer server.unregisterAttemptJob(attemptID)
		workerHost := localWorkerHostRecord(attemptID)
		if lock != nil {
			lock["workerHost"] = workerHost
			lock["runnerProcessState"] = "running"
			lock["updatedAt"] = nowISO()
			_ = saveExecutionLock(context.Background(), server, lock)
		}
		server.markAttemptWorkerHost(context.Background(), pipelineID, attemptID, workerHost)
		item := findWorkItem(database, text(pipeline, "workItemId"))
		if item == nil {
			_ = server.failDevFlowCycleJob(context.Background(), pipelineID, attemptID, fmt.Errorf("work item not found"))
			releaseLock("failed")
			return
		}
		stageItem, err := resolveWorkItemRepositoryTarget(database, item)
		if err != nil {
			_ = server.failDevFlowCycleJob(context.Background(), pipelineID, attemptID, err)
			releaseLock("failed")
			return
		}
		target := findRepositoryTarget(database, text(stageItem, "repositoryTargetId"))
		if target == nil {
			_ = server.failDevFlowCycleJob(context.Background(), pipelineID, attemptID, fmt.Errorf("work item %s has no repository workspace", text(stageItem, "key")))
			releaseLock("failed")
			return
		}
		result, err := server.executeDevFlowPRCycle(runContext, pipeline, stageItem, target, attemptID, autoApproveHuman, autoMerge)
		if err != nil {
			if runContext.Err() != nil {
				server.logInfo(context.Background(), "devflow.job.canceled", runContext.Err().Error(), map[string]any{"pipelineId": pipelineID, "attemptId": attemptID, "workItemId": text(stageItem, "id")})
				_ = server.cancelDevFlowCycleJob(context.Background(), pipelineID, attemptID, runContext.Err().Error())
				releaseLock("canceled")
				return
			}
			server.logError(context.Background(), "devflow.job.failed", err.Error(), map[string]any{"pipelineId": pipelineID, "attemptId": attemptID, "workItemId": text(stageItem, "id")})
			_ = server.failDevFlowCycleJobWithResult(context.Background(), pipelineID, attemptID, err, result)
			releaseLock("failed")
			return
		}
		if _, _, err := server.completeDevFlowCycleJob(context.Background(), pipelineID, attemptID, result); err != nil {
			server.logError(context.Background(), "devflow.job.complete_failed", err.Error(), map[string]any{"pipelineId": pipelineID, "attemptId": attemptID, "workItemId": text(stageItem, "id")})
			_ = server.failDevFlowCycleJob(context.Background(), pipelineID, attemptID, err)
			releaseLock("failed")
			return
		}
		server.logInfo(context.Background(), "devflow.job.completed", "DevFlow background job completed.", map[string]any{
			"entityType": "attempt",
			"entityId":   attemptID,
			"pipelineId": pipelineID,
			"attemptId":  attemptID,
			"workItemId": text(stageItem, "id"),
			"status":     text(result, "status"),
		})
		releaseLock("completed")
	}()
}

func (server *Server) failDevFlowCycleJob(ctx context.Context, pipelineID string, attemptID string, failure error) error {
	return server.failDevFlowCycleJobWithResult(ctx, pipelineID, attemptID, failure, nil)
}

func (server *Server) cancelDevFlowCycleJob(ctx context.Context, pipelineID string, attemptID string, reason string) error {
	database, err := mustLoad(server, ctx)
	if err != nil {
		return err
	}
	database, _ = markAttemptCanceled(database, attemptID, stringOr(reason, "Attempt canceled."))
	touch(&database)
	return server.Repo.Save(context.Background(), database)
}

func (server *Server) failDevFlowCycleJobWithResult(ctx context.Context, pipelineID string, attemptID string, failure error, result map[string]any) error {
	database, err := mustLoad(server, ctx)
	if err != nil {
		return err
	}
	pipelineIndex := findByID(database.Tables.Pipelines, pipelineID)
	if pipelineIndex < 0 {
		return fmt.Errorf("pipeline not found")
	}
	pipeline := cloneMap(database.Tables.Pipelines[pipelineIndex])
	pipeline["status"] = "failed"
	pipeline["updatedAt"] = nowISO()
	database.Tables.Pipelines[pipelineIndex] = pipeline
	if item := findWorkItem(database, text(pipeline, "workItemId")); item != nil {
		database = updateWorkItem(database, text(item, "id"), map[string]any{"status": "Blocked"})
	}
	if findByID(database.Tables.Attempts, attemptID) < 0 {
		if item := findWorkItem(database, text(pipeline, "workItemId")); item != nil {
			server.logError(ctx, "devflow.attempt.missing_backfilled", "Missing attempt was backfilled during failure handling.", map[string]any{"pipelineId": pipelineID, "attemptId": attemptID, "workItemId": text(item, "id")})
			attempt := makeAttemptRecord(item, pipeline, "manual", "devflow-pr", firstRunnableStageID(pipeline))
			attempt["id"] = attemptID
			database.Tables.Attempts = appendOrReplace(database.Tables.Attempts, attempt)
		}
	}
	database, _ = failAttemptRecord(database, attemptID, pipeline, failure.Error(), result)
	upsertRunWorkpad(&database, attemptID)
	touch(&database)
	return server.Repo.Save(context.Background(), database)
}

func (server *Server) completeDevFlowCycleJob(ctx context.Context, pipelineID string, attemptID string, result map[string]any) (map[string]any, map[string]any, error) {
	database, err := mustLoad(server, ctx)
	if err != nil {
		return nil, nil, err
	}
	pipelineIndex := findByID(database.Tables.Pipelines, pipelineID)
	if pipelineIndex < 0 {
		return nil, nil, fmt.Errorf("pipeline not found")
	}
	pipeline := cloneMap(database.Tables.Pipelines[pipelineIndex])
	item := findWorkItem(database, text(pipeline, "workItemId"))
	if item == nil {
		return nil, nil, fmt.Errorf("work item not found")
	}
	if findByID(database.Tables.Attempts, attemptID) < 0 {
		server.logError(ctx, "devflow.attempt.missing_backfilled", "Missing attempt was backfilled before applying job result.", map[string]any{"pipelineId": pipelineID, "attemptId": attemptID, "workItemId": text(item, "id"), "status": text(result, "status")})
		attempt := makeAttemptRecord(item, pipeline, "manual", "devflow-pr", firstRunnableStageID(pipeline))
		attempt["id"] = attemptID
		attempt["workspacePath"] = stringOr(result["workspacePath"], text(attempt, "workspacePath"))
		attempt["branchName"] = stringOr(result["branchName"], text(attempt, "branchName"))
		attempt["pullRequestUrl"] = stringOr(result["pullRequestUrl"], text(attempt, "pullRequestUrl"))
		database.Tables.Attempts = appendOrReplace(database.Tables.Attempts, attempt)
	}
	database, pipeline, item = applyDevFlowCycleResult(database, pipelineIndex, item, result)
	database, _ = completeAttemptRecord(database, attemptID, pipeline, result)
	upsertRunWorkpad(&database, attemptID)
	touch(&database)
	if err := server.Repo.Save(context.Background(), database); err != nil {
		return nil, nil, err
	}
	if text(result, "status") == "waiting-human" {
		server.sendFeishuReviewForPipelineIfConfigured(ctx, text(pipeline, "id"))
	}
	return pipeline, item, nil
}

func applyDevFlowCycleResult(database WorkspaceDatabase, pipelineIndex int, item map[string]any, result map[string]any) (WorkspaceDatabase, map[string]any, map[string]any) {
	pipeline := cloneMap(database.Tables.Pipelines[pipelineIndex])
	run := mapValue(pipeline["run"])
	if workflow := mapValue(result["workflow"]); len(workflow) > 0 {
		run["workflow"] = workflow
	}
	stages := arrayMaps(run["stages"])
	resultStatus := stringOr(result["status"], "done")
	evidenceByStage := map[string][]string{}
	stageStatusByStage := map[string]string{}
	for _, invocation := range arrayMaps(result["agentInvocations"]) {
		stageID := text(invocation, "stageId")
		if stageID == "" {
			continue
		}
		evidenceByStage[stageID] = append(evidenceByStage[stageID], stringSlice(invocation["proofFiles"])...)
		status := text(invocation, "status")
		if status != "" {
			stageStatusByStage[stageID] = status
		}
	}
	for _, stage := range stages {
		stageID := text(stage, "id")
		switch {
		case resultStatus == "waiting-human" && stageID == "human_review":
			stage["status"] = "needs-human"
			stage["notes"] = "Review agents passed. Human approval is required before delivery."
			stage["evidence"] = evidenceByStage[stageID]
		case resultStatus == "waiting-human" && (stageID == "merging" || stageID == "done"):
			stage["status"] = "waiting"
			stage["evidence"] = evidenceByStage[stageID]
		default:
			if stageStatusByStage[stageID] == "failed" {
				stage["status"] = "failed"
			} else {
				stage["status"] = "passed"
				stage["completedAt"] = nowISO()
			}
			stage["evidence"] = proofFilesForStage(result, stageID)
		}
	}
	run["stages"] = stages
	if resultStatus == "waiting-human" {
		appendRunEvent(run, "checkpoint.requested", "Human review is required before delivery.", "human_review", "human")
	} else {
		appendRunEvent(run, "devflow.cycle.completed", "DevFlow PR cycle completed", "delivery", "delivery")
	}
	pipeline["run"] = run
	if resultStatus == "waiting-human" {
		pipeline["status"] = "waiting-human"
	} else {
		pipeline["status"] = "done"
	}
	pipeline["updatedAt"] = nowISO()
	database.Tables.Pipelines[pipelineIndex] = pipeline
	if resultStatus == "waiting-human" {
		database = updateWorkItem(database, text(item, "id"), map[string]any{"status": "In Review"})
		upsertPendingCheckpoint(&database, pipeline)
	} else {
		database = updateWorkItem(database, text(item, "id"), map[string]any{"status": "Done"})
	}
	updatedItem := item
	if found := findWorkItem(database, text(item, "id")); found != nil {
		updatedItem = found
	}
	proofFiles := stringSlice(result["proofFiles"])
	for proofIndex, proof := range proofFiles {
		database.Tables.ProofRecords = appendOrReplace(database.Tables.ProofRecords, map[string]any{
			"id":          fmt.Sprintf("%s:devflow-proof:%d", text(pipeline, "id"), proofIndex+1),
			"operationId": fmt.Sprintf("%s:devflow-cycle", text(pipeline, "id")),
			"label":       "devflow-cycle-proof",
			"value":       filepath.Base(proof),
			"sourcePath":  proof,
			"createdAt":   nowISO(),
		})
	}
	database = appendAgentInvocationRecords(database, pipeline, item, result)
	touch(&database)
	return database, pipeline, updatedItem
}

func proofFilesForStage(result map[string]any, stageID string) []string {
	files := []string{}
	for _, invocation := range arrayMaps(result["agentInvocations"]) {
		if text(invocation, "stageId") != stageID {
			continue
		}
		files = append(files, stringSlice(invocation["proofFiles"])...)
	}
	if len(files) > 0 {
		return files
	}
	return stringSlice(result["proofFiles"])
}

func markDevFlowStageProgress(pipeline map[string]any, stageID string, status string, note string) map[string]any {
	next := cloneMap(pipeline)
	run := mapValue(next["run"])
	stages := arrayMaps(run["stages"])
	timestamp := nowISO()
	for _, stage := range stages {
		if text(stage, "id") != stageID {
			continue
		}
		stage["status"] = status
		stage["updatedAt"] = timestamp
		if status == "running" && text(stage, "startedAt") == "" {
			stage["startedAt"] = timestamp
		}
		if status == "passed" || status == "failed" {
			stage["completedAt"] = timestamp
		}
		if note != "" {
			stage["notes"] = note
		}
	}
	run["stages"] = stages
	next["run"] = run
	next["updatedAt"] = timestamp
	return next
}

func devFlowNextStageAfter(stageID string) string {
	switch stageID {
	case "todo":
		return "in_progress"
	case "in_progress":
		return "code_review_round_1"
	case "code_review_round_1":
		return "code_review_round_2"
	case "code_review_round_2":
		return "human_review"
	case "rework":
		return "code_review_round_1"
	case "human_review":
		return "merging"
	case "merging":
		return "done"
	default:
		return ""
	}
}

func devFlowNextStageAfterFromWorkflow(workflow map[string]any, stageID string) string {
	for _, transition := range arrayMaps(workflow["transitions"]) {
		if text(transition, "from") == stageID && text(transition, "on") == "passed" && text(transition, "to") != "" {
			return text(transition, "to")
		}
	}
	return devFlowNextStageAfter(stageID)
}

func devFlowStageStatusAfterInvocation(stageID string, agentID string, status string) (string, string) {
	return devFlowStageStatusAfterInvocationWithWorkflow(nil, stageID, agentID, status)
}

func devFlowStageStatusAfterInvocationWithWorkflow(workflow map[string]any, stageID string, agentID string, status string) (string, string) {
	if status == "running" {
		return "running", ""
	}
	if status == "failed" {
		return "failed", ""
	}
	if status == "changes-requested" {
		return "passed", devFlowTransitionFromWorkflow(workflow, stageID, "changes_requested", "rework")
	}
	if status == "needs-human" || status == "waiting-human" {
		return "needs-human", ""
	}
	if status != "passed" && status != "done" {
		return "running", ""
	}
	if stageID == "in_progress" && agentID != "testing" {
		return "running", ""
	}
	return "passed", devFlowNextStageAfterFromWorkflow(workflow, stageID)
}

func devFlowTransitionFromWorkflow(workflow map[string]any, from string, event string, fallback string) string {
	for _, transition := range arrayMaps(workflow["transitions"]) {
		if text(transition, "from") == from && text(transition, "on") == event && text(transition, "to") != "" {
			return text(transition, "to")
		}
	}
	return fallback
}

func (server *Server) persistDevFlowAgentInvocation(ctx context.Context, pipelineID string, itemID string, attemptID string, invocation map[string]any) error {
	if ctx == nil {
		ctx = context.Background()
	}
	database, err := server.Repo.Load(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		defaultDatabase := defaultWorkspaceDatabase()
		database = &defaultDatabase
	} else if err != nil {
		return err
	}
	pipelineIndex := findByID(database.Tables.Pipelines, pipelineID)
	if pipelineIndex < 0 {
		return nil
	}
	pipeline := cloneMap(database.Tables.Pipelines[pipelineIndex])
	workflow := mapValue(mapValue(pipeline["run"])["workflow"])
	stageStatus, nextStageID := devFlowStageStatusAfterInvocationWithWorkflow(workflow, text(invocation, "stageId"), text(invocation, "agentId"), text(invocation, "status"))
	pipeline = markDevFlowStageProgress(pipeline, text(invocation, "stageId"), stageStatus, text(invocation, "summary"))
	run := mapValue(pipeline["run"])
	if nextStageID != "" {
		pipeline = markDevFlowStageProgress(pipeline, nextStageID, "running", "Queued by local orchestrator.")
		run = mapValue(pipeline["run"])
	}
	if text(invocation, "status") == "failed" {
		pipeline["status"] = "failed"
		database.Tables.Pipelines[pipelineIndex] = pipeline
		databaseValue := updateWorkItem(*database, itemID, map[string]any{"status": "Blocked"})
		*database = databaseValue
	} else {
		pipeline["status"] = "running"
		database.Tables.Pipelines[pipelineIndex] = pipeline
		databaseValue := updateWorkItem(*database, itemID, map[string]any{"status": "In Review"})
		*database = databaseValue
	}
	appendRunEvent(run, "agent."+text(invocation, "status"), text(invocation, "summary"), text(invocation, "stageId"), text(invocation, "agentId"))
	pipeline["run"] = run
	pipeline["updatedAt"] = nowISO()
	database.Tables.Pipelines[pipelineIndex] = pipeline

	if attemptIndex := findByID(database.Tables.Attempts, attemptID); attemptIndex >= 0 {
		attempt := cloneMap(database.Tables.Attempts[attemptIndex])
		if text(invocation, "status") == "failed" {
			attempt["status"] = "failed"
			attempt["errorMessage"] = text(invocation, "summary")
			attempt["failureReason"] = text(invocation, "summary")
			attempt["failureStageId"] = text(invocation, "stageId")
			attempt["failureAgentId"] = text(invocation, "agentId")
		} else {
			attempt["status"] = "running"
		}
		attempt["currentStageId"] = text(invocation, "stageId")
		attempt["stages"] = attemptStageSnapshot(pipeline)
		attempt["lastSeenAt"] = nowISO()
		attempt["updatedAt"] = text(attempt, "lastSeenAt")
		events := arrayMaps(attempt["events"])
		events = append(events, map[string]any{
			"type":      "agent." + text(invocation, "status"),
			"message":   stringOr(text(invocation, "summary"), text(invocation, "agentId")+" "+text(invocation, "status")),
			"stageId":   text(invocation, "stageId"),
			"createdAt": nowISO(),
		})
		attempt["events"] = events
		database.Tables.Attempts[attemptIndex] = attempt
	} else if item := findWorkItem(*database, itemID); item != nil {
		server.logError(ctx, "devflow.attempt.missing_backfilled", "Missing attempt was backfilled during agent invocation persistence.", map[string]any{"pipelineId": pipelineID, "attemptId": attemptID, "workItemId": itemID, "stageId": text(invocation, "stageId"), "agentId": text(invocation, "agentId")})
		attempt := makeAttemptRecord(item, pipeline, "manual", "devflow-pr", text(invocation, "stageId"))
		attempt["id"] = attemptID
		attempt["status"] = "running"
		attempt["currentStageId"] = text(invocation, "stageId")
		attempt["stages"] = attemptStageSnapshot(pipeline)
		attempt["lastSeenAt"] = nowISO()
		attempt["updatedAt"] = text(attempt, "lastSeenAt")
		database.Tables.Attempts = appendOrReplace(database.Tables.Attempts, attempt)
	}
	upsertRunWorkpad(database, attemptID)
	operationID := stringOr(text(invocation, "operationId"), text(invocation, "id"))
	level := "INFO"
	if text(invocation, "status") == "failed" {
		level = "ERROR"
	}
	server.logRuntime(ctx, level, "agent.invocation."+text(invocation, "status"), stringOr(text(invocation, "summary"), "Agent invocation persisted."), map[string]any{
		"entityType": "operation",
		"entityId":   operationID,
		"pipelineId": pipelineID,
		"attemptId":  attemptID,
		"workItemId": itemID,
		"stageId":    text(invocation, "stageId"),
		"agentId":    text(invocation, "agentId"),
		"runner":     text(mapValue(invocation["process"]), "runner"),
		"status":     text(invocation, "status"),
	})

	missionID := fmt.Sprintf("mission_%s_agent_workflow", pipelineID)
	if item := findWorkItem(*database, itemID); item != nil {
		database.Tables.Missions = appendOrReplace(database.Tables.Missions, map[string]any{
			"id":         missionID,
			"pipelineId": pipelineID,
			"workItemId": itemID,
			"title":      item["title"],
			"status":     pipeline["status"],
			"mission": map[string]any{
				"id":                 missionID,
				"sourceWorkItemId":   itemID,
				"sourceIssueKey":     text(item, "key"),
				"title":              item["title"],
				"repositoryTargetId": text(item, "repositoryTargetId"),
			},
			"createdAt": nowISO(),
			"updatedAt": nowISO(),
		})
	}
	database.Tables.Operations = appendOrReplace(database.Tables.Operations, map[string]any{
		"id":            operationID,
		"missionId":     missionID,
		"stageId":       invocation["stageId"],
		"agentId":       invocation["agentId"],
		"status":        invocation["status"],
		"prompt":        invocation["prompt"],
		"requiredProof": []any{"agent-log", "artifact"},
		"runnerProcess": invocation["process"],
		"summary":       invocation["summary"],
		"createdAt":     stringOr(text(invocation, "startedAt"), nowISO()),
		"updatedAt":     stringOr(text(invocation, "finishedAt"), nowISO()),
	})
	for proofIndex, proof := range stringSlice(invocation["proofFiles"]) {
		database.Tables.ProofRecords = appendOrReplace(database.Tables.ProofRecords, map[string]any{
			"id":          fmt.Sprintf("%s:agent-proof:%d", operationID, proofIndex+1),
			"operationId": operationID,
			"label":       text(invocation, "agentId") + "-artifact",
			"value":       filepath.Base(proof),
			"sourcePath":  proof,
			"createdAt":   nowISO(),
		})
	}
	touch(database)
	return server.Repo.Save(context.Background(), *database)
}

func (server *Server) runnerHeartbeatRecorder(pipelineID string, itemID string, attemptID string, stageID string, agentID string, runnerID string) func(SupervisedCommandEvent) {
	var mu sync.Mutex
	return func(event SupervisedCommandEvent) {
		mu.Lock()
		defer mu.Unlock()
		server.recordAttemptRunnerHeartbeat(context.Background(), pipelineID, itemID, attemptID, stageID, agentID, runnerID, event)
	}
}

func (server *Server) recordAttemptRunnerHeartbeat(ctx context.Context, pipelineID string, itemID string, attemptID string, stageID string, agentID string, runnerID string, event SupervisedCommandEvent) {
	if strings.TrimSpace(attemptID) == "" {
		return
	}
	timestamp := stringOr(event.CreatedAt, nowISO())
	database, err := mustLoad(server, ctx)
	if err == nil {
		if attemptIndex := findByID(database.Tables.Attempts, attemptID); attemptIndex >= 0 {
			attempt := cloneMap(database.Tables.Attempts[attemptIndex])
			attempt["lastSeenAt"] = timestamp
			attempt["updatedAt"] = timestamp
			events := arrayMaps(attempt["events"])
			message := "Runner heartbeat."
			if event.Stream != "heartbeat" && strings.TrimSpace(event.Chunk) != "" {
				message = fmt.Sprintf("%s output: %s", event.Stream, truncateForProof(strings.TrimSpace(event.Chunk), 240))
			}
			events = append(events, map[string]any{
				"type":      "runner." + event.Stream,
				"message":   message,
				"stageId":   stageID,
				"agentId":   agentID,
				"createdAt": timestamp,
			})
			if len(events) > 200 {
				events = events[len(events)-200:]
			}
			attempt["events"] = events
			database.Tables.Attempts[attemptIndex] = attempt
			_ = server.Repo.Save(ctx, database)
		}
	}
	message := "Runner heartbeat."
	if event.Stream != "heartbeat" && strings.TrimSpace(event.Chunk) != "" {
		message = truncateForProof(strings.TrimSpace(event.Chunk), 500)
	}
	server.logDebug(ctx, "runner."+event.Stream, message, map[string]any{
		"entityType": "attempt",
		"entityId":   attemptID,
		"pipelineId": pipelineID,
		"attemptId":  attemptID,
		"workItemId": itemID,
		"stageId":    stageID,
		"agentId":    agentID,
		"runner":     runnerID,
		"stream":     event.Stream,
		"chunk":      truncateForProof(event.Chunk, 2000),
	})
}

func appendAgentInvocationRecords(database WorkspaceDatabase, pipeline map[string]any, item map[string]any, result map[string]any) WorkspaceDatabase {
	invocations := arrayMaps(result["agentInvocations"])
	if len(invocations) == 0 {
		return database
	}
	timestamp := nowISO()
	missionID := fmt.Sprintf("mission_%s_agent_workflow", text(pipeline, "id"))
	database.Tables.Missions = appendOrReplace(database.Tables.Missions, map[string]any{
		"id":         missionID,
		"pipelineId": text(pipeline, "id"),
		"workItemId": text(item, "id"),
		"title":      text(item, "title"),
		"status":     result["status"],
		"mission": map[string]any{
			"id":                 missionID,
			"sourceWorkItemId":   text(item, "id"),
			"sourceIssueKey":     text(item, "key"),
			"title":              text(item, "title"),
			"workspacePath":      result["workspacePath"],
			"repositoryPath":     result["repositoryPath"],
			"agentInvocations":   invocations,
			"repositoryTargetId": text(item, "repositoryTargetId"),
		},
		"createdAt": timestamp,
		"updatedAt": timestamp,
	})
	for _, invocation := range invocations {
		operationID := text(invocation, "operationId")
		if operationID == "" {
			operationID = text(invocation, "id")
		}
		database.Tables.Operations = appendOrReplace(database.Tables.Operations, map[string]any{
			"id":            operationID,
			"missionId":     missionID,
			"stageId":       invocation["stageId"],
			"agentId":       invocation["agentId"],
			"status":        invocation["status"],
			"prompt":        invocation["prompt"],
			"requiredProof": []any{"agent-log", "artifact"},
			"runnerProcess": invocation["process"],
			"summary":       invocation["summary"],
			"createdAt":     stringOr(invocation["startedAt"], timestamp),
			"updatedAt":     stringOr(invocation["finishedAt"], timestamp),
		})
		for proofIndex, proof := range stringSlice(invocation["proofFiles"]) {
			database.Tables.ProofRecords = appendOrReplace(database.Tables.ProofRecords, map[string]any{
				"id":          fmt.Sprintf("%s:agent-proof:%d", operationID, proofIndex+1),
				"operationId": operationID,
				"label":       text(invocation, "agentId") + "-artifact",
				"value":       filepath.Base(proof),
				"sourcePath":  proof,
				"createdAt":   timestamp,
			})
		}
	}
	return database
}

func makeAttemptRecord(item map[string]any, pipeline map[string]any, trigger string, runner string, currentStageID string) map[string]any {
	timestamp := nowISO()
	if currentStageID == "" {
		currentStageID = firstRunnableStageID(pipeline)
	}
	return map[string]any{
		"id":                 fmt.Sprintf("%s:attempt:%d", text(pipeline, "id"), time.Now().UnixNano()),
		"itemId":             text(item, "id"),
		"pipelineId":         text(pipeline, "id"),
		"repositoryTargetId": text(item, "repositoryTargetId"),
		"status":             "running",
		"trigger":            trigger,
		"runner":             runner,
		"currentStageId":     currentStageID,
		"startedAt":          timestamp,
		"lastSeenAt":         timestamp,
		"stages":             attemptStageSnapshot(pipeline),
		"events": []map[string]any{{
			"type":      "attempt.started",
			"message":   "Pipeline attempt started.",
			"stageId":   currentStageID,
			"createdAt": timestamp,
		}},
		"createdAt": timestamp,
		"updatedAt": timestamp,
	}
}

func completeAttemptRecord(database WorkspaceDatabase, attemptID string, pipeline map[string]any, result map[string]any) (WorkspaceDatabase, map[string]any) {
	index := findByID(database.Tables.Attempts, attemptID)
	if index < 0 {
		return database, nil
	}
	timestamp := nowISO()
	attempt := cloneMap(database.Tables.Attempts[index])
	status := stringOr(result["status"], text(pipeline, "status"))
	if status == "completed" {
		status = "done"
	}
	attempt["status"] = status
	attempt["currentStageId"] = firstRunnableStageID(pipeline)
	attempt["workspacePath"] = stringOr(result["workspacePath"], text(attempt, "workspacePath"))
	attempt["branchName"] = stringOr(result["branchName"], text(attempt, "branchName"))
	attempt["pullRequestUrl"] = stringOr(result["pullRequestUrl"], text(attempt, "pullRequestUrl"))
	if feedback := arrayMaps(result["pullRequestFeedback"]); len(feedback) > 0 {
		attempt["pullRequestFeedback"] = feedback
	}
	if feedback := arrayMaps(result["checkLogFeedback"]); len(feedback) > 0 {
		attempt["checkLogFeedback"] = feedback
	}
	if reports := arrayMaps(result["githubOutboundSync"]); len(reports) > 0 {
		attempt["githubOutboundSync"] = append(arrayMaps(attempt["githubOutboundSync"]), reports...)
	}
	if reviewPacket := mapValue(result["reviewPacket"]); len(reviewPacket) > 0 {
		attempt["reviewPacket"] = reviewPacket
	}
	attempt["stdoutSummary"] = truncateForProof(stringOr(result["stdout"], ""), 800)
	attempt["stderrSummary"] = truncateForProof(stringOr(result["stderr"], ""), 800)
	attempt["stages"] = attemptStageSnapshot(pipeline)
	attempt["finishedAt"] = timestamp
	attempt["lastSeenAt"] = timestamp
	attempt["durationMs"] = durationSinceMillis(text(attempt, "startedAt"), timestamp)
	attempt["updatedAt"] = timestamp
	events := arrayMaps(attempt["events"])
	events = append(events, map[string]any{
		"type":      "attempt.completed",
		"message":   fmt.Sprintf("Pipeline attempt finished with %s.", status),
		"stageId":   attempt["currentStageId"],
		"createdAt": timestamp,
	})
	attempt["events"] = events
	database.Tables.Attempts[index] = attempt
	upsertRunWorkpad(&database, attemptID)
	return database, attempt
}

func failAttemptRecord(database WorkspaceDatabase, attemptID string, pipeline map[string]any, message string, result map[string]any) (WorkspaceDatabase, map[string]any) {
	index := findByID(database.Tables.Attempts, attemptID)
	if index < 0 {
		return database, nil
	}
	timestamp := nowISO()
	attempt := cloneMap(database.Tables.Attempts[index])
	attempt["status"] = "failed"
	attempt["currentStageId"] = firstRunnableStageID(pipeline)
	attempt["errorMessage"] = message
	if result != nil {
		if stageID := text(result, "failureStageId"); stageID != "" {
			attempt["currentStageId"] = stageID
			attempt["failureStageId"] = stageID
		}
		if agentID := text(result, "failureAgentId"); agentID != "" {
			attempt["failureAgentId"] = agentID
		}
		if reason := text(result, "failureReason"); reason != "" {
			attempt["failureReason"] = reason
		}
		if detail := text(result, "failureDetail"); detail != "" {
			attempt["failureDetail"] = truncateForProof(detail, 1200)
		}
		if reviewFeedback := text(result, "failureReviewFeedback"); reviewFeedback != "" {
			attempt["failureReviewFeedback"] = truncateForProof(reviewFeedback, 1600)
		}
		attempt["workspacePath"] = stringOr(result["workspacePath"], text(attempt, "workspacePath"))
		attempt["branchName"] = stringOr(result["branchName"], text(attempt, "branchName"))
		attempt["pullRequestUrl"] = stringOr(result["pullRequestUrl"], text(attempt, "pullRequestUrl"))
		if feedback := arrayMaps(result["pullRequestFeedback"]); len(feedback) > 0 {
			attempt["pullRequestFeedback"] = feedback
		}
		if feedback := arrayMaps(result["checkLogFeedback"]); len(feedback) > 0 {
			attempt["checkLogFeedback"] = feedback
		}
		if reports := arrayMaps(result["githubOutboundSync"]); len(reports) > 0 {
			attempt["githubOutboundSync"] = append(arrayMaps(attempt["githubOutboundSync"]), reports...)
		}
		if reviewPacket := mapValue(result["reviewPacket"]); len(reviewPacket) > 0 {
			attempt["reviewPacket"] = reviewPacket
		}
		attempt["stdoutSummary"] = truncateForProof(stringOr(result["stdout"], ""), 800)
	}
	attempt["stderrSummary"] = truncateForProof(message, 800)
	attempt["stages"] = attemptStageSnapshot(pipeline)
	attempt["finishedAt"] = timestamp
	attempt["lastSeenAt"] = timestamp
	attempt["durationMs"] = durationSinceMillis(text(attempt, "startedAt"), timestamp)
	attempt["updatedAt"] = timestamp
	events := arrayMaps(attempt["events"])
	events = append(events, map[string]any{
		"type":      "attempt.failed",
		"message":   stringOr(message, "Pipeline attempt failed."),
		"stageId":   attempt["currentStageId"],
		"createdAt": timestamp,
	})
	attempt["events"] = events
	attempt["reworkChecklist"] = buildReworkChecklist(database, pipeline, attempt)
	database.Tables.Attempts[index] = attempt
	return database, attempt
}

func markAttemptCanceled(database WorkspaceDatabase, attemptID string, reason string) (WorkspaceDatabase, map[string]any) {
	index := findByID(database.Tables.Attempts, attemptID)
	if index < 0 {
		return database, nil
	}
	timestamp := nowISO()
	attempt := cloneMap(database.Tables.Attempts[index])
	if text(attempt, "status") == "done" || text(attempt, "status") == "canceled" {
		return database, attempt
	}
	attempt["status"] = "canceled"
	attempt["statusReason"] = reason
	attempt["finishedAt"] = timestamp
	attempt["lastSeenAt"] = timestamp
	attempt["durationMs"] = durationSinceMillis(text(attempt, "startedAt"), timestamp)
	attempt["updatedAt"] = timestamp
	events := arrayMaps(attempt["events"])
	events = append(events, map[string]any{
		"type":      "attempt.canceled",
		"message":   reason,
		"stageId":   text(attempt, "currentStageId"),
		"createdAt": timestamp,
	})
	attempt["events"] = events
	database.Tables.Attempts[index] = attempt
	if pipelineIndex := findByID(database.Tables.Pipelines, text(attempt, "pipelineId")); pipelineIndex >= 0 {
		pipeline := cloneMap(database.Tables.Pipelines[pipelineIndex])
		run := mapValue(pipeline["run"])
		appendRunEvent(run, "attempt.canceled", reason, text(attempt, "currentStageId"), "operator")
		pipeline["run"] = run
		pipeline["status"] = "canceled"
		pipeline["updatedAt"] = timestamp
		database.Tables.Pipelines[pipelineIndex] = pipeline
		attempt["reworkChecklist"] = buildReworkChecklist(database, pipeline, attempt)
		database.Tables.Attempts[index] = attempt
	}
	if item := findWorkItem(database, text(attempt, "itemId")); item != nil {
		database = updateWorkItem(database, text(item, "id"), map[string]any{"status": "Blocked"})
	}
	for checkpointIndex, checkpoint := range database.Tables.Checkpoints {
		if text(checkpoint, "attemptId") == attemptID && text(checkpoint, "status") == "pending" {
			nextCheckpoint := cloneMap(checkpoint)
			nextCheckpoint["status"] = "canceled"
			nextCheckpoint["decisionNote"] = reason
			nextCheckpoint["updatedAt"] = timestamp
			database.Tables.Checkpoints[checkpointIndex] = nextCheckpoint
		}
	}
	upsertRunWorkpad(&database, attemptID)
	touch(&database)
	return database, attempt
}

func attemptStageSnapshot(pipeline map[string]any) []map[string]any {
	stages := arrayMaps(mapValue(pipeline["run"])["stages"])
	output := make([]map[string]any, 0, len(stages))
	for _, stage := range stages {
		output = append(output, map[string]any{
			"id":              text(stage, "id"),
			"title":           text(stage, "title"),
			"status":          text(stage, "status"),
			"agentIds":        stringSlice(stage["agentIds"]),
			"inputArtifacts":  stringSlice(stage["inputArtifacts"]),
			"outputArtifacts": stringSlice(stage["outputArtifacts"]),
			"startedAt":       text(stage, "startedAt"),
			"completedAt":     text(stage, "completedAt"),
			"evidence":        stringSlice(stage["evidence"]),
		})
	}
	return output
}

func firstRunnableStageID(pipeline map[string]any) string {
	for _, stage := range arrayMaps(mapValue(pipeline["run"])["stages"]) {
		status := text(stage, "status")
		if status == "running" || status == "ready" || status == "needs-human" || status == "failed" {
			return text(stage, "id")
		}
	}
	return ""
}

func durationSinceMillis(startedAt string, finishedAt string) int {
	started, err := time.Parse(time.RFC3339Nano, startedAt)
	if err != nil {
		return 0
	}
	finished, err := time.Parse(time.RFC3339Nano, finishedAt)
	if err != nil {
		return 0
	}
	return int(finished.Sub(started).Milliseconds())
}

func devFlowMaxReviewCycles(template *PipelineTemplate) int {
	if template != nil && template.Runtime.MaxReviewCycles > 0 {
		return template.Runtime.MaxReviewCycles
	}
	return 3
}

func devFlowRunnerHeartbeatInterval(template *PipelineTemplate) time.Duration {
	if template != nil && template.Runtime.RunnerHeartbeatSeconds > 0 {
		return time.Duration(template.Runtime.RunnerHeartbeatSeconds) * time.Second
	}
	return 10 * time.Second
}

func devFlowAttemptTimeout(template *PipelineTemplate) time.Duration {
	if template != nil && template.Runtime.AttemptTimeoutMinutes > 0 {
		return time.Duration(template.Runtime.AttemptTimeoutMinutes) * time.Minute
	}
	return 30 * time.Minute
}

func devFlowTransitionTo(template *PipelineTemplate, from string, event string, fallback string) string {
	if template != nil {
		for _, transition := range template.Transitions {
			if transition.From == from && transition.On == event && transition.To != "" {
				return transition.To
			}
		}
	}
	return fallback
}

func ensureDevFlowRepositoryWorkspace(workspace string, repoWorkspace string, cloneTarget string, branchName string, baseBranch string) (string, error) {
	if _, err := os.Stat(filepath.Join(repoWorkspace, ".git")); err != nil {
		if output, cloneErr := cloneTargetRepository(workspace, cloneTarget, repoWorkspace); cloneErr != nil {
			return output, cloneErr
		}
	}
	_, _ = runCommand(repoWorkspace, "git", "fetch", "origin")
	if _, err := runCommand(repoWorkspace, "git", "checkout", branchName); err != nil {
		if _, remoteBranchErr := runCommand(repoWorkspace, "git", "rev-parse", "--verify", "origin/"+branchName); remoteBranchErr == nil {
			if _, checkoutErr := runCommand(repoWorkspace, "git", "checkout", "-B", branchName, "origin/"+branchName); checkoutErr != nil {
				return "", checkoutErr
			}
		} else {
			if _, branchErr := runCommand(repoWorkspace, "git", "checkout", "-B", branchName, "origin/"+baseBranch); branchErr != nil {
				if _, fallbackErr := runCommand(repoWorkspace, "git", "checkout", "-B", branchName); fallbackErr != nil {
					return "", fallbackErr
				}
			}
		}
	}
	_, _ = runCommand(repoWorkspace, "git", "config", "user.email", "omega-devflow@example.local")
	_, _ = runCommand(repoWorkspace, "git", "config", "user.name", "Omega DevFlow Runner")
	return "", nil
}

func ensureDevFlowPullRequest(repoWorkspace string, repoSlug string, branchName string, baseBranch string, title string, body string) (string, error) {
	existing, _ := runCommand(repoWorkspace, "gh", "pr", "list", "--repo", repoSlug, "--head", branchName, "--json", "url", "--jq", ".[0].url")
	if existing = strings.TrimSpace(existing); strings.HasPrefix(existing, "http") {
		return existing, nil
	}
	prURL, err := runCommand(repoWorkspace, "gh", "pr", "create", "--repo", repoSlug, "--head", branchName, "--base", baseBranch, "--title", title, "--body", body)
	if err != nil {
		fallback, _ := runCommand(repoWorkspace, "gh", "pr", "list", "--repo", repoSlug, "--head", branchName, "--json", "url", "--jq", ".[0].url")
		if fallback = strings.TrimSpace(fallback); strings.HasPrefix(fallback, "http") {
			return fallback, nil
		}
		return "", err
	}
	prURL = strings.TrimSpace(prURL)
	if prURL == "" {
		prURL = fmt.Sprintf("https://github.com/%s/pull/unknown", repoSlug)
	}
	return prURL, nil
}

func updateDevFlowPullRequestDescriptionIfChanged(repoWorkspace string, prURL string, title string, body string) (string, error) {
	current, _ := runCommand(repoWorkspace, "gh", "pr", "view", prURL, "--json", "body", "--jq", ".body")
	if strings.TrimSpace(current) == strings.TrimSpace(body) {
		return "pull request description already current", nil
	}
	return runCommand(repoWorkspace, "gh", "pr", "edit", prURL, "--title", title, "--body", body)
}

func buildDevFlowPullRequestBody(item map[string]any, changedFiles []string, testOutput string, humanChangeRequest string, incrementalDiff string) string {
	body := fmt.Sprintf("## Omega DevFlow Cycle\n\n### Work item\n- %s %s\n\n", text(item, "key"), text(item, "title"))
	if strings.TrimSpace(humanChangeRequest) != "" {
		body += fmt.Sprintf("### Human requested changes\n%s\n\n", strings.TrimSpace(humanChangeRequest))
	}
	body += fmt.Sprintf("### Changed\n%s\n", markdownFileList(changedFiles))
	if strings.TrimSpace(humanChangeRequest) != "" && strings.TrimSpace(incrementalDiff) != "" {
		body += fmt.Sprintf("\n### Rework diff since previous review\n```diff\n%s\n```\n", truncateForProof(incrementalDiff, 5000))
	}
	body += fmt.Sprintf("\n### Validation\n```text\n%s\n```\n", truncateForProof(testOutput, 2000))
	return body
}

func pushDevFlowBranch(repoWorkspace string, branchName string) error {
	if _, err := runCommand(repoWorkspace, "git", "push", "--set-upstream", "origin", branchName); err != nil {
		_, _ = runCommand(repoWorkspace, "git", "pull", "--rebase", "origin", branchName)
		if _, retryErr := runCommand(repoWorkspace, "git", "push", "--set-upstream", "origin", branchName); retryErr != nil {
			return retryErr
		}
	}
	return nil
}

func (server *Server) executeDevFlowPRCycle(ctx context.Context, pipeline map[string]any, item map[string]any, target map[string]any, attemptID string, autoApproveHuman bool, autoMerge bool) (map[string]any, error) {
	attempt := map[string]any{"id": attemptID}
	workspaceRoot, workspace, repoWorkspace, err := devFlowWorkspacePaths(ctx, server, item)
	if err != nil {
		return nil, err
	}
	server.WorkspaceRoot = workspaceRoot
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return nil, err
	}
	cloneTarget := repositoryTargetCloneTarget(target)
	if cloneTarget == "" {
		return nil, fmt.Errorf("repository target %s has no clone target", text(target, "id"))
	}
	repoSlug := repositoryTargetLabel(target)
	baseBranch := stringOr(text(target, "defaultBranch"), "main")
	currentAttempt := map[string]any{}
	profile := server.resolveAgentProfile(ctx, WorkspaceDatabase{}, item, target)
	if database, err := server.Repo.Load(ctx); err == nil {
		profile = server.resolveAgentProfile(ctx, *database, item, target)
		if attemptIndex := findByID(database.Tables.Attempts, attemptID); attemptIndex >= 0 {
			currentAttempt = cloneMap(database.Tables.Attempts[attemptIndex])
		}
	}
	branchName := stringOr(text(currentAttempt, "branchName"), devFlowRunBranchName(text(item, "key")))
	template := findPipelineTemplate(text(pipeline, "templateId"))
	if profileTemplate, validation, exists := loadProfileWorkflowTemplate(profile, text(pipeline, "templateId")); exists {
		if !validation.ok() {
			return map[string]any{"status": "failed", "workspacePath": workspace, "repositoryPath": repoWorkspace}, fmt.Errorf("profile workflow contract invalid: %s", strings.Join(validation.Errors, "; "))
		}
		template = &profileTemplate
		pipeline = applyWorkflowTemplateToPipeline(pipeline, profileTemplate)
	}
	if output, err := ensureDevFlowRepositoryWorkspace(workspace, repoWorkspace, cloneTarget, branchName, baseBranch); err != nil {
		return map[string]any{"status": "failed", "workspacePath": workspace, "stdout": output}, fmt.Errorf("clone target repository: %w", err)
	}
	if repoTemplate, validation, exists := loadRepositoryWorkflowTemplate(repoWorkspace, text(pipeline, "templateId")); exists {
		if !validation.ok() {
			return map[string]any{"status": "failed", "workspacePath": workspace, "repositoryPath": repoWorkspace}, fmt.Errorf("repository workflow contract invalid: %s", strings.Join(validation.Errors, "; "))
		}
		template = &repoTemplate
		pipeline = applyWorkflowTemplateToPipeline(pipeline, repoTemplate)
		server.logInfo(ctx, "workflow_contract.repository.loaded", "Repository-owned workflow contract loaded.", map[string]any{"pipelineId": text(pipeline, "id"), "workItemId": text(item, "id"), "source": repoTemplate.Source})
	}
	if err := writeRunnerPolicyFiles(repoWorkspace, profile, "coding"); err != nil {
		return nil, err
	}
	runnerRegistry := NewAgentRunnerRegistry()
	codingProfile := agentProfileForRole(profile, "coding")
	codingRunner, codingRunnerID := runnerRegistry.Resolve(codingProfile.Runner)
	reviewProfile := agentProfileForRole(profile, "review")
	reviewRunner, reviewRunnerID := runnerRegistry.Resolve(reviewProfile.Runner)

	proofDir := filepath.Join(workspace, ".omega", "proof")
	if err := os.MkdirAll(proofDir, 0o755); err != nil {
		return nil, err
	}
	if err := writeAgentRuntimeSpec(filepath.Join(workspace, ".omega", "agent-runtime.json"), map[string]any{
		"runner":           "devflow-pr",
		"agentId":          "master",
		"pipelineId":       text(pipeline, "id"),
		"workItemId":       text(item, "id"),
		"workspaceRoot":    workspaceRoot,
		"workspacePath":    workspace,
		"repositoryTarget": repositoryTargetLabel(target),
		"agentProfile":     agentRuntimeMetadata(profile, "master"),
	}); err != nil {
		return nil, err
	}
	if lifecycle, err := devFlowWorkspaceLifecycleSpec(ctx, server, item, target, pipeline, attempt); err != nil {
		return nil, err
	} else if err := writeJSONFile(filepath.Join(workspace, ".omega", "workspace-lifecycle.json"), lifecycle); err != nil {
		return nil, err
	}
	if err := writeRunnerPolicyFiles(workspace, profile, "master"); err != nil {
		return nil, err
	}
	runnerHeartbeatInterval := devFlowRunnerHeartbeatInterval(template)
	agentInvocations := []map[string]any{}
	stageArtifacts := []map[string]any{}
	pullRequestFeedback := []map[string]any{}
	checkLogFeedback := []map[string]any{}
	githubOutboundSync := []map[string]any{}
	combinedFeedback := func(primary string) string {
		parts := []string{}
		if strings.TrimSpace(primary) != "" {
			parts = append(parts, strings.TrimSpace(primary))
		}
		if feedback := githubPullRequestFeedbackPrompt(pullRequestFeedback); strings.TrimSpace(feedback) != "" {
			parts = append(parts, "PR feedback:\n"+strings.TrimSpace(feedback))
		}
		if feedback := githubPullRequestFeedbackPrompt(checkLogFeedback); strings.TrimSpace(feedback) != "" {
			parts = append(parts, "CI/check failure logs:\n"+strings.TrimSpace(feedback))
		}
		return strings.Join(parts, "\n\n")
	}
	recordAgent := func(stageID string, agentID string, status string, prompt string, artifact string, summary string, proofFiles []string, process map[string]any) {
		startedAt := nowISO()
		finishedAt := nowISO()
		invocationID := fmt.Sprintf("%s:agent:%03d:%s:%s", text(pipeline, "id"), len(agentInvocations)+1, stageID, agentID)
		invocation := map[string]any{
			"id":          invocationID,
			"stageId":     stageID,
			"agentId":     agentID,
			"status":      status,
			"prompt":      prompt,
			"artifact":    artifact,
			"summary":     summary,
			"proofFiles":  proofFiles,
			"process":     process,
			"startedAt":   startedAt,
			"finishedAt":  finishedAt,
			"operationId": invocationID,
		}
		agentInvocations = append(agentInvocations, invocation)
		if artifact != "" {
			stageArtifacts = append(stageArtifacts, map[string]any{"stageId": stageID, "agentId": agentID, "artifact": artifact})
		}
		_ = server.persistDevFlowAgentInvocation(context.Background(), text(pipeline, "id"), text(item, "id"), attemptID, invocation)
	}
	recordGitHubOutboundSync := func(event string, status string, stageID string, summary string, prURL string, checksOutput string, changedFiles []string, failureReason string, failureDetail string, reviewPacket map[string]any) {
		report := server.syncGitHubIssueOutbound(ctx, githubOutboundSyncInput{
			RepositoryPath:      repoWorkspace,
			Repository:          repoSlug,
			WorkItem:            item,
			Pipeline:            pipeline,
			Attempt:             currentAttempt,
			AttemptID:           attemptID,
			Event:               event,
			Status:              status,
			StageID:             stageID,
			Summary:             summary,
			PullRequestURL:      prURL,
			BranchName:          branchName,
			ChangedFiles:        changedFiles,
			ChecksOutput:        checksOutput,
			PullRequestFeedback: pullRequestFeedback,
			CheckLogFeedback:    checkLogFeedback,
			ReviewPacket:        reviewPacket,
			FailureReason:       failureReason,
			FailureDetail:       failureDetail,
		})
		if text(report, "state") == "skipped" {
			return
		}
		githubOutboundSync = append(githubOutboundSync, report)
		artifact := fmt.Sprintf("github-outbound-sync-%03d.json", len(githubOutboundSync))
		artifactPath := filepath.Join(proofDir, artifact)
		if err := writeJSONFile(artifactPath, report); err == nil {
			stageArtifacts = append(stageArtifacts, map[string]any{"stageId": stringOr(stageID, "delivery"), "agentId": "github", "artifact": artifact})
		}
	}
	failureResult := func(stageID string, agentID string, reason string, detail string, reviewFeedback string) map[string]any {
		recordGitHubOutboundSync("attempt.failed", "failed", stageID, reason, "", "", nil, reason, detail, nil)
		result := map[string]any{
			"status":                "failed",
			"workspacePath":         workspace,
			"repositoryPath":        repoWorkspace,
			"agentInvocations":      agentInvocations,
			"stageArtifacts":        stageArtifacts,
			"failureStageId":        stageID,
			"failureAgentId":        agentID,
			"failureReason":         reason,
			"failureDetail":         detail,
			"failureReviewFeedback": reviewFeedback,
		}
		if len(pullRequestFeedback) > 0 {
			result["pullRequestFeedback"] = pullRequestFeedback
		}
		if len(checkLogFeedback) > 0 {
			result["checkLogFeedback"] = checkLogFeedback
		}
		if len(githubOutboundSync) > 0 {
			result["githubOutboundSync"] = githubOutboundSync
		}
		return result
	}
	recordGitHubOutboundSync("attempt.started", "running", "todo", "Pipeline attempt started and repository workspace is ready.", "", "", nil, "", "", nil)

	humanChangeRequest := latestHumanChangeRequestFromPipeline(pipeline)
	reworkFeedbackInput := reworkChecklistPromptFromAttempt(currentAttempt, humanChangeRequest)
	effectiveDescription := text(item, "description")
	if reworkFeedbackInput != "" {
		effectiveDescription = strings.TrimSpace(effectiveDescription + "\n\nRework input:\n" + reworkFeedbackInput)
	}
	promptVariables := map[string]string{
		"repository":     repoSlug,
		"repositoryPath": repoWorkspace,
		"workItemKey":    text(item, "key"),
		"title":          text(item, "title"),
		"description":    effectiveDescription,
		"branchName":     branchName,
		"proofDir":       proofDir,
		"codingNotePath": filepath.Join(proofDir, "coding-agent-note.md"),
		"reworkNotePath": "",
		"pullRequestUrl": "",
		"reviewFeedback": reworkFeedbackInput,
		"changedFiles":   "",
		"testOutput":     "",
		"checksOutput":   "",
		"reviewFocus":    "",
	}

	reworkAssessment := mapValue(currentAttempt["reworkAssessment"])
	if text(currentAttempt, "trigger") == "human-request-changes" && text(reworkAssessment, "strategy") == reworkStrategyFastRework {
		prURL := text(currentAttempt, "pullRequestUrl")
		if prURL == "" {
			reason := "Fast rework requires the previous pull request URL."
			return failureResult("rework", "master", reason, "previous attempt did not record pullRequestUrl", humanChangeRequest), errors.New("fast rework missing previous pull request")
		}
		assessmentPath := filepath.Join(proofDir, "rework-assessment.md")
		if err := os.WriteFile(assessmentPath, []byte(reworkAssessmentMarkdown(reworkAssessment)), 0o644); err != nil {
			return nil, err
		}
		recordAgent("rework", "master", "passed", "Assess whether human-requested changes need fast rework or replanning.", "rework-assessment.md", text(reworkAssessment, "rationale"), []string{assessmentPath}, map[string]any{"runner": "local-orchestrator", "status": "passed", "strategy": text(reworkAssessment, "strategy")})

		previousHead, _ := runCommand(repoWorkspace, "git", "rev-parse", "HEAD")
		notePath := filepath.Join(proofDir, "human-rework-agent-note.md")
		promptPath := filepath.Join(proofDir, "human-rework-prompt.md")
		reworkVariables := cloneStringMap(promptVariables)
		reworkVariables["pullRequestUrl"] = prURL
		reworkVariables["reviewFeedback"] = reworkFeedbackInput
		reworkVariables["reworkNotePath"] = notePath
		reworkFallback := fmt.Sprintf(`You are the rework coding agent for Omega.

Repository: %s
Repository path: %s
Work item: %s
Title: %s
Pull request: %s

Requirement:
%s

Rework checklist to address:
%s

Rules:
- Continue in the same repository checkout, same branch, and same pull request.
- Treat this as a focused rework on top of the previously reviewed version.
- Address the human feedback with a real code change.
- Keep the diff minimal and reviewable.
- Do not commit, push, or create a pull request. Omega will handle git delivery after you finish editing.
- Write a short completion note to %s with these sections:
  - Human feedback addressed
  - What changed
  - Files changed
  - Validation run
  - Remaining risk
`, repoSlug, repoWorkspace, text(item, "key"), text(item, "title"), prURL, effectiveDescription, reworkFeedbackInput, notePath)
		reworkPrompt := renderWorkflowPromptSection(template, "rework", reworkVariables, reworkFallback) + "\n\n" + agentPolicyBlock(profile, "coding")
		if err := os.WriteFile(promptPath, []byte(reworkPrompt), 0o644); err != nil {
			return nil, err
		}
		recordAgent("rework", "coding", "running", reworkPrompt, "", "Fast rework agent is applying human feedback on the existing PR branch.", []string{promptPath}, map[string]any{"runner": codingRunnerID, "status": "running", "strategy": text(reworkAssessment, "strategy")})
		reworkTurn := codingRunner.RunTurn(ctx, AgentTurnRequest{
			Role:              "rework",
			StageID:           "rework",
			Runner:            codingRunnerID,
			Workspace:         repoWorkspace,
			Prompt:            reworkPrompt,
			OutputPath:        notePath,
			Sandbox:           "workspace-write",
			Model:             codingProfile.Model,
			HeartbeatInterval: runnerHeartbeatInterval,
			OnProcessEvent:    server.runnerHeartbeatRecorder(text(pipeline, "id"), text(item, "id"), attemptID, "rework", "coding", codingRunnerID),
		})
		if reworkTurn.Error != nil {
			reason := "Fast rework agent failed before producing an acceptable repository diff."
			recordAgent("rework", "coding", "failed", reworkPrompt, filepath.Base(notePath), reason, []string{promptPath, notePath}, reworkTurn.Process)
			return failureResult("rework", "coding", reason, reworkTurn.Error.Error(), humanChangeRequest), reworkTurn.Error
		}
		statusOutput, err := runCommand(repoWorkspace, "git", "status", "--short")
		if err != nil {
			return nil, fmt.Errorf("read fast rework changes: %w", err)
		}
		if strings.TrimSpace(statusOutput) == "" {
			reason := "Fast rework completed but produced no repository changes."
			recordAgent("rework", "coding", "failed", reworkPrompt, filepath.Base(notePath), reason, []string{promptPath, notePath}, reworkTurn.Process)
			return failureResult("rework", "coding", reason, "git status --short returned no changed files after fast rework.", humanChangeRequest), errors.New("fast rework produced no repository changes")
		}
		if _, err := runCommand(repoWorkspace, "git", "add", "-A"); err != nil {
			return nil, fmt.Errorf("stage fast rework changes: %w", err)
		}
		if _, err := runCommand(repoWorkspace, "git", "commit", "-m", "Omega human rework for "+text(item, "key")); err != nil {
			return nil, fmt.Errorf("commit fast rework changes: %w", err)
		}
		commitSha, _ := runCommand(repoWorkspace, "git", "rev-parse", "HEAD")
		commitSummary, _ := runCommand(repoWorkspace, "git", "show", "--stat", "--oneline", "--no-renames", "HEAD")
		diffRange := strings.TrimSpace(previousHead) + "..HEAD"
		diffText, _ := runCommand(repoWorkspace, "git", "diff", diffRange)
		changedNames, err := runCommand(repoWorkspace, "git", "diff", "--name-only", diffRange)
		if err != nil || strings.TrimSpace(changedNames) == "" {
			diffText, _ = runCommand(repoWorkspace, "git", "diff", "HEAD~1..HEAD")
			changedNames, err = runCommand(repoWorkspace, "git", "diff", "--name-only", "HEAD~1..HEAD")
		}
		if err != nil {
			return nil, fmt.Errorf("list fast rework changed files: %w", err)
		}
		changedFiles := compactLines(changedNames)
		diffPath := filepath.Join(proofDir, "git-diff-human-rework.patch")
		if err := os.WriteFile(diffPath, []byte(diffText), 0o644); err != nil {
			return nil, err
		}
		reworkSummaryPath := filepath.Join(proofDir, "human-rework-summary.md")
		reworkSummary := fmt.Sprintf("# Human Rework\n\n- Strategy: `%s`\n- Branch: `%s`\n- Commit: `%s`\n- Human feedback: %s\n- Changed files:\n%s\n```text\n%s\n```\n", text(reworkAssessment, "strategy"), branchName, strings.TrimSpace(commitSha), humanChangeRequest, markdownFileList(changedFiles), truncateForProof(commitSummary, 4000))
		if err := os.WriteFile(reworkSummaryPath, []byte(reworkSummary), 0o644); err != nil {
			return nil, err
		}
		recordAgent("rework", "coding", "passed", reworkPrompt, filepath.Base(reworkSummaryPath), fmt.Sprintf("Fast rework produced %d changed file(s).", len(changedFiles)), []string{promptPath, notePath, reworkSummaryPath, diffPath}, reworkTurn.Process)

		testOutput, testErr := runRepositoryValidation(repoWorkspace)
		testStatus := "passed"
		if testErr != nil {
			testStatus = "failed"
		}
		testReportPath := filepath.Join(proofDir, "test-report-human-rework.md")
		testVariables := cloneStringMap(promptVariables)
		testVariables["changedFiles"] = strings.Join(changedFiles, ", ")
		testVariables["testOutput"] = testOutput
		testPrompt := renderWorkflowPromptSection(template, "testing", testVariables, fmt.Sprintf("Validate %s after human-requested fast rework. Changed files: %s", text(item, "key"), strings.Join(changedFiles, ", ")))
		testReport := fmt.Sprintf("# Human Rework Test Report\n\nStatus: %s\n\n## Commands\n\n```text\n%s\n```\n\n## Acceptance coverage\n\n- Validation was run against the repository after human-requested fast rework.\n\n## Failures\n\n%s\n\n## Residual risk\n\n- Project-specific coverage depends on available repository test commands.\n", testStatus, stringOr(strings.TrimSpace(testOutput), "No validation output."), stringOr(testFailureSummary(testErr, testOutput), "None"))
		if err := os.WriteFile(testReportPath, []byte(testReport), 0o644); err != nil {
			return nil, err
		}
		recordAgent("rework", "testing", testStatus, testPrompt, filepath.Base(testReportPath), "Repository validation completed after human-requested fast rework.", []string{testReportPath}, map[string]any{"runner": "local-validation", "status": testStatus, "stdout": testOutput})
		if testErr != nil {
			reason := "Repository validation failed after human-requested fast rework."
			return failureResult("rework", "testing", reason, stringOr(testOutput, testErr.Error()), humanChangeRequest), fmt.Errorf("repository validation failed after fast rework: %w", testErr)
		}
		if err := pushDevFlowBranch(repoWorkspace, branchName); err != nil {
			return nil, fmt.Errorf("push fast rework branch: %w", err)
		}
		prTitle := text(item, "key") + " " + text(item, "title")
		prBody := buildDevFlowPullRequestBody(item, changedFiles, testOutput, humanChangeRequest, diffText)
		if output, err := updateDevFlowPullRequestDescriptionIfChanged(repoWorkspace, prURL, prTitle, prBody); err != nil {
			server.logDebug(ctx, "github.pr.description_update_skipped", "Pull request description update skipped after fast rework.", map[string]any{"pipelineId": text(pipeline, "id"), "attemptId": attemptID, "pullRequestUrl": prURL, "output": truncateForProof(output+"\n"+err.Error(), 1200)})
		} else {
			server.logInfo(ctx, "github.pr.description_updated", "Pull request description updated after fast rework.", map[string]any{"pipelineId": text(pipeline, "id"), "attemptId": attemptID, "pullRequestUrl": prURL, "output": truncateForProof(output, 1200)})
		}
		prDiff, _ := runCommand(repoWorkspace, "gh", "pr", "diff", prURL)
		checksOutput, _ := runCommand(repoWorkspace, "gh", "pr", "checks", prURL)
		pullRequestFeedback = githubPullRequestFeedback(ctx, repoWorkspace, prURL, repoSlug)
		checkLogFeedback = githubPullRequestCheckLogFeedback(ctx, repoWorkspace, prURL, repoSlug, nil)

		reviewRounds := defaultDevFlowReviewRounds()
		if template != nil && len(template.ReviewRounds) > 0 {
			reviewRounds = template.ReviewRounds
		}
		reviewApproved := true
		reviewFeedback := ""
		for _, reviewRound := range reviewRounds {
			stageID := stringOr(reviewRound.StageID, "code_review")
			artifact := stringOr(reviewRound.Artifact, stageID+".md")
			reviewPath := filepath.Join(proofDir, strings.TrimSuffix(artifact, filepath.Ext(artifact))+"-human-rework"+filepath.Ext(artifact))
			reviewDiff := diffText
			reviewChecks := ""
			if reviewRound.DiffSource == "pr_diff" {
				reviewDiff = prDiff
				reviewChecks = checksOutput
			}
			reviewVariables := cloneStringMap(promptVariables)
			reviewVariables["pullRequestUrl"] = prURL
			reviewVariables["changedFiles"] = strings.Join(changedFiles, ", ")
			reviewVariables["testOutput"] = testOutput
			reviewVariables["checksOutput"] = reviewChecks
			reviewVariables["reviewFocus"] = reviewRound.Focus
			reviewVariables["reviewFeedback"] = combinedFeedback(reworkFeedbackInput)
			reviewVariables["diff"] = reviewDiff
			reviewFallback := buildDevFlowReviewPrompt(item, repoSlug, prURL, changedFiles, reviewDiff, testOutput, reviewChecks, reviewRound.Focus, combinedFeedback(reworkFeedbackInput))
			reviewPrompt := renderWorkflowPromptSection(template, "review", reviewVariables, reviewFallback) + "\n\n" + agentPolicyBlock(profile, "review")
			reviewTurn := reviewRunner.RunTurn(ctx, AgentTurnRequest{
				Role:              "review",
				StageID:           stageID,
				Runner:            reviewRunnerID,
				Workspace:         repoWorkspace,
				Prompt:            reviewPrompt,
				OutputPath:        reviewPath,
				Sandbox:           "read-only",
				Model:             reviewProfile.Model,
				Effort:            "medium",
				HeartbeatInterval: runnerHeartbeatInterval,
				OnProcessEvent:    server.runnerHeartbeatRecorder(text(pipeline, "id"), text(item, "id"), attemptID, stageID, "review", reviewRunnerID),
			})
			if reviewTurn.Error != nil {
				reason := "Review agent failed while checking human-requested fast rework."
				recordAgent(stageID, "review", "failed", reviewPrompt, filepath.Base(reviewPath), reason, []string{reviewPath}, reviewTurn.Process)
				return failureResult(stageID, "review", reason, reviewTurn.Error.Error(), humanChangeRequest), fmt.Errorf("%s failed: %w", stageID, reviewTurn.Error)
			}
			outcome := devFlowReviewOutcome(reviewPath)
			if outcome.Verdict == "approved" {
				recordAgent(stageID, "review", "passed", reviewPrompt, filepath.Base(reviewPath), outcome.Summary, []string{reviewPath}, reviewTurn.Process)
				continue
			}
			status := "changes-requested"
			if outcome.Verdict == "needs_human_info" {
				status = "needs-human"
			}
			recordAgent(stageID, "review", status, reviewPrompt, filepath.Base(reviewPath), outcome.Summary, []string{reviewPath}, reviewTurn.Process)
			reviewApproved = false
			reviewFeedback = outcome.Summary
			break
		}
		if !reviewApproved && reviewFeedback != "" {
			recordAgent("human_review", "human", "waiting-human", reviewFeedback, "human-review-request.md", "Review did not approve the human-requested fast rework. Human input is required.", []string{}, map[string]any{"runner": "human", "status": "waiting-human"})
		}

		stageArtifacts = append(stageArtifacts, map[string]any{"stageId": "rework", "agentId": "master", "artifact": filepath.Base(assessmentPath)})
		humanReviewRequestPath := filepath.Join(proofDir, "human-review-request.md")
		humanReviewRequest := fmt.Sprintf("# Human Review Request\n\n- Work item: `%s` %s\n- Repository: `%s`\n- Pull request: %s\n- Rework strategy: `%s`\n- Human feedback: %s\n- Changed files:\n%s\n\n## Delivery state\n\n- Waiting for human approval after fast rework.\n\n## Proof\n\n- Human rework summary\n- Test report\n- Review reports\n- Pull request\n\n## Operator notes\n\nReview the PR and the increment since the previous reviewed version. Approve to continue delivery or request changes again.\n", text(item, "key"), text(item, "title"), repoSlug, prURL, text(reworkAssessment, "strategy"), humanChangeRequest, markdownFileList(changedFiles))
		if err := os.WriteFile(humanReviewRequestPath, []byte(humanReviewRequest), 0o644); err != nil {
			return nil, err
		}
		recordAgent("human_review", "human", "waiting-human", "Review the human-requested rework, PR diff, proof, and agent verdicts. Approve to continue delivery or request changes to send the run back.", "human-review-request.md", "Waiting for explicit human approval after fast rework.", []string{humanReviewRequestPath}, map[string]any{"runner": "human", "status": "waiting-human"})

		reportInput := devFlowRunReportInput{
			Item:                item,
			Repository:          repoSlug,
			BranchName:          branchName,
			PullRequestURL:      prURL,
			ChangedFiles:        changedFiles,
			DiffText:            stringOr(prDiff, diffText),
			TestOutput:          testOutput,
			ChecksOutput:        checksOutput,
			PullRequestFeedback: pullRequestFeedback,
			CheckLogFeedback:    checkLogFeedback,
			StageArtifacts:      stageArtifacts,
			AgentInvocations:    agentInvocations,
		}
		reviewPacket, reviewPacketPath, err := writeDevFlowReviewPacket(proofDir, reportInput)
		if err != nil {
			return nil, err
		}
		stageArtifacts = append(stageArtifacts, map[string]any{"stageId": "human_review", "agentId": "delivery", "artifact": filepath.Base(reviewPacketPath)})
		reportInput.ReviewPacket = reviewPacket
		if reportPath, err := writeDevFlowRunReport(proofDir, reportInput); err != nil {
			return nil, err
		} else {
			stageArtifacts = append(stageArtifacts, map[string]any{"stageId": "human_review", "agentId": "delivery", "artifact": filepath.Base(reportPath)})
		}
		deliveryPrompt := renderWorkflowPromptSection(template, "delivery", map[string]string{
			"repository":     repoSlug,
			"repositoryPath": repoWorkspace,
			"workItemKey":    text(item, "key"),
			"title":          text(item, "title"),
			"description":    effectiveDescription,
			"pullRequestUrl": prURL,
			"changedFiles":   strings.Join(changedFiles, ", "),
			"testOutput":     testOutput,
			"checksOutput":   checksOutput,
		}, fmt.Sprintf("Assemble delivery handoff for %s after fast rework. PR: %s.", text(item, "key"), prURL))
		stageArtifacts = append(stageArtifacts, map[string]any{"stageId": "done", "agentId": "delivery", "artifact": "handoff-bundle.json"})
		if err := writeJSONFile(filepath.Join(proofDir, "handoff-bundle.json"), map[string]any{
			"pipelineId":         text(pipeline, "id"),
			"workItemId":         text(item, "id"),
			"workItemKey":        text(item, "key"),
			"repositoryTargetId": text(target, "id"),
			"repositoryTarget":   repoSlug,
			"workspacePath":      workspace,
			"repositoryPath":     repoWorkspace,
			"branchName":         branchName,
			"pullRequestUrl":     prURL,
			"merged":             false,
			"humanGate":          "pending",
			"reworkAssessment":   reworkAssessment,
			"reviewPacket":       reviewPacket,
			"changedFiles":       changedFiles,
			"artifacts":          stageArtifacts,
			"agentInvocations":   agentInvocations,
			"createdAt":          nowISO(),
		}); err != nil {
			return nil, err
		}
		recordAgent("done", "delivery", "waiting-human", deliveryPrompt, "handoff-bundle.json", "Delivery is blocked by the human review checkpoint after fast rework.", []string{filepath.Join(proofDir, "handoff-bundle.json")}, map[string]any{"runner": "local-orchestrator", "status": "waiting-human"})
		recordGitHubOutboundSync("human_review.waiting", "waiting-human", "human_review", "Pull request is ready for human review after fast rework.", prURL, checksOutput, changedFiles, "", "", reviewPacket)
		proofFiles, _ := collectFiles(proofDir)
		return map[string]any{
			"status":              "waiting-human",
			"workflow":            mapValue(mapValue(pipeline["run"])["workflow"]),
			"workspacePath":       workspace,
			"repositoryPath":      repoWorkspace,
			"branchName":          branchName,
			"pullRequestUrl":      prURL,
			"merged":              false,
			"changedFiles":        changedFiles,
			"stageArtifacts":      stageArtifacts,
			"agentInvocations":    agentInvocations,
			"proofFiles":          proofFiles,
			"reviewPacket":        reviewPacket,
			"reworkAssessment":    reworkAssessment,
			"humanChangeRequest":  humanChangeRequest,
			"pullRequestFeedback": pullRequestFeedback,
			"checkLogFeedback":    checkLogFeedback,
			"githubOutboundSync":  githubOutboundSync,
		}, nil
	}

	requirementArtifact := map[string]any{
		"workItemId":          text(item, "id"),
		"workItemKey":         text(item, "key"),
		"title":               text(item, "title"),
		"description":         effectiveDescription,
		"source":              text(item, "source"),
		"repositoryTargetId":  text(target, "id"),
		"repositoryTarget":    repoSlug,
		"repositoryClonePath": cloneTarget,
		"defaultBranch":       baseBranch,
		"acceptanceCriteria":  item["acceptanceCriteria"],
		"createdAt":           nowISO(),
	}
	if err := writeJSONFile(filepath.Join(proofDir, "requirement-artifact.json"), requirementArtifact); err != nil {
		return nil, err
	}
	requirementFallback := fmt.Sprintf("Structure requirement %s for repository %s.\n\nTitle: %s\n\nDescription:\n%s", text(item, "key"), repoSlug, text(item, "title"), effectiveDescription)
	requirementPrompt := renderWorkflowPromptSection(template, "requirement", promptVariables, requirementFallback)
	recordAgent("todo", "requirement", "passed", requirementPrompt, "requirement-artifact.json", "Requirement artifact captured with repository boundary and acceptance criteria.", []string{filepath.Join(proofDir, "requirement-artifact.json")}, map[string]any{"runner": "local-orchestrator", "status": "passed"})

	solutionPlan := fmt.Sprintf("# Solution Plan\n\n"+
		"- Work item: `%s`\n"+
		"- Repository: `%s`\n"+
		"- Base branch: `%s`\n"+
		"- Delivery branch: `%s`\n"+
		"- Planned change: implement the requested product change in the repository, not a proof-only placeholder.\n\n"+
		"## Stage Handoff\n\n"+
		"1. Requirement intake reads the Omega work item and repository workspace boundary.\n"+
		"2. Solution design passes the full requirement, acceptance criteria, and repository path to the coding agent.\n"+
		"3. Coding agent edits the target repository and must produce a real git diff.\n"+
		"4. Testing runs repository validation against the new commit.\n"+
		"5. Review reads the PR diff and CI/check state before delivery.\n"+
		"6. Human review records the gate decision.\n"+
		"7. Delivery merges or leaves the PR waiting for manual review.\n",
		text(item, "key"), repoSlug, baseBranch, branchName)
	if err := os.WriteFile(filepath.Join(proofDir, "solution-plan.md"), []byte(solutionPlan), 0o644); err != nil {
		return nil, err
	}
	solutionFallback := fmt.Sprintf("Design implementation for %s in %s.\n\nRequirement:\n%s", text(item, "key"), repoSlug, effectiveDescription)
	solutionPrompt := renderWorkflowPromptSection(template, "architect", promptVariables, solutionFallback)
	recordAgent("in_progress", "architect", "passed", solutionPrompt, "solution-plan.md", "Solution plan created and handed to the coding agent.", []string{filepath.Join(proofDir, "solution-plan.md")}, map[string]any{"runner": "local-orchestrator", "status": "passed"})
	codingFallback := fmt.Sprintf(`You are the coding agent for Omega.

Repository: %s
Repository path: %s
Work item: %s
Title: %s

Requirement:
%s

Rules:
- Work only inside this repository checkout.
- Implement the requested behavior. Do not create a proof-only placeholder.
- Add or update tests or runnable examples when the requirement asks for them.
- Keep the diff minimal and reviewable.
- Do not commit, push, or create a pull request. Omega will handle git delivery after you finish editing.
- Write a short completion note to %s with these sections:
  - What changed
  - Files changed
  - Validation run
  - Known follow-up or risk
`, repoSlug, repoWorkspace, text(item, "key"), text(item, "title"), effectiveDescription, filepath.Join(proofDir, "coding-agent-note.md"))
	codingPrompt := renderWorkflowPromptSection(template, "coding", promptVariables, codingFallback) + "\n\n" + agentPolicyBlock(profile, "coding")
	if err := os.WriteFile(filepath.Join(proofDir, "coding-prompt.md"), []byte(codingPrompt), 0o644); err != nil {
		return nil, err
	}
	recordAgent("in_progress", "coding", "running", codingPrompt, "", "Coding agent is editing the repository workspace.", []string{filepath.Join(proofDir, "coding-prompt.md")}, map[string]any{"runner": codingRunnerID, "status": "running"})
	codingTurn := codingRunner.RunTurn(ctx, AgentTurnRequest{
		Role:              "coding",
		StageID:           "in_progress",
		Runner:            codingRunnerID,
		Workspace:         repoWorkspace,
		Prompt:            codingPrompt,
		OutputPath:        filepath.Join(proofDir, "coding-agent-note.md"),
		Sandbox:           "workspace-write",
		Model:             codingProfile.Model,
		HeartbeatInterval: runnerHeartbeatInterval,
		OnProcessEvent:    server.runnerHeartbeatRecorder(text(pipeline, "id"), text(item, "id"), attemptID, "in_progress", "coding", codingRunnerID),
	})
	codingProcess, codingErr := codingTurn.Process, codingTurn.Error
	if codingErr != nil {
		reason := "Coding agent failed before producing an acceptable repository diff."
		recordAgent("in_progress", "coding", "failed", codingPrompt, "coding-agent-note.md", reason, []string{filepath.Join(proofDir, "coding-prompt.md"), filepath.Join(proofDir, "coding-agent-note.md")}, codingProcess)
		return failureResult("in_progress", "coding", reason, codingErr.Error(), ""), fmt.Errorf("coding agent failed: %w", codingErr)
	}
	statusOutput, err := runCommand(repoWorkspace, "git", "status", "--short")
	if err != nil {
		return nil, fmt.Errorf("read coding agent changes: %w", err)
	}
	if strings.TrimSpace(statusOutput) == "" {
		reason := "Coding agent completed but produced no repository changes, so there is no diff for review or delivery."
		recordAgent("in_progress", "coding", "failed", codingPrompt, "coding-agent-note.md", reason, []string{filepath.Join(proofDir, "coding-prompt.md"), filepath.Join(proofDir, "coding-agent-note.md")}, codingProcess)
		return failureResult("in_progress", "coding", reason, "git status --short returned no changed files after the coding runner finished.", ""), errors.New("coding agent produced no repository changes")
	}
	if _, err := runCommand(repoWorkspace, "git", "add", "-A"); err != nil {
		return nil, fmt.Errorf("stage coding agent changes: %w", err)
	}
	if _, err := runCommand(repoWorkspace, "git", "commit", "-m", "Omega implementation for "+text(item, "key")); err != nil {
		return nil, fmt.Errorf("commit coding agent changes: %w", err)
	}
	commitSha, _ := runCommand(repoWorkspace, "git", "rev-parse", "HEAD")
	commitSummary, _ := runCommand(repoWorkspace, "git", "show", "--stat", "--oneline", "--no-renames", "HEAD")
	diffText, _ := runCommand(repoWorkspace, "git", "diff", "HEAD~1..HEAD")
	changedNames, err := runCommand(repoWorkspace, "git", "diff", "--name-only", "HEAD~1..HEAD")
	if err != nil {
		return nil, fmt.Errorf("list changed files: %w", err)
	}
	changedFiles := compactLines(changedNames)
	if err := os.WriteFile(filepath.Join(proofDir, "git-diff.patch"), []byte(diffText), 0o644); err != nil {
		return nil, err
	}
	implementationSummary := fmt.Sprintf("# Implementation\n\n- Branch: `%s`\n- Commit: `%s`\n- Changed files:\n%s\n```text\n%s\n```\n", branchName, strings.TrimSpace(commitSha), markdownFileList(changedFiles), truncateForProof(commitSummary, 4000))
	if err := os.WriteFile(filepath.Join(proofDir, "implementation-summary.md"), []byte(implementationSummary), 0o644); err != nil {
		return nil, err
	}
	recordAgent("in_progress", "coding", "passed", codingPrompt, "implementation-summary.md", fmt.Sprintf("Coding agent produced %d changed file(s).", len(changedFiles)), []string{filepath.Join(proofDir, "coding-prompt.md"), filepath.Join(proofDir, "coding-agent-note.md"), filepath.Join(proofDir, "implementation-summary.md"), filepath.Join(proofDir, "git-diff.patch")}, codingProcess)

	testOutput, testErr := runRepositoryValidation(repoWorkspace)
	testStatus := "passed"
	if testErr != nil {
		testStatus = "failed"
	}
	testVariables := cloneStringMap(promptVariables)
	testVariables["changedFiles"] = strings.Join(changedFiles, ", ")
	testVariables["testOutput"] = testOutput
	testFallback := fmt.Sprintf("Validate %s after coding changes. Changed files: %s", text(item, "key"), strings.Join(changedFiles, ", "))
	testPrompt := renderWorkflowPromptSection(template, "testing", testVariables, testFallback)
	testReport := fmt.Sprintf("# Test Report\n\nStatus: %s\n\n## Commands\n\n```text\n%s\n```\n\n## Acceptance coverage\n\n- Validation was run against the repository after coding changes.\n\n## Failures\n\n%s\n\n## Residual risk\n\n- Project-specific coverage depends on available repository test commands.\n", testStatus, stringOr(strings.TrimSpace(testOutput), "No validation output."), stringOr(testFailureSummary(testErr, testOutput), "None"))
	if err := os.WriteFile(filepath.Join(proofDir, "test-report.md"), []byte(testReport), 0o644); err != nil {
		return nil, err
	}
	recordAgent("in_progress", "testing", testStatus, testPrompt, "test-report.md", "Repository validation completed.", []string{filepath.Join(proofDir, "test-report.md")}, map[string]any{"runner": "local-validation", "status": testStatus, "stdout": testOutput})
	if testErr != nil {
		reason := "Repository validation failed after the coding agent produced changes."
		return failureResult("in_progress", "testing", reason, stringOr(testOutput, testErr.Error()), ""), fmt.Errorf("repository validation failed: %w", testErr)
	}

	if text(target, "kind") == "github" {
		_, _ = runCommand(repoWorkspace, "gh", "auth", "setup-git")
	}
	if err := pushDevFlowBranch(repoWorkspace, branchName); err != nil {
		return nil, fmt.Errorf("push branch: %w", err)
	}

	prTitle := text(item, "key") + " " + text(item, "title")
	prBody := buildDevFlowPullRequestBody(item, changedFiles, testOutput, humanChangeRequest, diffText)
	prURL, err := ensureDevFlowPullRequest(repoWorkspace, repoSlug, branchName, baseBranch, prTitle, prBody)
	if err != nil {
		return nil, fmt.Errorf("create pull request: %w", err)
	}
	if humanChangeRequest != "" {
		if output, err := updateDevFlowPullRequestDescriptionIfChanged(repoWorkspace, prURL, prTitle, prBody); err != nil {
			server.logDebug(ctx, "github.pr.description_update_skipped", "Pull request description update skipped after human-requested rework.", map[string]any{"pipelineId": text(pipeline, "id"), "attemptId": attemptID, "pullRequestUrl": prURL, "output": truncateForProof(output+"\n"+err.Error(), 1200)})
		} else {
			server.logInfo(ctx, "github.pr.description_updated", "Pull request description updated after human-requested rework.", map[string]any{"pipelineId": text(pipeline, "id"), "attemptId": attemptID, "pullRequestUrl": prURL, "output": truncateForProof(output, 1200)})
		}
	}
	prDiff, _ := runCommand(repoWorkspace, "gh", "pr", "diff", prURL)
	checksOutput, _ := runCommand(repoWorkspace, "gh", "pr", "checks", prURL)
	pullRequestFeedback = githubPullRequestFeedback(ctx, repoWorkspace, prURL, repoSlug)
	checkLogFeedback = githubPullRequestCheckLogFeedback(ctx, repoWorkspace, prURL, repoSlug, nil)

	reviewRounds := defaultDevFlowReviewRounds()
	if template != nil && len(template.ReviewRounds) > 0 {
		reviewRounds = template.ReviewRounds
	}
	maxReviewCycles := devFlowMaxReviewCycles(template)
	runReworkTurn := func(cycle int, feedback string) error {
		stageID := devFlowTransitionTo(template, "code_review_round_1", "changes_requested", "rework")
		if stageID == "" {
			stageID = "rework"
		}
		notePath := filepath.Join(proofDir, fmt.Sprintf("rework-agent-note-%d.md", cycle))
		promptPath := filepath.Join(proofDir, fmt.Sprintf("rework-prompt-%d.md", cycle))
		reworkVariables := cloneStringMap(promptVariables)
		reworkVariables["pullRequestUrl"] = prURL
		roundFeedback := strings.TrimSpace(feedback)
		if checklistPrompt := reworkChecklistPromptFromAttempt(currentAttempt, ""); checklistPrompt != "" {
			roundFeedback = strings.TrimSpace(checklistPrompt + "\n\nLatest review feedback:\n" + feedback)
		}
		reworkVariables["reviewFeedback"] = roundFeedback
		reworkVariables["reworkNotePath"] = notePath
		reworkFallback := fmt.Sprintf(`You are the rework coding agent for Omega.

Repository: %s
Repository path: %s
Work item: %s
Title: %s
Pull request: %s

Requirement:
%s

Rework checklist to address:
%s

Rules:
- Continue in the same repository checkout, same branch, and same pull request.
- Address the review feedback with a real code change.
- Keep the diff minimal and reviewable.
- Do not commit, push, or create a pull request. Omega will handle git delivery after you finish editing.
- Write a short completion note to %s with these sections:
  - Review feedback addressed
  - What changed
  - Files changed
  - Validation run
  - Remaining risk
`, repoSlug, repoWorkspace, text(item, "key"), text(item, "title"), prURL, effectiveDescription, roundFeedback, notePath)
		reworkPrompt := renderWorkflowPromptSection(template, "rework", reworkVariables, reworkFallback) + "\n\n" + agentPolicyBlock(profile, "coding")
		if err := os.WriteFile(promptPath, []byte(reworkPrompt), 0o644); err != nil {
			return err
		}
		recordAgent(stageID, "coding", "running", reworkPrompt, "", "Rework agent is applying review feedback in the same workspace.", []string{promptPath}, map[string]any{"runner": codingRunnerID, "status": "running", "reviewCycle": cycle})
		turn := codingRunner.RunTurn(ctx, AgentTurnRequest{
			Role:              "rework",
			StageID:           stageID,
			Runner:            codingRunnerID,
			Workspace:         repoWorkspace,
			Prompt:            reworkPrompt,
			OutputPath:        notePath,
			Sandbox:           "workspace-write",
			Model:             codingProfile.Model,
			HeartbeatInterval: runnerHeartbeatInterval,
			OnProcessEvent:    server.runnerHeartbeatRecorder(text(pipeline, "id"), text(item, "id"), attemptID, stageID, "coding", codingRunnerID),
		})
		if turn.Error != nil {
			recordAgent(stageID, "coding", "failed", reworkPrompt, filepath.Base(notePath), "Rework agent failed before producing an acceptable repository diff.", []string{promptPath, notePath}, turn.Process)
			return fmt.Errorf("rework agent failed: %w", turn.Error)
		}
		statusOutput, err := runCommand(repoWorkspace, "git", "status", "--short")
		if err != nil {
			return fmt.Errorf("read rework changes: %w", err)
		}
		if strings.TrimSpace(statusOutput) == "" {
			recordAgent(stageID, "coding", "failed", reworkPrompt, filepath.Base(notePath), "Rework agent produced no repository changes.", []string{promptPath, notePath}, turn.Process)
			return errors.New("rework agent produced no repository changes")
		}
		if _, err := runCommand(repoWorkspace, "git", "add", "-A"); err != nil {
			return fmt.Errorf("stage rework changes: %w", err)
		}
		if _, err := runCommand(repoWorkspace, "git", "commit", "-m", fmt.Sprintf("Omega rework for %s round %d", text(item, "key"), cycle)); err != nil {
			return fmt.Errorf("commit rework changes: %w", err)
		}
		commitSha, _ = runCommand(repoWorkspace, "git", "rev-parse", "HEAD")
		commitSummary, _ = runCommand(repoWorkspace, "git", "show", "--stat", "--oneline", "--no-renames", "HEAD")
		diffText, _ = runCommand(repoWorkspace, "git", "diff", "HEAD~1..HEAD")
		changedNames, err = runCommand(repoWorkspace, "git", "diff", "--name-only", "HEAD~1..HEAD")
		if err != nil {
			return fmt.Errorf("list rework changed files: %w", err)
		}
		changedFiles = uniqueStrings(append(changedFiles, compactLines(changedNames)...))
		if err := os.WriteFile(filepath.Join(proofDir, fmt.Sprintf("git-diff-rework-%d.patch", cycle)), []byte(diffText), 0o644); err != nil {
			return err
		}
		reworkSummaryPath := filepath.Join(proofDir, fmt.Sprintf("rework-summary-%d.md", cycle))
		reworkSummary := fmt.Sprintf("# Rework Round %d\n\n- Branch: `%s`\n- Commit: `%s`\n- Changed files:\n%s\n```text\n%s\n```\n", cycle, branchName, strings.TrimSpace(commitSha), markdownFileList(compactLines(changedNames)), truncateForProof(commitSummary, 4000))
		if err := os.WriteFile(reworkSummaryPath, []byte(reworkSummary), 0o644); err != nil {
			return err
		}
		recordAgent(stageID, "coding", "passed", reworkPrompt, filepath.Base(reworkSummaryPath), fmt.Sprintf("Rework agent produced %d changed file(s).", len(compactLines(changedNames))), []string{promptPath, notePath, reworkSummaryPath, filepath.Join(proofDir, fmt.Sprintf("git-diff-rework-%d.patch", cycle))}, turn.Process)
		testOutput, testErr = runRepositoryValidation(repoWorkspace)
		testStatus := "passed"
		if testErr != nil {
			testStatus = "failed"
		}
		testReportPath := filepath.Join(proofDir, fmt.Sprintf("test-report-rework-%d.md", cycle))
		testVariables := cloneStringMap(promptVariables)
		testVariables["changedFiles"] = strings.Join(changedFiles, ", ")
		testVariables["testOutput"] = testOutput
		testFallback := fmt.Sprintf("Validate %s after rework round %d. Changed files: %s", text(item, "key"), cycle, strings.Join(changedFiles, ", "))
		testPrompt := renderWorkflowPromptSection(template, "testing", testVariables, testFallback)
		testReport := fmt.Sprintf("# Rework Test Report\n\nStatus: %s\n\n## Commands\n\n```text\n%s\n```\n\n## Acceptance coverage\n\n- Validation was run against the repository after rework round %d.\n\n## Failures\n\n%s\n\n## Residual risk\n\n- Project-specific coverage depends on available repository test commands.\n", testStatus, stringOr(strings.TrimSpace(testOutput), "No validation output."), cycle, stringOr(testFailureSummary(testErr, testOutput), "None"))
		if err := os.WriteFile(testReportPath, []byte(testReport), 0o644); err != nil {
			return err
		}
		recordAgent(stageID, "testing", testStatus, testPrompt, filepath.Base(testReportPath), "Repository validation completed after rework.", []string{testReportPath}, map[string]any{"runner": "local-validation", "status": testStatus, "stdout": testOutput})
		if testErr != nil {
			return fmt.Errorf("repository validation failed after rework: %w", testErr)
		}
		if err := pushDevFlowBranch(repoWorkspace, branchName); err != nil {
			return fmt.Errorf("push rework branch: %w", err)
		}
		reworkPRBody := buildDevFlowPullRequestBody(item, changedFiles, testOutput, stringOr(feedback, humanChangeRequest), diffText)
		if output, err := updateDevFlowPullRequestDescriptionIfChanged(repoWorkspace, prURL, prTitle, reworkPRBody); err != nil {
			server.logDebug(ctx, "github.pr.description_update_skipped", "Pull request description update skipped after rework.", map[string]any{"pipelineId": text(pipeline, "id"), "attemptId": attemptID, "pullRequestUrl": prURL, "output": truncateForProof(output+"\n"+err.Error(), 1200)})
		} else {
			server.logInfo(ctx, "github.pr.description_updated", "Pull request description updated after rework.", map[string]any{"pipelineId": text(pipeline, "id"), "attemptId": attemptID, "pullRequestUrl": prURL, "output": truncateForProof(output, 1200)})
		}
		prDiff, _ = runCommand(repoWorkspace, "gh", "pr", "diff", prURL)
		checksOutput, _ = runCommand(repoWorkspace, "gh", "pr", "checks", prURL)
		pullRequestFeedback = githubPullRequestFeedback(ctx, repoWorkspace, prURL, repoSlug)
		checkLogFeedback = githubPullRequestCheckLogFeedback(ctx, repoWorkspace, prURL, repoSlug, nil)
		return nil
	}
	reviewCycle := 1
	for {
		var reviewFeedback string
		needsRework := false
		for _, reviewRound := range reviewRounds {
			stageID := stringOr(reviewRound.StageID, "code_review")
			artifact := stringOr(reviewRound.Artifact, stageID+".md")
			if reviewCycle > 1 {
				artifact = strings.TrimSuffix(artifact, filepath.Ext(artifact)) + fmt.Sprintf("-cycle-%d%s", reviewCycle, filepath.Ext(artifact))
			}
			reviewPath := filepath.Join(proofDir, artifact)
			reviewDiff := diffText
			reviewChecks := ""
			if reviewRound.DiffSource == "pr_diff" {
				reviewDiff = prDiff
				reviewChecks = checksOutput
			}
			reviewVariables := cloneStringMap(promptVariables)
			reviewVariables["pullRequestUrl"] = prURL
			reviewVariables["changedFiles"] = strings.Join(changedFiles, ", ")
			reviewVariables["testOutput"] = testOutput
			reviewVariables["checksOutput"] = reviewChecks
			reviewVariables["reviewFocus"] = reviewRound.Focus
			reviewVariables["diff"] = reviewDiff
			reviewVariables["reviewFeedback"] = combinedFeedback(stringOr(reviewFeedback, humanChangeRequest))
			reviewFallback := buildDevFlowReviewPrompt(item, repoSlug, prURL, changedFiles, reviewDiff, testOutput, reviewChecks, reviewRound.Focus, combinedFeedback(stringOr(reviewFeedback, humanChangeRequest)))
			reviewPrompt := renderWorkflowPromptSection(template, "review", reviewVariables, reviewFallback) + "\n\n" + agentPolicyBlock(profile, "review")
			reviewTurn := reviewRunner.RunTurn(ctx, AgentTurnRequest{
				Role:              "review",
				StageID:           stageID,
				Runner:            reviewRunnerID,
				Workspace:         repoWorkspace,
				Prompt:            reviewPrompt,
				OutputPath:        reviewPath,
				Sandbox:           "read-only",
				Model:             reviewProfile.Model,
				Effort:            "medium",
				HeartbeatInterval: runnerHeartbeatInterval,
				OnProcessEvent:    server.runnerHeartbeatRecorder(text(pipeline, "id"), text(item, "id"), attemptID, stageID, "review", reviewRunnerID),
			})
			reviewProcess, reviewErr := reviewTurn.Process, reviewTurn.Error
			if reviewErr != nil {
				reason := "Review agent failed before issuing a verdict."
				recordAgent(stageID, "review", "failed", reviewPrompt, artifact, reason, []string{reviewPath}, reviewProcess)
				return failureResult(stageID, "review", reason, reviewErr.Error(), ""), fmt.Errorf("%s failed: %w", stageID, reviewErr)
			}
			outcome := devFlowReviewOutcome(reviewPath)
			switch outcome.Verdict {
			case "approved":
				recordAgent(stageID, "review", "passed", reviewPrompt, artifact, outcome.Summary, []string{reviewPath}, reviewProcess)
			case "needs_human_info":
				recordAgent(stageID, "review", "needs-human", reviewPrompt, artifact, outcome.Summary, []string{reviewPath}, reviewProcess)
				reviewFeedback = outcome.Summary
				needsRework = false
				reviewCycle = maxReviewCycles + 1
				break
			case "changes_requested":
				recordAgent(stageID, "review", "changes-requested", reviewPrompt, artifact, outcome.Summary, []string{reviewPath}, reviewProcess)
				reviewFeedback = outcome.Summary
				needsRework = true
				break
			default:
				recordAgent(stageID, "review", "changes-requested", reviewPrompt, artifact, outcome.Summary, []string{reviewPath}, reviewProcess)
				reviewFeedback = outcome.Summary
				needsRework = true
				break
			}
			if needsRework || outcome.Verdict == "needs_human_info" {
				break
			}
		}
		if !needsRework {
			break
		}
		if reviewCycle >= maxReviewCycles {
			recordAgent("human_review", "human", "waiting-human", reviewFeedback, "human-review-request.md", "Review requested changes after the maximum automated rework cycles. Human input is required.", []string{}, map[string]any{"runner": "human", "status": "waiting-human"})
			break
		}
		reviewCycle++
		if err := runReworkTurn(reviewCycle, reviewFeedback); err != nil {
			reason := "Rework agent failed while applying review feedback."
			return failureResult("rework", "coding", reason, err.Error(), reviewFeedback), err
		}
	}

	humanReviewRequestPath := filepath.Join(proofDir, "human-review-request.md")
	humanReviewRequest := fmt.Sprintf("# Human Review Request\n\n- Work item: `%s` %s\n- Repository: `%s`\n- Pull request: %s\n- Changed files:\n%s\n\n## Delivery state\n\n- Waiting for human approval.\n\n## Proof\n\n- Implementation summary\n- Test report\n- Review reports\n- Pull request\n\n## Operator notes\n\nThe review agents approved the PR. A human must approve this checkpoint before Omega performs delivery/merge.\n", text(item, "key"), text(item, "title"), repoSlug, prURL, markdownFileList(changedFiles))
	if err := os.WriteFile(humanReviewRequestPath, []byte(humanReviewRequest), 0o644); err != nil {
		return nil, err
	}
	recordAgent("human_review", "human", "waiting-human", "Review the PR, proof, and agent verdicts. Approve to continue delivery or request changes to send the run back.", "human-review-request.md", "Waiting for explicit human approval before delivery.", []string{humanReviewRequestPath}, map[string]any{"runner": "human", "status": "waiting-human"})

	reportInput := devFlowRunReportInput{
		Item:                item,
		Repository:          repoSlug,
		BranchName:          branchName,
		PullRequestURL:      prURL,
		ChangedFiles:        changedFiles,
		DiffText:            stringOr(prDiff, diffText),
		TestOutput:          testOutput,
		ChecksOutput:        checksOutput,
		PullRequestFeedback: pullRequestFeedback,
		CheckLogFeedback:    checkLogFeedback,
		StageArtifacts:      stageArtifacts,
		AgentInvocations:    agentInvocations,
	}
	reviewPacket, reviewPacketPath, err := writeDevFlowReviewPacket(proofDir, reportInput)
	if err != nil {
		return nil, err
	}
	stageArtifacts = append(stageArtifacts, map[string]any{"stageId": "human_review", "agentId": "delivery", "artifact": filepath.Base(reviewPacketPath)})
	reportInput.ReviewPacket = reviewPacket
	if reportPath, err := writeDevFlowRunReport(proofDir, reportInput); err != nil {
		return nil, err
	} else {
		stageArtifacts = append(stageArtifacts, map[string]any{"stageId": "human_review", "agentId": "delivery", "artifact": filepath.Base(reportPath)})
	}

	merged := false
	deliveryVariables := cloneStringMap(promptVariables)
	deliveryVariables["pullRequestUrl"] = prURL
	deliveryVariables["changedFiles"] = strings.Join(changedFiles, ", ")
	deliveryVariables["testOutput"] = testOutput
	deliveryVariables["checksOutput"] = checksOutput
	deliveryFallback := fmt.Sprintf("Assemble delivery handoff for %s. PR: %s. Changed files: %s", text(item, "key"), prURL, strings.Join(changedFiles, ", "))
	deliveryPrompt := renderWorkflowPromptSection(template, "delivery", deliveryVariables, deliveryFallback)
	stageArtifacts = append(stageArtifacts, map[string]any{"stageId": "done", "agentId": "delivery", "artifact": "handoff-bundle.json"})
	if err := writeJSONFile(filepath.Join(proofDir, "handoff-bundle.json"), map[string]any{
		"pipelineId":         text(pipeline, "id"),
		"workItemId":         text(item, "id"),
		"workItemKey":        text(item, "key"),
		"repositoryTargetId": text(target, "id"),
		"repositoryTarget":   repoSlug,
		"workspacePath":      workspace,
		"repositoryPath":     repoWorkspace,
		"branchName":         branchName,
		"pullRequestUrl":     prURL,
		"merged":             merged,
		"humanGate":          "pending",
		"reviewPacket":       reviewPacket,
		"changedFiles":       changedFiles,
		"artifacts":          stageArtifacts,
		"agentInvocations":   agentInvocations,
		"createdAt":          nowISO(),
	}); err != nil {
		return nil, err
	}
	recordAgent("done", "delivery", "waiting-human", deliveryPrompt, "handoff-bundle.json", "Delivery is blocked by the human review checkpoint.", []string{filepath.Join(proofDir, "handoff-bundle.json")}, map[string]any{"runner": "local-orchestrator", "status": "waiting-human"})
	recordGitHubOutboundSync("human_review.waiting", "waiting-human", "human_review", "Pull request is ready for human review.", prURL, checksOutput, changedFiles, "", "", reviewPacket)
	proofFiles, _ := collectFiles(proofDir)
	return map[string]any{
		"status":              "waiting-human",
		"workflow":            mapValue(mapValue(pipeline["run"])["workflow"]),
		"workspacePath":       workspace,
		"repositoryPath":      repoWorkspace,
		"branchName":          branchName,
		"pullRequestUrl":      prURL,
		"merged":              merged,
		"changedFiles":        changedFiles,
		"stageArtifacts":      stageArtifacts,
		"agentInvocations":    agentInvocations,
		"proofFiles":          proofFiles,
		"reviewPacket":        reviewPacket,
		"pullRequestFeedback": pullRequestFeedback,
		"checkLogFeedback":    checkLogFeedback,
		"githubOutboundSync":  githubOutboundSync,
	}, nil
}

func latestHumanChangeRequestFromPipeline(pipeline map[string]any) string {
	events := arrayMaps(mapValue(pipeline["run"])["events"])
	for index := len(events) - 1; index >= 0; index-- {
		event := events[index]
		eventType := text(event, "type")
		if eventType != "human.rework.requested" && eventType != "gate.rejected" {
			continue
		}
		message := strings.TrimSpace(text(event, "message"))
		if message == "" {
			continue
		}
		for _, marker := range []string{"Human requested changes:", "rejected:"} {
			if offset := strings.Index(message, marker); offset >= 0 {
				return strings.TrimSpace(message[offset+len(marker):])
			}
		}
		return message
	}
	return ""
}
