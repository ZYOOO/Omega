package omegalocal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type devFlowRecordAgentFunc func(stageID string, agentID string, status string, prompt string, artifact string, summary string, proofFiles []string, process map[string]any)

type devFlowReworkActionHandler struct {
	server                  *Server
	ctx                     context.Context
	template                *PipelineTemplate
	profile                 ProjectAgentProfile
	masterProfile           AgentProfileConfig
	masterRunner            AgentRunner
	masterRunnerID          string
	codingProfile           AgentProfileConfig
	codingRunner            AgentRunner
	codingRunnerID          string
	testingProfile          AgentProfileConfig
	testingRunner           AgentRunner
	testingRunnerID         string
	runnerHeartbeatInterval time.Duration
	pipeline                map[string]any
	item                    map[string]any
	currentAttempt          map[string]any
	proofDir                string
	repoWorkspace           string
	repoSlug                string
	branchName              string
	prURL                   string
	prTitle                 string
	effectiveDescription    string
	attemptID               string
	humanChangeRequest      string
	promptVariables         map[string]string
	stageID                 string
	fromStageID             string
	cycle                   int
	feedback                string
	recordAgent             devFlowRecordAgentFunc

	roundFeedback string
	reworkPrompt  string
	notePath      string
	promptPath    string

	commitSha           *string
	commitSummary       *string
	diffText            *string
	changedNames        *string
	changedFiles        *[]string
	testOutput          *string
	testErr             *error
	prDiff              *string
	checksOutput        *string
	remoteChecks        *[]map[string]any
	remoteChecksRaw     *string
	remoteCheckSummary  *map[string]any
	pullRequestFeedback *[]map[string]any
	checkLogFeedback    *[]map[string]any
}

func newDevFlowReworkActionHandler(input devFlowReworkActionHandler) *devFlowReworkActionHandler {
	input.notePath = filepath.Join(input.proofDir, fmt.Sprintf("rework-agent-note-%d.md", input.cycle))
	input.promptPath = filepath.Join(input.proofDir, fmt.Sprintf("rework-prompt-%d.md", input.cycle))
	return &input
}

func (handler *devFlowReworkActionHandler) contractSteps() []devFlowContractActionStep {
	return []devFlowContractActionStep{
		{ID: "build_rework_checklist", Type: "build_rework_checklist", Agent: "master", Run: handler.buildChecklist},
		{ID: "apply_rework", Type: "run_agent", Agent: "coding", Run: handler.apply},
		{ID: "validate_rework", Type: "run_validation", Agent: "testing", Run: handler.validate},
		{ID: "update_pull_request", Type: "ensure_pr", Agent: "delivery", Run: handler.updatePullRequest},
		{ID: "collect_rework_ci_results", Type: "run_ci_checks", Agent: "testing", Run: handler.collectCI},
	}
}

