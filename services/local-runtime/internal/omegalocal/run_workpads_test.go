package omegalocal

import (
	"context"
	"testing"
)

func TestRunWorkpadRecordTracksAttemptRetryContext(t *testing.T) {
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)

	item := map[string]any{
		"id": "item_workpad_1", "projectId": "project_omega", "repositoryTargetId": "repo_test", "key": "OMG-1",
		"title": "Workpad item", "description": "Implement the requested change.", "status": "In Review",
		"priority": "High", "assignee": "coding", "labels": []any{"manual"}, "team": "Omega", "stageId": "coding", "target": "Repo",
		"acceptanceCriteria": []any{"The user can verify the change."},
	}
	pipeline := makePipelineWithTemplate(item, findPipelineTemplate("devflow-pr"))
	attempt := makeAttemptRecord(item, pipeline, "manual", "devflow-pr", "code_review_round_1")
	attempt["failureReason"] = "Review agent blocked delivery."
	attempt["failureReviewFeedback"] = "Fix missing loading state before merge."
	attempt["status"] = "failed"

	database, err := repo.Load(context.Background())
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	database.Tables.WorkItems = append(database.Tables.WorkItems, item)
	database.Tables.Pipelines = append(database.Tables.Pipelines, pipeline)
	database.Tables.Attempts = append(database.Tables.Attempts, attempt)
	upsertRunWorkpad(database, text(attempt, "id"))
	if err := repo.Save(context.Background(), *database); err != nil {
		t.Fatalf("save workspace: %v", err)
	}

	var workpads []map[string]any
	decode(t, mustGet(t, api.URL+"/run-workpads?attemptId="+text(attempt, "id")), &workpads)
	if len(workpads) != 1 {
		t.Fatalf("workpad count = %d", len(workpads))
	}
	workpad := mapValue(workpads[0]["workpad"])
	if got := text(workpads[0], "status"); got != "failed" {
		t.Fatalf("workpad status = %q", got)
	}
	if got := text(workpad, "retryReason"); got != "Review agent blocked delivery." {
		t.Fatalf("retry reason = %q", got)
	}
	if feedback := arrayValues(workpad["reviewFeedback"]); len(feedback) == 0 || feedback[0] != "Fix missing loading state before merge." {
		t.Fatalf("review feedback = %+v", feedback)
	}
}
