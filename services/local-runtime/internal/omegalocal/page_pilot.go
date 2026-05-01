package omegalocal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
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
	RunID                 string                    `json:"runId"`
	ProjectID             string                    `json:"projectId"`
	RepositoryTargetID    string                    `json:"repositoryTargetId"`
	ExecutionMode         string                    `json:"executionMode"`
	Instruction           string                    `json:"instruction"`
	Selection             pagePilotSelectionContext `json:"selection"`
	Runner                string                    `json:"runner"`
	ConversationBatch     map[string]any            `json:"conversationBatch"`
	SubmittedAnnotations  []map[string]any          `json:"submittedAnnotations"`
	ProcessEvents         []map[string]any          `json:"processEvents"`
	PreviewRuntimeProfile map[string]any            `json:"previewRuntimeProfile"`
}

type pagePilotDeliverRequest struct {
	RunID                 string                    `json:"runId"`
	ProjectID             string                    `json:"projectId"`
	RepositoryTargetID    string                    `json:"repositoryTargetId"`
	ExecutionMode         string                    `json:"executionMode"`
	Instruction           string                    `json:"instruction"`
	Selection             pagePilotSelectionContext `json:"selection"`
	BranchName            string                    `json:"branchName"`
	Draft                 bool                      `json:"draft"`
	ConversationBatch     map[string]any            `json:"conversationBatch"`
	SubmittedAnnotations  []map[string]any          `json:"submittedAnnotations"`
	ProcessEvents         []map[string]any          `json:"processEvents"`
	PreviewRuntimeProfile map[string]any            `json:"previewRuntimeProfile"`
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
	server.logInfo(ctx, "page_pilot.apply.requested", "Page Pilot apply requested.", map[string]any{"projectId": payload.ProjectID, "repositoryTargetId": payload.RepositoryTargetID, "stageId": "page_editing"})
	if strings.TrimSpace(payload.RepositoryTargetID) == "" {
		server.logError(ctx, "page_pilot.apply.invalid", "repositoryTargetId is required", map[string]any{"projectId": payload.ProjectID})
		return nil, errors.New("repositoryTargetId is required")
	}
	if strings.TrimSpace(payload.Instruction) == "" {
		server.logError(ctx, "page_pilot.apply.invalid", "instruction is required", map[string]any{"projectId": payload.ProjectID, "repositoryTargetId": payload.RepositoryTargetID})
		return nil, errors.New("instruction is required")
	}
	repoPath, target, profile, err := server.resolvePagePilotWorkspace(ctx, payload.ProjectID, payload.RepositoryTargetID, payload.Selection.SourceMapping.File)
	if err != nil {
		server.logError(ctx, "page_pilot.apply.resolve_failed", err.Error(), map[string]any{"projectId": payload.ProjectID, "repositoryTargetId": payload.RepositoryTargetID, "sourceFile": payload.Selection.SourceMapping.File})
		return nil, err
	}
	existingRun := map[string]any{}
	if strings.TrimSpace(payload.RunID) != "" {
		existingRun, err = server.getPagePilotRun(ctx, payload.RunID)
		if err != nil {
			return nil, fmt.Errorf("continue Page Pilot run: %w", err)
		}
		if text(existingRun, "repositoryTargetId") != payload.RepositoryTargetID {
			return nil, errors.New("Page Pilot run belongs to a different repository target")
		}
		if text(existingRun, "status") != "applied" {
			return nil, fmt.Errorf("Page Pilot run %s cannot accept another apply after status %s", payload.RunID, text(existingRun, "status"))
		}
	}
	lock, err := claimPagePilotLivePreviewLock(ctx, server, payload.ProjectID, payload.RepositoryTargetID, target, repoPath, payload.RunID)
	if err != nil {
		server.logError(ctx, "page_pilot.apply.lock_failed", err.Error(), map[string]any{"projectId": payload.ProjectID, "repositoryTargetId": payload.RepositoryTargetID, "repositoryPath": repoPath})
		return nil, err
	}
	releaseLockOnError := true
	defer func() {
		if releaseLockOnError {
			_ = releasePagePilotExecutionLock(ctx, server, lock, "released-after-failure")
		}
	}()
	runnerID := strings.TrimSpace(payload.Runner)
	if runnerID == "" {
		runnerID = "profile"
	}
	effectiveRunner, err := preflightAgentRunner(runnerID, profile, "coding")
	if err != nil {
		server.logError(ctx, "page_pilot.apply.preflight_failed", err.Error(), map[string]any{"projectId": payload.ProjectID, "repositoryTargetId": payload.RepositoryTargetID, "runner": runnerID})
		return nil, err
	}
	server.logDebug(ctx, "page_pilot.apply.runner_selected", "Page Pilot runner selected.", map[string]any{"projectId": payload.ProjectID, "repositoryTargetId": payload.RepositoryTargetID, "runner": effectiveRunner, "repositoryPath": repoPath})
	annotations := pagePilotSubmittedAnnotations(payload.ConversationBatch, payload.SubmittedAnnotations, payload.Selection)
	sourceMappingReport := pagePilotSourceMappingReport(annotations, payload.Selection)
	sourceLocator := pagePilotSourceLocatorReport(repoPath, annotations, payload.Selection, sourceMappingReport)
	proofDir := filepath.Join(repoPath, ".omega", "page-pilot")
	if err := os.MkdirAll(proofDir, 0o755); err != nil {
		return nil, err
	}
	promptPath := filepath.Join(proofDir, "page-pilot-prompt.md")
	notePath := filepath.Join(proofDir, "page-pilot-agent-note.md")
	prompt := buildPagePilotPrompt(repoPath, repositoryTargetLabel(target), payload, sourceMappingReport, sourceLocator)
	if err := os.WriteFile(promptPath, []byte(prompt), 0o644); err != nil {
		return nil, err
	}
	process := map[string]any{"runner": effectiveRunner, "status": "passed"}
	if effectiveRunner == "local-proof" || effectiveRunner == "demo-code" {
		process, err = applyPagePilotLocalPatch(repoPath, payload, sourceLocator)
	} else {
		runner, resolved := NewAgentRunnerRegistry().Resolve(effectiveRunner)
		profileForRole := agentProfileForRole(profile, "coding")
		model, env := server.runnerCredentialModelAndEnv(ctx, resolved, profileForRole.Model)
		turn := runner.RunTurn(ctx, AgentTurnRequest{
			Role:       "page-pilot",
			StageID:    "page_pilot",
			Runner:     resolved,
			Workspace:  repoPath,
			Prompt:     prompt,
			OutputPath: notePath,
			Sandbox:    "workspace-write",
			Model:      model,
			Env:        env,
		})
		process = turn.Process
		if turn.Error != nil {
			server.logError(ctx, "page_pilot.apply.runner_failed", turn.Error.Error(), map[string]any{"projectId": payload.ProjectID, "repositoryTargetId": payload.RepositoryTargetID, "runner": effectiveRunner, "repositoryPath": repoPath})
			return map[string]any{"status": "failed", "repositoryPath": repoPath, "runnerProcess": process}, fmt.Errorf("page pilot runner failed: %w", turn.Error)
		}
	}
	statusOutput, err := runCommand(repoPath, "git", "status", "--short")
	if err != nil {
		return nil, fmt.Errorf("read repository diff: %w", err)
	}
	sourceStatusFiles := pagePilotSourceStatusFiles(statusOutput)
	if len(sourceStatusFiles) == 0 {
		server.logError(ctx, "page_pilot.apply.no_changes", "Page Pilot runner produced no repository changes.", map[string]any{"projectId": payload.ProjectID, "repositoryTargetId": payload.RepositoryTargetID, "runner": effectiveRunner, "repositoryPath": repoPath})
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
	workRecord := map[string]any{}
	if len(existingRun) > 0 {
		workRecord = map[string]any{
			"requirementId": text(existingRun, "requirementId"),
			"workItemId":    text(existingRun, "workItemId"),
			"pipelineId":    text(existingRun, "pipelineId"),
			"pipelineRunId": text(existingRun, "pipelineRunId"),
		}
	} else {
		workRecord, err = server.ensurePagePilotWorkItem(ctx, payload, target)
		if err != nil {
			return nil, err
		}
	}
	runID := stringOr(strings.TrimSpace(payload.RunID), "page_pilot_"+safeSegment(payload.Selection.SourceMapping.Symbol+"_"+nowISO()))
	conversationSeed := payload.ConversationBatch
	if len(conversationSeed) == 0 {
		conversationSeed = mapValue(existingRun["conversationBatch"])
	}
	conversationBatch := pagePilotConversationBatch(conversationSeed, "applied", runID, payload.Instruction, annotations)
	processEvents := pagePilotProcessEvents(payload.ProcessEvents)
	if len(processEvents) == 0 {
		processEvents = arrayMaps(existingRun["processEvents"])
	}
	roundNumber := intNumber(existingRun["roundNumber"]) + 1
	if roundNumber <= 0 {
		roundNumber = 1
	}
	prPreview := pagePilotPRPreview(payload, changedFiles, diffSummary, lineSummary, target)
	visualProof := pagePilotVisualProof(payload.Selection, annotations, changedFiles, diffSummary, "dom-snapshot")
	executionMode := pagePilotExecutionMode(payload.ExecutionMode, target, repoPath)
	result := map[string]any{
		"id":                    runID,
		"status":                "applied",
		"projectId":             payload.ProjectID,
		"repositoryTargetId":    payload.RepositoryTargetID,
		"requirementId":         workRecord["requirementId"],
		"workItemId":            workRecord["workItemId"],
		"pipelineId":            workRecord["pipelineId"],
		"pipelineRunId":         workRecord["pipelineRunId"],
		"agentMode":             "single-page-pilot-agent",
		"executionMode":         executionMode,
		"isolation":             pagePilotIsolationRecord(executionMode, target, repoPath),
		"repositoryPath":        repoPath,
		"repositoryTarget":      repositoryTargetLabel(target),
		"runner":                effectiveRunner,
		"instruction":           payload.Instruction,
		"selection":             selectionRecord(payload.Selection),
		"primaryTarget":         pagePilotPrimaryTarget(conversationBatch, payload.Selection),
		"conversationBatch":     conversationBatch,
		"submittedAnnotations":  annotations,
		"processEvents":         processEvents,
		"roundNumber":           roundNumber,
		"previewRuntimeProfile": pagePilotPreviewRuntimeProfile(payload.PreviewRuntimeProfile, repoPath, payload.RepositoryTargetID),
		"sourceMappingReport":   sourceMappingReport,
		"sourceLocator":         sourceLocator,
		"changedFiles":          changedFiles,
		"diffSummary":           diffSummary,
		"lineDiffSummary":       lineSummary,
		"prPreview":             prPreview,
		"visualProof":           visualProof,
		"proofFiles":            []string{promptPath, notePath, diffPath, summaryPath},
		"hmr": map[string]any{
			"mode":         "vite-hmr-or-dev-server-reload",
			"touchedFiles": changedFiles,
		},
		"runnerProcess": process,
		"executionLock": lock,
		"createdAt":     nowISO(),
		"updatedAt":     nowISO(),
	}
	if len(existingRun) > 0 {
		createdAt := text(existingRun, "createdAt")
		for key, value := range result {
			existingRun[key] = value
		}
		if createdAt != "" {
			existingRun["createdAt"] = createdAt
		}
		result = existingRun
	}
	lock = attachPagePilotRunToExecutionLock(ctx, server, lock, result)
	result["executionLock"] = lock
	if err := server.Repo.SetPagePilotRun(ctx, result); err != nil {
		server.logError(ctx, "page_pilot.apply.persist_failed", err.Error(), map[string]any{"runId": runID, "projectId": payload.ProjectID, "repositoryTargetId": payload.RepositoryTargetID})
		return nil, err
	}
	releaseLockOnError = false
	if err := server.syncPagePilotRunRecords(ctx, result, "page_editing"); err != nil {
		server.logError(ctx, "page_pilot.apply.sync_failed", err.Error(), map[string]any{"runId": runID, "projectId": payload.ProjectID, "repositoryTargetId": payload.RepositoryTargetID})
		return nil, err
	}
	server.logInfo(ctx, "page_pilot.apply.applied", "Page Pilot source patch applied.", map[string]any{"entityType": "page-pilot-run", "entityId": runID, "projectId": payload.ProjectID, "repositoryTargetId": payload.RepositoryTargetID, "workItemId": text(result, "workItemId"), "pipelineId": text(result, "pipelineId"), "runner": effectiveRunner, "changedFiles": changedFiles})
	return result, nil
}

func (server *Server) executePagePilotDeliver(ctx context.Context, payload pagePilotDeliverRequest) (map[string]any, error) {
	server.logInfo(ctx, "page_pilot.deliver.requested", "Page Pilot delivery requested.", map[string]any{"entityType": "page-pilot-run", "entityId": payload.RunID, "projectId": payload.ProjectID, "repositoryTargetId": payload.RepositoryTargetID})
	if strings.TrimSpace(payload.RepositoryTargetID) == "" {
		return nil, errors.New("repositoryTargetId is required")
	}
	repoPath, target, _, err := server.resolvePagePilotWorkspace(ctx, payload.ProjectID, payload.RepositoryTargetID, payload.Selection.SourceMapping.File)
	if err != nil {
		server.logError(ctx, "page_pilot.deliver.resolve_failed", err.Error(), map[string]any{"entityType": "page-pilot-run", "entityId": payload.RunID, "projectId": payload.ProjectID, "repositoryTargetId": payload.RepositoryTargetID})
		return nil, err
	}
	lock, err := claimPagePilotLivePreviewLock(ctx, server, payload.ProjectID, payload.RepositoryTargetID, target, repoPath, payload.RunID)
	if err != nil {
		server.logError(ctx, "page_pilot.deliver.lock_failed", err.Error(), map[string]any{"entityType": "page-pilot-run", "entityId": payload.RunID, "projectId": payload.ProjectID, "repositoryTargetId": payload.RepositoryTargetID, "repositoryPath": repoPath})
		return nil, err
	}
	statusOutput, err := runCommand(repoPath, "git", "status", "--short")
	if err != nil {
		return nil, fmt.Errorf("read repository diff: %w", err)
	}
	if strings.TrimSpace(statusOutput) == "" {
		server.logError(ctx, "page_pilot.deliver.no_changes", "No Page Pilot changes are waiting for delivery.", map[string]any{"entityType": "page-pilot-run", "entityId": payload.RunID, "repositoryPath": repoPath})
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
			server.logError(ctx, "page_pilot.deliver.push_failed", err.Error(), map[string]any{"entityType": "page-pilot-run", "entityId": payload.RunID, "repositoryPath": repoPath, "branchName": branchName})
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
			server.logError(ctx, "page_pilot.deliver.pr_failed", err.Error(), map[string]any{"entityType": "page-pilot-run", "entityId": payload.RunID, "repositoryPath": repoPath, "branchName": branchName})
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
		"executionMode":      pagePilotExecutionMode(payload.ExecutionMode, target, repoPath),
		"branchName":         branchName,
		"commitSha":          strings.TrimSpace(commitSha),
		"pullRequestUrl":     prURL,
		"changedFiles":       changedFiles,
		"diffSummary":        buildPagePilotDiffSummary(commitDiff, changedFiles),
		"lineDiffSummary":    buildLineDiffSummary(commitDiff),
		"prPreview":          pagePilotPRPreview(payload, changedFiles, buildPagePilotDiffSummary(commitDiff, changedFiles), buildLineDiffSummary(commitDiff), target),
		"executionLock":      lock,
		"updatedAt":          nowISO(),
	}
	result["isolation"] = pagePilotIsolationRecord(text(result, "executionMode"), target, repoPath)
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
	result = applyPagePilotConversationUpdate(result, payload.ConversationBatch, payload.SubmittedAnnotations, payload.ProcessEvents, "delivered", text(result, "id"), payload.Instruction, payload.Selection)
	if len(payload.PreviewRuntimeProfile) > 0 {
		result["previewRuntimeProfile"] = pagePilotPreviewRuntimeProfile(payload.PreviewRuntimeProfile, repoPath, payload.RepositoryTargetID)
	}
	if result["sourceMappingReport"] == nil {
		result["sourceMappingReport"] = pagePilotSourceMappingReport(arrayMaps(result["submittedAnnotations"]), payload.Selection)
	}
	if result["sourceLocator"] == nil {
		result["sourceLocator"] = pagePilotSourceLocatorReport(repoPath, arrayMaps(result["submittedAnnotations"]), payload.Selection, mapValue(result["sourceMappingReport"]))
	}
	if result["visualProof"] == nil {
		result["visualProof"] = pagePilotVisualProof(payload.Selection, arrayMaps(result["submittedAnnotations"]), changedFiles, text(result, "diffSummary"), "dom-snapshot")
	}
	if text(result, "createdAt") == "" {
		result["createdAt"] = nowISO()
	}
	if err := server.Repo.SetPagePilotRun(ctx, result); err != nil {
		return nil, err
	}
	_ = server.updatePagePilotWorkItemStatus(ctx, text(result, "workItemId"), "Done", text(result, "pipelineId"), "delivered")
	_ = server.syncPagePilotRunRecords(ctx, result, "delivery")
	_ = releasePagePilotExecutionLock(ctx, server, lock, "released-after-delivery")
	server.logInfo(ctx, "page_pilot.deliver.delivered", "Page Pilot change delivered.", map[string]any{"entityType": "page-pilot-run", "entityId": text(result, "id"), "projectId": payload.ProjectID, "repositoryTargetId": payload.RepositoryTargetID, "workItemId": text(result, "workItemId"), "pipelineId": text(result, "pipelineId"), "branchName": branchName, "pullRequestUrl": prURL})
	return result, nil
}

func (server *Server) executePagePilotDiscard(ctx context.Context, runID string) (map[string]any, error) {
	server.logInfo(ctx, "page_pilot.discard.requested", "Page Pilot discard requested.", map[string]any{"entityType": "page-pilot-run", "entityId": runID})
	record, err := server.getPagePilotRun(ctx, runID)
	if err != nil {
		server.logError(ctx, "page_pilot.discard.not_found", err.Error(), map[string]any{"entityType": "page-pilot-run", "entityId": runID})
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
	if text(record, "executionMode") == "isolated-devflow" {
		_, _ = runCommand(repoPath, "git", "reset", "--hard")
		_, _ = runCommand(repoPath, "git", "clean", "-fd")
	} else {
		for _, rawFile := range changedFiles {
			file := strings.TrimSpace(fmt.Sprint(rawFile))
			if file == "" {
				continue
			}
			if _, err := runCommand(repoPath, "git", "checkout", "--", file); err != nil {
				_, _ = runCommand(repoPath, "git", "clean", "-f", "--", file)
			}
		}
	}
	statusOutput, _ := runCommand(repoPath, "git", "status", "--short")
	record["status"] = "discarded"
	record["discardedAt"] = nowISO()
	record["updatedAt"] = nowISO()
	record["lineDiffSummary"] = "Discarded local Page Pilot source changes."
	record["repositoryStatus"] = strings.TrimSpace(statusOutput)
	record = applyPagePilotConversationUpdate(record, nil, nil, nil, "discarded", runID, text(record, "instruction"), pagePilotSelectionFromRecord(mapValue(record["selection"])))
	if err := server.Repo.SetPagePilotRun(ctx, record); err != nil {
		return nil, err
	}
	_ = server.updatePagePilotWorkItemStatus(ctx, text(record, "workItemId"), "Blocked", text(record, "pipelineId"), "discarded")
	_ = server.syncPagePilotRunRecords(ctx, record, "delivery")
	_ = releasePagePilotRunExecutionLock(ctx, server, record, "released-after-discard")
	server.logInfo(ctx, "page_pilot.discard.discarded", "Page Pilot local source changes discarded.", map[string]any{"entityType": "page-pilot-run", "entityId": runID, "projectId": text(record, "projectId"), "repositoryTargetId": text(record, "repositoryTargetId"), "workItemId": text(record, "workItemId"), "pipelineId": text(record, "pipelineId")})
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
		candidate := pagePilotIsolatedWorkspacePath(target)
		if candidate != "" {
			if stat, statErr := os.Stat(filepath.Join(candidate, ".git")); statErr == nil && stat.IsDir() {
				repoPath = candidate
			} else if cloneTarget := repositoryTargetCloneTarget(target); cloneTarget != "" {
				if err := os.MkdirAll(filepath.Dir(candidate), 0o755); err != nil {
					return "", nil, ProjectAgentProfile{}, err
				}
				_ = os.RemoveAll(candidate)
				if output, cloneErr := cloneTargetRepository(filepath.Dir(candidate), cloneTarget, candidate); cloneErr != nil {
					return "", nil, ProjectAgentProfile{}, fmt.Errorf("prepare Page Pilot isolated workspace: %w\n%s", cloneErr, output)
				}
				repoPath = candidate
				_, _ = runCommand(repoPath, "git", "checkout", stringOr(text(target, "defaultBranch"), "main"))
				_, _ = runCommand(repoPath, "git", "config", "user.email", "omega-page-pilot@example.local")
				_, _ = runCommand(repoPath, "git", "config", "user.name", "Omega Page Pilot")
			}
		}
	}
	if repoPath == "" {
		return "", nil, ProjectAgentProfile{}, errors.New("Page Pilot needs an explicit local repository target or an Omega-managed isolated preview workspace")
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

func pagePilotExecutionMode(requested string, target map[string]any, repoPath string) string {
	if strings.TrimSpace(requested) != "" {
		return strings.TrimSpace(requested)
	}
	if text(target, "kind") == "github" && filepath.Clean(repoPath) == filepath.Clean(pagePilotIsolatedWorkspacePath(target)) {
		return "isolated-devflow"
	}
	return "live-preview"
}

func pagePilotIsolationRecord(mode string, target map[string]any, repoPath string) map[string]any {
	if mode != "isolated-devflow" {
		return map[string]any{"mode": mode, "repositoryPath": repoPath}
	}
	return map[string]any{
		"mode":             "isolated-devflow",
		"workspacePath":    repoPath,
		"targetRepository": repositoryTargetLabel(target),
		"confirmAction":    "branch-commit-pr",
		"safetyBoundary":   "preview changes stay in the isolated workspace until delivery",
	}
}

func pagePilotIsolatedWorkspacePath(target map[string]any) string {
	owner := strings.TrimSpace(text(target, "owner"))
	repo := strings.TrimSpace(text(target, "repo"))
	if owner == "" || repo == "" {
		return ""
	}
	root := strings.TrimSpace(os.Getenv("OMEGA_PAGE_PILOT_WORKSPACE_ROOT"))
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil || strings.TrimSpace(home) == "" {
			return ""
		}
		root = filepath.Join(home, "Omega", "workspaces", "page-pilot")
	}
	return filepath.Join(root, safeSegment(owner+"_"+repo))
}

func pagePilotLivePreviewExecutionScope(repositoryTargetID string, repoPath string) string {
	return fmt.Sprintf("page-pilot-live:%s:%s", strings.TrimSpace(repositoryTargetID), filepath.Clean(repoPath))
}

func claimPagePilotLivePreviewLock(ctx context.Context, server *Server, projectID string, repositoryTargetID string, target map[string]any, repoPath string, runID string) (map[string]any, error) {
	scope := pagePilotLivePreviewExecutionScope(repositoryTargetID, repoPath)
	if lock := existingExecutionLock(ctx, server, scope); lock != nil {
		if runID != "" && text(lock, "pagePilotRunId") == runID {
			lock["runnerProcessState"] = "running"
			lock["updatedAt"] = nowISO()
			_ = saveExecutionLock(ctx, server, lock)
			return lock, nil
		}
		owner := stringOr(text(lock, "pagePilotRunId"), text(lock, "attemptId"))
		if owner == "" {
			owner = text(lock, "id")
		}
		return nil, fmt.Errorf("Page Pilot live-preview workspace is locked by %s", owner)
	}
	if lock := conflictingRepositoryExecutionLock(ctx, server, repoPath, scope, runID); lock != nil {
		owner := stringOr(text(lock, "pagePilotRunId"), text(lock, "attemptId"))
		if owner == "" {
			owner = text(lock, "id")
		}
		return nil, fmt.Errorf("repository workspace is locked by %s", owner)
	}
	now := time.Now().UTC()
	lock := map[string]any{
		"id":                 executionLockID(scope),
		"scope":              scope,
		"status":             "claimed",
		"ownerType":          "page-pilot",
		"runnerProcessState": "running",
		"projectId":          projectID,
		"repositoryTargetId": repositoryTargetID,
		"repositoryTarget":   repositoryTargetLabel(target),
		"workspacePath":      repoPath,
		"repositoryPath":     repoPath,
		"cleanupPolicy":      "retain-live-preview-changes-until-confirm-or-discard",
		"expiresAt":          now.Add(2 * time.Hour).Format(time.RFC3339Nano),
		"createdAt":          now.Format(time.RFC3339Nano),
		"updatedAt":          now.Format(time.RFC3339Nano),
	}
	if runID != "" {
		lock["pagePilotRunId"] = runID
	}
	if err := saveExecutionLock(ctx, server, lock); err != nil {
		return nil, err
	}
	return lock, nil
}

func conflictingRepositoryExecutionLock(ctx context.Context, server *Server, repoPath string, allowedScope string, allowedRunID string) map[string]any {
	locks, err := server.Repo.ListSettings(ctx, "execution-lock:")
	if err != nil {
		return nil
	}
	cleanRepoPath := filepath.Clean(repoPath)
	for _, lock := range locks {
		if text(lock, "scope") == allowedScope && allowedRunID != "" && text(lock, "pagePilotRunId") == allowedRunID {
			continue
		}
		if text(lock, "status") == "released" || text(lock, "status") == "expired" {
			continue
		}
		if expiresAt := text(lock, "expiresAt"); expiresAt != "" {
			if parsed, err := time.Parse(time.RFC3339Nano, expiresAt); err == nil && time.Now().UTC().After(parsed) {
				continue
			}
		}
		if filepath.Clean(text(lock, "repositoryPath")) == cleanRepoPath {
			return lock
		}
	}
	return nil
}

func attachPagePilotRunToExecutionLock(ctx context.Context, server *Server, lock map[string]any, run map[string]any) map[string]any {
	if lock == nil {
		return nil
	}
	next := cloneMap(lock)
	next["pagePilotRunId"] = text(run, "id")
	next["workItemId"] = text(run, "workItemId")
	next["pipelineId"] = text(run, "pipelineId")
	next["runnerProcessState"] = "waiting-human"
	next["updatedAt"] = nowISO()
	_ = saveExecutionLock(ctx, server, next)
	return next
}

func releasePagePilotRunExecutionLock(ctx context.Context, server *Server, run map[string]any, reason string) error {
	if lock := mapValue(run["executionLock"]); text(lock, "id") != "" {
		return releasePagePilotExecutionLock(ctx, server, lock, reason)
	}
	scope := pagePilotLivePreviewExecutionScope(text(run, "repositoryTargetId"), text(run, "repositoryPath"))
	lock := existingExecutionLock(ctx, server, scope)
	if lock == nil || text(lock, "pagePilotRunId") != text(run, "id") {
		return nil
	}
	return releasePagePilotExecutionLock(ctx, server, lock, reason)
}

func releasePagePilotExecutionLock(ctx context.Context, server *Server, lock map[string]any, reason string) error {
	if lock == nil || text(lock, "id") == "" {
		return nil
	}
	next := cloneMap(lock)
	next["status"] = "released"
	next["runnerProcessState"] = stringOr(reason, "released")
	next["releasedAt"] = nowISO()
	next["updatedAt"] = nowISO()
	return saveExecutionLock(ctx, server, next)
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

func (server *Server) syncPagePilotRunRecords(ctx context.Context, run map[string]any, activeStageID string) error {
	pipelineID := text(run, "pipelineId")
	workItemID := text(run, "workItemId")
	if pipelineID == "" || workItemID == "" {
		return nil
	}
	database, err := mustLoad(server, ctx)
	if err != nil {
		return err
	}
	timestamp := nowISO()
	runID := text(run, "id")
	missionID := fmt.Sprintf("mission_%s_page_pilot", pipelineID)
	operationID := fmt.Sprintf("%s:page-pilot:%s", pipelineID, activeStageID)
	status := text(run, "status")
	operationStatus := "done"
	if status == "discarded" || status == "failed" {
		operationStatus = "failed"
	}
	database.Tables.Missions = appendOrReplace(database.Tables.Missions, map[string]any{
		"id":         missionID,
		"pipelineId": pipelineID,
		"workItemId": workItemID,
		"title":      "Page Pilot Agent",
		"status":     status,
		"mission": map[string]any{
			"id":                    missionID,
			"sourceWorkItemId":      workItemID,
			"title":                 "Page Pilot live-preview flow",
			"pagePilotRunId":        runID,
			"repositoryPath":        run["repositoryPath"],
			"repositoryTargetId":    run["repositoryTargetId"],
			"changedFiles":          run["changedFiles"],
			"primaryTarget":         run["primaryTarget"],
			"conversationBatch":     run["conversationBatch"],
			"processEvents":         run["processEvents"],
			"previewRuntimeProfile": run["previewRuntimeProfile"],
			"sourceMappingReport":   run["sourceMappingReport"],
			"sourceLocator":         run["sourceLocator"],
			"prPreview":             run["prPreview"],
			"visualProof":           run["visualProof"],
			"roundNumber":           run["roundNumber"],
		},
		"createdAt": stringOr(text(run, "createdAt"), timestamp),
		"updatedAt": timestamp,
	})
	database.Tables.Operations = appendOrReplace(database.Tables.Operations, map[string]any{
		"id":            operationID,
		"missionId":     missionID,
		"stageId":       activeStageID,
		"agentId":       "page-pilot",
		"status":        operationStatus,
		"prompt":        stringOr(text(run, "instruction"), "Page Pilot live-preview edit."),
		"requiredProof": []any{"selection-context", "source-patch", "diff-summary", "delivery-proof"},
		"runnerProcess": run["runnerProcess"],
		"summary":       pagePilotOperationSummary(run),
		"createdAt":     stringOr(text(run, "createdAt"), timestamp),
		"updatedAt":     timestamp,
	})
	for proofIndex, proof := range stringSlice(run["proofFiles"]) {
		database.Tables.ProofRecords = appendOrReplace(database.Tables.ProofRecords, map[string]any{
			"id":          fmt.Sprintf("%s:proof:%d", operationID, proofIndex+1),
			"operationId": operationID,
			"label":       "page-pilot-proof",
			"value":       filepath.Base(proof),
			"sourcePath":  proof,
			"createdAt":   timestamp,
		})
	}
	if prURL := text(run, "pullRequestUrl"); prURL != "" {
		database.Tables.ProofRecords = appendOrReplace(database.Tables.ProofRecords, map[string]any{
			"id":          fmt.Sprintf("%s:pull-request", operationID),
			"operationId": operationID,
			"label":       "page-pilot-pr",
			"value":       prURL,
			"sourceUrl":   prURL,
			"createdAt":   timestamp,
		})
	}
	for index, pipeline := range database.Tables.Pipelines {
		if text(pipeline, "id") != pipelineID {
			continue
		}
		next := cloneMap(pipeline)
		next["updatedAt"] = timestamp
		if status != "" {
			next["status"] = pagePilotPipelineStatus(status)
		}
		runRecord := mapValue(next["run"])
		runRecord["updatedAt"] = timestamp
		runRecord["status"] = next["status"]
		runRecord["events"] = appendPagePilotRunEvent(arrayMaps(runRecord["events"]), run, activeStageID, timestamp)
		artifacts := mapValue(runRecord["artifacts"])
		artifacts["selectionContext"] = run["selection"]
		artifacts["primaryTarget"] = run["primaryTarget"]
		artifacts["conversationBatch"] = run["conversationBatch"]
		artifacts["processEvents"] = run["processEvents"]
		artifacts["previewRuntimeProfile"] = run["previewRuntimeProfile"]
		artifacts["sourceMappingReport"] = run["sourceMappingReport"]
		artifacts["sourceLocator"] = run["sourceLocator"]
		artifacts["prPreview"] = run["prPreview"]
		artifacts["visualProof"] = run["visualProof"]
		artifacts["roundNumber"] = run["roundNumber"]
		runRecord["artifacts"] = artifacts
		stages := arrayMaps(runRecord["stages"])
		for stageIndex, stage := range stages {
			if text(stage, "id") != activeStageID {
				continue
			}
			stage["status"] = pagePilotStageStatus(status)
			stage["completedAt"] = timestamp
			stage["notes"] = pagePilotOperationSummary(run)
			stage["evidence"] = pagePilotEvidence(run)
			stages[stageIndex] = stage
		}
		runRecord["stages"] = stages
		next["run"] = runRecord
		database.Tables.Pipelines[index] = next
		break
	}
	touch(&database)
	return server.Repo.Save(ctx, database)
}

func pagePilotPipelineStatus(status string) string {
	switch status {
	case "applied":
		return "waiting-human"
	case "delivered":
		return "delivered"
	case "discarded":
		return "discarded"
	default:
		return status
	}
}

func pagePilotStageStatus(status string) string {
	switch status {
	case "applied", "delivered":
		return "passed"
	case "discarded":
		return "failed"
	default:
		return status
	}
}

func pagePilotEvidence(run map[string]any) []any {
	evidence := []any{}
	for _, proof := range stringSlice(run["proofFiles"]) {
		evidence = append(evidence, proof)
	}
	if prURL := text(run, "pullRequestUrl"); prURL != "" {
		evidence = append(evidence, prURL)
	}
	return evidence
}

func pagePilotOperationSummary(run map[string]any) string {
	status := stringOr(text(run, "status"), "updated")
	files := stringSlice(run["changedFiles"])
	if len(files) == 0 {
		return fmt.Sprintf("Page Pilot run %s.", status)
	}
	return fmt.Sprintf("Page Pilot run %s with %d changed file(s): %s.", status, len(files), strings.Join(files, ", "))
}

func appendPagePilotRunEvent(events []map[string]any, run map[string]any, stageID string, timestamp string) []map[string]any {
	eventID := fmt.Sprintf("event_%s_%s_%s", text(run, "id"), stageID, text(run, "status"))
	for _, event := range events {
		if text(event, "id") == eventID {
			return events
		}
	}
	return append(events, map[string]any{
		"id":        eventID,
		"type":      "page_pilot." + stringOr(text(run, "status"), "updated"),
		"message":   pagePilotOperationSummary(run),
		"timestamp": timestamp,
		"stageId":   stageID,
		"agentId":   "page-pilot",
	})
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

func buildPagePilotPrompt(repoPath string, repoLabel string, payload pagePilotApplyRequest, sourceMappingReport map[string]any, sourceLocator map[string]any) string {
	selectionBytes, _ := json.MarshalIndent(payload.Selection, "", "  ")
	reportBytes, _ := json.MarshalIndent(sourceMappingReport, "", "  ")
	locatorBytes, _ := json.MarshalIndent(sourceLocator, "", "  ")
	return fmt.Sprintf(`You are Omega Page Pilot, editing a live React preview through source metadata.

Repository: %s
Repository path: %s

User instruction:
%s

Selected element context:
%s

Source mapping coverage report:
%s

Source locator candidates:
%s

Rules:
- Work only inside this repository.
- Treat the selected element context above as the primary target. If the instruction contains multiple annotations, use them as supporting context unless the user explicitly says otherwise.
- Use the primary target source mapping first; do not rely on brittle DOM reverse lookup when a source mapping is available.
- If source mapping coverage is partial or DOM-only, inspect the source locator candidates before making changes. Prefer candidates with exact text or selector evidence.
- If no candidate has useful evidence, do not make broad unrelated edits; explain the missing mapping in the output note.
- Modify the real source file(s), not generated proof files.
- Keep the change minimal and consistent with light and dark themes.
- Do not commit, push, or open a pull request. Omega will handle delivery after user confirmation.
`, repoLabel, repoPath, payload.Instruction, string(selectionBytes), string(reportBytes), string(locatorBytes))
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

func pagePilotSubmittedAnnotations(batch map[string]any, annotations []map[string]any, selection pagePilotSelectionContext) []map[string]any {
	if len(annotations) > 0 {
		return annotations
	}
	if fromBatch := arrayMaps(mapValue(batch)["annotations"]); len(fromBatch) > 0 {
		return fromBatch
	}
	return []map[string]any{{
		"id":        1,
		"comment":   "",
		"selection": selectionRecord(selection),
	}}
}

func pagePilotConversationBatch(batch map[string]any, status string, runID string, instruction string, annotations []map[string]any) map[string]any {
	next := cloneMap(batch)
	if len(next) == 0 {
		next = map[string]any{
			"id":                  "page_pilot_batch_" + safeSegment(nowISO()),
			"createdAt":           nowISO(),
			"annotations":         annotations,
			"primaryAnnotationId": pagePilotPrimaryAnnotationID(annotations),
			"instruction":         instruction,
		}
	}
	if len(arrayMaps(next["annotations"])) == 0 {
		next["annotations"] = annotations
	}
	if text(next, "instruction") == "" {
		next["instruction"] = instruction
	}
	if next["primaryAnnotationId"] == nil {
		next["primaryAnnotationId"] = pagePilotPrimaryAnnotationID(arrayMaps(next["annotations"]))
	}
	next["status"] = status
	next["runId"] = runID
	next["updatedAt"] = nowISO()
	return next
}

func pagePilotProcessEvents(events []map[string]any) []map[string]any {
	if len(events) == 0 {
		return []map[string]any{}
	}
	return events
}

func pagePilotPreviewRuntimeProfile(profile map[string]any, repoPath string, repositoryTargetID string) map[string]any {
	next := cloneMap(profile)
	if len(next) == 0 {
		return map[string]any{}
	}
	next["repositoryTargetId"] = stringOr(text(next, "repositoryTargetId"), repositoryTargetID)
	next["workingDirectory"] = stringOr(text(next, "workingDirectory"), repoPath)
	next["capturedAt"] = nowISO()
	return next
}

func pagePilotPRPreview(payload any, changedFiles []string, diffSummary string, lineSummary string, target map[string]any) map[string]any {
	instruction := ""
	selector := ""
	source := ""
	switch typed := payload.(type) {
	case pagePilotApplyRequest:
		instruction = typed.Instruction
		selector = typed.Selection.StableSelector
		source = typed.Selection.SourceMapping.Source
	case pagePilotDeliverRequest:
		instruction = typed.Instruction
		selector = typed.Selection.StableSelector
		source = typed.Selection.SourceMapping.Source
	}
	title := "Omega Page Pilot: " + stringOr(compactCommitSubjectFromValues(source, selector, instruction), "selected page change")
	body := fmt.Sprintf("## Page Pilot change\n\n### Instruction\n%s\n\n### Target\n- Repository: %s\n- Source: `%s`\n- Selector: `%s`\n\n### Changed files\n%s\n\n### Summary\n%s\n\n### Line-level diff\n%s\n",
		strings.TrimSpace(instruction),
		repositoryTargetLabel(target),
		source,
		selector,
		markdownFileList(changedFiles),
		strings.TrimSpace(diffSummary),
		strings.TrimSpace(lineSummary),
	)
	return map[string]any{
		"title":           title,
		"body":            body,
		"changedFiles":    changedFiles,
		"lineDiffSummary": lineSummary,
		"generatedAt":     nowISO(),
	}
}

func compactCommitSubjectFromValues(source string, selector string, instruction string) string {
	for _, value := range []string{source, selector, instruction} {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		value = strings.ReplaceAll(value, "\n", " ")
		if len(value) > 80 {
			value = value[:80]
		}
		return value
	}
	return ""
}

func pagePilotVisualProof(selection pagePilotSelectionContext, annotations []map[string]any, changedFiles []string, diffSummary string, mode string) map[string]any {
	return map[string]any{
		"kind":            mode,
		"beforeSelection": selectionRecord(selection),
		"annotationCount": len(annotations),
		"changedFiles":    changedFiles,
		"diffSummary":     truncateForProof(diffSummary, 1600),
		"captureStrategy": "DOM context and style snapshot are captured by the target-page bridge; screenshot evidence can be attached by the desktop shell later.",
		"requiresRefresh": true,
		"generatedAt":     nowISO(),
	}
}

func pagePilotSourceMappingReport(annotations []map[string]any, fallback pagePilotSelectionContext) map[string]any {
	total := len(annotations)
	strong := 0
	domOnly := 0
	missingFile := 0
	for _, annotation := range annotations {
		selection := mapValue(annotation["selection"])
		sourceMapping := mapValue(selection["sourceMapping"])
		source := strings.TrimSpace(text(sourceMapping, "source"))
		file := strings.TrimSpace(text(sourceMapping, "file"))
		if source == "" || strings.EqualFold(source, "DOM-only") {
			domOnly++
		}
		if file == "" {
			missingFile++
			continue
		}
		strong++
	}
	if total == 0 {
		total = 1
		if fallback.SourceMapping.File != "" {
			strong = 1
		} else {
			domOnly = 1
			missingFile = 1
		}
	}
	return map[string]any{
		"totalSelections":       total,
		"strongSourceMappings":  strong,
		"domOnlySelections":     domOnly,
		"missingFileSelections": missingFile,
		"coverageRatio":         float64(strong) / float64(total),
		"status":                pagePilotSourceMappingStatus(strong, total),
		"generatedAt":           nowISO(),
	}
}

func pagePilotSourceMappingStatus(strong int, total int) string {
	if total <= 0 || strong == 0 {
		return "dom-only"
	}
	if strong == total {
		return "strong"
	}
	return "partial"
}

func pagePilotSourceLocatorReport(repoPath string, annotations []map[string]any, fallback pagePilotSelectionContext, sourceMappingReport map[string]any) map[string]any {
	status := text(sourceMappingReport, "status")
	if status == "strong" {
		return map[string]any{
			"status":      "not-needed",
			"reason":      "All selections include sourceMapping.file.",
			"generatedAt": nowISO(),
		}
	}
	results := []map[string]any{}
	for _, annotation := range annotations {
		selection := pagePilotSelectionFromRecord(mapValue(annotation["selection"]))
		if selection.SourceMapping.File != "" {
			continue
		}
		candidates := pagePilotSourceCandidates(repoPath, selection, text(annotation, "comment"))
		results = append(results, map[string]any{
			"annotationId": annotation["id"],
			"elementKind":  selection.ElementKind,
			"textSnapshot": selection.TextSnapshot,
			"selector":     selection.StableSelector,
			"status":       pagePilotSourceCandidateStatus(candidates),
			"candidates":   candidates,
		})
	}
	if len(results) == 0 && fallback.SourceMapping.File == "" {
		candidates := pagePilotSourceCandidates(repoPath, fallback, "")
		results = append(results, map[string]any{
			"annotationId": 1,
			"elementKind":  fallback.ElementKind,
			"textSnapshot": fallback.TextSnapshot,
			"selector":     fallback.StableSelector,
			"status":       pagePilotSourceCandidateStatus(candidates),
			"candidates":   candidates,
		})
	}
	locatorStatus := "candidates-ready"
	if len(results) == 0 {
		locatorStatus = "not-needed"
	}
	for _, result := range results {
		if text(result, "status") == "no-candidate" {
			locatorStatus = "needs-human-source-mapping"
			break
		}
	}
	return map[string]any{
		"status":      locatorStatus,
		"strategy":    "repo-text-and-selector-search",
		"results":     results,
		"generatedAt": nowISO(),
	}
}

func pagePilotSourceCandidateStatus(candidates []map[string]any) string {
	if len(candidates) == 0 {
		return "no-candidate"
	}
	return "candidate"
}

func pagePilotSourceCandidates(repoPath string, selection pagePilotSelectionContext, comment string) []map[string]any {
	needles := pagePilotLocatorNeedles(selection, comment)
	if len(needles) == 0 {
		return []map[string]any{}
	}
	candidates := []map[string]any{}
	_ = filepath.WalkDir(repoPath, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := entry.Name()
		if entry.IsDir() {
			if pagePilotIgnoredSourceDir(name) {
				return filepath.SkipDir
			}
			return nil
		}
		if !pagePilotSearchableSourceFile(name) {
			return nil
		}
		stat, statErr := entry.Info()
		if statErr != nil || stat.Size() > 512*1024 {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		content := string(raw)
		score := 0
		evidence := []string{}
		for _, needle := range needles {
			if strings.Contains(content, needle.value) {
				score += needle.score
				evidence = append(evidence, needle.reason)
			}
		}
		if score == 0 {
			return nil
		}
		relative, relErr := filepath.Rel(repoPath, path)
		if relErr != nil {
			relative = path
		}
		candidates = append(candidates, map[string]any{
			"file":     filepath.ToSlash(relative),
			"score":    score,
			"evidence": mergeUniqueStrings(evidence),
			"preview":  pagePilotCandidatePreview(content, needles),
		})
		return nil
	})
	sort.Slice(candidates, func(left int, right int) bool {
		leftScore, _ := candidates[left]["score"].(int)
		rightScore, _ := candidates[right]["score"].(int)
		if leftScore == rightScore {
			return text(candidates[left], "file") < text(candidates[right], "file")
		}
		return leftScore > rightScore
	})
	if len(candidates) > 5 {
		candidates = candidates[:5]
	}
	return candidates
}

type pagePilotLocatorNeedle struct {
	value  string
	score  int
	reason string
}

func pagePilotLocatorNeedles(selection pagePilotSelectionContext, comment string) []pagePilotLocatorNeedle {
	seen := map[string]bool{}
	add := func(value string, score int, reason string, needles *[]pagePilotLocatorNeedle) {
		value = strings.TrimSpace(value)
		if value == "" || len(value) < 2 || seen[value] {
			return
		}
		seen[value] = true
		*needles = append(*needles, pagePilotLocatorNeedle{value: value, score: score, reason: reason})
	}
	needles := []pagePilotLocatorNeedle{}
	add(selection.TextSnapshot, 10, "exact text snapshot", &needles)
	add(selection.StableSelector, 4, "stable selector", &needles)
	for _, token := range pagePilotSelectorTokens(selection.StableSelector) {
		add(token, 3, "selector token "+token, &needles)
	}
	add(text(selection.DOMContext, "tagName"), 1, "dom tag", &needles)
	for _, token := range pagePilotInstructionTokens(comment) {
		add(token, 1, "annotation token "+token, &needles)
	}
	return needles
}

func pagePilotSelectorTokens(selector string) []string {
	tokens := []string{}
	for _, part := range strings.FieldsFunc(selector, func(r rune) bool {
		return !(r == '-' || r == '_' || r == ':' || r == '.' || r == '#' || r == '"' || r == '\'' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'))
	}) {
		part = strings.Trim(part, ".#\"'")
		if len(part) >= 3 && !strings.HasPrefix(part, "data-omega-source") {
			tokens = append(tokens, part)
		}
	}
	return mergeUniqueStrings(tokens)
}

func pagePilotInstructionTokens(value string) []string {
	tokens := []string{}
	for _, part := range strings.FieldsFunc(value, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_')
	}) {
		if len(part) >= 4 {
			tokens = append(tokens, part)
		}
	}
	return mergeUniqueStrings(tokens)
}

func pagePilotIgnoredSourceDir(name string) bool {
	switch name {
	case ".git", ".omega", "node_modules", "dist", "build", ".next", "coverage", ".turbo":
		return true
	default:
		return false
	}
}

func pagePilotSearchableSourceFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".tsx", ".jsx", ".ts", ".js", ".vue", ".svelte", ".html", ".css", ".scss", ".md", ".mdx":
		return true
	default:
		return false
	}
}

func pagePilotCandidatePreview(content string, needles []pagePilotLocatorNeedle) string {
	for _, needle := range needles {
		index := strings.Index(content, needle.value)
		if index < 0 {
			continue
		}
		start := strings.LastIndex(content[:index], "\n")
		if start < 0 {
			start = 0
		} else {
			start++
		}
		end := strings.Index(content[index:], "\n")
		if end < 0 {
			end = len(content)
		} else {
			end = index + end
		}
		return strings.TrimSpace(content[start:end])
	}
	return ""
}

func pagePilotPrimaryTarget(batch map[string]any, selection pagePilotSelectionContext) map[string]any {
	primaryID := fmt.Sprint(batch["primaryAnnotationId"])
	for _, annotation := range arrayMaps(batch["annotations"]) {
		if fmt.Sprint(annotation["id"]) == primaryID {
			return map[string]any{
				"annotationId": annotation["id"],
				"comment":      annotation["comment"],
				"selection":    mapValue(annotation["selection"]),
			}
		}
	}
	return map[string]any{
		"annotationId": pagePilotPrimaryAnnotationID(arrayMaps(batch["annotations"])),
		"selection":    selectionRecord(selection),
	}
}

func pagePilotPrimaryAnnotationID(annotations []map[string]any) any {
	if len(annotations) == 0 {
		return 1
	}
	if annotations[0]["id"] != nil {
		return annotations[0]["id"]
	}
	return 1
}

func applyPagePilotConversationUpdate(run map[string]any, batch map[string]any, annotations []map[string]any, events []map[string]any, status string, runID string, instruction string, selection pagePilotSelectionContext) map[string]any {
	next := cloneMap(run)
	existingBatch := mapValue(next["conversationBatch"])
	if len(batch) > 0 {
		existingBatch = batch
	}
	mergedAnnotations := annotations
	if len(mergedAnnotations) == 0 {
		mergedAnnotations = arrayMaps(next["submittedAnnotations"])
	}
	if len(mergedAnnotations) == 0 {
		mergedAnnotations = pagePilotSubmittedAnnotations(existingBatch, nil, selection)
	}
	next["submittedAnnotations"] = mergedAnnotations
	next["conversationBatch"] = pagePilotConversationBatch(existingBatch, status, runID, instruction, mergedAnnotations)
	if len(events) > 0 {
		next["processEvents"] = events
	} else if next["processEvents"] == nil {
		next["processEvents"] = []map[string]any{}
	}
	next["primaryTarget"] = pagePilotPrimaryTarget(mapValue(next["conversationBatch"]), selection)
	return next
}

func pagePilotSelectionFromRecord(record map[string]any) pagePilotSelectionContext {
	source := mapValue(record["sourceMapping"])
	return pagePilotSelectionContext{
		ElementKind:    text(record, "elementKind"),
		StableSelector: text(record, "stableSelector"),
		TextSnapshot:   text(record, "textSnapshot"),
		StyleSnapshot:  stringMap(record["styleSnapshot"]),
		DOMContext:     mapValue(record["domContext"]),
		SourceMapping: pagePilotSourceMapping{
			Source: text(source, "source"),
			File:   text(source, "file"),
			Symbol: text(source, "symbol"),
			Line:   intNumber(source["line"]),
		},
	}
}

func stringMap(value any) map[string]string {
	if typed, ok := value.(map[string]string); ok {
		return typed
	}
	output := map[string]string{}
	for key, raw := range mapValue(value) {
		output[key] = fmt.Sprint(raw)
	}
	return output
}

func intNumber(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		number, _ := typed.Int64()
		return int(number)
	default:
		return 0
	}
}

func applyPagePilotLocalPatch(repoPath string, payload pagePilotApplyRequest, sourceLocator map[string]any) (map[string]any, error) {
	sourceFile := pagePilotEffectiveSourceFile(payload.Selection.SourceMapping.File, sourceLocator)
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

func pagePilotEffectiveSourceFile(sourceFile string, sourceLocator map[string]any) string {
	sourceFile = strings.TrimSpace(sourceFile)
	if sourceFile != "" {
		return sourceFile
	}
	for _, result := range arrayMaps(sourceLocator["results"]) {
		candidates := arrayMaps(result["candidates"])
		if len(candidates) > 0 {
			return text(candidates[0], "file")
		}
	}
	return ""
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
