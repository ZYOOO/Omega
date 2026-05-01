package omegalocal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWorkspaceAndWorkItemAPI(t *testing.T) {
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)

	item := map[string]any{
		"id": "item_manual_1", "key": "OMG-1", "title": "Go item", "description": "Created by Go service.",
		"status": "Ready", "priority": "High", "assignee": "requirement", "labels": []any{"manual"}, "team": "Omega", "stageId": "intake", "target": "No target",
	}
	response := postJSON(t, api.URL+"/work-items", map[string]any{"item": item})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("create item status = %d", response.StatusCode)
	}

	patch := requestJSON(t, http.MethodPatch, api.URL+"/work-items/item_manual_1", map[string]any{"status": "Done", "priority": "Urgent"})
	if patch.StatusCode != http.StatusOK {
		t.Fatalf("patch item status = %d", patch.StatusCode)
	}
	var database WorkspaceDatabase
	decode(t, patch, &database)
	if got := database.Tables.WorkItems[0]["status"]; got != "Done" {
		t.Fatalf("patched status = %v", got)
	}
	if got := database.Tables.WorkItems[0]["source"]; got != "manual" {
		t.Fatalf("default source = %v", got)
	}
	if got := arrayValues(database.Tables.WorkItems[0]["acceptanceCriteria"]); len(got) == 0 {
		t.Fatalf("default acceptance criteria missing: %+v", database.Tables.WorkItems[0])
	}
	if len(database.Tables.Requirements) != 1 {
		t.Fatalf("requirement count = %d", len(database.Tables.Requirements))
	}
	requirement := database.Tables.Requirements[0]
	if requirement["title"] != "Go item" || requirement["source"] != "manual" || requirement["status"] != "converted" {
		t.Fatalf("requirement = %+v", requirement)
	}
	if database.Tables.WorkItems[0]["requirementId"] != requirement["id"] {
		t.Fatalf("item requirement link = item:%+v requirement:%+v", database.Tables.WorkItems[0], requirement)
	}

	var requirements []map[string]any
	decode(t, mustGet(t, api.URL+"/requirements"), &requirements)
	if len(requirements) != 1 || requirements[0]["id"] != requirement["id"] {
		t.Fatalf("requirements endpoint = %+v", requirements)
	}
}

func TestCreateWorkItemInitializesEmptyWorkspace(t *testing.T) {
	api, _ := newTestAPI(t)

	item := map[string]any{
		"id": "item_manual_1", "key": "OMG-1", "title": "Initialize workspace", "description": "Created before a workspace snapshot exists.",
		"status": "Ready", "priority": "Medium", "assignee": "requirement", "labels": []any{"manual"}, "team": "Omega", "stageId": "intake", "target": "No target",
	}
	response := postJSON(t, api.URL+"/work-items", map[string]any{"item": item})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("create item status = %d", response.StatusCode)
	}

	var database WorkspaceDatabase
	decode(t, response, &database)
	if len(database.Tables.Projects) != 1 || database.Tables.Projects[0]["id"] != "project_omega" {
		t.Fatalf("default project missing: %+v", database.Tables.Projects)
	}
	if len(database.Tables.WorkItems) != 1 || database.Tables.WorkItems[0]["key"] != "OMG-1" {
		t.Fatalf("created work item missing: %+v", database.Tables.WorkItems)
	}
	if len(database.Tables.Requirements) != 1 || database.Tables.WorkItems[0]["requirementId"] != database.Tables.Requirements[0]["id"] {
		t.Fatalf("requirement link missing: items=%+v requirements=%+v", database.Tables.WorkItems, database.Tables.Requirements)
	}
}

func TestDeleteNotStartedWorkItemRemovesUnsharedRequirement(t *testing.T) {
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)

	item := map[string]any{
		"id": "item_manual_delete", "key": "OMG-18", "title": "Delete stale item", "description": "Created by mistake.",
		"status": "Ready", "priority": "Medium", "assignee": "requirement", "labels": []any{"manual"}, "team": "Omega", "stageId": "intake", "target": "No target",
	}
	create := postJSON(t, api.URL+"/work-items", map[string]any{"item": item})
	if create.StatusCode != http.StatusOK {
		t.Fatalf("create item status = %d", create.StatusCode)
	}
	remove := requestJSON(t, http.MethodDelete, api.URL+"/work-items/item_manual_delete", nil)
	if remove.StatusCode != http.StatusOK {
		t.Fatalf("delete item status = %d", remove.StatusCode)
	}
	var database WorkspaceDatabase
	decode(t, remove, &database)
	if len(database.Tables.WorkItems) != 0 {
		t.Fatalf("work item was not deleted: %+v", database.Tables.WorkItems)
	}
	if len(database.Tables.Requirements) != 0 {
		t.Fatalf("unshared requirement was not deleted: %+v", database.Tables.Requirements)
	}
	stateItems := arrayMaps(database.Tables.MissionControlStates[0]["workItems"])
	if len(stateItems) != 0 {
		t.Fatalf("mission state work item projection was not deleted: %+v", stateItems)
	}
}

func TestCreateWorkItemStoresMasterRequirementDispatch(t *testing.T) {
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)

	item := map[string]any{
		"id": "item_manual_dispatch", "key": "OMG-77", "title": "Add dashboard export", "description": "Users need an export button with CSV output and proof.",
		"status": "Ready", "priority": "High", "assignee": "requirement", "labels": []any{"manual"}, "team": "Omega", "stageId": "intake", "target": "https://github.com/ZYOOO/TestRepo", "repositoryTargetId": "repo_ZYOOO_TestRepo",
	}
	response := postJSON(t, api.URL+"/work-items", map[string]any{"item": item})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("create item status = %d", response.StatusCode)
	}

	var database WorkspaceDatabase
	decode(t, response, &database)
	if len(database.Tables.Requirements) != 1 {
		t.Fatalf("requirement count = %d", len(database.Tables.Requirements))
	}
	requirement := database.Tables.Requirements[0]
	structured := mapValue(requirement["structured"])
	if structured["masterAgentId"] != "master" || structured["dispatchStatus"] != "ready" {
		t.Fatalf("requirement dispatch metadata missing: %+v", structured)
	}
	if len(arrayMaps(structured["suggestedWorkItems"])) < 3 {
		t.Fatalf("suggested work items missing: %+v", structured)
	}
	if len(anySlice(requirement["acceptanceCriteria"])) < 3 {
		t.Fatalf("structured acceptance criteria missing: %+v", requirement)
	}
}

func TestPipelineTemplateIncludesAgentContractsDependenciesAndDataFlow(t *testing.T) {
	template := findPipelineTemplate("feature")
	if template == nil {
		t.Fatal("feature template missing")
	}
	pipeline := makePipelineWithTemplate(map[string]any{
		"id": "item_manual_88", "key": "OMG-88", "title": "Ship feature", "description": "Build it", "assignee": "requirement", "requirementId": "req_item_manual_88",
	}, template)
	run := mapValue(pipeline["run"])
	agents := arrayMaps(run["agents"])
	if len(agents) < 6 {
		t.Fatalf("agents should be materialized into pipeline run: %+v", agents)
	}
	firstStage := arrayMaps(run["stages"])[0]
	secondStage := arrayMaps(run["stages"])[1]
	if len(anySlice(firstStage["outputArtifacts"])) == 0 || len(anySlice(secondStage["inputArtifacts"])) == 0 {
		t.Fatalf("stage artifact contracts missing: first=%+v second=%+v", firstStage, secondStage)
	}
	dependencies := anySlice(secondStage["dependsOn"])
	if len(dependencies) != 1 || dependencies[0] != firstStage["id"] {
		t.Fatalf("stage dependency missing: second=%+v", secondStage)
	}
	if len(arrayMaps(run["dataFlow"])) < len(arrayMaps(run["stages"]))-1 {
		t.Fatalf("data flow edges missing: %+v", run["dataFlow"])
	}
	orchestrator := mapValue(run["orchestrator"])
	if orchestrator["masterAgentId"] != "master" {
		t.Fatalf("master orchestrator missing: %+v", orchestrator)
	}
}

func TestDevFlowTemplateLoadsWorkflowMarkdownContract(t *testing.T) {
	template := findPipelineTemplate("devflow-pr")
	if template == nil {
		t.Fatal("devflow-pr template missing")
	}
	if template.Source == "" || !strings.HasSuffix(template.Source, filepath.Join("services", "local-runtime", "workflows", "devflow-pr.md")) {
		t.Fatalf("devflow-pr should load from workflow markdown, got source=%q", template.Source)
	}
	if len(template.StageProfiles) != 8 {
		t.Fatalf("stage profiles = %+v", template.StageProfiles)
	}
	if len(template.StateProfiles) != 8 {
		t.Fatalf("state profiles should come from workflow action graph: %+v", template.StateProfiles)
	}
	actionPlan := workflowActionPlan(template)
	if len(actionPlan) < 12 || workflowExecutionMode(template) != "contract-action-plan" {
		t.Fatalf("workflow action plan missing: mode=%s actions=%+v", workflowExecutionMode(template), actionPlan)
	}
	if len(template.TaskClasses) != 2 || template.TaskClasses[0].ID != "simple" || template.TaskClasses[1].WorkpadMode != "full" {
		t.Fatalf("workflow task classes missing: %+v", template.TaskClasses)
	}
	implementation := template.StageProfiles[1]
	if implementation.ID != "in_progress" || strings.Join(implementation.AgentIDs, ",") != "architect,coding,testing" {
		t.Fatalf("implementation agents should come from workflow markdown: %+v", implementation)
	}
	if len(template.ReviewRounds) != 2 || template.ReviewRounds[0].Focus != "correctness, regressions, and acceptance criteria" || template.ReviewRounds[1].DiffSource != "pr_diff" {
		t.Fatalf("review rounds should come from workflow markdown: %+v", template.ReviewRounds)
	}
	if template.Runtime.MaxReviewCycles != 3 || template.Runtime.RunnerHeartbeatSeconds != 10 || template.Runtime.AttemptTimeoutMinutes != 30 || template.Runtime.MaxRetryAttempts != 2 || template.Runtime.RetryBackoffSeconds != 300 || template.Runtime.CleanupRetentionSeconds != 86400 || template.Runtime.MaxContinuationTurns != 2 || len(template.Transitions) == 0 {
		t.Fatalf("workflow runtime/transitions should come from markdown: runtime=%+v transitions=%+v", template.Runtime, template.Transitions)
	}
	for _, section := range []string{"requirement", "architect", "coding", "testing", "rework", "review", "delivery"} {
		if !strings.Contains(template.PromptSections[section], "{{") {
			t.Fatalf("workflow prompt section %s missing variables: prompts=%+v", section, template.PromptSections)
		}
	}
	if !validateWorkflowTemplate(*template).ok() || !strings.Contains(template.PromptSections["coding"], "{{repositoryPath}}") || !strings.Contains(template.PromptSections["review"], "Blocking findings") {
		t.Fatalf("workflow validation/prompt sections missing: validation=%+v prompts=%+v", validateWorkflowTemplate(*template), template.PromptSections)
	}
	pipeline := makePipelineWithTemplate(map[string]any{
		"id": "item_manual_89", "key": "OMG-89", "title": "Ship feature", "description": "Build it", "assignee": "requirement", "requirementId": "req_item_manual_89",
	}, template)
	run := mapValue(pipeline["run"])
	workflow := mapValue(run["workflow"])
	if workflow["source"] == "" || len(arrayMaps(workflow["reviewRounds"])) != 2 || len(arrayMaps(workflow["actions"])) == 0 || text(workflow, "executionMode") != "contract-action-plan" {
		t.Fatalf("pipeline run should preserve workflow metadata: %+v", workflow)
	}
	if len(arrayMaps(workflow["taskClasses"])) != 2 || len(arrayMaps(workflow["states"])) != 8 {
		t.Fatalf("pipeline run should preserve workflow state/action policy: %+v", workflow)
	}
	stages := arrayMaps(run["stages"])
	if got := anySlice(stages[1]["agentIds"]); len(got) != 3 || got[0] != "architect" || got[2] != "testing" {
		t.Fatalf("pipeline stages did not preserve workflow agents: %+v", stages[1])
	}
	if got := anySlice(stages[1]["outputArtifacts"]); len(got) == 0 || got[len(got)-1] != "pull-request" {
		t.Fatalf("pipeline stages did not preserve workflow artifacts: %+v", stages[1])
	}
}

func TestWorkflowContractParsesStateActionsAndRejectsBrokenActionRoute(t *testing.T) {
	markdown := `---
id: custom-flow
name: Custom Flow
states:
  - id: todo
    title: Todo
    agentId: requirement
    agents: [requirement]
    actions:
      - id: capture
        type: write_requirement_artifact
        agent: requirement
        prompt: requirement
    transitions:
      passed: coding
  - id: coding
    title: Coding
    agentId: coding
    agents: [coding]
    actions:
      - id: implement
        type: run_agent
        agent: coding
        prompt: coding
        requiresDiff: true
        transitions:
          passed: missing
taskClasses:
  - id: simple
    workpadMode: compact
---

## Prompt: coding
Ship {{title}}.
`
	template, err := parseWorkflowTemplateMarkdown(markdown, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(template.StateProfiles) != 2 || len(template.StageProfiles) != 2 || len(workflowActionPlan(&template)) != 2 {
		t.Fatalf("workflow state/action parse failed: %+v", template)
	}
	if !template.StateProfiles[1].Actions[0].RequiresDiff {
		t.Fatalf("requiresDiff should be parsed: %+v", template.StateProfiles[1].Actions[0])
	}
	if got := template.TaskClasses[0].WorkpadMode; got != "compact" {
		t.Fatalf("task class not parsed: %+v", template.TaskClasses)
	}
	validation := validateWorkflowTemplate(template)
	if validation.ok() || !strings.Contains(strings.Join(validation.Errors, "; "), "unknown stage") {
		t.Fatalf("expected action transition validation failure, got %+v", validation)
	}
}

func TestRepositoryWorkflowTemplateOverrideValidatesAndParsesPrompts(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".omega"), 0o755); err != nil {
		t.Fatal(err)
	}
	workflow := `---
id: devflow-pr
name: Repo workflow
stages:
  - id: todo
    title: Todo
    agentId: requirement
  - id: done
    title: Done
    agentId: delivery
runtime:
  maxReviewCycles: 1
  maxContinuationTurns: 3
transitions:
  - from: todo
    on: passed
    to: done
---

# Repo Workflow

## Prompt: coding

Repo-owned coding prompt for {{repositoryPath}}.
`
	if err := os.WriteFile(filepath.Join(repo, ".omega", "WORKFLOW.md"), []byte(workflow), 0o644); err != nil {
		t.Fatal(err)
	}
	template, validation, exists := loadRepositoryWorkflowTemplate(repo, "devflow-pr")
	if !exists || !validation.ok() {
		t.Fatalf("repo workflow should be valid: exists=%v validation=%+v", exists, validation)
	}
	if template.Source != filepath.Join(repo, ".omega", "WORKFLOW.md") || template.Runtime.MaxContinuationTurns != 3 {
		t.Fatalf("repo workflow not parsed: %+v", template)
	}
	rendered := renderWorkflowPromptSection(&template, "coding", map[string]string{"repositoryPath": "/tmp/repo"}, "fallback")
	if rendered != "Repo-owned coding prompt for /tmp/repo." {
		t.Fatalf("rendered prompt = %q", rendered)
	}
}

func TestAgentDefinitionsExposeStructuredHandoffContracts(t *testing.T) {
	definitions := agentDefinitions(defaultProviderSelection())
	byID := map[string]AgentDefinition{}
	for _, definition := range definitions {
		byID[definition.ID] = definition
	}
	expectOutput := func(agentID string, expected string) {
		for _, value := range byID[agentID].OutputContract {
			if value == expected {
				return
			}
		}
		t.Fatalf("agent %s output contract missing %q: %+v", agentID, expected, byID[agentID].OutputContract)
	}
	expectOutput("requirement", "Repository boundary")
	expectOutput("architect", "Agent handoff")
	expectOutput("coding", "Known follow-up or risk")
	expectOutput("testing", "Residual risk")
	expectOutput("review", "Rework instructions")
	expectOutput("delivery", "Operator notes")
}

func TestRepositoryWorkflowTemplateValidationRejectsBrokenContract(t *testing.T) {
	template := PipelineTemplate{
		ID:            "devflow-pr",
		StageProfiles: []StageProfile{{ID: "todo", Title: "Todo", Agent: "requirement"}},
		Transitions:   []WorkflowTransitionProfile{{From: "todo", On: "passed", To: "missing"}},
	}
	validation := validateWorkflowTemplate(template)
	if validation.ok() || !strings.Contains(strings.Join(validation.Errors, "; "), "unknown stage") {
		t.Fatalf("expected validation failure, got %+v", validation)
	}
}

func TestDevFlowRunNamesAreStablePerItem(t *testing.T) {
	firstBranch := devFlowRunBranchName("OMG-6")
	secondBranch := devFlowRunBranchName("OMG-6")
	if firstBranch != secondBranch {
		t.Fatalf("branches should be stable per item: %s != %s", firstBranch, secondBranch)
	}
	if firstBranch != "omega/OMG-6-devflow" {
		t.Fatalf("branch shape = %s", firstBranch)
	}

	firstWorkspace := devFlowRunWorkspaceName("OMG-6")
	secondWorkspace := devFlowRunWorkspaceName("OMG-6")
	if firstWorkspace != secondWorkspace {
		t.Fatalf("workspaces should be stable per item: %s != %s", firstWorkspace, secondWorkspace)
	}
	if strings.Contains(firstWorkspace, "/") || !strings.HasSuffix(firstWorkspace, "-devflow-pr") {
		t.Fatalf("workspace segment should be path-safe: %s", firstWorkspace)
	}
}

func TestDevFlowReviewOutcomeRoutesChangesRequestedToRework(t *testing.T) {
	dir := t.TempDir()
	review := filepath.Join(dir, "review.md")
	if err := os.WriteFile(review, []byte("# Review\n\nVerdict: CHANGES_REQUESTED\n\nSummary:\n- The diff does not satisfy the registration requirement.\n\nBlocking findings:\n- [high] src/register.tsx - submit does not persist the user - wire it to the save API.\n\nRework instructions:\n- Add the save call and cover it with a focused test."), 0o644); err != nil {
		t.Fatal(err)
	}
	outcome := devFlowReviewOutcome(review)
	if outcome.Verdict != "changes_requested" {
		t.Fatalf("outcome = %+v", outcome)
	}
	if !strings.Contains(outcome.Summary, "submit does not persist the user") || strings.Contains(outcome.Summary, "Verdict:") {
		t.Fatalf("review summary should preserve agent feedback: %+v", outcome)
	}
	template := findPipelineTemplate("devflow-pr")
	if got := devFlowTransitionTo(template, "code_review_round_1", "changes_requested", ""); got != "rework" {
		t.Fatalf("changes_requested transition = %q", got)
	}
	if got := devFlowTransitionTo(template, "rework", "passed", ""); got != "code_review_round_1" {
		t.Fatalf("rework transition = %q", got)
	}
}

func TestDevFlowStageStatusAfterChangesRequestedQueuesRework(t *testing.T) {
	status, next := devFlowStageStatusAfterInvocation("code_review_round_1", "review", "changes-requested")
	if status != "passed" || next != "rework" {
		t.Fatalf("stage status after changes requested = %s next=%s", status, next)
	}
	status, next = devFlowStageStatusAfterInvocation("rework", "testing", "passed")
	if status != "passed" || next != "code_review_round_1" {
		t.Fatalf("stage status after rework = %s next=%s", status, next)
	}
	workflow := map[string]any{"transitions": []any{
		map[string]any{"from": "code_review_round_1", "on": "changes_requested", "to": "human_review"},
		map[string]any{"from": "rework", "on": "passed", "to": "code_review_round_2"},
	}}
	status, next = devFlowStageStatusAfterInvocationWithWorkflow(workflow, "code_review_round_1", "review", "changes-requested")
	if status != "passed" || next != "human_review" {
		t.Fatalf("contract changes-requested route = %s next=%s", status, next)
	}
	status, next = devFlowStageStatusAfterInvocationWithWorkflow(workflow, "rework", "testing", "passed")
	if status != "passed" || next != "code_review_round_2" {
		t.Fatalf("contract rework pass route = %s next=%s", status, next)
	}
}

func TestBeginDevFlowAttemptResetsStaleFailedStages(t *testing.T) {
	database := defaultWorkspaceDatabase()
	item := map[string]any{
		"id": "item_manual_7", "key": "OMG-7", "title": "Retry stale pipeline", "status": "Blocked", "assignee": "requirement",
	}
	pipeline := makePipelineWithTemplate(item, findPipelineTemplate("devflow-pr"))
	pipeline = markDevFlowStageProgress(pipeline, "code_review_round_1", "running", "old run")
	pipeline = markDevFlowStageProgress(pipeline, "code_review_round_2", "failed", "old failure")
	database.Tables.Pipelines = []map[string]any{pipeline}
	database, nextPipeline, attempt := beginDevFlowAttempt(database, 0, item, pipeline, "manual")

	if attempt["status"] != "running" || database.Tables.Pipelines[0]["status"] != "running" {
		t.Fatalf("attempt/pipeline not running: attempt=%+v pipeline=%+v", attempt, database.Tables.Pipelines[0])
	}
	for index, stage := range arrayMaps(mapValue(nextPipeline["run"])["stages"]) {
		if index == 0 {
			if stage["status"] != "running" {
				t.Fatalf("first stage should be running: %+v", stage)
			}
			continue
		}
		if stage["status"] != "waiting" || stage["notes"] != nil || len(stringSlice(stage["evidence"])) != 0 {
			t.Fatalf("stale stage not reset: %+v", stage)
		}
	}
}

func TestFailedAttemptPreservesPartialRunEvidence(t *testing.T) {
	database := defaultWorkspaceDatabase()
	item := map[string]any{
		"id": "item_manual_6", "key": "OMG-6", "title": "Failed review", "description": "Need evidence.",
		"status": "Ready", "assignee": "requirement", "stageId": "intake", "source": "manual", "repositoryTargetId": "repo_ZYOOO_TestRepo", "target": "https://github.com/ZYOOO/TestRepo",
	}
	pipeline := makePipelineWithTemplate(item, findPipelineTemplate("devflow-pr"))
	database.Tables.Pipelines = []map[string]any{pipeline}
	database, pipeline, attempt := beginDevFlowAttempt(database, 0, item, pipeline, "manual")
	pipeline = markDevFlowStageProgress(pipeline, "code_review_round_1", "failed", "Review requested changes.")

	result := map[string]any{
		"workspacePath":  "/tmp/omega/OMG-6-attempt-devflow-pr",
		"branchName":     "omega/OMG-6-attempt-devflow",
		"pullRequestUrl": "https://github.com/ZYOOO/TestRepo/pull/15",
		"stdout":         "partial run stdout",
		"repositoryPath": "/tmp/omega/OMG-6-attempt-devflow-pr/repo",
		"changedFiles":   []any{"index.html"},
		"stageArtifacts": []any{map[string]any{"stageId": "code_review_round_1", "artifact": "code-review-round-1.md"}},
		"proofFiles":     []any{"/tmp/omega/OMG-6-attempt-devflow-pr/.omega/proof/code-review-round-1.md"},
		"agentInvocations": []any{map[string]any{
			"stageId": "code_review_round_1", "agentId": "review", "status": "failed", "summary": "Review requested changes.",
		}},
	}
	database, failedAttempt := failAttemptRecord(database, text(attempt, "id"), pipeline, "code_review_round_1 blocked delivery: Review requested changes.", result)

	if failedAttempt["status"] != "failed" || failedAttempt["workspacePath"] != result["workspacePath"] || failedAttempt["branchName"] != result["branchName"] || failedAttempt["pullRequestUrl"] != result["pullRequestUrl"] {
		t.Fatalf("failed attempt lost partial evidence: %+v", failedAttempt)
	}
	if database.Tables.Attempts[0]["stderrSummary"] == "" || database.Tables.Attempts[0]["stdoutSummary"] == "" {
		t.Fatalf("failed attempt summaries missing: %+v", database.Tables.Attempts[0])
	}
}

func TestEnsureTablesBackfillsLegacyRequirementAndPipelineExecutionMetadata(t *testing.T) {
	database := defaultWorkspaceDatabase()
	item := map[string]any{
		"id": "item_legacy", "key": "OMG-9", "title": "Legacy requirement", "description": "Need a real dispatch plan.",
		"status": "Ready", "assignee": "requirement", "stageId": "intake", "source": "manual", "repositoryTargetId": "repo_ZYOOO_TestRepo", "target": "https://github.com/ZYOOO/TestRepo", "requirementId": "req_item_legacy",
	}
	database.Tables.WorkItems = []map[string]any{item}
	database.Tables.Requirements = []map[string]any{{
		"id": "req_item_legacy", "title": "Legacy requirement", "status": "converted", "structured": map[string]any{"summary": "Legacy requirement"},
	}}
	database.Tables.Pipelines = []map[string]any{{
		"id": "pipeline_item_legacy", "workItemId": "item_legacy", "templateId": "devflow-pr", "status": "done",
		"run": map[string]any{"stages": []any{
			map[string]any{"id": "todo", "title": "Todo intake", "status": "passed"},
			map[string]any{"id": "in_progress", "title": "Implementation and PR", "status": "passed"},
		}},
	}}

	ensureTables(&database)

	requirement := database.Tables.Requirements[0]
	structured := mapValue(requirement["structured"])
	if structured["masterAgentId"] != "master" || structured["dispatchStatus"] != "ready" || len(mapValue(structured["dispatchPlan"])) == 0 {
		t.Fatalf("legacy requirement was not backfilled with master dispatch: %+v", structured)
	}
	pipeline := database.Tables.Pipelines[0]
	run := mapValue(pipeline["run"])
	if len(arrayMaps(run["agents"])) < 6 || len(arrayMaps(run["dataFlow"])) == 0 || mapValue(run["orchestrator"])["masterAgentId"] != "master" {
		t.Fatalf("legacy pipeline execution metadata missing: %+v", run)
	}
	stages := arrayMaps(run["stages"])
	if len(stages) != 8 || stages[0]["status"] != "passed" || len(anySlice(stages[1]["dependsOn"])) == 0 || len(anySlice(stages[1]["inputArtifacts"])) == 0 {
		t.Fatalf("legacy stage state/artifacts not normalized: %+v", stages)
	}
}

func TestSQLiteRepositoryWaitsForTransientDatabaseLock(t *testing.T) {
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)
	defer api.Close()

	lock := exec.Command("sqlite3", repo.Path)
	lock.Stdin = strings.NewReader("BEGIN EXCLUSIVE;\nUPDATE workspace_snapshots SET saved_at = saved_at WHERE id = 'default';\n.shell sleep 1\nCOMMIT;\n")
	if err := lock.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Wait() }()
	time.Sleep(100 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if _, err := repo.Load(ctx); err != nil {
		t.Fatalf("load should wait for transient sqlite lock: %v", err)
	}
}

func TestCreateWorkItemAllocatesUniqueIDForDuplicateClientID(t *testing.T) {
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)

	item := map[string]any{
		"id": "item_manual_1", "key": "OMG-1", "title": "First item", "description": "Created by Go service.",
		"status": "Ready", "priority": "High", "assignee": "requirement", "labels": []any{"manual"}, "team": "Omega", "stageId": "intake", "target": "No target",
	}
	_ = postJSON(t, api.URL+"/work-items", map[string]any{"item": item})

	duplicate := cloneMap(item)
	duplicate["title"] = "Second item"
	var database WorkspaceDatabase
	decode(t, postJSON(t, api.URL+"/work-items", map[string]any{"item": duplicate}), &database)

	if len(database.Tables.WorkItems) != 2 {
		t.Fatalf("work item count = %d", len(database.Tables.WorkItems))
	}
	if database.Tables.WorkItems[0]["id"] == database.Tables.WorkItems[1]["id"] {
		t.Fatalf("work item ids should be unique: %+v", database.Tables.WorkItems)
	}
	if database.Tables.WorkItems[1]["id"] != "item_manual_1_2" {
		t.Fatalf("second work item id = %v", database.Tables.WorkItems[1]["id"])
	}
	if database.Tables.WorkItems[1]["key"] == "OMG-1" {
		t.Fatalf("second work item key should be unique: %+v", database.Tables.WorkItems)
	}
}