func (handler *devFlowReworkActionHandler) buildChecklist() error {
	handler.roundFeedback = strings.TrimSpace(handler.feedback)
	if checklistPrompt := reworkChecklistPromptFromAttempt(handler.currentAttempt, ""); checklistPrompt != "" {
		handler.roundFeedback = strings.TrimSpace(checklistPrompt + "\n\nLatest review feedback:\n" + handler.feedback)
	}
	checklistPath := filepath.Join(handler.proofDir, fmt.Sprintf("rework-checklist-%d.md", handler.cycle))
	checklist := fmt.Sprintf("# Rework Checklist %d\n\n- Source stage: `%s`\n- Target stage: `%s`\n- Pull request: %s\n\n## Feedback\n\n%s\n", handler.cycle, handler.fromStageID, handler.stageID, handler.prURL, stringOr(handler.roundFeedback, "No review feedback was provided."))
	if err := os.WriteFile(checklistPath, []byte(checklist), 0o644); err != nil {
		return err
	}
	masterPrompt := fmt.Sprintf(`You are the Master Orchestrator Agent for Omega.

Build the rework checklist for the next coding pass.

Repository: %s
Repository path: %s
Work item: %s
Title: %s
Pull request: %s
Review cycle: %d
Source stage: %s
Target stage: %s

Requirement:
%s

Feedback:
%s

Omega has written the machine-readable checklist draft to:
%s

Prepare the final checklist with:
- exact feedback items to address
- expected files or areas to inspect
- validation required before review
- risks or blocked assumptions
`, handler.repoSlug, handler.repoWorkspace, text(handler.item, "key"), text(handler.item, "title"), handler.prURL, handler.cycle, handler.fromStageID, handler.stageID, handler.effectiveDescription, handler.roundFeedback, checklistPath) + agentArtifactCaptureInstruction(checklistPath) + "\n\n" + agentPolicyBlock(handler.profile, "master")
	if err := writeRunnerPolicyFiles(handler.repoWorkspace, handler.profile, "master"); err != nil {
		process := runnerProcessNotAvailable(handler.masterRunnerID, handler.masterRunnerID, handler.repoWorkspace, err)
		handler.recordAgent(handler.stageID, "master", "failed", masterPrompt, filepath.Base(checklistPath), "Master agent policy files could not be materialized for rework checklist.", []string{checklistPath}, process)
		return err
	}
	masterModel, masterEnv := handler.server.runnerCredentialModelAndEnv(handler.ctx, handler.masterRunnerID, handler.masterProfile.Model)
	turn := handler.masterRunner.RunTurn(handler.ctx, AgentTurnRequest{
		Role:              "master",
		StageID:           handler.stageID,
		Runner:            handler.masterRunnerID,
		Workspace:         handler.repoWorkspace,
		Prompt:            masterPrompt,
		OutputPath:        checklistPath,
		Sandbox:           "read-only",
		Model:             masterModel,
		Effort:            "medium",
		Env:               mergeEnvMaps(agentCapabilityEnv(handler.profile, "master"), masterEnv),
		HeartbeatInterval: handler.runnerHeartbeatInterval,
		OnProcessEvent:    handler.server.runnerHeartbeatRecorder(text(handler.pipeline, "id"), text(handler.item, "id"), handler.attemptID, handler.stageID, "master", handler.masterRunnerID),
	})
	process := turn.Process
	if process == nil {
		process = map[string]any{"runner": handler.masterRunnerID, "status": "passed"}
	}
	process["reviewCycle"] = handler.cycle
	if turn.Error != nil {
		handler.recordAgent(handler.stageID, "master", "failed", masterPrompt, filepath.Base(checklistPath), "Master agent failed while building the rework checklist.", []string{checklistPath}, process)
		return turn.Error
	}
	handler.recordAgent(handler.stageID, "master", "passed", masterPrompt, filepath.Base(checklistPath), "Master agent captured the rework checklist for the next coding pass.", []string{checklistPath}, process)
	return nil
}

