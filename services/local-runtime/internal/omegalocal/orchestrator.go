package omegalocal

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

type orchestratorTickPayload struct {
	RepositoryTargetID string `json:"repositoryTargetId"`
	Limit              string `json:"limit"`
	AutoRun            bool   `json:"autoRun"`
	AutoApproveHuman   bool   `json:"autoApproveHuman"`
	AutoMerge          bool   `json:"autoMerge"`
}

func (server *Server) orchestratorTick(response http.ResponseWriter, request *http.Request) {
	var payload orchestratorTickPayload
	_ = json.NewDecoder(request.Body).Decode(&payload)
	result, status, err := server.runOrchestratorTick(request.Context(), payload)
	if err != nil {
		writeError(response, status, err)
		return
	}
	writeJSON(response, status, result)
}

func (server *Server) runOrchestratorTick(ctx context.Context, payload orchestratorTickPayload) (map[string]any, int, error) {
	database, err := mustLoad(server, ctx)
	if err != nil {
		return nil, http.StatusNotFound, err
	}
	target := findRepositoryTarget(database, payload.RepositoryTargetID)
	if target == nil && payload.RepositoryTargetID == "" {
		target = firstRepositoryTarget(database)
	}
	if target == nil {
		return nil, http.StatusBadRequest, fmt.Errorf("repository target is required")
	}
	repo := repositoryTargetLabel(target)
	if repo == "" {
		return nil, http.StatusBadRequest, fmt.Errorf("repository target %s has no repository label", text(target, "id"))
	}

	issues, err := listGitHubIssues(ctx, repo, stringOr(payload.Limit, "20"))
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	template := findPipelineTemplate("devflow-pr")
	if template == nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("devflow-pr template not found")
	}
	for _, issue := range issues {
		if !isExecutableGitHubIssue(issue) {
			continue
		}
		item := githubIssueToWorkItem(repo, issue)
		item["repositoryTargetId"] = text(target, "id")
		if item["target"] == "" {
			item["target"] = repositoryTargetCloneTarget(target)
		}
		if lock := existingExecutionLock(ctx, server, issueExecutionScope(repo, issue)); lock != nil {
			return map[string]any{
				"status":             "locked",
				"repositoryTargetId": text(target, "id"),
				"lock":               lock,
			}, http.StatusOK, nil
		}
		if workItemExistsForExternalRef(database, text(item, "sourceExternalRef")) || findByID(database.Tables.WorkItems, text(item, "id")) >= 0 {
			continue
		}
		database = appendWorkItem(database, item)
		item = cloneMap(database.Tables.WorkItems[len(database.Tables.WorkItems)-1])
		pipeline := makePipelineWithTemplate(item, template)
		database.Tables.Pipelines = append(database.Tables.Pipelines, pipeline)
		pipelineIndex := len(database.Tables.Pipelines) - 1
		touch(&database)
		if err := server.Repo.Save(ctx, database); err != nil {
			return nil, http.StatusInternalServerError, err
		}
		lock := map[string]any{
			"id":                 executionLockID(issueExecutionScope(repo, issue)),
			"scope":              issueExecutionScope(repo, issue),
			"repositoryTargetId": text(target, "id"),
			"sourceExternalRef":  text(item, "sourceExternalRef"),
			"workItemId":         text(item, "id"),
			"pipelineId":         text(pipeline, "id"),
			"status":             "claimed",
			"owner":              "local-app",
			"createdAt":          nowISO(),
			"updatedAt":          nowISO(),
			"expiresAt":          time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339Nano),
			"runnerProcessState": "not-started",
		}
		if err := saveExecutionLock(ctx, server, lock); err != nil {
			return nil, http.StatusInternalServerError, err
		}
		if payload.AutoRun {
			database, pipeline, attempt := beginDevFlowAttempt(database, pipelineIndex, item, pipeline, "orchestrator")
			lock["runnerProcessState"] = "running"
			lock["updatedAt"] = nowISO()
			if err := server.Repo.Save(ctx, database); err != nil {
				return nil, http.StatusInternalServerError, err
			}
			if err := saveExecutionLock(ctx, server, lock); err != nil {
				return nil, http.StatusInternalServerError, err
			}
			server.startDevFlowCycleJob(text(pipeline, "id"), text(attempt, "id"), payload.AutoApproveHuman, payload.AutoMerge, lock)
			return map[string]any{
				"status":             "accepted",
				"repositoryTargetId": text(target, "id"),
				"workItem":           item,
				"pipeline":           pipeline,
				"lock":               lock,
				"attempt":            attempt,
			}, http.StatusOK, nil
		}
		return map[string]any{
			"status":             "claimed",
			"repositoryTargetId": text(target, "id"),
			"workItem":           item,
			"pipeline":           pipeline,
			"lock":               lock,
		}, http.StatusOK, nil
	}

	return map[string]any{
		"status":             "idle",
		"repositoryTargetId": text(target, "id"),
		"reason":             "no eligible open issue found",
	}, http.StatusOK, nil
}