func TestGitHubRepositorySlugFromTarget(t *testing.T) {
	cases := map[string]string{
		"https://github.com/ZYOOO/TestRepo/issues/12": "ZYOOO/TestRepo",
		"https://github.com/ZYOOO/TestRepo.git":       "ZYOOO/TestRepo",
		"ZYOOO/TestRepo":                              "ZYOOO/TestRepo",
		"https://example.com/ZYOOO/TestRepo":          "",
	}
	for input, want := range cases {
		if got := githubRepositorySlugFromTarget(input); got != want {
			t.Fatalf("githubRepositorySlugFromTarget(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestDemoFixtureChangeCanSatisfyRequestedEmptyMarkdownFile(t *testing.T) {
	file, content := demoFixtureChangeForMission(
		map[string]any{"title": "Add an empty md file"},
		map[string]any{"prompt": "Create a markdown placeholder"},
	)
	if file != "omega-empty.md" {
		t.Fatalf("file = %q", file)
	}
	if content == nil || len(content) != 0 {
		t.Fatalf("content = %q, want empty bytes", string(content))
	}
}

func TestPipelineCheckpointAndOperationAPI(t *testing.T) {
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)
	item := map[string]any{
		"id": "item_manual_1", "key": "OMG-1", "title": "Pipeline item", "description": "Needs orchestration.",
		"status": "Ready", "priority": "High", "assignee": "requirement", "labels": []any{"manual"}, "team": "Omega", "stageId": "intake", "target": "No target",
	}
	_ = postJSON(t, api.URL+"/work-items", map[string]any{"item": item})

	var pipeline map[string]any
	decode(t, postJSON(t, api.URL+"/pipelines/from-work-item", map[string]any{"item": item}), &pipeline)
	decode(t, requestJSON(t, http.MethodPost, api.URL+"/pipelines/"+pipeline["id"].(string)+"/start", nil), &pipeline)
	if pipeline["status"] != "running" {
		t.Fatalf("started pipeline status = %v", pipeline["status"])
	}
	decode(t, postJSON(t, api.URL+"/pipelines/"+pipeline["id"].(string)+"/complete-stage", map[string]any{"passed": true, "notes": "ready"}), &pipeline)
	if pipeline["status"] != "waiting-human" {
		t.Fatalf("completed pipeline status = %v", pipeline["status"])
	}

	checkpointsResponse, err := http.Get(api.URL + "/checkpoints")
	if err != nil {
		t.Fatal(err)
	}
	var checkpoints []map[string]any
	decode(t, checkpointsResponse, &checkpoints)
	if len(checkpoints) != 1 {
		t.Fatalf("checkpoint count = %d", len(checkpoints))
	}
	var approved map[string]any
	decode(t, postJSON(t, api.URL+"/checkpoints/"+checkpoints[0]["id"].(string)+"/approve", map[string]any{"reviewer": "alice"}), &approved)
	if approved["status"] != "approved" {
		t.Fatalf("checkpoint status = %v", approved["status"])
	}

	item2 := map[string]any{
		"id": "item_manual_2", "key": "OMG-2", "title": "Rejected pipeline item", "description": "Needs rejection loop.",
		"status": "Ready", "priority": "High", "assignee": "requirement", "labels": []any{"manual"}, "team": "Omega", "stageId": "intake", "target": "No target",
	}
	_ = postJSON(t, api.URL+"/work-items", map[string]any{"item": item2})
	var rejectedPipeline map[string]any
	decode(t, postJSON(t, api.URL+"/pipelines/from-work-item", map[string]any{"item": item2}), &rejectedPipeline)
	decode(t, requestJSON(t, http.MethodPost, api.URL+"/pipelines/"+rejectedPipeline["id"].(string)+"/start", nil), &rejectedPipeline)
	decode(t, postJSON(t, api.URL+"/pipelines/"+rejectedPipeline["id"].(string)+"/complete-stage", map[string]any{"passed": true, "notes": "needs edits"}), &rejectedPipeline)
	checkpointsResponse, err = http.Get(api.URL + "/checkpoints")
	if err != nil {
		t.Fatal(err)
	}
	decode(t, checkpointsResponse, &checkpoints)
	var pending map[string]any
	for _, checkpoint := range checkpoints {
		if checkpoint["pipelineId"] == rejectedPipeline["id"] && checkpoint["status"] == "pending" {
			pending = checkpoint
		}
	}
	if pending == nil {
		t.Fatalf("pending checkpoint for rejected pipeline not found: %+v", checkpoints)
	}
	var rejected map[string]any
	decode(t, postJSON(t, api.URL+"/checkpoints/"+pending["id"].(string)+"/request-changes", map[string]any{"reason": "Needs clearer acceptance criteria"}), &rejected)
	if rejected["status"] != "rejected" || rejected["decisionNote"] != "Needs clearer acceptance criteria" {
		t.Fatalf("rejected checkpoint = %+v", rejected)
	}
	pipelinesResponse, err := http.Get(api.URL + "/pipelines")
	if err != nil {
		t.Fatal(err)
	}
	var pipelines []map[string]any
	decode(t, pipelinesResponse, &pipelines)
	var foundRejectedPipeline map[string]any
	for _, candidate := range pipelines {
		if candidate["id"] == rejectedPipeline["id"] {
			foundRejectedPipeline = candidate
		}
	}
	if foundRejectedPipeline == nil {
		t.Fatalf("rejected pipeline not found: %+v", pipelines)
	}
	if foundRejectedPipeline["status"] != "running" {
		t.Fatalf("rejected pipeline status = %v", foundRejectedPipeline["status"])
	}
	stages := arrayMaps(mapValue(foundRejectedPipeline["run"])["stages"])
	if stages[0]["status"] != "ready" || stages[0]["rejectionReason"] != "Needs clearer acceptance criteria" {
		t.Fatalf("rejected stage = %+v", stages[0])
	}

	var mission map[string]any
	decode(t, postJSON(t, api.URL+"/missions/from-work-item", map[string]any{"item": item}), &mission)
	resultResponse := postJSON(t, api.URL+"/operations/run", map[string]any{"mission": mission, "operationId": "operation_intake", "runner": "local-proof"})
	var result OperationResult
	decode(t, resultResponse, &result)
	if result.Status != "passed" || len(result.ProofFiles) == 0 {
		t.Fatalf("operation result = %+v", result)
	}

	var operations []map[string]any
	operationsResponse, err := http.Get(api.URL + "/operations")
	if err != nil {
		t.Fatal(err)
	}
	decode(t, operationsResponse, &operations)
	if len(operations) == 0 || operations[0]["status"] != "done" {
		t.Fatalf("operations = %+v", operations)
	}
}

func TestDevFlowCheckpointApprovalToleratesMissingAttempt(t *testing.T) {
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)
	timestamp := nowISO()
	database, err := repo.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	item := map[string]any{
		"id":        "item_manual_legacy",
		"projectId": "project_omega", "key": "OMG-legacy", "title": "Legacy human gate", "description": "Checkpoint predates attempts.",
		"status": "In Review", "priority": "High", "assignee": "delivery", "labels": []any{"manual"}, "team": "Omega", "stageId": "human_review", "target": "ZYOOO/TestRepo", "createdAt": timestamp, "updatedAt": timestamp,
	}
	pipeline := makePipelineWithTemplate(item, &PipelineTemplate{
		ID:          "devflow-pr",
		Name:        "DevFlow PR cycle",
		Description: "DevFlow",
		StageProfiles: []StageProfile{
			{ID: "human_review", Title: "Human Review", Agent: "delivery", HumanGate: true},
			{ID: "merging", Title: "Merging", Agent: "delivery"},
			{ID: "done", Title: "Done", Agent: "delivery"},
		},
	})
	run := mapValue(pipeline["run"])
	stages := arrayMaps(run["stages"])
	stages[0]["status"] = "needs-human"
	run["stages"] = stages
	pipeline["run"] = run
	pipeline["status"] = "waiting-human"
	checkpoint := map[string]any{
		"id": "pipeline_item_manual_legacy:human_review", "pipelineId": pipeline["id"], "stageId": "human_review",
		"status": "pending", "title": "Human Review", "summary": "Waiting for approval.", "createdAt": timestamp, "updatedAt": timestamp,
	}
	database.Tables.WorkItems = append(database.Tables.WorkItems, item)
	database.Tables.Pipelines = append(database.Tables.Pipelines, pipeline)
	database.Tables.Checkpoints = append(database.Tables.Checkpoints, checkpoint)
	if err := repo.Save(context.Background(), *database); err != nil {
		t.Fatal(err)
	}

	var approved map[string]any
	decode(t, postJSON(t, api.URL+"/checkpoints/"+text(checkpoint, "id")+"/approve", map[string]any{"reviewer": "alice"}), &approved)
	if approved["status"] != "approved" {
		t.Fatalf("checkpoint status = %v", approved["status"])
	}
	updated, err := repo.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	index := findByID(updated.Tables.Pipelines, text(pipeline, "id"))
	if index < 0 {
		t.Fatalf("pipeline not found")
	}
	events := arrayMaps(mapValue(updated.Tables.Pipelines[index]["run"])["events"])
	if !containsEventType(events, "gate.approved.incomplete") {
		t.Fatalf("incomplete approval event missing: %+v", events)
	}
	if len(updated.Tables.Attempts) != 1 || text(updated.Tables.Checkpoints[0], "attemptId") == "" {
		t.Fatalf("legacy approval should recover attempt link: attempts=%+v checkpoints=%+v", updated.Tables.Attempts, updated.Tables.Checkpoints)
	}
}

func TestJobSupervisorTickBackfillsPendingCheckpointAttemptLink(t *testing.T) {
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)
	timestamp := nowISO()
	database, err := repo.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	item := map[string]any{
		"id":        "item_manual_supervisor",
		"projectId": "project_omega", "key": "OMG-supervisor", "title": "Repair checkpoint link", "description": "Pending gate lost attempt.",
		"status": "In Review", "priority": "High", "assignee": "delivery", "labels": []any{"manual"}, "team": "Omega", "stageId": "human_review", "target": "ZYOOO/TestRepo", "createdAt": timestamp, "updatedAt": timestamp,
	}
	pipeline := makePipelineWithTemplate(item, &PipelineTemplate{
		ID:          "devflow-pr",
		Name:        "DevFlow PR cycle",
		Description: "DevFlow",
		StageProfiles: []StageProfile{
			{ID: "human_review", Title: "Human Review", Agent: "delivery", HumanGate: true},
			{ID: "merging", Title: "Merging", Agent: "delivery"},
			{ID: "done", Title: "Done", Agent: "delivery"},
		},
	})
	run := mapValue(pipeline["run"])
	stages := arrayMaps(run["stages"])
	stages[0]["status"] = "needs-human"
	run["stages"] = stages
	pipeline["run"] = run
	pipeline["status"] = "waiting-human"
	checkpoint := map[string]any{
		"id": "pipeline_item_manual_supervisor:human_review", "pipelineId": pipeline["id"], "stageId": "human_review",
		"status": "pending", "title": "Human Review", "summary": "Waiting for approval.", "createdAt": timestamp, "updatedAt": timestamp,
	}
	database.Tables.WorkItems = append(database.Tables.WorkItems, item)
	database.Tables.Pipelines = append(database.Tables.Pipelines, pipeline)
	database.Tables.Checkpoints = append(database.Tables.Checkpoints, checkpoint)
	if err := repo.Save(context.Background(), *database); err != nil {
		t.Fatal(err)
	}

	var summary map[string]any
	decode(t, postJSON(t, api.URL+"/job-supervisor/tick", map[string]any{}), &summary)
	if summary["backfilledAttempts"].(float64) != 1 || summary["linkedCheckpoints"].(float64) != 1 {
		t.Fatalf("supervisor summary = %+v", summary)
	}
	updated, err := repo.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Tables.Attempts) != 1 || text(updated.Tables.Checkpoints[0], "attemptId") != text(updated.Tables.Attempts[0], "id") {
		t.Fatalf("repaired state attempts=%+v checkpoints=%+v", updated.Tables.Attempts, updated.Tables.Checkpoints)
	}
}

func TestJobSupervisorTickMarksStalledRunningAttempt(t *testing.T) {
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)
	timestamp := nowISO()
	database, err := repo.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	item := map[string]any{
		"id":        "item_manual_stalled",
		"projectId": "project_omega", "key": "OMG-stalled", "title": "Detect stalled run", "description": "Runner stopped heartbeating.",
		"status": "In Review", "priority": "High", "assignee": "coding", "labels": []any{"manual"}, "team": "Omega", "stageId": "in_progress", "target": "ZYOOO/TestRepo", "createdAt": timestamp, "updatedAt": timestamp,
	}
	pipeline := makePipelineWithTemplate(item, findPipelineTemplate("devflow-pr"))
	run := mapValue(pipeline["run"])
	stages := arrayMaps(run["stages"])
	stages[0]["status"] = "passed"
	stages[1]["status"] = "running"
	stages[1]["startedAt"] = timestamp
	run["stages"] = stages
	pipeline["run"] = run
	pipeline["status"] = "running"
	attempt := makeAttemptRecord(item, pipeline, "manual", "devflow-pr", "in_progress")
	attempt["id"] = "attempt_stalled"
	attempt["lastSeenAt"] = time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339Nano)
	attempt["updatedAt"] = attempt["lastSeenAt"]
	database.Tables.WorkItems = append(database.Tables.WorkItems, item)
	database.Tables.Pipelines = append(database.Tables.Pipelines, pipeline)
	database.Tables.Attempts = append(database.Tables.Attempts, attempt)
	if err := repo.Save(context.Background(), *database); err != nil {
		t.Fatal(err)
	}

	var summary map[string]any
	decode(t, postJSON(t, api.URL+"/job-supervisor/tick", map[string]any{"staleAfterSeconds": 60}), &summary)
	if summary["stalledAttempts"].(float64) != 1 {
		t.Fatalf("supervisor summary = %+v", summary)
	}
	updated, err := repo.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if updated.Tables.Attempts[0]["status"] != "stalled" || updated.Tables.Pipelines[0]["status"] != "stalled" || updated.Tables.WorkItems[0]["status"] != "Blocked" {
		t.Fatalf("stalled state attempt=%+v pipeline=%+v item=%+v", updated.Tables.Attempts[0], updated.Tables.Pipelines[0], updated.Tables.WorkItems[0])
	}
	if text(updated.Tables.Attempts[0], "stalledAt") == "" || text(updated.Tables.Attempts[0], "statusReason") == "" {
		t.Fatalf("stalled metadata missing: %+v", updated.Tables.Attempts[0])
	}
}

