package omegalocal

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Server struct {
	Repo           *SQLiteRepository
	WorkspaceRoot  string
	OpenAPIPath    string
	GitHubOAuth    GitHubOAuthConfig
	HTTPClient     *http.Client
	CommandStarter func(name string, args ...string) error
	watcherMu      sync.Mutex
	watcherStarted bool
	watcherCancel  context.CancelFunc
	jobMu          sync.Mutex
	attemptCancels map[string]context.CancelFunc
	previewMu      sync.Mutex
	previewRuntime map[string]*previewRuntimeSession
}

type GitHubOAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	TokenURL     string
}

func NewServer(databasePath, workspaceRoot, openAPIPath string) *Server {
	return &Server{
		Repo:          NewSQLiteRepository(databasePath),
		WorkspaceRoot: workspaceRoot,
		OpenAPIPath:   openAPIPath,
		GitHubOAuth: GitHubOAuthConfig{
			ClientID:     os.Getenv("OMEGA_GITHUB_CLIENT_ID"),
			ClientSecret: os.Getenv("OMEGA_GITHUB_CLIENT_SECRET"),
			RedirectURI:  stringOr(os.Getenv("OMEGA_GITHUB_REDIRECT_URI"), defaultGitHubRedirectURI()),
			TokenURL:     stringOr(os.Getenv("OMEGA_GITHUB_TOKEN_URL"), defaultGitHubTokenURL()),
		},
		HTTPClient:     http.DefaultClient,
		attemptCancels: map[string]context.CancelFunc{},
		previewRuntime: map[string]*previewRuntimeSession{},
	}
}

func (server *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", server.route)
	return cors(mux)
}

func (server *Server) route(response http.ResponseWriter, request *http.Request) {
	if request.Method == http.MethodOptions {
		response.WriteHeader(http.StatusNoContent)
		return
	}

	path := request.URL.Path
	requestID := fmt.Sprintf("req_%d", time.Now().UnixNano())
	startedAt := time.Now()
	logger := &loggingResponseWriter{ResponseWriter: response}
	response = logger
	response.Header().Set("x-omega-request-id", requestID)
	defer func() {
		status := logger.status
		if status == 0 {
			status = http.StatusOK
		}
		level := runtimeLogLevelForStatus(status, request.Method)
		server.logRuntime(context.Background(), level, "api.request", fmt.Sprintf("%s %s -> %d", request.Method, path, status), map[string]any{
			"requestId":  requestID,
			"method":     request.Method,
			"path":       path,
			"status":     status,
			"bytes":      logger.bytes,
			"durationMs": time.Since(startedAt).Milliseconds(),
		})
	}()
	switch {
	case request.Method == http.MethodGet && path == "/health":
		writeJSON(response, http.StatusOK, map[string]any{"ok": true, "implementation": "go", "persistence": "sqlite", "databasePath": server.Repo.Path})
	case request.Method == http.MethodGet && path == "/openapi.yaml":
		server.openAPI(response)
	case request.Method == http.MethodGet && path == "/workspace":
		server.getWorkspace(response, request)
	case request.Method == http.MethodPut && path == "/workspace":
		server.putWorkspace(response, request)
	case request.Method == http.MethodGet && path == "/requirements":
		server.listTable(response, request, "requirements")
	case request.Method == http.MethodGet && path == "/events":
		server.listTable(response, request, "missionEvents")
	case request.Method == http.MethodGet && path == "/pipelines":
		server.listTable(response, request, "pipelines")
	case request.Method == http.MethodGet && path == "/attempts":
		server.listTable(response, request, "attempts")
	case request.Method == http.MethodGet && strings.HasPrefix(path, "/attempts/") && strings.HasSuffix(path, "/action-plan"):
		server.attemptActionPlan(response, request)
	case request.Method == http.MethodGet && strings.HasPrefix(path, "/attempts/") && strings.HasSuffix(path, "/timeline"):
		server.attemptTimeline(response, request)
	case request.Method == http.MethodPost && strings.HasPrefix(path, "/attempts/") && strings.HasSuffix(path, "/retry"):
		server.retryAttempt(response, request)
	case request.Method == http.MethodPost && strings.HasPrefix(path, "/attempts/") && strings.HasSuffix(path, "/cancel"):
		server.cancelAttempt(response, request)
	case request.Method == http.MethodGet && path == "/checkpoints":
		server.listTable(response, request, "checkpoints")
	case request.Method == http.MethodGet && path == "/missions":
		server.listTable(response, request, "missions")
	case request.Method == http.MethodGet && path == "/operations":
		server.listTable(response, request, "operations")
	case request.Method == http.MethodGet && path == "/proof-records":
		server.listTable(response, request, "proofRecords")
	case request.Method == http.MethodGet && path == "/run-workpads":
		server.listTable(response, request, "runWorkpads")
	case request.Method == http.MethodPatch && strings.HasPrefix(path, "/run-workpads/"):
		server.patchRunWorkpad(response, request)
	case request.Method == http.MethodGet && path == "/execution-locks":
		server.listExecutionLocks(response, request)
	case request.Method == http.MethodPost && strings.HasPrefix(path, "/execution-locks/") && strings.HasSuffix(path, "/release"):
		server.releaseExecutionLock(response, request)
	case request.Method == http.MethodPost && path == "/workspaces/cleanup":
		server.cleanupWorkspaces(response, request)
	case request.Method == http.MethodPost && path == "/job-supervisor/tick":
		server.jobSupervisorTick(response, request)
	case request.Method == http.MethodGet && path == "/migrations":
		server.listMigrations(response, request)
	case request.Method == http.MethodGet && path == "/pipeline-templates":
		writeJSON(response, http.StatusOK, pipelineTemplates())
	case request.Method == http.MethodGet && path == "/workflow-templates":
		writeJSON(response, http.StatusOK, pipelineTemplates())
	case request.Method == http.MethodPost && path == "/pipelines/from-template":
		server.createPipelineFromTemplate(response, request)
	case request.Method == http.MethodGet && path == "/llm-providers":
		writeJSON(response, http.StatusOK, llmProviders())
	case request.Method == http.MethodGet && path == "/llm-provider-selection":
		server.getProviderSelection(response, request)
	case request.Method == http.MethodPut && path == "/llm-provider-selection":
		server.putProviderSelection(response, request)
	case request.Method == http.MethodGet && path == "/agent-definitions":
		server.listAgentDefinitions(response, request)
	case request.Method == http.MethodGet && path == "/agent-profile":
		server.getAgentProfile(response, request)
	case request.Method == http.MethodPut && path == "/agent-profile":
		server.putAgentProfile(response, request)
	case request.Method == http.MethodGet && path == "/observability":
		server.observability(response, request)
	case request.Method == http.MethodGet && path == "/runtime-logs":
		server.runtimeLogs(response, request)
	case request.Method == http.MethodGet && path == "/runtime-logs/export":
		server.runtimeLogsExport(response, request)
	case request.Method == http.MethodGet && path == "/local-capabilities":
		server.localCapabilities(response, request)
	case request.Method == http.MethodGet && path == "/local-workspace-root":
		server.getLocalWorkspaceRoot(response, request)
	case request.Method == http.MethodPut && path == "/local-workspace-root":
		server.putLocalWorkspaceRoot(response, request)
	case request.Method == http.MethodPost && path == "/page-pilot/apply":
		server.pagePilotApply(response, request)
	case request.Method == http.MethodPost && path == "/page-pilot/deliver":
		server.pagePilotDeliver(response, request)
	case request.Method == http.MethodGet && path == "/page-pilot/runs":
		server.listPagePilotRuns(response, request)
	case request.Method == http.MethodPost && strings.HasPrefix(path, "/page-pilot/runs/") && strings.HasSuffix(path, "/discard"):
		server.discardPagePilotRun(response, request)
	case request.Method == http.MethodPost && path == "/page-pilot/preview-runtime/resolve":
		server.pagePilotPreviewRuntimeResolve(response, request)
	case request.Method == http.MethodPost && path == "/page-pilot/preview-runtime/start":
		server.pagePilotPreviewRuntimeStart(response, request)
	case request.Method == http.MethodPost && path == "/page-pilot/preview-runtime/restart":
		server.pagePilotPreviewRuntimeRestart(response, request)
	case request.Method == http.MethodGet && path == "/github/status":
		server.githubStatus(response, request)
	case request.Method == http.MethodGet && path == "/github/oauth/config":
		server.githubOAuthConfig(response, request)
	case request.Method == http.MethodPut && path == "/github/oauth/config":
		server.putGitHubOAuthConfig(response, request)
	case request.Method == http.MethodPost && path == "/github/oauth/start":
		server.githubOAuthStart(response, request)
	case request.Method == http.MethodPost && path == "/github/cli-login/start":
		server.githubCliLoginStart(response, request)
	case request.Method == http.MethodGet && path == "/auth/github/callback":
		server.githubOAuthCallback(response, request)
	case request.Method == http.MethodGet && path == "/github/repositories":
		server.githubRepositories(response, request)
	case request.Method == http.MethodPost && path == "/github/repo-info":
		server.githubRepoInfo(response, request)
	case request.Method == http.MethodPost && path == "/github/bind-repository-target":
		server.githubBindRepositoryTarget(response, request)
	case request.Method == http.MethodDelete && strings.HasPrefix(path, "/github/repository-targets/"):
		server.githubDeleteRepositoryTarget(response, request)
	case request.Method == http.MethodPost && path == "/github/import-issues":
		server.githubImportIssues(response, request)
	case request.Method == http.MethodPost && path == "/github/create-pr":
		server.githubCreatePR(response, request)
	case request.Method == http.MethodPost && path == "/github/pr-status":
		server.githubPRStatus(response, request)
	case request.Method == http.MethodPost && path == "/feishu/notify":
		server.feishuNotify(response, request)
	case request.Method == http.MethodPost && path == "/feishu/review-request":
		server.feishuReviewRequest(response, request)
	case request.Method == http.MethodPost && path == "/feishu/review-callback":
		server.feishuReviewCallback(response, request)
	case request.Method == http.MethodPost && path == "/requirements/decompose":
		server.decomposeRequirement(response, request)
	case request.Method == http.MethodPost && path == "/orchestrator/tick":
		server.orchestratorTick(response, request)
	case request.Method == http.MethodGet && path == "/orchestrator/watchers":
		server.listOrchestratorWatchers(response, request)
	case request.Method == http.MethodPut && strings.HasPrefix(path, "/orchestrator/watchers/"):
		server.putOrchestratorWatcher(response, request)
	case request.Method == http.MethodPost && path == "/work-items":
		server.createWorkItem(response, request)
	case request.Method == http.MethodDelete && strings.HasPrefix(path, "/work-items/"):
		server.deleteWorkItem(response, request)
	case request.Method == http.MethodPatch && strings.HasPrefix(path, "/work-items/"):
		server.patchWorkItem(response, request)
	case request.Method == http.MethodPost && path == "/pipelines/from-work-item":
		server.createPipeline(response, request)
	case request.Method == http.MethodPost && strings.HasPrefix(path, "/pipelines/") && strings.HasSuffix(path, "/run-devflow-cycle"):
		server.runDevFlowCycle(response, request)
	case request.Method == http.MethodPost && strings.HasPrefix(path, "/pipelines/") && strings.HasSuffix(path, "/start"):
		server.startPipeline(response, request)
	case request.Method == http.MethodPost && strings.HasPrefix(path, "/pipelines/") && strings.HasSuffix(path, "/run-current-stage"):
		server.runCurrentPipelineStage(response, request)
	case request.Method == http.MethodPost && strings.HasPrefix(path, "/pipelines/") && strings.HasSuffix(path, "/complete-stage"):
		server.completePipelineStage(response, request)
	case request.Method == http.MethodPost && strings.HasPrefix(path, "/pipelines/") && strings.HasSuffix(path, "/pause"):
		server.setPipelineStatus(response, request, "paused")
	case request.Method == http.MethodPost && strings.HasPrefix(path, "/pipelines/") && strings.HasSuffix(path, "/resume"):
		server.setPipelineStatus(response, request, "running")
	case request.Method == http.MethodPost && strings.HasPrefix(path, "/pipelines/") && strings.HasSuffix(path, "/terminate"):
		server.setPipelineStatus(response, request, "terminated")
	case request.Method == http.MethodPost && strings.HasPrefix(path, "/checkpoints/") && strings.HasSuffix(path, "/approve"):
		server.decideCheckpoint(response, request, "approved")
	case request.Method == http.MethodPost && strings.HasPrefix(path, "/checkpoints/") && strings.HasSuffix(path, "/request-changes"):
		server.decideCheckpoint(response, request, "rejected")
	case request.Method == http.MethodPost && path == "/missions/from-work-item":
		server.createMission(response, request)
	case request.Method == http.MethodPost && path == "/operations/run":
		server.runOperation(response, request, true)
	case request.Method == http.MethodPost && path == "/run-operation":
		server.runOperation(response, request, false)
	default:
		writeJSON(response, http.StatusNotFound, map[string]any{"error": "not found"})
	}
}

func (server *Server) observability(response http.ResponseWriter, request *http.Request) {
	database, err := server.Repo.Load(request.Context())
	if errors.Is(err, sql.ErrNoRows) {
		summary := emptyObservability()
		if logs, logErr := server.Repo.ListRuntimeLogs(request.Context(), map[string]string{"level": "ERROR"}, 5); logErr == nil {
			summary["recentErrors"] = logs
			mapValue(summary["counts"])["runtimeLogs"] = len(logs)
		}
		writeJSON(response, http.StatusOK, summary)
		return
	}
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	summary := observabilitySummary(*database)
	if logs, logErr := server.Repo.ListRuntimeLogs(request.Context(), map[string]string{}, 50); logErr == nil {
		mapValue(summary["counts"])["runtimeLogs"] = len(logs)
	}
	if errors, logErr := server.Repo.ListRuntimeLogs(request.Context(), map[string]string{"level": "ERROR"}, 5); logErr == nil {
		summary["recentErrors"] = errors
	}
	writeJSON(response, http.StatusOK, summary)
}

func (server *Server) localCapabilities(response http.ResponseWriter, request *http.Request) {
	writeJSON(response, http.StatusOK, detectLocalCapabilities(request.Context()))
}

func (server *Server) getProviderSelection(response http.ResponseWriter, request *http.Request) {
	record, err := server.Repo.GetSetting(request.Context(), providerSelectionSettingKey)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(response, http.StatusOK, defaultProviderSelection())
		return
	}
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, providerSelectionFromMap(record))
}

func (server *Server) putProviderSelection(response http.ResponseWriter, request *http.Request) {
	var selection ProviderSelection
	if err := json.NewDecoder(request.Body).Decode(&selection); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	if !validateProviderSelection(selection) {
		writeJSON(response, http.StatusBadRequest, map[string]any{"error": "invalid provider selection"})
		return
	}
	if err := server.Repo.SetSetting(request.Context(), providerSelectionSettingKey, providerSelectionToMap(selection)); err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, selection)
}

func (server *Server) listAgentDefinitions(response http.ResponseWriter, request *http.Request) {
	record, err := server.Repo.GetSetting(request.Context(), providerSelectionSettingKey)
	selection := defaultProviderSelection()
	if err == nil {
		selection = providerSelectionFromMap(record)
	}
	writeJSON(response, http.StatusOK, agentDefinitions(selection))
}

func (server *Server) openAPI(response http.ResponseWriter) {
	raw, err := os.ReadFile(server.OpenAPIPath)
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	response.Header().Set("content-type", "application/yaml; charset=utf-8")
	response.WriteHeader(http.StatusOK)
	_, _ = response.Write(raw)
}

func (server *Server) getWorkspace(response http.ResponseWriter, request *http.Request) {
	database, err := server.Repo.Load(request.Context())
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(response, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, database)
}

func (server *Server) putWorkspace(response http.ResponseWriter, request *http.Request) {
	var database WorkspaceDatabase
	if err := json.NewDecoder(request.Body).Decode(&database); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	server.logDebug(request.Context(), "workspace.save.requested", "Workspace snapshot save requested.", map[string]any{
		"entityType": "workspace",
		"workItems":  len(database.Tables.WorkItems),
		"pipelines":  len(database.Tables.Pipelines),
		"attempts":   len(database.Tables.Attempts),
	})
	if err := server.Repo.Save(request.Context(), database); err != nil {
		server.logError(request.Context(), "workspace.save.failed", err.Error(), map[string]any{"entityType": "workspace"})
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"ok": true})
}