const orchestratorWatcherPrefix = "orchestrator-watcher:"

type orchestratorWatcherUpdate struct {
	Status           string `json:"status"`
	IntervalSeconds  int    `json:"intervalSeconds"`
	AutoRun          *bool  `json:"autoRun"`
	AutoApproveHuman *bool  `json:"autoApproveHuman"`
	AutoMerge        *bool  `json:"autoMerge"`
	Limit            string `json:"limit"`
}

func (server *Server) listOrchestratorWatchers(response http.ResponseWriter, request *http.Request) {
	watchers, err := server.Repo.ListSettings(request.Context(), orchestratorWatcherPrefix)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(response, http.StatusOK, []map[string]any{})
		return
	}
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	for _, watcher := range watchers {
		normalizeOrchestratorWatcher(watcher)
	}
	writeJSON(response, http.StatusOK, watchers)
}

func (server *Server) putOrchestratorWatcher(response http.ResponseWriter, request *http.Request) {
	targetID := strings.TrimPrefix(request.URL.Path, "/orchestrator/watchers/")
	targetID, _ = url.PathUnescape(targetID)
	if targetID == "" {
		writeError(response, http.StatusBadRequest, fmt.Errorf("repository target id is required"))
		return
	}
	var payload orchestratorWatcherUpdate
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	database, err := mustLoad(server, request.Context())
	if err != nil {
		writeError(response, http.StatusNotFound, err)
		return
	}
	target := findRepositoryTarget(database, targetID)
	if target == nil {
		writeError(response, http.StatusNotFound, fmt.Errorf("repository target not found"))
		return
	}
	watcher, err := server.Repo.GetSetting(request.Context(), orchestratorWatcherID(targetID))
	if errors.Is(err, sql.ErrNoRows) {
		watcher = map[string]any{
			"id":                 orchestratorWatcherID(targetID),
			"repositoryTargetId": targetID,
			"createdAt":          nowISO(),
		}
	} else if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	applyOrchestratorWatcherUpdate(watcher, payload)
	if err := server.Repo.SetSetting(request.Context(), orchestratorWatcherID(targetID), watcher); err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	if text(watcher, "status") == "active" {
		server.ensureOrchestratorWatcherLoop()
		go server.runOrchestratorWatcher(context.Background(), watcher, true)
	}
	writeJSON(response, http.StatusOK, watcher)
}

func orchestratorWatcherID(targetID string) string {
	return orchestratorWatcherPrefix + targetID
}