func TestJobSupervisorLoopRunsMaintenanceTicks(t *testing.T) {
	root := t.TempDir()
	openAPI := filepath.Join(root, "openapi.yaml")
	if err := os.WriteFile(openAPI, []byte("openapi: 3.1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := NewServer(filepath.Join(root, "omega.db"), filepath.Join(root, "workspace"), openAPI)
	seedWorkspace(t, server.Repo)
	timestamp := nowISO()
	database, err := server.Repo.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	item := map[string]any{
		"id":        "item_manual_supervisor_loop",
		"projectId": "project_omega", "key": "OMG-loop", "title": "Detect stalled run in loop", "description": "Runner stopped heartbeating.",
		"status": "In Review", "priority": "High", "assignee": "coding", "labels": []any{"manual"}, "team": "Omega", "stageId": "in_progress", "target": "ZYOOO/TestRepo", "createdAt": timestamp, "updatedAt": timestamp,
	}
	pipeline := makePipelineWithTemplate(item, &PipelineTemplate{
		ID:          "devflow-pr",
		Name:        "DevFlow PR cycle",
		Description: "DevFlow",
		StageProfiles: []StageProfile{
			{ID: "todo", Title: "Requirement", Agent: "requirement"},
			{ID: "in_progress", Title: "Implementation", Agent: "coding"},
		},
	})
	pipeline["status"] = "running"
	attempt := makeAttemptRecord(item, pipeline, "manual", "devflow-pr", "in_progress")
	attempt["id"] = "attempt_supervisor_loop"
	attempt["lastSeenAt"] = time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339Nano)
	attempt["updatedAt"] = attempt["lastSeenAt"]
	database.Tables.WorkItems = append(database.Tables.WorkItems, item)
	database.Tables.Pipelines = append(database.Tables.Pipelines, pipeline)
	database.Tables.Attempts = append(database.Tables.Attempts, attempt)
	if err := server.Repo.Save(context.Background(), *database); err != nil {
		t.Fatal(err)
	}

	stop := server.StartJobSupervisor(context.Background(), JobSupervisorConfig{
		Enabled:            true,
		Interval:           10 * time.Millisecond,
		StaleAfter:         time.Second,
		ReadyScanItemLimit: 1,
	})
	defer stop()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		updated, err := server.Repo.Load(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(updated.Tables.Attempts) == 1 && updated.Tables.Attempts[0]["status"] == "stalled" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	updated, err := server.Repo.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Fatalf("supervisor loop did not mark attempt stalled: %+v", updated.Tables.Attempts)
}

func TestCancelAttemptAPIUpdatesStateAndSignalsRunningJob(t *testing.T) {
	root := t.TempDir()
	openAPI := filepath.Join(root, "openapi.yaml")
	if err := os.WriteFile(openAPI, []byte("openapi: 3.1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := NewServer(filepath.Join(root, "omega.db"), filepath.Join(root, "workspace"), openAPI)
	api := httptest.NewServer(server.Handler())
	t.Cleanup(api.Close)
	seedWorkspace(t, server.Repo)
	database, err := server.Repo.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	item := map[string]any{
		"id": "item_manual_cancel", "projectId": "project_omega", "key": "OMG-cancel", "title": "Cancel run", "description": "Cancel a running attempt.",
		"status": "In Review", "priority": "High", "assignee": "coding", "labels": []any{"manual"}, "team": "Omega", "stageId": "in_progress", "target": "ZYOOO/TestRepo", "createdAt": nowISO(), "updatedAt": nowISO(),
	}
	pipeline := makePipelineWithTemplate(item, &PipelineTemplate{
		ID:          "devflow-pr",
		Name:        "DevFlow PR cycle",
		Description: "DevFlow",
		StageProfiles: []StageProfile{
			{ID: "todo", Title: "Requirement", Agent: "requirement"},
			{ID: "in_progress", Title: "Implementation", Agent: "coding"},
		},
	})
	pipeline["status"] = "running"
	attempt := makeAttemptRecord(item, pipeline, "manual", "devflow-pr", "in_progress")
	attempt["id"] = "attempt_cancel"
	database.Tables.WorkItems = append(database.Tables.WorkItems, item)
	database.Tables.Pipelines = append(database.Tables.Pipelines, pipeline)
	database.Tables.Attempts = append(database.Tables.Attempts, attempt)
	if err := server.Repo.Save(context.Background(), *database); err != nil {
		t.Fatal(err)
	}
	jobContext, cancel := context.WithCancel(context.Background())
	server.registerAttemptJob("attempt_cancel", cancel)
	defer server.unregisterAttemptJob("attempt_cancel")

	var result map[string]any
	decode(t, postJSON(t, api.URL+"/attempts/attempt_cancel/cancel", map[string]any{"reason": "Operator stopped the run."}), &result)
	if result["cancelSignalSent"] != true {
		t.Fatalf("cancel result = %+v", result)
	}
	select {
	case <-jobContext.Done():
	case <-time.After(time.Second):
		t.Fatalf("registered job was not canceled")
	}
	updated, err := server.Repo.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if updated.Tables.Attempts[0]["status"] != "canceled" || updated.Tables.Pipelines[0]["status"] != "canceled" || updated.Tables.WorkItems[0]["status"] != "Blocked" {
		t.Fatalf("canceled state attempt=%+v pipeline=%+v item=%+v", updated.Tables.Attempts[0], updated.Tables.Pipelines[0], updated.Tables.WorkItems[0])
	}
	if updated.Tables.Attempts[0]["statusReason"] != "Operator stopped the run." {
		t.Fatalf("status reason = %+v", updated.Tables.Attempts[0])
	}
}

func TestPrepareDevFlowAttemptRetryLinksAttempts(t *testing.T) {
	root := t.TempDir()
	server := NewServer(filepath.Join(root, "omega.db"), filepath.Join(root, "workspace"), filepath.Join(root, "openapi.yaml"))
	seedWorkspace(t, server.Repo)
	repoPath := createDemoGitRepo(t)
	ctx := context.Background()
	profile := defaultAgentProfile("project_omega", "repo_retry")
	for index := range profile.AgentProfiles {
		profile.AgentProfiles[index].Runner = "demo-code"
	}
	if err := server.Repo.SetAgentProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	database, err := server.Repo.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	project := cloneMap(database.Tables.Projects[0])
	project["repositoryTargets"] = []any{map[string]any{"id": "repo_retry", "kind": "local", "path": repoPath, "createdAt": nowISO(), "updatedAt": nowISO()}}
	project["defaultRepositoryTargetId"] = "repo_retry"
	database.Tables.Projects[0] = project
	item := map[string]any{
		"id": "item_retry", "projectId": "project_omega", "key": "OMG-retry", "title": "Retry failed attempt", "description": "Retry this failed work.",
		"status": "Blocked", "priority": "High", "assignee": "coding", "labels": []any{"manual"}, "team": "Omega", "stageId": "in_progress", "target": repoPath, "repositoryTargetId": "repo_retry", "createdAt": nowISO(), "updatedAt": nowISO(),
	}
	pipeline := makePipelineWithTemplate(item, findPipelineTemplate("devflow-pr"))
	pipeline["id"] = "pipeline_retry"
	pipeline["status"] = "failed"
	attempt := makeAttemptRecord(item, pipeline, "manual", "devflow-pr", "in_progress")
	attempt["id"] = "attempt_retry_old"
	attempt["status"] = "failed"
	attempt["errorMessage"] = "Coding agent produced no repository changes."
	attempt["failureReviewFeedback"] = "Review found missing proof and asked for a focused validation note."
	database.Tables.WorkItems = append(database.Tables.WorkItems, item)
	database.Tables.Pipelines = append(database.Tables.Pipelines, pipeline)
	database.Tables.Attempts = append(database.Tables.Attempts, attempt)

	updated, retryPipeline, retryAttempt, err := server.prepareDevFlowAttemptRetry(ctx, *database, "attempt_retry_old", "")
	if err != nil {
		t.Fatal(err)
	}
	if text(retryAttempt, "retryOfAttemptId") != "attempt_retry_old" || text(retryAttempt, "retryRootAttemptId") != "attempt_retry_old" || intValue(retryAttempt["retryIndex"]) != 1 {
		t.Fatalf("retry attempt metadata = %+v", retryAttempt)
	}
	if text(retryPipeline, "status") != "running" {
		t.Fatalf("retry pipeline = %+v", retryPipeline)
	}
	if !strings.Contains(text(retryAttempt, "retryReason"), "Review found missing proof") {
		t.Fatalf("retry reason should come from the rework checklist = %+v", retryAttempt)
	}
	reworkChecklist := mapValue(retryAttempt["reworkChecklist"])
	if text(reworkChecklist, "status") != "needs-rework" || !strings.Contains(text(reworkChecklist, "prompt"), "focused validation note") {
		t.Fatalf("retry attempt should carry rework checklist = %+v", reworkChecklist)
	}
	oldAttempt := updated.Tables.Attempts[findByID(updated.Tables.Attempts, "attempt_retry_old")]
	if text(oldAttempt, "retryAttemptId") != text(retryAttempt, "id") {
		t.Fatalf("old attempt retry link = %+v", oldAttempt)
	}
	if text(mapValue(oldAttempt["reworkChecklist"]), "status") != "needs-rework" {
		t.Fatalf("old attempt checklist = %+v", oldAttempt)
	}
	itemAfter := findWorkItem(updated, "item_retry")
	if text(itemAfter, "status") != "In Review" {
		t.Fatalf("work item status = %+v", itemAfter)
	}
}

func TestPrepareDevFlowHumanRequestedReworkStartsAttemptWithFeedback(t *testing.T) {
	root := t.TempDir()
	server := NewServer(filepath.Join(root, "omega.db"), filepath.Join(root, "workspace"), filepath.Join(root, "openapi.yaml"))
	seedWorkspace(t, server.Repo)
	repoPath := createDemoGitRepo(t)
	ctx := context.Background()
	profile := defaultAgentProfile("project_omega", "repo_human_rework")
	for index := range profile.AgentProfiles {
		profile.AgentProfiles[index].Runner = "demo-code"
	}
	if err := server.Repo.SetAgentProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	database, err := server.Repo.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	project := cloneMap(database.Tables.Projects[0])
	project["repositoryTargets"] = []any{map[string]any{"id": "repo_human_rework", "kind": "local", "path": repoPath, "createdAt": nowISO(), "updatedAt": nowISO()}}
	project["defaultRepositoryTargetId"] = "repo_human_rework"
	database.Tables.Projects[0] = project
	item := map[string]any{
		"id": "item_human_rework", "projectId": "project_omega", "key": "OMG-rework", "title": "Rename default user", "description": "Rename the default user.",
		"status": "In Review", "priority": "High", "assignee": "delivery", "labels": []any{"manual"}, "team": "Omega", "stageId": "human_review", "target": repoPath, "repositoryTargetId": "repo_human_rework", "createdAt": nowISO(), "updatedAt": nowISO(),
	}
	pipeline := makePipelineWithTemplate(item, &PipelineTemplate{
		ID:          "devflow-pr",
		Name:        "DevFlow PR cycle",
		Description: "DevFlow",
		StageProfiles: []StageProfile{
			{ID: "todo", Title: "Requirement", Agent: "requirement"},
			{ID: "in_progress", Title: "Implementation", Agent: "coding"},
			{ID: "code_review_round_1", Title: "Code Review Round 1", Agent: "review"},
			{ID: "rework", Title: "Rework", Agent: "coding"},
			{ID: "human_review", Title: "Human Review", Agent: "delivery", HumanGate: true},
			{ID: "merging", Title: "Merging", Agent: "delivery"},
			{ID: "done", Title: "Done", Agent: "delivery"},
		},
	})
	pipeline["id"] = "pipeline_human_rework"
	pipeline["status"] = "waiting-human"
	attempt := makeAttemptRecord(item, pipeline, "manual", "devflow-pr", "human_review")
	attempt["id"] = "attempt_human_review"
	attempt["status"] = "waiting-human"
	attempt["branchName"] = "omega/OMG-rework-devflow"
	attempt["pullRequestUrl"] = "https://github.com/acme/demo/pull/44"
	attempt["workspacePath"] = filepath.Join(root, "workspace", "OMG-rework")
	checkpoint := map[string]any{"id": "checkpoint_human_rework", "pipelineId": text(pipeline, "id"), "attemptId": text(attempt, "id"), "stageId": "human_review", "status": "pending", "title": "Human Review", "summary": "Approve delivery."}
	database.Tables.WorkItems = append(database.Tables.WorkItems, item)
	database.Tables.Pipelines = append(database.Tables.Pipelines, pipeline)
	database.Tables.Attempts = append(database.Tables.Attempts, attempt)
	database.Tables.Checkpoints = append(database.Tables.Checkpoints, checkpoint)

	reason := "改成章四"
	rejectPipelineStage(database, checkpoint, reason)
	updated, reworkPipeline, reworkAttempt, err := server.prepareDevFlowHumanRequestedRework(ctx, *database, checkpoint, reason)
	if err != nil {
		t.Fatal(err)
	}
	if text(reworkAttempt, "trigger") != "human-request-changes" || text(reworkAttempt, "humanChangeRequest") != reason {
		t.Fatalf("rework attempt metadata = %+v", reworkAttempt)
	}
	if text(reworkAttempt, "retryOfAttemptId") != "attempt_human_review" {
		t.Fatalf("rework link = %+v", reworkAttempt)
	}
	if text(reworkAttempt, "currentStageId") != "rework" {
		t.Fatalf("fast rework should enter rework stage, got %+v", reworkAttempt)
	}
	assessment := mapValue(reworkAttempt["reworkAssessment"])
	if text(assessment, "strategy") != reworkStrategyFastRework || text(assessment, "entryStageId") != "rework" {
		t.Fatalf("rework assessment = %+v", assessment)
	}
	if text(reworkAttempt, "branchName") != "omega/OMG-rework-devflow" || text(reworkAttempt, "pullRequestUrl") != "https://github.com/acme/demo/pull/44" {
		t.Fatalf("rework attempt did not inherit delivery branch/PR = %+v", reworkAttempt)
	}
	reworkStages := arrayMaps(mapValue(reworkPipeline["run"])["stages"])
	reworkStage := map[string]any{}
	for _, stage := range reworkStages {
		if text(stage, "id") == "rework" {
			reworkStage = stage
		}
	}
	if text(reworkStages[0], "status") != "passed" || text(reworkStage, "status") != "running" {
		t.Fatalf("fast rework pipeline stages = %+v", reworkStages)
	}
	if got := latestHumanChangeRequestFromPipeline(reworkPipeline); got != reason {
		t.Fatalf("human change request = %q", got)
	}
	oldAttempt := updated.Tables.Attempts[findByID(updated.Tables.Attempts, "attempt_human_review")]
	if text(oldAttempt, "status") != "changes-requested" || text(oldAttempt, "failureReviewFeedback") != reason {
		t.Fatalf("old attempt = %+v", oldAttempt)
	}
	workpadIndex := findByID(updated.Tables.RunWorkpads, text(reworkAttempt, "id")+":workpad")
	if workpadIndex < 0 {
		t.Fatalf("rework workpad missing: %+v", updated.Tables.RunWorkpads)
	}
	workpad := mapValue(updated.Tables.RunWorkpads[workpadIndex]["workpad"])
	if !strings.Contains(strings.Join(stringSlice(workpad["reviewFeedback"]), "\n"), reason) || !strings.Contains(text(workpad, "retryReason"), reason) {
		t.Fatalf("workpad feedback = %+v", workpad)
	}
	workpadAssessment := mapValue(workpad["reworkAssessment"])
	if text(workpadAssessment, "strategy") != reworkStrategyFastRework {
		t.Fatalf("workpad rework assessment = %+v", workpadAssessment)
	}
}

func TestAssessHumanRequestedReworkRoutesByScope(t *testing.T) {
	item := map[string]any{"id": "item_scope", "key": "OMG-scope", "title": "Scope"}
	previousAttempt := map[string]any{"id": "attempt_previous", "branchName": "omega/OMG-scope-devflow", "pullRequestUrl": "https://github.com/acme/demo/pull/9"}
	pipeline := map[string]any{"id": "pipeline_scope"}

	fast := assessHumanRequestedRework("把默认用户名改成章四", item, previousAttempt, pipeline)
	if text(fast, "strategy") != reworkStrategyFastRework || text(fast, "entryStageId") != "rework" {
		t.Fatalf("fast assessment = %+v", fast)
	}

	replan := assessHumanRequestedRework("需要重做接口和权限流程", item, previousAttempt, pipeline)
	if text(replan, "strategy") != reworkStrategyReplanRework || text(replan, "entryStageId") != "todo" {
		t.Fatalf("replan assessment = %+v", replan)
	}

	needsInfo := assessHumanRequestedRework("你看着办？", item, previousAttempt, pipeline)
	if text(needsInfo, "strategy") != reworkStrategyNeedsHumanInfo || text(needsInfo, "entryStageId") != "human_review" {
		t.Fatalf("needs-human assessment = %+v", needsInfo)
	}
}

func TestPrepareDevFlowHumanRequestedReworkWaitsWhenFeedbackNeedsInfo(t *testing.T) {
	root := t.TempDir()
	server := NewServer(filepath.Join(root, "omega.db"), filepath.Join(root, "workspace"), filepath.Join(root, "openapi.yaml"))
	seedWorkspace(t, server.Repo)
	repoPath := createDemoGitRepo(t)
	ctx := context.Background()
	profile := defaultAgentProfile("project_omega", "repo_human_rework_info")
	for index := range profile.AgentProfiles {
		profile.AgentProfiles[index].Runner = "demo-code"
	}
	if err := server.Repo.SetAgentProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	database, err := server.Repo.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	project := cloneMap(database.Tables.Projects[0])
	project["repositoryTargets"] = []any{map[string]any{"id": "repo_human_rework_info", "kind": "local", "path": repoPath, "createdAt": nowISO(), "updatedAt": nowISO()}}
	project["defaultRepositoryTargetId"] = "repo_human_rework_info"
	database.Tables.Projects[0] = project
	item := map[string]any{
		"id": "item_human_rework_info", "projectId": "project_omega", "key": "OMG-rework-info", "title": "Clarify user", "description": "Clarify user change.",
		"status": "In Review", "priority": "High", "assignee": "delivery", "labels": []any{"manual"}, "team": "Omega", "stageId": "human_review", "target": repoPath, "repositoryTargetId": "repo_human_rework_info", "createdAt": nowISO(), "updatedAt": nowISO(),
	}
	pipeline := makePipelineWithTemplate(item, findPipelineTemplate("devflow-pr"))
	pipeline["id"] = "pipeline_human_rework_info"
	pipeline["status"] = "waiting-human"
	attempt := makeAttemptRecord(item, pipeline, "manual", "devflow-pr", "human_review")
	attempt["id"] = "attempt_human_review_info"
	attempt["status"] = "waiting-human"
	attempt["branchName"] = "omega/OMG-rework-info-devflow"
	attempt["pullRequestUrl"] = "https://github.com/acme/demo/pull/45"
	checkpoint := map[string]any{"id": "checkpoint_human_rework_info", "pipelineId": text(pipeline, "id"), "attemptId": text(attempt, "id"), "stageId": "human_review", "status": "pending", "title": "Human Review", "summary": "Approve delivery."}
	database.Tables.WorkItems = append(database.Tables.WorkItems, item)
	database.Tables.Pipelines = append(database.Tables.Pipelines, pipeline)
	database.Tables.Attempts = append(database.Tables.Attempts, attempt)
	database.Tables.Checkpoints = append(database.Tables.Checkpoints, checkpoint)

	reason := "你看着办？"
	rejectPipelineStage(database, checkpoint, reason)
	_, reworkPipeline, reworkAttempt, err := server.prepareDevFlowHumanRequestedRework(ctx, *database, checkpoint, reason)
	if err != nil {
		t.Fatal(err)
	}
	if text(reworkAttempt, "status") != "waiting-human" || text(reworkAttempt, "currentStageId") != "human_review" {
		t.Fatalf("needs-info attempt should wait for human, got %+v", reworkAttempt)
	}
	assessment := mapValue(reworkAttempt["reworkAssessment"])
	if text(assessment, "strategy") != reworkStrategyNeedsHumanInfo {
		t.Fatalf("needs-info assessment = %+v", assessment)
	}
	stages := arrayMaps(mapValue(reworkPipeline["run"])["stages"])
	humanStage := map[string]any{}
	for _, stage := range stages {
		if text(stage, "id") == "human_review" {
			humanStage = stage
			break
		}
	}
	if text(humanStage, "status") != "needs-human" {
		t.Fatalf("human stage should need info: %+v", stages)
	}
}

func TestResetDevFlowPipelineForAttemptFromStageFallsBackWhenStageMissing(t *testing.T) {
	item := map[string]any{"id": "item_reset", "projectId": "project_omega", "key": "OMG-reset", "title": "Reset", "repositoryTargetId": "repo_reset"}
	pipeline := makePipelineWithTemplate(item, &PipelineTemplate{
		ID: "devflow-pr",
		StageProfiles: []StageProfile{
			{ID: "todo", Title: "Todo", Agent: "requirement"},
			{ID: "rework", Title: "Rework", Agent: "coding"},
		},
	})
	reset := resetDevFlowPipelineForAttemptFromStage(pipeline, "missing-stage")
	stages := arrayMaps(mapValue(reset["run"])["stages"])
	if text(stages[0], "status") != "running" || text(stages[1], "status") != "waiting" {
		t.Fatalf("missing stage should fall back to todo: %+v", stages)
	}
}

func TestEnsureDevFlowRepositoryWorkspaceRestoresRemoteDeliveryBranch(t *testing.T) {
	repoPath := createDemoGitRepo(t)
	branchName := "omega/OMG-restore-devflow"
	runGit(t, repoPath, "checkout", "-b", branchName)
	if err := os.WriteFile(filepath.Join(repoPath, "reviewed-version.txt"), []byte("first reviewed version\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoPath, "add", ".")
	runGit(t, repoPath, "commit", "-m", "first reviewed version")
	runGit(t, repoPath, "checkout", "main")

	workspace := filepath.Join(t.TempDir(), "workspace")
	repoWorkspace := filepath.Join(workspace, "repo")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureDevFlowRepositoryWorkspace(workspace, repoWorkspace, repoPath, branchName, "main"); err != nil {
		t.Fatal(err)
	}
	currentBranch := strings.TrimSpace(runGit(t, repoWorkspace, "branch", "--show-current"))
	if currentBranch != branchName {
		t.Fatalf("current branch = %s", currentBranch)
	}
	if _, err := os.Stat(filepath.Join(repoWorkspace, "reviewed-version.txt")); err != nil {
		t.Fatalf("remote delivery branch content was not restored: %v", err)
	}
}

func TestFailAttemptRecordPersistsFailureReasonContract(t *testing.T) {
	item := map[string]any{
		"id": "item_failure_reason", "projectId": "project_omega", "key": "OMG-failure", "title": "Failure reason", "description": "Capture retry reason.",
		"status": "In Review", "priority": "High", "assignee": "coding", "labels": []any{"manual"}, "team": "Omega", "stageId": "in_progress", "target": "repo", "createdAt": nowISO(), "updatedAt": nowISO(),
	}
	pipeline := makePipelineWithTemplate(item, &PipelineTemplate{
		ID:          "devflow-pr",
		Name:        "DevFlow PR cycle",
		Description: "DevFlow",
		StageProfiles: []StageProfile{
			{ID: "todo", Title: "Requirement", Agent: "requirement"},
			{ID: "in_progress", Title: "Implementation and PR", Agent: "coding"},
			{ID: "code_review_round_1", Title: "Code Review Round 1", Agent: "review"},
		},
	})
	attempt := makeAttemptRecord(item, pipeline, "manual", "devflow-pr", "in_progress")
	attempt["id"] = "attempt_failure_reason"
	database := WorkspaceDatabase{Tables: WorkspaceTables{WorkItems: []map[string]any{item}, Pipelines: []map[string]any{pipeline}, Attempts: []map[string]any{attempt}}}
	result := map[string]any{
		"failureStageId":        "rework",
		"failureAgentId":        "coding",
		"failureReason":         "Rework agent failed while applying review feedback.",
		"failureDetail":         "rework agent produced no repository changes",
		"failureReviewFeedback": "Review requested a clearer validation state.",
	}
	updated, failedAttempt := failAttemptRecord(database, "attempt_failure_reason", pipeline, "rework agent produced no repository changes", result)
	if text(failedAttempt, "failureReason") != "Rework agent failed while applying review feedback." || text(failedAttempt, "failureStageId") != "rework" || text(failedAttempt, "failureAgentId") != "coding" {
		t.Fatalf("failure contract = %+v", failedAttempt)
	}
	if text(failedAttempt, "failureReviewFeedback") == "" || text(failedAttempt, "failureDetail") == "" {
		t.Fatalf("failure detail missing = %+v", failedAttempt)
	}
	persisted := updated.Tables.Attempts[findByID(updated.Tables.Attempts, "attempt_failure_reason")]
	if text(persisted, "currentStageId") != "rework" {
		t.Fatalf("current stage = %+v", persisted)
	}
}

func TestJobSupervisorTickReportsRunnableReadyWork(t *testing.T) {
	root := t.TempDir()
	server := NewServer(filepath.Join(root, "omega.db"), filepath.Join(root, "workspace"), filepath.Join(root, "openapi.yaml"))
	api := httptest.NewServer(server.Handler())
	t.Cleanup(api.Close)
	seedWorkspace(t, server.Repo)
	repoPath := createDemoGitRepo(t)
	ctx := context.Background()
	profile := defaultAgentProfile("project_omega", "repo_supervisor_ready")
	for index := range profile.AgentProfiles {
		profile.AgentProfiles[index].Runner = "demo-code"
	}
	if err := server.Repo.SetAgentProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	database, err := server.Repo.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	project := cloneMap(database.Tables.Projects[0])
	project["repositoryTargets"] = []any{map[string]any{"id": "repo_supervisor_ready", "kind": "local", "path": repoPath, "createdAt": nowISO(), "updatedAt": nowISO()}}
	project["defaultRepositoryTargetId"] = "repo_supervisor_ready"
	database.Tables.Projects[0] = project
	item := map[string]any{
		"id": "item_supervisor_ready", "projectId": "project_omega", "key": "OMG-ready", "title": "Ready supervisor run", "description": "Runnable work.",
		"status": "Ready", "priority": "High", "assignee": "coding", "labels": []any{"manual"}, "team": "Omega", "stageId": "todo", "target": repoPath, "repositoryTargetId": "repo_supervisor_ready", "createdAt": nowISO(), "updatedAt": nowISO(),
	}
	database.Tables.WorkItems = append(database.Tables.WorkItems, item)
	if err := server.Repo.Save(ctx, *database); err != nil {
		t.Fatal(err)
	}

	var summary map[string]any
	decode(t, postJSON(t, api.URL+"/job-supervisor/tick", map[string]any{"limit": 5}), &summary)
	if summary["runnableItems"].(float64) != 1 || len(arrayMaps(summary["runnableWorkItems"])) != 1 {
		t.Fatalf("supervisor summary = %+v", summary)
	}
	loaded, err := server.Repo.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Tables.Attempts) != 0 {
		t.Fatalf("scan without autoRunReady should not create attempts: %+v", loaded.Tables.Attempts)
	}
}

func TestJobSupervisorTickBackfillsWorkflowContractMetadata(t *testing.T) {
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)
	database, err := repo.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	item := map[string]any{"id": "item_workflow_contract", "projectId": "project_omega", "key": "OMG-contract", "title": "Contract", "status": "Ready", "repositoryTargetId": "repo_contract", "createdAt": nowISO(), "updatedAt": nowISO()}
	pipeline := makePipelineWithTemplate(item, findPipelineTemplate("devflow-pr"))
	run := mapValue(pipeline["run"])
	delete(run, "workflow")
	pipeline["run"] = run
	database.Tables.WorkItems = append(database.Tables.WorkItems, item)
	database.Tables.Pipelines = append(database.Tables.Pipelines, pipeline)
	if err := repo.Save(context.Background(), *database); err != nil {
		t.Fatal(err)
	}

	var summary map[string]any
	decode(t, postJSON(t, api.URL+"/job-supervisor/tick", map[string]any{"limit": 1}), &summary)
	if summary["workflowContractPipelines"].(float64) != 1 || summary["workflowContractMissing"].(float64) != 0 {
		t.Fatalf("workflow summary = %+v", summary)
	}
	contracts := arrayMaps(summary["workflowContracts"])
	if len(contracts) != 1 || intValue(contracts[0]["transitionCount"]) == 0 || intValue(contracts[0]["maxReviewCycles"]) == 0 {
		t.Fatalf("workflow contracts = %+v", contracts)
	}
	updated, err := repo.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	workflow := mapValue(mapValue(updated.Tables.Pipelines[0]["run"])["workflow"])
	if len(arrayMaps(workflow["transitions"])) == 0 || len(mapValue(workflow["runtime"])) == 0 {
		t.Fatalf("workflow was not backfilled: %+v", workflow)
	}
}

func TestJobSupervisorScanRecoverableAttemptsRetriesWithPolicy(t *testing.T) {
	root := t.TempDir()
	server := NewServer(filepath.Join(root, "omega.db"), filepath.Join(root, "workspace"), filepath.Join(root, "openapi.yaml"))
	seedWorkspace(t, server.Repo)
	repoPath := createDemoGitRepo(t)
	ctx := context.Background()
	profile := defaultAgentProfile("project_omega", "repo_supervisor_retry")
	for index := range profile.AgentProfiles {
		profile.AgentProfiles[index].Runner = "demo-code"
	}
	if err := server.Repo.SetAgentProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	database, err := server.Repo.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	project := cloneMap(database.Tables.Projects[0])
	project["repositoryTargets"] = []any{map[string]any{"id": "repo_supervisor_retry", "kind": "local", "path": repoPath, "createdAt": nowISO(), "updatedAt": nowISO()}}
	project["defaultRepositoryTargetId"] = "repo_supervisor_retry"
	database.Tables.Projects[0] = project
	item := map[string]any{
		"id": "item_supervisor_retry", "projectId": "project_omega", "key": "OMG-retry-supervisor", "title": "Supervisor retry", "description": "Retry this failed work.",
		"status": "Blocked", "priority": "High", "assignee": "coding", "labels": []any{"manual"}, "team": "Omega", "stageId": "in_progress", "target": repoPath, "repositoryTargetId": "repo_supervisor_retry", "createdAt": nowISO(), "updatedAt": nowISO(),
	}
	pipeline := makePipelineWithTemplate(item, findPipelineTemplate("devflow-pr"))
	pipeline["id"] = "pipeline_supervisor_retry"
	pipeline["status"] = "failed"
	attempt := makeAttemptRecord(item, pipeline, "manual", "devflow-pr", "in_progress")
	attempt["id"] = "attempt_supervisor_failed"
	attempt["status"] = "failed"
	attempt["errorMessage"] = "temporary network failure"
	attempt["updatedAt"] = time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339Nano)
	database.Tables.WorkItems = append(database.Tables.WorkItems, item)
	database.Tables.Pipelines = append(database.Tables.Pipelines, pipeline)
	database.Tables.Attempts = append(database.Tables.Attempts, attempt)

	jobs := []map[string]any{}
	summary := server.scanRecoverableAttempts(ctx, database, jobSupervisorTickOptions{AutoRetryFailed: true, MaxRetryAttempts: 2, RetryBackoffSeconds: 60, Limit: 5}, &jobs)
	if summary["acceptedRetryAttempts"] != 1 || len(jobs) != 1 {
		t.Fatalf("recovery summary=%+v jobs=%+v", summary, jobs)
	}
	if len(database.Tables.Attempts) != 2 {
		t.Fatalf("retry attempt not appended: %+v", database.Tables.Attempts)
	}
	retryAttempt := database.Tables.Attempts[1]
	if text(retryAttempt, "retryOfAttemptId") != "attempt_supervisor_failed" || intValue(retryAttempt["retryIndex"]) != 1 {
		t.Fatalf("retry attempt = %+v", retryAttempt)
	}
	previous := database.Tables.Attempts[findByID(database.Tables.Attempts, "attempt_supervisor_failed")]
	if text(previous, "retryAttemptId") != text(retryAttempt, "id") {
		t.Fatalf("previous attempt link = %+v", previous)
	}
	if text(mapValue(jobs[0]["lock"]), "attemptId") != text(retryAttempt, "id") {
		t.Fatalf("retry job should carry workspace lock: %+v", jobs[0])
	}
	policy := mapValue(jobs[0]["recoveryPolicy"])
	if text(policy, "class") != "transient_network" || text(policy, "action") != "wait-and-retry" {
		t.Fatalf("retry job missing recovery policy: %+v", jobs[0])
	}
	actionPlan := mapValue(jobs[0]["actionPlan"])
	if text(actionPlan, "executionMode") != "contract-action-plan" || text(actionPlan, "currentStateId") != "todo" || text(actionPlan, "currentActionId") != "capture_requirement" {
		t.Fatalf("retry job missing action plan summary: %+v", jobs[0])
	}
	recoverable := arrayMaps(summary["recoverableAttempts"])
	if len(recoverable) == 0 || text(mapValue(recoverable[0]["actionPlan"]), "retryAction") != "retry_attempt" {
		t.Fatalf("recoverable attempt missing action plan summary: %+v", summary)
	}
}

func TestJobSupervisorRecoveryPolicyBlocksPermissionAutoRetry(t *testing.T) {
	server := NewServer(filepath.Join(t.TempDir(), "omega.db"), filepath.Join(t.TempDir(), "workspace"), filepath.Join(t.TempDir(), "openapi.yaml"))
	database := &WorkspaceDatabase{Tables: WorkspaceTables{Attempts: []map[string]any{
		{
			"id": "attempt_permission_failed", "pipelineId": "pipeline_permission", "itemId": "item_permission", "status": "failed",
			"updatedAt":    time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339Nano),
			"errorMessage": "gh repo view returned viewerPermission=READ and cannot create pull requests",
		},
	}}}
	jobs := []map[string]any{}
	summary := server.scanRecoverableAttempts(context.Background(), database, jobSupervisorTickOptions{AutoRetryFailed: true, MaxRetryAttempts: 2, Limit: 5}, &jobs)
	if summary["manualRecoveryRequired"] != 1 || summary["acceptedRetryAttempts"] != 0 || len(jobs) != 0 {
		t.Fatalf("permission failure should require manual action: summary=%+v jobs=%+v", summary, jobs)
	}
	record := arrayMaps(summary["recoverableAttempts"])[0]
	if text(record, "decision") != "manual-fix-permission" || text(mapValue(record["recoveryPolicy"]), "class") != "permission_failure" {
		t.Fatalf("record should explain permission recovery: %+v", record)
	}
}

func TestJobSupervisorScanRecoverableAttemptsUsesWorkflowRetryPolicy(t *testing.T) {
	server := NewServer(filepath.Join(t.TempDir(), "omega.db"), filepath.Join(t.TempDir(), "workspace"), filepath.Join(t.TempDir(), "openapi.yaml"))
	database := &WorkspaceDatabase{Tables: WorkspaceTables{}}
	item := map[string]any{"id": "item_contract_retry", "key": "OMG-contract-retry", "status": "Blocked", "repositoryTargetId": "repo_contract_retry", "createdAt": nowISO(), "updatedAt": nowISO()}
	pipeline := makePipelineWithTemplate(item, &PipelineTemplate{
		ID:            "devflow-pr",
		Name:          "DevFlow PR cycle",
		StageProfiles: []StageProfile{{ID: "todo", Title: "Todo", Agent: "requirement"}, {ID: "in_progress", Title: "Implementation", Agent: "coding"}},
		Runtime:       WorkflowRuntimeProfile{MaxRetryAttempts: 1, RetryBackoffSeconds: 7200},
	})
	pipeline["id"] = "pipeline_contract_retry"
	attempt := makeAttemptRecord(item, pipeline, "manual", "devflow-pr", "in_progress")
	attempt["id"] = "attempt_contract_retry"
	attempt["status"] = "failed"
	attempt["updatedAt"] = time.Now().UTC().Add(-30 * time.Minute).Format(time.RFC3339Nano)
	database.Tables.Pipelines = append(database.Tables.Pipelines, pipeline)
	database.Tables.Attempts = append(database.Tables.Attempts, attempt)

	jobs := []map[string]any{}
	summary := server.scanRecoverableAttempts(context.Background(), database, jobSupervisorTickOptions{AutoRetryFailed: true, Limit: 5}, &jobs)
	if summary["retryBackoff"] != 1 || summary["acceptedRetryAttempts"] != 0 {
		t.Fatalf("workflow retry policy was not applied: %+v", summary)
	}
	record := arrayMaps(summary["recoverableAttempts"])[0]
	if intValue(record["retryBackoffSeconds"]) != 7200 || intValue(record["maxRetryAttempts"]) != 1 {
		t.Fatalf("recoverable record missing contract policy: %+v", record)
	}
}

func TestDevFlowWorkspaceLifecycleSpecLocksRepositoryWorkspace(t *testing.T) {
	root := t.TempDir()
	server := NewServer(filepath.Join(root, "omega.db"), filepath.Join(root, "workspace"), filepath.Join(root, "openapi.yaml"))
	ctx := context.Background()
	item := map[string]any{"id": "item_lifecycle", "key": "OMG-lifecycle", "repositoryTargetId": "repo_lifecycle"}
	target := map[string]any{"id": "repo_lifecycle", "kind": "local", "path": filepath.Join(root, "target")}
	pipeline := makePipelineWithTemplate(item, &PipelineTemplate{ID: "devflow-pr", Name: "DevFlow PR cycle", StageProfiles: []StageProfile{{ID: "todo", Title: "Todo", Agent: "requirement"}}, Runtime: WorkflowRuntimeProfile{AttemptTimeoutMinutes: 7}})
	attempt := makeAttemptRecord(item, pipeline, "manual", "devflow-pr", "todo")

	lock, err := claimDevFlowWorkspaceLock(ctx, server, item, target, pipeline, attempt)
	if err != nil {
		t.Fatal(err)
	}
	if text(lock, "status") != "claimed" || text(lock, "repositoryPath") == "" || !pathInsideRoot(text(lock, "workspaceRoot"), text(lock, "repositoryPath")) {
		t.Fatalf("lock should describe bounded workspace: %+v", lock)
	}
	if _, err := claimDevFlowWorkspaceLock(ctx, server, item, target, pipeline, attempt); err == nil {
		t.Fatalf("expected second lock claim to fail")
	}
}

func TestWorkspaceCleanupRemovesRepoAndRetainsProof(t *testing.T) {
	root := t.TempDir()
	server := NewServer(filepath.Join(root, "omega.db"), filepath.Join(root, "workspace"), filepath.Join(root, "openapi.yaml"))
	workspace, err := workspaceChildPath(server.WorkspaceRoot, "OMG-cleanup")
	if err != nil {
		t.Fatal(err)
	}
	repoPath := filepath.Join(workspace, "repo")
	proofPath := filepath.Join(workspace, ".omega", "proof")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(proofPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "file.txt"), []byte("repo"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proofPath, "proof.md"), []byte("proof"), 0o644); err != nil {
		t.Fatal(err)
	}
	item := map[string]any{"id": "item_cleanup", "key": "OMG-cleanup", "repositoryTargetId": "repo_cleanup"}
	pipeline := makePipelineWithTemplate(item, &PipelineTemplate{ID: "devflow-pr", Name: "DevFlow", StageProfiles: []StageProfile{{ID: "todo", Title: "Todo", Agent: "requirement"}}, Runtime: WorkflowRuntimeProfile{CleanupRetentionSeconds: 1}})
	attempt := makeAttemptRecord(item, pipeline, "manual", "devflow-pr", "done")
	attempt["id"] = "attempt_cleanup"
	attempt["status"] = "done"
	attempt["workspacePath"] = workspace
	attempt["finishedAt"] = time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	database := &WorkspaceDatabase{Tables: WorkspaceTables{Pipelines: []map[string]any{pipeline}, Attempts: []map[string]any{attempt}}}

	summary := server.scanWorkspaceCleanup(context.Background(), database, workspaceCleanupOptions{AutoCleanupWorkspaces: true, WorkspaceRetentionSeconds: 1, Limit: 5})
	if summary["cleanedWorkspaces"] != 1 {
		t.Fatalf("cleanup summary = %+v", summary)
	}
	if pathExists(repoPath) {
		t.Fatalf("repo path should be removed: %s", repoPath)
	}
	if !pathExists(filepath.Join(proofPath, "proof.md")) {
		t.Fatalf("proof should be retained")
	}
	if text(mapValue(database.Tables.Attempts[0]["workspaceCleanup"]), "status") != "cleaned" {
		t.Fatalf("attempt cleanup metadata = %+v", database.Tables.Attempts[0])
	}
}

func TestJobSupervisorMarksOrphanedWorkerAttemptStalled(t *testing.T) {
	server := NewServer(filepath.Join(t.TempDir(), "omega.db"), filepath.Join(t.TempDir(), "workspace"), filepath.Join(t.TempDir(), "openapi.yaml"))
	item := map[string]any{"id": "item_orphan", "key": "OMG-orphan", "repositoryTargetId": "repo_orphan"}
	pipeline := makePipelineWithTemplate(item, &PipelineTemplate{ID: "devflow-pr", Name: "DevFlow", StageProfiles: []StageProfile{{ID: "todo", Title: "Todo", Agent: "requirement"}}})
	attempt := makeAttemptRecord(item, pipeline, "manual", "devflow-pr", "todo")
	attempt["id"] = "attempt_orphan"
	database := &WorkspaceDatabase{Tables: WorkspaceTables{WorkItems: []map[string]any{item}, Pipelines: []map[string]any{pipeline}, Attempts: []map[string]any{attempt}}}

	summary := server.scanWorkerHostLeases(context.Background(), database)
	if summary["orphanedWorkerAttempts"] != 1 || database.Tables.Attempts[0]["status"] != "stalled" {
		t.Fatalf("orphan summary=%+v attempt=%+v", summary, database.Tables.Attempts[0])
	}
}

func TestJobSupervisorScanRecoverableAttemptsRespectsBackoffAndLimit(t *testing.T) {
	server := NewServer(filepath.Join(t.TempDir(), "omega.db"), filepath.Join(t.TempDir(), "workspace"), filepath.Join(t.TempDir(), "openapi.yaml"))
	now := time.Now().UTC().Format(time.RFC3339Nano)
	database := &WorkspaceDatabase{Tables: WorkspaceTables{Attempts: []map[string]any{
		{"id": "attempt_recent_failed", "pipelineId": "pipeline_recent", "itemId": "item_recent", "status": "failed", "updatedAt": now, "errorMessage": "recent failure"},
		{"id": "attempt_limit_failed", "pipelineId": "pipeline_limit", "itemId": "item_limit", "status": "failed", "updatedAt": "2026-04-29T00:00:00Z", "retryRootAttemptId": "attempt_limit_root", "errorMessage": "limit failure"},
		{"id": "attempt_limit_retry_1", "pipelineId": "pipeline_limit", "itemId": "item_limit", "status": "done", "retryRootAttemptId": "attempt_limit_root"},
	}}}
	jobs := []map[string]any{}
	summary := server.scanRecoverableAttempts(context.Background(), database, jobSupervisorTickOptions{AutoRetryFailed: true, MaxRetryAttempts: 1, RetryBackoffSeconds: 3600, Limit: 5}, &jobs)
	if summary["retryBackoff"] != 1 || summary["retryLimitReached"] != 1 || summary["acceptedRetryAttempts"] != 0 || len(jobs) != 0 {
		t.Fatalf("policy summary=%+v jobs=%+v", summary, jobs)
	}
}

func TestRuntimeLogsAPIListsAndFiltersRecords(t *testing.T) {
	api, repo := newTestAPI(t)
	database := WorkspaceDatabase{Tables: WorkspaceTables{
		Requirements: []map[string]any{{
			"id": "req_runtime_1", "projectId": "project_omega", "source": "manual", "title": "Runtime logs", "rawText": "Trace approval",
			"structured": map[string]any{}, "acceptanceCriteria": []any{}, "risks": []any{}, "status": "converted", "createdAt": "2026-04-29T09:00:00Z", "updatedAt": "2026-04-29T09:00:00Z",
		}},
		WorkItems: []map[string]any{{
			"id": "item_runtime_1", "projectId": "project_omega", "key": "OMG-runtime", "title": "Runtime", "description": "Runtime", "status": "Running",
			"priority": "High", "assignee": "requirement", "labels": []any{}, "team": "Omega", "stageId": "human_review", "target": "target", "requirementId": "req_runtime_1",
			"createdAt": "2026-04-29T09:00:00Z", "updatedAt": "2026-04-29T09:00:00Z",
		}},
		Pipelines: []map[string]any{{"id": "pipeline_item_manual_1", "workItemId": "item_runtime_1", "runId": "run_runtime_1", "status": "running", "run": map[string]any{}, "createdAt": "2026-04-29T09:00:00Z", "updatedAt": "2026-04-29T09:00:00Z"}},
		Attempts:  []map[string]any{{"id": "attempt_missing", "itemId": "item_runtime_1", "pipelineId": "pipeline_item_manual_1", "status": "failed", "trigger": "manual", "runner": "codex", "startedAt": "2026-04-29T09:00:00Z", "stages": []any{}, "events": []any{}, "createdAt": "2026-04-29T09:00:00Z", "updatedAt": "2026-04-29T09:00:00Z"}},
	}}
	if err := repo.Save(context.Background(), database); err != nil {
		t.Fatal(err)
	}
	if err := repo.AppendRuntimeLog(context.Background(), RuntimeLogRecord{
		ID:         "log_error_1",
		Level:      "ERROR",
		EventType:  "checkpoint.approve.missing_attempt",
		Message:    "Missing attempt was detected.",
		PipelineID: "pipeline_item_manual_1",
		AttemptID:  "attempt_missing",
		Details:    map[string]any{"pipelineId": "pipeline_item_manual_1", "note": "approval failed in review gate"},
		CreatedAt:  "2026-04-29T10:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.AppendRuntimeLog(context.Background(), RuntimeLogRecord{
		ID:        "log_debug_1",
		Level:     "DEBUG",
		EventType: "api.request",
		Message:   "GET /health -> 200",
		Details:   map[string]any{"path": "/health"},
		CreatedAt: "2026-04-28T10:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.AppendRuntimeLog(context.Background(), RuntimeLogRecord{
		ID:            "log_req_1",
		Level:         "INFO",
		EventType:     "requirement.trace",
		Message:       "Requirement trace collected.",
		RequirementID: "req_runtime_1",
		Details:       map[string]any{"requirementId": "req_runtime_1"},
		CreatedAt:     "2026-04-29T09:30:00Z",
	}); err != nil {
		t.Fatal(err)
	}

	var logs []RuntimeLogRecord
	decode(t, mustGet(t, api.URL+"/runtime-logs?level=ERROR"), &logs)
	if len(logs) != 1 {
		t.Fatalf("filtered logs = %+v", logs)
	}
	if logs[0].Level != "ERROR" || logs[0].EventType != "checkpoint.approve.missing_attempt" || logs[0].PipelineID != "pipeline_item_manual_1" {
		t.Fatalf("error log = %+v", logs[0])
	}
	decode(t, mustGet(t, api.URL+"/runtime-logs?eventType=checkpoint.approve.missing_attempt&createdAfter=2026-04-29T00%3A00%3A00Z"), &logs)
	if len(logs) != 1 || logs[0].ID != "log_error_1" {
		t.Fatalf("time filtered logs = %+v", logs)
	}
	decode(t, mustGet(t, api.URL+"/runtime-logs?requirementId=req_runtime_1&q=approval"), &logs)
	if len(logs) != 1 || logs[0].ID != "log_error_1" {
		t.Fatalf("requirement/search filtered logs = %+v", logs)
	}
	var page RuntimeLogPage
	decode(t, mustGet(t, api.URL+"/runtime-logs?page=1&limit=1&requirementId=req_runtime_1"), &page)
	if len(page.Items) != 1 || !page.HasMore || page.NextCursor == "" || page.Items[0].ID != "log_error_1" {
		t.Fatalf("runtime log page = %+v", page)
	}
	var nextPage RuntimeLogPage
	decode(t, mustGet(t, api.URL+"/runtime-logs?page=1&limit=1&requirementId=req_runtime_1&cursor="+url.QueryEscape(page.NextCursor)), &nextPage)
	if len(nextPage.Items) != 1 || nextPage.Items[0].ID != "log_req_1" {
		t.Fatalf("runtime log next page = %+v", nextPage)
	}
	exportResponse := mustGet(t, api.URL+"/runtime-logs/export?format=csv&requirementId=req_runtime_1")
	defer exportResponse.Body.Close()
	if exportResponse.StatusCode != http.StatusOK {
		t.Fatalf("export status = %d", exportResponse.StatusCode)
	}
	rawExport, err := io.ReadAll(exportResponse.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rawExport), "requirementId") || !strings.Contains(string(rawExport), "Requirement trace collected.") {
		t.Fatalf("csv export = %s", string(rawExport))
	}

	var summary map[string]any
	decode(t, mustGet(t, api.URL+"/observability"), &summary)
	recentErrors := arrayMaps(summary["recentErrors"])
	if len(recentErrors) == 0 || recentErrors[0]["eventType"] != "checkpoint.approve.missing_attempt" {
		t.Fatalf("observability recent errors = %+v", recentErrors)
	}
}

