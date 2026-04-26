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
		HTTPClient: http.DefaultClient,
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
	case request.Method == http.MethodGet && path == "/checkpoints":
		server.listTable(response, request, "checkpoints")
	case request.Method == http.MethodGet && path == "/missions":
		server.listTable(response, request, "missions")
	case request.Method == http.MethodGet && path == "/operations":
		server.listTable(response, request, "operations")
	case request.Method == http.MethodGet && path == "/proof-records":
		server.listTable(response, request, "proofRecords")
	case request.Method == http.MethodGet && path == "/execution-locks":
		server.listExecutionLocks(response, request)
	case request.Method == http.MethodPost && strings.HasPrefix(path, "/execution-locks/") && strings.HasSuffix(path, "/release"):
		server.releaseExecutionLock(response, request)
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
	case request.Method == http.MethodGet && path == "/observability":
		server.observability(response, request)
	case request.Method == http.MethodGet && path == "/local-capabilities":
		server.localCapabilities(response, request)
	case request.Method == http.MethodGet && path == "/local-workspace-root":
		server.getLocalWorkspaceRoot(response, request)
	case request.Method == http.MethodPut && path == "/local-workspace-root":
		server.putLocalWorkspaceRoot(response, request)
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
		writeJSON(response, http.StatusOK, emptyObservability())
		return
	}
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, observabilitySummary(*database))
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
	if err := server.Repo.Save(request.Context(), database); err != nil {
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
	}
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
	var attempt map[string]any
	database, pipeline, attempt = beginDevFlowAttempt(database, pipelineIndex, stageItem, pipeline, "manual")
	if err := server.Repo.Save(request.Context(), database); err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	if !payload.Wait {
		server.startDevFlowCycleJob(text(pipeline, "id"), text(attempt, "id"), payload.AutoApproveHuman, payload.AutoMerge, nil)
		writeJSON(response, http.StatusAccepted, map[string]any{
			"status":   "accepted",
			"pipeline": pipeline,
			"attempt":  attempt,
		})
		return
	}
	runContext, cancelRun := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancelRun()
	result, err := server.executeDevFlowPRCycle(runContext, pipeline, stageItem, target, text(attempt, "id"), payload.AutoApproveHuman, payload.AutoMerge)
	if err != nil {
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

func beginDevFlowAttempt(database WorkspaceDatabase, pipelineIndex int, item map[string]any, pipeline map[string]any, trigger string) (WorkspaceDatabase, map[string]any, map[string]any) {
	pipeline = resetDevFlowPipelineForAttempt(pipeline)
	pipeline["status"] = "running"
	pipeline["updatedAt"] = nowISO()
	database.Tables.Pipelines[pipelineIndex] = pipeline
	database = updateWorkItem(database, text(item, "id"), map[string]any{"status": "In Review"})
	attempt := makeAttemptRecord(item, pipeline, trigger, "devflow-pr", "todo")
	database.Tables.Attempts = appendOrReplace(database.Tables.Attempts, attempt)
	touch(&database)
	return database, pipeline, attempt
}

func resetDevFlowPipelineForAttempt(pipeline map[string]any) map[string]any {
	next := cloneMap(pipeline)
	run := mapValue(next["run"])
	stages := arrayMaps(run["stages"])
	for index, stage := range stages {
		if index == 0 {
			stage["status"] = "running"
			stage["startedAt"] = nowISO()
		} else {
			stage["status"] = "waiting"
			delete(stage, "startedAt")
		}
		delete(stage, "completedAt")
		delete(stage, "notes")
		stage["evidence"] = []any{}
	}
	run["stages"] = stages
	events := arrayMaps(run["events"])
	events = append(events, map[string]any{
		"id":        fmt.Sprintf("event_%d", time.Now().UnixNano()),
		"type":      "attempt.reset",
		"message":   "Pipeline stages reset for a new attempt.",
		"timestamp": nowISO(),
		"stageId":   "todo",
		"agentId":   "master",
	})
	run["events"] = events
	next["run"] = run
	next["updatedAt"] = nowISO()
	return next
}

func (server *Server) startDevFlowCycleJob(pipelineID string, attemptID string, autoApproveHuman bool, autoMerge bool, lock map[string]any) {
	go func() {
		runContext, cancelRun := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancelRun()
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

		database, err := mustLoad(server, runContext)
		if err != nil {
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
			_ = server.failDevFlowCycleJobWithResult(context.Background(), pipelineID, attemptID, err, result)
			releaseLock("failed")
			return
		}
		if _, _, err := server.completeDevFlowCycleJob(context.Background(), pipelineID, attemptID, result); err != nil {
			_ = server.failDevFlowCycleJob(context.Background(), pipelineID, attemptID, err)
			releaseLock("failed")
			return
		}
		releaseLock("completed")
	}()
}

func (server *Server) failDevFlowCycleJob(ctx context.Context, pipelineID string, attemptID string, failure error) error {
	return server.failDevFlowCycleJobWithResult(ctx, pipelineID, attemptID, failure, nil)
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
	database, _ = failAttemptRecord(database, attemptID, pipeline, failure.Error(), result)
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
	database, pipeline, item = applyDevFlowCycleResult(database, pipelineIndex, item, result)
	database, _ = completeAttemptRecord(database, attemptID, pipeline, result)
	touch(&database)
	if err := server.Repo.Save(context.Background(), database); err != nil {
		return nil, nil, err
	}
	return pipeline, item, nil
}

func applyDevFlowCycleResult(database WorkspaceDatabase, pipelineIndex int, item map[string]any, result map[string]any) (WorkspaceDatabase, map[string]any, map[string]any) {
	pipeline := cloneMap(database.Tables.Pipelines[pipelineIndex])
	run := mapValue(pipeline["run"])
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

func devFlowStageStatusAfterInvocation(stageID string, agentID string, status string) (string, string) {
	if status == "running" {
		return "running", ""
	}
	if status == "failed" {
		return "failed", ""
	}
	if status == "changes-requested" {
		return "passed", "rework"
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
	return "passed", devFlowNextStageAfter(stageID)
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
	stageStatus, nextStageID := devFlowStageStatusAfterInvocation(text(invocation, "stageId"), text(invocation, "agentId"), text(invocation, "status"))
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
		} else {
			attempt["status"] = "running"
		}
		attempt["currentStageId"] = text(invocation, "stageId")
		attempt["stages"] = attemptStageSnapshot(pipeline)
		attempt["updatedAt"] = nowISO()
		events := arrayMaps(attempt["events"])
		events = append(events, map[string]any{
			"type":      "agent." + text(invocation, "status"),
			"message":   stringOr(text(invocation, "summary"), text(invocation, "agentId")+" "+text(invocation, "status")),
			"stageId":   text(invocation, "stageId"),
			"createdAt": nowISO(),
		})
		attempt["events"] = events
		database.Tables.Attempts[attemptIndex] = attempt
	}

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
	operationID := stringOr(text(invocation, "operationId"), text(invocation, "id"))
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
	attempt["stdoutSummary"] = truncateForProof(stringOr(result["stdout"], ""), 800)
	attempt["stderrSummary"] = truncateForProof(stringOr(result["stderr"], ""), 800)
	attempt["stages"] = attemptStageSnapshot(pipeline)
	attempt["finishedAt"] = timestamp
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
		attempt["workspacePath"] = stringOr(result["workspacePath"], text(attempt, "workspacePath"))
		attempt["branchName"] = stringOr(result["branchName"], text(attempt, "branchName"))
		attempt["pullRequestUrl"] = stringOr(result["pullRequestUrl"], text(attempt, "pullRequestUrl"))
		attempt["stdoutSummary"] = truncateForProof(stringOr(result["stdout"], ""), 800)
	}
	attempt["stderrSummary"] = truncateForProof(message, 800)
	attempt["stages"] = attemptStageSnapshot(pipeline)
	attempt["finishedAt"] = timestamp
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
	database.Tables.Attempts[index] = attempt
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
		if _, branchErr := runCommand(repoWorkspace, "git", "checkout", "-B", branchName, "origin/"+baseBranch); branchErr != nil {
			if _, fallbackErr := runCommand(repoWorkspace, "git", "checkout", "-B", branchName); fallbackErr != nil {
				return "", fallbackErr
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
	workspaceRoot := server.localWorkspaceRoot(ctx)
	server.WorkspaceRoot = workspaceRoot
	workspace, err := workspaceChildPath(workspaceRoot, devFlowRunWorkspaceName(text(item, "key")))
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return nil, err
	}
	repoWorkspace := filepath.Join(workspace, "repo")
	cloneTarget := repositoryTargetCloneTarget(target)
	if cloneTarget == "" {
		return nil, fmt.Errorf("repository target %s has no clone target", text(target, "id"))
	}
	repoSlug := repositoryTargetLabel(target)
	baseBranch := stringOr(text(target, "defaultBranch"), "main")
	branchName := devFlowRunBranchName(text(item, "key"))
	if output, err := ensureDevFlowRepositoryWorkspace(workspace, repoWorkspace, cloneTarget, branchName, baseBranch); err != nil {
		return map[string]any{"status": "failed", "workspacePath": workspace, "stdout": output}, fmt.Errorf("clone target repository: %w", err)
	}
	agentRunner := CodexExecAgentRunner{}

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
	}); err != nil {
		return nil, err
	}
	agentInvocations := []map[string]any{}
	stageArtifacts := []map[string]any{}
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

	requirementArtifact := map[string]any{
		"workItemId":          text(item, "id"),
		"workItemKey":         text(item, "key"),
		"title":               text(item, "title"),
		"description":         text(item, "description"),
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
	requirementPrompt := fmt.Sprintf("Structure requirement %s for repository %s.\n\nTitle: %s\n\nDescription:\n%s", text(item, "key"), repoSlug, text(item, "title"), text(item, "description"))
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
	solutionPrompt := fmt.Sprintf("Design implementation for %s in %s.\n\nRequirement:\n%s", text(item, "key"), repoSlug, text(item, "description"))
	recordAgent("in_progress", "architect", "passed", solutionPrompt, "solution-plan.md", "Solution plan created and handed to the coding agent.", []string{filepath.Join(proofDir, "solution-plan.md")}, map[string]any{"runner": "local-orchestrator", "status": "passed"})

	codingPrompt := fmt.Sprintf(`You are the coding agent for Omega.

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
- Write a short completion note to %s.
`, repoSlug, repoWorkspace, text(item, "key"), text(item, "title"), text(item, "description"), filepath.Join(proofDir, "coding-agent-note.md"))
	if err := os.WriteFile(filepath.Join(proofDir, "coding-prompt.md"), []byte(codingPrompt), 0o644); err != nil {
		return nil, err
	}
	recordAgent("in_progress", "coding", "running", codingPrompt, "", "Coding agent is editing the repository workspace.", []string{filepath.Join(proofDir, "coding-prompt.md")}, map[string]any{"runner": "codex", "status": "running"})
	codingTurn := agentRunner.RunTurn(ctx, AgentTurnRequest{
		Role:       "coding",
		StageID:    "in_progress",
		Runner:     "codex",
		Workspace:  repoWorkspace,
		Prompt:     codingPrompt,
		OutputPath: filepath.Join(proofDir, "coding-agent-note.md"),
		Sandbox:    "workspace-write",
	})
	codingProcess, codingErr := codingTurn.Process, codingTurn.Error
	if codingErr != nil {
		recordAgent("in_progress", "coding", "failed", codingPrompt, "coding-agent-note.md", "Coding agent failed before producing an acceptable repository diff.", []string{filepath.Join(proofDir, "coding-prompt.md"), filepath.Join(proofDir, "coding-agent-note.md")}, codingProcess)
		return map[string]any{"status": "failed", "workspacePath": workspace, "repositoryPath": repoWorkspace, "agentInvocations": agentInvocations, "stageArtifacts": stageArtifacts}, fmt.Errorf("coding agent failed: %w", codingErr)
	}
	statusOutput, err := runCommand(repoWorkspace, "git", "status", "--short")
	if err != nil {
		return nil, fmt.Errorf("read coding agent changes: %w", err)
	}
	if strings.TrimSpace(statusOutput) == "" {
		recordAgent("in_progress", "coding", "failed", codingPrompt, "coding-agent-note.md", "Coding agent produced no repository changes.", []string{filepath.Join(proofDir, "coding-prompt.md"), filepath.Join(proofDir, "coding-agent-note.md")}, codingProcess)
		return map[string]any{"status": "failed", "workspacePath": workspace, "repositoryPath": repoWorkspace, "agentInvocations": agentInvocations, "stageArtifacts": stageArtifacts}, errors.New("coding agent produced no repository changes")
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

	testPrompt := fmt.Sprintf("Validate %s after coding changes. Changed files: %s", text(item, "key"), strings.Join(changedFiles, ", "))
	testOutput, testErr := runRepositoryValidation(repoWorkspace)
	testStatus := "passed"
	if testErr != nil {
		testStatus = "failed"
	}
	testReport := fmt.Sprintf("# Test Report\n\nStatus: %s\n\n```text\n%s\n```\n", testStatus, stringOr(strings.TrimSpace(testOutput), "No validation output."))
	if err := os.WriteFile(filepath.Join(proofDir, "test-report.md"), []byte(testReport), 0o644); err != nil {
		return nil, err
	}
	recordAgent("in_progress", "testing", testStatus, testPrompt, "test-report.md", "Repository validation completed.", []string{filepath.Join(proofDir, "test-report.md")}, map[string]any{"runner": "local-validation", "status": testStatus, "stdout": testOutput})
	if testErr != nil {
		return map[string]any{"status": "failed", "workspacePath": workspace, "repositoryPath": repoWorkspace, "agentInvocations": agentInvocations, "stageArtifacts": stageArtifacts}, fmt.Errorf("repository validation failed: %w", testErr)
	}

	if text(target, "kind") == "github" {
		_, _ = runCommand(repoWorkspace, "gh", "auth", "setup-git")
	}
	if err := pushDevFlowBranch(repoWorkspace, branchName); err != nil {
		return nil, fmt.Errorf("push branch: %w", err)
	}

	prBody := fmt.Sprintf("## Omega DevFlow Cycle\n\n### Work item\n- %s %s\n\n### Changed\n%s\n### Validation\n```text\n%s\n```\n", text(item, "key"), text(item, "title"), markdownFileList(changedFiles), truncateForProof(testOutput, 2000))
	prURL, err := ensureDevFlowPullRequest(repoWorkspace, repoSlug, branchName, baseBranch, text(item, "key")+" "+text(item, "title"), prBody)
	if err != nil {
		return nil, fmt.Errorf("create pull request: %w", err)
	}
	prDiff, _ := runCommand(repoWorkspace, "gh", "pr", "diff", prURL)
	checksOutput, _ := runCommand(repoWorkspace, "gh", "pr", "checks", prURL)

	template := findPipelineTemplate(text(pipeline, "templateId"))
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
		reworkPrompt := fmt.Sprintf(`You are the rework coding agent for Omega.

Repository: %s
Repository path: %s
Work item: %s
Title: %s
Pull request: %s

Requirement:
%s

Review feedback to address:
%s

Rules:
- Continue in the same repository checkout, same branch, and same pull request.
- Address the review feedback with a real code change.
- Keep the diff minimal and reviewable.
- Do not commit, push, or create a pull request. Omega will handle git delivery after you finish editing.
- Write a short completion note to %s.
`, repoSlug, repoWorkspace, text(item, "key"), text(item, "title"), prURL, text(item, "description"), feedback, notePath)
		if err := os.WriteFile(promptPath, []byte(reworkPrompt), 0o644); err != nil {
			return err
		}
		recordAgent(stageID, "coding", "running", reworkPrompt, "", "Rework agent is applying review feedback in the same workspace.", []string{promptPath}, map[string]any{"runner": "codex", "status": "running", "reviewCycle": cycle})
		turn := agentRunner.RunTurn(ctx, AgentTurnRequest{
			Role:       "rework",
			StageID:    stageID,
			Runner:     "codex",
			Workspace:  repoWorkspace,
			Prompt:     reworkPrompt,
			OutputPath: notePath,
			Sandbox:    "workspace-write",
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
		testPrompt := fmt.Sprintf("Validate %s after rework round %d. Changed files: %s", text(item, "key"), cycle, strings.Join(changedFiles, ", "))
		testOutput, testErr = runRepositoryValidation(repoWorkspace)
		testStatus := "passed"
		if testErr != nil {
			testStatus = "failed"
		}
		testReportPath := filepath.Join(proofDir, fmt.Sprintf("test-report-rework-%d.md", cycle))
		testReport := fmt.Sprintf("# Rework Test Report\n\nStatus: %s\n\n```text\n%s\n```\n", testStatus, stringOr(strings.TrimSpace(testOutput), "No validation output."))
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
		prDiff, _ = runCommand(repoWorkspace, "gh", "pr", "diff", prURL)
		checksOutput, _ = runCommand(repoWorkspace, "gh", "pr", "checks", prURL)
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
			reviewPrompt := buildDevFlowReviewPrompt(item, repoSlug, prURL, changedFiles, reviewDiff, testOutput, reviewChecks, reviewRound.Focus)
			reviewProcess, reviewErr := runDevFlowReviewAgent(repoWorkspace, reviewPrompt, reviewPath)
			if reviewErr != nil {
				recordAgent(stageID, "review", "failed", reviewPrompt, artifact, "Review agent failed before issuing a verdict.", []string{reviewPath}, reviewProcess)
				return map[string]any{"status": "failed", "workspacePath": workspace, "repositoryPath": repoWorkspace, "agentInvocations": agentInvocations, "stageArtifacts": stageArtifacts}, fmt.Errorf("%s failed: %w", stageID, reviewErr)
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
				if raw, err := os.ReadFile(reviewPath); err == nil {
					reviewFeedback = string(raw)
				} else {
					reviewFeedback = outcome.Summary
				}
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
			return map[string]any{"status": "failed", "workspacePath": workspace, "repositoryPath": repoWorkspace, "agentInvocations": agentInvocations, "stageArtifacts": stageArtifacts}, err
		}
	}

	humanReviewRequestPath := filepath.Join(proofDir, "human-review-request.md")
	humanReviewRequest := fmt.Sprintf("# Human Review Request\n\n- Work item: `%s` %s\n- Repository: `%s`\n- Pull request: %s\n- Changed files:\n%s\n\nThe review agents approved the PR. A human must approve this checkpoint before Omega performs delivery/merge.\n", text(item, "key"), text(item, "title"), repoSlug, prURL, markdownFileList(changedFiles))
	if err := os.WriteFile(humanReviewRequestPath, []byte(humanReviewRequest), 0o644); err != nil {
		return nil, err
	}
	recordAgent("human_review", "human", "waiting-human", "Review the PR, proof, and agent verdicts. Approve to continue delivery or request changes to send the run back.", "human-review-request.md", "Waiting for explicit human approval before delivery.", []string{humanReviewRequestPath}, map[string]any{"runner": "human", "status": "waiting-human"})

	merged := false
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
		"changedFiles":       changedFiles,
		"artifacts":          stageArtifacts,
		"agentInvocations":   agentInvocations,
		"createdAt":          nowISO(),
	}); err != nil {
		return nil, err
	}
	recordAgent("done", "delivery", "waiting-human", "Assemble delivery handoff bundle after human approval.", "handoff-bundle.json", "Delivery is blocked by the human review checkpoint.", []string{filepath.Join(proofDir, "handoff-bundle.json")}, map[string]any{"runner": "local-orchestrator", "status": "waiting-human"})
	proofFiles, _ := collectFiles(proofDir)
	return map[string]any{
		"status":           "waiting-human",
		"workspacePath":    workspace,
		"repositoryPath":   repoWorkspace,
		"branchName":       branchName,
		"pullRequestUrl":   prURL,
		"merged":           merged,
		"changedFiles":     changedFiles,
		"stageArtifacts":   stageArtifacts,
		"agentInvocations": agentInvocations,
		"proofFiles":       proofFiles,
	}, nil
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
	stageItem["stageId"] = stage["id"]
	stageItem["assignee"] = stage["ownerAgentId"]
	mission := makeMission(stageItem)
	operationID := fmt.Sprintf("operation_%s", text(stage, "id"))
	attempt := makeAttemptRecord(stageItem, pipeline, "manual", payload.Runner, text(stage, "id"))
	database.Tables.Attempts = appendOrReplace(database.Tables.Attempts, attempt)

	upsertMissionAndOperation(&database, mission, text(pipeline, "id"))
	result, err := server.runLocalProof(mission, operationID, payload.Runner)
	if err != nil {
		database, _ = failAttemptRecord(database, text(attempt, "id"), pipeline, err.Error(), nil)
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
	database, err := mustLoad(server, request.Context())
	if err != nil {
		writeError(response, http.StatusNotFound, err)
		return
	}
	index := findByID(database.Tables.Checkpoints, checkpointID)
	if index < 0 {
		writeJSON(response, http.StatusNotFound, map[string]any{"error": "checkpoint not found"})
		return
	}
	var payload map[string]any
	_ = json.NewDecoder(request.Body).Decode(&payload)
	checkpoint := cloneMap(database.Tables.Checkpoints[index])
	checkpoint["status"] = status
	if status == "approved" {
		reviewer := stringOr(payload["reviewer"], "human")
		checkpoint["decisionNote"] = fmt.Sprintf("approved by %s", reviewer)
		approvePipelineStage(&database, checkpoint)
		if err := server.completeApprovedDevFlowCheckpoint(&database, checkpoint, reviewer); err != nil {
			writeError(response, http.StatusInternalServerError, err)
			return
		}
	} else {
		reason := stringOr(payload["reason"], "changes requested")
		checkpoint["decisionNote"] = reason
		rejectPipelineStage(&database, checkpoint, reason)
	}
	checkpoint["updatedAt"] = nowISO()
	database.Tables.Checkpoints[index] = checkpoint
	touch(&database)
	if err := server.Repo.Save(request.Context(), database); err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, checkpoint)
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
	result, err := server.runLocalProof(mission, payload.OperationID, payload.Runner)
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

func (server *Server) runLocalProof(mission map[string]any, operationID string, runner string) (OperationResult, error) {
	operation := findOperation(mission, operationID)
	if operation == nil {
		return OperationResult{}, fmt.Errorf("unknown operation: %s", operationID)
	}
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
		"agentId":            text(operation, "agentId"),
		"missionId":          text(mission, "id"),
		"operationId":        operationID,
		"workspaceRoot":      server.WorkspaceRoot,
		"workspacePath":      workspace,
		"repositoryTargetId": text(mission, "repositoryTargetId"),
		"repositoryTarget":   text(mission, "repositoryTargetLabel"),
	}); err != nil {
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
	} else if runner == "codex" {
		codexResult, err := server.runCodexChange(mission, operation, workspace, proofDir)
		stdout = codexResult.stdout
		stderr = codexResult.stderr
		branchName = codexResult.branchName
		commitSha = codexResult.commitSha
		changedFiles = codexResult.changedFiles
		if codexResult.runnerProcess != nil {
			runnerProcess = codexResult.runnerProcess
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

func (server *Server) runCodexChange(mission map[string]any, operation map[string]any, workspace string, proofDir string) (demoCodeRunResult, error) {
	targetRepo := strings.TrimSpace(text(mission, "target"))
	if targetRepo == "" || targetRepo == "No target" {
		process, err := runSupervisedCommand(workspace, text(operation, "prompt"), "codex", "--ask-for-approval", "never", "exec", "--model", "gpt-5.4-mini", "--skip-git-repo-check", "--sandbox", "workspace-write", "--output-last-message", filepath.Join(proofDir, "codex-last-message.txt"), "-")
		if err != nil {
			return demoCodeRunResult{stdout: text(process, "stdout"), stderr: text(process, "stderr"), runnerProcess: process}, err
		}
		return demoCodeRunResult{stdout: text(process, "stdout"), stderr: text(process, "stderr"), runnerProcess: process}, nil
	}

	repoWorkspace := filepath.Join(workspace, "repo")
	if output, err := cloneTargetRepository(workspace, targetRepo, repoWorkspace); err != nil {
		return demoCodeRunResult{stdout: output}, fmt.Errorf("clone target repository: %w", err)
	}
	branchName := "omega/" + safeSegment(text(mission, "sourceIssueKey")) + "-" + safeSegment(text(operation, "stageId")) + "-codex"
	if _, err := runCommand(repoWorkspace, "git", "checkout", "-b", branchName); err != nil {
		return demoCodeRunResult{branchName: branchName}, fmt.Errorf("create branch: %w", err)
	}
	_, _ = runCommand(repoWorkspace, "git", "config", "user.email", "omega-codex@example.local")
	_, _ = runCommand(repoWorkspace, "git", "config", "user.name", "Omega Codex Runner")

	prompt := fmt.Sprintf("%s\n\nRepository target: %s\nCreate the requested code change in this repository. Leave generated proof in %s.", text(operation, "prompt"), targetRepo, proofDir)
	process, err := runSupervisedCommand(repoWorkspace, prompt, "codex", "--ask-for-approval", "never", "exec", "--model", "gpt-5.4-mini", "--skip-git-repo-check", "--sandbox", "workspace-write", "--output-last-message", filepath.Join(proofDir, "codex-last-message.txt"), "-")
	stdout := text(process, "stdout")
	stderr := text(process, "stderr")
	if err != nil {
		return demoCodeRunResult{stdout: stdout, stderr: stderr, branchName: branchName, runnerProcess: process}, err
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
	summary := fmt.Sprintf("# Omega Codex Code Change\n\n- Work item: %s\n- Stage: %s\n- Branch: `%s`\n- Commit: `%s`\n- Changed files:\n%s\n```diffstat\n%s```\n", text(mission, "sourceIssueKey"), text(operation, "stageId"), branchName, strings.TrimSpace(commitSha), markdownFileList(changedFiles), stat)
	if err := os.WriteFile(filepath.Join(proofDir, "change-summary.md"), []byte(summary), 0o644); err != nil {
		return demoCodeRunResult{stdout: stdout, stderr: stderr, branchName: branchName, runnerProcess: process}, err
	}
	stdout += fmt.Sprintf("\ncodex repository change committed\nbranch: %s\ncommit: %s\n", branchName, strings.TrimSpace(commitSha))
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

func buildDevFlowReviewPrompt(item map[string]any, repoSlug string, prURL string, changedFiles []string, diffText string, testOutput string, checksOutput string, focus string) string {
	return fmt.Sprintf(`You are the review agent for Omega.

Repository: %s
Pull request: %s
Work item: %s
Title: %s

Requirement:
%s

Acceptance criteria:
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
- Do not approve just because a file changed or tests passed.
- If the diff does not satisfy the requested behavior, request changes.
- Do not edit files.

Write the review as Markdown and include exactly one verdict line:
Verdict: APPROVED
or
Verdict: CHANGES_REQUESTED
or
Verdict: NEEDS_HUMAN_INFO
`, repoSlug, prURL, text(item, "key"), text(item, "title"), text(item, "description"), markdownAnyList(item["acceptanceCriteria"]), markdownFileList(changedFiles), focus, truncateForProof(testOutput, 4000), truncateForProof(checksOutput, 4000), truncateForProof(diffText, 12000))
}

func runDevFlowReviewAgent(repoWorkspace string, prompt string, outputPath string) (map[string]any, error) {
	process, err := runSupervisedCommand(repoWorkspace, prompt, "codex", "--ask-for-approval", "never", "exec", "--model", "gpt-5.4-mini", "-c", "model_reasoning_effort=\"medium\"", "--skip-git-repo-check", "--sandbox", "read-only", "--output-last-message", outputPath, "-")
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
		return DevFlowReviewOutcome{Verdict: "needs_human_info", Summary: "Review needs human input before continuing."}
	case strings.Contains(normalized, "verdict: changes_requested") ||
		strings.Contains(normalized, "changes requested") ||
		strings.Contains(normalized, `"approved": false`) ||
		strings.Contains(normalized, "blocked"):
		return DevFlowReviewOutcome{Verdict: "changes_requested", Summary: "Review requested changes."}
	case strings.Contains(normalized, "verdict: approved") ||
		strings.Contains(normalized, `"approved": true`):
		return DevFlowReviewOutcome{Verdict: "approved", Summary: "Review approved the diff against the requirement."}
	default:
		return DevFlowReviewOutcome{Verdict: "missing", Summary: "Review did not include an explicit approved verdict."}
	}
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
	started := time.Now()
	command := exec.Command(name, args...)
	command.Dir = dir
	if stdin != "" {
		command.Stdin = strings.NewReader(stdin)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
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
	err := command.Wait()
	finished := time.Now()
	process["stdout"] = stdout.String()
	process["stderr"] = stderr.String()
	process["finishedAt"] = finished.UTC().Format(time.RFC3339Nano)
	process["durationMs"] = float64(finished.Sub(started).Milliseconds())
	exitCode := 0
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

func appendWorkItem(database WorkspaceDatabase, item map[string]any) WorkspaceDatabase {
	timestamp := nowISO()
	normalized := normalizeWorkItem(item)
	normalized["id"] = uniqueWorkItemID(database, text(normalized, "id"))
	database, normalized = ensureRequirementForWorkItem(database, normalized, timestamp)
	record := cloneMap(normalized)
	record["projectId"] = firstProjectID(database)
	record["createdAt"] = timestamp
	record["updatedAt"] = timestamp
	database.Tables.WorkItems = append(database.Tables.WorkItems, record)
	for index, state := range database.Tables.MissionControlStates {
		nextState := cloneMap(state)
		nextState["workItems"] = append(arrayMaps(nextState["workItems"]), normalized)
		nextState["updatedAt"] = timestamp
		database.Tables.MissionControlStates[index] = nextState
	}
	touch(&database)
	return database
}

func normalizeRequirementLinks(database WorkspaceDatabase) WorkspaceDatabase {
	timestamp := nowISO()
	for index, item := range database.Tables.WorkItems {
		database, item = ensureRequirementForWorkItem(database, item, timestamp)
		database.Tables.WorkItems[index] = item
	}
	for stateIndex, state := range database.Tables.MissionControlStates {
		nextState := cloneMap(state)
		items := arrayMaps(nextState["workItems"])
		for itemIndex, item := range items {
			database, item = ensureRequirementForWorkItem(database, item, timestamp)
			items[itemIndex] = item
		}
		nextState["workItems"] = items
		database.Tables.MissionControlStates[stateIndex] = nextState
	}
	return database
}

func ensureRequirementForWorkItem(database WorkspaceDatabase, item map[string]any, timestamp string) (WorkspaceDatabase, map[string]any) {
	nextItem := normalizeWorkItem(item)
	if text(nextItem, "requirementId") != "" {
		requirementIndex := findByID(database.Tables.Requirements, text(nextItem, "requirementId"))
		if requirementIndex >= 0 {
			generated := requirementFromWorkItem(database, nextItem, timestamp)
			database.Tables.Requirements[requirementIndex] = enrichRequirementRecord(database.Tables.Requirements[requirementIndex], generated, timestamp)
			return database, nextItem
		}
	}

	requirement := requirementFromWorkItem(database, nextItem, timestamp)
	if existingID := findRequirementID(database, requirement); existingID != "" {
		nextItem["requirementId"] = existingID
		if requirementIndex := findByID(database.Tables.Requirements, existingID); requirementIndex >= 0 {
			database.Tables.Requirements[requirementIndex] = enrichRequirementRecord(database.Tables.Requirements[requirementIndex], requirement, timestamp)
		}
		return database, nextItem
	}
	nextItem["requirementId"] = text(requirement, "id")
	database.Tables.Requirements = append(database.Tables.Requirements, requirement)
	return database, nextItem
}

func enrichRequirementRecord(existing map[string]any, generated map[string]any, timestamp string) map[string]any {
	next := cloneMap(existing)
	changed := false
	for _, key := range []string{"projectId", "repositoryTargetId", "source", "sourceExternalRef", "rawText"} {
		if text(next, key) == "" && text(generated, key) != "" {
			next[key] = generated[key]
			changed = true
		}
	}
	if len(anySlice(next["acceptanceCriteria"])) == 0 && len(anySlice(generated["acceptanceCriteria"])) > 0 {
		next["acceptanceCriteria"] = generated["acceptanceCriteria"]
		changed = true
	}
	if len(anySlice(next["risks"])) == 0 && len(anySlice(generated["risks"])) > 0 {
		next["risks"] = generated["risks"]
		changed = true
	}
	structured := mapValue(next["structured"])
	generatedStructured := mapValue(generated["structured"])
	for _, key := range []string{"summary", "sourceWorkItemKey", "repositoryTargetId", "sourceExternalRef", "initialExecutorHint", "masterAgentId", "dispatchStatus", "dispatchPlan", "suggestedWorkItems", "assumptions"} {
		if isEmptyStructuredValue(structured[key]) && !isEmptyStructuredValue(generatedStructured[key]) {
			structured[key] = generatedStructured[key]
			changed = true
		}
	}
	if text(structured, "masterAgentId") == "" {
		structured["masterAgentId"] = "master"
		changed = true
	}
	if text(structured, "dispatchStatus") == "" {
		structured["dispatchStatus"] = "ready"
		changed = true
	}
	next["structured"] = structured
	if changed {
		next["updatedAt"] = timestamp
	}
	return next
}

func isEmptyStructuredValue(value any) bool {
	if value == nil {
		return true
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) == ""
	case []any:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	default:
		return false
	}
}

func requirementFromWorkItem(database WorkspaceDatabase, item map[string]any, timestamp string) map[string]any {
	source := stringOr(text(item, "source"), "manual")
	externalRef := text(item, "sourceExternalRef")
	repositoryTargetID := text(item, "repositoryTargetId")
	idSource := text(item, "id")
	if externalRef != "" {
		idSource = externalRef
	}
	if idSource == "" {
		idSource = text(item, "title")
	}
	requirementID := text(item, "requirementId")
	if requirementID == "" {
		requirementID = fmt.Sprintf("req_%s", safeSegment(idSource))
	}
	rawText := text(item, "description")
	if rawText == "" {
		rawText = text(item, "title")
	}
	decomposition := decomposeRequirementPayload(
		stringOr(text(item, "title"), "Untitled requirement"),
		rawText,
		text(item, "target"),
		source,
	)
	criteria := anySlice(decomposition["acceptanceCriteria"])
	if len(criteria) == 0 {
		criteria = anySlice(item["acceptanceCriteria"])
	}
	risks := anySlice(decomposition["risks"])
	structured := map[string]any{
		"summary":             stringOr(text(item, "title"), "Untitled requirement"),
		"sourceWorkItemKey":   text(item, "key"),
		"repositoryTargetId":  repositoryTargetID,
		"sourceExternalRef":   externalRef,
		"initialExecutorHint": text(item, "assignee"),
		"masterAgentId":       "master",
		"dispatchStatus":      "ready",
		"dispatchPlan": map[string]any{
			"templateId":          "feature",
			"repositoryTargetId":  repositoryTargetID,
			"repositoryTarget":    text(item, "target"),
			"stageOrder":          decomposition["pipelineStages"],
			"assignedBy":          "master",
			"requiresHumanReview": true,
		},
		"suggestedWorkItems": decomposition["suggestedWorkItems"],
		"assumptions":        decomposition["assumptions"],
	}
	return map[string]any{
		"id":                 requirementID,
		"projectId":          firstProjectID(database),
		"repositoryTargetId": repositoryTargetID,
		"source":             source,
		"sourceExternalRef":  externalRef,
		"title":              stringOr(text(item, "title"), "Untitled requirement"),
		"rawText":            rawText,
		"structured":         structured,
		"acceptanceCriteria": criteria,
		"risks":              risks,
		"status":             "converted",
		"createdAt":          timestamp,
		"updatedAt":          timestamp,
	}
}

func findRequirementID(database WorkspaceDatabase, requirement map[string]any) string {
	if ref := text(requirement, "sourceExternalRef"); ref != "" {
		for _, candidate := range database.Tables.Requirements {
			if text(candidate, "sourceExternalRef") == ref && text(candidate, "source") == text(requirement, "source") {
				return text(candidate, "id")
			}
		}
	}
	id := text(requirement, "id")
	if findByID(database.Tables.Requirements, id) < 0 {
		return ""
	}
	for suffix := 2; ; suffix++ {
		next := fmt.Sprintf("%s_%d", id, suffix)
		if findByID(database.Tables.Requirements, next) < 0 {
			requirement["id"] = next
			return ""
		}
	}
}

func normalizeWorkItem(item map[string]any) map[string]any {
	next := cloneMap(item)
	if text(next, "source") == "" {
		next["source"] = "manual"
	}
	if _, ok := next["acceptanceCriteria"]; !ok || arrayLength(next["acceptanceCriteria"]) == 0 {
		next["acceptanceCriteria"] = []any{"Request is described clearly", "Human can verify the result"}
	}
	if _, ok := next["blockedByItemIds"]; !ok {
		next["blockedByItemIds"] = []any{}
	}
	if text(next, "team") == "" {
		next["team"] = "Omega"
	}
	if text(next, "stageId") == "" {
		next["stageId"] = "intake"
	}
	if text(next, "target") == "" {
		next["target"] = "No target"
	}
	return next
}

func uniqueWorkItemID(database WorkspaceDatabase, candidate string) string {
	base := strings.TrimSpace(candidate)
	if base == "" {
		base = fmt.Sprintf("item_manual_%d", len(database.Tables.WorkItems)+1)
	}
	used := map[string]bool{}
	for _, item := range database.Tables.WorkItems {
		if id := text(item, "id"); id != "" {
			used[id] = true
		}
	}
	for _, state := range database.Tables.MissionControlStates {
		for _, item := range arrayMaps(state["workItems"]) {
			if id := text(item, "id"); id != "" {
				used[id] = true
			}
		}
	}
	if !used[base] {
		return base
	}
	for suffix := 2; ; suffix++ {
		next := fmt.Sprintf("%s_%d", base, suffix)
		if !used[next] {
			return next
		}
	}
}

func updateWorkItem(database WorkspaceDatabase, itemID string, patch map[string]any) WorkspaceDatabase {
	timestamp := nowISO()
	for index, item := range database.Tables.WorkItems {
		if text(item, "id") == itemID {
			next := cloneMap(item)
			for key, value := range patch {
				next[key] = value
			}
			next["updatedAt"] = timestamp
			database.Tables.WorkItems[index] = next
		}
	}
	for stateIndex, state := range database.Tables.MissionControlStates {
		nextState := cloneMap(state)
		items := arrayMaps(nextState["workItems"])
		for itemIndex, item := range items {
			if text(item, "id") == itemID {
				for key, value := range patch {
					item[key] = value
				}
				items[itemIndex] = item
			}
		}
		nextState["workItems"] = items
		nextState["updatedAt"] = timestamp
		database.Tables.MissionControlStates[stateIndex] = nextState
	}
	touch(&database)
	return database
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

func makePipeline(item map[string]any) map[string]any {
	template := findPipelineTemplate("feature")
	if template == nil {
		template = &PipelineTemplate{ID: "feature", Name: "Feature delivery", Description: "Default feature delivery flow.", StageProfiles: defaultStageProfiles()}
	}
	return makePipelineWithTemplate(item, template)
}

func makePipelineWithTemplate(item map[string]any, template *PipelineTemplate) map[string]any {
	createdAt := nowISO()
	runID := fmt.Sprintf("run_%s", text(item, "id"))
	stages := stagesFromTemplate(template)
	agents := agentContractsForStages(stages)
	return map[string]any{
		"id":         fmt.Sprintf("pipeline_%s", text(item, "id")),
		"workItemId": text(item, "id"),
		"runId":      runID,
		"status":     "draft",
		"templateId": template.ID,
		"run": map[string]any{
			"id": runID,
			"requirement": map[string]any{
				"id":          stringOr(text(item, "requirementId"), fmt.Sprintf("req_%s", text(item, "id"))),
				"identifier":  text(item, "key"),
				"title":       text(item, "title"),
				"description": text(item, "description"),
				"source":      stringOr(text(item, "source"), "manual"),
				"priority":    "high",
				"requester":   text(item, "assignee"),
				"labels":      item["labels"],
				"createdAt":   createdAt,
			},
			"goal":            fmt.Sprintf("Deliver %s: %s", text(item, "key"), text(item, "title")),
			"successCriteria": []any{"All pipeline stages are passed", "All human gates are approved", "Testing and review evidence is attached", "Delivery notes and rollback plan are attached"},
			"stages":          stages,
			"agents":          agents,
			"orchestrator": map[string]any{
				"masterAgentId":      "master",
				"dispatchStatus":     "ready",
				"templateId":         template.ID,
				"repositoryTargetId": text(item, "repositoryTargetId"),
			},
			"workflow": map[string]any{
				"id":           template.ID,
				"name":         template.Name,
				"source":       template.Source,
				"reviewRounds": template.ReviewRounds,
			},
			"dataFlow":             dataFlowForStages(stages),
			"selectedCapabilities": map[string]any{"llmProvider": defaultProviderSelection().ProviderID, "model": defaultProviderSelection().Model},
			"events": []map[string]any{
				{"id": fmt.Sprintf("event_%s_1", runID), "type": "run.created", "message": fmt.Sprintf("Pipeline created for %s", text(item, "key")), "timestamp": createdAt, "stageId": "intake", "agentId": "master"},
				{"id": fmt.Sprintf("event_%s_2", runID), "type": "master.dispatch.created", "message": fmt.Sprintf("Master agent dispatched %d stage agent contract(s)", len(agents)), "timestamp": createdAt, "stageId": "orchestration", "agentId": "master"},
			},
			"createdAt": createdAt,
			"updatedAt": createdAt,
		},
		"createdAt": createdAt,
		"updatedAt": createdAt,
	}
}

func normalizePipelineExecutionMetadata(database WorkspaceDatabase) WorkspaceDatabase {
	timestamp := nowISO()
	for index, pipeline := range database.Tables.Pipelines {
		template := findPipelineTemplate(text(pipeline, "templateId"))
		if template == nil {
			continue
		}
		item := findWorkItem(database, text(pipeline, "workItemId"))
		if item == nil {
			item = map[string]any{"id": text(pipeline, "workItemId"), "key": text(pipeline, "workItemId")}
		}
		normalized := makePipelineWithTemplate(item, template)
		next := cloneMap(pipeline)
		run := mapValue(next["run"])
		normalizedRun := mapValue(normalized["run"])
		run["stages"] = mergeStageRuntimeState(arrayMaps(run["stages"]), arrayMaps(normalizedRun["stages"]))
		if len(arrayMaps(run["agents"])) == 0 {
			run["agents"] = normalizedRun["agents"]
		}
		if len(arrayMaps(run["dataFlow"])) == 0 {
			run["dataFlow"] = normalizedRun["dataFlow"]
		}
		if len(mapValue(run["orchestrator"])) == 0 {
			run["orchestrator"] = normalizedRun["orchestrator"]
		}
		if len(mapValue(run["selectedCapabilities"])) == 0 {
			run["selectedCapabilities"] = normalizedRun["selectedCapabilities"]
		}
		if len(arrayMaps(run["events"])) == 0 {
			run["events"] = normalizedRun["events"]
		}
		if len(mapValue(run["requirement"])) == 0 {
			run["requirement"] = normalizedRun["requirement"]
		}
		next["run"] = run
		next["updatedAt"] = stringOr(text(next, "updatedAt"), timestamp)
		database.Tables.Pipelines[index] = next
	}
	return database
}

func mergeStageRuntimeState(existing []map[string]any, normalized []map[string]any) []map[string]any {
	existingByID := map[string]map[string]any{}
	for _, stage := range existing {
		existingByID[text(stage, "id")] = stage
	}
	result := make([]map[string]any, 0, len(normalized))
	for _, base := range normalized {
		next := cloneMap(base)
		if prior := existingByID[text(base, "id")]; prior != nil {
			for _, key := range []string{"status", "startedAt", "completedAt", "notes", "evidence", "acceptanceCriteria", "approvedBy", "rejectionReason"} {
				if prior[key] != nil {
					next[key] = prior[key]
				}
			}
		}
		result = append(result, next)
	}
	return result
}

func defaultStages() []map[string]any {
	return stagesFromTemplate(&PipelineTemplate{StageProfiles: defaultStageProfiles()})
}

func stage(id, title, agent string, humanGate bool, status string) map[string]any {
	return map[string]any{"id": id, "name": title, "title": title, "description": title, "agentId": agent, "ownerAgentId": agent, "status": status, "humanGate": humanGate, "dependsOn": []any{}, "inputArtifacts": []any{}, "outputArtifacts": stageOutputArtifacts(id), "acceptanceCriteria": []any{"Criteria is satisfied"}, "evidence": []any{}}
}

type StageProfile struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Agent           string   `json:"agentId"`
	AgentIDs        []string `json:"agents,omitempty"`
	HumanGate       bool     `json:"humanGate"`
	InputArtifacts  []string `json:"inputArtifacts,omitempty"`
	OutputArtifacts []string `json:"outputArtifacts,omitempty"`
}

type PipelineTemplate struct {
	ID               string                      `json:"id"`
	Name             string                      `json:"name"`
	Description      string                      `json:"description"`
	Source           string                      `json:"source,omitempty"`
	PromptTemplate   string                      `json:"promptTemplate,omitempty"`
	WorkflowMarkdown string                      `json:"workflowMarkdown,omitempty"`
	StageProfiles    []StageProfile              `json:"stages"`
	ReviewRounds     []ReviewRoundProfile        `json:"reviewRounds,omitempty"`
	Runtime          WorkflowRuntimeProfile      `json:"runtime,omitempty"`
	Transitions      []WorkflowTransitionProfile `json:"transitions,omitempty"`
}

func isDevFlowPRTemplate(templateID string) bool {
	return templateID == "devflow-pr"
}

func pipelineTemplates() []PipelineTemplate {
	workflowTemplates := workflowPipelineTemplates()
	devflowTemplate, hasWorkflowDevFlow := firstTemplateByID(workflowTemplates, "devflow-pr")
	templates := []PipelineTemplate{
		{
			ID:            "feature",
			Name:          "Feature delivery",
			Description:   "Full requirement to delivery flow for new product capabilities.",
			StageProfiles: defaultStageProfiles(),
		},
	}
	if hasWorkflowDevFlow {
		templates = append(templates, devflowTemplate)
	} else {
		templates = append(templates, PipelineTemplate{
			ID:          "devflow-pr",
			Name:        "DevFlow PR cycle",
			Description: "Local-first Omega flow: intake, implementation, two code review rounds, human review, merge, done.",
			StageProfiles: []StageProfile{
				{ID: "todo", Title: "Todo intake", Agent: "requirement", HumanGate: false},
				{ID: "in_progress", Title: "Implementation and PR", Agent: "coding", HumanGate: false, AgentIDs: []string{"architect", "coding", "testing"}},
				{ID: "code_review_round_1", Title: "Code Review Round 1", Agent: "review", HumanGate: false},
				{ID: "code_review_round_2", Title: "Code Review Round 2", Agent: "review", HumanGate: false},
				{ID: "rework", Title: "Rework", Agent: "coding", HumanGate: false, AgentIDs: []string{"coding", "testing"}},
				{ID: "human_review", Title: "Human Review", Agent: "delivery", HumanGate: true, AgentIDs: []string{"human", "review", "delivery"}},
				{ID: "merging", Title: "Merging", Agent: "delivery", HumanGate: false},
				{ID: "done", Title: "Done", Agent: "delivery", HumanGate: false},
			},
			ReviewRounds: defaultDevFlowReviewRounds(),
			Runtime:      WorkflowRuntimeProfile{MaxReviewCycles: 3},
		})
	}
	templates = append(templates,
		PipelineTemplate{
			ID:          "bugfix",
			Name:        "Bug fix",
			Description: "Tighter flow focused on reproduction, patching, regression tests, and review.",
			StageProfiles: []StageProfile{
				{ID: "intake", Title: "Reproduce", Agent: "requirement", HumanGate: true},
				{ID: "coding", Title: "Patch", Agent: "coding", HumanGate: false},
				{ID: "testing", Title: "Regression", Agent: "testing", HumanGate: true},
				{ID: "review", Title: "Review", Agent: "review", HumanGate: true},
				{ID: "delivery", Title: "Delivery", Agent: "delivery", HumanGate: true},
			},
		},
		PipelineTemplate{
			ID:          "refactor",
			Name:        "Refactor",
			Description: "Architecture-sensitive flow with explicit solution and review gates.",
			StageProfiles: []StageProfile{
				{ID: "intake", Title: "Scope", Agent: "requirement", HumanGate: true},
				{ID: "solution", Title: "Design", Agent: "architect", HumanGate: true},
				{ID: "coding", Title: "Refactor", Agent: "coding", HumanGate: false},
				{ID: "testing", Title: "Safety checks", Agent: "testing", HumanGate: true},
				{ID: "review", Title: "Architecture review", Agent: "review", HumanGate: true},
				{ID: "delivery", Title: "Delivery", Agent: "delivery", HumanGate: true},
			},
		},
	)
	return templates
}

func firstTemplateByID(templates []PipelineTemplate, id string) (PipelineTemplate, bool) {
	for _, template := range templates {
		if template.ID == id {
			return template, true
		}
	}
	return PipelineTemplate{}, false
}

func removeDuplicateTemplateIDs(templates []PipelineTemplate) []PipelineTemplate {
	seen := map[string]bool{}
	output := make([]PipelineTemplate, 0, len(templates))
	for _, template := range templates {
		if template.ID == "" || seen[template.ID] {
			continue
		}
		seen[template.ID] = true
		output = append(output, template)
	}
	return output
}

func defaultDevFlowReviewRounds() []ReviewRoundProfile {
	return []ReviewRoundProfile{
		{StageID: "code_review_round_1", Artifact: "code-review-round-1.md", Focus: "correctness, regressions, and acceptance criteria", DiffSource: "local_diff", ChangesRequestedTo: "rework", NeedsHumanInfoTo: "human_review"},
		{StageID: "code_review_round_2", Artifact: "code-review-round-2.md", Focus: "maintainability, tests, edge cases, and delivery readiness", DiffSource: "pr_diff", ChangesRequestedTo: "rework", NeedsHumanInfoTo: "human_review"},
	}
}

func defaultStageProfiles() []StageProfile {
	return []StageProfile{
		{ID: "intake", Title: "Intake", Agent: "requirement", HumanGate: true},
		{ID: "solution", Title: "Solution", Agent: "architect", HumanGate: true},
		{ID: "coding", Title: "Implementation", Agent: "coding", HumanGate: false},
		{ID: "testing", Title: "Testing", Agent: "testing", HumanGate: true},
		{ID: "review", Title: "Review", Agent: "review", HumanGate: true},
		{ID: "delivery", Title: "Delivery", Agent: "delivery", HumanGate: true},
	}
}

func findPipelineTemplate(templateID string) *PipelineTemplate {
	if templateID == "" {
		templateID = "feature"
	}
	for _, template := range pipelineTemplates() {
		if template.ID == templateID {
			return &template
		}
	}
	return nil
}

func stagesFromTemplate(template *PipelineTemplate) []map[string]any {
	stages := make([]map[string]any, 0, len(template.StageProfiles))
	for index, profile := range template.StageProfiles {
		status := "waiting"
		if index == 0 {
			status = "ready"
		}
		nextStage := stage(profile.ID, profile.Title, profile.Agent, profile.HumanGate, status)
		nextStage["agentIds"] = stageAgentIDs(profile)
		if len(profile.OutputArtifacts) > 0 {
			nextStage["outputArtifacts"] = anyListFromStrings(profile.OutputArtifacts)
		}
		if len(profile.InputArtifacts) > 0 {
			nextStage["inputArtifacts"] = anyListFromStrings(profile.InputArtifacts)
		}
		if index > 0 {
			previous := stages[index-1]
			nextStage["dependsOn"] = []any{text(previous, "id")}
			if len(profile.InputArtifacts) == 0 {
				nextStage["inputArtifacts"] = previous["outputArtifacts"]
			}
		} else {
			if len(profile.InputArtifacts) == 0 {
				nextStage["inputArtifacts"] = []any{"raw-requirement", "repository-target"}
			}
		}
		stages = append(stages, nextStage)
	}
	return stages
}

func stageAgentIDs(profile StageProfile) []any {
	if len(profile.AgentIDs) > 0 {
		return anyListFromStrings(profile.AgentIDs)
	}
	switch profile.ID {
	case "in_progress":
		return []any{"architect", "coding", "testing"}
	case "human_review":
		return []any{"review", "delivery"}
	case "merging", "done":
		return []any{"delivery"}
	default:
		return []any{profile.Agent}
	}
}

func agentContractsForStages(stages []map[string]any) []map[string]any {
	selection := defaultProviderSelection()
	definitions := agentDefinitions(selection)
	contractsByID := map[string]map[string]any{}
	for _, definition := range definitions {
		contractsByID[definition.ID] = map[string]any{
			"id":             definition.ID,
			"name":           definition.Name,
			"stageId":        definition.StageID,
			"systemPrompt":   definition.SystemPrompt,
			"inputContract":  definition.InputContract,
			"outputContract": definition.OutputContract,
			"defaultTools":   definition.DefaultTools,
			"defaultModel":   definition.DefaultModel,
		}
	}
	contracts := []map[string]any{contractsByID["master"]}
	seen := map[string]bool{"master": true}
	for _, stage := range stages {
		agentIDs := anySlice(stage["agentIds"])
		if len(agentIDs) == 0 {
			agentIDs = []any{text(stage, "ownerAgentId")}
		}
		for _, value := range agentIDs {
			agentID := fmt.Sprint(value)
			if seen[agentID] {
				continue
			}
			if contract := contractsByID[agentID]; contract != nil {
				contracts = append(contracts, contract)
				seen[agentID] = true
			}
		}
	}
	return contracts
}

func dataFlowForStages(stages []map[string]any) []map[string]any {
	flows := []map[string]any{}
	for index := 1; index < len(stages); index++ {
		from := stages[index-1]
		to := stages[index]
		flows = append(flows, map[string]any{
			"fromStageId": text(from, "id"),
			"toStageId":   text(to, "id"),
			"artifacts":   from["outputArtifacts"],
		})
	}
	return flows
}

func stageOutputArtifacts(stageID string) []any {
	switch stageID {
	case "intake", "todo":
		return []any{"structured-requirement", "acceptance-criteria", "dispatch-plan"}
	case "solution":
		return []any{"technical-plan", "file-change-list", "test-strategy"}
	case "coding", "in_progress":
		return []any{"code-diff", "changed-files", "implementation-notes"}
	case "testing":
		return []any{"test-report", "coverage-risk-notes"}
	case "review", "code_review_round_1", "code_review_round_2":
		return []any{"review-report", "blocking-risks", "merge-recommendation"}
	case "human_review":
		return []any{"human-decision", "review-notes"}
	case "merging", "delivery":
		return []any{"pull-request", "delivery-summary", "rollback-plan"}
	case "done":
		return []any{"handoff-bundle", "proof-records"}
	default:
		return []any{stageID + "-artifact"}
	}
}

func makeMission(item map[string]any) map[string]any {
	stageID := text(item, "stageId")
	if stageID == "" {
		stageID = "intake"
	}
	agent := text(item, "assignee")
	if agent == "" {
		agent = "requirement"
	}
	status := "ready"
	if text(item, "status") == "Done" {
		status = "done"
	}
	return map[string]any{
		"id":                    fmt.Sprintf("mission_%s_%s", text(item, "key"), stageID),
		"sourceIssueKey":        text(item, "key"),
		"sourceWorkItemId":      text(item, "id"),
		"title":                 text(item, "title"),
		"target":                text(item, "target"),
		"repositoryTargetId":    text(item, "repositoryTargetId"),
		"repositoryTargetLabel": text(item, "repositoryTargetLabel"),
		"status":                status,
		"checkpointRequired":    stageID != "coding",
		"operations": []map[string]any{{
			"id":            fmt.Sprintf("operation_%s", stageID),
			"stageId":       stageID,
			"agentId":       agent,
			"status":        status,
			"prompt":        fmt.Sprintf("Mission: %s\nSource work item: %s\nStage: %s\nAgent: %s\nRepository target ID: %s\nRepository target: %s\nRepository label: %s", text(item, "title"), text(item, "key"), stageID, agent, stringOr(text(item, "repositoryTargetId"), "unscoped"), text(item, "target"), text(item, "repositoryTargetLabel")),
			"requiredProof": []any{"proof"},
		}},
		"links": []any{},
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
	attemptIndex := latestAttemptIndexForPipeline(*database, text(pipeline, "id"))
	if attemptIndex < 0 {
		return fmt.Errorf("devflow approval cannot continue: attempt not found")
	}
	attempt := cloneMap(database.Tables.Attempts[attemptIndex])
	workspace := text(attempt, "workspacePath")
	prURL := text(attempt, "pullRequestUrl")
	if workspace == "" || prURL == "" {
		return fmt.Errorf("devflow approval cannot continue: workspace or pull request is missing")
	}
	repoWorkspace := filepath.Join(workspace, "repo")
	proofDir := filepath.Join(workspace, ".omega", "proof")
	if err := os.MkdirAll(proofDir, 0o755); err != nil {
		return err
	}

	humanReviewPath := filepath.Join(proofDir, "human-review.md")
	if err := os.WriteFile(humanReviewPath, []byte(fmt.Sprintf("# Human Review\n\n- Reviewer: %s\n- Decision: approved\n- Pull request: %s\n- Approved at: %s\n", reviewer, prURL, nowISO())), 0o644); err != nil {
		return err
	}
	if _, err := runCommand(repoWorkspace, "gh", "pr", "merge", prURL, "--squash", "--delete-branch", "--subject", "Omega DevFlow cycle approved"); err != nil {
		return fmt.Errorf("merge pull request after human approval: %w", err)
	}
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
	if item := findWorkItem(*database, text(pipeline, "workItemId")); item != nil {
		*database = updateWorkItem(*database, text(item, "id"), map[string]any{"status": "Done"})
	}

	proofFiles, _ := collectFiles(proofDir)
	result := map[string]any{
		"status":         "done",
		"workspacePath":  workspace,
		"branchName":     text(attempt, "branchName"),
		"pullRequestUrl": prURL,
		"proofFiles":     proofFiles,
	}
	nextDatabase, _ := completeAttemptRecord(*database, text(attempt, "id"), pipeline, result)
	*database = nextDatabase
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