func applyOrchestratorWatcherUpdate(watcher map[string]any, update orchestratorWatcherUpdate) {
	status := strings.ToLower(strings.TrimSpace(update.Status))
	if status == "" {
		status = stringOr(text(watcher, "status"), "active")
	}
	if status != "active" && status != "paused" {
		status = "paused"
	}
	interval := update.IntervalSeconds
	if interval <= 0 {
		interval = intValue(watcher["intervalSeconds"])
	}
	if interval <= 0 {
		interval = 60
	}
	if interval < 1 {
		interval = 1
	}
	watcher["status"] = status
	watcher["intervalSeconds"] = interval
	watcher["limit"] = stringOr(update.Limit, stringOr(text(watcher, "limit"), "20"))
	watcher["autoRun"] = boolPointerValue(update.AutoRun, boolValueDefault(watcher["autoRun"], true))
	watcher["autoApproveHuman"] = boolPointerValue(update.AutoApproveHuman, boolValueDefault(watcher["autoApproveHuman"], false))
	watcher["autoMerge"] = boolPointerValue(update.AutoMerge, boolValueDefault(watcher["autoMerge"], false))
	watcher["updatedAt"] = nowISO()
	normalizeOrchestratorWatcher(watcher)
}

func normalizeOrchestratorWatcher(watcher map[string]any) {
	if text(watcher, "status") == "" {
		watcher["status"] = "paused"
	}
	if intValue(watcher["intervalSeconds"]) <= 0 {
		watcher["intervalSeconds"] = 60
	}
	if text(watcher, "limit") == "" {
		watcher["limit"] = "20"
	}
	if _, ok := watcher["autoRun"]; !ok {
		watcher["autoRun"] = true
	}
	if _, ok := watcher["autoApproveHuman"]; !ok {
		watcher["autoApproveHuman"] = false
	}
	if _, ok := watcher["autoMerge"]; !ok {
		watcher["autoMerge"] = false
	}
}

func boolPointerValue(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func boolValueDefault(value any, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return boolValue(value)
}

func (server *Server) ensureOrchestratorWatcherLoop() {
	server.watcherMu.Lock()
	defer server.watcherMu.Unlock()
	if server.watcherStarted {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	server.watcherCancel = cancel
	server.watcherStarted = true
	go server.orchestratorWatcherLoop(ctx)
}

func (server *Server) orchestratorWatcherLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			server.runDueOrchestratorWatchers(ctx)
		}
	}
}

func (server *Server) runDueOrchestratorWatchers(ctx context.Context) {
	watchers, err := server.Repo.ListSettings(ctx, orchestratorWatcherPrefix)
	if err != nil {
		return
	}
	for _, watcher := range watchers {
		normalizeOrchestratorWatcher(watcher)
		if text(watcher, "status") != "active" {
			continue
		}
		if !orchestratorWatcherDue(watcher, time.Now().UTC()) {
			continue
		}
		server.runOrchestratorWatcher(ctx, watcher, false)
	}
}

func orchestratorWatcherDue(watcher map[string]any, now time.Time) bool {
	lastTickAt := text(watcher, "lastTickAt")
	if lastTickAt == "" {
		return true
	}
	parsed, err := time.Parse(time.RFC3339Nano, lastTickAt)
	if err != nil {
		return true
	}
	interval := time.Duration(intValue(watcher["intervalSeconds"])) * time.Second
	if interval <= 0 {
		interval = 60 * time.Second
	}
	return !now.Before(parsed.Add(interval))
}

func (server *Server) runOrchestratorWatcher(ctx context.Context, watcher map[string]any, force bool) {
	normalizeOrchestratorWatcher(watcher)
	if text(watcher, "status") != "active" {
		return
	}
	if !force && !orchestratorWatcherDue(watcher, time.Now().UTC()) {
		return
	}
	watcher["lastTickAt"] = nowISO()
	result, _, err := server.runOrchestratorTick(ctx, orchestratorTickPayload{
		RepositoryTargetID: text(watcher, "repositoryTargetId"),
		Limit:              text(watcher, "limit"),
		AutoRun:            boolValueDefault(watcher["autoRun"], true),
		AutoApproveHuman:   boolValueDefault(watcher["autoApproveHuman"], false),
		AutoMerge:          boolValueDefault(watcher["autoMerge"], false),
	})
	if err != nil {
		watcher["lastTickStatus"] = "error"
		watcher["lastError"] = err.Error()
	} else {
		watcher["lastTickStatus"] = text(result, "status")
		watcher["lastTickReason"] = text(result, "reason")
		watcher["lastError"] = ""
	}
	watcher["updatedAt"] = nowISO()
	_ = server.Repo.SetSetting(context.Background(), orchestratorWatcherID(text(watcher, "repositoryTargetId")), watcher)
}

