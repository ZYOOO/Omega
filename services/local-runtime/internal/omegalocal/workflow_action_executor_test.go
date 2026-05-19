package omegalocal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

func TestWorkflowActionRouteDoesNotAutoAdvanceFailedActions(t *testing.T) {
	workflow := map[string]any{"states": []any{
		map[string]any{
			"id":          "in_progress",
			"transitions": map[string]any{"failed": "rework"},
		},
	}}

	route := workflowActionRoute(workflow, nil, "in_progress", "git_recovery", "failed")
	if route.StageStatus != "failed" || route.Event != "failed" || route.NextStageID != "" {
		t.Fatalf("failed action should block without queuing the next stage: %+v", route)
	}
}

func TestDevFlowGitHubRecoveryScopeIsLimited(t *testing.T) {
	if !devFlowGitHubRecoveryAllowed("in_progress", "publish_pull_request", "ensure_pr", devFlowGitRecoveryAgentID) {
		t.Fatal("publish pull request should allow git recovery")
	}
	if !devFlowGitHubRecoveryAllowed("rework", "update_pull_request", "ensure_pr", devFlowGitRecoveryAgentID) {
		t.Fatal("rework PR update should allow git recovery")
	}
	for _, testCase := range []struct {
		stage  string
		action string
		agent  string
	}{
		{stage: "todo", action: "capture_requirement", agent: devFlowGitRecoveryAgentID},
		{stage: "in_progress", action: "implement_change", agent: devFlowGitRecoveryAgentID},
		{stage: "in_progress", action: "publish_pull_request", agent: "coding"},
		{stage: "merging", action: "merge_pull_request", agent: devFlowGitRecoveryAgentID},
	} {
		if devFlowGitHubRecoveryAllowed(testCase.stage, testCase.action, "ensure_pr", testCase.agent) {
			t.Fatalf("git recovery should not be allowed for %+v", testCase)
		}
	}
}

func TestDevFlowGitHubRecoveryAgentRepairsPRDelivery(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := runCommand(repo, "git", "init"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCommand(repo, "git", "config", "user.email", "omega-test@example.local"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCommand(repo, "git", "config", "user.name", "Omega Test"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# Demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runCommand(repo, "git", "add", "README.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCommand(repo, "git", "commit", "-m", "init"); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "pr-ready")
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ghPath := filepath.Join(binDir, "gh")
	ghScript := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "pr" ] && [ "$2" = "list" ]; then
  if [ -f %q ]; then
    printf 'https://github.com/acme/demo/pull/7\n'
  fi
  exit 0
fi
if [ "$1" = "pr" ] && [ "$2" = "create" ]; then
  if [ -f %q ]; then
    printf 'https://github.com/acme/demo/pull/7\n'
    exit 0
  fi
  printf 'GraphQL: The omega/demo branch has no history in common with main\n' >&2
  exit 1
fi
exit 0
`, marker, marker)
	if err := os.WriteFile(ghPath, []byte(ghScript), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	server := NewServer(filepath.Join(root, "omega.db"), filepath.Join(root, "workspace"), filepath.Join(root, "openapi.yaml"))
	profile := defaultAgentProfile("project_omega", "repo_demo")
	template := findPipelineTemplate("devflow-pr")
	runner := fakeGitRecoveryRunner{run: func(request AgentTurnRequest) AgentTurnResult {
		if request.StageID != "in_progress" || request.Role != devFlowGitRecoveryAgentID || request.Sandbox != devFlowGitRecoverySandbox {
			t.Fatalf("unexpected request = %+v", request)
		}
		if !strings.Contains(request.Prompt, "in_progress/publish_pull_request") || !strings.Contains(request.Prompt, "Keep the branch name") {
			t.Fatalf("prompt missing recovery policy:\n%s", request.Prompt)
		}
		if err := os.WriteFile(marker, []byte("ready"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(request.OutputPath, []byte("Status: recovered\nPull request: https://github.com/acme/demo/pull/7\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return AgentTurnResult{Status: "passed", Process: map[string]any{"runner": "fake", "status": "passed"}}
	}}

	result, err := runDevFlowGitHubRecoveryAgent(devFlowGitHubRecoveryInput{
		Server:              server,
		Context:             context.Background(),
		Template:            template,
		Profile:             profile,
		Agent:               agentProfileForRole(profile, devFlowGitRecoveryAgentID),
		Runner:              runner,
		RunnerID:            "fake",
		Pipeline:            map[string]any{"id": "pipeline_demo"},
		Item:                map[string]any{"id": "item_demo", "key": "OMG-1", "title": "Demo"},
		RepositoryWorkspace: repo,
		Repository:          "acme/demo",
		BranchName:          "omega/demo",
		BaseBranch:          "main",
		PullRequestTitle:    "OMG-1 Demo",
		PullRequestBody:     "body",
		ProofDir:            filepath.Join(root, "proof"),
		AttemptID:           "attempt_demo",
		StageID:             "in_progress",
		ActionID:            "publish_pull_request",
		ActionType:          "ensure_pr",
		InitialError:        fmt.Errorf("create pull request: no history in common"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.PullRequestURL != "https://github.com/acme/demo/pull/7" || text(result.Process, "verifiedPullRequestUrl") != result.PullRequestURL {
		t.Fatalf("recovery result = %+v", result)
	}
}

type fakeGitRecoveryRunner struct {
	run func(AgentTurnRequest) AgentTurnResult
}

func (runner fakeGitRecoveryRunner) RunTurn(_ context.Context, request AgentTurnRequest) AgentTurnResult {
	return runner.run(request)
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
				{ID: "collect_rework_ci_results", Type: "run_ci_checks", Agent: "testing"},
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
		{ID: "collect_rework_ci_results", Type: "run_ci_checks", Agent: "testing", Run: func() error { order = append(order, "ci"); return nil }},
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
	if strings.Join(order, ",") != "checklist,apply,validate,pr,ci,refresh,merge,handoff" {
		t.Fatalf("contract action order = %v", order)
	}
}

func TestDefaultDevFlowWorkflowIncludesGitHubActionsCIAction(t *testing.T) {
	template := findPipelineTemplate("devflow-pr")
	if template == nil {
		t.Fatal("devflow-pr template missing")
	}
	actions := workflowActionMapsFromTemplate(template, "in_progress")
	if len(actions) == 0 {
		t.Fatalf("in_progress actions missing")
	}
	found := false
	for _, action := range actions {
		if text(action, "id") == "architecture_handoff" {
			outputs := strings.Join(stringSlice(action["outputArtifacts"]), ",")
			if !strings.Contains(outputs, "functional-todo-list") || !strings.Contains(outputs, "project-todo-list") {
				t.Fatalf("architecture handoff should produce todo list artifacts: %+v", action)
			}
		}
		if text(action, "id") == "collect_ci_results" && text(action, "type") == "run_ci_checks" && workflowActionHandlerName(text(action, "type")) == "devflow.github_actions.run_ci_checks" {
			found = true
		}
	}
	if !found {
		t.Fatalf("default workflow should collect GitHub Actions CI before review: %+v", actions)
	}
	reworkActions := workflowActionMapsFromTemplate(template, "rework")
	found = false
	for _, action := range reworkActions {
		if text(action, "id") == "collect_rework_ci_results" && text(action, "type") == "run_ci_checks" {
			found = true
		}
	}
	if !found {
		t.Fatalf("rework workflow should re-collect GitHub Actions CI: %+v", reworkActions)
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