func (server *Server) listTable(response http.ResponseWriter, request *http.Request, table string) {
	database, err := server.Repo.Load(request.Context())
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(response, http.StatusOK, []map[string]any{})
		return
	}
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	if table == "checkpoints" {
		if summary := server.reconcileAttemptIntegrityInDatabase(request.Context(), database); intValue(summary["changed"]) > 0 {
			if err := server.Repo.Save(request.Context(), *database); err != nil {
				writeError(response, http.StatusInternalServerError, err)
				return
			}
		}
	}
	switch table {
	case "requirements":
		writeJSON(response, http.StatusOK, database.Tables.Requirements)
	case "missionEvents":
		writeJSON(response, http.StatusOK, database.Tables.MissionEvents)
	case "pipelines":
		writeJSON(response, http.StatusOK, database.Tables.Pipelines)
	case "attempts":
		writeJSON(response, http.StatusOK, database.Tables.Attempts)
	case "checkpoints":
		writeJSON(response, http.StatusOK, database.Tables.Checkpoints)
	case "missions":
		writeJSON(response, http.StatusOK, database.Tables.Missions)
	case "operations":
		writeJSON(response, http.StatusOK, database.Tables.Operations)
	case "proofRecords":
		writeJSON(response, http.StatusOK, database.Tables.ProofRecords)
	case "runWorkpads":
		writeJSON(response, http.StatusOK, filterRunWorkpads(database.Tables.RunWorkpads, request.URL.Query()))
	}
}

func (server *Server) cancelAttempt(response http.ResponseWriter, request *http.Request) {
	attemptID := strings.TrimSuffix(pathID(request.URL.Path), "/cancel")
	var payload struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(request.Body).Decode(&payload)
	cancelSignalSent := server.cancelRegisteredAttemptJob(attemptID)
	database, err := mustLoad(server, request.Context())
	if err != nil {
		writeError(response, http.StatusNotFound, err)
		return
	}
	nextDatabase, attempt := markAttemptCanceled(database, attemptID, stringOr(payload.Reason, "Canceled by operator."))
	if attempt == nil {
		writeJSON(response, http.StatusNotFound, map[string]any{"error": "attempt not found", "cancelSignalSent": cancelSignalSent})
		return
	}
	if err := server.Repo.Save(request.Context(), nextDatabase); err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	server.logInfo(request.Context(), "attempt.canceled", text(attempt, "statusReason"), map[string]any{
		"entityType":       "attempt",
		"entityId":         attemptID,
		"pipelineId":       text(attempt, "pipelineId"),
		"attemptId":        attemptID,
		"workItemId":       text(attempt, "itemId"),
		"cancelSignalSent": cancelSignalSent,
	})
	writeJSON(response, http.StatusOK, map[string]any{"attempt": attempt, "cancelSignalSent": cancelSignalSent})
}

func (server *Server) listMigrations(response http.ResponseWriter, request *http.Request) {
	migrations, err := server.Repo.Migrations(request.Context())
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, migrations)
}

func (server *Server) createWorkItem(response http.ResponseWriter, request *http.Request) {
	database, err := mustLoad(server, request.Context())
	if err != nil {
		writeError(response, http.StatusNotFound, err)
		return
	}
	var payload struct {
		Item map[string]any `json:"item"`
	}
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	next := appendWorkItem(database, payload.Item)
	if err := server.Repo.Save(request.Context(), next); err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, next)
}

func (server *Server) patchWorkItem(response http.ResponseWriter, request *http.Request) {
	itemID := strings.TrimPrefix(request.URL.Path, "/work-items/")
	database, err := mustLoad(server, request.Context())
	if err != nil {
		writeError(response, http.StatusNotFound, err)
		return
	}
	var patch map[string]any
	if err := json.NewDecoder(request.Body).Decode(&patch); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	next := updateWorkItem(database, itemID, patch)
	if err := server.Repo.Save(request.Context(), next); err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, next)
}

func (server *Server) deleteWorkItem(response http.ResponseWriter, request *http.Request) {
	itemID := strings.TrimPrefix(request.URL.Path, "/work-items/")
	database, err := mustLoad(server, request.Context())
	if err != nil {
		writeError(response, http.StatusNotFound, err)
		return
	}
	next, deleted, reason := deleteWorkItemRecord(database, itemID)
	if !deleted {
		status := http.StatusConflict
		if reason == "work item not found" {
			status = http.StatusNotFound
		}
		writeJSON(response, status, map[string]any{"error": reason})
		return
	}
	if err := server.Repo.Save(request.Context(), next); err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, next)
}

func (server *Server) createPipeline(response http.ResponseWriter, request *http.Request) {
	database, err := mustLoad(server, request.Context())
	if err != nil {
		writeError(response, http.StatusNotFound, err)
		return
	}
	var payload struct {
		Item map[string]any `json:"item"`
	}
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	pipeline := makePipeline(payload.Item)
	target := findRepositoryTarget(database, text(payload.Item, "repositoryTargetId"))
	profile := server.resolveAgentProfile(request.Context(), database, payload.Item, target)
	pipeline = attachAgentProfileToPipeline(pipeline, profile)
	database.Tables.Pipelines = append(database.Tables.Pipelines, pipeline)
	touch(&database)
	if err := server.Repo.Save(request.Context(), database); err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, pipeline)
}

func (server *Server) createPipelineFromTemplate(response http.ResponseWriter, request *http.Request) {
	database, err := mustLoad(server, request.Context())
	if err != nil {
		writeError(response, http.StatusNotFound, err)
		return
	}
	var payload struct {
		Item       map[string]any `json:"item"`
		TemplateID string         `json:"templateId"`
	}
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	template := findPipelineTemplate(payload.TemplateID)
	if template == nil {
		writeJSON(response, http.StatusNotFound, map[string]any{"error": "pipeline template not found"})
		return
	}
	pipeline := makePipelineWithTemplate(payload.Item, template)
	target := findRepositoryTarget(database, text(payload.Item, "repositoryTargetId"))
	profile := server.resolveAgentProfile(request.Context(), database, payload.Item, target)
	pipeline = attachAgentProfileToPipeline(pipeline, profile)
	if pipelineIndex := findByID(database.Tables.Pipelines, text(pipeline, "id")); pipelineIndex >= 0 {
		database.Tables.Pipelines[pipelineIndex] = pipeline
	} else {
		database.Tables.Pipelines = append(database.Tables.Pipelines, pipeline)
	}
	touch(&database)
	if err := server.Repo.Save(request.Context(), database); err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, pipeline)
}