func TestAttemptTimelineAggregatesRunRecords(t *testing.T) {
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)
	timestamp := "2026-04-29T10:00:00Z"
	database, err := repo.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	item := map[string]any{
		"id": "item_timeline", "projectId": "project_omega", "key": "OMG-timeline", "title": "Timeline", "description": "Show run trace.",
		"status": "In Review", "priority": "High", "assignee": "delivery", "labels": []any{"manual"}, "team": "Omega", "stageId": "human_review", "target": "ZYOOO/TestRepo", "createdAt": timestamp, "updatedAt": timestamp,
	}
	pipeline := makePipelineWithTemplate(item, &PipelineTemplate{
		ID:          "devflow-pr",
		Name:        "DevFlow PR cycle",
		Description: "DevFlow",
		StageProfiles: []StageProfile{
			{ID: "in_progress", Title: "Implementation", Agent: "coding"},
			{ID: "human_review", Title: "Human Review", Agent: "delivery", HumanGate: true},
		},
	})
	pipeline["id"] = "pipeline_timeline"
	run := mapValue(pipeline["run"])
	run["events"] = []map[string]any{{"type": "gate.created", "message": "Human review checkpoint opened.", "stageId": "human_review", "agentId": "delivery", "timestamp": "2026-04-29T10:03:00Z"}}
	stages := arrayMaps(run["stages"])
	stages[0]["status"] = "passed"
	stages[0]["completedAt"] = "2026-04-29T10:02:00Z"
	stages[1]["status"] = "needs-human"
	stages[1]["startedAt"] = "2026-04-29T10:03:00Z"
	run["stages"] = stages
	pipeline["run"] = run
	pipeline["status"] = "waiting-human"
	attempt := makeAttemptRecord(item, pipeline, "manual", "devflow-pr", "human_review")
	attempt["id"] = "attempt_timeline"
	attempt["events"] = []map[string]any{{"type": "attempt.started", "message": "Attempt started.", "stageId": "in_progress", "createdAt": "2026-04-29T10:01:00Z"}}
	checkpoint := map[string]any{
		"id": "checkpoint_timeline", "pipelineId": pipeline["id"], "attemptId": attempt["id"], "stageId": "human_review",
		"status": "pending", "title": "Human Review", "summary": "Waiting for approval.", "createdAt": "2026-04-29T10:03:00Z", "updatedAt": "2026-04-29T10:03:00Z",
	}
	operation := map[string]any{
		"id": "pipeline_timeline:agent:in_progress:coding", "missionId": "mission_pipeline_timeline_agent_workflow",
		"stageId": "in_progress", "agentId": "coding", "status": "passed", "summary": "Implementation completed.", "createdAt": "2026-04-29T10:01:00Z", "updatedAt": "2026-04-29T10:02:00Z",
	}
	proof := map[string]any{
		"id": "proof_timeline", "operationId": operation["id"], "label": "git-diff", "sourcePath": "/tmp/proof/git-diff.patch", "createdAt": "2026-04-29T10:02:30Z",
	}
	database.Tables.WorkItems = append(database.Tables.WorkItems, item)
	database.Tables.Pipelines = append(database.Tables.Pipelines, pipeline)
	database.Tables.Attempts = append(database.Tables.Attempts, attempt)
	database.Tables.Checkpoints = append(database.Tables.Checkpoints, checkpoint)
	database.Tables.Operations = append(database.Tables.Operations, operation)
	database.Tables.ProofRecords = append(database.Tables.ProofRecords, proof)
	if err := repo.Save(context.Background(), *database); err != nil {
		t.Fatal(err)
	}
	if err := repo.AppendRuntimeLog(context.Background(), RuntimeLogRecord{
		ID:         "log_timeline",
		Level:      "INFO",
		EventType:  "checkpoint.pending",
		Message:    "Checkpoint pending.",
		PipelineID: "pipeline_timeline",
		AttemptID:  "attempt_timeline",
		StageID:    "human_review",
		CreatedAt:  "2026-04-29T10:03:10Z",
	}); err != nil {
		t.Fatal(err)
	}

	var timeline AttemptTimelineResponse
	decode(t, mustGet(t, api.URL+"/attempts/attempt_timeline/timeline"), &timeline)
	if text(timeline.Attempt, "id") != "attempt_timeline" || text(timeline.Pipeline, "id") != "pipeline_timeline" {
		t.Fatalf("timeline identity = %+v", timeline)
	}
	found := map[string]bool{}
	previous := ""
	for _, item := range timeline.Items {
		found[item.Source] = true
		if previous != "" && item.Time < previous {
			t.Fatalf("timeline not sorted: %+v", timeline.Items)
		}
		previous = item.Time
	}
	for _, source := range []string{"attempt", "pipeline", "stage", "operation", "proof", "checkpoint", "runtime-log"} {
		if !found[source] {
			t.Fatalf("timeline missing source %s: %+v", source, timeline.Items)
		}
	}
}

func TestRunSupervisedCommandContextTimesOutProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell timeout test uses POSIX sh")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	process, err := runSupervisedCommandContext(ctx, t.TempDir(), "", "sh", "-c", "sleep 5")
	if err == nil {
		t.Fatalf("expected timeout error")
	}
	if process["status"] != "timed-out" || process["exitCode"] != -1 {
		t.Fatalf("process = %+v", process)
	}
	if process["cancellationReason"] != context.DeadlineExceeded.Error() {
		t.Fatalf("cancellation reason = %+v", process)
	}
}

func TestCompleteDevFlowCycleBackfillsMissingAttempt(t *testing.T) {
	root := t.TempDir()
	openAPI := filepath.Join(root, "openapi.yaml")
	if err := os.WriteFile(openAPI, []byte("openapi: 3.1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := NewServer(filepath.Join(root, "omega.db"), filepath.Join(root, "workspace"), openAPI)
	api := httptest.NewServer(server.Handler())
	t.Cleanup(api.Close)
	repo := server.Repo
	seedWorkspace(t, repo)
	item := map[string]any{
		"id": "item_manual_backfill", "key": "OMG-backfill", "title": "Backfill attempt", "description": "Recover missing attempt.",
		"status": "Ready", "priority": "High", "assignee": "requirement", "labels": []any{"manual"}, "team": "Omega", "stageId": "intake", "target": "No target",
	}
	_ = postJSON(t, api.URL+"/work-items", map[string]any{"item": item})

	var pipeline map[string]any
	decode(t, postJSON(t, api.URL+"/pipelines/from-template", map[string]any{"templateId": "devflow-pr", "item": item}), &pipeline)
	result := map[string]any{
		"status":         "waiting-human",
		"workspacePath":  "/tmp/omega-backfill",
		"branchName":     "omega/backfill",
		"pullRequestUrl": "https://github.com/ZYOOO/TestRepo/pull/999",
		"proofFiles":     []string{},
	}
	_, _, err := server.completeDevFlowCycleJob(context.Background(), text(pipeline, "id"), "missing-attempt-id", result)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := repo.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	index := findByID(updated.Tables.Attempts, "missing-attempt-id")
	if index < 0 {
		t.Fatalf("attempt was not backfilled: %+v", updated.Tables.Attempts)
	}
	if updated.Tables.Attempts[index]["status"] != "waiting-human" || updated.Tables.Attempts[index]["pullRequestUrl"] != result["pullRequestUrl"] {
		t.Fatalf("backfilled attempt = %+v", updated.Tables.Attempts[index])
	}
	var checkpoints []map[string]any
	decode(t, mustGet(t, api.URL+"/checkpoints"), &checkpoints)
	if len(checkpoints) != 1 || text(checkpoints[0], "attemptId") != "missing-attempt-id" {
		t.Fatalf("checkpoint attempt link = %+v", checkpoints)
	}
}

func TestRunCurrentPipelineStagePersistsOperationProofAndCheckpoint(t *testing.T) {
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)
	item := map[string]any{
		"id": "item_manual_1", "key": "OMG-1", "title": "Runnable pipeline item", "description": "Needs a local execution loop.",
		"status": "Ready", "priority": "High", "assignee": "requirement", "labels": []any{"manual"}, "team": "Omega", "stageId": "intake", "target": "No target",
	}
	_ = postJSON(t, api.URL+"/work-items", map[string]any{"item": item})

	var pipeline map[string]any
	decode(t, postJSON(t, api.URL+"/pipelines/from-template", map[string]any{"templateId": "feature", "item": item}), &pipeline)

	var result map[string]any
	decode(t, postJSON(t, api.URL+"/pipelines/"+pipeline["id"].(string)+"/run-current-stage", map[string]any{"runner": "local-proof"}), &result)
	updated := mapValue(result["pipeline"])
	if updated["status"] != "waiting-human" {
		t.Fatalf("pipeline status = %v", updated["status"])
	}
	operationResult := mapValue(result["operationResult"])
	if operationResult["status"] != "passed" || len(arrayValues(operationResult["proofFiles"])) == 0 {
		t.Fatalf("operation result = %+v", operationResult)
	}

	var checkpoints []map[string]any
	decode(t, mustGet(t, api.URL+"/checkpoints"), &checkpoints)
	if len(checkpoints) != 1 || checkpoints[0]["pipelineId"] != pipeline["id"] || checkpoints[0]["stageId"] != "intake" {
		t.Fatalf("checkpoints = %+v", checkpoints)
	}

	var operations []map[string]any
	decode(t, mustGet(t, api.URL+"/operations"), &operations)
	if len(operations) == 0 || operations[0]["id"] != "operation_intake" || operations[0]["status"] != "done" {
		t.Fatalf("operations = %+v", operations)
	}

	var proofs []map[string]any
	decode(t, mustGet(t, api.URL+"/proof-records"), &proofs)
	if len(proofs) == 0 || proofs[0]["operationId"] != "operation_intake" {
		t.Fatalf("proofs = %+v", proofs)
	}

	var attempts []map[string]any
	decode(t, mustGet(t, api.URL+"/attempts"), &attempts)
	if len(attempts) != 1 || attempts[0]["itemId"] != "item_manual_1" || attempts[0]["status"] != "waiting-human" {
		t.Fatalf("attempts = %+v", attempts)
	}
	if len(arrayMaps(attempts[0]["stages"])) == 0 || text(attempts[0], "workspacePath") == "" {
		t.Fatalf("attempt did not capture stage/workspace evidence: %+v", attempts[0])
	}
}

func TestRunDevFlowPRCycleCreatesBranchPRAndMergeProof(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required for devflow PR cycle")
	}
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)
	targetRepo := createDemoGitRepo(t)

	bin := t.TempDir()
	ghLog := filepath.Join(bin, "gh.log")
	gh := filepath.Join(bin, "gh")
	script := fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >> %s
if [ "$1" = "auth" ] && [ "$2" = "status" ]; then
  printf 'ok\n'
elif [ "$1" = "repo" ] && [ "$2" = "view" ]; then
  printf '{"nameWithOwner":"acme/demo","viewerPermission":"WRITE","defaultBranchRef":{"name":"main"}}'
elif [ "$1" = "pr" ] && [ "$2" = "list" ]; then
  printf '[]'
elif [ "$1" = "pr" ] && [ "$2" = "create" ]; then
  printf 'https://github.com/acme/demo/pull/123\n'
elif [ "$1" = "pr" ] && [ "$2" = "checks" ]; then
  printf 'no checks found\n'
else
  printf 'ok\n'
fi
`, ghLog)
	if err := os.WriteFile(gh, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	codex := filepath.Join(bin, "codex")
	codexScript := `#!/bin/sh
prompt="$(cat)"
output=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--output-last-message" ]; then
    shift
    output="$1"
  fi
  shift
done
if printf '%s' "$prompt" | grep -q "You are the review agent for Omega"; then
  if [ -n "$output" ]; then
    cat > "$output" <<'EOF'
# Review

Verdict: APPROVED

The diff satisfies the requirement and the validation output is acceptable.
EOF
  fi
  echo "fake review agent approved"
  exit 0
fi
mkdir -p scripts examples test
cat > scripts/task-summary.mjs <<'EOF'
#!/usr/bin/env node
import { readFileSync } from "node:fs";
const file = process.argv[2];
if (!file) {
  console.error("Usage: task-summary <markdown-file>");
  process.exit(1);
}
let text;
try {
  text = readFileSync(file, "utf8");
} catch {
  console.error("Cannot read markdown file");
  process.exit(1);
}
const done = (text.match(/- \[[xX]\]/g) ?? []).length;
const pending = (text.match(/- \[ \]/g) ?? []).length;
const total = done + pending;
console.log(JSON.stringify({ total, done, pending, completionRate: total === 0 ? 0 : done / total }));
EOF
cat > examples/tasks.md <<'EOF'
- [x] one
- [ ] two
- [X] three
EOF
cat > test/task-summary.test.mjs <<'EOF'
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
const result = spawnSync(process.execPath, ["scripts/task-summary.mjs", "examples/tasks.md"], { encoding: "utf8" });
assert.equal(result.status, 0);
assert.deepEqual(JSON.parse(result.stdout), { total: 3, done: 2, pending: 1, completionRate: 2 / 3 });
EOF
if [ -n "$output" ]; then
  echo "fake coding agent completed" > "$output"
