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
	if plan["executionMode"] != "contract-action-plan" || text(mapValue(plan["currentState"]), "id") != "in_progress" {
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

func workflowActionPlanHasTransition(plan map[string]any, event string, target string) bool {
	for _, transition := range arrayMaps(plan["transitions"]) {
		if text(transition, "on") == event && text(transition, "to") == target {
			return true
		}
	}
	return false
}
