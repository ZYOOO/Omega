package omegalocal

import "testing"

func TestWorkflowActionRouteUsesReviewVerdicts(t *testing.T) {
	workflow := map[string]any{"states": []any{
		map[string]any{
			"id": "code_review_round_1",
			"actions": []any{map[string]any{
				"id":    "review_round_1",
				"type":  "run_review",
				"agent": "review",
				"verdicts": map[string]any{
					"approved":          "release_review",
					"changes_requested": "targeted_rework",
				},
			}},
		},
	}}

	approved := workflowActionRoute(workflow, nil, "code_review_round_1", "review", "passed")
	if approved.StageStatus != "passed" || approved.Event != "approved" || approved.NextStageID != "release_review" {
		t.Fatalf("approved review route = %+v", approved)
	}
	if approved.ActionID != "review_round_1" || approved.ActionType != "run_review" || approved.Handler != "workflow-action-handler" {
		t.Fatalf("approved action metadata = %+v", approved)
	}

	changes := workflowActionRoute(workflow, nil, "code_review_round_1", "review", "changes-requested")
	if changes.StageStatus != "passed" || changes.Event != "changes_requested" || changes.NextStageID != "targeted_rework" {
		t.Fatalf("changes-requested review route = %+v", changes)
	}
}

func TestWorkflowActionRouteUsesReworkAndMergingStateTransitions(t *testing.T) {
	workflow := map[string]any{"states": []any{
		map[string]any{
			"id": "rework",
			"actions": []any{
				map[string]any{"id": "apply_rework", "type": "run_agent", "agent": "coding"},
				map[string]any{"id": "validate_rework", "type": "run_validation", "agent": "testing"},
			},
			"transitions": map[string]any{"passed": "code_review_round_2"},
		},
		map[string]any{
			"id": "merging",
			"actions": []any{map[string]any{
				"id":              "merge_pull_request",
				"type":            "merge_pr",
				"agent":           "delivery",
				"outputArtifacts": []any{"merge-proof"},
			}},
			"transitions": map[string]any{"passed": "ship_done", "failed": "rework"},
		},
	}}

	rework := workflowActionRoute(workflow, nil, "rework", "testing", "passed")
	if rework.StageStatus != "passed" || rework.Event != "passed" || rework.NextStageID != "code_review_round_2" {
		t.Fatalf("rework route = %+v", rework)
	}

	merge := workflowActionRoute(workflow, nil, "merging", "delivery", "passed")
	if merge.StageStatus != "passed" || merge.ActionID != "merge_pull_request" || merge.ActionType != "merge_pr" || merge.NextStageID != "ship_done" {
		t.Fatalf("merge route = %+v", merge)
	}
}

func TestWorkflowActionRouteCanReadTemplateActions(t *testing.T) {
	template := &PipelineTemplate{
		ID: "devflow-pr",
		StateProfiles: []WorkflowStateProfile{{
			ID:    "code_review_round_1",
			Title: "Review",
			Actions: []WorkflowActionProfile{{
				ID:    "review_round_1",
				Type:  "run_review",
				Agent: "review",
				Verdicts: map[string]string{
					"approved":          "human_review",
					"changes_requested": "rework",
				},
			}},
		}},
	}
	if got := devFlowTransitionTo(template, "code_review_round_1", "changes_requested", ""); got != "rework" {
		t.Fatalf("template action verdict route = %q", got)
	}
	route := workflowActionRoute(nil, template, "code_review_round_1", "review", "passed")
	if route.NextStageID != "human_review" || route.Event != "approved" {
		t.Fatalf("template action route = %+v", route)
	}
}