fi
echo "fake coding agent completed"
`
	if err := os.WriteFile(codex, []byte(codexScript), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	var database WorkspaceDatabase
	decode(t, postJSON(t, api.URL+"/github/bind-repository-target", map[string]any{
		"owner":         "acme",
		"repo":          "demo",
		"nameWithOwner": "acme/demo",
		"defaultBranch": "main",
		"url":           targetRepo,
	}), &database)
	item := map[string]any{
		"id": "item_devflow_1", "key": "OMG-99", "title": "Add task summary CLI", "description": "Create a Node.js CLI at scripts/task-summary.mjs that reads examples/tasks.md and reports task totals as JSON. Add a node:test test.",
		"status": "Ready", "priority": "High", "assignee": "requirement", "labels": []any{"manual"}, "team": "Omega", "stageId": "intake",
		"target": targetRepo, "source": "manual", "repositoryTargetId": "repo_acme_demo",
	}
	decode(t, postJSON(t, api.URL+"/work-items", map[string]any{"item": item}), &database)

	var pipeline map[string]any
	decode(t, postJSON(t, api.URL+"/pipelines/from-template", map[string]any{"templateId": "devflow-pr", "item": item}), &pipeline)

	var result map[string]any
	decode(t, postJSON(t, api.URL+"/pipelines/"+text(pipeline, "id")+"/run-devflow-cycle", map[string]any{"autoApproveHuman": false, "autoMerge": false, "wait": true}), &result)
	if result["status"] != "waiting-human" || result["pullRequestUrl"] != "https://github.com/acme/demo/pull/123" {
		t.Fatalf("cycle result = %+v", result)
	}
	if !strings.HasPrefix(text(result, "workspacePath"), filepath.Join(filepath.Dir(repo.Path), "workspace")) {
		t.Fatalf("workspace path = %v", result["workspacePath"])
	}
	proofFiles := stringSlice(result["proofFiles"])
	for _, want := range []string{
		"requirement-artifact.json",
		"solution-plan.md",
		"implementation-summary.md",
		"test-report.md",
		"code-review-round-1.md",
		"code-review-round-2.md",
		"human-review-request.md",
		"attempt-review-packet.json",
		"attempt-run-report.md",
		"handoff-bundle.json",
	} {
		if !containsSuffix(proofFiles, want) {
			t.Fatalf("proof files missing %s: %+v", want, proofFiles)
		}
	}
	proofDir := filepath.Join(text(result, "workspacePath"), ".omega", "proof")
	var handoff map[string]any
	handoffRaw, err := os.ReadFile(filepath.Join(proofDir, "handoff-bundle.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(handoffRaw, &handoff); err != nil {
		t.Fatal(err)
	}
	if handoff["repositoryTargetId"] != "repo_acme_demo" || handoff["pipelineId"] != text(pipeline, "id") {
		t.Fatalf("handoff bundle identity = %+v", handoff)
	}
	if artifacts := arrayMaps(handoff["artifacts"]); len(artifacts) < 7 {
		t.Fatalf("handoff artifacts = %+v", artifacts)
	}
	reviewPacket := mapValue(handoff["reviewPacket"])
	if text(mapValue(reviewPacket["diffPreview"]), "summary") == "" || text(mapValue(reviewPacket["testPreview"]), "status") == "" || text(mapValue(reviewPacket["risk"]), "level") == "" || len(arrayMaps(reviewPacket["recommendedActions"])) == 0 {
		t.Fatalf("handoff review packet = %+v", reviewPacket)
	}
	packetRaw, err := os.ReadFile(filepath.Join(proofDir, "attempt-review-packet.json"))
	if err != nil {
		t.Fatal(err)
	}
	var packetFile map[string]any
	if err := json.Unmarshal(packetRaw, &packetFile); err != nil {
		t.Fatal(err)
	}
	if text(mapValue(packetFile["diffPreview"]), "summary") != text(mapValue(reviewPacket["diffPreview"]), "summary") {
		t.Fatalf("packet file should match handoff packet: file=%+v handoff=%+v", packetFile, reviewPacket)
	}
	reportRaw, err := os.ReadFile(filepath.Join(proofDir, "attempt-run-report.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(reportRaw), "Attempt Run Report") || !strings.Contains(string(reportRaw), "Diff Preview") || !strings.Contains(string(reportRaw), "Recommended Actions") || !strings.Contains(string(reportRaw), "https://github.com/acme/demo/pull/123") {
		t.Fatalf("attempt report = %s", string(reportRaw))
	}
	planRaw, err := os.ReadFile(filepath.Join(proofDir, "solution-plan.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(planRaw), "acme/demo") || !strings.Contains(string(planRaw), "implement the requested product change") {
		t.Fatalf("solution plan = %s", string(planRaw))
	}
	logRaw, err := os.ReadFile(ghLog)
	if err != nil {
		t.Fatal(err)
	}
	logText := string(logRaw)
	if !strings.Contains(logText, "auth setup-git") || !strings.Contains(logText, "pr create") ||
		!strings.Contains(logText, "pr diff") || !strings.Contains(logText, "pr checks") {
		t.Fatalf("gh log = %s", logText)
	}

	var checkpoints []map[string]any
	decode(t, mustGet(t, api.URL+"/checkpoints"), &checkpoints)
	if len(checkpoints) != 1 || checkpoints[0]["stageId"] != "human_review" || checkpoints[0]["status"] != "pending" {
		t.Fatalf("checkpoints = %+v", checkpoints)
	}
	var approved map[string]any
	decode(t, postJSON(t, api.URL+"/checkpoints/"+text(checkpoints[0], "id")+"/approve", map[string]any{"reviewer": "alice"}), &approved)
	if approved["status"] != "approved" {
		t.Fatalf("approved checkpoint = %+v", approved)
	}
	logRaw, err = os.ReadFile(ghLog)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logRaw), "pr merge") {
		t.Fatalf("gh log after approval = %s", string(logRaw))
	}

	var pipelines []map[string]any
	decode(t, mustGet(t, api.URL+"/pipelines"), &pipelines)
	if len(pipelines) != 1 || pipelines[0]["status"] != "done" {
		t.Fatalf("pipelines = %+v", pipelines)
	}
	for _, stage := range arrayMaps(mapValue(pipelines[0]["run"])["stages"]) {
		if stage["status"] != "passed" {
			t.Fatalf("stage was not marked passed after devflow cycle: %+v", stage)
		}
	}
	var workspaces WorkspaceDatabase
	decode(t, mustGet(t, api.URL+"/workspace"), &workspaces)
	if len(workspaces.Tables.ProofRecords) == 0 {
		t.Fatalf("proof records were not persisted")
	}
	if len(workspaces.Tables.Attempts) != 1 {
		t.Fatalf("attempt records were not persisted: %+v", workspaces.Tables.Attempts)
	}
	if len(workspaces.Tables.Operations) < 6 {
		t.Fatalf("agent operation records were not persisted: %+v", workspaces.Tables.Operations)
	}
	seenCoding := false
	for _, operation := range workspaces.Tables.Operations {
		if operation["agentId"] == "coding" && operation["status"] == "passed" && strings.Contains(text(operation, "prompt"), "scripts/task-summary.mjs") {
			seenCoding = true
		}
	}
	if !seenCoding {
		t.Fatalf("coding agent invocation was not recorded: %+v", workspaces.Tables.Operations)
	}
	attempt := workspaces.Tables.Attempts[0]
	if attempt["status"] != "done" || attempt["pullRequestUrl"] != "https://github.com/acme/demo/pull/123" || attempt["repositoryTargetId"] != "repo_acme_demo" {
		t.Fatalf("attempt = %+v", attempt)
	}
	if len(arrayMaps(attempt["stages"])) < 7 || text(attempt, "branchName") == "" {
		t.Fatalf("attempt stage/branch evidence missing: %+v", attempt)
	}
}

func TestApproveDevFlowCheckpointIgnoresBranchCleanupFailure(t *testing.T) {
	root := t.TempDir()
	server := NewServer(filepath.Join(root, "omega.db"), filepath.Join(root, "workspace"), filepath.Join(root, "openapi.yaml"))
	repoWorkspace := filepath.Join(root, "workspace", "OMG-cleanup", "repo")
	proofDir := filepath.Join(root, "workspace", "OMG-cleanup", ".omega", "proof")
	if err := os.MkdirAll(repoWorkspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(proofDir, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoWorkspace, "init")
	runGit(t, repoWorkspace, "checkout", "-b", "main")
	runGit(t, repoWorkspace, "config", "user.email", "omega-test@example.local")
	runGit(t, repoWorkspace, "config", "user.name", "Omega Test")
	if err := os.WriteFile(filepath.Join(repoWorkspace, "README.md"), []byte("demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoWorkspace, "add", ".")
	runGit(t, repoWorkspace, "commit", "-m", "initial")
	if err := writeJSONFile(filepath.Join(proofDir, "handoff-bundle.json"), map[string]any{"merged": false}); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	gh := filepath.Join(bin, "gh")
	script := "#!/bin/sh\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"merge\" ]; then\n  printf 'merged\\n'\n  exit 0\nfi\nprintf 'MERGED\\n'\n"
	if err := os.WriteFile(gh, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	item := map[string]any{"id": "item_cleanup_approve", "key": "OMG-cleanup", "repositoryTargetId": "repo_cleanup"}
	pipeline := makePipelineWithTemplate(item, findPipelineTemplate("devflow-pr"))
	pipeline["id"] = "pipeline_cleanup_approve"
	attempt := makeAttemptRecord(item, pipeline, "manual", "devflow-pr", "human_review")
	attempt["id"] = "attempt_cleanup_approve"
	attempt["status"] = "waiting-human"
	attempt["workspacePath"] = filepath.Join(root, "workspace", "OMG-cleanup")
	attempt["pullRequestUrl"] = "https://github.com/acme/demo/pull/1"
	attempt["branchName"] = "omega/missing-branch"
	checkpoint := map[string]any{"id": "checkpoint_cleanup_approve", "pipelineId": text(pipeline, "id"), "attemptId": text(attempt, "id"), "stageId": "human_review", "status": "pending"}
	database := WorkspaceDatabase{Tables: WorkspaceTables{WorkItems: []map[string]any{item}, Pipelines: []map[string]any{pipeline}, Attempts: []map[string]any{attempt}, Checkpoints: []map[string]any{checkpoint}}}

	if err := server.completeApprovedDevFlowCheckpoint(&database, checkpoint, "alice"); err != nil {
		t.Fatal(err)
	}
	if database.Tables.Pipelines[0]["status"] != "done" || database.Tables.Attempts[0]["status"] != "done" {
		t.Fatalf("approval should complete despite branch cleanup failure: pipeline=%+v attempt=%+v", database.Tables.Pipelines[0], database.Tables.Attempts[0])
	}
}

func TestApproveDevFlowCheckpointCanContinueDeliveryAsync(t *testing.T) {
	api, repo := newTestAPI(t)
	root := t.TempDir()
	repoWorkspace := filepath.Join(root, "workspace", "OMG-async", "repo")
	proofDir := filepath.Join(root, "workspace", "OMG-async", ".omega", "proof")
	if err := os.MkdirAll(repoWorkspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(proofDir, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoWorkspace, "init")
	runGit(t, repoWorkspace, "checkout", "-b", "main")
	runGit(t, repoWorkspace, "config", "user.email", "omega-test@example.local")
	runGit(t, repoWorkspace, "config", "user.name", "Omega Test")
	if err := os.WriteFile(filepath.Join(repoWorkspace, "README.md"), []byte("demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoWorkspace, "add", ".")
	runGit(t, repoWorkspace, "commit", "-m", "initial")
	if err := writeJSONFile(filepath.Join(proofDir, "handoff-bundle.json"), map[string]any{"merged": false}); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	gh := filepath.Join(bin, "gh")
	script := "#!/bin/sh\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"merge\" ]; then\n  sleep 1\n  printf 'merged\\n'\n  exit 0\nfi\nprintf 'MERGED\\n'\n"
	if err := os.WriteFile(gh, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	item := map[string]any{"id": "item_async_approve", "projectId": "project_omega", "key": "OMG-async", "title": "Async approve", "status": "In Review", "priority": "High", "assignee": "delivery", "labels": []any{"manual"}, "team": "Omega", "stageId": "human_review", "target": "repo", "repositoryTargetId": "repo_async"}
	pipeline := makePipelineWithTemplate(item, findPipelineTemplate("devflow-pr"))
	pipeline["id"] = "pipeline_async_approve"
	pipeline["status"] = "waiting-human"
	attempt := makeAttemptRecord(item, pipeline, "manual", "devflow-pr", "human_review")
	attempt["id"] = "attempt_async_approve"
	attempt["status"] = "waiting-human"
	attempt["workspacePath"] = filepath.Join(root, "workspace", "OMG-async")
	attempt["pullRequestUrl"] = "https://github.com/acme/demo/pull/2"
	attempt["branchName"] = "omega/missing-async-branch"
	checkpoint := map[string]any{"id": "checkpoint_async_approve", "pipelineId": text(pipeline, "id"), "attemptId": text(attempt, "id"), "stageId": "human_review", "status": "pending", "title": "Human Review", "summary": "Approve delivery."}
	database := defaultWorkspaceDatabase()
	database.Tables.WorkItems = append(database.Tables.WorkItems, item)
	database.Tables.Pipelines = append(database.Tables.Pipelines, pipeline)
	database.Tables.Attempts = append(database.Tables.Attempts, attempt)
	database.Tables.Checkpoints = append(database.Tables.Checkpoints, checkpoint)
	if err := repo.Save(context.Background(), database); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	var approved map[string]any
	decode(t, postJSON(t, api.URL+"/checkpoints/checkpoint_async_approve/approve", map[string]any{"reviewer": "alice", "asyncDelivery": true}), &approved)
	if elapsed := time.Since(start); elapsed > 900*time.Millisecond {
		t.Fatalf("async approval waited for merge: %s", elapsed)
	}
	if approved["status"] != "approved" {
		t.Fatalf("approved response = %+v", approved)
	}
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		updated, err := repo.Load(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		pipelineIndex := findByID(updated.Tables.Pipelines, "pipeline_async_approve")
		attemptIndex := findByID(updated.Tables.Attempts, "attempt_async_approve")
		if pipelineIndex >= 0 && attemptIndex >= 0 && updated.Tables.Pipelines[pipelineIndex]["status"] == "done" && updated.Tables.Attempts[attemptIndex]["status"] == "done" {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	updated, _ := repo.Load(context.Background())
	t.Fatalf("async delivery did not complete: pipelines=%+v attempts=%+v", updated.Tables.Pipelines, updated.Tables.Attempts)
}

func TestRequirementDecomposeProducesStructuredArtifacts(t *testing.T) {
	api, _ := newTestAPI(t)

	var result map[string]any
	decode(t, postJSON(t, api.URL+"/requirements/decompose", map[string]any{
		"title":            "Add GitHub PR delivery",
		"description":      "When coding is done, create a PR with diff summary and expose checks as delivery proof.",
		"repositoryTarget": "/Users/demo/Omega",
	}), &result)

	if result["summary"] != "Add GitHub PR delivery" || result["repositoryTarget"] != "/Users/demo/Omega" {
		t.Fatalf("decomposition result = %+v", result)
	}
	if criteria := arrayValues(result["acceptanceCriteria"]); len(criteria) < 3 {
		t.Fatalf("acceptance criteria = %+v", criteria)
	}
	if risks := arrayValues(result["risks"]); len(risks) == 0 {
		t.Fatalf("risks = %+v", risks)
	}
	items := arrayMaps(result["suggestedWorkItems"])
	if len(items) < 2 || items[0]["stageId"] != "intake" || items[1]["stageId"] != "solution" {
		t.Fatalf("suggested items = %+v", items)
	}
}

func TestDemoCodeRunnerCreatesBranchCommitAndDiffProof(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required for demo-code runner")
	}
	api, _ := newTestAPI(t)
	targetRepo := createDemoGitRepo(t)

	mission := map[string]any{
		"id":               "mission_OMG-9_coding",
		"sourceIssueKey":   "OMG-9",
		"sourceWorkItemId": "item_manual_9",
		"title":            "Add visible demo code",
		"target":           targetRepo,
		"operations": []map[string]any{{
			"id":            "operation_coding",
			"stageId":       "coding",
			"agentId":       "coding",
			"status":        "ready",
			"prompt":        "Add a small exported TypeScript module that proves the coding stage can modify the repository.",
			"requiredProof": []any{"diff", "commit"},
		}},
	}

	var result OperationResult
	decode(t, postJSON(t, api.URL+"/operations/run", map[string]any{"mission": mission, "operationId": "operation_coding", "runner": "demo-code"}), &result)
	if result.Status != "passed" {
		t.Fatalf("operation status = %s stderr=%s stdout=%s", result.Status, result.Stderr, result.Stdout)
	}
	if !containsSuffix(result.ProofFiles, "git-diff.patch") || !containsSuffix(result.ProofFiles, "change-summary.md") {
		t.Fatalf("proof files = %+v", result.ProofFiles)
	}

	workspaceRepo := filepath.Join(result.WorkspacePath, "repo")
	generated := filepath.Join(workspaceRepo, "src", "omega-demo-change.ts")
	raw, err := os.ReadFile(generated)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "omegaDemoChange") || !strings.Contains(string(raw), "OMG-9") {
		t.Fatalf("generated code = %s", string(raw))
	}
	if branch := runGit(t, workspaceRepo, "branch", "--show-current"); !strings.HasPrefix(strings.TrimSpace(branch), "omega/OMG-9-coding") {
		t.Fatalf("branch = %s", branch)
	}
	if subject := runGit(t, workspaceRepo, "log", "-1", "--pretty=%s"); !strings.Contains(subject, "Omega demo code change for OMG-9") {
		t.Fatalf("commit subject = %s", subject)
	}
	diff, err := os.ReadFile(filepath.Join(result.WorkspacePath, ".omega", "proof", "git-diff.patch"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(diff), "omegaDemoChange") {
		t.Fatalf("diff proof = %s", string(diff))
	}
}

func TestCodexRunnerClonesTargetRepoAndCommitsChanges(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required for codex target runner")
	}
	if runtime.GOOS == "windows" {
		t.Skip("fake codex script uses POSIX sh")
	}
	api, _ := newTestAPI(t)
	targetRepo := createDemoGitRepo(t)
	bin := t.TempDir()
	codex := filepath.Join(bin, "codex")
	script := "#!/bin/sh\nmkdir -p src\nprintf '%s\\n' 'export const codexGenerated = true;' > src/codex-generated.ts\nprintf '%s\\n' 'fake codex changed repository'\n"
	if err := os.WriteFile(codex, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	mission := map[string]any{
		"id":               "mission_OMG-10_coding",
		"sourceIssueKey":   "OMG-10",
		"sourceWorkItemId": "item_manual_10",
		"title":            "Use Codex to modify code",
		"target":           targetRepo,
		"operations": []map[string]any{{
			"id":      "operation_coding",
			"stageId": "coding",
			"agentId": "coding",
			"status":  "ready",
			"prompt":  "Use Codex to add a repository change.",
		}},
	}

	var result OperationResult
	decode(t, postJSON(t, api.URL+"/operations/run", map[string]any{"mission": mission, "operationId": "operation_coding", "runner": "codex"}), &result)
	if result.Status != "passed" {
		t.Fatalf("operation status = %s stderr=%s stdout=%s", result.Status, result.Stderr, result.Stdout)
	}
	if !containsSuffix(result.ProofFiles, "git-diff.patch") || result.CommitSha == "" {
		t.Fatalf("operation result = %+v", result)
	}
	if _, err := os.Stat(filepath.Join(result.WorkspacePath, "repo", "src", "codex-generated.ts")); err != nil {
		t.Fatal(err)
	}
	if subject := runGit(t, filepath.Join(result.WorkspacePath, "repo"), "log", "-1", "--pretty=%s"); !strings.Contains(subject, "Omega codex code change for OMG-10") {
		t.Fatalf("commit subject = %s", subject)
	}
}

func TestCodexRunnerFailureCapturesSupervisedProcessResult(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake codex script uses POSIX sh")
	}
	api, _ := newTestAPI(t)
	bin := t.TempDir()
	codex := filepath.Join(bin, "codex")
	script := "#!/bin/sh\nprintf '%s\\n' 'codex stdout before failure'\nprintf '%s\\n' 'codex stderr before failure' >&2\nexit 7\n"
	if err := os.WriteFile(codex, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	mission := map[string]any{
		"id":               "mission_OMG-11_coding",
		"sourceIssueKey":   "OMG-11",
		"sourceWorkItemId": "item_manual_11",
		"title":            "Capture codex failure",
		"target":           "No target",
		"operations": []map[string]any{{
			"id":      "operation_coding",
			"stageId": "coding",
			"agentId": "coding",
			"status":  "ready",
			"prompt":  "Fail in a controlled way.",
		}},
	}

	var result OperationResult
	decode(t, postJSON(t, api.URL+"/operations/run", map[string]any{"mission": mission, "operationId": "operation_coding", "runner": "codex"}), &result)
	if result.Status != "failed" {
		t.Fatalf("operation status = %s result=%+v", result.Status, result)
	}
	if !strings.Contains(result.Stdout, "codex stdout before failure") || !strings.Contains(result.Stderr, "codex stderr before failure") {
		t.Fatalf("stdout/stderr not captured separately: stdout=%q stderr=%q", result.Stdout, result.Stderr)
	}
	if result.RunnerProcess["exitCode"] != float64(7) || result.RunnerProcess["status"] != "failed" {
		t.Fatalf("runner process = %+v", result.RunnerProcess)
	}
}

func TestSupervisedCommandStreamsOutputAndHeartbeatEvents(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script uses POSIX sh")
	}
	var mu sync.Mutex
	seen := []SupervisedCommandEvent{}
	process, err := runSupervisedCommandContextWithOptions(context.Background(), SupervisedCommandOptions{
		HeartbeatInterval: 20 * time.Millisecond,
		OnEvent: func(event SupervisedCommandEvent) {
			mu.Lock()
			defer mu.Unlock()
			seen = append(seen, event)
		},
	}, t.TempDir(), "", "sh", "-c", "printf 'stream stdout'; sleep 0.08; printf 'stream stderr' >&2; sleep 0.08")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text(process, "stdout"), "stream stdout") || !strings.Contains(text(process, "stderr"), "stream stderr") {
		t.Fatalf("process output = %+v", process)
	}
	events := arrayMaps(process["events"])
	if !hasProcessStream(events, "stdout") || !hasProcessStream(events, "stderr") || !hasProcessStream(events, "heartbeat") {
		t.Fatalf("process events = %+v", events)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) < 3 {
		t.Fatalf("callback events = %+v", seen)
	}
}

func TestRunnerHeartbeatRefreshesAttemptAndLogsTrace(t *testing.T) {
	root := t.TempDir()
	server := NewServer(filepath.Join(root, "omega.db"), filepath.Join(root, "workspace"), filepath.Join(root, "openapi.yaml"))
	seedWorkspace(t, server.Repo)
	database, err := server.Repo.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	item := map[string]any{"id": "item_runner_heartbeat", "projectId": "project_omega", "key": "OMG-heartbeat", "title": "Heartbeat", "status": "In Review", "repositoryTargetId": "repo_heartbeat", "createdAt": nowISO(), "updatedAt": nowISO()}
	pipeline := makePipelineWithTemplate(item, findPipelineTemplate("devflow-pr"))
	attempt := makeAttemptRecord(item, pipeline, "manual", "devflow-pr", "in_progress")
	attempt["id"] = "attempt_runner_heartbeat"
	attempt["lastSeenAt"] = "2026-04-29T00:00:00Z"
	database.Tables.WorkItems = append(database.Tables.WorkItems, item)
	database.Tables.Pipelines = append(database.Tables.Pipelines, pipeline)
	database.Tables.Attempts = append(database.Tables.Attempts, attempt)
	if err := server.Repo.Save(context.Background(), *database); err != nil {
		t.Fatal(err)
	}

	server.recordAttemptRunnerHeartbeat(context.Background(), text(pipeline, "id"), text(item, "id"), "attempt_runner_heartbeat", "in_progress", "coding", "codex", SupervisedCommandEvent{
		Stream:    "stdout",
		Chunk:     "still working",
		CreatedAt: "2026-04-29T00:01:00Z",
	})
	updated, err := server.Repo.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	updatedAttempt := updated.Tables.Attempts[findByID(updated.Tables.Attempts, "attempt_runner_heartbeat")]
	if text(updatedAttempt, "lastSeenAt") != "2026-04-29T00:01:00Z" {
		t.Fatalf("attempt heartbeat = %+v", updatedAttempt)
	}
	if !containsEventType(arrayMaps(updatedAttempt["events"]), "runner.stdout") {
		t.Fatalf("attempt events = %+v", updatedAttempt["events"])
	}
	logs, err := server.Repo.ListRuntimeLogs(context.Background(), map[string]string{"eventType": "runner.stdout"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].AttemptID != "attempt_runner_heartbeat" {
		t.Fatalf("runtime logs = %+v", logs)
	}
}

func TestRunOperationPersistsRunnerProcessResult(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake codex script uses POSIX sh")
	}
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)
	bin := t.TempDir()
	codex := filepath.Join(bin, "codex")
	script := "#!/bin/sh\nprintf '%s\\n' 'supervised codex ok'\n"
	if err := os.WriteFile(codex, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	mission := map[string]any{
		"id":               "mission_OMG-12_review",
		"sourceIssueKey":   "OMG-12",
		"sourceWorkItemId": "item_manual_12",
		"title":            "Persist runner process",
		"target":           "No target",
		"operations": []map[string]any{{
			"id":      "operation_review",
			"stageId": "review",
			"agentId": "review",
			"status":  "ready",
			"prompt":  "Review this change.",
		}},
	}
	var result OperationResult
	decode(t, postJSON(t, api.URL+"/operations/run", map[string]any{"mission": mission, "operationId": "operation_review", "runner": "codex"}), &result)
	if result.Status != "passed" {
		t.Fatalf("result = %+v", result)
	}

	var operations []map[string]any
	decode(t, mustGet(t, api.URL+"/operations"), &operations)
	if len(operations) != 1 {
		t.Fatalf("operations = %+v", operations)
	}
	process := mapValue(operations[0]["runnerProcess"])
	if process["status"] != "passed" || process["exitCode"] != float64(0) || !strings.Contains(text(process, "stdout"), "supervised codex ok") {
		t.Fatalf("persisted runner process = %+v", process)
	}
}

func TestCreateMissionCarriesWorkItemRepositoryTarget(t *testing.T) {
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)
	decode(t, postJSON(t, api.URL+"/github/bind-repository-target", map[string]any{
		"owner":         "acme",
		"repo":          "demo",
		"nameWithOwner": "acme/demo",
		"defaultBranch": "main",
		"url":           "https://github.com/acme/demo",
	}), &WorkspaceDatabase{})
	item := map[string]any{
		"id": "item_manual_9", "key": "OMG-9", "title": "Targeted item", "description": "Needs repo context.",
		"status": "Ready", "priority": "High", "assignee": "coding", "labels": []any{"manual"}, "team": "Omega", "stageId": "coding",
		"target": "https://github.com/acme/demo/issues/42", "repositoryTargetId": "repo_acme_demo",
	}

	var mission map[string]any
	decode(t, postJSON(t, api.URL+"/missions/from-work-item", map[string]any{"item": item}), &mission)
	if mission["target"] != "https://github.com/acme/demo" || mission["repositoryTargetId"] != "repo_acme_demo" || mission["repositoryTargetLabel"] != "acme/demo" {
		t.Fatalf("mission target fields = %+v", mission)
	}
	operation := arrayMaps(mission["operations"])[0]
	if !strings.Contains(text(operation, "prompt"), "Repository target ID: repo_acme_demo") ||
		!strings.Contains(text(operation, "prompt"), "Repository target: https://github.com/acme/demo") {
		t.Fatalf("operation prompt missing repository boundary: %s", text(operation, "prompt"))
	}
}

func TestCreateMissionRejectsMissingRepositoryTarget(t *testing.T) {
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)
	item := map[string]any{
		"id": "item_manual_9", "key": "OMG-9", "title": "Targeted item", "description": "Needs repo context.",
		"status": "Ready", "priority": "High", "assignee": "coding", "labels": []any{"manual"}, "team": "Omega", "stageId": "coding",
		"target": "No target", "repositoryTargetId": "repo_missing",
	}

	response := postJSON(t, api.URL+"/missions/from-work-item", map[string]any{"item": item})
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", response.StatusCode)
	}
}

func TestOpenAPIAndHealth(t *testing.T) {
	api, _ := newTestAPI(t)
	health, err := http.Get(api.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	decode(t, health, &body)
	if body["implementation"] != "go" {
		t.Fatalf("health implementation = %v", body["implementation"])
	}

	openapi, err := http.Get(api.URL + "/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if openapi.StatusCode != http.StatusOK {
		t.Fatalf("openapi status = %d", openapi.StatusCode)
	}
}

func TestMigrationsAndPipelineTemplates(t *testing.T) {
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)
	item := map[string]any{
		"id": "item_manual_1", "key": "OMG-1", "title": "Template item", "description": "Uses bugfix template.",
		"status": "Ready", "priority": "High", "assignee": "requirement", "labels": []any{"manual"}, "team": "Omega", "stageId": "intake", "target": "No target",
	}

	var migrations []map[string]any
	migrationsResponse, err := http.Get(api.URL + "/migrations")
	if err != nil {
		t.Fatal(err)
	}
	decode(t, migrationsResponse, &migrations)
	if len(migrations) == 0 || migrations[0]["version"] != "20260424_001" {
		t.Fatalf("migrations = %+v", migrations)
	}

	var templates []map[string]any
	templatesResponse, err := http.Get(api.URL + "/pipeline-templates")
	if err != nil {
		t.Fatal(err)
	}
	decode(t, templatesResponse, &templates)
	if len(templates) < 3 {
		t.Fatalf("template count = %d", len(templates))
	}

	var pipeline map[string]any
	decode(t, postJSON(t, api.URL+"/pipelines/from-template", map[string]any{"item": item, "templateId": "bugfix"}), &pipeline)
	if pipeline["templateId"] != "bugfix" {
		t.Fatalf("template id = %v", pipeline["templateId"])
	}
	stages := arrayMaps(mapValue(pipeline["run"])["stages"])
	if len(stages) != 5 {
		t.Fatalf("bugfix stage count = %d", len(stages))
	}
	if stages[1]["id"] != "coding" {
		t.Fatalf("second bugfix stage = %v", stages[1]["id"])
	}
}

func TestLLMProviderSelectionAndAgentDefinitions(t *testing.T) {
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)

	var providers []map[string]any
	providersResponse, err := http.Get(api.URL + "/llm-providers")
	if err != nil {
		t.Fatal(err)
	}
	decode(t, providersResponse, &providers)
	if len(providers) < 2 {
		t.Fatalf("provider count = %d", len(providers))
	}

	var defaultSelection map[string]any
	defaultResponse, err := http.Get(api.URL + "/llm-provider-selection")
	if err != nil {
		t.Fatal(err)
	}
	decode(t, defaultResponse, &defaultSelection)
	if defaultSelection["providerId"] != "openai" {
		t.Fatalf("default provider = %v", defaultSelection["providerId"])
	}

	nextSelection := map[string]any{"providerId": "openai-compatible", "model": "qwen-plus", "reasoningEffort": "medium"}
	var saved map[string]any
	decode(t, requestJSON(t, http.MethodPut, api.URL+"/llm-provider-selection", nextSelection), &saved)
	if saved["providerId"] != "openai-compatible" {
		t.Fatalf("saved provider = %v", saved["providerId"])
	}

	var agents []map[string]any
	agentsResponse, err := http.Get(api.URL + "/agent-definitions")
	if err != nil {
		t.Fatal(err)
	}
	decode(t, agentsResponse, &agents)
	if len(agents) < 6 {
		t.Fatalf("agent count = %d", len(agents))
	}
	defaultModel := mapValue(agents[0]["defaultModel"])
	if defaultModel["providerId"] != "openai-compatible" {
		t.Fatalf("agent model provider = %v", defaultModel["providerId"])
	}

	invalid := requestJSON(t, http.MethodPut, api.URL+"/llm-provider-selection", map[string]any{"providerId": "openai-compatible", "model": "missing-model", "reasoningEffort": "medium"})
	if invalid.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid provider selection status = %d", invalid.StatusCode)
	}
	_ = invalid.Body.Close()
}

func TestObservabilitySummary(t *testing.T) {
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)
	item := map[string]any{
		"id": "item_manual_1", "key": "OMG-1", "title": "Observable item", "description": "Needs status summary.",
		"status": "Ready", "priority": "High", "assignee": "requirement", "labels": []any{"manual"}, "team": "Omega", "stageId": "intake", "target": "No target",
	}
	_ = postJSON(t, api.URL+"/work-items", map[string]any{"item": item})
	var pipeline map[string]any
	decode(t, postJSON(t, api.URL+"/pipelines/from-work-item", map[string]any{"item": item}), &pipeline)
	decode(t, requestJSON(t, http.MethodPost, api.URL+"/pipelines/"+pipeline["id"].(string)+"/start", nil), &pipeline)
	decode(t, postJSON(t, api.URL+"/pipelines/"+pipeline["id"].(string)+"/complete-stage", map[string]any{"passed": true, "notes": "ready"}), &pipeline)

	response, err := http.Get(api.URL + "/observability")
	if err != nil {
		t.Fatal(err)
	}
	var summary map[string]any
	decode(t, response, &summary)
	counts := mapValue(summary["counts"])
	if counts["pipelines"].(float64) != 1 {
		t.Fatalf("pipeline count = %v", counts["pipelines"])
	}
	attention := mapValue(summary["attention"])
	if attention["waitingHuman"].(float64) < 1 {
		t.Fatalf("attention = %+v", attention)
	}
}

func TestObservabilityDashboardMetrics(t *testing.T) {
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)
	startedAt := "2026-04-29T09:00:00Z"
	completedAt := "2026-04-29T09:03:00Z"
	database, err := repo.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	item := map[string]any{
		"id": "item_dashboard", "projectId": "project_omega", "key": "OMG-dashboard", "title": "Dashboard", "description": "Observe run health.",
		"status": "In Review", "priority": "High", "assignee": "delivery", "labels": []any{"manual"}, "team": "Omega", "stageId": "human_review", "target": "ZYOOO/TestRepo", "createdAt": startedAt, "updatedAt": completedAt,
	}
	pipeline := makePipelineWithTemplate(item, &PipelineTemplate{
		ID:          "devflow-pr",
		Name:        "DevFlow PR cycle",
		Description: "DevFlow",
		StageProfiles: []StageProfile{
			{ID: "todo", Title: "Requirement", Agent: "requirement"},
			{ID: "in_progress", Title: "Implementation", Agent: "coding"},
			{ID: "human_review", Title: "Human Review", Agent: "delivery", HumanGate: true},
		},
	})
	pipeline["status"] = "waiting-human"
	checkpoint := map[string]any{"id": "checkpoint_dashboard", "pipelineId": text(pipeline, "id"), "attemptId": "attempt_waiting", "stageId": "human_review", "status": "pending", "title": "Human Review", "createdAt": startedAt, "updatedAt": completedAt}
	database.Tables.WorkItems = append(database.Tables.WorkItems, item)
	database.Tables.Pipelines = append(database.Tables.Pipelines, pipeline)
	database.Tables.Checkpoints = append(database.Tables.Checkpoints, checkpoint)
	database.Tables.Attempts = append(database.Tables.Attempts,
		map[string]any{"id": "attempt_done", "itemId": text(item, "id"), "pipelineId": text(pipeline, "id"), "repositoryTargetId": "repo_1", "status": "done", "runner": "codex", "currentStageId": "done", "pullRequestUrl": "https://github.com/acme/demo/pull/10", "startedAt": startedAt, "finishedAt": completedAt, "updatedAt": completedAt, "stages": []map[string]any{{"id": "in_progress", "title": "Implementation", "status": "passed", "startedAt": startedAt, "completedAt": completedAt}}},
		map[string]any{"id": "attempt_failed", "itemId": text(item, "id"), "pipelineId": text(pipeline, "id"), "repositoryTargetId": "repo_1", "status": "failed", "runner": "codex", "currentStageId": "in_progress", "pullRequestUrl": "https://github.com/acme/demo/pull/11", "errorMessage": "tests failed", "startedAt": startedAt, "finishedAt": completedAt, "updatedAt": completedAt, "stages": []map[string]any{{"id": "code_review", "title": "Review", "status": "failed", "durationMs": 240000}}},
		map[string]any{"id": "attempt_running", "itemId": text(item, "id"), "pipelineId": text(pipeline, "id"), "repositoryTargetId": "repo_1", "status": "running", "runner": "opencode", "currentStageId": "in_progress", "lastSeenAt": startedAt, "startedAt": startedAt, "updatedAt": startedAt, "stages": []map[string]any{}},
		map[string]any{"id": "attempt_waiting", "itemId": text(item, "id"), "pipelineId": text(pipeline, "id"), "repositoryTargetId": "repo_1", "status": "waiting-human", "runner": "claude-code", "currentStageId": "human_review", "lastSeenAt": completedAt, "startedAt": startedAt, "updatedAt": completedAt, "stages": []map[string]any{}},
	)
	database.Tables.ProofRecords = append(database.Tables.ProofRecords, map[string]any{"id": "proof_merge", "operationId": "operation_merge", "label": "merge proof", "value": "https://github.com/acme/demo/pull/10", "createdAt": completedAt})
	if err := repo.Save(context.Background(), *database); err != nil {
		t.Fatal(err)
	}

	var summary map[string]any
	decode(t, mustGet(t, api.URL+"/observability"), &summary)
	counts := mapValue(summary["counts"])
	if counts["attempts"].(float64) != 4 {
		t.Fatalf("attempt count = %+v", counts)
	}
	dashboard := mapValue(summary["dashboard"])
	attempts := mapValue(dashboard["attempts"])
	if attempts["total"].(float64) != 4 || attempts["terminal"].(float64) != 2 || attempts["active"].(float64) != 1 {
		t.Fatalf("attempt dashboard = %+v", attempts)
	}
	if attempts["successRate"].(float64) != 0.5 {
		t.Fatalf("success rate = %+v", attempts["successRate"])
	}
	failures := arrayMaps(dashboard["failureReasons"])
	if len(failures) == 0 || failures[0]["reason"] != "tests failed" {
		t.Fatalf("failure reasons = %+v", failures)
	}
	slowStages := arrayMaps(dashboard["slowStages"])
	if len(slowStages) == 0 || slowStages[0]["stageId"] != "code_review" {
		t.Fatalf("slow stages = %+v", slowStages)
	}
	stageAverages := arrayMaps(dashboard["stageAverageDurations"])
	if len(stageAverages) < 2 || stageAverages[0]["stageId"] != "code_review" || intNumber(stageAverages[0]["averageDurationMs"]) != 240000 {
		t.Fatalf("stage averages = %+v", stageAverages)
	}
	runnerUsage := arrayMaps(dashboard["runnerUsage"])
	if len(runnerUsage) == 0 || runnerUsage[0]["runner"] != "codex" || intNumber(runnerUsage[0]["count"]) != 2 || intNumber(runnerUsage[0]["successCount"]) != 1 || intNumber(runnerUsage[0]["failureCount"]) != 1 {
		t.Fatalf("runner usage = %+v", runnerUsage)
	}
	checkpointWaits := mapValue(dashboard["checkpointWaitTimes"])
	if intNumber(checkpointWaits["total"]) != 1 || intNumber(checkpointWaits["pending"]) != 1 || intNumber(checkpointWaits["maxWaitSeconds"]) <= 0 {
		t.Fatalf("checkpoint waits = %+v", checkpointWaits)
	}
	pullRequests := mapValue(dashboard["pullRequests"])
	if intNumber(pullRequests["created"]) != 2 || intNumber(pullRequests["merged"]) != 1 || intNumber(pullRequests["open"]) != 1 {
		t.Fatalf("pull request metrics = %+v", pullRequests)
	}
	trends := arrayMaps(dashboard["trends"])
	if len(trends) == 0 || intNumber(trends[0]["attemptsStarted"]) != 4 || intNumber(trends[0]["pullRequestsCreated"]) != 2 || intNumber(trends[0]["pullRequestsMerged"]) != 1 {
		t.Fatalf("trends = %+v", trends)
	}
	waiting := arrayMaps(dashboard["waitingHumanQueue"])
	if len(waiting) != 1 || waiting[0]["checkpointId"] != "checkpoint_dashboard" {
		t.Fatalf("waiting queue = %+v", waiting)
	}
	activeRuns := arrayMaps(dashboard["activeRuns"])
	if len(activeRuns) != 2 {
		t.Fatalf("active runs = %+v", activeRuns)
	}
	actions := arrayMaps(dashboard["recommendedActions"])
	if len(actions) == 0 {
		t.Fatalf("recommended actions missing: %+v", dashboard)
	}
}

func TestGitHubIssueImportUsesGhAndPersistsWorkItems(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell gh script uses POSIX sh")
	}
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)

	bin := t.TempDir()
	gh := filepath.Join(bin, "gh")
	script := "#!/bin/sh\nprintf '%s' '[{\"number\":7,\"title\":\"Imported issue\",\"body\":\"From GitHub\",\"url\":\"https://github.test/issues/7\",\"labels\":[{\"name\":\"bug\"}],\"assignees\":[{\"login\":\"alice\"}]}]'\n"
	if err := os.WriteFile(gh, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	codex := filepath.Join(bin, "codex")
	codexScript := `#!/bin/sh
prompt="$(cat)"
output=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--output-last-message" ]; then
    shift
    output="$1"
  fi
  shift
done
if printf '%s' "$prompt" | grep -q "You are the review agent for Omega"; then
  if [ -n "$output" ]; then
    printf '%s\n' '# Review' '' 'Verdict: APPROVED' '' 'The change satisfies the requirement.' > "$output"
  fi
  echo "fake review agent approved"
  exit 0
fi
printf '%s\n' '# Claimed issue proof' '' 'Implemented by fake codex for auto-run.' > omega-claimed-proof.md
if [ -n "$output" ]; then
  echo "fake coding agent completed" > "$output"
fi
echo "fake coding agent completed"
`
	if err := os.WriteFile(codex, []byte(codexScript), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	response := postJSON(t, api.URL+"/github/import-issues", map[string]any{"owner": "acme", "repo": "demo"})
	var database WorkspaceDatabase
	decode(t, response, &database)
	if len(database.Tables.WorkItems) != 1 {
		t.Fatalf("work item count = %d", len(database.Tables.WorkItems))
	}
	if got := database.Tables.WorkItems[0]["title"]; got != "Imported issue" {
		t.Fatalf("imported title = %v", got)
	}
	if got := database.Tables.WorkItems[0]["assignee"]; got != "alice" {
		t.Fatalf("imported assignee = %v", got)
	}
	if got := database.Tables.WorkItems[0]["source"]; got != "github_issue" {
		t.Fatalf("source = %v", got)
	}
	if got := database.Tables.WorkItems[0]["sourceExternalRef"]; got != "acme/demo#7" {
		t.Fatalf("source external ref = %v", got)
	}
	if got := database.Tables.WorkItems[0]["repositoryTargetId"]; got != "repo_acme_demo" {
		t.Fatalf("repository target id = %v", got)
	}
	if len(database.Tables.Requirements) != 1 {
		t.Fatalf("requirement count = %d", len(database.Tables.Requirements))
	}
	requirement := database.Tables.Requirements[0]
	if requirement["source"] != "github_issue" || requirement["sourceExternalRef"] != "acme/demo#7" || requirement["repositoryTargetId"] != "repo_acme_demo" {
		t.Fatalf("imported requirement = %+v", requirement)
	}
	if database.Tables.WorkItems[0]["requirementId"] != requirement["id"] {
		t.Fatalf("imported item requirement link = item:%+v requirement:%+v", database.Tables.WorkItems[0], requirement)
	}
	targets := arrayMaps(database.Tables.Projects[0]["repositoryTargets"])
	if len(targets) != 1 || targets[0]["id"] != "repo_acme_demo" || targets[0]["kind"] != "github" {
		t.Fatalf("repository targets = %+v", targets)
	}
}

func TestOrchestratorTickClaimsNextGitHubIssueAndCreatesDevFlowPipeline(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell gh script uses POSIX sh")
	}
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)

	bin := t.TempDir()
	gh := filepath.Join(bin, "gh")
	script := "#!/bin/sh\nprintf '%s' '[{\"number\":11,\"title\":\"Automate issue claim\",\"body\":\"Make Omega claim this issue.\",\"url\":\"https://github.com/acme/demo/issues/11\",\"labels\":[{\"name\":\"omega-ready\"}],\"assignees\":[]}]'\n"
	if err := os.WriteFile(gh, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	codex := filepath.Join(bin, "codex")
	codexScript := `#!/bin/sh
prompt="$(cat)"
output=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--output-last-message" ]; then
    shift
    output="$1"
  fi
  shift
done
if printf '%s' "$prompt" | grep -q "You are the review agent for Omega"; then
  if [ -n "$output" ]; then
    printf '%s\n' '# Review' '' 'Verdict: APPROVED' '' 'The change satisfies the requirement.' > "$output"
  fi
  echo "fake review agent approved"
  exit 0
fi
printf '%s\n' '# Claimed issue proof' '' 'Implemented by fake codex for auto-run.' > omega-claimed-proof.md
if [ -n "$output" ]; then
  echo "fake coding agent completed" > "$output"
