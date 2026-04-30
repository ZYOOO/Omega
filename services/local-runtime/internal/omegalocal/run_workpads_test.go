package omegalocal

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
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
	attempt["checkLogFeedback"] = []any{map[string]any{"kind": "ci-check-log", "label": "lint", "message": "Lint failed on the loading state copy."}}
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
	if feedback := strings.Join(stringSlice(workpad["reviewFeedback"]), "\n"); !strings.Contains(feedback, "Lint failed") {
		t.Fatalf("review feedback missing check log = %s", feedback)
	}
	reworkChecklist := mapValue(workpad["reworkChecklist"])
	if text(reworkChecklist, "status") != "needs-rework" {
		t.Fatalf("rework checklist status = %+v", reworkChecklist)
	}
	if checklist := stringSlice(reworkChecklist["checklist"]); len(checklist) == 0 || !strings.Contains(strings.Join(checklist, "\n"), "Fix missing loading state") {
		t.Fatalf("rework checklist = %+v", reworkChecklist)
	}
}

func TestPatchRunWorkpadPersistsFieldPatchesAcrossRefresh(t *testing.T) {
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)

	item := map[string]any{
		"id": "item_workpad_patch", "projectId": "project_omega", "repositoryTargetId": "repo_test", "key": "OMG-2",
		"title": "Patchable workpad", "description": "Keep supervisor notes.", "status": "In Review",
		"priority": "High", "assignee": "coding", "labels": []any{"manual"}, "team": "Omega", "stageId": "coding", "target": "Repo",
	}
	pipeline := makePipelineWithTemplate(item, findPipelineTemplate("devflow-pr"))
	attempt := makeAttemptRecord(item, pipeline, "manual", "devflow-pr", "human_review")
	attempt["status"] = "waiting-human"

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

	recordID := text(attempt, "id") + ":workpad"
	var patched map[string]any
	decode(t, requestJSON(t, http.MethodPatch, api.URL+"/run-workpads/"+recordID, map[string]any{
		"updatedBy": "job-supervisor",
		"reason":    "CI gate changed",
		"source": map[string]any{
			"kind":      "ci-check",
			"id":        "check_lint",
			"label":     "lint",
			"attemptId": text(attempt, "id"),
		},
		"workpad": map[string]any{
			"validation":     map[string]any{"status": "needs-attention", "summary": "Waiting for required checks."},
			"reviewFeedback": []any{"Supervisor captured a check gate."},
			"blockers":       []any{"Required check is still pending."},
		},
	}), &patched)
	workpad := mapValue(patched["workpad"])
	if text(mapValue(workpad["validation"]), "summary") != "Waiting for required checks." || text(workpad, "updatedBy") != "job-supervisor" {
		t.Fatalf("patched workpad = %+v", workpad)
	}
	if history := arrayMaps(patched["fieldPatchHistory"]); len(history) != 1 || text(history[0], "updatedBy") != "job-supervisor" {
		t.Fatalf("patch history = %+v", history)
	}
	sources := mapValue(patched["fieldPatchSources"])
	if source := mapValue(sources["validation"]); text(source, "kind") != "ci-check" || text(source, "reason") != "CI gate changed" {
		t.Fatalf("field patch source = %+v", source)
	}

	database, err = repo.Load(context.Background())
	if err != nil {
		t.Fatalf("reload workspace: %v", err)
	}
	upsertRunWorkpad(database, text(attempt, "id"))
	if err := repo.Save(context.Background(), *database); err != nil {
		t.Fatalf("save refreshed workspace: %v", err)
	}
	var workpads []map[string]any
	decode(t, mustGet(t, api.URL+"/run-workpads?attemptId="+text(attempt, "id")), &workpads)
	refreshed := mapValue(workpads[0]["workpad"])
	if text(mapValue(refreshed["validation"]), "summary") != "Waiting for required checks." {
		t.Fatalf("field patch was not reapplied after refresh: %+v", refreshed)
	}
	if feedback := strings.Join(stringSlice(refreshed["reviewFeedback"]), "\n"); !strings.Contains(feedback, "Supervisor captured") {
		t.Fatalf("review feedback patch missing after refresh: %s", feedback)
	}
}

func TestPatchRunWorkpadRejectsDisallowedActorFields(t *testing.T) {
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)

	item := map[string]any{
		"id": "item_workpad_patch_permissions", "projectId": "project_omega", "repositoryTargetId": "repo_test", "key": "OMG-3",
		"title": "Patchable workpad permissions", "description": "Keep operator patches bounded.", "status": "In Review",
		"priority": "High", "assignee": "coding", "labels": []any{"manual"}, "team": "Omega", "stageId": "coding", "target": "Repo",
	}
	pipeline := makePipelineWithTemplate(item, findPipelineTemplate("devflow-pr"))
	attempt := makeAttemptRecord(item, pipeline, "manual", "devflow-pr", "human_review")
	attempt["status"] = "waiting-human"

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

	response := requestJSON(t, http.MethodPatch, api.URL+"/run-workpads/"+text(attempt, "id")+":workpad", map[string]any{
		"updatedBy": "operator",
		"workpad": map[string]any{
			"plan": map[string]any{"summary": "Operator should not rewrite the plan."},
		},
	})
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", response.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if !strings.Contains(strings.Join(stringSlice(body["fields"]), ","), "plan") {
		t.Fatalf("unexpected invalid field response = %+v", body)
	}
}