func (handler *devFlowReworkActionHandler) apply() error {
	if strings.TrimSpace(handler.roundFeedback) == "" {
		handler.roundFeedback = strings.TrimSpace(handler.feedback)
	}
	reworkVariables := cloneStringMap(handler.promptVariables)
	reworkVariables["pullRequestUrl"] = handler.prURL
	reworkVariables["reviewFeedback"] = handler.roundFeedback
	reworkVariables["reworkNotePath"] = handler.notePath
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
`, handler.repoSlug, handler.repoWorkspace, text(handler.item, "key"), text(handler.item, "title"), handler.prURL, handler.effectiveDescription, handler.roundFeedback, handler.notePath)
	handler.reworkPrompt = renderWorkflowPromptSection(handler.template, "rework", reworkVariables, reworkFallback) + "\n\n" + agentPolicyBlock(handler.profile, "coding")
	if err := os.WriteFile(handler.promptPath, []byte(handler.reworkPrompt), 0o644); err != nil {
		return err
	}
	if err := writeRunnerPolicyFiles(handler.repoWorkspace, handler.profile, "coding"); err != nil {
		return err
	}
	handler.recordAgent(handler.stageID, "coding", "running", handler.reworkPrompt, "", "Rework agent is applying review feedback in the same workspace.", []string{handler.promptPath}, map[string]any{"runner": handler.codingRunnerID, "status": "running", "reviewCycle": handler.cycle})
	reworkModel, reworkEnv := handler.server.runnerCredentialModelAndEnv(handler.ctx, handler.codingRunnerID, handler.codingProfile.Model)
	turn := handler.codingRunner.RunTurn(handler.ctx, AgentTurnRequest{
		Role:              "rework",
		StageID:           handler.stageID,
		Runner:            handler.codingRunnerID,
		Workspace:         handler.repoWorkspace,
		Prompt:            handler.reworkPrompt,
		OutputPath:        handler.notePath,
		Sandbox:           "workspace-write",
		Model:             reworkModel,
		Env:               mergeEnvMaps(agentCapabilityEnv(handler.profile, "coding"), reworkEnv),
		HeartbeatInterval: handler.runnerHeartbeatInterval,
		OnProcessEvent:    handler.server.runnerHeartbeatRecorder(text(handler.pipeline, "id"), text(handler.item, "id"), handler.attemptID, handler.stageID, "coding", handler.codingRunnerID),
	})
	if turn.Error != nil {
		handler.recordAgent(handler.stageID, "coding", "failed", handler.reworkPrompt, filepath.Base(handler.notePath), "Rework agent failed before producing an acceptable repository diff.", []string{handler.promptPath, handler.notePath}, turn.Process)
		return fmt.Errorf("rework agent failed: %w", turn.Error)
	}
	statusOutput, err := runCommand(handler.repoWorkspace, "git", "status", "--short", "--", ".", ":(exclude).omega", ":(exclude).codex", ":(exclude).claude", ":(exclude).opencode", ":(exclude).trae")
	if err != nil {
		return fmt.Errorf("read rework changes: %w", err)
	}
	if strings.TrimSpace(statusOutput) == "" {
		handler.recordAgent(handler.stageID, "coding", "failed", handler.reworkPrompt, filepath.Base(handler.notePath), "Rework agent produced no repository changes.", []string{handler.promptPath, handler.notePath}, turn.Process)
		return errors.New("rework agent produced no repository changes")
	}
	if _, err := runCommand(handler.repoWorkspace, "git", "add", "-A", "--", ".", ":(exclude).omega", ":(exclude).codex", ":(exclude).claude", ":(exclude).opencode", ":(exclude).trae"); err != nil {
		return fmt.Errorf("stage rework changes: %w", err)
	}
	if _, err := runCommand(handler.repoWorkspace, "git", "commit", "-m", fmt.Sprintf("Omega rework for %s round %d", text(handler.item, "key"), handler.cycle)); err != nil {
		return fmt.Errorf("commit rework changes: %w", err)
	}
	*handler.commitSha, _ = runCommand(handler.repoWorkspace, "git", "rev-parse", "HEAD")
	*handler.commitSummary, _ = runCommand(handler.repoWorkspace, "git", "show", "--stat", "--oneline", "--no-renames", "HEAD")
	var changedNames string
	*handler.diffText, changedNames, err = gitHeadDiff(handler.repoWorkspace)
	if err != nil {
		return fmt.Errorf("list rework changed files: %w", err)
	}
	*handler.changedNames = changedNames
	*handler.changedFiles = uniqueStrings(append(*handler.changedFiles, compactLines(changedNames)...))
	diffPath := filepath.Join(handler.proofDir, fmt.Sprintf("git-diff-rework-%d.patch", handler.cycle))
	if err := os.WriteFile(diffPath, []byte(*handler.diffText), 0o644); err != nil {
		return err
	}
	reworkSummaryPath := filepath.Join(handler.proofDir, fmt.Sprintf("rework-summary-%d.md", handler.cycle))
	reworkSummary := fmt.Sprintf("# Rework Round %d\n\n- Branch: `%s`\n- Commit: `%s`\n- Changed files:\n%s\n```text\n%s\n```\n", handler.cycle, handler.branchName, strings.TrimSpace(*handler.commitSha), markdownFileList(compactLines(changedNames)), truncateForProof(*handler.commitSummary, 4000))
	if err := os.WriteFile(reworkSummaryPath, []byte(reworkSummary), 0o644); err != nil {
		return err
	}
	handler.recordAgent(handler.stageID, "coding", "passed", handler.reworkPrompt, filepath.Base(reworkSummaryPath), fmt.Sprintf("Rework agent produced %d changed file(s).", len(compactLines(changedNames))), []string{handler.promptPath, handler.notePath, reworkSummaryPath, diffPath}, turn.Process)
	return nil
}

func (handler *devFlowReworkActionHandler) validate() error {
	*handler.testOutput, *handler.testErr = runRepositoryValidation(handler.repoWorkspace)
	testStatus := "passed"
	if *handler.testErr != nil {
		testStatus = "failed"
	}
	testReportPath := filepath.Join(handler.proofDir, fmt.Sprintf("test-report-rework-%d.md", handler.cycle))
	testVariables := cloneStringMap(handler.promptVariables)
	testVariables["changedFiles"] = strings.Join(*handler.changedFiles, ", ")
	testVariables["testOutput"] = *handler.testOutput
	testFallback := fmt.Sprintf("Validate %s after rework round %d. Changed files: %s", text(handler.item, "key"), handler.cycle, strings.Join(*handler.changedFiles, ", "))
	testPrompt := renderWorkflowPromptSection(handler.template, "testing", testVariables, testFallback) + "\n\n" + agentPolicyBlock(handler.profile, "testing") + fmt.Sprintf(`

Local repository validation has already run for rework round %d. Use the output below as evidence and prepare the final validation report.

Validation status: %s

Validation output:
~~~text
%s
~~~

Validation failure summary:
%s
`, handler.cycle, testStatus, stringOr(strings.TrimSpace(*handler.testOutput), "No validation output."), stringOr(testFailureSummary(*handler.testErr, *handler.testOutput), "None")) + agentArtifactCaptureInstruction(testReportPath)
	if err := writeRunnerPolicyFiles(handler.repoWorkspace, handler.profile, "testing"); err != nil {
		return err
	}
	testModel, testEnv := handler.server.runnerCredentialModelAndEnv(handler.ctx, handler.testingRunnerID, handler.testingProfile.Model)
	turn := handler.testingRunner.RunTurn(handler.ctx, AgentTurnRequest{
		Role:              "testing",
		StageID:           handler.stageID,
		Runner:            handler.testingRunnerID,
		Workspace:         handler.repoWorkspace,
		Prompt:            testPrompt,
		OutputPath:        testReportPath,
		Sandbox:           "read-only",
		Model:             testModel,
		Effort:            "medium",
		Env:               mergeEnvMaps(agentCapabilityEnv(handler.profile, "testing"), testEnv),
		HeartbeatInterval: handler.runnerHeartbeatInterval,
		OnProcessEvent:    handler.server.runnerHeartbeatRecorder(text(handler.pipeline, "id"), text(handler.item, "id"), handler.attemptID, handler.stageID, "testing", handler.testingRunnerID),
	})
	if turn.Error != nil {
		handler.recordAgent(handler.stageID, "testing", "failed", testPrompt, filepath.Base(testReportPath), "Testing agent failed while verifying rework validation output.", []string{testReportPath}, turn.Process)
		return fmt.Errorf("testing agent failed after rework: %w", turn.Error)
	}
	if reportRaw, _ := os.ReadFile(testReportPath); strings.TrimSpace(string(reportRaw)) == "" {
		testReport := fmt.Sprintf("# Rework Test Report\n\nStatus: %s\n\n## Commands\n\n```text\n%s\n```\n\n## Acceptance coverage\n\n- Validation was run against the repository after rework round %d.\n\n## Failures\n\n%s\n\n## Residual risk\n\n- Project-specific coverage depends on available repository test commands.\n", testStatus, stringOr(strings.TrimSpace(*handler.testOutput), "No validation output."), handler.cycle, stringOr(testFailureSummary(*handler.testErr, *handler.testOutput), "None"))
		if err := os.WriteFile(testReportPath, []byte(testReport), 0o644); err != nil {
			return err
		}
	}
	testProcess := cloneMap(turn.Process)
	testProcess["localValidation"] = map[string]any{"runner": "local-validation", "status": testStatus, "stdout": *handler.testOutput}
	handler.recordAgent(handler.stageID, "testing", testStatus, testPrompt, filepath.Base(testReportPath), "Testing agent verified repository validation output after rework.", []string{testReportPath}, testProcess)
	if *handler.testErr != nil {
		return fmt.Errorf("repository validation failed after rework: %w", *handler.testErr)
	}
	return nil
}

func (handler *devFlowReworkActionHandler) updatePullRequest() error {
	if err := pushDevFlowBranch(handler.repoWorkspace, handler.branchName); err != nil {
		return fmt.Errorf("push rework branch: %w", err)
	}
	reworkPRBody := buildDevFlowPullRequestBody(handler.item, *handler.changedFiles, *handler.testOutput, stringOr(handler.feedback, handler.humanChangeRequest), *handler.diffText)
	if output, err := updateDevFlowPullRequestDescriptionIfChanged(handler.repoWorkspace, handler.prURL, handler.prTitle, reworkPRBody); err != nil {
		handler.server.logDebug(handler.ctx, "github.pr.description_update_skipped", "Pull request description update skipped after rework.", map[string]any{"pipelineId": text(handler.pipeline, "id"), "attemptId": handler.attemptID, "pullRequestUrl": handler.prURL, "output": truncateForProof(output+"\n"+err.Error(), 1200)})
	} else {
		handler.server.logInfo(handler.ctx, "github.pr.description_updated", "Pull request description updated after rework.", map[string]any{"pipelineId": text(handler.pipeline, "id"), "attemptId": handler.attemptID, "pullRequestUrl": handler.prURL, "output": truncateForProof(output, 1200)})
	}
	*handler.prDiff, _ = runCommand(handler.repoWorkspace, "gh", "pr", "diff", handler.prURL)
	*handler.checksOutput, _ = runCommand(handler.repoWorkspace, "gh", "pr", "checks", handler.prURL)
	*handler.remoteChecks, *handler.remoteChecksRaw = githubPullRequestChecks(handler.ctx, handler.repoWorkspace, handler.prURL, handler.repoSlug)
	if strings.TrimSpace(*handler.remoteChecksRaw) != "" {
		*handler.checksOutput = *handler.remoteChecksRaw
	}
	*handler.remoteCheckSummary = githubCheckSummaryWithRequired(*handler.remoteChecks, handler.template.Runtime.RequiredChecks)
	*handler.pullRequestFeedback = githubPullRequestFeedback(handler.ctx, handler.repoWorkspace, handler.prURL, handler.repoSlug)
	*handler.checkLogFeedback = githubPullRequestCheckLogFeedback(handler.ctx, handler.repoWorkspace, handler.prURL, handler.repoSlug, *handler.remoteChecks)
	return nil
}

func (handler *devFlowReworkActionHandler) collectCI() error {
	ciResult := collectDevFlowGitHubActionsCI(handler.ctx, handler.repoWorkspace, handler.repoSlug, handler.prURL, handler.template.Runtime.RequiredChecks, handler.proofDir, fmt.Sprintf("ci-checks-rework-%d.md", handler.cycle))
	*handler.checksOutput = ciResult.ChecksOutput
	*handler.remoteChecks = ciResult.RemoteChecks
	*handler.remoteChecksRaw = ciResult.RemoteChecksRaw
	*handler.remoteCheckSummary = ciResult.CheckSummary
	*handler.checkLogFeedback = ciResult.CheckLogFeedback
	artifact := ""
	proofFiles := []string{}
	if strings.TrimSpace(ciResult.ReportPath) != "" {
		artifact = filepath.Base(ciResult.ReportPath)
		proofFiles = append(proofFiles, ciResult.ReportPath)
	}
	handler.recordAgent(handler.stageID, "testing", "passed", "Collect GitHub Actions CI checks and failed check logs after rework.", artifact, ciResult.Summary, proofFiles, map[string]any{"runner": "github-actions", "status": ciResult.Status, "checkSummary": ciResult.CheckSummary, "checkLogFeedback": ciResult.CheckLogFeedback, "reviewCycle": handler.cycle})
	return nil
}