fi
echo "fake coding agent completed"
`
	if err := os.WriteFile(codex, []byte(codexScript), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	decode(t, postJSON(t, api.URL+"/github/bind-repository-target", map[string]any{
		"owner":         "acme",
		"repo":          "demo",
		"nameWithOwner": "acme/demo",
		"defaultBranch": "main",
		"url":           "https://github.com/acme/demo",
	}), &WorkspaceDatabase{})

	var result map[string]any
	decode(t, postJSON(t, api.URL+"/orchestrator/tick", map[string]any{"repositoryTargetId": "repo_acme_demo"}), &result)
	if result["status"] != "claimed" {
		t.Fatalf("tick result = %+v", result)
	}
	item := mapValue(result["workItem"])
	if item["sourceExternalRef"] != "acme/demo#11" || item["repositoryTargetId"] != "repo_acme_demo" {
		t.Fatalf("claimed item = %+v", item)
	}
	if item["requirementId"] == "" {
		t.Fatalf("claimed item missing requirement link: %+v", item)
	}
	pipeline := mapValue(result["pipeline"])
	if pipeline["templateId"] != "devflow-pr" || pipeline["workItemId"] != item["id"] {
		t.Fatalf("pipeline = %+v", pipeline)
	}

	var database WorkspaceDatabase
	decode(t, mustGet(t, api.URL+"/workspace"), &database)
	if len(database.Tables.WorkItems) != 1 || len(database.Tables.Requirements) != 1 || len(database.Tables.Pipelines) != 1 {
		t.Fatalf("database after tick = requirements:%+v workItems:%+v pipelines:%+v", database.Tables.Requirements, database.Tables.WorkItems, database.Tables.Pipelines)
	}
	if database.Tables.Requirements[0]["id"] != item["requirementId"] {
		t.Fatalf("claimed requirement link = item:%+v requirements:%+v", item, database.Tables.Requirements)
	}

	var second map[string]any
	decode(t, postJSON(t, api.URL+"/orchestrator/tick", map[string]any{"repositoryTargetId": "repo_acme_demo"}), &second)
	if second["status"] != "locked" {
		t.Fatalf("second tick should report already claimed issue lock: %+v", second)
	}
}

func TestOrchestratorTickSkipsIssuesWithoutReadyLabel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell gh script uses POSIX sh")
	}
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)

	bin := t.TempDir()
	gh := filepath.Join(bin, "gh")
	script := "#!/bin/sh\nprintf '%s' '[{\"number\":13,\"title\":\"Open but not authorized\",\"body\":\"Do not claim this yet.\",\"url\":\"https://github.com/acme/demo/issues/13\",\"labels\":[],\"assignees\":[]}]'\n"
	if err := os.WriteFile(gh, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	codex := filepath.Join(bin, "codex")
	codexScript := `#!/bin/sh
prompt="$(cat)"
output=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--output-last-message" ]; then
    shift
    output="$1"
  fi
  shift
done
if printf '%s' "$prompt" | grep -q "You are the review agent for Omega"; then
  if [ -n "$output" ]; then
    printf '%s\n' '# Review' '' 'Verdict: APPROVED' '' 'The change satisfies the requirement.' > "$output"
  fi
  echo "fake review agent approved"
  exit 0
fi
printf '%s\n' '# Claimed issue proof' '' 'Implemented by fake codex for auto-run.' > omega-claimed-proof.md
if [ -n "$output" ]; then
  echo "fake coding agent completed" > "$output"
fi
echo "fake coding agent completed"
`
	if err := os.WriteFile(codex, []byte(codexScript), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	decode(t, postJSON(t, api.URL+"/github/bind-repository-target", map[string]any{
		"owner":         "acme",
		"repo":          "demo",
		"nameWithOwner": "acme/demo",
		"defaultBranch": "main",
		"url":           "https://github.com/acme/demo",
	}), &WorkspaceDatabase{})

	var result map[string]any
	decode(t, postJSON(t, api.URL+"/orchestrator/tick", map[string]any{"repositoryTargetId": "repo_acme_demo"}), &result)
	if result["status"] != "idle" {
		t.Fatalf("tick should ignore open issues without an execution-ready label: %+v", result)
	}

	var database WorkspaceDatabase
	decode(t, mustGet(t, api.URL+"/workspace"), &database)
	if len(database.Tables.WorkItems) != 0 || len(database.Tables.Pipelines) != 0 {
		t.Fatalf("database after skipped tick = workItems:%+v pipelines:%+v", database.Tables.WorkItems, database.Tables.Pipelines)
	}
}

func TestOrchestratorTickCanClaimAndRunDevFlowCycle(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required for devflow PR cycle")
	}
	if runtime.GOOS == "windows" {
		t.Skip("fake shell gh script uses POSIX sh")
	}
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)
	targetRepo := createDemoGitRepo(t)

	bin := t.TempDir()
	ghLog := filepath.Join(bin, "gh.log")
	gh := filepath.Join(bin, "gh")
	script := fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >> %s
if [ "$1" = "issue" ] && [ "$2" = "list" ]; then
  printf '%%s' '[{"number":12,"title":"Run claimed issue through DevFlow","body":"Create a minimal proof change from an automatically claimed issue.","url":"https://github.com/acme/demo/issues/12","labels":[{"name":"omega-ready"}],"assignees":[]}]'
elif [ "$1" = "pr" ] && [ "$2" = "create" ]; then
  printf 'https://github.com/acme/demo/pull/124\n'
elif [ "$1" = "pr" ] && [ "$2" = "checks" ]; then
  printf 'no checks found\n'
else
  printf 'ok\n'
fi
`, ghLog)
	if err := os.WriteFile(gh, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	codex := filepath.Join(bin, "codex")
	codexScript := `#!/bin/sh
prompt="$(cat)"
output=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--output-last-message" ]; then
    shift
    output="$1"
  fi
  shift
done
if printf '%s' "$prompt" | grep -q "You are the review agent for Omega"; then
  if [ -n "$output" ]; then
    printf '%s\n' '# Review' '' 'Verdict: APPROVED' '' 'The change satisfies the requirement.' > "$output"
  fi
  echo "fake review agent approved"
  exit 0
fi
printf '%s\n' '# Claimed issue proof' '' 'Implemented by fake codex for auto-run.' > omega-claimed-proof.md
if [ -n "$output" ]; then
  echo "fake coding agent completed" > "$output"
fi
echo "fake coding agent completed"
`
	if err := os.WriteFile(codex, []byte(codexScript), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	decode(t, postJSON(t, api.URL+"/github/bind-repository-target", map[string]any{
		"owner":         "acme",
		"repo":          "demo",
		"nameWithOwner": "acme/demo",
		"defaultBranch": "main",
		"url":           targetRepo,
	}), &WorkspaceDatabase{})

	var result map[string]any
	decode(t, postJSON(t, api.URL+"/orchestrator/tick", map[string]any{
		"repositoryTargetId": "repo_acme_demo",
		"autoRun":            true,
		"autoApproveHuman":   true,
		"autoMerge":          true,
	}), &result)
	if result["status"] != "accepted" {
		t.Fatalf("tick result = %+v", result)
	}
	item := mapValue(result["workItem"])
	if item["sourceExternalRef"] != "acme/demo#12" {
		t.Fatalf("claimed item = %+v", item)
	}
	pipeline := mapValue(result["pipeline"])
	if pipeline["templateId"] != "devflow-pr" || pipeline["status"] != "running" {
		t.Fatalf("pipeline = %+v", pipeline)
	}
	attempt := mapValue(result["attempt"])
	if attempt["status"] != "running" || attempt["currentStageId"] != "todo" {
		t.Fatalf("attempt = %+v", attempt)
	}
	var locks []map[string]any
	decode(t, mustGet(t, api.URL+"/execution-locks"), &locks)
	if len(locks) != 1 || locks[0]["status"] != "claimed" {
		t.Fatalf("accepted auto-run should keep execution lock while background job runs: %+v", locks)
	}

	var database WorkspaceDatabase
	decode(t, mustGet(t, api.URL+"/workspace"), &database)
	if len(database.Tables.WorkItems) != 1 || database.Tables.WorkItems[0]["status"] != "In Review" {
		t.Fatalf("database work items = %+v", database.Tables.WorkItems)
	}
	if len(database.Tables.Pipelines) != 1 || (database.Tables.Pipelines[0]["status"] != "running" && database.Tables.Pipelines[0]["status"] != "waiting-human") {
		t.Fatalf("database pipelines = %+v", database.Tables.Pipelines)
	}
	if len(database.Tables.Attempts) != 1 {
		t.Fatalf("auto-run attempt records were not persisted: %+v", database.Tables.Attempts)
	}
	persistedAttempt := database.Tables.Attempts[0]
	if (persistedAttempt["status"] != "running" && persistedAttempt["status"] != "waiting-human") || persistedAttempt["trigger"] != "orchestrator" {
		t.Fatalf("auto-run attempt = %+v", persistedAttempt)
	}
	if raw, err := os.ReadFile(ghLog); err != nil || !strings.Contains(string(raw), "issue list") {
		t.Fatalf("gh log = %s err=%v", string(raw), err)
	}
	deadline := time.Now().Add(10 * time.Second)
	completed := false
	for time.Now().Before(deadline) {
		var refreshed WorkspaceDatabase
		decode(t, mustGet(t, api.URL+"/workspace"), &refreshed)
		if len(refreshed.Tables.Attempts) == 1 && refreshed.Tables.Attempts[0]["status"] != "running" {
			completed = true
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !completed {
		var refreshed WorkspaceDatabase
		decode(t, mustGet(t, api.URL+"/workspace"), &refreshed)
		t.Fatalf("auto-run attempt did not settle before cleanup: %+v", refreshed.Tables.Attempts)
	}
	lockReleased := false
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var refreshedLocks []map[string]any
		decode(t, mustGet(t, api.URL+"/execution-locks"), &refreshedLocks)
		if len(refreshedLocks) == 1 && refreshedLocks[0]["status"] == "released" {
			lockReleased = true
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !lockReleased {
		var refreshedLocks []map[string]any
		decode(t, mustGet(t, api.URL+"/execution-locks"), &refreshedLocks)
		t.Fatalf("auto-run lock did not release before cleanup: %+v", refreshedLocks)
	}
}

func TestExecutionLockPreventsDuplicateClaimForSameIssue(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell gh script uses POSIX sh")
	}
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)

	bin := t.TempDir()
	gh := filepath.Join(bin, "gh")
	script := "#!/bin/sh\nprintf '%s' '[{\"number\":14,\"title\":\"Guard duplicate execution\",\"body\":\"Only one local app should claim this.\",\"url\":\"https://github.com/acme/demo/issues/14\",\"labels\":[{\"name\":\"omega-ready\"}],\"assignees\":[]}]'\n"
	if err := os.WriteFile(gh, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	decode(t, postJSON(t, api.URL+"/github/bind-repository-target", map[string]any{
		"owner":         "acme",
		"repo":          "demo",
		"nameWithOwner": "acme/demo",
		"defaultBranch": "main",
		"url":           "https://github.com/acme/demo",
	}), &WorkspaceDatabase{})
	var first map[string]any
	decode(t, postJSON(t, api.URL+"/orchestrator/tick", map[string]any{"repositoryTargetId": "repo_acme_demo"}), &first)
	if first["status"] != "claimed" {
		t.Fatalf("first tick = %+v", first)
	}
	var second map[string]any
	decode(t, postJSON(t, api.URL+"/orchestrator/tick", map[string]any{"repositoryTargetId": "repo_acme_demo"}), &second)
	if second["status"] != "locked" {
		t.Fatalf("second tick should report existing execution lock: %+v", second)
	}
	lock := mapValue(second["lock"])
	if lock["scope"] != "github-issue:acme/demo#14" || lock["workItemId"] != "github_acme-demo_14" {
		t.Fatalf("lock = %+v", lock)
	}
	var database WorkspaceDatabase
	decode(t, mustGet(t, api.URL+"/workspace"), &database)
	if len(database.Tables.WorkItems) != 1 || len(database.Tables.Pipelines) != 1 {
		t.Fatalf("duplicate records were created: workItems=%+v pipelines=%+v", database.Tables.WorkItems, database.Tables.Pipelines)
	}
}

func TestOrchestratorWatcherPersistsAndScansReadyIssues(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell gh script uses POSIX sh")
	}
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)

	bin := t.TempDir()
	gh := filepath.Join(bin, "gh")
	script := "#!/bin/sh\nprintf '%s' '[{\"number\":16,\"title\":\"Watcher claimed issue\",\"body\":\"The local watcher should claim this ready issue.\",\"url\":\"https://github.com/acme/demo/issues/16\",\"labels\":[{\"name\":\"omega-ready\"}],\"assignees\":[]}]'\n"
	if err := os.WriteFile(gh, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	decode(t, postJSON(t, api.URL+"/github/bind-repository-target", map[string]any{
		"owner":         "acme",
		"repo":          "demo",
		"nameWithOwner": "acme/demo",
		"defaultBranch": "main",
		"url":           "https://github.com/acme/demo",
	}), &WorkspaceDatabase{})

	var watcher map[string]any
	decode(t, putJSON(t, api.URL+"/orchestrator/watchers/repo_acme_demo", map[string]any{
		"status":          "active",
		"intervalSeconds": 1,
		"autoRun":         false,
		"limit":           "20",
	}), &watcher)
	if watcher["status"] != "active" || intValue(watcher["intervalSeconds"]) != 1 {
		t.Fatalf("watcher = %+v", watcher)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var database WorkspaceDatabase
		decode(t, mustGet(t, api.URL+"/workspace"), &database)
		if len(database.Tables.WorkItems) == 1 {
			if database.Tables.WorkItems[0]["sourceExternalRef"] != "acme/demo#16" {
				t.Fatalf("claimed wrong item: %+v", database.Tables.WorkItems[0])
			}
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	var database WorkspaceDatabase
	decode(t, mustGet(t, api.URL+"/workspace"), &database)
	if len(database.Tables.WorkItems) != 1 {
		t.Fatalf("watcher did not claim a ready issue: %+v", database.Tables.WorkItems)
	}
	var watchers []map[string]any
	decode(t, mustGet(t, api.URL+"/orchestrator/watchers"), &watchers)
	if len(watchers) != 1 || watchers[0]["lastTickStatus"] != "claimed" {
		t.Fatalf("watchers = %+v", watchers)
	}
}

func TestExecutionLocksCanBeListedAndReleased(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell gh script uses POSIX sh")
	}
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)

	bin := t.TempDir()
	gh := filepath.Join(bin, "gh")
	script := "#!/bin/sh\nprintf '%s' '[{\"number\":15,\"title\":\"Release execution lock\",\"body\":\"Release this after claiming.\",\"url\":\"https://github.com/acme/demo/issues/15\",\"labels\":[{\"name\":\"omega-ready\"}],\"assignees\":[]}]'\n"
	if err := os.WriteFile(gh, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	decode(t, postJSON(t, api.URL+"/github/bind-repository-target", map[string]any{
		"owner":         "acme",
		"repo":          "demo",
		"nameWithOwner": "acme/demo",
		"defaultBranch": "main",
		"url":           "https://github.com/acme/demo",
	}), &WorkspaceDatabase{})
	var claimed map[string]any
	decode(t, postJSON(t, api.URL+"/orchestrator/tick", map[string]any{"repositoryTargetId": "repo_acme_demo"}), &claimed)
	lockID := text(mapValue(claimed["lock"]), "id")

	var locks []map[string]any
	decode(t, mustGet(t, api.URL+"/execution-locks"), &locks)
	if len(locks) != 1 || locks[0]["id"] != lockID || locks[0]["status"] != "claimed" {
		t.Fatalf("locks = %+v", locks)
	}

	var released map[string]any
	decode(t, postJSON(t, api.URL+"/execution-locks/"+url.PathEscape(lockID)+"/release", map[string]any{}), &released)
	if released["status"] != "released" {
		t.Fatalf("released = %+v", released)
	}

	decode(t, mustGet(t, api.URL+"/execution-locks"), &locks)
	if len(locks) != 1 || locks[0]["status"] != "released" {
		t.Fatalf("locks after release = %+v", locks)
	}
}

func TestExecutionLocksReturnsEmptyListWhenNoLocksExist(t *testing.T) {
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)

	var locks []map[string]any
	decode(t, mustGet(t, api.URL+"/execution-locks"), &locks)
	if len(locks) != 0 {
		t.Fatalf("locks = %+v", locks)
	}
}

func TestGitHubCreatePRUsesGhWithProofSummary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell gh script uses POSIX sh")
	}
	api, _ := newTestAPI(t)
	workspace := t.TempDir()
	repositoryPath := filepath.Join(workspace, "repo")
	if err := os.MkdirAll(filepath.Join(repositoryPath, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	proofDir := filepath.Join(workspace, ".omega", "proof")
	if err := os.MkdirAll(proofDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proofDir, "change-summary.md"), []byte("# Change\n\n- Added PR delivery\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	bin := t.TempDir()
	argsFile := filepath.Join(bin, "args.txt")
	bodyFile := filepath.Join(bin, "body.txt")
	gh := filepath.Join(bin, "gh")
	script := "#!/bin/sh\nprintf '%s\\n' \"$PWD $*\" > " + argsFile + "\nwhile [ \"$#\" -gt 0 ]; do if [ \"$1\" = \"--body-file\" ]; then shift; cat \"$1\" > " + bodyFile + "; fi; shift; done\nprintf '%s\\n' 'https://github.com/acme/demo/pull/12'\n"
	if err := os.WriteFile(gh, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	var result map[string]any
	decode(t, postJSON(t, api.URL+"/github/create-pr", map[string]any{
		"workspacePath":   workspace,
		"title":           "Omega delivery for OMG-1",
		"branchName":      "omega/OMG-1-coding",
		"baseBranch":      "main",
		"draft":           true,
		"changedFiles":    []any{"src/omega-demo-change.ts"},
		"repositoryOwner": "acme",
		"repositoryName":  "demo",
	}), &result)

	if result["status"] != "created" || result["url"] != "https://github.com/acme/demo/pull/12" {
		t.Fatalf("create pr result = %+v", result)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	argsText := string(args)
	if !strings.Contains(argsText, repositoryPath) || !strings.Contains(argsText, "pr create") || !strings.Contains(argsText, "--head omega/OMG-1-coding") || !strings.Contains(argsText, "--draft") {
		t.Fatalf("gh args = %s", argsText)
	}
	body, err := os.ReadFile(bodyFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "Added PR delivery") || !strings.Contains(string(body), "src/omega-demo-change.ts") {
		t.Fatalf("body = %s", string(body))
	}
}

func TestGitHubPRStatusUsesGhViewAndChecks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell gh script uses POSIX sh")
	}
	api, _ := newTestAPI(t)

	bin := t.TempDir()
	gh := filepath.Join(bin, "gh")
	script := `#!/bin/sh
if [ "$1" = "pr" ] && [ "$2" = "view" ] && echo "$*" | grep -q 'comments,reviews'; then
  printf '%s' '{"comments":[{"author":{"login":"reviewer"},"body":"Please tighten empty-state copy.","createdAt":"2026-04-30T08:00:00Z","url":"https://github.com/acme/demo/pull/12#issuecomment-1"}],"reviews":[{"author":{"login":"lead"},"state":"CHANGES_REQUESTED","body":"Update the retry explanation.","submittedAt":"2026-04-30T08:01:00Z","url":"https://github.com/acme/demo/pull/12#pullrequestreview-1"}]}'
  exit 0
fi
if [ "$1" = "pr" ] && [ "$2" = "view" ]; then
  printf '%s' '{"number":12,"title":"Omega delivery","state":"OPEN","mergeable":"MERGEABLE","reviewDecision":"APPROVED","headRefName":"omega/OMG-1-coding","baseRefName":"main","url":"https://github.com/acme/demo/pull/12"}'
  exit 0
fi
if [ "$1" = "pr" ] && [ "$2" = "checks" ]; then
  printf '%s' '[{"name":"lint","state":"SUCCESS","link":"https://github.com/acme/demo/actions/runs/1"},{"name":"test","state":"PENDING","link":"https://github.com/acme/demo/actions/runs/2"}]'
  exit 0
fi
exit 1
`
	if err := os.WriteFile(gh, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	var result map[string]any
	decode(t, postJSON(t, api.URL+"/github/pr-status", map[string]any{
		"repositoryOwner": "acme",
		"repositoryName":  "demo",
		"number":          12,
	}), &result)

	if result["state"] != "OPEN" || result["reviewDecision"] != "APPROVED" || result["deliveryGate"] != "pending" {
		t.Fatalf("pr status = %+v", result)
	}
	checks := arrayMaps(result["checks"])
	if len(checks) != 2 || checks[0]["name"] != "lint" || checks[1]["state"] != "PENDING" {
		t.Fatalf("checks = %+v", checks)
	}
	checkSummary := mapValue(result["checkSummary"])
	if checkSummary["passed"].(float64) != 1 || checkSummary["pending"].(float64) != 1 || checkSummary["failed"].(float64) != 0 {
		t.Fatalf("check summary = %+v", checkSummary)
	}
	proofs := arrayMaps(result["proofRecords"])
	if len(proofs) != 3 || proofs[0]["label"] != "pull-request" || proofs[1]["label"] != "check" {
		t.Fatalf("proof records = %+v", proofs)
	}
	actions := arrayMaps(result["recommendedActions"])
	if len(actions) != 1 || actions[0]["type"] != "checks-pending" {
		t.Fatalf("recommended actions = %+v", actions)
	}
	feedback := arrayMaps(result["reviewFeedback"])
	if len(feedback) != 2 || feedback[0]["kind"] != "pr-comment" || feedback[1]["kind"] != "pr-review" {
		t.Fatalf("review feedback = %+v", feedback)
	}
	branchSync := mapValue(result["branchSync"])
	if branchSync["status"] != "unknown" {
		t.Fatalf("branch sync = %+v", branchSync)
	}
}

func TestGitHubPullRequestFeedbackFromView(t *testing.T) {
	feedback := githubPullRequestFeedbackFromView(map[string]any{
		"comments": []any{
			map[string]any{"author": map[string]any{"login": "designer"}, "body": "The secondary button contrast is too low.", "url": "https://example.test/comment"},
		},
		"reviews": []any{
			map[string]any{"author": map[string]any{"login": "reviewer"}, "state": "CHANGES_REQUESTED", "body": "Add validation proof before merge."},
			map[string]any{"author": map[string]any{"login": "maintainer"}, "state": "COMMENTED"},
		},
	})
	if len(feedback) != 3 {
		t.Fatalf("feedback = %+v", feedback)
	}
	prompt := githubPullRequestFeedbackPrompt(feedback)
	for _, expected := range []string{"secondary button contrast", "Add validation proof", "Review state: COMMENTED"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("prompt missing %q in:\n%s", expected, prompt)
		}
	}
}

func TestGitHubPullRequestReviewThreadFeedbackFromGraphQL(t *testing.T) {
	feedback := githubPullRequestReviewThreadFeedbackFromGraphQL(map[string]any{
		"data": map[string]any{
			"repository": map[string]any{
				"pullRequest": map[string]any{
					"reviewThreads": map[string]any{
						"nodes": []any{
							map[string]any{
								"isResolved": false,
								"path":       "src/Button.tsx",
								"line":       float64(42),
								"comments": map[string]any{"nodes": []any{
									map[string]any{
										"author":    map[string]any{"login": "reviewer"},
										"body":      "Please keep the button copy short.",
										"createdAt": "2026-05-01T01:00:00Z",
										"url":       "https://github.com/acme/demo/pull/1#discussion_r1",
										"diffHunk":  "@@ -1,2 +1,2 @@",
									},
								}},
							},
							map[string]any{
								"isResolved": true,
								"path":       "src/Card.tsx",
								"line":       float64(9),
								"comments": map[string]any{"nodes": []any{
									map[string]any{"body": "Resolved copy note."},
								}},
							},
						},
					},
				},
			},
		},
	})
	if len(feedback) != 2 {
		t.Fatalf("feedback = %+v", feedback)
	}
	if text(feedback[0], "kind") != "pr-review-thread" || text(feedback[0], "state") != "unresolved" || text(feedback[0], "path") != "src/Button.tsx" || text(feedback[0], "line") != "42" {
		t.Fatalf("unresolved thread feedback missing line context: %+v", feedback[0])
	}
	if !boolValue(feedback[1]["resolved"]) || text(feedback[1], "state") != "resolved" {
		t.Fatalf("resolved thread state missing: %+v", feedback[1])
	}
}

func TestGitHubPRStatusClassifiesFailedChecks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell gh script uses POSIX sh")
	}
	api, _ := newTestAPI(t)
	bin := t.TempDir()
	gh := filepath.Join(bin, "gh")
	script := `#!/bin/sh
if [ "$1" = "pr" ] && [ "$2" = "view" ]; then
  printf '%s' '{"number":13,"title":"Omega delivery","state":"OPEN","mergeable":"MERGEABLE","reviewDecision":"APPROVED","headRefName":"omega/OMG-2-coding","baseRefName":"main","url":"https://github.com/acme/demo/pull/13"}'
  exit 0
fi
if [ "$1" = "pr" ] && [ "$2" = "checks" ]; then
  printf '%s' '[{"name":"lint","state":"FAILURE","link":"https://github.com/acme/demo/actions/runs/3"},{"name":"test","state":"SUCCESS","link":"https://github.com/acme/demo/actions/runs/4"}]'
  exit 0
fi
if [ "$1" = "run" ] && [ "$2" = "view" ] && [ "$3" = "3" ]; then
  printf '%s' 'lint	Failing test output: expected button text to be shorter'
  exit 0
fi
exit 1
`
	if err := os.WriteFile(gh, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	var result map[string]any
	decode(t, postJSON(t, api.URL+"/github/pr-status", map[string]any{"repositoryOwner": "acme", "repositoryName": "demo", "number": 13}), &result)
	if result["deliveryGate"] != "blocked" {
		t.Fatalf("delivery gate = %+v", result)
	}
	checkSummary := mapValue(result["checkSummary"])
	if checkSummary["failed"].(float64) != 1 || len(arrayMaps(checkSummary["failedChecks"])) != 1 {
		t.Fatalf("check summary = %+v", checkSummary)
	}
	actions := arrayMaps(result["recommendedActions"])
	if len(actions) == 0 || actions[0]["type"] != "checks-failed" {
		t.Fatalf("recommended actions = %+v", actions)
	}
	checkFeedback := arrayMaps(result["checkLogFeedback"])
	if len(checkFeedback) != 1 || !strings.Contains(text(checkFeedback[0], "message"), "expected button text") {
		t.Fatalf("check log feedback = %+v", checkFeedback)
	}
}

func TestGitHubOAuthStartAndCallbackPersistConnection(t *testing.T) {
	root := t.TempDir()
	repo := NewSQLiteRepository(filepath.Join(root, "omega.db"))
	seedWorkspace(t, repo)

	tokenServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("token method = %s", request.Method)
		}
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["client_id"] != "github-client" || payload["client_secret"] != "github-secret" || payload["code"] != "code_123" {
			t.Fatalf("unexpected token payload: %+v", payload)
		}
		writeJSON(response, http.StatusOK, map[string]any{
			"access_token": "gho_test",
			"scope":        "repo,read:org,workflow",
			"token_type":   "bearer",
		})
	}))
	defer tokenServer.Close()

	openAPI := filepath.Join(root, "openapi.yaml")
	if err := os.WriteFile(openAPI, []byte("openapi: 3.1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := &Server{
		Repo:          repo,
		WorkspaceRoot: filepath.Join(root, "workspace"),
		OpenAPIPath:   openAPI,
		GitHubOAuth: GitHubOAuthConfig{
			ClientID:     "github-client",
			ClientSecret: "github-secret",
			RedirectURI:  "http://127.0.0.1:3888/auth/github/callback",
			TokenURL:     tokenServer.URL,
		},
		HTTPClient: tokenServer.Client(),
	}
	api := httptest.NewServer(server.Handler())
	defer api.Close()

	var start map[string]any
	decode(t, requestJSON(t, http.MethodPost, api.URL+"/github/oauth/start", nil), &start)
	if start["configured"] != true {
		t.Fatalf("configured = %v", start["configured"])
	}
	authorizeURL := fmt.Sprint(start["authorizeUrl"])
	if !strings.Contains(authorizeURL, "client_id=github-client") || !strings.Contains(authorizeURL, "scope=repo+read%3Aorg+workflow") {
		t.Fatalf("authorize url = %s", authorizeURL)
	}
	state := fmt.Sprint(start["state"])
	if state == "" {
		t.Fatal("state missing")
	}

	var callback map[string]any
	decode(t, mustGet(t, api.URL+"/auth/github/callback?code=code_123&state="+state), &callback)
	if callback["connected"] != true {
		t.Fatalf("callback = %+v", callback)
	}
	if got := arrayValues(callback["scopes"]); len(got) != 3 {
		t.Fatalf("scopes = %+v", callback["scopes"])
	}

	token, err := repo.GetSetting(context.Background(), "github_oauth_token")
	if err != nil {
		t.Fatal(err)
	}
	if token["accessToken"] != "gho_test" {
		t.Fatalf("stored token = %+v", token)
	}

	database, err := repo.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(database.Tables.Connections) != 1 {
		t.Fatalf("connection count = %d", len(database.Tables.Connections))
	}
	connection := database.Tables.Connections[0]
	if connection["providerId"] != "github" || connection["status"] != "connected" {
		t.Fatalf("connection = %+v", connection)
	}
	if got := arrayValues(connection["grantedPermissions"]); len(got) == 0 {
		t.Fatalf("permissions missing: %+v", connection)
	}
}

