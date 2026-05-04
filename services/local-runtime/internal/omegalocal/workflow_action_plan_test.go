package omegalocal

import (
	"context"
	"testing"
)

func TestBuildAttemptActionPlanUsesWorkflowSnapshot(t *testing.T) {
	database := defaultWorkspaceDatabase()
	item := map[string]any{
		"id": "item_action_plan", "key": "OMG-action", "title": "Action plan", "status": "Running",
		"repositoryTargetId": "repo_ZYOOO_TestRepo",
	}
	pipeline := makePipelineWithTemplate(item, findPipelineTemplate("devflow-pr"))
	pipeline["id"] = "pipeline_action_plan"
	pipeline = markDevFlowStageProgress(pipeline, "todo", "passed", "Requirement captured.")
	pipeline = markDevFlowStageProgress(pipeline, "in_progress", "running", "Implementation started.")
	pipeline["status"] = "running"
	attempt := makeAttemptRecord(item, pipeline, "manual", "devflow-pr", "in_progress")
	attempt["id"] = "attempt_action_plan"
	attempt["status"] = "running"
	database.Tables.Pipelines = []map[string]any{pipeline}
	database.Tables.Attempts = []map[string]any{attempt}

	plan, status := buildAttemptActionPlan(database, "attempt_action_plan")
	if status != 200 {
		t.Fatalf("status = %d plan=%+v", status, plan)
	}
	if plan["executionMode"] != "contract-action-executor" || text(mapValue(plan["currentState"]), "id") != "in_progress" {
		t.Fatalf("plan identity = %+v", plan)
	}
	currentAction := mapValue(plan["currentAction"])
	if text(currentAction, "id") != "classify_task" || text(currentAction, "status") != "running" {
		t.Fatalf("current action = %+v", currentAction)
	}
	if len(arrayMaps(plan["actions"])) != 5 || intValue(plan["contractActionCount"]) < 10 {
		t.Fatalf("actions not from workflow snapshot = %+v", plan)
	}
	if !workflowActionPlanHasTransition(plan, "passed", "code_review_round_1") {
		t.Fatalf("plan missing in_progress passed transition = %+v", plan["transitions"])
	}
}

func TestAttemptActionPlanAPIIncludesRetryPolicy(t *testing.T) {
	api, repo := newTestAPI(t)
	database := defaultWorkspaceDatabase()
	item := map[string]any{
		"id": "item_action_retry", "key": "OMG-action-retry", "title": "Retry action plan", "status": "Blocked",
		"repositoryTargetId": "repo_ZYOOO_TestRepo",
	}
	pipeline := makePipelineWithTemplate(item, findPipelineTemplate("devflow-pr"))
	pipeline["id"] = "pipeline_action_retry"
	pipeline = markDevFlowStageProgress(pipeline, "in_progress", "failed", "GitHub API unavailable.")
	pipeline["status"] = "failed"
	attempt := makeAttemptRecord(item, pipeline, "manual", "devflow-pr", "in_progress")
	attempt["id"] = "attempt_action_retry"
	attempt["status"] = "failed"
	attempt["errorMessage"] = "gh api rate limit exceeded"
	database.Tables.Pipelines = []map[string]any{pipeline}
	database.Tables.Attempts = []map[string]any{attempt}
	if err := repo.Save(context.Background(), database); err != nil {
		t.Fatal(err)
	}

	var plan map[string]any
	decode(t, mustGet(t, api.URL+"/attempts/attempt_action_retry/action-plan"), &plan)
	retry := mapValue(plan["retry"])
	policy := mapValue(retry["policy"])
	if retry["available"] != true || text(policy, "class") != "github_api_transient" || text(policy, "action") != "wait-and-retry" {
		t.Fatalf("retry policy = %+v plan=%+v", retry, plan)
	}
	currentAction := mapValue(plan["currentAction"])
	if text(currentAction, "status") != "blocked" {
		t.Fatalf("failed attempt current action = %+v", currentAction)
	}
}

