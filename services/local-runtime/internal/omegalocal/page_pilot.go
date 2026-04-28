package omegalocal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type pagePilotSourceMapping struct {
	Source string `json:"source"`
	File   string `json:"file"`
	Symbol string `json:"symbol"`
	Line   int    `json:"line"`
}

type pagePilotSelectionContext struct {
	ElementKind    string                 `json:"elementKind"`
	StableSelector string                 `json:"stableSelector"`
	TextSnapshot   string                 `json:"textSnapshot"`
	StyleSnapshot  map[string]string      `json:"styleSnapshot"`
	DOMContext     map[string]any         `json:"domContext"`
	SourceMapping  pagePilotSourceMapping `json:"sourceMapping"`
}

type pagePilotApplyRequest struct {
	ProjectID          string                    `json:"projectId"`
	RepositoryTargetID string                    `json:"repositoryTargetId"`
	Instruction        string                    `json:"instruction"`
	Selection          pagePilotSelectionContext `json:"selection"`
	Runner             string                    `json:"runner"`
}

type pagePilotDeliverRequest struct {
	RunID              string                    `json:"runId"`
	ProjectID          string                    `json:"projectId"`
	RepositoryTargetID string                    `json:"repositoryTargetId"`
	Instruction        string                    `json:"instruction"`
	Selection          pagePilotSelectionContext `json:"selection"`
	BranchName         string                    `json:"branchName"`
	Draft              bool                      `json:"draft"`
}

const pagePilotRunSettingPrefix = "page-pilot-run:"

func (server *Server) pagePilotApply(response http.ResponseWriter, request *http.Request) {
	var payload pagePilotApplyRequest
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	result, err := server.executePagePilotApply(request.Context(), payload)
	if err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	writeJSON(response, http.StatusOK, result)
}

func (server *Server) listPagePilotRuns(response http.ResponseWriter, request *http.Request) {
	runs, err := server.Repo.ListPagePilotRuns(request.Context())
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	if len(runs) == 0 {
		if legacyRuns, legacyErr := server.Repo.ListSettings(request.Context(), pagePilotRunSettingPrefix); legacyErr == nil {
			runs = legacyRuns
		}
	}
	writeJSON(response, http.StatusOK, runs)
}

func (server *Server) discardPagePilotRun(response http.ResponseWriter, request *http.Request) {
	runID := strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, "/page-pilot/runs/"), "/discard")
	if strings.TrimSpace(runID) == "" {
		writeError(response, http.StatusBadRequest, errors.New("Page Pilot run id is required"))
		return
	}
	result, err := server.executePagePilotDiscard(request.Context(), runID)
	if err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	writeJSON(response, http.StatusOK, result)
}

func (server *Server) pagePilotDeliver(response http.ResponseWriter, request *http.Request) {
	var payload pagePilotDeliverRequest
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	result, err := server.executePagePilotDeliver(request.Context(), payload)
	if err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	writeJSON(response, http.StatusOK, result)
}