func (server *Server) runDevFlowCycle(response http.ResponseWriter, request *http.Request) {
	var payload struct {
		AutoApproveHuman bool `json:"autoApproveHuman"`
		AutoMerge        bool `json:"autoMerge"`
		Wait             bool `json:"wait"`
	}
	_ = json.NewDecoder(request.Body).Decode(&payload)
	pipelineID := pathID(request.URL.Path)
	database, err := mustLoad(server, request.Context())
	if err != nil {
		writeError(response, http.StatusNotFound, err)
		return
	}
	pipelineIndex := findByID(database.Tables.Pipelines, pipelineID)
	if pipelineIndex < 0 {
		writeJSON(response, http.StatusNotFound, map[string]any{"error": "pipeline not found"})
		return
	}
	pipeline := cloneMap(database.Tables.Pipelines[pipelineIndex])
	if !isDevFlowPRTemplate(text(pipeline, "templateId")) {
		writeJSON(response, http.StatusConflict, map[string]any{"error": "pipeline is not using the devflow-pr template"})
		return
	}
	item := findWorkItem(database, text(pipeline, "workItemId"))
	if item == nil {
		writeJSON(response, http.StatusNotFound, map[string]any{"error": "work item not found"})
		return
	}
	stageItem, err := resolveWorkItemRepositoryTarget(database, item)
	if err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	target := findRepositoryTarget(database, text(stageItem, "repositoryTargetId"))
	if target == nil {
		writeError(response, http.StatusBadRequest, fmt.Errorf("work item %s has no repository workspace", text(stageItem, "key")))
		return
	}
	profile := server.resolveAgentProfile(request.Context(), database, stageItem, target)
	preflight := server.preflightDevFlowRun(request.Context(), database, stageItem, target, profile)
	if !preflight.ok() {
		server.logError(request.Context(), "devflow.preflight.failed", strings.Join(preflight.Errors, "; "), map[string]any{"workItemId": text(stageItem, "id"), "pipelineId": text(pipeline, "id"), "repositoryTargetId": text(stageItem, "repositoryTargetId")})
		writeJSON(response, http.StatusBadRequest, map[string]any{"error": "DevFlow preflight failed", "preflight": preflight})
		return
	}
	var attempt map[string]any
	database, pipeline, attempt = beginDevFlowAttempt(database, pipelineIndex, stageItem, pipeline, "manual")
	lock, lockErr := claimDevFlowWorkspaceLock(request.Context(), server, stageItem, target, pipeline, attempt)
	if lockErr != nil {
		server.logError(request.Context(), "devflow.workspace.lock_failed", lockErr.Error(), map[string]any{"workItemId": text(stageItem, "id"), "pipelineId": text(pipeline, "id"), "attemptId": text(attempt, "id"), "repositoryTargetId": text(stageItem, "repositoryTargetId")})
		writeJSON(response, http.StatusConflict, map[string]any{"error": lockErr.Error()})
		return
	}
	server.logInfo(request.Context(), "devflow.attempt.created", "DevFlow attempt created.", map[string]any{
		"entityType":         "attempt",
		"entityId":           text(attempt, "id"),
		"projectId":          text(stageItem, "projectId"),
		"repositoryTargetId": text(stageItem, "repositoryTargetId"),
		"workItemId":         text(stageItem, "id"),
		"pipelineId":         text(pipeline, "id"),
		"attemptId":          text(attempt, "id"),
	})
	if err := server.Repo.Save(request.Context(), database); err != nil {
		nextLock := cloneMap(lock)
		nextLock["status"] = "released"
		nextLock["runnerProcessState"] = "failed"
		nextLock["releasedAt"] = nowISO()
		nextLock["updatedAt"] = nowISO()
		_ = saveExecutionLock(context.Background(), server, nextLock)
		server.logError(request.Context(), "devflow.attempt.create_failed", err.Error(), map[string]any{"pipelineId": text(pipeline, "id"), "attemptId": text(attempt, "id"), "workItemId": text(stageItem, "id")})
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	if !payload.Wait {
		server.startDevFlowCycleJob(text(pipeline, "id"), text(attempt, "id"), payload.AutoApproveHuman, payload.AutoMerge, lock)
		writeJSON(response, http.StatusAccepted, map[string]any{
			"status":   "accepted",
			"pipeline": pipeline,
			"attempt":  attempt,
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
	result, err := server.executeDevFlowPRCycle(runContext, pipeline, stageItem, target, text(attempt, "id"), payload.AutoApproveHuman, payload.AutoMerge)
	if err != nil {
		lockProcessState = "failed"
		_ = server.failDevFlowCycleJobWithResult(context.Background(), text(pipeline, "id"), text(attempt, "id"), err, result)
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	pipeline, _, err = server.completeDevFlowCycleJob(context.Background(), text(pipeline, "id"), text(attempt, "id"), result)
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	result["pipeline"] = pipeline
	writeJSON(response, http.StatusOK, result)
}

func (server *Server) startPipeline(response http.ResponseWriter, request *http.Request) {
	server.mutatePipeline(response, request, func(pipeline map[string]any) map[string]any {
		run := mapValue(pipeline["run"])
		stage := firstStageWithStatus(run, "ready")
		if stage != nil {
			stage["status"] = "running"
			stage["startedAt"] = nowISO()
			appendRunEvent(run, "stage.started", fmt.Sprintf("%s started", text(stage, "title")), text(stage, "id"), text(stage, "ownerAgentId"))
		}
		pipeline["run"] = run
		pipeline["status"] = "running"
		pipeline["updatedAt"] = nowISO()
		return pipeline
	})
}

func (server *Server) completePipelineStage(response http.ResponseWriter, request *http.Request) {
	var payload struct {
		Passed *bool  `json:"passed"`
		Notes  string `json:"notes"`
	}
	_ = json.NewDecoder(request.Body).Decode(&payload)
	passed := true
	if payload.Passed != nil {
		passed = *payload.Passed
	}
	server.mutatePipeline(response, request, func(pipeline map[string]any) map[string]any {
		run := mapValue(pipeline["run"])
		stage := firstStageWithStatus(run, "running")
		if stage == nil {
			pipeline["status"] = "failed"
			return pipeline
		}
		stage["notes"] = payload.Notes
		stage["completedAt"] = nowISO()
		if !passed {
			stage["status"] = "failed"
			pipeline["status"] = "failed"
			appendRunEvent(run, "stage.failed", fmt.Sprintf("%s failed", text(stage, "title")), text(stage, "id"), text(stage, "ownerAgentId"))
		} else if boolValue(stage["humanGate"]) {
			stage["status"] = "needs-human"
			pipeline["status"] = "waiting-human"
			appendRunEvent(run, "gate.requested", fmt.Sprintf("%s is waiting for human approval", text(stage, "title")), text(stage, "id"), text(stage, "ownerAgentId"))
		} else {
			stage["status"] = "passed"
			markNextStageReady(run, text(stage, "id"))
			pipeline["status"] = pipelineStatusFromRun(run)
			appendRunEvent(run, "stage.completed", fmt.Sprintf("%s passed", text(stage, "title")), text(stage, "id"), text(stage, "ownerAgentId"))
		}
		pipeline["run"] = run
		pipeline["updatedAt"] = nowISO()
		return pipeline
	})
}

func (server *Server) runCurrentPipelineStage(response http.ResponseWriter, request *http.Request) {
	var payload struct {
		Runner string `json:"runner"`
	}
	_ = json.NewDecoder(request.Body).Decode(&payload)
	if payload.Runner == "" {
		payload.Runner = "local-proof"
	}
	pipelineID := pathID(request.URL.Path)
	database, err := mustLoad(server, request.Context())
	if err != nil {
		writeError(response, http.StatusNotFound, err)
		return
	}
	index := findByID(database.Tables.Pipelines, pipelineID)
	if index < 0 {
		writeJSON(response, http.StatusNotFound, map[string]any{"error": "pipeline not found"})
		return
	}
	pipeline := cloneMap(database.Tables.Pipelines[index])
	run := mapValue(pipeline["run"])
	stage := firstStageWithStatus(run, "running")
	if stage == nil {
		stage = firstStageWithStatus(run, "ready")
		if stage != nil {
			stage["status"] = "running"
			stage["startedAt"] = nowISO()
			appendRunEvent(run, "stage.started", fmt.Sprintf("%s started", text(stage, "title")), text(stage, "id"), text(stage, "ownerAgentId"))
		}
	}
	if stage == nil {
		writeJSON(response, http.StatusConflict, map[string]any{"error": "pipeline has no runnable stage"})
		return
	}
	item := findWorkItem(database, text(pipeline, "workItemId"))
	if item == nil {
		writeJSON(response, http.StatusNotFound, map[string]any{"error": "work item not found"})
		return
	}
	stageItem, err := resolveWorkItemRepositoryTarget(database, item)
	if err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	target := findRepositoryTarget(database, text(stageItem, "repositoryTargetId"))
	profile := server.resolveAgentProfile(request.Context(), database, stageItem, target)
	stageItem["stageId"] = stage["id"]
	stageItem["assignee"] = stage["ownerAgentId"]
	if _, err := preflightAgentRunner(payload.Runner, profile, text(stage, "ownerAgentId")); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	mission := makeMission(stageItem)
	operationID := fmt.Sprintf("operation_%s", text(stage, "id"))
	attempt := makeAttemptRecord(stageItem, pipeline, "manual", payload.Runner, text(stage, "id"))
	database.Tables.Attempts = appendOrReplace(database.Tables.Attempts, attempt)
	upsertRunWorkpad(&database, text(attempt, "id"))

	upsertMissionAndOperation(&database, mission, text(pipeline, "id"))
	result, err := server.runLocalProof(mission, operationID, payload.Runner, profile)
	if err != nil {
		database, _ = failAttemptRecord(database, text(attempt, "id"), pipeline, err.Error(), nil)
		upsertRunWorkpad(&database, text(attempt, "id"))
		touch(&database)
		_ = server.Repo.Save(context.Background(), database)
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	applyMissionEvents(&database, result.Events)
	nextOperationStatus := "done"
	if result.Status != "passed" {
		nextOperationStatus = "failed"
	}
	upsertOperationStatus(&database, mission, operationID, nextOperationStatus)
	upsertOperationRunnerProcess(&database, operationID, result.RunnerProcess)
	for proofIndex, proof := range result.ProofFiles {
		database.Tables.ProofRecords = appendOrReplace(database.Tables.ProofRecords, map[string]any{
			"id":          fmt.Sprintf("%s:proof:%d", operationID, proofIndex+1),
			"operationId": operationID,
			"label":       "proof-file",
			"value":       filepath.Base(proof),
			"sourcePath":  proof,
			"createdAt":   nowISO(),
		})
	}

	stage["notes"] = fmt.Sprintf("Runner %s finished with %s", payload.Runner, result.Status)
	stage["completedAt"] = nowISO()
	stage["evidence"] = result.ProofFiles
	if result.Status != "passed" {
		stage["status"] = "failed"
		pipeline["status"] = "failed"
		database = updateWorkItem(database, text(stageItem, "id"), map[string]any{"status": "Blocked"})
		appendRunEvent(run, "stage.failed", fmt.Sprintf("%s failed", text(stage, "title")), text(stage, "id"), text(stage, "ownerAgentId"))
	} else if boolValue(stage["humanGate"]) {
		stage["status"] = "needs-human"
		pipeline["status"] = "waiting-human"
		appendRunEvent(run, "gate.requested", fmt.Sprintf("%s is waiting for human approval", text(stage, "title")), text(stage, "id"), text(stage, "ownerAgentId"))
	} else {
		stage["status"] = "passed"
		markNextStageReady(run, text(stage, "id"))
		pipeline["status"] = pipelineStatusFromRun(run)
		appendRunEvent(run, "stage.completed", fmt.Sprintf("%s passed", text(stage, "title")), text(stage, "id"), text(stage, "ownerAgentId"))
	}
	pipeline["run"] = run
	pipeline["updatedAt"] = nowISO()
	database.Tables.Pipelines[index] = pipeline
	upsertPendingCheckpoint(&database, pipeline)
	if result.Status == "passed" {
		database, _ = completeAttemptRecord(database, text(attempt, "id"), pipeline, map[string]any{
			"status":        pipeline["status"],
			"workspacePath": result.WorkspacePath,
			"branchName":    result.BranchName,
			"changedFiles":  result.ChangedFiles,
			"proofFiles":    result.ProofFiles,
			"stdout":        result.Stdout,
			"stderr":        result.Stderr,
		})
	} else {
		database, _ = failAttemptRecord(database, text(attempt, "id"), pipeline, result.Stderr, nil)
	}
	upsertRunWorkpad(&database, text(attempt, "id"))
	touch(&database)
	if err := server.Repo.Save(context.Background(), database); err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"pipeline": pipeline, "operationResult": result})
}

func (server *Server) setPipelineStatus(response http.ResponseWriter, request *http.Request, status string) {
	server.mutatePipeline(response, request, func(pipeline map[string]any) map[string]any {
		pipeline["status"] = status
		pipeline["updatedAt"] = nowISO()
		return pipeline
	})
}

func (server *Server) mutatePipeline(response http.ResponseWriter, request *http.Request, mutate func(map[string]any) map[string]any) {
	pipelineID := pathID(request.URL.Path)
	database, err := mustLoad(server, request.Context())
	if err != nil {
		writeError(response, http.StatusNotFound, err)
		return
	}
	index := findByID(database.Tables.Pipelines, pipelineID)
	if index < 0 {
		writeJSON(response, http.StatusNotFound, map[string]any{"error": "pipeline not found"})
		return
	}
	pipeline := mutate(cloneMap(database.Tables.Pipelines[index]))
	database.Tables.Pipelines[index] = pipeline
	upsertPendingCheckpoint(&database, pipeline)
	touch(&database)
	if err := server.Repo.Save(request.Context(), database); err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, pipeline)
}

func (server *Server) decideCheckpoint(response http.ResponseWriter, request *http.Request, status string) {
	checkpointID := pathID(request.URL.Path)
	var payload map[string]any
	_ = json.NewDecoder(request.Body).Decode(&payload)
	checkpoint, httpStatus, err := server.applyCheckpointDecision(request.Context(), checkpointID, status, payload)
	if err != nil {
		if httpStatus >= 500 {
			writeError(response, httpStatus, err)
		} else {
			writeJSON(response, httpStatus, map[string]any{"error": err.Error()})
		}
		return
	}
	writeJSON(response, http.StatusOK, checkpoint)
}

func (server *Server) applyCheckpointDecision(ctx context.Context, checkpointID string, status string, payload map[string]any) (map[string]any, int, error) {
	if payload == nil {
		payload = map[string]any{}
	}
	server.logInfo(ctx, "checkpoint.decision.requested", "Checkpoint decision requested.", map[string]any{"entityType": "checkpoint", "entityId": checkpointID, "status": status})
	database, err := mustLoad(server, ctx)
	if err != nil {
		server.logError(ctx, "checkpoint.decision.load_failed", err.Error(), map[string]any{"entityType": "checkpoint", "entityId": checkpointID, "status": status})
		return nil, http.StatusNotFound, err
	}
	server.reconcileAttemptIntegrityInDatabase(ctx, &database)
	index := findByID(database.Tables.Checkpoints, checkpointID)
	if index < 0 {
		server.logError(ctx, "checkpoint.decision.not_found", "Checkpoint not found.", map[string]any{"entityType": "checkpoint", "entityId": checkpointID, "status": status})
		return nil, http.StatusNotFound, fmt.Errorf("checkpoint not found")
	}
	checkpoint := cloneMap(database.Tables.Checkpoints[index])
	checkpoint["status"] = status
	asyncApprovedDelivery := false
	var reworkLock map[string]any
	var reworkPipelineID string
	var reworkAttemptID string
	if status == "approved" {
		reviewer := stringOr(payload["reviewer"], "human")
		checkpoint["decisionNote"] = fmt.Sprintf("approved by %s", reviewer)
		approvePipelineStage(&database, checkpoint)
		asyncApprovedDelivery = boolValue(payload["asyncDelivery"]) && server.canCompleteApprovedDevFlowCheckpointAsync(database, checkpoint)
		if asyncApprovedDelivery {
			markApprovedDevFlowDeliveryQueued(&database, checkpoint)
		} else {
			if err := server.completeApprovedDevFlowCheckpoint(&database, checkpoint, reviewer); err != nil {
				server.logError(ctx, "checkpoint.approve.failed", err.Error(), map[string]any{"entityType": "checkpoint", "entityId": checkpointID, "pipelineId": text(checkpoint, "pipelineId"), "stageId": text(checkpoint, "stageId")})
				return nil, http.StatusInternalServerError, err
			}
		}
	} else {
		reason := stringOr(payload["reason"], "changes requested")
		checkpoint["decisionNote"] = reason
		rejectPipelineStage(&database, checkpoint, reason)
		if text(checkpoint, "stageId") == "human_review" {
			var pipeline map[string]any
			var reworkAttempt map[string]any
			var prepareErr error
			database, pipeline, reworkAttempt, prepareErr = server.prepareDevFlowHumanRequestedRework(ctx, database, checkpoint, reason)
			if prepareErr != nil {
				checkpoint["reworkStatus"] = "failed"
				checkpoint["reworkError"] = prepareErr.Error()
				server.logError(ctx, "checkpoint.rework.prepare_failed", prepareErr.Error(), map[string]any{"entityType": "checkpoint", "entityId": checkpointID, "pipelineId": text(checkpoint, "pipelineId"), "stageId": text(checkpoint, "stageId")})
			} else if text(reworkAttempt, "status") == "waiting-human" {
				checkpoint["reworkStatus"] = "waiting-human"
				checkpoint["reworkAttemptId"] = text(reworkAttempt, "id")
				checkpoint["reworkError"] = text(reworkAttempt, "statusReason")
			} else {
				item := findWorkItem(database, text(pipeline, "workItemId"))
				target := findRepositoryTarget(database, text(item, "repositoryTargetId"))
				lock, lockErr := claimDevFlowWorkspaceLock(ctx, server, item, target, pipeline, reworkAttempt)
				if lockErr != nil {
					database, _ = failAttemptRecord(database, text(reworkAttempt, "id"), pipeline, lockErr.Error(), map[string]any{
						"failureStageId":        "todo",
						"failureAgentId":        "master",
						"failureReason":         "Human-requested rework could not start because the workspace lock was unavailable.",
						"failureReviewFeedback": reason,
					})
					checkpoint["reworkStatus"] = "failed"
					checkpoint["reworkAttemptId"] = text(reworkAttempt, "id")
					checkpoint["reworkError"] = lockErr.Error()
					server.logError(ctx, "checkpoint.rework.lock_failed", lockErr.Error(), map[string]any{"entityType": "checkpoint", "entityId": checkpointID, "pipelineId": text(pipeline, "id"), "attemptId": text(reworkAttempt, "id")})
				} else {
					reworkLock = lock
					reworkPipelineID = text(pipeline, "id")
					reworkAttemptID = text(reworkAttempt, "id")
					checkpoint["reworkStatus"] = "queued"
					checkpoint["reworkAttemptId"] = reworkAttemptID
				}
			}
		}
	}
	checkpoint["updatedAt"] = nowISO()
	database.Tables.Checkpoints[index] = checkpoint
	touch(&database)
	if err := server.Repo.Save(ctx, database); err != nil {
		if reworkLock != nil {
			nextLock := cloneMap(reworkLock)
			nextLock["status"] = "released"
			nextLock["runnerProcessState"] = "failed"
			nextLock["releasedAt"] = nowISO()
			nextLock["updatedAt"] = nowISO()
			_ = saveExecutionLock(context.Background(), server, nextLock)
		}
		server.logError(ctx, "checkpoint.decision.save_failed", err.Error(), map[string]any{"entityType": "checkpoint", "entityId": checkpointID, "pipelineId": text(checkpoint, "pipelineId"), "stageId": text(checkpoint, "stageId")})
		return nil, http.StatusInternalServerError, err
	}
	server.logInfo(ctx, "checkpoint.decision.saved", "Checkpoint decision saved.", map[string]any{"entityType": "checkpoint", "entityId": checkpointID, "pipelineId": text(checkpoint, "pipelineId"), "stageId": text(checkpoint, "stageId"), "status": status})
	if asyncApprovedDelivery {
		reviewer := stringOr(payload["reviewer"], "human")
		server.completeApprovedDevFlowCheckpointInBackground(text(checkpoint, "id"), reviewer)
	}
	if reworkAttemptID != "" {
		server.startDevFlowCycleJob(reworkPipelineID, reworkAttemptID, false, false, reworkLock)
	}
	return checkpoint, http.StatusOK, nil
}

func (server *Server) createMission(response http.ResponseWriter, request *http.Request) {
	var payload struct {
		Item map[string]any `json:"item"`
	}
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	item, err := server.resolveWorkItemRepositoryTarget(request.Context(), payload.Item)
	if err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	mission := makeMission(item)
	if database, err := server.Repo.Load(request.Context()); err == nil {
		upsertMissionAndOperation(database, mission, fmt.Sprintf("pipeline_%s", text(payload.Item, "id")))
		_ = server.Repo.Save(request.Context(), *database)
	}
	writeJSON(response, http.StatusOK, mission)
}

func (server *Server) runOperation(response http.ResponseWriter, request *http.Request, persist bool) {
	var payload struct {
		Mission     map[string]any `json:"mission"`
		OperationID string         `json:"operationId"`
		Runner      string         `json:"runner"`
	}
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	if payload.Runner == "" {
		payload.Runner = "local-proof"
	}
	mission, err := server.resolveMissionRepositoryTarget(request.Context(), payload.Mission)
	if err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	server.WorkspaceRoot = server.localWorkspaceRoot(request.Context())
	profile := server.resolveAgentProfileForMission(request.Context(), mission)
	if operation := findOperation(mission, payload.OperationID); operation != nil {
		if _, err := preflightAgentRunner(payload.Runner, profile, text(operation, "agentId")); err != nil {
			writeError(response, http.StatusBadRequest, err)
			return
		}
	}
	result, err := server.runLocalProof(mission, payload.OperationID, payload.Runner, profile)
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	if persist {
		if database, err := server.Repo.Load(request.Context()); err == nil {
			applyMissionEvents(database, result.Events)
			upsertMissionAndOperation(database, mission, fmt.Sprintf("pipeline_%s", text(mission, "sourceWorkItemId")))
			upsertOperationStatus(database, mission, payload.OperationID, "done")
			upsertOperationRunnerProcess(database, payload.OperationID, result.RunnerProcess)
			for index, proof := range result.ProofFiles {
				database.Tables.ProofRecords = append(database.Tables.ProofRecords, map[string]any{
					"id":          fmt.Sprintf("%s:proof:%d", payload.OperationID, index+1),
					"operationId": payload.OperationID,
					"label":       "proof-file",
					"value":       filepath.Base(proof),
					"sourcePath":  proof,
					"createdAt":   nowISO(),
				})
			}
			_ = server.Repo.Save(request.Context(), *database)
		}
	}
	writeJSON(response, http.StatusOK, result)
}

func (server *Server) runLocalProof(mission map[string]any, operationID string, runner string, profile ProjectAgentProfile) (OperationResult, error) {
	operation := findOperation(mission, operationID)
	if operation == nil {
		return OperationResult{}, fmt.Errorf("unknown operation: %s", operationID)
	}
	agentID := text(operation, "agentId")
	workspace, err := workspaceChildPath(server.WorkspaceRoot, text(mission, "sourceIssueKey"), text(operation, "stageId"))
	if err != nil {
		return OperationResult{}, err
	}
	if err := os.RemoveAll(workspace); err != nil {
		return OperationResult{}, err
	}
	proofDir := filepath.Join(workspace, ".omega", "proof")
	if err := os.MkdirAll(proofDir, 0o755); err != nil {
		return OperationResult{}, err
	}
	if err := writeJSONFile(filepath.Join(workspace, ".omega", "job.json"), map[string]any{"missionId": mission["id"], "operation": operation}); err != nil {
		return OperationResult{}, err
	}
	if err := writeAgentRuntimeSpec(filepath.Join(workspace, ".omega", "agent-runtime.json"), map[string]any{
		"runner":             runner,
		"agentId":            agentID,
		"missionId":          text(mission, "id"),
		"operationId":        operationID,
		"workspaceRoot":      server.WorkspaceRoot,
		"workspacePath":      workspace,
		"repositoryTargetId": text(mission, "repositoryTargetId"),
		"repositoryTarget":   text(mission, "repositoryTargetLabel"),
		"agentProfile":       agentRuntimeMetadata(profile, agentID),
	}); err != nil {
		return OperationResult{}, err
	}
	if err := writeRunnerPolicyFiles(workspace, profile, agentID); err != nil {
		return OperationResult{}, err
	}
	if err := os.WriteFile(filepath.Join(workspace, ".omega", "prompt.md"), []byte(text(operation, "prompt")), 0o644); err != nil {
		return OperationResult{}, err
	}

	stdout := "local proof complete\n"
	stderr := ""
	status := "passed"
	branchName := ""
	commitSha := ""
	changedFiles := []string(nil)
	runnerProcess := map[string]any{"runner": runner, "status": "passed", "exitCode": 0}
	if runner == "demo-code" {
		demoResult, err := server.runDemoCodeChange(mission, operation, workspace, proofDir)
		stdout = demoResult.stdout
		branchName = demoResult.branchName
		commitSha = demoResult.commitSha
		changedFiles = demoResult.changedFiles
		if err != nil {
			status = "failed"
			stderr = err.Error()
			runnerProcess["status"] = "failed"
			runnerProcess["exitCode"] = -1
		}
	} else if isAIRunnerID(runner) {
		agentResult, err := server.runAgentRepositoryChange(mission, operation, workspace, proofDir, profile, runner)
		stdout = agentResult.stdout
		stderr = agentResult.stderr
		branchName = agentResult.branchName
		commitSha = agentResult.commitSha
		changedFiles = agentResult.changedFiles
		if agentResult.runnerProcess != nil {
			runnerProcess = agentResult.runnerProcess
		}
		if err != nil {
			status = "failed"
			if stderr == "" {
				stderr = err.Error()
			}
		}
	} else {
		if err := os.WriteFile(filepath.Join(proofDir, "local-proof.txt"), []byte("local proof"), 0o644); err != nil {
			return OperationResult{}, err
		}
	}
	proofs, _ := collectFiles(proofDir)
	now := nowISO()
	events := []map[string]any{
		{"type": "operation.started", "missionId": mission["id"], "workItemId": mission["sourceWorkItemId"], "operationId": operationID, "operationTitle": mission["title"], "timestamp": now},
		{"type": "operation.proof-attached", "missionId": mission["id"], "workItemId": mission["sourceWorkItemId"], "operationId": operationID, "operationTitle": mission["title"], "proofFiles": proofs, "summary": "Proof collected.", "timestamp": nowISO()},
		{"type": map[bool]string{true: "operation.completed", false: "operation.failed"}[status == "passed"], "missionId": mission["id"], "workItemId": mission["sourceWorkItemId"], "operationId": operationID, "operationTitle": mission["title"], "timestamp": nowISO()},
	}
	return OperationResult{OperationID: operationID, Status: status, WorkspacePath: workspace, ProofFiles: proofs, Stdout: stdout, Stderr: stderr, BranchName: branchName, CommitSha: commitSha, ChangedFiles: changedFiles, RunnerProcess: runnerProcess, Events: events}, nil
}

type demoCodeRunResult struct {
	stdout        string
	stderr        string
	branchName    string
	commitSha     string
	changedFiles  []string
	runnerProcess map[string]any
}

func githubRepositorySlugFromTarget(target string) string {
	value := strings.TrimSpace(strings.TrimSuffix(target, ".git"))
	if value == "" {
		return ""
	}
	if parsed, err := url.Parse(value); err == nil && strings.EqualFold(parsed.Host, "github.com") {
		parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		if len(parts) >= 2 && parts[0] != "" && parts[1] != "" {
			return parts[0] + "/" + strings.TrimSuffix(parts[1], ".git")
		}
	}
	if !strings.Contains(value, "://") && strings.Count(value, "/") == 1 && !strings.ContainsAny(value, " \t\n") {
		return value
	}
	return ""
}

func cloneTargetRepository(workspace string, target string, destination string) (string, error) {
	if slug := githubRepositorySlugFromTarget(target); slug != "" {
		output, err := runCommand(workspace, "gh", "repo", "clone", slug, destination, "--", "--no-hardlinks", "--quiet")
		if err == nil {
			return output, nil
		}
		_ = os.RemoveAll(destination)
		fallbackOutput, fallbackErr := runCommand(workspace, "git", "clone", "--no-hardlinks", "--quiet", "https://github.com/"+slug+".git", destination)
		if fallbackErr != nil {
			return output + fallbackOutput, fallbackErr
		}
		return output + fallbackOutput, nil
	}
	return runCommand(workspace, "git", "clone", "--no-hardlinks", "--quiet", target, destination)
}

func demoFixtureChangeForMission(mission map[string]any, operation map[string]any) (string, []byte) {
	intent := strings.ToLower(text(mission, "title") + "\n" + text(operation, "prompt"))
	if strings.Contains(intent, "markdown") || strings.Contains(intent, " md ") || strings.Contains(intent, ".md") || strings.Contains(intent, "md file") {
		return "omega-empty.md", []byte{}
	}
	return "src/omega-demo-change.ts", nil
}

func (server *Server) runDemoCodeChange(mission map[string]any, operation map[string]any, workspace string, proofDir string) (demoCodeRunResult, error) {
	targetRepo := strings.TrimSpace(text(mission, "target"))
	if targetRepo == "" || targetRepo == "No target" {
		return demoCodeRunResult{}, errors.New("demo-code runner requires a local git repository target")
	}
	repoWorkspace := filepath.Join(workspace, "repo")
	if output, err := cloneTargetRepository(workspace, targetRepo, repoWorkspace); err != nil {
		return demoCodeRunResult{stdout: output}, fmt.Errorf("clone target repository: %w", err)
	}

	branchName := "omega/" + safeSegment(text(mission, "sourceIssueKey")) + "-" + safeSegment(text(operation, "stageId"))
	if _, err := runCommand(repoWorkspace, "git", "checkout", "-b", branchName); err != nil {
		return demoCodeRunResult{branchName: branchName}, fmt.Errorf("create branch: %w", err)
	}
	_, _ = runCommand(repoWorkspace, "git", "config", "user.email", "omega-demo@example.local")
	_, _ = runCommand(repoWorkspace, "git", "config", "user.name", "Omega Demo Runner")

	changedFile, content := demoFixtureChangeForMission(mission, operation)
	generatedPath := filepath.Join(repoWorkspace, changedFile)
	if err := os.MkdirAll(filepath.Dir(generatedPath), 0o755); err != nil {
		return demoCodeRunResult{branchName: branchName}, err
	}
	code := fmt.Sprintf(`export const omegaDemoChange = {
  workItem: %q,
  stage: %q,
  title: %q,
  prompt: %q
} as const;
`, text(mission, "sourceIssueKey"), text(operation, "stageId"), text(mission, "title"), text(operation, "prompt"))
	if content == nil {
		content = []byte(code)
	}
	if err := os.WriteFile(generatedPath, content, 0o644); err != nil {
		return demoCodeRunResult{branchName: branchName}, err
	}

	if _, err := runCommand(repoWorkspace, "git", "add", changedFile); err != nil {
		return demoCodeRunResult{branchName: branchName}, fmt.Errorf("stage generated code: %w", err)
	}
	if _, err := runCommand(repoWorkspace, "git", "commit", "-m", "Omega demo code change for "+text(mission, "sourceIssueKey")); err != nil {
		return demoCodeRunResult{branchName: branchName}, fmt.Errorf("commit generated code: %w", err)
	}
	commitSha, err := runCommand(repoWorkspace, "git", "rev-parse", "--short", "HEAD")
	if err != nil {
		return demoCodeRunResult{branchName: branchName}, fmt.Errorf("read commit sha: %w", err)
	}
	diff, err := runCommand(repoWorkspace, "git", "diff", "HEAD~1..HEAD")
	if err != nil {
		return demoCodeRunResult{branchName: branchName}, fmt.Errorf("create diff proof: %w", err)
	}
	stat, err := runCommand(repoWorkspace, "git", "diff", "--stat", "HEAD~1..HEAD")
	if err != nil {
		return demoCodeRunResult{branchName: branchName}, fmt.Errorf("create diff stat: %w", err)
	}
	if err := os.WriteFile(filepath.Join(proofDir, "git-diff.patch"), []byte(diff), 0o644); err != nil {
		return demoCodeRunResult{branchName: branchName}, err
	}
	summary := fmt.Sprintf("# Omega Demo Code Change\n\n- Work item: %s\n- Stage: %s\n- Branch: `%s`\n- Commit: `%s`\n- Changed files:\n  - `%s`\n\n```diffstat\n%s```\n", text(mission, "sourceIssueKey"), text(operation, "stageId"), branchName, strings.TrimSpace(commitSha), changedFile, stat)
	if err := os.WriteFile(filepath.Join(proofDir, "change-summary.md"), []byte(summary), 0o644); err != nil {
		return demoCodeRunResult{branchName: branchName}, err
	}

	stdout := fmt.Sprintf("demo code runner complete\nbranch: %s\ncommit: %s\nchanged: %s\n", branchName, strings.TrimSpace(commitSha), changedFile)
	return demoCodeRunResult{stdout: stdout, branchName: branchName, commitSha: strings.TrimSpace(commitSha), changedFiles: []string{changedFile}}, nil
}

func (server *Server) runAgentRepositoryChange(mission map[string]any, operation map[string]any, workspace string, proofDir string, profile ProjectAgentProfile, requestedRunner string) (demoCodeRunResult, error) {
	agentID := text(operation, "agentId")
	agent := agentProfileForRole(profile, agentID)
	runnerID := effectiveAgentRunnerID(requestedRunner, profile, agentID)
	agentRunner, resolvedRunnerID := NewAgentRunnerRegistry().Resolve(runnerID)
	targetRepo := strings.TrimSpace(text(mission, "target"))
	if targetRepo == "" || targetRepo == "No target" {
		prompt := text(operation, "prompt") + "\n\n" + agentPolicyBlock(profile, agentID)
		turn := agentRunner.RunTurn(context.Background(), AgentTurnRequest{
			Role:       agentID,
			StageID:    text(operation, "stageId"),
			Runner:     resolvedRunnerID,
			Workspace:  workspace,
			Prompt:     prompt,
			OutputPath: filepath.Join(proofDir, resolvedRunnerID+"-last-message.txt"),
			Sandbox:    "workspace-write",
			Model:      stringOr(agent.Model, "gpt-5.4-mini"),
		})
		return demoCodeRunResult{stdout: text(turn.Process, "stdout"), stderr: text(turn.Process, "stderr"), runnerProcess: turn.Process}, turn.Error
	}

	repoWorkspace := filepath.Join(workspace, "repo")
	if output, err := cloneTargetRepository(workspace, targetRepo, repoWorkspace); err != nil {
		return demoCodeRunResult{stdout: output}, fmt.Errorf("clone target repository: %w", err)
	}
	if err := writeRunnerPolicyFiles(repoWorkspace, profile, agentID); err != nil {
		return demoCodeRunResult{}, err
	}
	branchName := "omega/" + safeSegment(text(mission, "sourceIssueKey")) + "-" + safeSegment(text(operation, "stageId")) + "-" + safeSegment(resolvedRunnerID)
	if _, err := runCommand(repoWorkspace, "git", "checkout", "-b", branchName); err != nil {
		return demoCodeRunResult{branchName: branchName}, fmt.Errorf("create branch: %w", err)
	}
	_, _ = runCommand(repoWorkspace, "git", "config", "user.email", "omega-codex@example.local")
	_, _ = runCommand(repoWorkspace, "git", "config", "user.name", "Omega Codex Runner")

	prompt := fmt.Sprintf("%s\n\nRepository target: %s\nCreate the requested code change in this repository. Leave generated proof in %s.\n\n%s", text(operation, "prompt"), targetRepo, proofDir, agentPolicyBlock(profile, agentID))
	turn := agentRunner.RunTurn(context.Background(), AgentTurnRequest{
		Role:       agentID,
		StageID:    text(operation, "stageId"),
		Runner:     resolvedRunnerID,
		Workspace:  repoWorkspace,
		Prompt:     prompt,
		OutputPath: filepath.Join(proofDir, resolvedRunnerID+"-last-message.txt"),
		Sandbox:    "workspace-write",
		Model:      stringOr(agent.Model, "gpt-5.4-mini"),
	})
	process := turn.Process
	stdout := text(process, "stdout")
	stderr := text(process, "stderr")
	if turn.Error != nil {
		return demoCodeRunResult{stdout: stdout, stderr: stderr, branchName: branchName, runnerProcess: process}, turn.Error
	}

	changed, err := runCommand(repoWorkspace, "git", "status", "--short")
	if err != nil {
		return demoCodeRunResult{stdout: stdout, stderr: stderr, branchName: branchName, runnerProcess: process}, fmt.Errorf("read repository changes: %w", err)
	}
	if strings.TrimSpace(changed) == "" {
		if text(operation, "stageId") == "coding" {
			return demoCodeRunResult{stdout: stdout, stderr: stderr, branchName: branchName, runnerProcess: process}, errors.New("codex produced no repository changes for coding stage")
		}
		return demoCodeRunResult{stdout: stdout, stderr: stderr, branchName: branchName, runnerProcess: process}, nil
	}
	if _, err := runCommand(repoWorkspace, "git", "add", "-A"); err != nil {
		return demoCodeRunResult{stdout: stdout, stderr: stderr, branchName: branchName, runnerProcess: process}, fmt.Errorf("stage codex changes: %w", err)
	}
	commitMessage := "Omega codex code change for " + text(mission, "sourceIssueKey")
	if _, err := runCommand(repoWorkspace, "git", "commit", "-m", commitMessage); err != nil {
		return demoCodeRunResult{stdout: stdout, stderr: stderr, branchName: branchName, runnerProcess: process}, fmt.Errorf("commit codex changes: %w", err)
	}
	commitSha, err := runCommand(repoWorkspace, "git", "rev-parse", "--short", "HEAD")
	if err != nil {
		return demoCodeRunResult{stdout: stdout, stderr: stderr, branchName: branchName, runnerProcess: process}, fmt.Errorf("read commit sha: %w", err)
	}
	diff, err := runCommand(repoWorkspace, "git", "diff", "HEAD~1..HEAD")
	if err != nil {
		return demoCodeRunResult{stdout: stdout, stderr: stderr, branchName: branchName, runnerProcess: process}, fmt.Errorf("create diff proof: %w", err)
	}
	stat, err := runCommand(repoWorkspace, "git", "diff", "--stat", "HEAD~1..HEAD")
	if err != nil {
		return demoCodeRunResult{stdout: stdout, stderr: stderr, branchName: branchName, runnerProcess: process}, fmt.Errorf("create diff stat: %w", err)
	}
	changedNames, err := runCommand(repoWorkspace, "git", "diff", "--name-only", "HEAD~1..HEAD")
	if err != nil {
		return demoCodeRunResult{stdout: stdout, stderr: stderr, branchName: branchName, runnerProcess: process}, fmt.Errorf("list changed files: %w", err)
	}
	if err := os.WriteFile(filepath.Join(proofDir, "git-diff.patch"), []byte(diff), 0o644); err != nil {
		return demoCodeRunResult{stdout: stdout, stderr: stderr, branchName: branchName, runnerProcess: process}, err
	}
	changedFiles := compactLines(changedNames)
	summary := fmt.Sprintf("# Omega Agent Code Change\n\n- Work item: %s\n- Stage: %s\n- Runner: `%s`\n- Branch: `%s`\n- Commit: `%s`\n- Changed files:\n%s\n```diffstat\n%s```\n", text(mission, "sourceIssueKey"), text(operation, "stageId"), resolvedRunnerID, branchName, strings.TrimSpace(commitSha), markdownFileList(changedFiles), stat)
	if err := os.WriteFile(filepath.Join(proofDir, "change-summary.md"), []byte(summary), 0o644); err != nil {
		return demoCodeRunResult{stdout: stdout, stderr: stderr, branchName: branchName, runnerProcess: process}, err
	}
	stdout += fmt.Sprintf("\n%s repository change committed\nbranch: %s\ncommit: %s\n", resolvedRunnerID, branchName, strings.TrimSpace(commitSha))
	return demoCodeRunResult{stdout: stdout, stderr: stderr, branchName: branchName, commitSha: strings.TrimSpace(commitSha), changedFiles: changedFiles, runnerProcess: process}, nil
}

func compactLines(raw string) []string {
	lines := strings.Split(raw, "\n")
	items := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			items = append(items, line)
		}
	}
	return items
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	items := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		items = append(items, value)
	}
	return items
}

