package omegalocal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func devFlowWorkspacePaths(ctx context.Context, server *Server, item map[string]any) (string, string, string, error) {
	workspaceRoot := server.localWorkspaceRoot(ctx)
	workspace, err := workspaceChildPath(workspaceRoot, devFlowRunWorkspaceName(text(item, "key")))
	if err != nil {
		return "", "", "", err
	}
	repoWorkspace, err := ensurePathInsideRoot(workspaceRoot, filepath.Join(workspace, "repo"))
	if err != nil {
		return "", "", "", err
	}
	return workspaceRoot, workspace, repoWorkspace, nil
}

func devFlowExecutionScope(item map[string]any, target map[string]any) string {
	return fmt.Sprintf("devflow:%s:%s", text(target, "id"), text(item, "id"))
}

func devFlowWorkspaceLifecycleSpec(ctx context.Context, server *Server, item map[string]any, target map[string]any, pipeline map[string]any, attempt map[string]any) (map[string]any, error) {
	workspaceRoot, workspacePath, repositoryPath, err := devFlowWorkspacePaths(ctx, server, item)
	if err != nil {
		return nil, err
	}
	template := findPipelineTemplate(text(pipeline, "templateId"))
	timeout := devFlowAttemptTimeout(template)
	scope := devFlowExecutionScope(item, target)
	return map[string]any{
		"scope":              scope,
		"workspaceRoot":      workspaceRoot,
		"workspacePath":      workspacePath,
		"repositoryPath":     repositoryPath,
		"repositoryTargetId": text(target, "id"),
		"repositoryTarget":   repositoryTargetLabel(target),
		"workItemId":         text(item, "id"),
		"pipelineId":         text(pipeline, "id"),
		"attemptId":          text(attempt, "id"),
		"cleanupPolicy":      "retain-proof-until-manual-cleanup",
		"lockTtlSeconds":     int(timeout.Seconds()),
		"createdAt":          nowISO(),
	}, nil
}

func claimDevFlowWorkspaceLock(ctx context.Context, server *Server, item map[string]any, target map[string]any, pipeline map[string]any, attempt map[string]any) (map[string]any, error) {
	lifecycle, err := devFlowWorkspaceLifecycleSpec(ctx, server, item, target, pipeline, attempt)
	if err != nil {
		return nil, err
	}
	scope := text(lifecycle, "scope")
	if lock := existingExecutionLock(ctx, server, scope); lock != nil {
		return nil, fmt.Errorf("workspace is locked by attempt %s", text(lock, "attemptId"))
	}
	now := time.Now().UTC()
	lock := map[string]any{
		"id":                 executionLockID(scope),
		"scope":              scope,
		"status":             "claimed",
		"runnerProcessState": "queued",
		"repositoryTargetId": text(target, "id"),
		"repositoryTarget":   repositoryTargetLabel(target),
		"workItemId":         text(item, "id"),
		"pipelineId":         text(pipeline, "id"),
		"attemptId":          text(attempt, "id"),
		"workspaceRoot":      text(lifecycle, "workspaceRoot"),
		"workspacePath":      text(lifecycle, "workspacePath"),
		"repositoryPath":     text(lifecycle, "repositoryPath"),
		"cleanupPolicy":      text(lifecycle, "cleanupPolicy"),
		"expiresAt":          now.Add(time.Duration(intValue(lifecycle["lockTtlSeconds"])) * time.Second).Format(time.RFC3339Nano),
		"createdAt":          now.Format(time.RFC3339Nano),
		"updatedAt":          now.Format(time.RFC3339Nano),
	}
	if err := saveExecutionLock(ctx, server, lock); err != nil {
		return nil, err
	}
	return lock, nil
}

type workspaceCleanupOptions struct {
	AutoCleanupWorkspaces     bool
	WorkspaceRetentionSeconds int
	Limit                     int
}