func (server *Server) executePagePilotApply(ctx context.Context, payload pagePilotApplyRequest) (map[string]any, error) {
	if strings.TrimSpace(payload.RepositoryTargetID) == "" {
		return nil, errors.New("repositoryTargetId is required")
	}
	if strings.TrimSpace(payload.Instruction) == "" {
		return nil, errors.New("instruction is required")
	}
	repoPath, target, profile, err := server.resolvePagePilotWorkspace(ctx, payload.ProjectID, payload.RepositoryTargetID, payload.Selection.SourceMapping.File)
	if err != nil {
		return nil, err
	}
	runnerID := strings.TrimSpace(payload.Runner)
	if runnerID == "" {
		runnerID = "profile"
	}
	effectiveRunner, err := preflightAgentRunner(runnerID, profile, "coding")
	if err != nil {
		return nil, err
	}
	proofDir := filepath.Join(repoPath, ".omega", "page-pilot")
	if err := os.MkdirAll(proofDir, 0o755); err != nil {
		return nil, err
	}
	promptPath := filepath.Join(proofDir, "page-pilot-prompt.md")
	notePath := filepath.Join(proofDir, "page-pilot-agent-note.md")
	prompt := buildPagePilotPrompt(repoPath, repositoryTargetLabel(target), payload)
	if err := os.WriteFile(promptPath, []byte(prompt), 0o644); err != nil {
		return nil, err
	}
	process := map[string]any{"runner": effectiveRunner, "status": "passed"}
	if effectiveRunner == "local-proof" || effectiveRunner == "demo-code" {
		process, err = applyPagePilotLocalPatch(repoPath, payload)
	} else {
		runner, resolved := NewAgentRunnerRegistry().Resolve(effectiveRunner)
		profileForRole := agentProfileForRole(profile, "coding")
		turn := runner.RunTurn(ctx, AgentTurnRequest{
			Role:       "page-pilot",
			StageID:    "page_pilot",
			Runner:     resolved,
			Workspace:  repoPath,
			Prompt:     prompt,
			OutputPath: notePath,
			Sandbox:    "workspace-write",
			Model:      profileForRole.Model,
		})
		process = turn.Process
		if turn.Error != nil {
			return map[string]any{"status": "failed", "repositoryPath": repoPath, "runnerProcess": process}, fmt.Errorf("page pilot runner failed: %w", turn.Error)
		}
	}
	statusOutput, err := runCommand(repoPath, "git", "status", "--short")
	if err != nil {
		return nil, fmt.Errorf("read repository diff: %w", err)
	}
	sourceStatusFiles := pagePilotSourceStatusFiles(statusOutput)
	if len(sourceStatusFiles) == 0 {
		return nil, errors.New("page pilot runner produced no repository changes")
	}
	diffText, _ := runCommand(repoPath, "git", "diff", "--", sourceFileOrDot(payload.Selection.SourceMapping.File))
	changedNames, _ := runCommand(repoPath, "git", "diff", "--name-only")
	changedFiles := mergeUniqueStrings(compactLines(changedNames), sourceStatusFiles)
	if len(changedFiles) == 0 && payload.Selection.SourceMapping.File != "" {
		changedFiles = []string{payload.Selection.SourceMapping.File}
	}
	diffSummary := buildPagePilotDiffSummary(diffText, changedFiles)
	lineSummary := buildLineDiffSummary(diffText)
	diffPath := filepath.Join(proofDir, "page-pilot.diff")
	summaryPath := filepath.Join(proofDir, "page-pilot-summary.md")
	_ = os.WriteFile(diffPath, []byte(diffText), 0o644)
	_ = os.WriteFile(summaryPath, []byte(diffSummary+"\n\n## Line diff\n\n"+lineSummary+"\n"), 0o644)
	workRecord, err := server.ensurePagePilotWorkItem(ctx, payload, target)
	if err != nil {
		return nil, err
	}
	runID := "page_pilot_" + safeSegment(payload.Selection.SourceMapping.Symbol+"_"+nowISO())
	result := map[string]any{
		"id":                 runID,
		"status":             "applied",
		"projectId":          payload.ProjectID,
		"repositoryTargetId": payload.RepositoryTargetID,
		"requirementId":      workRecord["requirementId"],
		"workItemId":         workRecord["workItemId"],
		"pipelineId":         workRecord["pipelineId"],
		"pipelineRunId":      workRecord["pipelineRunId"],
		"agentMode":          "single-page-pilot-agent",
		"executionMode":      "live-preview",
		"repositoryPath":     repoPath,
		"repositoryTarget":   repositoryTargetLabel(target),
		"runner":             effectiveRunner,
		"instruction":        payload.Instruction,
		"selection":          selectionRecord(payload.Selection),
		"changedFiles":       changedFiles,
		"diffSummary":        diffSummary,
		"lineDiffSummary":    lineSummary,
		"proofFiles":         []string{promptPath, notePath, diffPath, summaryPath},
		"hmr": map[string]any{
			"mode":         "vite-hmr-or-dev-server-reload",
			"touchedFiles": changedFiles,
		},
		"runnerProcess": process,
		"createdAt":     nowISO(),
		"updatedAt":     nowISO(),
	}
	if err := server.Repo.SetPagePilotRun(ctx, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (server *Server) executePagePilotDeliver(ctx context.Context, payload pagePilotDeliverRequest) (map[string]any, error) {
	if strings.TrimSpace(payload.RepositoryTargetID) == "" {
		return nil, errors.New("repositoryTargetId is required")
	}
	repoPath, target, _, err := server.resolvePagePilotWorkspace(ctx, payload.ProjectID, payload.RepositoryTargetID, payload.Selection.SourceMapping.File)
	if err != nil {
		return nil, err
	}
	statusOutput, err := runCommand(repoPath, "git", "status", "--short")
	if err != nil {
		return nil, fmt.Errorf("read repository diff: %w", err)
	}
	if strings.TrimSpace(statusOutput) == "" {
		return nil, errors.New("no Page Pilot changes are waiting for delivery")
	}
	branchName := strings.TrimSpace(payload.BranchName)
	if branchName == "" {
		branchName = "omega/page-pilot-" + safeSegment(payload.Selection.SourceMapping.Symbol+"-"+nowISO())
	}
	currentBranch, _ := runCommand(repoPath, "git", "branch", "--show-current")
	currentBranch = strings.TrimSpace(currentBranch)
	if currentBranch != branchName {
		if _, err := runCommand(repoPath, "git", "switch", "-c", branchName); err != nil {
			if _, retryErr := runCommand(repoPath, "git", "switch", branchName); retryErr != nil {
				return nil, fmt.Errorf("create delivery branch: %w", err)
			}
		}
	}
	changedNames, _ := runCommand(repoPath, "git", "diff", "--name-only")
	changedFiles := compactLines(changedNames)
	if _, err := runCommand(repoPath, "git", "add", "-A"); err != nil {
		return nil, fmt.Errorf("stage page pilot changes: %w", err)
	}
	commitMessage := "Omega Page Pilot: " + compactCommitSubject(payload.Selection)
	if _, err := runCommand(repoPath, "git", "commit", "-m", commitMessage); err != nil {
		return nil, fmt.Errorf("commit page pilot changes: %w", err)
	}
	commitSha, _ := runCommand(repoPath, "git", "rev-parse", "HEAD")
	commitDiff, _ := runCommand(repoPath, "git", "diff", "HEAD~1..HEAD")
	prURL := ""
	if text(target, "kind") == "github" {
		_, _ = runCommand(repoPath, "gh", "auth", "setup-git")
		if err := pushDevFlowBranch(repoPath, branchName); err != nil {
			return nil, fmt.Errorf("push page pilot branch: %w", err)
		}
		prBody := fmt.Sprintf("## Omega Page Pilot\n\n### Instruction\n%s\n\n### Source\n- `%s`\n- Selector: `%s`\n\n### Changed files\n%s\n\n### Line-level summary\n%s\n",
			payload.Instruction,
			payload.Selection.SourceMapping.Source,
			payload.Selection.StableSelector,
			markdownFileList(changedFiles),
			buildLineDiffSummary(commitDiff),
		)
		prURL, err = ensureDevFlowPullRequest(repoPath, repositoryTargetLabel(target), branchName, stringOr(text(target, "defaultBranch"), "main"), commitMessage, prBody)
		if err != nil {
			return nil, fmt.Errorf("create page pilot pull request: %w", err)
		}
	}
	result := map[string]any{
		"id":                 stringOr(payload.RunID, "page_pilot_delivery_"+safeSegment(branchName)),
		"status":             "delivered",
		"projectId":          payload.ProjectID,
		"repositoryTargetId": payload.RepositoryTargetID,
		"instruction":        payload.Instruction,
		"selection":          selectionRecord(payload.Selection),
		"repositoryPath":     repoPath,
		"branchName":         branchName,
		"commitSha":          strings.TrimSpace(commitSha),
		"pullRequestUrl":     prURL,
		"changedFiles":       changedFiles,
		"diffSummary":        buildPagePilotDiffSummary(commitDiff, changedFiles),
		"lineDiffSummary":    buildLineDiffSummary(commitDiff),
		"updatedAt":          nowISO(),
	}
	if payload.RunID != "" {
		if existing, err := server.getPagePilotRun(ctx, payload.RunID); err == nil {
			for key, value := range result {
				existing[key] = value
			}
			if text(existing, "createdAt") == "" {
				existing["createdAt"] = nowISO()
			}
			result = existing
		}
	}
	if text(result, "createdAt") == "" {
		result["createdAt"] = nowISO()
	}
	if err := server.Repo.SetPagePilotRun(ctx, result); err != nil {
		return nil, err
	}
	_ = server.updatePagePilotWorkItemStatus(ctx, text(result, "workItemId"), "Done", text(result, "pipelineId"), "delivered")
	return result, nil
}

func (server *Server) executePagePilotDiscard(ctx context.Context, runID string) (map[string]any, error) {
	record, err := server.getPagePilotRun(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("Page Pilot run %s not found", runID)
	}
	if text(record, "status") != "applied" {
		return nil, fmt.Errorf("Page Pilot run %s cannot be discarded after status %s", runID, text(record, "status"))
	}
	repoPath := text(record, "repositoryPath")
	if repoPath == "" {
		return nil, errors.New("Page Pilot run has no repository path")
	}
	changedFiles := arrayValues(record["changedFiles"])
	if len(changedFiles) == 0 {
		return nil, errors.New("Page Pilot run has no changed files to discard")
	}
	for _, rawFile := range changedFiles {
		file := strings.TrimSpace(fmt.Sprint(rawFile))
		if file == "" {
			continue
		}
		if _, err := runCommand(repoPath, "git", "checkout", "--", file); err != nil {
			_, _ = runCommand(repoPath, "git", "clean", "-f", "--", file)
		}
	}
	statusOutput, _ := runCommand(repoPath, "git", "status", "--short")
	record["status"] = "discarded"
	record["discardedAt"] = nowISO()
	record["updatedAt"] = nowISO()
	record["lineDiffSummary"] = "Discarded local Page Pilot source changes."
	record["repositoryStatus"] = strings.TrimSpace(statusOutput)
	if err := server.Repo.SetPagePilotRun(ctx, record); err != nil {
		return nil, err
	}
	_ = server.updatePagePilotWorkItemStatus(ctx, text(record, "workItemId"), "Blocked", text(record, "pipelineId"), "discarded")
	return record, nil
}

func (server *Server) getPagePilotRun(ctx context.Context, runID string) (map[string]any, error) {
	record, err := server.Repo.GetPagePilotRun(ctx, runID)
	if err == nil {
		return record, nil
	}
	if legacy, legacyErr := server.Repo.GetSetting(ctx, pagePilotRunSettingPrefix+runID); legacyErr == nil {
		return legacy, nil
	}
	return nil, err
}

func (server *Server) resolvePagePilotWorkspace(ctx context.Context, projectID string, repositoryTargetID string, sourceFile string) (string, map[string]any, ProjectAgentProfile, error) {
	database, err := mustLoad(server, ctx)
	if err != nil {
		return "", nil, ProjectAgentProfile{}, err
	}
	target := findRepositoryTarget(database, repositoryTargetID)
	if target == nil {
		return "", nil, ProjectAgentProfile{}, fmt.Errorf("repository target %s not found", repositoryTargetID)
	}
	repoPath := ""
	if text(target, "kind") == "local" {
		repoPath = text(target, "path")
	} else if text(target, "kind") == "github" {
		cwd, _ := os.Getwd()
		if pagePilotGitRemoteMatches(cwd, text(target, "owner"), text(target, "repo")) {
			repoPath = cwd
		}
	}
	if repoPath == "" {
		return "", nil, ProjectAgentProfile{}, errors.New("Page Pilot needs a local repository workspace for HMR; bind a local repository target or run the local runtime from the matching GitHub worktree")
	}
	absRepo, err := filepath.Abs(repoPath)
	if err != nil {
		return "", nil, ProjectAgentProfile{}, err
	}
	if sourceFile != "" {
		sourcePath := filepath.Clean(filepath.Join(absRepo, sourceFile))
		if !strings.HasPrefix(sourcePath, absRepo+string(os.PathSeparator)) && sourcePath != absRepo {
			return "", nil, ProjectAgentProfile{}, fmt.Errorf("source mapping escapes repository: %s", sourceFile)
		}
		if _, err := os.Stat(sourcePath); err != nil {
			return "", nil, ProjectAgentProfile{}, fmt.Errorf("source mapping file is not present in repository workspace: %s", sourceFile)
		}
	}
	item := map[string]any{"projectId": projectID, "repositoryTargetId": repositoryTargetID}
	profile := server.resolveAgentProfile(ctx, database, item, target)
	return absRepo, target, profile, nil
}

func (server *Server) ensurePagePilotWorkItem(ctx context.Context, payload pagePilotApplyRequest, target map[string]any) (map[string]any, error) {
	database, err := mustLoad(server, ctx)
	if err != nil {
		return nil, err
	}
	timestamp := nowISO()
	symbol := strings.TrimSpace(payload.Selection.SourceMapping.Symbol)
	if symbol == "" {
		symbol = payload.Selection.ElementKind
	}
	titleSubject := strings.TrimSpace(payload.Selection.TextSnapshot)
	if titleSubject == "" {
		titleSubject = strings.TrimSpace(payload.Selection.StableSelector)
	}
	if len(titleSubject) > 64 {
		titleSubject = strings.TrimSpace(titleSubject[:64]) + "..."
	}
	itemID := "item_page_pilot_" + safeSegment(payload.RepositoryTargetID+"_"+symbol+"_"+timestamp)
	sourceRef := "page-pilot:" + itemID
	item := map[string]any{
		"id":                 itemID,
		"key":                "PP-" + fmt.Sprint(len(database.Tables.WorkItems)+1),
		"title":              "Page Pilot: " + stringOr(titleSubject, "selected page element"),
		"description":        buildPagePilotRequirementText(payload),
		"status":             "In Review",
		"priority":           "High",
		"assignee":           "page-pilot",
		"labels":             []any{"page-pilot", "live-preview"},
		"team":               "Omega",
		"stageId":            "page_pilot",
		"target":             repositoryTargetLabel(target),
		"repositoryTargetId": payload.RepositoryTargetID,
		"source":             "page_pilot",
		"sourceExternalRef":  sourceRef,
		"acceptanceCriteria": pagePilotAcceptanceCriteria(payload),
		"artifacts": map[string]any{
			"selectionContext": selectionRecord(payload.Selection),
			"agentMode":        "single-page-pilot-agent",
			"executionMode":    "live-preview",
		},
	}
	database = appendWorkItem(database, item)
	persisted := findWorkItem(database, itemID)
	if persisted == nil {
		for _, candidate := range database.Tables.WorkItems {
			if text(candidate, "sourceExternalRef") == sourceRef {
				persisted = candidate
				break
			}
		}
	}
	if persisted == nil {
		return nil, errors.New("Page Pilot work item was not persisted")
	}
	pipeline := makePagePilotPipeline(persisted, payload)
	if pipelineIndex := findByID(database.Tables.Pipelines, text(pipeline, "id")); pipelineIndex >= 0 {
		database.Tables.Pipelines[pipelineIndex] = pipeline
	} else {
		database.Tables.Pipelines = append(database.Tables.Pipelines, pipeline)
	}
	touch(&database)
	if err := server.Repo.Save(ctx, database); err != nil {
		return nil, err
	}
	return map[string]any{
		"requirementId": text(persisted, "requirementId"),
		"workItemId":    text(persisted, "id"),
		"pipelineId":    text(pipeline, "id"),
		"pipelineRunId": text(pipeline, "runId"),
	}, nil
}

func makePagePilotPipeline(item map[string]any, payload pagePilotApplyRequest) map[string]any {
	createdAt := nowISO()
	runID := fmt.Sprintf("run_%s", text(item, "id"))
	stages := []map[string]any{
		{"id": "preview_runtime", "title": "Preview runtime", "status": "passed", "agentIds": []any{"page-pilot"}, "outputArtifacts": []any{"preview-runtime-profile"}},
		{"id": "page_editing", "title": "Page editing", "status": "passed", "agentIds": []any{"page-pilot"}, "inputArtifacts": []any{"selection-context", "preview-runtime-profile"}, "outputArtifacts": []any{"source-patch", "diff-summary"}},
		{"id": "delivery", "title": "Delivery", "status": "waiting-human", "agentIds": []any{"page-pilot"}, "inputArtifacts": []any{"source-patch", "diff-summary"}, "outputArtifacts": []any{"branch", "commit", "pull-request"}},
	}
	return map[string]any{
		"id":         fmt.Sprintf("pipeline_%s", text(item, "id")),
		"workItemId": text(item, "id"),
		"runId":      runID,
		"status":     "waiting-human",
		"templateId": "page-pilot",
		"run": map[string]any{
			"id": runID,
			"requirement": map[string]any{
				"id":          text(item, "requirementId"),
				"identifier":  text(item, "key"),
				"title":       text(item, "title"),
				"description": text(item, "description"),
				"source":      "page_pilot",
				"priority":    "high",
				"requester":   "page-pilot",
				"labels":      item["labels"],
				"createdAt":   createdAt,
			},
			"goal":            fmt.Sprintf("Apply Page Pilot edit for %s", text(item, "key")),
			"successCriteria": pagePilotAcceptanceCriteria(payload),
			"stages":          stages,
			"agents": []map[string]any{{
				"id":             "page-pilot",
				"name":           "Page Pilot Agent",
				"role":           "Preview runtime, page editing, and delivery",
				"inputContract":  []any{"selection-context", "preview-runtime-profile", "user-instruction"},
				"outputContract": []any{"source-patch", "diff-summary", "delivery-proof"},
			}},
			"orchestrator": map[string]any{
				"masterAgentId":      "page-pilot",
				"dispatchStatus":     "waiting-human",
				"templateId":         "page-pilot",
				"repositoryTargetId": text(item, "repositoryTargetId"),
				"executionMode":      "live-preview",
			},
			"workflow": map[string]any{
				"id":   "page-pilot",
				"name": "Page Pilot live-preview flow",
			},
			"dataFlow": []map[string]any{
				{"from": "preview_runtime", "to": "page_editing", "artifact": "preview-runtime-profile"},
				{"from": "page_editing", "to": "delivery", "artifact": "source-patch"},
			},
			"selectedCapabilities": map[string]any{"agentMode": "single-page-pilot-agent", "executionMode": "live-preview"},
			"artifacts": map[string]any{
				"selectionContext": selectionRecord(payload.Selection),
				"instruction":      payload.Instruction,
			},
			"events": []map[string]any{
				{"id": fmt.Sprintf("event_%s_1", runID), "type": "run.created", "message": "Page Pilot session captured as a Requirement-backed run.", "timestamp": createdAt, "stageId": "preview_runtime", "agentId": "page-pilot"},
				{"id": fmt.Sprintf("event_%s_2", runID), "type": "page_pilot.patch.applied", "message": "Page Pilot Agent applied a live-preview patch and is waiting for confirmation.", "timestamp": createdAt, "stageId": "page_editing", "agentId": "page-pilot"},
			},
			"createdAt": createdAt,
			"updatedAt": createdAt,
		},
		"createdAt": createdAt,
		"updatedAt": createdAt,
	}
}

func (server *Server) updatePagePilotWorkItemStatus(ctx context.Context, workItemID string, status string, pipelineID string, pipelineStatus string) error {
	if strings.TrimSpace(workItemID) == "" {
		return nil
	}
	database, err := mustLoad(server, ctx)
	if err != nil {
		return err
	}
	database = updateWorkItem(database, workItemID, map[string]any{"status": status})
	if strings.TrimSpace(pipelineID) != "" {
		for index, pipeline := range database.Tables.Pipelines {
			if text(pipeline, "id") == pipelineID {
				next := cloneMap(pipeline)
				next["status"] = pipelineStatus
				next["updatedAt"] = nowISO()
				run := mapValue(next["run"])
				run["updatedAt"] = nowISO()
				run["status"] = pipelineStatus
				next["run"] = run
				database.Tables.Pipelines[index] = next
			}
		}
	}
	return server.Repo.Save(ctx, database)
}

func buildPagePilotRequirementText(payload pagePilotApplyRequest) string {
	selectionBytes, _ := json.MarshalIndent(selectionRecord(payload.Selection), "", "  ")
	return fmt.Sprintf("Page Pilot live-preview edit.\n\nUser instruction:\n%s\n\nSelection context:\n%s", strings.TrimSpace(payload.Instruction), string(selectionBytes))
}

func pagePilotAcceptanceCriteria(payload pagePilotApplyRequest) []any {
	criteria := []any{
		"Selected page element is updated according to the user instruction.",
		"Change is applied inside the bound repository workspace only.",
		"Preview can be refreshed or reloaded to inspect the result.",
	}
	if payload.Selection.SourceMapping.File != "" {
		criteria = append(criteria, fmt.Sprintf("Mapped source file `%s` is updated intentionally.", payload.Selection.SourceMapping.File))
	}
	if payload.Selection.StableSelector != "" {
		criteria = append(criteria, fmt.Sprintf("Element selector `%s` remains inspectable after the change.", payload.Selection.StableSelector))
	}
	return criteria
}

func pagePilotSourceStatusFiles(statusOutput string) []string {
	files := []string{}
	for _, line := range strings.Split(statusOutput, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		file := strings.TrimSpace(line)
		if len(line) > 3 {
			file = strings.TrimSpace(line[3:])
		}
		if strings.Contains(file, " -> ") {
			parts := strings.Split(file, " -> ")
			file = strings.TrimSpace(parts[len(parts)-1])
		}
		if strings.HasPrefix(file, ".omega/") || file == ".omega" {
			continue
		}
		files = append(files, file)
	}
	return mergeUniqueStrings(files)
}

func mergeUniqueStrings(groups ...[]string) []string {
	seen := map[string]bool{}
	merged := []string{}
	for _, group := range groups {
		for _, value := range group {
			value = strings.TrimSpace(value)
			if value == "" || seen[value] {
				continue
			}
			seen[value] = true
			merged = append(merged, value)
		}
	}
	return merged
}

func pagePilotGitRemoteMatches(repoPath string, owner string, repo string) bool {
	if repoPath == "" || owner == "" || repo == "" {
		return false
	}
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); err != nil {
		return false
	}
	remote, err := runCommand(repoPath, "git", "remote", "get-url", "origin")
	if err != nil {
		return false
	}
	normalized := strings.ToLower(strings.TrimSpace(remote))
	target := strings.ToLower(owner + "/" + repo)
	return strings.Contains(normalized, target) || strings.Contains(strings.TrimSuffix(normalized, ".git"), target)
}

func buildPagePilotPrompt(repoPath string, repoLabel string, payload pagePilotApplyRequest) string {
	selectionBytes, _ := json.MarshalIndent(payload.Selection, "", "  ")
	return fmt.Sprintf(`You are Omega Page Pilot, editing a live React preview through source metadata.

Repository: %s
Repository path: %s

User instruction:
%s

Selected element context:
%s

Rules:
- Work only inside this repository.
- Treat the selected element context above as the primary target. If the instruction contains multiple annotations, use them as supporting context unless the user explicitly says otherwise.
- Use the primary target source mapping first; do not rely on brittle DOM reverse lookup when a source mapping is available.
- Modify the real source file(s), not generated proof files.
- Keep the change minimal and consistent with light and dark themes.
- Do not commit, push, or open a pull request. Omega will handle delivery after user confirmation.
`, repoLabel, repoPath, payload.Instruction, string(selectionBytes))
}

func selectionRecord(selection pagePilotSelectionContext) map[string]any {
	return map[string]any{
		"elementKind":    selection.ElementKind,
		"stableSelector": selection.StableSelector,
		"textSnapshot":   selection.TextSnapshot,
		"styleSnapshot":  selection.StyleSnapshot,
		"domContext":     selection.DOMContext,
		"sourceMapping": map[string]any{
			"source": selection.SourceMapping.Source,
			"file":   selection.SourceMapping.File,
			"symbol": selection.SourceMapping.Symbol,
			"line":   selection.SourceMapping.Line,
		},
	}
}

func applyPagePilotLocalPatch(repoPath string, payload pagePilotApplyRequest) (map[string]any, error) {
	sourceFile := strings.TrimSpace(payload.Selection.SourceMapping.File)
	if sourceFile == "" {
		return nil, errors.New("local Page Pilot patch requires sourceMapping.file")
	}
	path := filepath.Join(repoPath, sourceFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	before := payload.Selection.TextSnapshot
	after := directReplacementText(payload.Instruction)
	if strings.TrimSpace(after) == "" {
		return nil, errors.New("local Page Pilot patch requires a direct replacement instruction such as: replace text with \"...\"")
	}
	content := string(raw)
	if before == "" || !strings.Contains(content, before) {
		return nil, errors.New("selected text was not found in mapped source file")
	}
	next := strings.Replace(content, before, after, 1)
	if err := os.WriteFile(path, []byte(next), 0o644); err != nil {
		return nil, err
	}
	return map[string]any{"runner": "local-proof", "status": "passed", "stdout": "Applied direct Page Pilot source text replacement."}, nil
}

func directReplacementText(instruction string) string {
	trimmed := strings.TrimSpace(instruction)
	lower := strings.ToLower(trimmed)
	for _, marker := range []string{"replace text with", "change text to", "set text to", "改成", "替换为"} {
		if index := strings.Index(lower, marker); index >= 0 {
			value := strings.TrimSpace(trimmed[index+len(marker):])
			return strings.Trim(value, " \"'`：:")
		}
	}
	return ""
}

func sourceFileOrDot(sourceFile string) string {
	if strings.TrimSpace(sourceFile) == "" {
		return "."
	}
	return filepath.Clean(sourceFile)
}

func buildPagePilotDiffSummary(diffText string, changedFiles []string) string {
	return fmt.Sprintf("Changed %d file(s): %s\n\n```diff\n%s\n```", len(changedFiles), strings.Join(changedFiles, ", "), truncateForProof(diffText, 4000))
}

func buildLineDiffSummary(diffText string) string {
	lines := []string{}
	for _, line := range strings.Split(diffText, "\n") {
		if strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") {
			continue
		}
		if strings.HasPrefix(line, "@@") || (strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++")) || (strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---")) {
			lines = append(lines, line)
		}
		if len(lines) >= 18 {
			break
		}
	}
	if len(lines) == 0 {
		return "No line-level diff available."
	}
	return "```diff\n" + strings.Join(lines, "\n") + "\n```"
}

func compactCommitSubject(selection pagePilotSelectionContext) string {
	if selection.SourceMapping.Symbol != "" {
		return selection.SourceMapping.Symbol
	}
	if selection.ElementKind != "" {
		return selection.ElementKind
	}
	return "page update"
}