func markdownFileList(files []string) string {
	if len(files) == 0 {
		return "- none\n"
	}
	var builder strings.Builder
	for _, file := range files {
		builder.WriteString("- `")
		builder.WriteString(file)
		builder.WriteString("`\n")
	}
	return builder.String()
}

func runRepositoryValidation(repoWorkspace string) (string, error) {
	outputs := []string{}
	diffOutput, diffErr := runCommand(repoWorkspace, "git", "diff", "--check", "HEAD~1", "HEAD")
	outputs = append(outputs, "$ git diff --check HEAD~1 HEAD\n"+stringOr(strings.TrimSpace(diffOutput), "No whitespace errors found."))
	if diffErr != nil {
		return strings.Join(outputs, "\n\n"), diffErr
	}
	if pathExists(filepath.Join(repoWorkspace, "test")) || pathExists(filepath.Join(repoWorkspace, "tests")) {
		nodeOutput, nodeErr := runCommand(repoWorkspace, "node", "--test")
		outputs = append(outputs, "$ node --test\n"+strings.TrimSpace(nodeOutput))
		if nodeErr != nil {
			return strings.Join(outputs, "\n\n"), nodeErr
		}
	}
	if pathExists(filepath.Join(repoWorkspace, "scripts", "task-summary.mjs")) && pathExists(filepath.Join(repoWorkspace, "examples", "tasks.md")) {
		cliOutput, cliErr := runCommand(repoWorkspace, "node", "scripts/task-summary.mjs", "examples/tasks.md")
		outputs = append(outputs, "$ node scripts/task-summary.mjs examples/tasks.md\n"+strings.TrimSpace(cliOutput))
		if cliErr != nil {
			return strings.Join(outputs, "\n\n"), cliErr
		}
	}
	return strings.Join(outputs, "\n\n"), nil
}

