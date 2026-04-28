package omegalocal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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
	implementation := template.StageProfiles[1]
	if implementation.ID != "in_progress" || strings.Join(implementation.AgentIDs, ",") != "architect,coding,testing" {
		t.Fatalf("implementation agents should come from workflow markdown: %+v", implementation)
	}
	if len(template.ReviewRounds) != 2 || template.ReviewRounds[0].Focus != "correctness, regressions, and acceptance criteria" || template.ReviewRounds[1].DiffSource != "pr_diff" {
		t.Fatalf("review rounds should come from workflow markdown: %+v", template.ReviewRounds)
	}
	if template.Runtime.MaxReviewCycles != 3 || len(template.Transitions) == 0 {
		t.Fatalf("workflow runtime/transitions should come from markdown: runtime=%+v transitions=%+v", template.Runtime, template.Transitions)
	}
	pipeline := makePipelineWithTemplate(map[string]any{
		"id": "item_manual_89", "key": "OMG-89", "title": "Ship feature", "description": "Build it", "assignee": "requirement", "requirementId": "req_item_manual_89",
	}, template)
	run := mapValue(pipeline["run"])
	workflow := mapValue(run["workflow"])
	if workflow["source"] == "" || len(arrayMaps(workflow["reviewRounds"])) != 2 {
		t.Fatalf("pipeline run should preserve workflow metadata: %+v", workflow)
	}
	stages := arrayMaps(run["stages"])
	if got := anySlice(stages[1]["agentIds"]); len(got) != 3 || got[0] != "architect" || got[2] != "testing" {
		t.Fatalf("pipeline stages did not preserve workflow agents: %+v", stages[1])
	}
	if got := anySlice(stages[1]["outputArtifacts"]); len(got) == 0 || got[len(got)-1] != "test-report" {
		t.Fatalf("pipeline stages did not preserve workflow artifacts: %+v", stages[1])
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
	if err := os.WriteFile(review, []byte("# Review\n\nVerdict: CHANGES_REQUESTED\n\nPlease fix the missing behavior."), 0o644); err != nil {
		t.Fatal(err)
	}
	outcome := devFlowReviewOutcome(review)
	if outcome.Verdict != "changes_requested" {
		t.Fatalf("outcome = %+v", outcome)
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
if [ "$1" = "pr" ] && [ "$2" = "create" ]; then
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
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var refreshed WorkspaceDatabase
		decode(t, mustGet(t, api.URL+"/workspace"), &refreshed)
		if len(refreshed.Tables.Attempts) == 1 && refreshed.Tables.Attempts[0]["status"] != "running" {
			break
		}
		time.Sleep(25 * time.Millisecond)
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
	proofs := arrayMaps(result["proofRecords"])
	if len(proofs) != 3 || proofs[0]["label"] != "pull-request" || proofs[1]["label"] != "check" {
		t.Fatalf("proof records = %+v", proofs)
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
	t.Setenv("PATH", bin)

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
	var applied map[string]any
	decode(t, postJSON(t, api.URL+"/page-pilot/apply", map[string]any{
		"projectId":          "project_omega",
		"repositoryTargetId": "repo_local_page",
		"instruction":        `replace text with "New headline"`,
		"selection":          selection,
		"runner":             "profile",
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
	raw, err = os.ReadFile(sourceFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "Old headline") {
		t.Fatalf("source not discarded: %s", raw)
	}

	decode(t, postJSON(t, api.URL+"/page-pilot/apply", map[string]any{
		"projectId":          "project_omega",
		"repositoryTargetId": "repo_local_page",
		"instruction":        `replace text with "New headline"`,
		"selection":          selection,
		"runner":             "profile",
	}), &applied)
	runID = text(applied, "id")

	var delivered map[string]any
	decode(t, postJSON(t, api.URL+"/page-pilot/deliver", map[string]any{
		"runId":              runID,
		"projectId":          "project_omega",
		"repositoryTargetId": "repo_local_page",
		"instruction":        `replace text with "New headline"`,
		"selection":          selection,
		"branchName":         "omega/page-pilot-test",
	}), &delivered)
	if delivered["status"] != "delivered" || delivered["branchName"] != "omega/page-pilot-test" || delivered["commitSha"] == "" {
		t.Fatalf("delivered = %+v", delivered)
	}
	if branch := strings.TrimSpace(runGit(t, targetRepo, "branch", "--show-current")); branch != "omega/page-pilot-test" {
		t.Fatalf("branch = %s", branch)
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
	t.Setenv("PATH", bin)

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