func (server *Server) scanWorkspaceCleanup(ctx context.Context, database *WorkspaceDatabase, options workspaceCleanupOptions) map[string]any {
	summary := map[string]any{
		"changed":                0,
		"checkedWorkspaces":      0,
		"cleanupCandidates":      0,
		"cleanedWorkspaces":      0,
		"cleanupSkipped":         0,
		"workspaceCleanups":      []map[string]any{},
		"workspaceCleanupErrors": []map[string]any{},
	}
	if database == nil {
		return summary
	}
	limit := options.Limit
	if limit <= 0 {
		limit = 10
	}
	for index, attempt := range database.Tables.Attempts {
		if intValue(summary["checkedWorkspaces"]) >= limit {
			break
		}
		if !workspaceCleanupTerminalStatus(text(attempt, "status")) || text(attempt, "workspacePath") == "" {
			continue
		}
		summary["checkedWorkspaces"] = intValue(summary["checkedWorkspaces"]) + 1
		if text(mapValue(attempt["workspaceCleanup"]), "status") == "cleaned" {
			summary["cleanupSkipped"] = intValue(summary["cleanupSkipped"]) + 1
			continue
		}
		policy := workspaceCleanupPolicyForAttempt(*database, attempt, options)
		record := map[string]any{
			"attemptId":        text(attempt, "id"),
			"pipelineId":       text(attempt, "pipelineId"),
			"workItemId":       text(attempt, "itemId"),
			"status":           text(attempt, "status"),
			"workspacePath":    text(attempt, "workspacePath"),
			"repositoryPath":   filepath.Join(text(attempt, "workspacePath"), "repo"),
			"policy":           text(policy, "policy"),
			"retentionSeconds": intValue(policy["retentionSeconds"]),
			"decision":         text(policy, "decision"),
		}
		if text(policy, "decision") != "cleanup-ready" {
			summary["cleanupSkipped"] = intValue(summary["cleanupSkipped"]) + 1
			summary["workspaceCleanups"] = append(arrayMaps(summary["workspaceCleanups"]), record)
			continue
		}
		summary["cleanupCandidates"] = intValue(summary["cleanupCandidates"]) + 1
		if !options.AutoCleanupWorkspaces {
			summary["workspaceCleanups"] = append(arrayMaps(summary["workspaceCleanups"]), withSupervisorDecision(record, "dry-run"))
			continue
		}
		repositoryPath := text(record, "repositoryPath")
		if err := removeWorkspaceRepository(ctx, server, text(attempt, "workspacePath"), repositoryPath); err != nil {
			summary["workspaceCleanupErrors"] = append(arrayMaps(summary["workspaceCleanupErrors"]), map[string]any{"attemptId": text(attempt, "id"), "error": err.Error()})
			server.logError(ctx, "workspace.cleanup.failed", err.Error(), map[string]any{"attemptId": text(attempt, "id"), "pipelineId": text(attempt, "pipelineId"), "workspacePath": text(attempt, "workspacePath")})
			continue
		}
		nextAttempt := cloneMap(attempt)
		nextAttempt["workspaceCleanup"] = map[string]any{
			"status":           "cleaned",
			"policy":           text(policy, "policy"),
			"repositoryPath":   repositoryPath,
			"proofRetained":    true,
			"cleanedAt":        nowISO(),
			"retentionSeconds": intValue(policy["retentionSeconds"]),
		}
		nextAttempt["updatedAt"] = nowISO()
		events := arrayMaps(nextAttempt["events"])
		events = append(events, map[string]any{"type": "workspace.cleaned", "message": "Repository checkout removed; proof artifacts retained.", "createdAt": nowISO()})
		nextAttempt["events"] = events
		database.Tables.Attempts[index] = nextAttempt
		summary["changed"] = intValue(summary["changed"]) + 1
		summary["cleanedWorkspaces"] = intValue(summary["cleanedWorkspaces"]) + 1
		summary["workspaceCleanups"] = append(arrayMaps(summary["workspaceCleanups"]), withSupervisorDecision(record, "cleaned"))
		server.logInfo(ctx, "workspace.cleanup.cleaned", "Repository checkout removed and proof retained.", map[string]any{"attemptId": text(attempt, "id"), "pipelineId": text(attempt, "pipelineId"), "workspacePath": text(attempt, "workspacePath"), "repositoryPath": repositoryPath})
	}
	return summary
}

func (server *Server) cleanupWorkspaces(response http.ResponseWriter, request *http.Request) {
	var payload struct {
		Apply            bool `json:"apply"`
		RetentionSeconds int  `json:"retentionSeconds"`
		Limit            int  `json:"limit"`
	}
	_ = json.NewDecoder(request.Body).Decode(&payload)
	database, err := mustLoad(server, request.Context())
	if err != nil {
		writeError(response, http.StatusNotFound, err)
		return
	}
	summary := server.scanWorkspaceCleanup(request.Context(), &database, workspaceCleanupOptions{
		AutoCleanupWorkspaces:     payload.Apply,
		WorkspaceRetentionSeconds: payload.RetentionSeconds,
		Limit:                     payload.Limit,
	})
	if intValue(summary["changed"]) > 0 {
		if err := server.Repo.Save(request.Context(), database); err != nil {
			writeError(response, http.StatusInternalServerError, err)
			return
		}
	}
	writeJSON(response, http.StatusOK, summary)
}

func workspaceCleanupTerminalStatus(status string) bool {
	switch status {
	case "done", "completed", "failed", "canceled", "stalled":
		return true
	default:
		return false
	}
}

func workspaceCleanupPolicyForAttempt(database WorkspaceDatabase, attempt map[string]any, options workspaceCleanupOptions) map[string]any {
	status := text(attempt, "status")
	if status != "done" && status != "completed" {
		return map[string]any{"policy": "retain-for-debugging", "decision": "retain-terminal-status", "retentionSeconds": 0}
	}
	retentionSeconds := options.WorkspaceRetentionSeconds
	if retentionSeconds <= 0 {
		retentionSeconds = intValue(workflowRuntimeFromPipeline(pipelineByID(database, text(attempt, "pipelineId")))["cleanupRetentionSeconds"])
	}
	if retentionSeconds <= 0 {
		retentionSeconds = 24 * 60 * 60
	}
	finishedAt := stringOr(text(attempt, "finishedAt"), text(attempt, "updatedAt"))
	if !timestampOlderThan(finishedAt, retentionSeconds) {
		return map[string]any{"policy": "retain-until-retention-elapses", "decision": "waiting-retention", "retentionSeconds": retentionSeconds}
	}
	return map[string]any{"policy": "delete-repo-retain-proof", "decision": "cleanup-ready", "retentionSeconds": retentionSeconds}
}

func timestampOlderThan(value string, seconds int) bool {
	if seconds <= 0 {
		return true
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return false
	}
	return time.Since(parsed) >= time.Duration(seconds)*time.Second
}

func removeWorkspaceRepository(ctx context.Context, server *Server, workspacePath string, repositoryPath string) error {
	workspaceRoot := server.localWorkspaceRoot(ctx)
	if _, err := ensurePathInsideRoot(workspaceRoot, workspacePath); err != nil {
		return err
	}
	repositoryPath, err := ensurePathInsideRoot(workspaceRoot, repositoryPath)
	if err != nil {
		return err
	}
	if filepath.Base(repositoryPath) != "repo" {
		return fmt.Errorf("cleanup refuses to remove non-repo path: %s", repositoryPath)
	}
	return os.RemoveAll(repositoryPath)
}