func TestNormalizeDevFlowPipelineStageStatusesKeepsSingleActiveStage(t *testing.T) {
	database := defaultWorkspaceDatabase()
	item := map[string]any{
		"id": "item_stage_canonical", "key": "OMG-canonical", "title": "Canonical stage", "status": "Human Review",
		"repositoryTargetId": "repo_ZYOOO_TestRepo",
	}
	pipeline := makePipelineWithTemplate(item, findPipelineTemplate("devflow-pr"))
	pipeline["id"] = "pipeline_stage_canonical"
	run := mapValue(pipeline["run"])
	for _, stage := range arrayMaps(run["stages"]) {
		switch text(stage, "id") {
		case "todo":
			stage["status"] = "passed"
		case "in_progress":
			stage["status"] = "running"
		case "code_review_round_2":
			stage["status"] = "passed"
		case "human_review":
			stage["status"] = "needs-human"
		case "done":
			stage["status"] = "needs-human"
		}
	}
	pipeline["run"] = run
	pipeline["status"] = "waiting-human"
	attempt := makeAttemptRecord(item, pipeline, "manual", "devflow-pr", "human_review")
	attempt["id"] = "attempt_stage_canonical"
	attempt["status"] = "waiting-human"
	checkpoint := map[string]any{
		"id": "pipeline_stage_canonical:human_review", "pipelineId": "pipeline_stage_canonical", "attemptId": "attempt_stage_canonical",
		"stageId": "human_review", "status": "pending",
	}
	database.Tables.Pipelines = []map[string]any{pipeline}
	database.Tables.Attempts = []map[string]any{attempt}
	database.Tables.Checkpoints = []map[string]any{checkpoint}

	normalized := normalizePipelineExecutionMetadata(database)
	stages := arrayMaps(mapValue(normalized.Tables.Pipelines[0]["run"])["stages"])
	statusByID := map[string]string{}
	active := []string{}
	for _, stage := range stages {
		status := text(stage, "status")
		statusByID[text(stage, "id")] = status
		if devFlowStageStatusIsActive(status) {
			active = append(active, text(stage, "id"))
		}
	}
	if len(active) != 1 || active[0] != "human_review" {
		t.Fatalf("active stages = %v statuses=%+v", active, statusByID)
	}
	if statusByID["in_progress"] != "passed" || statusByID["code_review_round_1"] != "passed" || statusByID["done"] != "waiting" {
		t.Fatalf("canonical statuses = %+v", statusByID)
	}
	if statusByID["rework"] != "waiting" {
		t.Fatalf("optional rework should stay waiting, statuses=%+v", statusByID)
	}
	if text(normalized.Tables.Pipelines[0], "status") != "waiting-human" {
		t.Fatalf("pipeline status = %s", text(normalized.Tables.Pipelines[0], "status"))
	}
	attemptStages := arrayMaps(normalized.Tables.Attempts[0]["stages"])
	attemptActive := []string{}
	attemptStatusByID := map[string]string{}
	for _, stage := range attemptStages {
		status := text(stage, "status")
		attemptStatusByID[text(stage, "id")] = status
		if devFlowStageStatusIsActive(status) {
			attemptActive = append(attemptActive, text(stage, "id"))
		}
	}
	if len(attemptActive) != 1 || attemptActive[0] != "human_review" {
		t.Fatalf("attempt active stages = %v statuses=%+v", attemptActive, attemptStatusByID)
	}
}

func TestMarkDevFlowStageProgressCanonicalizesCurrentStage(t *testing.T) {
	item := map[string]any{"id": "item_stage_writer", "key": "OMG-writer", "title": "Writer stage"}
	pipeline := makePipelineWithTemplate(item, findPipelineTemplate("devflow-pr"))
	run := mapValue(pipeline["run"])
	for _, stage := range arrayMaps(run["stages"]) {
		switch text(stage, "id") {
		case "todo":
			stage["status"] = "passed"
		case "in_progress":
			stage["status"] = "running"
		case "done":
			stage["status"] = "needs-human"
		}
	}
	pipeline["run"] = run

	pipeline = markDevFlowStageProgress(pipeline, "human_review", "needs-human", "Waiting for approval.")
	stages := arrayMaps(mapValue(pipeline["run"])["stages"])
	statusByID := map[string]string{}
	active := []string{}
	for _, stage := range stages {
		status := text(stage, "status")
		statusByID[text(stage, "id")] = status
		if devFlowStageStatusCanBeCurrent(status) {
			active = append(active, text(stage, "id"))
		}
	}
	if len(active) != 1 || active[0] != "human_review" {
		t.Fatalf("active stages = %v statuses=%+v", active, statusByID)
	}
	if statusByID["in_progress"] != "passed" || statusByID["code_review_round_1"] != "passed" || statusByID["done"] != "waiting" {
		t.Fatalf("writer canonical statuses = %+v", statusByID)
	}
}

func workflowActionPlanHasTransition(plan map[string]any, event string, target string) bool {
	for _, transition := range arrayMaps(plan["transitions"]) {
		if text(transition, "on") == event && text(transition, "to") == target {
			return true
		}
	}
	return false
}
