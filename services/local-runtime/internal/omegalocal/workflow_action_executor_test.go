package omegalocal

import (
	"strings"
	"testing"
)

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

func TestDevFlowReviewRoundsComeFromContractActions(t *testing.T) {
	template := &PipelineTemplate{
		ID: "devflow-pr",
		StateProfiles: []WorkflowStateProfile{
			{
				ID:    "code_review_round_1",
				Title: "Security Review",
				Actions: []WorkflowActionProfile{{
					ID:         "security_review",
					Type:       "run_review",
					Agent:      "review",
					DiffSource: "pr_diff",
					Verdicts: map[string]string{
						"approved":          "human_review",
						"changes_requested": "targeted_rework",
						"needs_human_info":  "human_review",
					},
				}},
			},
		},
		ReviewRounds: []ReviewRoundProfile{{
			StageID:    "code_review_round_1",
			Artifact:   "security-review.md",
			Focus:      "security and release risk",
			DiffSource: "local_diff",
		}},
	}

	rounds := devFlowReviewRoundsFromContract(template)
	if len(rounds) != 1 {
		t.Fatalf("rounds = %+v", rounds)
	}
	round := rounds[0]
	if round.StageID != "code_review_round_1" || round.Artifact != "security-review.md" || round.Focus != "security and release risk" {
		t.Fatalf("round metadata should preserve the contract/legacy display fields: %+v", round)
	}
	if round.DiffSource != "pr_diff" || round.ChangesRequestedTo != "targeted_rework" || round.NeedsHumanInfoTo != "human_review" {
		t.Fatalf("round execution fields should come from action verdicts: %+v", round)
	}
}

func TestWorkflowContractRejectsUnsupportedActionType(t *testing.T) {
	template := PipelineTemplate{
		ID: "custom",
		StageProfiles: []StageProfile{
			{ID: "todo", Title: "Todo", Agent: "requirement"},
		},
		StateProfiles: []WorkflowStateProfile{{
			ID: "todo",
			Actions: []WorkflowActionProfile{{
				ID:   "invented_action",
				Type: "unknown_runtime_action",
			}},
		}},
	}
	validation := validateWorkflowTemplate(template)
	if validation.ok() {
		t.Fatalf("unsupported action type should fail validation: %+v", validation)
	}
	if !strings.Contains(strings.Join(validation.Errors, "\n"), "unsupported action type") {
		t.Fatalf("validation errors should mention unsupported action type: %+v", validation.Errors)
	}
}

func TestRunDevFlowContractStateUsesContractOrder(t *testing.T) {
	template := &PipelineTemplate{StateProfiles: []WorkflowStateProfile{{
		ID: "in_progress",
		Actions: []WorkflowActionProfile{
			{ID: "validate_repository", Type: "run_validation", Agent: "testing"},
			{ID: "architecture_handoff", Type: "run_agent", Agent: "architect"},
		},
	}}}
	order := []string{}
	err := runDevFlowContractState(template, "in_progress", []devFlowContractActionStep{
		{ID: "architecture_handoff", Type: "run_agent", Agent: "architect", Run: func() error {
			order = append(order, "architect")
			return nil
		}},
		{ID: "validate_repository", Type: "run_validation", Agent: "testing", Run: func() error {
			order = append(order, "testing")
			return nil
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(order, ",") != "testing,architect" {
		t.Fatalf("contract action order not honored: %v", order)
	}
}

func TestRunDevFlowContractStateUsesReworkAndMergingActions(t *testing.T) {
	template := &PipelineTemplate{StateProfiles: []WorkflowStateProfile{
		{
			ID: "rework",
			Actions: []WorkflowActionProfile{
				{ID: "build_rework_checklist", Type: "build_rework_checklist", Agent: "master"},
				{ID: "apply_rework", Type: "run_agent", Agent: "coding"},
				{ID: "validate_rework", Type: "run_validation", Agent: "testing"},
				{ID: "update_pull_request", Type: "ensure_pr", Agent: "delivery"},
			},
		},
		{
			ID: "merging",
			Actions: []WorkflowActionProfile{
				{ID: "refresh_pr_status", Type: "refresh_pr_status", Agent: "delivery"},
				{ID: "merge_pull_request", Type: "merge_pr", Agent: "delivery"},
			},
		},
		{
			ID: "done",
			Actions: []WorkflowActionProfile{
				{ID: "finalize_handoff", Type: "write_handoff", Agent: "delivery"},
			},
		},
	}}
	order := []string{}
	if err := runDevFlowContractState(template, "rework", []devFlowContractActionStep{
		{ID: "apply_rework", Type: "run_agent", Agent: "coding", Run: func() error { order = append(order, "apply"); return nil }},
		{ID: "build_rework_checklist", Type: "build_rework_checklist", Agent: "master", Run: func() error { order = append(order, "checklist"); return nil }},
		{ID: "update_pull_request", Type: "ensure_pr", Agent: "delivery", Run: func() error { order = append(order, "pr"); return nil }},
		{ID: "validate_rework", Type: "run_validation", Agent: "testing", Run: func() error { order = append(order, "validate"); return nil }},
	}); err != nil {
		t.Fatal(err)
	}
	if err := runDevFlowContractState(template, "merging", []devFlowContractActionStep{
		{ID: "merge_pull_request", Type: "merge_pr", Agent: "delivery", Run: func() error { order = append(order, "merge"); return nil }},
		{ID: "refresh_pr_status", Type: "refresh_pr_status", Agent: "delivery", Run: func() error { order = append(order, "refresh"); return nil }},
	}); err != nil {
		t.Fatal(err)
	}
	if err := runDevFlowContractState(template, "done", []devFlowContractActionStep{
		{ID: "finalize_handoff", Type: "write_handoff", Agent: "delivery", Run: func() error { order = append(order, "handoff"); return nil }},
	}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(order, ",") != "checklist,apply,validate,pr,refresh,merge,handoff" {
		t.Fatalf("contract action order = %v", order)
	}
}

func TestRunDevFlowContractStateRequiresHandler(t *testing.T) {
	template := &PipelineTemplate{StateProfiles: []WorkflowStateProfile{{
		ID: "in_progress",
		Actions: []WorkflowActionProfile{{
			ID: "publish_pull_request", Type: "ensure_pr", Agent: "delivery",
		}},
	}}}
	err := runDevFlowContractState(template, "in_progress", []devFlowContractActionStep{
		{ID: "validate_repository", Type: "run_validation", Agent: "testing", Run: func() error { return nil }},
	})
	if err == nil || !strings.Contains(err.Error(), "no DevFlow runtime handler") {
		t.Fatalf("missing handler error = %v", err)
	}
}