func TestGitHubOAuthConfigCanBeSavedFromApp(t *testing.T) {
	root := t.TempDir()
	repo := NewSQLiteRepository(filepath.Join(root, "omega.db"))
	seedWorkspace(t, repo)
	openAPI := filepath.Join(root, "openapi.yaml")
	if err := os.WriteFile(openAPI, []byte("openapi: 3.1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := &Server{
		Repo:          repo,
		WorkspaceRoot: filepath.Join(root, "workspace"),
		OpenAPIPath:   openAPI,
		GitHubOAuth: GitHubOAuthConfig{
			RedirectURI: "http://127.0.0.1:3888/auth/github/callback",
			TokenURL:    "https://github.com/login/oauth/access_token",
		},
		HTTPClient: http.DefaultClient,
	}
	api := httptest.NewServer(server.Handler())
	defer api.Close()

	var initial map[string]any
	decode(t, mustGet(t, api.URL+"/github/oauth/config"), &initial)
	if initial["configured"] != false || initial["secretConfigured"] != false {
		t.Fatalf("initial config = %+v", initial)
	}

	var saved map[string]any
	decode(t, requestJSON(t, http.MethodPut, api.URL+"/github/oauth/config", map[string]any{
		"clientId":     "app-client",
		"clientSecret": "app-secret",
		"redirectUri":  "http://127.0.0.1:3888/auth/github/callback",
		"tokenUrl":     "https://github.test/oauth/access_token",
	}), &saved)
	if saved["configured"] != true || saved["clientId"] != "app-client" || saved["secretConfigured"] != true {
		t.Fatalf("saved config = %+v", saved)
	}
	if _, exists := saved["clientSecret"]; exists {
		t.Fatalf("saved config leaked secret: %+v", saved)
	}

	var persisted map[string]any
	decode(t, mustGet(t, api.URL+"/github/oauth/config"), &persisted)
	if persisted["clientId"] != "app-client" || persisted["source"] != "app" {
		t.Fatalf("persisted config = %+v", persisted)
	}

	var start map[string]any
	decode(t, requestJSON(t, http.MethodPost, api.URL+"/github/oauth/start", nil), &start)
	if start["configured"] != true {
		t.Fatalf("oauth start = %+v", start)
	}
	authorizeURL := fmt.Sprint(start["authorizeUrl"])
	if !strings.Contains(authorizeURL, "client_id=app-client") || !strings.Contains(authorizeURL, "redirect_uri=http%3A%2F%2F127.0.0.1%3A3888%2Fauth%2Fgithub%2Fcallback") {
		t.Fatalf("authorize url = %s", authorizeURL)
	}
}

func TestGitHubCliLoginStartUsesGhWebFlow(t *testing.T) {
	root := t.TempDir()
	repo := NewSQLiteRepository(filepath.Join(root, "omega.db"))
	seedWorkspace(t, repo)
	openAPI := filepath.Join(root, "openapi.yaml")
	if err := os.WriteFile(openAPI, []byte("openapi: 3.1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var commandName string
	var commandArgs []string
	server := &Server{
		Repo:          repo,
		WorkspaceRoot: filepath.Join(root, "workspace"),
		OpenAPIPath:   openAPI,
		HTTPClient:    http.DefaultClient,
		CommandStarter: func(name string, args ...string) error {
			commandName = name
			commandArgs = append([]string{}, args...)
			return nil
		},
	}
	api := httptest.NewServer(server.Handler())
	defer api.Close()

	var result map[string]any
	decode(t, requestJSON(t, http.MethodPost, api.URL+"/github/cli-login/start", nil), &result)
	if result["started"] != true || result["method"] != "gh-cli" {
		t.Fatalf("result = %+v", result)
	}
	if result["verificationUrl"] != "https://github.com/login/device" {
		t.Fatalf("verification url = %+v", result)
	}
	if commandName != "gh" {
		t.Fatalf("command name = %s", commandName)
	}
	got := strings.Join(commandArgs, " ")
	for _, want := range []string{"auth login", "--hostname github.com", "--web", "--clipboard", "--scopes repo,read:org,workflow"} {
		if !strings.Contains(got, want) {
			t.Fatalf("command args %q missing %q", got, want)
		}
	}
}

func TestGitHubStatusPersistsGhCliConnection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell script uses POSIX sh")
	}
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)

	bin := t.TempDir()
	gh := filepath.Join(bin, "gh")
	script := "#!/bin/sh\nif [ \"$1\" = \"auth\" ] && [ \"$2\" = \"status\" ]; then printf '%s\\n' 'github.com' '  ✓ Logged in to github.com account ZYOOO (keyring)' '  - Token scopes: '\\''repo'\\'', '\\''read:org'\\'', '\\''workflow'\\'''; exit 0; fi\nexit 1\n"
	if err := os.WriteFile(gh, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	var status map[string]any
	decode(t, mustGet(t, api.URL+"/github/status"), &status)
	if status["authenticated"] != true || status["account"] != "ZYOOO" {
		t.Fatalf("status = %+v", status)
	}
	database, err := repo.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(database.Tables.Connections) == 0 || database.Tables.Connections[0]["connectedAs"] != "ZYOOO" {
		t.Fatalf("connections = %+v", database.Tables.Connections)
	}
}

func TestGitHubRepositoriesUsesGhRepoList(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell script uses POSIX sh")
	}
	api, _ := newTestAPI(t)

	bin := t.TempDir()
	gh := filepath.Join(bin, "gh")
	script := `#!/bin/sh
if [ "$1" = "repo" ] && [ "$2" = "list" ]; then
  printf '%s\n' '[{"name":"demo","nameWithOwner":"acme/demo","owner":{"login":"acme"},"description":"Demo repo","url":"https://github.com/acme/demo","isPrivate":false,"defaultBranchRef":{"name":"main"}}]'
  exit 0
fi
exit 1
`
	if err := os.WriteFile(gh, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	var repositories []map[string]any
	decode(t, mustGet(t, api.URL+"/github/repositories"), &repositories)
	if len(repositories) != 1 || repositories[0]["nameWithOwner"] != "acme/demo" {
		t.Fatalf("repositories = %+v", repositories)
	}
	branch := mapValue(repositories[0]["defaultBranchRef"])
	if branch["name"] != "main" {
		t.Fatalf("default branch = %+v", branch)
	}
}

func TestGitHubBindRepositoryTargetPersistsProjectTarget(t *testing.T) {
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)

	var database WorkspaceDatabase
	decode(t, postJSON(t, api.URL+"/github/bind-repository-target", map[string]any{
		"owner":         "acme",
		"repo":          "demo",
		"nameWithOwner": "acme/demo",
		"defaultBranch": "trunk",
		"url":           "https://github.com/acme/demo",
	}), &database)

	targets := arrayMaps(database.Tables.Projects[0]["repositoryTargets"])
	if len(targets) != 1 {
		t.Fatalf("repository targets = %+v", targets)
	}
	if targets[0]["id"] != "repo_acme_demo" || targets[0]["defaultBranch"] != "trunk" {
		t.Fatalf("repository target = %+v", targets[0])
	}
	if database.Tables.Projects[0]["defaultRepositoryTargetId"] != "repo_acme_demo" {
		t.Fatalf("default target = %+v", database.Tables.Projects[0])
	}

	loaded, err := repo.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(arrayMaps(loaded.Tables.Projects[0]["repositoryTargets"])) != 1 {
		t.Fatalf("loaded project = %+v", loaded.Tables.Projects[0])
	}
}

func TestGitHubDeleteRepositoryTargetRemovesWorkspaceRecords(t *testing.T) {
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)

	var database WorkspaceDatabase
	decode(t, postJSON(t, api.URL+"/github/bind-repository-target", map[string]any{
		"owner":         "acme",
		"repo":          "demo",
		"nameWithOwner": "acme/demo",
		"defaultBranch": "main",
		"url":           "https://github.com/acme/demo",
	}), &database)

	item := map[string]any{
		"id": "item_repo_1", "key": "GH-1", "title": "Repo scoped", "description": "Bound to repo.",
		"status": "Ready", "priority": "High", "assignee": "requirement", "labels": []any{"github"}, "team": "Omega", "stageId": "intake",
		"target": "https://github.com/acme/demo/issues/1", "source": "github_issue", "repositoryTargetId": "repo_acme_demo",
	}
	_ = postJSON(t, api.URL+"/work-items", map[string]any{"item": item})

	response := requestJSON(t, http.MethodDelete, api.URL+"/github/repository-targets/repo_acme_demo", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("delete status = %d", response.StatusCode)
	}
	decode(t, response, &database)
	if len(arrayMaps(database.Tables.Projects[0]["repositoryTargets"])) != 0 {
		t.Fatalf("repository targets = %+v", database.Tables.Projects[0]["repositoryTargets"])
	}
	if len(database.Tables.WorkItems) != 0 {
		t.Fatalf("work items = %+v", database.Tables.WorkItems)
	}
}

func TestLocalCapabilitiesReportsInstalledCliTools(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell scripts use POSIX sh")
	}
	api, _ := newTestAPI(t)

	bin := t.TempDir()
	tools := map[string]string{
		"git":      "git version 2.45.0",
		"gh":       "gh version 2.60.1",
		"lark-cli": "lark-cli version 1.0.18",
	}
	for name, output := range tools {
		path := filepath.Join(bin, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf '%s\\n' '"+output+"'\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)

	response, err := http.Get(api.URL + "/local-capabilities")
	if err != nil {
		t.Fatal(err)
	}
	var capabilities []map[string]any
	decode(t, response, &capabilities)
	byID := map[string]map[string]any{}
	for _, capability := range capabilities {
		byID[fmt.Sprint(capability["id"])] = capability
	}
	if byID["git"]["available"] != true || !strings.Contains(fmt.Sprint(byID["git"]["version"]), "2.45.0") {
		t.Fatalf("git capability = %+v", byID["git"])
	}
	if byID["gh"]["available"] != true || byID["gh"]["category"] != "github" {
		t.Fatalf("gh capability = %+v", byID["gh"])
	}
	if byID["lark-cli"]["available"] != true || byID["lark-cli"]["category"] != "feishu" {
		t.Fatalf("lark-cli capability = %+v", byID["lark-cli"])
	}
	if byID["codex"]["available"] != false || byID["opencode"]["available"] != false {
		t.Fatalf("ai cli capabilities = codex:%+v opencode:%+v", byID["codex"], byID["opencode"])
	}
}

func TestLocalWorkspaceRootCanBeConfiguredAndUsedByRuns(t *testing.T) {
	api, _ := newTestAPI(t)
	root := filepath.Join(t.TempDir(), "omega-workspaces")

	var config map[string]any
	decode(t, requestJSON(t, http.MethodPut, api.URL+"/local-workspace-root", map[string]any{"workspaceRoot": root}), &config)
	if config["workspaceRoot"] != root {
		t.Fatalf("workspace root config = %+v", config)
	}

	var loaded map[string]any
	decode(t, mustGet(t, api.URL+"/local-workspace-root"), &loaded)
	if loaded["workspaceRoot"] != root {
		t.Fatalf("loaded workspace root = %+v", loaded)
	}

	mission := map[string]any{
		"id":               "mission_OMG-77_intake",
		"sourceIssueKey":   "OMG-77",
		"sourceWorkItemId": "item_manual_77",
		"title":            "Use configured workspace root",
		"target":           "No target",
		"operations": []map[string]any{{
			"id":            "operation_intake",
			"stageId":       "intake",
			"agentId":       "requirement",
			"status":        "ready",
			"prompt":        "Collect local proof.",
			"requiredProof": []any{"local-proof"},
		}},
	}
	var result OperationResult
	decode(t, postJSON(t, api.URL+"/operations/run", map[string]any{"mission": mission, "operationId": "operation_intake", "runner": "local-proof"}), &result)
	if !strings.HasPrefix(result.WorkspacePath, root) {
		t.Fatalf("workspace path %q should be under %q", result.WorkspacePath, root)
	}
	var runtimeSpec map[string]any
	raw, err := os.ReadFile(filepath.Join(result.WorkspacePath, ".omega", "agent-runtime.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &runtimeSpec); err != nil {
		t.Fatal(err)
	}
	if runtimeSpec["runner"] != "local-proof" || runtimeSpec["sandboxPolicy"] != "workspace-write" {
		t.Fatalf("runtime spec = %+v", runtimeSpec)
	}
}

func TestProjectAgentProfilePersistsAndFeedsRuntimeBundle(t *testing.T) {
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)

	profile := map[string]any{
		"projectId":        "project_omega",
		"workflowTemplate": "devflow-pr",
		"workflowMarkdown": "workflow: devflow-pr\nstages:\n  - requirement",
		"stagePolicy":      "Human Review blocks delivery.",
		"skillAllowlist":   "browser-use",
		"mcpAllowlist":     "github",
		"codexPolicy":      "workspace-write only",
		"claudePolicy":     "repository only",
		"agentProfiles": []map[string]any{{
			"id":           "requirement",
			"label":        "Requirement",
			"runner":       "codex",
			"model":        "gpt-5.4-mini",
			"skills":       "browser-use",
			"mcp":          "github",
			"stageNotes":   "Clarify acceptance criteria.",
			"codexPolicy":  "write requirement artifact",
			"claudePolicy": "summarize requirement",
		}},
	}
	var saved ProjectAgentProfile
	decode(t, requestJSON(t, http.MethodPut, api.URL+"/agent-profile", profile), &saved)
	if saved.Source != "project" || saved.WorkflowTemplate != "devflow-pr" {
		t.Fatalf("saved profile = %+v", saved)
	}
	storedProfile, err := repo.GetAgentProfile(context.Background(), "project_omega", "")
	if err != nil {
		t.Fatal(err)
	}
	if storedProfile["workflowTemplate"] != "devflow-pr" {
		t.Fatalf("first-class profile = %+v", storedProfile)
	}
	compatProfile, err := repo.GetSetting(context.Background(), agentProfileSettingKey("project_omega", ""))
	if err != nil {
		t.Fatal(err)
	}
	if compatProfile["workflowTemplate"] != "devflow-pr" {
		t.Fatalf("compat profile = %+v", compatProfile)
	}

	var loaded ProjectAgentProfile
	decode(t, mustGet(t, api.URL+"/agent-profile?projectId=project_omega"), &loaded)
	if loaded.Source != "project" || len(loaded.AgentProfiles) != 1 {
		t.Fatalf("loaded profile = %+v", loaded)
	}

	item := map[string]any{
		"id": "item_manual_profile", "key": "OMG-88", "title": "Profile aware run", "description": "Use the saved profile.",
		"status": "Ready", "priority": "High", "assignee": "requirement", "labels": []any{"manual"}, "team": "Omega", "stageId": "intake", "target": "No target", "projectId": "project_omega",
	}
	var pipeline map[string]any
	decode(t, postJSON(t, api.URL+"/pipelines/from-template", map[string]any{"templateId": "devflow-pr", "item": item}), &pipeline)
	runProfile := mapValue(mapValue(pipeline["run"])["agentProfile"])
	if runProfile["source"] != "project" || runProfile["workflowTemplate"] != "devflow-pr" {
		t.Fatalf("pipeline profile metadata = %+v", runProfile)
	}

	mission := map[string]any{
		"id":               "mission_OMG-88_intake",
		"sourceIssueKey":   "OMG-88",
		"sourceWorkItemId": "item_manual_profile",
		"title":            "Use configured profile",
		"target":           "No target",
		"operations": []map[string]any{{
			"id":            "operation_intake",
			"stageId":       "intake",
			"agentId":       "requirement",
			"status":        "ready",
			"prompt":        "Collect local proof.",
			"requiredProof": []any{"local-proof"},
		}},
	}
	var result OperationResult
	decode(t, postJSON(t, api.URL+"/operations/run", map[string]any{"mission": mission, "operationId": "operation_intake", "runner": "local-proof"}), &result)
	raw, err := os.ReadFile(filepath.Join(result.WorkspacePath, ".omega", "agent-runtime.json"))
	if err != nil {
		t.Fatal(err)
	}
	var runtimeSpec map[string]any
	if err := json.Unmarshal(raw, &runtimeSpec); err != nil {
		t.Fatal(err)
	}
	runtimeProfile := mapValue(runtimeSpec["agentProfile"])
	if runtimeProfile["source"] != "project" || mapValue(runtimeProfile["agent"])["id"] != "requirement" {
		t.Fatalf("runtime profile = %+v", runtimeProfile)
	}
	codexPolicy, err := os.ReadFile(filepath.Join(result.WorkspacePath, ".codex", "OMEGA.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(codexPolicy), "write requirement artifact") {
		t.Fatalf("codex policy not written: %s", codexPolicy)
	}
}

func TestProfileRunnerRegistrySelectsConfiguredAgentRunner(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake opencode script uses POSIX sh")
	}
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)
	bin := t.TempDir()
	opencode := filepath.Join(bin, "opencode")
	script := "#!/bin/sh\nprintf '%s\\n' 'opencode profile runner ok'\n"
	if err := os.WriteFile(opencode, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	profile := map[string]any{
		"projectId":        "project_omega",
		"workflowTemplate": "devflow-pr",
		"agentProfiles": []map[string]any{{
			"id":     "coding",
			"label":  "Coding",
			"runner": "opencode",
			"model":  "gpt-5.4-mini",
		}},
	}
	var saved ProjectAgentProfile
	decode(t, requestJSON(t, http.MethodPut, api.URL+"/agent-profile", profile), &saved)
	if saved.AgentProfiles[0].Runner != "opencode" {
		t.Fatalf("saved profile = %+v", saved)
	}

	mission := map[string]any{
		"id":               "mission_OMG-89_coding",
		"sourceIssueKey":   "OMG-89",
		"sourceWorkItemId": "item_manual_89",
		"title":            "Use configured opencode runner",
		"target":           "No target",
		"operations": []map[string]any{{
			"id":      "operation_coding",
			"stageId": "coding",
			"agentId": "coding",
			"status":  "ready",
			"prompt":  "Run using the configured runner.",
		}},
	}
	var result OperationResult
	decode(t, postJSON(t, api.URL+"/operations/run", map[string]any{"mission": mission, "operationId": "operation_coding", "runner": "profile"}), &result)
	if result.Status != "passed" {
		t.Fatalf("operation status = %s result=%+v", result.Status, result)
	}
	if result.RunnerProcess["runner"] != "opencode" || result.RunnerProcess["command"] != "opencode" {
		t.Fatalf("runner process = %+v", result.RunnerProcess)
	}
	if !strings.Contains(result.Stdout, "opencode profile runner ok") {
		t.Fatalf("stdout = %q", result.Stdout)
	}
}

func TestProfileRunnerPreflightRejectsUnavailableRunner(t *testing.T) {
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)

	profile := map[string]any{
		"projectId":        "project_omega",
		"workflowTemplate": "devflow-pr",
		"agentProfiles": []map[string]any{{
			"id":     "coding",
			"label":  "Coding",
			"runner": "opencode",
			"model":  "gpt-5.4-mini",
		}},
	}
	var saved ProjectAgentProfile
	decode(t, requestJSON(t, http.MethodPut, api.URL+"/agent-profile", profile), &saved)
	bin := t.TempDir()
	sqlite, err := exec.LookPath("sqlite3")
	if err != nil {
		t.Skip("sqlite3 is required for runner preflight test")
	}
	if err := os.Symlink(sqlite, filepath.Join(bin, "sqlite3")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	mission := map[string]any{
		"id":               "mission_OMG-90_coding",
		"sourceIssueKey":   "OMG-90",
		"sourceWorkItemId": "item_manual_90",
		"title":            "Reject unavailable runner",
		"target":           "No target",
		"operations": []map[string]any{{
			"id":      "operation_coding",
			"stageId": "coding",
			"agentId": "coding",
			"status":  "ready",
			"prompt":  "This should not start without the configured runner.",
		}},
	}
	response := postJSON(t, api.URL+"/operations/run", map[string]any{"mission": mission, "operationId": "operation_coding", "runner": "profile"})
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", response.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text(body, "error"), "opencode") || !strings.Contains(text(body, "error"), "cannot start") {
		t.Fatalf("error body = %+v", body)
	}
}

func TestGitHubDeliveryContractPreflightChecksPermissions(t *testing.T) {
	bin := t.TempDir()
	gh := filepath.Join(bin, "gh")
	script := `#!/bin/sh
case "$1 $2" in
  "auth status")
    exit 0
    ;;
  "repo view")
    printf '%s' '{"nameWithOwner":"ZYOOO/TestRepo","viewerPermission":"WRITE","defaultBranchRef":{"name":"main"}}'
    exit 0
    ;;
  "pr list")
    printf '%s' '[]'
    exit 0
    ;;
esac
echo "unexpected gh args: $*" >&2
exit 1
`
	if err := os.WriteFile(gh, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	checks := githubDeliveryContractPreflight(context.Background(), map[string]any{
		"kind":  "github",
		"owner": "ZYOOO",
		"repo":  "TestRepo",
	})
	statusByID := map[string]string{}
	for _, check := range checks {
		statusByID[check.ID] = check.Status
	}
	for _, id := range []string{"github-auth", "github-repository", "github-branch-permission", "github-pr-create-permission", "github-checks-read-permission"} {
		if statusByID[id] != "passed" {
			t.Fatalf("%s should pass, checks=%+v", id, checks)
		}
	}
}

func TestWorkspaceChildPathCannotEscapeConfiguredRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "omega-workspaces")
	workspace, err := workspaceChildPath(root, "../../escape", "../coding")
	if err != nil {
		t.Fatal(err)
	}
	if !pathInsideRoot(root, workspace) {
		t.Fatalf("workspace path %q escaped root %q", workspace, root)
	}
	if strings.Contains(workspace, "..") {
		t.Fatalf("workspace path should be sanitized: %q", workspace)
	}
	if _, err := ensurePathInsideRoot(root, filepath.Join(root, "..", "escape")); err == nil {
		t.Fatalf("expected explicit outside path to be rejected")
	}
}