func testFailureSummary(err error, output string) string {
	if err == nil {
		return ""
	}
	detail := strings.TrimSpace(output)
	if detail == "" {
		detail = err.Error()
	}
	return truncateForProof(detail, 900)
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func runCommand(dir string, name string, args ...string) (string, error) {
	command := exec.Command(name, args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%s %s failed: %w\n%s", name, strings.Join(args, " "), err, string(output))
	}
	return string(output), nil
}

func buildDevFlowReviewPrompt(item map[string]any, repoSlug string, prURL string, changedFiles []string, diffText string, testOutput string, checksOutput string, focus string, reviewFeedback string) string {
	return fmt.Sprintf(`You are the review agent for Omega.

Repository: %s
Pull request: %s
Work item: %s
Title: %s

Requirement:
%s

Acceptance criteria:
%s

Human or previous review feedback to verify:
%s

Changed files:
%s

Focus:
%s

Validation output:
%s

Remote checks:
%s

Diff:
%s

Review rules:
- Review the actual diff against the requirement and acceptance criteria.
- If this is a human-requested rework, treat the diff as the increment since the previous reviewed version and verify it directly addresses the human feedback.
- Do not approve just because a file changed or tests passed.
- If the diff does not satisfy the requested behavior, request changes.
- Do not edit files.

Write the review as Markdown and include exactly one verdict line:
Verdict: APPROVED
or
Verdict: CHANGES_REQUESTED
or
Verdict: NEEDS_HUMAN_INFO

Then write a concise review packet with these sections:

Summary:
- One or two sentences explaining the decision.

Blocking findings:
- [severity] file-or-scope - what is wrong - required change.

Validation gaps:
- Missing or weak validation that must be fixed before delivery.

Rework instructions:
- Concrete edits the rework agent should make next.

Residual risks:
- Risks that remain even if approved, or "None known".

If the verdict is CHANGES_REQUESTED, include at least one Blocking finding or Rework instruction.
If the verdict is NEEDS_HUMAN_INFO, include the exact question a human must answer.
If the verdict is APPROVED, explain why the diff satisfies the requirement and list residual risk.
`, repoSlug, prURL, text(item, "key"), text(item, "title"), text(item, "description"), markdownAnyList(item["acceptanceCriteria"]), stringOr(strings.TrimSpace(reviewFeedback), "None."), markdownFileList(changedFiles), focus, truncateForProof(testOutput, 4000), truncateForProof(checksOutput, 4000), truncateForProof(diffText, 12000))
}

func runDevFlowReviewAgent(repoWorkspace string, prompt string, outputPath string, model string) (map[string]any, error) {
	process, err := runSupervisedCommand(repoWorkspace, prompt, "codex", "--ask-for-approval", "never", "exec", "--model", stringOr(model, "gpt-5.4-mini"), "-c", "model_reasoning_effort=\"medium\"", "--skip-git-repo-check", "--sandbox", "read-only", "--output-last-message", outputPath, "-")
	ensureAgentOutputFile(outputPath, process)
	return process, err
}

type DevFlowReviewOutcome struct {
	Verdict string
	Summary string
}

func devFlowReviewOutcome(path string) DevFlowReviewOutcome {
	raw, err := os.ReadFile(path)
	if err != nil {
		return DevFlowReviewOutcome{Verdict: "missing", Summary: "Review verdict missing: " + err.Error()}
	}
	content := strings.TrimSpace(string(raw))
	normalized := strings.ToLower(content)
	switch {
	case strings.Contains(normalized, "verdict: needs_human_info") ||
		strings.Contains(normalized, "needs human info") ||
		strings.Contains(normalized, "needs_human_info"):
		return DevFlowReviewOutcome{Verdict: "needs_human_info", Summary: devFlowReviewSummary(content, "Review needs human input before continuing.")}
	case strings.Contains(normalized, "verdict: changes_requested") ||
		strings.Contains(normalized, "changes requested") ||
		strings.Contains(normalized, `"approved": false`) ||
		strings.Contains(normalized, "blocked"):
		return DevFlowReviewOutcome{Verdict: "changes_requested", Summary: devFlowReviewSummary(content, "Review requested changes.")}
	case strings.Contains(normalized, "verdict: approved") ||
		strings.Contains(normalized, `"approved": true`):
		return DevFlowReviewOutcome{Verdict: "approved", Summary: devFlowReviewSummary(content, "Review approved the diff against the requirement.")}
	default:
		return DevFlowReviewOutcome{Verdict: "missing", Summary: devFlowReviewSummary(content, "Review did not include an explicit approved verdict.")}
	}
}

func devFlowReviewSummary(content string, fallback string) string {
	lines := []string{}
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "verdict:") {
			continue
		}
		trimmed = strings.TrimSpace(strings.TrimLeft(trimmed, "# "))
		if trimmed == "" || strings.EqualFold(trimmed, "review") {
			continue
		}
		lines = append(lines, trimmed)
	}
	if len(lines) == 0 {
		return fallback
	}
	return truncateForProof(strings.Join(lines, "\n"), 700)
}

func devFlowReviewVerdict(path string) (bool, string) {
	outcome := devFlowReviewOutcome(path)
	return outcome.Verdict == "approved", outcome.Summary
}

func markdownAnyList(value any) string {
	values := arrayValues(value)
	if len(values) == 0 {
		return "- none\n"
	}
	var builder strings.Builder
	for _, item := range values {
		builder.WriteString("- ")
		builder.WriteString(fmt.Sprint(item))
		builder.WriteString("\n")
	}
	return builder.String()
}

func runSupervisedCommand(dir string, stdin string, name string, args ...string) (map[string]any, error) {
	return runSupervisedCommandContext(context.Background(), dir, stdin, name, args...)
}

func runSupervisedCommandContext(ctx context.Context, dir string, stdin string, name string, args ...string) (map[string]any, error) {
	return runSupervisedCommandContextWithOptions(ctx, SupervisedCommandOptions{}, dir, stdin, name, args...)
}

type SupervisedCommandEvent struct {
	Stream    string
	Chunk     string
	CreatedAt string
}

type SupervisedCommandOptions struct {
	HeartbeatInterval time.Duration
	OnEvent           func(SupervisedCommandEvent)
}

type supervisedStreamWriter struct {
	stream string
	buffer *bytes.Buffer
	mu     *sync.Mutex
	record func(stream string, chunk string)
}

func (writer supervisedStreamWriter) Write(chunk []byte) (int, error) {
	writer.mu.Lock()
	_, _ = writer.buffer.Write(chunk)
	writer.mu.Unlock()
	if writer.record != nil && len(chunk) > 0 {
		writer.record(writer.stream, string(chunk))
	}
	return len(chunk), nil
}

func runSupervisedCommandContextWithOptions(ctx context.Context, options SupervisedCommandOptions, dir string, stdin string, name string, args ...string) (map[string]any, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	started := time.Now()
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = dir
	if stdin != "" {
		command.Stdin = strings.NewReader(stdin)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var stdoutMu sync.Mutex
	var stderrMu sync.Mutex
	var eventsMu sync.Mutex
	processEvents := []map[string]any{}
	recordEvent := func(stream string, chunk string) {
		event := SupervisedCommandEvent{
			Stream:    stream,
			Chunk:     truncateForProof(chunk, 4000),
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		}
		eventsMu.Lock()
		processEvents = append(processEvents, map[string]any{"stream": event.Stream, "chunk": event.Chunk, "createdAt": event.CreatedAt})
		if len(processEvents) > 200 {
			processEvents = processEvents[len(processEvents)-200:]
		}
		eventsMu.Unlock()
		if options.OnEvent != nil {
			options.OnEvent(event)
		}
	}
	command.Stdout = supervisedStreamWriter{stream: "stdout", buffer: &stdout, mu: &stdoutMu, record: recordEvent}
	command.Stderr = supervisedStreamWriter{stream: "stderr", buffer: &stderr, mu: &stderrMu, record: recordEvent}
	process := map[string]any{
		"command":   name,
		"args":      args,
		"cwd":       dir,
		"status":    "running",
		"startedAt": started.UTC().Format(time.RFC3339Nano),
	}
	if err := command.Start(); err != nil {
		process["status"] = "failed"
		process["exitCode"] = -1
		process["stderr"] = err.Error()
		process["finishedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
		process["durationMs"] = float64(time.Since(started).Milliseconds())
		return process, err
	}
	process["pid"] = command.Process.Pid
	done := make(chan struct{})
	if options.HeartbeatInterval > 0 {
		go func() {
			ticker := time.NewTicker(options.HeartbeatInterval)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					recordEvent("heartbeat", "")
				}
			}
		}()
	}
	err := command.Wait()
	close(done)
	finished := time.Now()
	stdoutMu.Lock()
	process["stdout"] = stdout.String()
	stdoutMu.Unlock()
	stderrMu.Lock()
	process["stderr"] = stderr.String()
	stderrMu.Unlock()
	eventsMu.Lock()
	process["events"] = append([]map[string]any{}, processEvents...)
	eventsMu.Unlock()
	process["finishedAt"] = finished.UTC().Format(time.RFC3339Nano)
	process["durationMs"] = float64(finished.Sub(started).Milliseconds())
	exitCode := 0
	if ctxErr := ctx.Err(); ctxErr != nil {
		status := "canceled"
		if errors.Is(ctxErr, context.DeadlineExceeded) {
			status = "timed-out"
		}
		process["status"] = status
		process["exitCode"] = -1
		process["cancellationReason"] = ctxErr.Error()
		return process, fmt.Errorf("%s %s %s: %w", name, strings.Join(args, " "), status, ctxErr)
	}
	if err != nil {
		exitCode = -1
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		}
		process["status"] = "failed"
		process["exitCode"] = exitCode
		return process, fmt.Errorf("%s %s failed with exit code %d", name, strings.Join(args, " "), exitCode)
	}
	process["status"] = "passed"
	process["exitCode"] = exitCode
	return process, nil
}

func startDetachedCommand(name string, args ...string) error {
	command := exec.Command(name, args...)
	logFile, err := os.OpenFile(filepath.Join(os.TempDir(), "omega-gh-auth.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err == nil {
		command.Stdout = logFile
		command.Stderr = logFile
	}
	if err := command.Start(); err != nil {
		if logFile != nil {
			_ = logFile.Close()
		}
		return err
	}
	go func() {
		_ = command.Wait()
		if logFile != nil {
			_ = logFile.Close()
		}
	}()
	return nil
}

func githubCliLoginCommand() string {
	return "gh auth login --hostname github.com --git-protocol https --web --clipboard --scopes repo,read:org,workflow"
}

func githubCliVerificationURL() string {
	return "https://github.com/login/device"
}

func githubAccountFromStatusOutput(output string) string {
	for _, line := range strings.Split(output, "\n") {
		const marker = "Logged in to github.com account "
		index := strings.Index(line, marker)
		if index == -1 {
			continue
		}
		rest := strings.TrimSpace(line[index+len(marker):])
		if rest == "" {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) > 0 {
			return fields[0]
		}
	}
	return ""
}

func (server *Server) githubStatus(response http.ResponseWriter, request *http.Request) {
	command := exec.CommandContext(request.Context(), "gh", "auth", "status")
	output, err := command.CombinedOutput()
	_, ghPathErr := exec.LookPath("gh")
	account := githubAccountFromStatusOutput(string(output))
	if err == nil {
		if database, loadErr := server.Repo.Load(request.Context()); loadErr == nil {
			upsertGitHubConnection(database, stringOr(account, "gh-cli"))
			_ = server.Repo.Save(request.Context(), *database)
		}
	}
	oauthToken, oauthErr := server.Repo.GetSetting(request.Context(), "github_oauth_token")
	writeJSON(response, http.StatusOK, map[string]any{
		"available":          ghPathErr == nil,
		"authenticated":      err == nil,
		"account":            account,
		"output":             string(output),
		"oauthConfigured":    githubOAuthConfigured(server.effectiveGitHubOAuthConfig(request.Context())),
		"oauthAuthenticated": oauthErr == nil,
		"oauthConnectedAs":   text(oauthToken, "accountId"),
	})
}

func (server *Server) githubOAuthConfig(response http.ResponseWriter, request *http.Request) {
	writeJSON(response, http.StatusOK, server.githubOAuthConfigInfo(request.Context()))
}

func (server *Server) putGitHubOAuthConfig(response http.ResponseWriter, request *http.Request) {
	var payload map[string]string
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	current := server.effectiveGitHubOAuthConfig(request.Context())
	clientSecret := strings.TrimSpace(payload["clientSecret"])
	if clientSecret == "" {
		clientSecret = current.ClientSecret
	}
	next := GitHubOAuthConfig{
		ClientID:     strings.TrimSpace(payload["clientId"]),
		ClientSecret: clientSecret,
		RedirectURI:  stringOr(strings.TrimSpace(payload["redirectUri"]), defaultGitHubRedirectURI()),
		TokenURL:     stringOr(strings.TrimSpace(payload["tokenUrl"]), defaultGitHubTokenURL()),
	}
	if next.ClientID == "" {
		writeJSON(response, http.StatusBadRequest, map[string]any{"error": "clientId is required"})
		return
	}
	if next.ClientSecret == "" {
		writeJSON(response, http.StatusBadRequest, map[string]any{"error": "clientSecret is required"})
		return
	}
	if err := server.Repo.SetSetting(request.Context(), "github_oauth_config", map[string]any{
		"clientId":     next.ClientID,
		"clientSecret": next.ClientSecret,
		"redirectUri":  next.RedirectURI,
		"tokenUrl":     next.TokenURL,
		"updatedAt":    nowISO(),
	}); err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, githubOAuthConfigInfo(next, "app"))
}

func (server *Server) githubOAuthStart(response http.ResponseWriter, request *http.Request) {
	config := server.effectiveGitHubOAuthConfig(request.Context())
	if !githubOAuthConfigured(config) {
		writeJSON(response, http.StatusOK, map[string]any{"configured": false, "reason": "GitHub OAuth app is not configured. Omega can use GitHub CLI sign-in as a local fallback."})
		return
	}
	state, err := randomState()
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	if err := server.Repo.SetSetting(request.Context(), "github_oauth_state", map[string]any{"state": state, "createdAt": nowISO()}); err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	scopes := []string{"repo", "read:org", "workflow"}
	authorizeURL, err := githubAuthorizeURL(config, scopes, state)
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"configured":   true,
		"authorizeUrl": authorizeURL,
		"state":        state,
		"redirectUri":  config.RedirectURI,
		"scopes":       scopes,
	})
}

func (server *Server) githubCliLoginStart(response http.ResponseWriter, request *http.Request) {
	if _, err := exec.LookPath("gh"); err != nil && server.CommandStarter == nil {
		writeJSON(response, http.StatusOK, map[string]any{
			"started":         false,
			"reason":          "GitHub CLI is not installed or not on PATH.",
			"command":         githubCliLoginCommand(),
			"verificationUrl": githubCliVerificationURL(),
		})
		return
	}
	args := []string{
		"auth",
		"login",
		"--hostname",
		"github.com",
		"--git-protocol",
		"https",
		"--web",
		"--clipboard",
		"--scopes",
		"repo,read:org,workflow",
	}
	starter := server.CommandStarter
	if starter == nil {
		starter = startDetachedCommand
	}
	if err := starter("gh", args...); err != nil {
		writeJSON(response, http.StatusOK, map[string]any{
			"started":         false,
			"reason":          err.Error(),
			"command":         githubCliLoginCommand(),
			"verificationUrl": githubCliVerificationURL(),
		})
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"started":         true,
		"method":          "gh-cli",
		"message":         "GitHub CLI sign-in opened. Paste the copied one-time code on GitHub's device page.",
		"command":         githubCliLoginCommand(),
		"verificationUrl": githubCliVerificationURL(),
	})
}

func (server *Server) githubOAuthCallback(response http.ResponseWriter, request *http.Request) {
	config := server.effectiveGitHubOAuthConfig(request.Context())
	if !githubOAuthConfigured(config) {
		writeJSON(response, http.StatusBadRequest, map[string]any{"error": "GitHub OAuth is not configured"})
		return
	}
	code := request.URL.Query().Get("code")
	state := request.URL.Query().Get("state")
	if code == "" || state == "" {
		writeJSON(response, http.StatusBadRequest, map[string]any{"error": "code and state are required"})
		return
	}
	expected, err := server.Repo.GetSetting(request.Context(), "github_oauth_state")
	if err != nil || text(expected, "state") != state {
		writeJSON(response, http.StatusBadRequest, map[string]any{"error": "invalid GitHub OAuth state"})
		return
	}
	token, err := server.exchangeGitHubOAuthCode(request.Context(), code, config)
	if err != nil {
		writeError(response, http.StatusBadGateway, err)
		return
	}
	scopes := scopesFromString(text(token, "scope"))
	record := map[string]any{
		"provider":    "github",
		"accountId":   "github-oauth",
		"accessToken": text(token, "access_token"),
		"tokenType":   text(token, "token_type"),
		"scopes":      scopes,
		"connectedAt": nowISO(),
	}
	if err := server.Repo.SetSetting(request.Context(), "github_oauth_token", record); err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	if database, err := server.Repo.Load(request.Context()); err == nil {
		upsertGitHubConnection(database, "github-oauth")
		_ = server.Repo.Save(request.Context(), *database)
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"connected": true,
		"accountId": "github-oauth",
		"scopes":    scopes,
	})
}

func (server *Server) githubRepoInfo(response http.ResponseWriter, request *http.Request) {
	var payload map[string]string
	_ = json.NewDecoder(request.Body).Decode(&payload)
	repo := payload["owner"] + "/" + payload["repo"]
	command := exec.CommandContext(request.Context(), "gh", "repo", "view", repo, "--json", "name,owner,description,url,isPrivate,defaultBranchRef")
	output, err := command.Output()
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	var data map[string]any
	_ = json.Unmarshal(output, &data)
	writeJSON(response, http.StatusOK, data)
}

func normalizeWorkspaceRoot(path string) (string, error) {
	value := strings.TrimSpace(path)
	if value == "" {
		return "", errors.New("workspaceRoot is required")
	}
	if value == "~" || strings.HasPrefix(value, "~/") {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return "", errors.New("cannot resolve user home directory")
		}
		value = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(value, "~"), "/"))
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	return absolute, nil
}