func listGitHubIssues(ctx context.Context, repo string, limit string) ([]map[string]any, error) {
	command := exec.CommandContext(ctx, "gh", "issue", "list", "--repo", repo, "--state", "open", "--limit", limit, "--json", "number,title,body,state,labels,assignees,url")
	output, err := command.Output()
	if err != nil {
		return nil, err
	}
	var issues []map[string]any
	if err := json.Unmarshal(output, &issues); err != nil {
		return nil, err
	}
	return issues, nil
}

func firstRepositoryTarget(database WorkspaceDatabase) map[string]any {
	for _, project := range database.Tables.Projects {
		for _, target := range arrayMaps(project["repositoryTargets"]) {
			if text(target, "id") != "" {
				return target
			}
		}
	}
	return nil
}

func workItemExistsForExternalRef(database WorkspaceDatabase, externalRef string) bool {
	if externalRef == "" {
		return false
	}
	for _, item := range database.Tables.WorkItems {
		if text(item, "sourceExternalRef") == externalRef {
			return true
		}
	}
	return false
}

func isExecutableGitHubIssue(issue map[string]any) bool {
	readyLabels := map[string]bool{
		"omega-ready":   true,
		"devflow-ready": true,
		"agent-ready":   true,
		"omega-run":     true,
	}
	for _, label := range arrayMaps(issue["labels"]) {
		if readyLabels[strings.ToLower(text(label, "name"))] {
			return true
		}
	}
	return false
}

func issueExecutionScope(repo string, issue map[string]any) string {
	number := intValue(issue["number"])
	return fmt.Sprintf("github-issue:%s#%d", repo, number)
}

func executionLockID(scope string) string {
	return "execution-lock:" + strings.NewReplacer("/", "_", "#", "_", ":", "_").Replace(scope)
}

func existingExecutionLock(ctx context.Context, server *Server, scope string) map[string]any {
	lock, err := server.Repo.GetSetting(ctx, executionLockID(scope))
	if err != nil {
		return nil
	}
	if text(lock, "status") == "released" || text(lock, "status") == "expired" {
		return nil
	}
	if expiresAt := text(lock, "expiresAt"); expiresAt != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, expiresAt); err == nil && time.Now().UTC().After(parsed) {
			lock["status"] = "expired"
			lock["updatedAt"] = nowISO()
			_ = saveExecutionLock(ctx, server, lock)
			return nil
		}
	}
	return lock
}

func saveExecutionLock(ctx context.Context, server *Server, lock map[string]any) error {
	return server.Repo.SetSetting(ctx, text(lock, "id"), lock)
}

func (server *Server) listExecutionLocks(response http.ResponseWriter, request *http.Request) {
	locks, err := server.Repo.ListSettings(request.Context(), "execution-lock:")
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, locks)
}

func (server *Server) releaseExecutionLock(response http.ResponseWriter, request *http.Request) {
	lockID := strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, "/execution-locks/"), "/release")
	lockID, _ = url.PathUnescape(lockID)
	if lockID == "" {
		writeError(response, http.StatusBadRequest, fmt.Errorf("execution lock id is required"))
		return
	}
	lock, err := server.Repo.GetSetting(request.Context(), lockID)
	if err != nil {
		writeError(response, http.StatusNotFound, fmt.Errorf("execution lock not found"))
		return
	}
	lock["status"] = "released"
	lock["releasedAt"] = nowISO()
	lock["updatedAt"] = nowISO()
	if err := saveExecutionLock(request.Context(), server, lock); err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, lock)
}
