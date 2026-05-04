package omegalocal

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

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
		if shouldPersistAPIRequestLog(request.Method, path, status) {
			level := runtimeLogLevelForStatus(status, request.Method)
			server.logRuntime(context.Background(), level, "api.request", fmt.Sprintf("%s %s -> %d", request.Method, path, status), map[string]any{
				"requestId":  requestID,
				"method":     request.Method,
				"path":       path,
				"status":     status,
				"bytes":      logger.bytes,
				"durationMs": time.Since(startedAt).Milliseconds(),
			})
		} else if shouldWriteAPIRequestDiagnosticLog(request.Method, path, status) {
			server.logRuntimeDiagnosticFile("DEBUG", "api.request", fmt.Sprintf("%s %s -> %d", request.Method, path, status), map[string]any{
				"requestId":  requestID,
				"method":     request.Method,
				"path":       path,
				"status":     status,
				"bytes":      logger.bytes,
				"durationMs": time.Since(startedAt).Milliseconds(),
			})
		}
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
	case request.Method == http.MethodPost && path == "/projects":
		server.createProject(response, request)
	case request.Method == http.MethodGet && path == "/requirements":
		server.listTable(response, request, "requirements")
	case request.Method == http.MethodGet && path == "/repository-targets":
		server.listRepositoryTargets(response, request)
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
	case request.Method == http.MethodGet && strings.HasPrefix(path, "/proof-records/") && strings.HasSuffix(path, "/preview"):
		server.proofRecordPreview(response, request)
	case request.Method == http.MethodGet && path == "/handoff-bundles":
		server.listHandoffBundles(response, request)
	case request.Method == http.MethodGet && path == "/operation-queue":
		server.listOperationQueue(response, request)
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
		server.listWorkflowTemplates(response, request)
	case request.Method == http.MethodPost && path == "/workflow-templates/validate":
		server.validateWorkflowTemplate(response, request)
	case request.Method == http.MethodPut && strings.HasPrefix(path, "/workflow-templates/"):
		server.putWorkflowTemplate(response, request)
	case request.Method == http.MethodPost && strings.HasPrefix(path, "/workflow-templates/") && strings.HasSuffix(path, "/restore-default"):
		server.restoreWorkflowTemplateDefault(response, request)
	case request.Method == http.MethodPost && path == "/pipelines/from-template":
		server.createPipelineFromTemplate(response, request)
	case request.Method == http.MethodGet && path == "/llm-providers":
		writeJSON(response, http.StatusOK, llmProviders())
	case request.Method == http.MethodGet && path == "/llm-provider-selection":
		server.getProviderSelection(response, request)
	case request.Method == http.MethodPut && path == "/llm-provider-selection":
		server.putProviderSelection(response, request)
	case request.Method == http.MethodGet && path == "/runner-credentials":
		server.listRunnerCredentials(response, request)
	case request.Method == http.MethodPut && path == "/runner-credentials":
		server.putRunnerCredential(response, request)
	case request.Method == http.MethodGet && path == "/agent-definitions":
		server.listAgentDefinitions(response, request)
	case request.Method == http.MethodGet && path == "/agent-profile":
		server.getAgentProfile(response, request)
	case request.Method == http.MethodPut && path == "/agent-profile":
		server.putAgentProfile(response, request)
	case request.Method == http.MethodPost && path == "/agent-profile/import-template":
		server.importAgentProfileTemplate(response, request)
	case request.Method == http.MethodGet && path == "/observability":
		server.observability(response, request)
	case request.Method == http.MethodGet && path == "/runtime-logs":
		server.runtimeLogs(response, request)
	case request.Method == http.MethodGet && path == "/runtime-logs/export":
		server.runtimeLogsExport(response, request)
	case request.Method == http.MethodGet && path == "/local-capabilities":
		server.localCapabilities(response, request)
	case request.Method == http.MethodPost && path == "/agent-runner/preflight":
		server.testAgentRunner(response, request)
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
	case request.Method == http.MethodGet && path == "/feishu/config":
		server.getFeishuConfig(response, request)
	case request.Method == http.MethodPut && path == "/feishu/config":
		server.putFeishuConfig(response, request)
	case request.Method == http.MethodPost && path == "/feishu/config/test":
		server.testFeishuConfig(response, request)
	case request.Method == http.MethodPost && path == "/feishu/users/search":
		server.searchFeishuUsers(response, request)
	case request.Method == http.MethodPost && path == "/feishu/notify":
		server.feishuNotify(response, request)
	case request.Method == http.MethodPost && path == "/feishu/review-request":
		server.feishuReviewRequest(response, request)
	case request.Method == http.MethodPost && path == "/feishu/review-task/sync":
		server.feishuReviewTaskSync(response, request)
	case request.Method == http.MethodPost && path == "/feishu/review-task/comment":
		server.feishuReviewTaskComment(response, request)
	case request.Method == http.MethodPost && path == "/feishu/review-task/bridge/tick":
		server.feishuReviewTaskBridgeTick(response, request)
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

func shouldWriteAPIRequestDiagnosticLog(method string, path string, status int) bool {
	return status < 400 && path != "/health"
}

func shouldPersistAPIRequestLog(method string, path string, status int) bool {
	if status >= 400 {
		return true
	}
	if strings.EqualFold(os.Getenv("OMEGA_RUNTIME_LOG_DEBUG_API"), "true") {
		return path != "/health"
	}
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}