func workspaceChildPath(root string, parts ...string) (string, error) {
	normalizedRoot, err := normalizeWorkspaceRoot(root)
	if err != nil {
		return "", err
	}
	segments := []string{normalizedRoot}
	for _, part := range parts {
		segment := strings.Trim(safeSegment(part), ".-_")
		if segment == "" {
			segment = "workspace"
		}
		segments = append(segments, segment)
	}
	return ensurePathInsideRoot(normalizedRoot, filepath.Join(segments...))
}

func ensurePathInsideRoot(root string, path string) (string, error) {
	normalizedRoot, err := normalizeWorkspaceRoot(root)
	if err != nil {
		return "", err
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if !pathInsideRoot(normalizedRoot, absolutePath) {
		return "", fmt.Errorf("workspace path %s escapes configured workspace root %s", absolutePath, normalizedRoot)
	}
	return absolutePath, nil
}

func pathInsideRoot(root string, path string) bool {
	normalizedRoot, err := normalizeWorkspaceRoot(root)
	if err != nil {
		return false
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(normalizedRoot, absolutePath)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)) && !filepath.IsAbs(relative))
}

func (server *Server) localWorkspaceRoot(ctx context.Context) string {
	if setting, err := server.Repo.GetSetting(ctx, "local_workspace_root"); err == nil {
		if root, err := normalizeWorkspaceRoot(text(setting, "workspaceRoot")); err == nil {
			return root
		}
	}
	if root, err := normalizeWorkspaceRoot(server.WorkspaceRoot); err == nil {
		return root
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), "omega-workspaces")
	}
	return filepath.Join(home, "Omega", "workspaces")
}

func (server *Server) getLocalWorkspaceRoot(response http.ResponseWriter, request *http.Request) {
	root := server.localWorkspaceRoot(request.Context())
	writeJSON(response, http.StatusOK, map[string]any{"workspaceRoot": root})
}

func (server *Server) putLocalWorkspaceRoot(response http.ResponseWriter, request *http.Request) {
	var payload map[string]string
	_ = json.NewDecoder(request.Body).Decode(&payload)
	root, err := normalizeWorkspaceRoot(payload["workspaceRoot"])
	if err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	server.WorkspaceRoot = root
	record := map[string]any{"workspaceRoot": root, "updatedAt": nowISO()}
	if err := server.Repo.SetSetting(request.Context(), "local_workspace_root", record); err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"workspaceRoot": root})
}

func (server *Server) githubRepositories(response http.ResponseWriter, request *http.Request) {
	limit := request.URL.Query().Get("limit")
	if limit == "" {
		limit = "100"
	}
	command := exec.CommandContext(
		request.Context(),
		"gh",
		"repo",
		"list",
		"--limit",
		limit,
		"--json",
		"name,nameWithOwner,owner,description,url,isPrivate,defaultBranchRef",
	)
	output, err := command.Output()
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	var repositories []map[string]any
	if err := json.Unmarshal(output, &repositories); err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, repositories)
}

func (server *Server) githubBindRepositoryTarget(response http.ResponseWriter, request *http.Request) {
	var payload struct {
		Owner         string `json:"owner"`
		Repo          string `json:"repo"`
		NameWithOwner string `json:"nameWithOwner"`
		DefaultBranch string `json:"defaultBranch"`
		URL           string `json:"url"`
	}
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	repo := payload.NameWithOwner
	if repo == "" && payload.Owner != "" && payload.Repo != "" {
		repo = payload.Owner + "/" + payload.Repo
	}
	if repo == "" {
		writeError(response, http.StatusBadRequest, fmt.Errorf("repository is required"))
		return
	}
	database, err := mustLoad(server, request.Context())
	if err != nil {
		writeError(response, http.StatusNotFound, err)
		return
	}
	upsertGitHubRepositoryTarget(&database, repo, payload.DefaultBranch, payload.URL)
	touch(&database)
	if err := server.Repo.Save(request.Context(), database); err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, database)
}

func (server *Server) githubDeleteRepositoryTarget(response http.ResponseWriter, request *http.Request) {
	targetID, err := url.PathUnescape(strings.TrimPrefix(request.URL.Path, "/github/repository-targets/"))
	if err != nil || strings.TrimSpace(targetID) == "" {
		writeError(response, http.StatusBadRequest, fmt.Errorf("repository target id is required"))
		return
	}
	database, err := mustLoad(server, request.Context())
	if err != nil {
		writeError(response, http.StatusNotFound, err)
		return
	}
	next, deleted := deleteRepositoryTarget(database, targetID)
	if !deleted {
		writeJSON(response, http.StatusNotFound, map[string]any{"error": "repository target not found"})
		return
	}
	touch(&next)
	if err := server.Repo.Save(request.Context(), next); err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, next)
}

func (server *Server) githubImportIssues(response http.ResponseWriter, request *http.Request) {
	database, err := mustLoad(server, request.Context())
	if err != nil {
		writeError(response, http.StatusNotFound, err)
		return
	}
	var payload map[string]string
	_ = json.NewDecoder(request.Body).Decode(&payload)
	repo := payload["owner"] + "/" + payload["repo"]
	command := exec.CommandContext(request.Context(), "gh", "issue", "list", "--repo", repo, "--state", "open", "--limit", "50", "--json", "number,title,body,state,labels,assignees,url")
	output, err := command.Output()
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	var issues []map[string]any
	if err := json.Unmarshal(output, &issues); err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	upsertGitHubRepositoryTarget(&database, repo, "", "")
	for _, issue := range issues {
		item := githubIssueToWorkItem(repo, issue)
		if findByID(database.Tables.WorkItems, text(item, "id")) < 0 {
			database = appendWorkItem(database, item)
		}
	}
	if err := server.Repo.Save(request.Context(), database); err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, database)
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("access-control-allow-origin", "*")
		response.Header().Set("access-control-allow-methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		response.Header().Set("access-control-allow-headers", "content-type")
		next.ServeHTTP(response, request)
	})
}