func TestPagePilotApplyAndDeliverUsesLocalRepositoryTarget(t *testing.T) {
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)
	targetRepo := createDemoGitRepo(t)
	sourceFile := filepath.Join(targetRepo, "src", "Page.tsx")
	if err := os.WriteFile(sourceFile, []byte("export function Page() {\n  return <h1>Old headline</h1>;\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, targetRepo, "add", ".")
	runGit(t, targetRepo, "commit", "-m", "add page")

	database, err := repo.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	project := cloneMap(database.Tables.Projects[0])
	project["repositoryTargets"] = []any{map[string]any{
		"id":            "repo_local_page",
		"kind":          "local",
		"path":          targetRepo,
		"defaultBranch": "main",
	}}
	project["defaultRepositoryTargetId"] = "repo_local_page"
	database.Tables.Projects[0] = project
	if err := repo.Save(context.Background(), *database); err != nil {
		t.Fatal(err)
	}

	profile := map[string]any{
		"projectId":          "project_omega",
		"repositoryTargetId": "repo_local_page",
		"workflowTemplate":   "devflow-pr",
		"agentProfiles": []map[string]any{{
			"id":     "coding",
			"label":  "Coding",
			"runner": "local-proof",
			"model":  "local",
		}},
	}
	var saved ProjectAgentProfile
	decode(t, requestJSON(t, http.MethodPut, api.URL+"/agent-profile", profile), &saved)

	selection := map[string]any{
		"elementKind":    "title",
		"stableSelector": `[data-omega-source="src/Page.tsx:headline"]`,
		"textSnapshot":   "Old headline",
		"styleSnapshot":  map[string]any{"fontSize": "32px"},
		"domContext":     map[string]any{"tagName": "h1"},
		"sourceMapping":  map[string]any{"source": "src/Page.tsx:headline", "file": "src/Page.tsx", "symbol": "headline"},
	}
	conflictScope := "devflow:repo_local_page:item_other"
	conflictLock := map[string]any{
		"id":                 executionLockID(conflictScope),
		"scope":              conflictScope,
		"status":             "claimed",
		"attemptId":          "attempt_other",
		"repositoryTargetId": "repo_local_page",
		"repositoryPath":     targetRepo,
		"expiresAt":          time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano),
		"createdAt":          nowISO(),
		"updatedAt":          nowISO(),
	}
	if err := repo.SetSetting(context.Background(), text(conflictLock, "id"), conflictLock); err != nil {
		t.Fatal(err)
	}
	conversationBatch := map[string]any{
		"id":                  "page_pilot_batch_test",
		"createdAt":           "17:05:38",
		"primaryAnnotationId": 1,
		"instruction":         `replace text with "New headline"`,
		"annotations": []any{map[string]any{
			"id":        1,
			"comment":   "Rename the headline",
			"selection": selection,
		}},
		"status": "running",
	}
	submittedAnnotations := []any{map[string]any{
		"id":        1,
		"comment":   "Rename the headline",
		"selection": selection,
	}}
	processEvents := []any{
		map[string]any{"at": "17:05:38", "text": "Captured 1 page annotation(s)."},
		map[string]any{"at": "17:05:38", "text": "Submitting selection context to the single Page Pilot Agent."},
	}
	previewRuntimeProfile := map[string]any{
		"agentId":            "preview-runtime-agent",
		"stageId":            "preview_runtime",
		"repositoryTargetId": "repo_local_page",
		"workingDirectory":   targetRepo,
		"previewUrl":         "http://127.0.0.1:3009/",
		"source":             "npm:dev",
		"reloadStrategy":     "hmr-wait",
		"devCommand":         "npm run dev -- --host 127.0.0.1 --port 3009",
		"evidence":           []any{"package.json"},
	}
	blockedByOperation := postJSON(t, api.URL+"/page-pilot/apply", map[string]any{
		"projectId":          "project_omega",
		"repositoryTargetId": "repo_local_page",
		"instruction":        `replace text with "New headline"`,
		"selection":          selection,
		"runner":             "profile",
	})
	if blockedByOperation.StatusCode != http.StatusBadRequest {
		t.Fatalf("Page Pilot apply should be blocked by an active repository lock, status = %d", blockedByOperation.StatusCode)
	}
	_ = blockedByOperation.Body.Close()
	conflictLock["status"] = "released"
	if err := repo.SetSetting(context.Background(), text(conflictLock, "id"), conflictLock); err != nil {
		t.Fatal(err)
	}
	var applied map[string]any
	decode(t, postJSON(t, api.URL+"/page-pilot/apply", map[string]any{
		"projectId":             "project_omega",
		"repositoryTargetId":    "repo_local_page",
		"instruction":           `replace text with "New headline"`,
		"selection":             selection,
		"runner":                "profile",
		"conversationBatch":     conversationBatch,
		"submittedAnnotations":  submittedAnnotations,
		"processEvents":         processEvents,
		"previewRuntimeProfile": previewRuntimeProfile,
	}), &applied)
	if applied["status"] != "applied" || applied["repositoryPath"] != targetRepo {
		t.Fatalf("applied = %+v", applied)
	}
	runID := text(applied, "id")
	if runID == "" {
		t.Fatalf("Page Pilot run id missing: %+v", applied)
	}
	raw, err := os.ReadFile(sourceFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "New headline") {
		t.Fatalf("source not patched: %s", raw)
	}
	if got := arrayValues(applied["changedFiles"]); len(got) != 1 || got[0] != "src/Page.tsx" {
		t.Fatalf("changed files = %+v", applied["changedFiles"])
	}
	if batch := mapValue(applied["conversationBatch"]); text(batch, "status") != "applied" || text(batch, "runId") != runID || len(arrayMaps(batch["annotations"])) != 1 {
		t.Fatalf("conversation batch should be persisted on apply: %+v", batch)
	}
	if len(arrayMaps(applied["submittedAnnotations"])) != 1 || len(arrayMaps(applied["processEvents"])) != 2 {
		t.Fatalf("conversation details missing from apply result: %+v", applied)
	}
	if primary := mapValue(applied["primaryTarget"]); fmt.Sprint(primary["annotationId"]) != "1" {
		t.Fatalf("primary target missing: %+v", primary)
	}
	if profile := mapValue(applied["previewRuntimeProfile"]); text(profile, "source") != "npm:dev" || text(profile, "workingDirectory") != targetRepo {
		t.Fatalf("preview runtime profile should be persisted on apply: %+v", profile)
	}
	if report := mapValue(applied["sourceMappingReport"]); report["strongSourceMappings"].(float64) != 1 || text(report, "status") != "strong" {
		t.Fatalf("source mapping report should count strong mappings: %+v", report)
	}
	lock := mapValue(applied["executionLock"])
	if text(lock, "status") != "claimed" || text(lock, "ownerType") != "page-pilot" || text(lock, "pagePilotRunId") != runID {
		t.Fatalf("Page Pilot apply should hold a live-preview lock: %+v", lock)
	}
	selectionRoundTwo := cloneMap(selection)
	selectionRoundTwo["textSnapshot"] = "New headline"
	var reapplied map[string]any
	decode(t, postJSON(t, api.URL+"/page-pilot/apply", map[string]any{
		"runId":                 runID,
		"projectId":             "project_omega",
		"repositoryTargetId":    "repo_local_page",
		"instruction":           `replace text with "Round two headline"`,
		"selection":             selectionRoundTwo,
		"runner":                "profile",
		"conversationBatch":     conversationBatch,
		"submittedAnnotations":  submittedAnnotations,
		"processEvents":         processEvents,
		"previewRuntimeProfile": previewRuntimeProfile,
	}), &reapplied)
	if text(reapplied, "id") != runID || intNumber(reapplied["roundNumber"]) != 2 {
		t.Fatalf("multi-round apply should keep the same run and increment round: %+v", reapplied)
	}
	raw, err = os.ReadFile(sourceFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "Round two headline") {
		t.Fatalf("multi-round source not patched: %s", raw)
	}
	blockedApply := postJSON(t, api.URL+"/page-pilot/apply", map[string]any{
		"projectId":          "project_omega",
		"repositoryTargetId": "repo_local_page",
		"instruction":        `replace text with "Another headline"`,
		"selection":          selection,
		"runner":             "profile",
	})
	if blockedApply.StatusCode != http.StatusBadRequest {
		t.Fatalf("second Page Pilot apply should be blocked by live-preview lock, status = %d", blockedApply.StatusCode)
	}
	_ = blockedApply.Body.Close()
	storedRun, err := repo.GetPagePilotRun(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if storedRun["status"] != "applied" || storedRun["repositoryTargetId"] != "repo_local_page" {
		t.Fatalf("stored run = %+v", storedRun)
	}
	if text(storedRun, "requirementId") == "" || text(storedRun, "workItemId") == "" || text(storedRun, "pipelineId") == "" {
		t.Fatalf("stored run should link Page Pilot back to feature-one records: %+v", storedRun)
	}
	if batch := mapValue(storedRun["conversationBatch"]); text(batch, "status") != "applied" || len(arrayMaps(batch["annotations"])) != 1 {
		t.Fatalf("stored run should keep server-side Page Pilot conversation: %+v", storedRun)
	}
	if profile := mapValue(storedRun["previewRuntimeProfile"]); text(profile, "agentId") != "preview-runtime-agent" {
		t.Fatalf("stored run should keep preview runtime profile: %+v", storedRun)
	}
	if prPreview := mapValue(storedRun["prPreview"]); text(prPreview, "body") == "" {
		t.Fatalf("stored run should keep PR preview: %+v", storedRun)
	}
	if visualProof := mapValue(storedRun["visualProof"]); text(visualProof, "kind") != "dom-snapshot" {
		t.Fatalf("stored run should keep visual proof: %+v", storedRun)
	}
	if report := mapValue(storedRun["sourceMappingReport"]); text(report, "status") != "strong" {
		t.Fatalf("stored run should keep source mapping report: %+v", storedRun)
	}
	linkedDatabase, err := repo.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	workItem := findWorkItem(*linkedDatabase, text(storedRun, "workItemId"))
	if workItem == nil || workItem["source"] != "page_pilot" || workItem["repositoryTargetId"] != "repo_local_page" {
		t.Fatalf("Page Pilot work item missing or unscoped: %+v", workItem)
	}
	requirementIndex := findByID(linkedDatabase.Tables.Requirements, text(storedRun, "requirementId"))
	if requirementIndex < 0 || linkedDatabase.Tables.Requirements[requirementIndex]["source"] != "page_pilot" {
		t.Fatalf("Page Pilot requirement missing: %+v", linkedDatabase.Tables.Requirements)
	}
	pipelineIndex := findByID(linkedDatabase.Tables.Pipelines, text(storedRun, "pipelineId"))
	if pipelineIndex < 0 || linkedDatabase.Tables.Pipelines[pipelineIndex]["templateId"] != "page-pilot" {
		t.Fatalf("Page Pilot pipeline missing: %+v", linkedDatabase.Tables.Pipelines)
	}
	pagePilotProofs := 0
	for _, proof := range linkedDatabase.Tables.ProofRecords {
		if strings.Contains(text(proof, "operationId"), text(storedRun, "pipelineId")) {
			pagePilotProofs++
		}
	}
	if pagePilotProofs == 0 {
		t.Fatalf("Page Pilot proof records missing: %+v", linkedDatabase.Tables.ProofRecords)
	}
	pagePilotOperations := 0
	for _, operation := range linkedDatabase.Tables.Operations {
		if strings.Contains(text(operation, "missionId"), text(storedRun, "pipelineId")) && text(operation, "agentId") == "page-pilot" {
			pagePilotOperations++
		}
	}
	if pagePilotOperations == 0 {
		t.Fatalf("Page Pilot operation record missing: %+v", linkedDatabase.Tables.Operations)
	}
	var runs []map[string]any
	decode(t, mustGet(t, api.URL+"/page-pilot/runs"), &runs)
	if len(runs) != 1 || runs[0]["id"] != runID || runs[0]["status"] != "applied" {
		t.Fatalf("runs = %+v", runs)
	}
	var discarded map[string]any
	decode(t, postJSON(t, api.URL+"/page-pilot/runs/"+runID+"/discard", map[string]any{}), &discarded)
	if discarded["status"] != "discarded" {
		t.Fatalf("discarded = %+v", discarded)
	}
	if batch := mapValue(discarded["conversationBatch"]); text(batch, "status") != "discarded" {
		t.Fatalf("discard should mark conversation discarded: %+v", batch)
	}
	raw, err = os.ReadFile(sourceFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "Old headline") {
		t.Fatalf("source not discarded: %s", raw)
	}
	releasedLock, err := repo.GetSetting(context.Background(), text(lock, "id"))
	if err != nil {
		t.Fatal(err)
	}
	if text(releasedLock, "status") != "released" {
		t.Fatalf("discard should release Page Pilot lock: %+v", releasedLock)
	}
	discardedDatabase, err := repo.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	discardedItemIndex := findByID(discardedDatabase.Tables.WorkItems, text(discarded, "workItemId"))
	if discardedItemIndex < 0 || discardedDatabase.Tables.WorkItems[discardedItemIndex]["status"] != "Blocked" {
		t.Fatalf("discarded Page Pilot work item status = %+v", discardedDatabase.Tables.WorkItems)
	}
	discardedPipelineIndex := findByID(discardedDatabase.Tables.Pipelines, text(discarded, "pipelineId"))
	if discardedPipelineIndex < 0 || discardedDatabase.Tables.Pipelines[discardedPipelineIndex]["status"] != "discarded" {
		t.Fatalf("discarded Page Pilot pipeline status = %+v", discardedDatabase.Tables.Pipelines)
	}

	decode(t, postJSON(t, api.URL+"/page-pilot/apply", map[string]any{
		"projectId":             "project_omega",
		"repositoryTargetId":    "repo_local_page",
		"instruction":           `replace text with "New headline"`,
		"selection":             selection,
		"runner":                "profile",
		"conversationBatch":     conversationBatch,
		"submittedAnnotations":  submittedAnnotations,
		"processEvents":         processEvents,
		"previewRuntimeProfile": previewRuntimeProfile,
	}), &applied)
	runID = text(applied, "id")
	lock = mapValue(applied["executionLock"])

	var delivered map[string]any
	decode(t, postJSON(t, api.URL+"/page-pilot/deliver", map[string]any{
		"runId":                 runID,
		"projectId":             "project_omega",
		"repositoryTargetId":    "repo_local_page",
		"instruction":           `replace text with "New headline"`,
		"selection":             selection,
		"branchName":            "omega/page-pilot-test",
		"conversationBatch":     mapValue(applied["conversationBatch"]),
		"submittedAnnotations":  arrayMaps(applied["submittedAnnotations"]),
		"processEvents":         append(arrayMaps(applied["processEvents"]), map[string]any{"at": "17:06:00", "text": "User confirmed the Page Pilot changes for delivery."}),
		"previewRuntimeProfile": mapValue(applied["previewRuntimeProfile"]),
	}), &delivered)
	if delivered["status"] != "delivered" || delivered["branchName"] != "omega/page-pilot-test" || delivered["commitSha"] == "" {
		t.Fatalf("delivered = %+v", delivered)
	}
	if batch := mapValue(delivered["conversationBatch"]); text(batch, "status") != "delivered" || len(arrayMaps(delivered["processEvents"])) != 3 {
		t.Fatalf("delivery should keep conversation context: %+v", delivered)
	}
	if profile := mapValue(delivered["previewRuntimeProfile"]); text(profile, "reloadStrategy") != "hmr-wait" {
		t.Fatalf("delivery should keep preview runtime profile: %+v", delivered)
	}
	if report := mapValue(delivered["sourceMappingReport"]); text(report, "status") != "strong" {
		t.Fatalf("delivery should keep source mapping report: %+v", delivered)
	}
	releasedLock, err = repo.GetSetting(context.Background(), text(lock, "id"))
	if err != nil {
		t.Fatal(err)
	}
	if text(releasedLock, "status") != "released" {
		t.Fatalf("delivery should release Page Pilot lock: %+v", releasedLock)
	}
	deliveredRun, err := repo.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	pipelineIndex = findByID(deliveredRun.Tables.Pipelines, text(delivered, "pipelineId"))
	if pipelineIndex < 0 || deliveredRun.Tables.Pipelines[pipelineIndex]["status"] != "delivered" {
		t.Fatalf("Page Pilot delivered pipeline not updated: %+v", deliveredRun.Tables.Pipelines)
	}
	if branch := strings.TrimSpace(runGit(t, targetRepo, "branch", "--show-current")); branch != "omega/page-pilot-test" {
		t.Fatalf("branch = %s", branch)
	}
}

func TestPagePilotDomOnlySelectionUsesSourceLocator(t *testing.T) {
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)
	targetRepo := createDemoGitRepo(t)
	sourceFile := filepath.Join(targetRepo, "src", "Page.tsx")
	if err := os.WriteFile(sourceFile, []byte("export function Page() {\n  return <button className=\"card-action\">Start trial</button>;\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, targetRepo, "add", ".")
	runGit(t, targetRepo, "commit", "-m", "add page")

	database, err := repo.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	project := cloneMap(database.Tables.Projects[0])
	project["repositoryTargets"] = []any{map[string]any{
		"id":            "repo_local_dom_only",
		"kind":          "local",
		"path":          targetRepo,
		"defaultBranch": "main",
	}}
	project["defaultRepositoryTargetId"] = "repo_local_dom_only"
	database.Tables.Projects[0] = project
	if err := repo.Save(context.Background(), *database); err != nil {
		t.Fatal(err)
	}
	decode(t, requestJSON(t, http.MethodPut, api.URL+"/agent-profile", map[string]any{
		"projectId":          "project_omega",
		"repositoryTargetId": "repo_local_dom_only",
		"workflowTemplate":   "devflow-pr",
		"agentProfiles": []map[string]any{{
			"id":     "coding",
			"label":  "Coding",
			"runner": "local-proof",
			"model":  "local",
		}},
	}), &ProjectAgentProfile{})

	selection := map[string]any{
		"elementKind":    "button",
		"stableSelector": `.card-action`,
		"textSnapshot":   "Start trial",
		"styleSnapshot":  map[string]any{"fontWeight": "700"},
		"domContext":     map[string]any{"tagName": "button"},
		"sourceMapping":  map[string]any{"source": "DOM-only", "file": "", "symbol": ""},
	}
	var applied map[string]any
	decode(t, postJSON(t, api.URL+"/page-pilot/apply", map[string]any{
		"projectId":          "project_omega",
		"repositoryTargetId": "repo_local_dom_only",
		"instruction":        `replace text with "Launch now"`,
		"selection":          selection,
		"runner":             "profile",
	}), &applied)
	if applied["status"] != "applied" {
		t.Fatalf("applied = %+v", applied)
	}
	raw, err := os.ReadFile(sourceFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "Launch now") {
		t.Fatalf("DOM-only source locator did not patch candidate source: %s", raw)
	}
	report := mapValue(applied["sourceMappingReport"])
	if text(report, "status") != "dom-only" || report["domOnlySelections"].(float64) != 1 {
		t.Fatalf("source mapping report = %+v", report)
	}
	locator := mapValue(applied["sourceLocator"])
	if text(locator, "status") != "candidates-ready" {
		t.Fatalf("source locator = %+v", locator)
	}
	candidates := arrayMaps(arrayMaps(locator["results"])[0]["candidates"])
	if len(candidates) == 0 || text(candidates[0], "file") != "src/Page.tsx" {
		t.Fatalf("source candidates = %+v", candidates)
	}
	promptBytes, err := os.ReadFile(stringSlice(applied["proofFiles"])[0])
	if err != nil {
		t.Fatal(err)
	}
	prompt := string(promptBytes)
	if !strings.Contains(prompt, "Source locator candidates") || !strings.Contains(prompt, "src/Page.tsx") {
		t.Fatalf("prompt should include source locator candidates:\n%s", prompt)
	}
}

func TestPagePilotPreviewRuntimeStartPersistsGoProfile(t *testing.T) {
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)
	targetRepo := createDemoGitRepo(t)
	if err := os.WriteFile(filepath.Join(targetRepo, "package.json"), []byte(`{"scripts":{"dev":"vite --host 127.0.0.1"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, targetRepo, "add", ".")
	runGit(t, targetRepo, "commit", "-m", "add preview script")

	database, err := repo.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	project := cloneMap(database.Tables.Projects[0])
	project["repositoryTargets"] = []any{map[string]any{
		"id":            "repo_local_preview",
		"kind":          "local",
		"path":          targetRepo,
		"defaultBranch": "main",
	}}
	project["defaultRepositoryTargetId"] = "repo_local_preview"
	database.Tables.Projects[0] = project
	if err := repo.Save(context.Background(), *database); err != nil {
		t.Fatal(err)
	}
	preview := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_, _ = response.Write([]byte("ok"))
	}))
	defer preview.Close()

	var result map[string]any
	decode(t, postJSON(t, api.URL+"/page-pilot/preview-runtime/start", map[string]any{
		"projectId":          "project_omega",
		"repositoryTargetId": "repo_local_preview",
		"previewUrl":         preview.URL,
	}), &result)
	if result["ok"] != true || text(result, "status") != "external" {
		t.Fatalf("preview runtime result = %+v", result)
	}
	profile := mapValue(result["profile"])
	if text(profile, "agentId") != "preview-runtime-agent" || text(profile, "source") != "npm:dev" {
		t.Fatalf("preview runtime profile = %+v", profile)
	}
	stored, err := repo.GetSetting(context.Background(), pagePilotPreviewRuntimeKey("repo_local_preview"))
	if err != nil {
		t.Fatal(err)
	}
	if text(mapValue(stored["profile"]), "workingDirectory") != targetRepo {
		t.Fatalf("stored preview runtime = %+v", stored)
	}
}

func TestFeishuNotifyUsesLocalLarkCLI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell script uses POSIX sh")
	}
	api, _ := newTestAPI(t)

	bin := t.TempDir()
	argsFile := filepath.Join(bin, "args.txt")
	larkCLI := filepath.Join(bin, "lark-cli")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" > " + argsFile + "\nprintf '%s' '{\"message_id\":\"om_123\"}'\n"
	if err := os.WriteFile(larkCLI, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	response := postJSON(t, api.URL+"/feishu/notify", map[string]any{"chatId": "oc_demo", "text": "Pipeline waiting for review"})
	var result map[string]any
	decode(t, response, &result)
	if result["status"] != "sent" || result["messageId"] != "om_123" {
		t.Fatalf("notify result = %+v", result)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "im +messages-send --chat-id oc_demo --text Pipeline waiting for review") {
		t.Fatalf("lark-cli args = %s", string(args))
	}
}

func TestFeishuReviewRequestSendsInteractiveWebhookCard(t *testing.T) {
	api, repo := newTestAPI(t)
	database := WorkspaceDatabase{
		SchemaVersion: 1,
		SavedAt:       nowISO(),
		Tables: WorkspaceTables{
			Projects: []map[string]any{{"id": "project_omega", "name": "Omega", "status": "Active", "createdAt": nowISO(), "updatedAt": nowISO()}},
			Requirements: []map[string]any{{
				"id": "req_1", "title": "Add review card", "description": strings.Repeat("Requirement detail. ", 20), "status": "converted", "structured": map[string]any{}, "createdAt": nowISO(), "updatedAt": nowISO(),
			}},
			WorkItems: []map[string]any{{
				"id": "item_1", "projectId": "project_omega", "requirementId": "req_1", "key": "OMG-1", "title": "Add review card", "description": "Send review card.", "status": "In Review", "labels": []any{}, "acceptanceCriteria": []any{}, "risks": []any{}, "createdAt": nowISO(), "updatedAt": nowISO(),
			}},
			Pipelines: []map[string]any{{
				"id": "pipeline_1", "workItemId": "item_1", "status": "waiting-human", "templateId": "devflow-pr", "run": map[string]any{"stages": []any{}}, "createdAt": nowISO(), "updatedAt": nowISO(),
			}},
			Attempts: []map[string]any{{
				"id": "attempt_1", "itemId": "item_1", "pipelineId": "pipeline_1", "status": "waiting-human", "branchName": "omega/OMG-1-devflow", "pullRequestUrl": "https://github.com/ZYOOO/TestRepo/pull/1", "reviewPacket": map[string]any{"summary": "Review packet ready.", "risk": map[string]any{"level": "medium"}}, "createdAt": nowISO(), "updatedAt": nowISO(),
			}},
			Checkpoints: []map[string]any{{
				"id": "pipeline_1:human_review", "pipelineId": "pipeline_1", "attemptId": "attempt_1", "stageId": "human_review", "status": "pending", "title": "Human Review", "summary": "Waiting for approval.", "createdAt": nowISO(), "updatedAt": nowISO(),
			}},
		},
	}
	if err := repo.Save(context.Background(), database); err != nil {
		t.Fatal(err)
	}
	var received map[string]any
	webhook := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Fatal(err)
		}
		writeJSON(response, http.StatusOK, map[string]any{"StatusCode": 0, "message_id": "om_card"})
	}))
	defer webhook.Close()
	t.Setenv("OMEGA_FEISHU_WEBHOOK_URL", webhook.URL)
	t.Setenv("OMEGA_PUBLIC_APP_URL", "http://127.0.0.1:5173")
	t.Setenv("OMEGA_PUBLIC_API_URL", "https://omega.example.test")
	t.Setenv("OMEGA_FEISHU_REVIEW_TOKEN", "secret-token")

	response := postJSON(t, api.URL+"/feishu/review-request", map[string]any{"checkpointId": "pipeline_1:human_review"})
	var result map[string]any
	decode(t, response, &result)
	if text(result, "status") != "sent" || text(result, "format") != "interactive-card" {
		t.Fatalf("review request result = %+v", result)
	}
	if text(received, "msg_type") != "interactive" {
		t.Fatalf("webhook payload = %+v", received)
	}
	card := mapValue(received["card"])
	if text(mapValue(card["header"]), "template") != "orange" {
		t.Fatalf("card header = %+v", card["header"])
	}
	loaded, err := repo.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := loaded.Tables.Checkpoints[0]
	if text(mapValue(checkpoint["feishuReview"]), "status") != "sent" {
		t.Fatalf("checkpoint feishu review = %+v", checkpoint["feishuReview"])
	}
}

func TestFeishuReviewRequestUsesLarkCLIInteractiveCard(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell script uses POSIX sh")
	}
	api, repo := newTestAPI(t)
	database := WorkspaceDatabase{
		SchemaVersion: 1,
		SavedAt:       nowISO(),
		Tables: WorkspaceTables{
			Projects: []map[string]any{{"id": "project_omega", "name": "Omega", "status": "Active", "createdAt": nowISO(), "updatedAt": nowISO()}},
			Requirements: []map[string]any{{
				"id": "req_1", "title": "Add review card", "description": "Requirement detail.", "status": "converted", "structured": map[string]any{}, "createdAt": nowISO(), "updatedAt": nowISO(),
			}},
			WorkItems: []map[string]any{{
				"id": "item_1", "projectId": "project_omega", "requirementId": "req_1", "key": "OMG-1", "title": "Add review card", "description": "Send review card.", "status": "In Review", "labels": []any{}, "acceptanceCriteria": []any{}, "risks": []any{}, "createdAt": nowISO(), "updatedAt": nowISO(),
			}},
			Pipelines: []map[string]any{{
				"id": "pipeline_1", "workItemId": "item_1", "status": "waiting-human", "templateId": "devflow-pr", "run": map[string]any{"stages": []any{}}, "createdAt": nowISO(), "updatedAt": nowISO(),
			}},
			Attempts: []map[string]any{{
				"id": "attempt_1", "itemId": "item_1", "pipelineId": "pipeline_1", "status": "waiting-human", "branchName": "omega/OMG-1-devflow", "pullRequestUrl": "https://github.com/ZYOOO/TestRepo/pull/1", "reviewPacket": map[string]any{"summary": "Review packet ready.", "risk": map[string]any{"level": "medium"}}, "createdAt": nowISO(), "updatedAt": nowISO(),
			}},
			Checkpoints: []map[string]any{{
				"id": "pipeline_1:human_review", "pipelineId": "pipeline_1", "attemptId": "attempt_1", "stageId": "human_review", "status": "pending", "title": "Human Review", "summary": "Waiting for approval.", "createdAt": nowISO(), "updatedAt": nowISO(),
			}},
		},
	}
	if err := repo.Save(context.Background(), database); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	argsFile := filepath.Join(bin, "args.txt")
	larkCLI := filepath.Join(bin, "lark-cli")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" > " + argsFile + "\nprintf '%s' '{\"message_id\":\"om_card\"}'\n"
	if err := os.WriteFile(larkCLI, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("OMEGA_PUBLIC_APP_URL", "http://127.0.0.1:5173")

	response := postJSON(t, api.URL+"/feishu/review-request", map[string]any{"checkpointId": "pipeline_1:human_review", "chatId": "oc_demo"})
	var result map[string]any
	decode(t, response, &result)
	if text(result, "status") != "sent" || text(result, "format") != "interactive-card" || text(result, "tool") != "lark-cli" {
		t.Fatalf("review request result = %+v", result)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	argsText := string(args)
	for _, expected := range []string{"im +messages-send --chat-id oc_demo", "--msg-type interactive", "--content"} {
		if !strings.Contains(argsText, expected) {
			t.Fatalf("lark-cli args missing %q: %s", expected, argsText)
		}
	}
	loaded, err := repo.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if text(mapValue(loaded.Tables.Checkpoints[0]["feishuReview"]), "format") != "interactive-card" {
		t.Fatalf("checkpoint feishu review = %+v", loaded.Tables.Checkpoints[0]["feishuReview"])
	}
}

func TestFeishuReviewCallbackApprovesCheckpointThroughSharedDecisionPath(t *testing.T) {
	api, repo := newTestAPI(t)
	database := WorkspaceDatabase{
		SchemaVersion: 1,
		SavedAt:       nowISO(),
		Tables: WorkspaceTables{
			Projects:  []map[string]any{{"id": "project_omega", "name": "Omega", "status": "Active", "createdAt": nowISO(), "updatedAt": nowISO()}},
			WorkItems: []map[string]any{{"id": "item_1", "projectId": "project_omega", "key": "OMG-1", "title": "Review item", "status": "In Review", "labels": []any{}, "acceptanceCriteria": []any{}, "risks": []any{}, "createdAt": nowISO(), "updatedAt": nowISO()}},
			Pipelines: []map[string]any{{
				"id": "pipeline_1", "workItemId": "item_1", "status": "waiting-human", "templateId": "manual-review", "run": map[string]any{"stages": []any{map[string]any{"id": "human_review", "status": "needs-human"}, map[string]any{"id": "done", "status": "waiting"}}}, "createdAt": nowISO(), "updatedAt": nowISO(),
			}},
			Checkpoints: []map[string]any{{"id": "pipeline_1:human_review", "pipelineId": "pipeline_1", "stageId": "human_review", "status": "pending", "title": "Human Review", "summary": "Waiting.", "createdAt": nowISO(), "updatedAt": nowISO()}},
		},
	}
	if err := repo.Save(context.Background(), database); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OMEGA_FEISHU_REVIEW_TOKEN", "secret-token")

	response := postJSON(t, api.URL+"/feishu/review-callback", map[string]any{"checkpointId": "pipeline_1:human_review", "action": "approve", "reviewer": "feishu-user", "token": "secret-token"})
	var result map[string]any
	decode(t, response, &result)
	if text(result, "status") != "approved" {
		t.Fatalf("callback result = %+v", result)
	}
	loaded, err := repo.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if text(loaded.Tables.Checkpoints[0], "status") != "approved" || !strings.Contains(text(loaded.Tables.Checkpoints[0], "decisionNote"), "feishu-user") {
		t.Fatalf("checkpoint after callback = %+v", loaded.Tables.Checkpoints[0])
	}
}

func TestSQLiteSaveToleratesLegacyDuplicateRecordIDs(t *testing.T) {
	repo := NewSQLiteRepository(filepath.Join(t.TempDir(), "omega.db"))
	timestamp := nowISO()
	database := WorkspaceDatabase{
		SchemaVersion: 1,
		SavedAt:       timestamp,
		Tables: WorkspaceTables{
			Projects: []map[string]any{{
				"id":          "project_omega",
				"name":        "Omega",
				"description": "Omega",
				"team":        "Omega",
				"status":      "Active",
				"labels":      []any{},
				"createdAt":   timestamp,
				"updatedAt":   timestamp,
			}},
			WorkItems: []map[string]any{
				{"id": "item_manual_1", "projectId": "project_omega", "key": "OMG-1", "title": "Original", "description": "", "status": "Ready", "priority": "High", "assignee": "requirement", "labels": []any{}, "team": "Omega", "stageId": "intake", "target": "No target", "createdAt": timestamp, "updatedAt": timestamp},
				{"id": "item_manual_1", "projectId": "project_omega", "key": "OMG-1", "title": "Legacy duplicate", "description": "", "status": "Ready", "priority": "High", "assignee": "requirement", "labels": []any{}, "team": "Omega", "stageId": "intake", "target": "No target", "createdAt": timestamp, "updatedAt": timestamp},
			},
			ProofRecords: []map[string]any{
				{"id": "operation_intake:proof:1", "operationId": "operation_intake", "label": "proof-file", "value": "one.txt", "createdAt": timestamp},
				{"id": "operation_intake:proof:1", "operationId": "operation_intake", "label": "proof-file", "value": "two.txt", "createdAt": timestamp},
			},
		},
	}

	if err := repo.Save(context.Background(), database); err != nil {
		t.Fatalf("Save should tolerate duplicate legacy ids, got %v", err)
	}
	loaded, err := repo.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Tables.WorkItems) != 2 {
		t.Fatalf("snapshot work items should be preserved, got %+v", loaded.Tables.WorkItems)
	}
}

func TestPagePilotApplyUsesIsolatedWorkspaceForGitHubTarget(t *testing.T) {
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)
	previewRoot := t.TempDir()
	t.Setenv("OMEGA_PAGE_PILOT_WORKSPACE_ROOT", previewRoot)
	targetRepo := filepath.Join(previewRoot, safeSegment("ZYOOO_TestRepo"))
	if err := os.MkdirAll(filepath.Join(targetRepo, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	sourceFile := filepath.Join(targetRepo, "src", "Page.tsx")
	if err := os.WriteFile(sourceFile, []byte("export function Page() {\n  return <h1>Old headline</h1>;\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, targetRepo, "init")
	runGit(t, targetRepo, "checkout", "-b", "main")
	runGit(t, targetRepo, "config", "user.email", "omega-test@example.local")
	runGit(t, targetRepo, "config", "user.name", "Omega Test")
	runGit(t, targetRepo, "add", ".")
	runGit(t, targetRepo, "commit", "-m", "add page")

	database, err := repo.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	project := cloneMap(database.Tables.Projects[0])
	project["repositoryTargets"] = []any{map[string]any{
		"id":            "repo_ZYOOO_TestRepo",
		"kind":          "github",
		"owner":         "ZYOOO",
		"repo":          "TestRepo",
		"url":           "https://github.com/ZYOOO/TestRepo",
		"defaultBranch": "main",
	}}
	project["defaultRepositoryTargetId"] = "repo_ZYOOO_TestRepo"
	database.Tables.Projects[0] = project
	if err := repo.Save(context.Background(), *database); err != nil {
		t.Fatal(err)
	}

	var saved ProjectAgentProfile
	decode(t, requestJSON(t, http.MethodPut, api.URL+"/agent-profile", map[string]any{
		"projectId":          "project_omega",
		"repositoryTargetId": "repo_ZYOOO_TestRepo",
		"workflowTemplate":   "devflow-pr",
		"agentProfiles": []map[string]any{{
			"id":     "coding",
			"label":  "Coding",
			"runner": "local-proof",
			"model":  "local",
		}},
	}), &saved)

	selection := map[string]any{
		"elementKind":    "title",
		"stableSelector": `[data-omega-source="src/Page.tsx:headline"]`,
		"textSnapshot":   "Old headline",
		"styleSnapshot":  map[string]any{"fontSize": "32px"},
		"domContext":     map[string]any{"tagName": "h1"},
		"sourceMapping":  map[string]any{"source": "src/Page.tsx:headline", "file": "src/Page.tsx", "symbol": "headline"},
	}
	var applied map[string]any
	decode(t, postJSON(t, api.URL+"/page-pilot/apply", map[string]any{
		"projectId":          "project_omega",
		"repositoryTargetId": "repo_ZYOOO_TestRepo",
		"instruction":        `replace text with "New headline"`,
		"selection":          selection,
		"runner":             "profile",
	}), &applied)
	if applied["status"] != "applied" || applied["repositoryPath"] != targetRepo {
		t.Fatalf("applied = %+v", applied)
	}
	raw, err := os.ReadFile(sourceFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "New headline") {
		t.Fatalf("isolated workspace source not patched: %s", raw)
	}
}

func newTestAPI(t *testing.T) (*httptest.Server, *SQLiteRepository) {
	t.Helper()
	root := t.TempDir()
	openAPI := filepath.Join(root, "openapi.yaml")
	if err := os.WriteFile(openAPI, []byte("openapi: 3.1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := NewServer(filepath.Join(root, "omega.db"), filepath.Join(root, "workspace"), openAPI)
	api := httptest.NewServer(server.Handler())
	t.Cleanup(api.Close)
	return api, server.Repo
}

func seedWorkspace(t *testing.T, repo *SQLiteRepository) {
	t.Helper()
	database := WorkspaceDatabase{
		SchemaVersion: 1,
		SavedAt:       nowISO(),
		Tables: WorkspaceTables{
			Projects:             []map[string]any{{"id": "project_omega", "name": "Omega", "description": "Omega", "team": "Omega", "status": "Active", "labels": []any{}, "createdAt": nowISO(), "updatedAt": nowISO()}},
			MissionControlStates: []map[string]any{{"runId": "run_req_omega_001", "projectId": "project_omega", "workItems": []any{}, "events": []any{}, "syncIntents": []any{}, "updatedAt": nowISO()}},
			Connections:          []map[string]any{},
			UIPreferences:        []map[string]any{},
		},
	}
	if err := repo.Save(context.Background(), database); err != nil {
		t.Fatal(err)
	}
}

func createDemoGitRepo(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "target-repo")
	if err := os.MkdirAll(filepath.Join(repo, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "src", "existing.ts"), []byte("export const existing = true;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "init")
	runGit(t, repo, "checkout", "-b", "main")
	runGit(t, repo, "config", "user.email", "omega-test@example.local")
	runGit(t, repo, "config", "user.name", "Omega Test")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "initial")
	return repo
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(output))
	}
	return string(output)
}

func requestJSON(t *testing.T, method string, url string, body any) *http.Response {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	request, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("content-type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func postJSON(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	return requestJSON(t, http.MethodPost, url, body)
}

func putJSON(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	return requestJSON(t, http.MethodPut, url, body)
}

func mustGet(t *testing.T, url string) *http.Response {
	t.Helper()
	response, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func decode(t *testing.T, response *http.Response, target any) {
	t.Helper()
	defer response.Body.Close()
	if response.StatusCode >= 400 {
		var body map[string]any
		_ = json.NewDecoder(response.Body).Decode(&body)
		t.Fatalf("status %d: %+v", response.StatusCode, body)
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatal(err)
	}
}

func containsSuffix(values []string, suffix string) bool {
	for _, value := range values {
		if strings.HasSuffix(value, suffix) {
			return true
		}
	}
	return false
}

func containsEventType(events []map[string]any, eventType string) bool {
	for _, event := range events {
		if text(event, "type") == eventType {
			return true
		}
	}
	return false
}

func hasProcessStream(events []map[string]any, stream string) bool {
	for _, event := range events {
		if text(event, "stream") == stream {
			return true
		}
	}
	return false
}