func writeJSON(response http.ResponseWriter, status int, body any) {
	response.Header().Set("content-type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(body)
}

func writeError(response http.ResponseWriter, status int, err error) {
	writeJSON(response, status, map[string]any{"error": err.Error()})
}

func mustLoad(server *Server, ctx context.Context) (WorkspaceDatabase, error) {
	database, err := server.Repo.Load(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return defaultWorkspaceDatabase(), nil
	}
	if err != nil {
		return WorkspaceDatabase{}, err
	}
	return *database, nil
}

func defaultWorkspaceDatabase() WorkspaceDatabase {
	timestamp := nowISO()
	return WorkspaceDatabase{
		SchemaVersion: 1,
		SavedAt:       timestamp,
		Tables: WorkspaceTables{
			Projects: []map[string]any{{
				"id":          "project_omega",
				"name":        "Omega",
				"description": "Omega local development workspace",
				"team":        "Omega",
				"status":      "Active",
				"labels":      []any{},
				"createdAt":   timestamp,
				"updatedAt":   timestamp,
			}},
			MissionControlStates: []map[string]any{{
				"runId":       "run_req_omega_001",
				"projectId":   "project_omega",
				"workItems":   []any{},
				"events":      []any{},
				"syncIntents": []any{},
				"updatedAt":   timestamp,
			}},
			Connections:   []map[string]any{},
			UIPreferences: []map[string]any{},
		},
	}
}

func touch(database *WorkspaceDatabase) {
	database.SavedAt = nowISO()
}

func applyMissionEvents(database *WorkspaceDatabase, events []map[string]any) {
	if len(database.Tables.MissionControlStates) == 0 {
		return
	}
	for _, event := range events {
		if nextStatus := statusFromEvent(text(event, "type")); nextStatus != "" {
			for index, item := range database.Tables.WorkItems {
				if text(item, "id") == text(event, "workItemId") {
					database.Tables.WorkItems[index]["status"] = nextStatus
					database.Tables.WorkItems[index]["updatedAt"] = nowISO()
				}
			}
		}
	}
	state := cloneMap(database.Tables.MissionControlStates[0])
	stateEvents := arrayMaps(state["events"])
	stateEvents = append(stateEvents, events...)
	state["events"] = stateEvents
	state["updatedAt"] = nowISO()
	database.Tables.MissionControlStates[0] = state
	runID := text(state, "runId")
	for index, event := range stateEvents {
		database.Tables.MissionEvents = appendOrReplace(database.Tables.MissionEvents, map[string]any{
			"id":       fmt.Sprintf("%s:event:%d", runID, index+1),
			"runId":    runID,
			"sequence": index + 1,
			"event":    event,
		})
	}
	touch(database)
}

func statusFromEvent(eventType string) string {
	switch eventType {
	case "operation.started":
		return "In Review"
	case "operation.completed":
		return "Done"
	case "operation.failed":
		return "Blocked"
	case "checkpoint.requested":
		return "Ready"
	default:
		return ""
	}
}

func upsertMissionAndOperation(database *WorkspaceDatabase, mission map[string]any, pipelineID string) {
	timestamp := nowISO()
	missionID := text(mission, "id")
	missionRecord := map[string]any{"id": missionID, "pipelineId": pipelineID, "workItemId": mission["sourceWorkItemId"], "title": mission["title"], "status": mission["status"], "mission": mission, "createdAt": timestamp, "updatedAt": timestamp}
	database.Tables.Missions = appendOrReplace(database.Tables.Missions, missionRecord)
	for _, operation := range arrayMaps(mission["operations"]) {
		operationRecord := map[string]any{"id": operation["id"], "missionId": missionID, "stageId": operation["stageId"], "agentId": operation["agentId"], "status": operation["status"], "prompt": operation["prompt"], "requiredProof": operation["requiredProof"], "createdAt": timestamp, "updatedAt": timestamp}
		database.Tables.Operations = appendOrReplace(database.Tables.Operations, operationRecord)
	}
	touch(database)
}

func upsertOperationStatus(database *WorkspaceDatabase, mission map[string]any, operationID string, status string) {
	for index, operation := range database.Tables.Operations {
		if text(operation, "id") == operationID {
			database.Tables.Operations[index]["status"] = status
			database.Tables.Operations[index]["updatedAt"] = nowISO()
			return
		}
	}
	upsertMissionAndOperation(database, mission, fmt.Sprintf("pipeline_%s", text(mission, "sourceWorkItemId")))
	upsertOperationStatus(database, mission, operationID, status)
}

func upsertOperationRunnerProcess(database *WorkspaceDatabase, operationID string, runnerProcess map[string]any) {
	if runnerProcess == nil {
		return
	}
	for index, operation := range database.Tables.Operations {
		if text(operation, "id") == operationID {
			database.Tables.Operations[index]["runnerProcess"] = runnerProcess
			database.Tables.Operations[index]["updatedAt"] = nowISO()
			return
		}
	}
}

func upsertPendingCheckpoint(database *WorkspaceDatabase, pipeline map[string]any) {
	run := mapValue(pipeline["run"])
	stage := firstStageWithStatus(run, "needs-human")
	if stage == nil {
		return
	}
	checkpoint := map[string]any{"id": fmt.Sprintf("%s:%s", text(pipeline, "id"), text(stage, "id")), "pipelineId": pipeline["id"], "stageId": stage["id"], "status": "pending", "title": fmt.Sprintf("%s 审批", text(stage, "title")), "summary": fmt.Sprintf("%s 需要人工确认后才能继续", text(stage, "title")), "createdAt": nowISO(), "updatedAt": nowISO()}
	if attemptIndex := latestAttemptIndexForPipeline(*database, text(pipeline, "id")); attemptIndex >= 0 {
		checkpoint["attemptId"] = text(database.Tables.Attempts[attemptIndex], "id")
	}
	database.Tables.Checkpoints = appendOrReplace(database.Tables.Checkpoints, checkpoint)
}

func approvePipelineStage(database *WorkspaceDatabase, checkpoint map[string]any) {
	for index, pipeline := range database.Tables.Pipelines {
		if text(pipeline, "id") != text(checkpoint, "pipelineId") {
			continue
		}
		run := mapValue(pipeline["run"])
		for _, stage := range arrayMaps(run["stages"]) {
			if text(stage, "id") == text(checkpoint, "stageId") {
				stage["status"] = "passed"
				stage["approvedBy"] = "human"
				markNextStageReady(run, text(stage, "id"))
			}
		}
		pipeline["run"] = run
		pipeline["status"] = pipelineStatusFromRun(run)
		pipeline["updatedAt"] = nowISO()
		database.Tables.Pipelines[index] = pipeline
	}
}

func (server *Server) canCompleteApprovedDevFlowCheckpointAsync(database WorkspaceDatabase, checkpoint map[string]any) bool {
	if text(checkpoint, "stageId") != "human_review" {
		return false
	}
	pipelineIndex := findByID(database.Tables.Pipelines, text(checkpoint, "pipelineId"))
	if pipelineIndex < 0 {
		return false
	}
	return isDevFlowPRTemplate(text(database.Tables.Pipelines[pipelineIndex], "templateId"))
}

func markApprovedDevFlowDeliveryQueued(database *WorkspaceDatabase, checkpoint map[string]any) {
	for index, pipeline := range database.Tables.Pipelines {
		if text(pipeline, "id") != text(checkpoint, "pipelineId") {
			continue
		}
		next := cloneMap(pipeline)
		run := mapValue(next["run"])
		appendRunEvent(run, "gate.approved", "Human review approved. Delivery merge is running in the background.", "human_review", "human")
		stages := arrayMaps(run["stages"])
		for _, stage := range stages {
			switch text(stage, "id") {
			case "human_review":
				stage["status"] = "passed"
				stage["approvedBy"] = "human"
				stage["completedAt"] = nowISO()
			case "merging":
				if text(stage, "status") == "ready" || text(stage, "status") == "waiting" {
					stage["status"] = "running"
					stage["startedAt"] = nowISO()
					stage["notes"] = "Merge is running after human approval."
				}
			}
		}
		run["stages"] = stages
		next["run"] = run
		next["status"] = "running"
		next["updatedAt"] = nowISO()
		database.Tables.Pipelines[index] = next
		upsertLatestRunWorkpadForPipeline(database, text(next, "id"))
		return
	}
}

func (server *Server) completeApprovedDevFlowCheckpointInBackground(checkpointID string, reviewer string) {
	go func() {
		ctx := context.Background()
		server.logInfo(ctx, "checkpoint.approve.delivery_started", "Approved delivery continuation started.", map[string]any{"entityType": "checkpoint", "entityId": checkpointID})
		database, err := mustLoad(server, ctx)
		if err != nil {
			server.logError(ctx, "checkpoint.approve.delivery_load_failed", err.Error(), map[string]any{"entityType": "checkpoint", "entityId": checkpointID})
			return
		}
		index := findByID(database.Tables.Checkpoints, checkpointID)
		if index < 0 {
			server.logError(ctx, "checkpoint.approve.delivery_checkpoint_missing", "Checkpoint not found for approved delivery continuation.", map[string]any{"entityType": "checkpoint", "entityId": checkpointID})
			return
		}
		checkpoint := cloneMap(database.Tables.Checkpoints[index])
		if err := server.completeApprovedDevFlowCheckpoint(&database, checkpoint, reviewer); err != nil {
			server.logError(ctx, "checkpoint.approve.delivery_failed", err.Error(), map[string]any{"entityType": "checkpoint", "entityId": checkpointID, "pipelineId": text(checkpoint, "pipelineId"), "stageId": text(checkpoint, "stageId")})
			return
		}
		touch(&database)
		if err := server.Repo.Save(ctx, database); err != nil {
			server.logError(ctx, "checkpoint.approve.delivery_save_failed", err.Error(), map[string]any{"entityType": "checkpoint", "entityId": checkpointID, "pipelineId": text(checkpoint, "pipelineId"), "stageId": text(checkpoint, "stageId")})
			return
		}
		server.logInfo(ctx, "checkpoint.approve.delivery_completed", "Approved delivery continuation completed.", map[string]any{"entityType": "checkpoint", "entityId": checkpointID, "pipelineId": text(checkpoint, "pipelineId"), "stageId": text(checkpoint, "stageId")})
	}()
}

func (server *Server) completeApprovedDevFlowCheckpoint(database *WorkspaceDatabase, checkpoint map[string]any, reviewer string) error {
	if text(checkpoint, "stageId") != "human_review" {
		return nil
	}
	pipelineIndex := findByID(database.Tables.Pipelines, text(checkpoint, "pipelineId"))
	if pipelineIndex < 0 {
		return nil
	}
	pipeline := cloneMap(database.Tables.Pipelines[pipelineIndex])
	if !isDevFlowPRTemplate(text(pipeline, "templateId")) {
		return nil
	}
	attemptIndex := attemptIndexForCheckpoint(*database, checkpoint)
	if attemptIndex < 0 {
		server.logError(context.Background(), "checkpoint.approve.missing_attempt", "Human review approval could not continue delivery because attempt record is missing.", map[string]any{"pipelineId": text(pipeline, "id"), "stageId": text(checkpoint, "stageId")})
		run := mapValue(pipeline["run"])
		appendRunEvent(run, "gate.approved.legacy", "Human review approved, but delivery continuation was skipped because this pipeline has no attempt record.", "human_review", "human")
		pipeline["run"] = run
		database.Tables.Pipelines[pipelineIndex] = pipeline
		return nil
	}
	attempt := cloneMap(database.Tables.Attempts[attemptIndex])
	workspace := text(attempt, "workspacePath")
	prURL := text(attempt, "pullRequestUrl")
	if workspace == "" || prURL == "" {
		server.logError(context.Background(), "checkpoint.approve.incomplete_attempt", "Human review approval could not continue delivery because workspace or pull request proof is missing.", map[string]any{"pipelineId": text(pipeline, "id"), "attemptId": text(attempt, "id"), "workspacePath": workspace, "pullRequestUrl": prURL})
		run := mapValue(pipeline["run"])
		appendRunEvent(run, "gate.approved.incomplete", "Human review approved, but delivery continuation was skipped because workspace or pull request proof is missing.", "human_review", "human")
		pipeline["run"] = run
		database.Tables.Pipelines[pipelineIndex] = pipeline
		return nil
	}
	repoWorkspace := filepath.Join(workspace, "repo")
	proofDir := filepath.Join(workspace, ".omega", "proof")
	if err := os.MkdirAll(proofDir, 0o755); err != nil {
		return err
	}
	item := findWorkItem(*database, text(pipeline, "workItemId"))

	humanReviewPath := filepath.Join(proofDir, "human-review.md")
	if err := os.WriteFile(humanReviewPath, []byte(fmt.Sprintf("# Human Review\n\n- Reviewer: %s\n- Decision: approved\n- Pull request: %s\n- Approved at: %s\n", reviewer, prURL, nowISO())), 0o644); err != nil {
		return err
	}
	if err := server.mergeApprovedDevFlowPullRequest(repoWorkspace, prURL, text(attempt, "branchName"), text(pipeline, "id"), text(attempt, "id")); err != nil {
		server.logError(context.Background(), "github.pr.merge_failed", err.Error(), map[string]any{"pipelineId": text(pipeline, "id"), "attemptId": text(attempt, "id"), "pullRequestUrl": prURL})
		if item != nil {
			report := server.syncGitHubIssueOutbound(context.Background(), githubOutboundSyncInput{
				RepositoryPath: repoWorkspace,
				Repository:     repositoryFromPullRequestURL(prURL),
				WorkItem:       item,
				Pipeline:       pipeline,
				Attempt:        attempt,
				Event:          "delivery.merge_failed",
				Status:         "failed",
				StageID:        "merging",
				Summary:        "Human review approved, but pull request merge failed.",
				PullRequestURL: prURL,
				BranchName:     text(attempt, "branchName"),
				FailureReason:  "Pull request merge failed after human approval.",
				FailureDetail:  err.Error(),
			})
			if text(report, "state") != "skipped" {
				_ = writeJSONFile(filepath.Join(proofDir, "github-outbound-sync-merge-failed.json"), report)
			}
		}
		return fmt.Errorf("merge pull request after human approval: %w", err)
	}
	server.logInfo(context.Background(), "github.pr.merged", "Pull request merged after human approval.", map[string]any{"pipelineId": text(pipeline, "id"), "attemptId": text(attempt, "id"), "pullRequestUrl": prURL})
	mergePath := filepath.Join(proofDir, "merge.md")
	if err := os.WriteFile(mergePath, []byte(fmt.Sprintf("# Merge\n\nMerged after human approval: %s\n", prURL)), 0o644); err != nil {
		return err
	}
	handoffPath := filepath.Join(proofDir, "handoff-bundle.json")
	if raw, err := os.ReadFile(handoffPath); err == nil {
		var handoff map[string]any
		if json.Unmarshal(raw, &handoff) == nil {
			handoff["merged"] = true
			handoff["humanGate"] = "approved"
			handoff["approvedBy"] = reviewer
			handoff["approvedAt"] = nowISO()
			_ = writeJSONFile(handoffPath, handoff)
		}
	}

	run := mapValue(pipeline["run"])
	stages := arrayMaps(run["stages"])
	for _, stage := range stages {
		switch text(stage, "id") {
		case "human_review":
			stage["status"] = "passed"
			stage["approvedBy"] = reviewer
			stage["completedAt"] = nowISO()
			stage["evidence"] = []string{humanReviewPath}
		case "merging":
			stage["status"] = "passed"
			stage["completedAt"] = nowISO()
			stage["evidence"] = []string{mergePath}
		case "done":
			stage["status"] = "passed"
			stage["completedAt"] = nowISO()
			stage["evidence"] = []string{handoffPath}
		}
	}
	run["stages"] = stages
	appendRunEvent(run, "gate.approved", fmt.Sprintf("Human review approved by %s.", reviewer), "human_review", "human")
	appendRunEvent(run, "devflow.cycle.completed", "DevFlow PR cycle completed after human approval.", "done", "delivery")
	pipeline["run"] = run
	pipeline["status"] = "done"
	pipeline["updatedAt"] = nowISO()
	database.Tables.Pipelines[pipelineIndex] = pipeline
	if item != nil {
		*database = updateWorkItem(*database, text(item, "id"), map[string]any{"status": "Done"})
	}

	proofFiles, _ := collectFiles(proofDir)
	githubOutboundSync := []map[string]any{}
	if item != nil {
		report := server.syncGitHubIssueOutbound(context.Background(), githubOutboundSyncInput{
			RepositoryPath: repoWorkspace,
			Repository:     repositoryFromPullRequestURL(prURL),
			WorkItem:       item,
			Pipeline:       pipeline,
			Attempt:        attempt,
			Event:          "delivery.completed",
			Status:         "done",
			StageID:        "done",
			Summary:        "Pull request merged after human approval.",
			PullRequestURL: prURL,
			BranchName:     text(attempt, "branchName"),
		})
		if text(report, "state") != "skipped" {
			githubOutboundSync = append(githubOutboundSync, report)
			_ = writeJSONFile(filepath.Join(proofDir, "github-outbound-sync-delivery-completed.json"), report)
			proofFiles, _ = collectFiles(proofDir)
		}
	}
	result := map[string]any{
		"status":         "done",
		"workspacePath":  workspace,
		"branchName":     text(attempt, "branchName"),
		"pullRequestUrl": prURL,
		"proofFiles":     proofFiles,
	}
	if len(githubOutboundSync) > 0 {
		result["githubOutboundSync"] = githubOutboundSync
	}
	nextDatabase, _ := completeAttemptRecord(*database, text(attempt, "id"), pipeline, result)
	*database = nextDatabase
	upsertRunWorkpad(database, text(attempt, "id"))
	for _, operation := range []map[string]any{
		{"id": fmt.Sprintf("%s:agent:human_review:human", text(pipeline, "id")), "missionId": fmt.Sprintf("mission_%s_agent_workflow", text(pipeline, "id")), "stageId": "human_review", "agentId": "human", "status": "passed", "prompt": "Human approved delivery checkpoint.", "requiredProof": []any{"human-decision"}, "summary": "Human review approved.", "createdAt": nowISO(), "updatedAt": nowISO()},
		{"id": fmt.Sprintf("%s:agent:merging:delivery", text(pipeline, "id")), "missionId": fmt.Sprintf("mission_%s_agent_workflow", text(pipeline, "id")), "stageId": "merging", "agentId": "delivery", "status": "passed", "prompt": "Merge approved pull request.", "requiredProof": []any{"pull-request"}, "summary": "Pull request merged after human approval.", "createdAt": nowISO(), "updatedAt": nowISO()},
	} {
		database.Tables.Operations = appendOrReplace(database.Tables.Operations, operation)
	}
	for index, proof := range []string{humanReviewPath, mergePath, handoffPath} {
		database.Tables.ProofRecords = appendOrReplace(database.Tables.ProofRecords, map[string]any{
			"id":          fmt.Sprintf("%s:human-delivery-proof:%d", text(pipeline, "id"), index+1),
			"operationId": fmt.Sprintf("%s:agent:human_review:human", text(pipeline, "id")),
			"label":       "human-delivery-proof",
			"value":       filepath.Base(proof),
			"sourcePath":  proof,
			"createdAt":   nowISO(),
		})
	}
	return nil
}

func (server *Server) mergeApprovedDevFlowPullRequest(repoWorkspace string, prURL string, branchName string, pipelineID string, attemptID string) error {
	output, err := runCommand(repoWorkspace, "gh", "pr", "merge", prURL, "--squash", "--subject", "Omega DevFlow cycle approved")
	if err != nil {
		if merged, viewOutput := pullRequestAlreadyMerged(repoWorkspace, prURL); merged {
			server.logInfo(context.Background(), "github.pr.merge_already_done", "Pull request was already merged; continuing approved delivery.", map[string]any{"pipelineId": pipelineID, "attemptId": attemptID, "pullRequestUrl": prURL, "output": truncateForProof(viewOutput, 1200)})
		} else {
			return err
		}
	} else {
		server.logDebug(context.Background(), "github.pr.merge_output", "Pull request merge command completed.", map[string]any{"pipelineId": pipelineID, "attemptId": attemptID, "pullRequestUrl": prURL, "output": truncateForProof(output, 1200)})
	}
	server.cleanupMergedDevFlowBranch(repoWorkspace, branchName, pipelineID, attemptID)
	return nil
}

func pullRequestAlreadyMerged(repoWorkspace string, prURL string) (bool, string) {
	output, err := runCommand(repoWorkspace, "gh", "pr", "view", prURL, "--json", "state", "--jq", ".state")
	if err != nil {
		return false, output
	}
	return strings.TrimSpace(output) == "MERGED", output
}

func (server *Server) cleanupMergedDevFlowBranch(repoWorkspace string, branchName string, pipelineID string, attemptID string) {
	branchName = strings.TrimSpace(branchName)
	if branchName == "" {
		return
	}
	if output, err := runCommand(repoWorkspace, "git", "branch", "-D", branchName); err != nil {
		server.logDebug(context.Background(), "github.pr.local_branch_cleanup_skipped", "Local branch cleanup skipped after merge.", map[string]any{"pipelineId": pipelineID, "attemptId": attemptID, "branchName": branchName, "output": truncateForProof(output+"\n"+err.Error(), 1200)})
	}
	if output, err := runCommand(repoWorkspace, "git", "push", "origin", "--delete", branchName); err != nil {
		server.logDebug(context.Background(), "github.pr.remote_branch_cleanup_skipped", "Remote branch cleanup skipped after merge.", map[string]any{"pipelineId": pipelineID, "attemptId": attemptID, "branchName": branchName, "output": truncateForProof(output+"\n"+err.Error(), 1200)})
	}
}

func latestAttemptIndexForPipeline(database WorkspaceDatabase, pipelineID string) int {
	for index := len(database.Tables.Attempts) - 1; index >= 0; index-- {
		if text(database.Tables.Attempts[index], "pipelineId") == pipelineID {
			return index
		}
	}
	return -1
}

func rejectPipelineStage(database *WorkspaceDatabase, checkpoint map[string]any, reason string) {
	for index, pipeline := range database.Tables.Pipelines {
		if text(pipeline, "id") != text(checkpoint, "pipelineId") {
			continue
		}
		run := mapValue(pipeline["run"])
		stages := arrayMaps(run["stages"])
		resetFollowing := false
		for _, stage := range stages {
			if resetFollowing {
				stage["status"] = "waiting"
				delete(stage, "approvedBy")
				delete(stage, "rejectionReason")
				delete(stage, "startedAt")
				delete(stage, "completedAt")
				delete(stage, "notes")
				continue
			}
			if text(stage, "id") == text(checkpoint, "stageId") {
				stage["status"] = "ready"
				stage["rejectionReason"] = reason
				delete(stage, "approvedBy")
				delete(stage, "startedAt")
				delete(stage, "completedAt")
				delete(stage, "notes")
				resetFollowing = true
				appendRunEvent(run, "gate.rejected", fmt.Sprintf("%s rejected: %s", text(stage, "title"), reason), text(stage, "id"), text(stage, "ownerAgentId"))
			}
		}
		run["stages"] = stages
		pipeline["run"] = run
		pipeline["status"] = "running"
		pipeline["updatedAt"] = nowISO()
		database.Tables.Pipelines[index] = pipeline
	}
}

func collectFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		files = append(files, path)
		return nil
	})
	return files, err
}

func writeJSONFile(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, _ := json.MarshalIndent(value, "", "  ")
	return os.WriteFile(path, raw, 0o644)
}

func writeAgentRuntimeSpec(path string, fields map[string]any) error {
	spec := map[string]any{
		"sandboxPolicy": "workspace-write",
		"cwdPolicy":     "operation-workspace-only",
		"createdAt":     nowISO(),
	}
	for key, value := range fields {
		spec[key] = value
	}
	return writeJSONFile(path, spec)
}

func truncateForProof(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "\n\n... truncated ..."
}

func firstProjectID(database WorkspaceDatabase) string {
	if len(database.Tables.Projects) == 0 {
		return "project_unknown"
	}
	return text(database.Tables.Projects[0], "id")
}

func text(record map[string]any, key string) string {
	return stringOr(record[key], "")
}

func stringOr(value any, fallback string) string {
	if value == nil {
		return fallback
	}
	text := fmt.Sprint(value)
	if text == "" {
		return fallback
	}
	return text
}

func mapValue(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	raw, _ := json.Marshal(value)
	var output map[string]any
	_ = json.Unmarshal(raw, &output)
	return output
}

func arrayMaps(value any) []map[string]any {
	if typed, ok := value.([]map[string]any); ok {
		return typed
	}
	raw, _ := json.Marshal(value)
	var output []map[string]any
	_ = json.Unmarshal(raw, &output)
	if output == nil {
		return []map[string]any{}
	}
	return output
}

func arrayValues(value any) []any {
	if values, ok := value.([]any); ok {
		return values
	}
	return nil
}

func arrayLength(value any) int {
	switch typed := value.(type) {
	case []any:
		return len(typed)
	case []string:
		return len(typed)
	default:
		return 0
	}
}

func anySlice(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case []string:
		output := make([]any, 0, len(typed))
		for _, item := range typed {
			output = append(output, item)
		}
		return output
	default:
		return []any{}
	}
}

func stringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		output := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
				output = append(output, text)
			}
		}
		return output
	default:
		return []string{}
	}
}

func firstStageWithStatus(run map[string]any, status string) map[string]any {
	stages := arrayMaps(run["stages"])
	for _, stage := range stages {
		if text(stage, "status") == status {
			run["stages"] = stages
			return stage
		}
	}
	run["stages"] = stages
	return nil
}

func markNextStageReady(run map[string]any, stageID string) {
	stages := arrayMaps(run["stages"])
	for index, stage := range stages {
		if text(stage, "id") == stageID && index+1 < len(stages) && text(stages[index+1], "status") == "waiting" {
			stages[index+1]["status"] = "ready"
		}
	}
	run["stages"] = stages
}

func appendRunEvent(run map[string]any, eventType, message, stageID, agentID string) {
	events := arrayMaps(run["events"])
	events = append(events, map[string]any{"id": fmt.Sprintf("event_%d", time.Now().UnixNano()), "type": eventType, "message": message, "timestamp": nowISO(), "stageId": stageID, "agentId": agentID})
	run["events"] = events
	run["updatedAt"] = nowISO()
}

func pipelineStatusFromRun(run map[string]any) string {
	stages := arrayMaps(run["stages"])
	allPassed := true
	for _, stage := range stages {
		if text(stage, "status") == "needs-human" {
			return "waiting-human"
		}
		if text(stage, "status") != "passed" {
			allPassed = false
		}
	}
	if allPassed {
		return "completed"
	}
	return "running"
}

func findOperation(mission map[string]any, operationID string) map[string]any {
	for _, operation := range arrayMaps(mission["operations"]) {
		if text(operation, "id") == operationID {
			return operation
		}
	}
	return nil
}

func findWorkItem(database WorkspaceDatabase, itemID string) map[string]any {
	for _, item := range database.Tables.WorkItems {
		if text(item, "id") == itemID {
			return item
		}
	}
	return nil
}

func appendOrReplace(records []map[string]any, record map[string]any) []map[string]any {
	id := text(record, "id")
	for index, candidate := range records {
		if text(candidate, "id") == id {
			records[index] = record
			return records
		}
	}
	return append(records, record)
}

func findByID(records []map[string]any, id string) int {
	for index, record := range records {
		if text(record, "id") == id {
			return index
		}
	}
	return -1
}

func pathID(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) > 2 {
		return parts[2]
	}
	return ""
}

func boolValue(value any) bool {
	if typed, ok := value.(bool); ok {
		return typed
	}
	return fmt.Sprint(value) == "true"
}

func safeSegment(input string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			return r
		}
		return '-'
	}, input)
}

func devFlowAttemptBranchName(itemKey string, attemptID string) string {
	itemSegment := strings.Trim(safeSegment(itemKey), ".-_")
	if itemSegment == "" {
		itemSegment = "item"
	}
	return fmt.Sprintf("omega/%s-%s-devflow", itemSegment, shortAttemptSegment(attemptID))
}

func devFlowAttemptWorkspaceName(itemKey string, attemptID string) string {
	itemSegment := strings.Trim(safeSegment(itemKey), ".-_")
	if itemSegment == "" {
		itemSegment = "item"
	}
	return fmt.Sprintf("%s-%s-devflow-pr", itemSegment, shortAttemptSegment(attemptID))
}

func devFlowRunBranchName(itemKey string) string {
	itemSegment := strings.Trim(safeSegment(itemKey), ".-_")
	if itemSegment == "" {
		itemSegment = "item"
	}
	return fmt.Sprintf("omega/%s-devflow", itemSegment)
}

func devFlowRunWorkspaceName(itemKey string) string {
	itemSegment := strings.Trim(safeSegment(itemKey), ".-_")
	if itemSegment == "" {
		itemSegment = "item"
	}
	return fmt.Sprintf("%s-devflow-pr", itemSegment)
}

func shortAttemptSegment(attemptID string) string {
	segment := strings.Trim(safeSegment(attemptID), ".-_")
	if segment == "" {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	if len(segment) > 18 {
		segment = segment[len(segment)-18:]
	}
	return segment
}

func repositoryTargetID(repo string) string {
	return "repo_" + safeSegment(strings.ReplaceAll(repo, "/", "_"))
}

func findRepositoryTarget(database WorkspaceDatabase, targetID string) map[string]any {
	for _, project := range database.Tables.Projects {
		for _, target := range arrayMaps(project["repositoryTargets"]) {
			if text(target, "id") == targetID {
				return target
			}
		}
	}
	return nil
}

func repositoryTargetLabel(target map[string]any) string {
	switch text(target, "kind") {
	case "github":
		return text(target, "owner") + "/" + text(target, "repo")
	case "local":
		return text(target, "path")
	default:
		return text(target, "id")
	}
}

func repositoryTargetCloneTarget(target map[string]any) string {
	switch text(target, "kind") {
	case "github":
		if url := text(target, "url"); url != "" {
			return url
		}
		return fmt.Sprintf("https://github.com/%s/%s", text(target, "owner"), text(target, "repo"))
	case "local":
		return text(target, "path")
	default:
		return text(target, "target")
	}
}

func resolveWorkItemRepositoryTarget(database WorkspaceDatabase, item map[string]any) (map[string]any, error) {
	next := cloneMap(item)
	targetID := text(next, "repositoryTargetId")
	if targetID == "" {
		return next, nil
	}
	target := findRepositoryTarget(database, targetID)
	if target == nil {
		return nil, fmt.Errorf("work item %s references missing repository target %s", text(next, "key"), targetID)
	}
	cloneTarget := repositoryTargetCloneTarget(target)
	if cloneTarget == "" {
		return nil, fmt.Errorf("repository target %s has no runnable clone target", targetID)
	}
	next["target"] = cloneTarget
	next["repositoryTargetLabel"] = repositoryTargetLabel(target)
	return next, nil
}

func (server *Server) resolveWorkItemRepositoryTarget(ctx context.Context, item map[string]any) (map[string]any, error) {
	if text(item, "repositoryTargetId") == "" {
		return cloneMap(item), nil
	}
	database, err := server.Repo.Load(ctx)
	if err != nil {
		return nil, err
	}
	return resolveWorkItemRepositoryTarget(*database, item)
}

func (server *Server) resolveMissionRepositoryTarget(ctx context.Context, mission map[string]any) (map[string]any, error) {
	next := cloneMap(mission)
	targetID := text(next, "repositoryTargetId")
	if targetID == "" {
		return next, nil
	}
	database, err := server.Repo.Load(ctx)
	if err != nil {
		return nil, err
	}
	target := findRepositoryTarget(*database, targetID)
	if target == nil {
		return nil, fmt.Errorf("mission %s references missing repository target %s", text(next, "sourceIssueKey"), targetID)
	}
	cloneTarget := repositoryTargetCloneTarget(target)
	if cloneTarget == "" {
		return nil, fmt.Errorf("repository target %s has no runnable clone target", targetID)
	}
	next["target"] = cloneTarget
	next["repositoryTargetLabel"] = repositoryTargetLabel(target)
	for _, operation := range arrayMaps(next["operations"]) {
		prompt := text(operation, "prompt")
		if !strings.Contains(prompt, "Repository target ID:") {
			operation["prompt"] = fmt.Sprintf("%s\nRepository target ID: %s\nRepository target: %s", prompt, targetID, cloneTarget)
		}
	}
	return next, nil
}

func upsertGitHubRepositoryTarget(database *WorkspaceDatabase, repo string, defaultBranch string, repositoryURL string) {
	if len(database.Tables.Projects) == 0 {
		database.Tables.Projects = append(database.Tables.Projects, map[string]any{
			"id":                "project_omega",
			"name":              "Omega",
			"description":       "",
			"team":              "Omega",
			"status":            "Active",
			"labels":            []any{},
			"repositoryTargets": []any{},
			"createdAt":         nowISO(),
			"updatedAt":         nowISO(),
		})
	}
	parts := strings.SplitN(repo, "/", 2)
	owner := repo
	name := repo
	if len(parts) == 2 {
		owner = parts[0]
		name = parts[1]
	}
	if defaultBranch == "" {
		defaultBranch = "main"
	}
	if repositoryURL == "" {
		repositoryURL = fmt.Sprintf("https://github.com/%s", repo)
	}
	target := map[string]any{
		"id":            repositoryTargetID(repo),
		"kind":          "github",
		"owner":         owner,
		"repo":          name,
		"defaultBranch": defaultBranch,
		"url":           repositoryURL,
	}
	project := cloneMap(database.Tables.Projects[0])
	targets := arrayMaps(project["repositoryTargets"])
	for _, candidate := range targets {
		if text(candidate, "id") == text(target, "id") {
			return
		}
	}
	nextTargets := make([]any, 0, len(targets)+1)
	for _, candidate := range targets {
		nextTargets = append(nextTargets, candidate)
	}
	nextTargets = append(nextTargets, target)
	project["repositoryTargets"] = nextTargets
	if text(project, "defaultRepositoryTargetId") == "" {
		project["defaultRepositoryTargetId"] = target["id"]
	}
	project["updatedAt"] = nowISO()
	database.Tables.Projects[0] = project
}

func deleteRepositoryTarget(database WorkspaceDatabase, targetID string) (WorkspaceDatabase, bool) {
	deleted := false
	deletedWorkItemIDs := map[string]bool{}
	for projectIndex, project := range database.Tables.Projects {
		targets := arrayMaps(project["repositoryTargets"])
		nextTargets := make([]any, 0, len(targets))
		for _, target := range targets {
			if text(target, "id") == targetID {
				deleted = true
				continue
			}
			nextTargets = append(nextTargets, target)
		}
		nextProject := cloneMap(project)
		nextProject["repositoryTargets"] = nextTargets
		if text(nextProject, "defaultRepositoryTargetId") == targetID {
			if len(nextTargets) > 0 {
				nextProject["defaultRepositoryTargetId"] = text(mapValue(nextTargets[0]), "id")
			} else {
				delete(nextProject, "defaultRepositoryTargetId")
			}
		}
		nextProject["updatedAt"] = nowISO()
		database.Tables.Projects[projectIndex] = nextProject
	}
	if !deleted {
		return database, false
	}

	nextWorkItems := make([]map[string]any, 0, len(database.Tables.WorkItems))
	deletedRequirementIDs := map[string]bool{}
	for _, item := range database.Tables.WorkItems {
		if text(item, "repositoryTargetId") == targetID {
			deletedWorkItemIDs[text(item, "id")] = true
			if requirementID := text(item, "requirementId"); requirementID != "" {
				deletedRequirementIDs[requirementID] = true
			}
			continue
		}
		nextWorkItems = append(nextWorkItems, item)
	}
	database.Tables.WorkItems = nextWorkItems
	nextRequirements := make([]map[string]any, 0, len(database.Tables.Requirements))
	for _, requirement := range database.Tables.Requirements {
		if text(requirement, "repositoryTargetId") == targetID || deletedRequirementIDs[text(requirement, "id")] {
			continue
		}
		nextRequirements = append(nextRequirements, requirement)
	}
	database.Tables.Requirements = nextRequirements
	for stateIndex, state := range database.Tables.MissionControlStates {
		nextState := cloneMap(state)
		items := arrayMaps(nextState["workItems"])
		nextItems := make([]any, 0, len(items))
		for _, item := range items {
			if deletedWorkItemIDs[text(item, "id")] {
				continue
			}
			nextItems = append(nextItems, item)
		}
		nextState["workItems"] = nextItems
		nextState["updatedAt"] = nowISO()
		database.Tables.MissionControlStates[stateIndex] = nextState
	}

	deletedPipelineIDs := map[string]bool{}
	nextPipelines := make([]map[string]any, 0, len(database.Tables.Pipelines))
	for _, pipeline := range database.Tables.Pipelines {
		if deletedWorkItemIDs[text(pipeline, "workItemId")] {
			deletedPipelineIDs[text(pipeline, "id")] = true
			continue
		}
		nextPipelines = append(nextPipelines, pipeline)
	}
	database.Tables.Pipelines = nextPipelines

	nextCheckpoints := make([]map[string]any, 0, len(database.Tables.Checkpoints))
	for _, checkpoint := range database.Tables.Checkpoints {
		if deletedPipelineIDs[text(checkpoint, "pipelineId")] {
			continue
		}
		nextCheckpoints = append(nextCheckpoints, checkpoint)
	}
	database.Tables.Checkpoints = nextCheckpoints

	deletedMissionIDs := map[string]bool{}
	nextMissions := make([]map[string]any, 0, len(database.Tables.Missions))
	for _, mission := range database.Tables.Missions {
		if deletedWorkItemIDs[text(mission, "workItemId")] || deletedPipelineIDs[text(mission, "pipelineId")] {
			deletedMissionIDs[text(mission, "id")] = true
			continue
		}
		nextMissions = append(nextMissions, mission)
	}
	database.Tables.Missions = nextMissions

	deletedOperationIDs := map[string]bool{}
	nextOperations := make([]map[string]any, 0, len(database.Tables.Operations))
	for _, operation := range database.Tables.Operations {
		if deletedMissionIDs[text(operation, "missionId")] {
			deletedOperationIDs[text(operation, "id")] = true
			continue
		}
		nextOperations = append(nextOperations, operation)
	}
	database.Tables.Operations = nextOperations

	nextProofs := make([]map[string]any, 0, len(database.Tables.ProofRecords))
	for _, proof := range database.Tables.ProofRecords {
		if deletedOperationIDs[text(proof, "operationId")] {
			continue
		}
		nextProofs = append(nextProofs, proof)
	}
	database.Tables.ProofRecords = nextProofs
	return database, true
}

func defaultGitHubRedirectURI() string {
	return "http://127.0.0.1:3888/auth/github/callback"
}

func defaultGitHubTokenURL() string {
	return "https://github.com/login/oauth/access_token"
}

func (server *Server) effectiveGitHubOAuthConfig(ctx context.Context) GitHubOAuthConfig {
	config := server.GitHubOAuth
	config.RedirectURI = stringOr(config.RedirectURI, defaultGitHubRedirectURI())
	config.TokenURL = stringOr(config.TokenURL, defaultGitHubTokenURL())
	if saved, err := server.Repo.GetSetting(ctx, "github_oauth_config"); err == nil {
		config.ClientID = stringOr(text(saved, "clientId"), config.ClientID)
		config.ClientSecret = stringOr(text(saved, "clientSecret"), config.ClientSecret)
		config.RedirectURI = stringOr(text(saved, "redirectUri"), config.RedirectURI)
		config.TokenURL = stringOr(text(saved, "tokenUrl"), config.TokenURL)
	}
	return config
}

func (server *Server) githubOAuthConfigInfo(ctx context.Context) map[string]any {
	config := server.effectiveGitHubOAuthConfig(ctx)
	source := "empty"
	if _, err := server.Repo.GetSetting(ctx, "github_oauth_config"); err == nil {
		source = "app"
	} else if server.GitHubOAuth.ClientID != "" || server.GitHubOAuth.ClientSecret != "" {
		source = "env"
	}
	return githubOAuthConfigInfo(config, source)
}

func githubOAuthConfigInfo(config GitHubOAuthConfig, source string) map[string]any {
	return map[string]any{
		"configured":       githubOAuthConfigured(config),
		"clientId":         config.ClientID,
		"redirectUri":      stringOr(config.RedirectURI, defaultGitHubRedirectURI()),
		"tokenUrl":         stringOr(config.TokenURL, defaultGitHubTokenURL()),
		"secretConfigured": config.ClientSecret != "",
		"source":           source,
	}
}

func githubOAuthConfigured(config GitHubOAuthConfig) bool {
	return config.ClientID != "" && config.ClientSecret != "" && config.RedirectURI != "" && config.TokenURL != ""
}

func githubAuthorizeURL(config GitHubOAuthConfig, scopes []string, state string) (string, error) {
	authorizeURL, err := url.Parse("https://github.com/login/oauth/authorize")
	if err != nil {
		return "", err
	}
	query := authorizeURL.Query()
	query.Set("client_id", config.ClientID)
	query.Set("redirect_uri", config.RedirectURI)
	query.Set("scope", strings.Join(scopes, " "))
	query.Set("state", state)
	authorizeURL.RawQuery = query.Encode()
	return authorizeURL.String(), nil
}

func randomState() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "omega_" + hex.EncodeToString(raw), nil
}

func (server *Server) exchangeGitHubOAuthCode(ctx context.Context, code string, config GitHubOAuthConfig) (map[string]any, error) {
	body, _ := json.Marshal(map[string]string{
		"client_id":     config.ClientID,
		"client_secret": config.ClientSecret,
		"code":          code,
		"redirect_uri":  config.RedirectURI,
	})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, config.TokenURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("accept", "application/json")
	request.Header.Set("content-type", "application/json")
	client := server.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode >= 400 {
		return nil, fmt.Errorf("GitHub OAuth exchange failed: %d", response.StatusCode)
	}
	var token map[string]any
	if err := json.NewDecoder(response.Body).Decode(&token); err != nil {
		return nil, err
	}
	if text(token, "access_token") == "" {
		return nil, errors.New("GitHub OAuth response did not include access_token")
	}
	return token, nil
}

func scopesFromString(input string) []any {
	scopes := []any{}
	for _, scope := range strings.Split(input, ",") {
		trimmed := strings.TrimSpace(scope)
		if trimmed != "" {
			scopes = append(scopes, trimmed)
		}
	}
	return scopes
}

func upsertGitHubConnection(database *WorkspaceDatabase, connectedAs string) {
	record := map[string]any{
		"providerId":         "github",
		"status":             "connected",
		"grantedPermissions": []any{"repo:read", "pull_request:write", "checks:read", "issues:write"},
		"connectedAs":        stringOr(connectedAs, "github"),
		"updatedAt":          nowISO(),
	}
	for index, connection := range database.Tables.Connections {
		if text(connection, "providerId") == "github" {
			database.Tables.Connections[index] = record
			return
		}
	}
	database.Tables.Connections = append(database.Tables.Connections, record)
}

func githubIssueToWorkItem(repo string, issue map[string]any) map[string]any {
	number := intValue(issue["number"])
	labels := []any{"github"}
	for _, label := range arrayMaps(issue["labels"]) {
		if name := text(label, "name"); name != "" {
			labels = append(labels, name)
		}
	}
	assignee := "requirement"
	assignees := arrayMaps(issue["assignees"])
	if len(assignees) > 0 {
		assignee = stringOr(assignees[0]["login"], assignee)
	}
	return map[string]any{
		"id":                 fmt.Sprintf("github_%s_%d", safeSegment(repo), number),
		"key":                fmt.Sprintf("GH-%d", number),
		"title":              stringOr(issue["title"], fmt.Sprintf("GitHub issue #%d", number)),
		"description":        stringOr(issue["body"], ""),
		"status":             "Ready",
		"priority":           "Medium",
		"assignee":           assignee,
		"labels":             labels,
		"team":               "Omega",
		"stageId":            "intake",
		"target":             stringOr(issue["url"], repo),
		"source":             "github_issue",
		"sourceExternalRef":  fmt.Sprintf("%s#%d", repo, number),
		"repositoryTargetId": repositoryTargetID(repo),
		"acceptanceCriteria": []any{"Imported GitHub issue is understood", "Implementation resolves the issue"},
		"blockedByItemIds":   []any{},
	}
}
