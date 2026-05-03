package omegalocal

import (
	"strings"
	"testing"
)

func TestBuildReworkChecklistMergesReviewHumanAndDeliveryGateSignals(t *testing.T) {
	database := defaultWorkspaceDatabase()
	pipeline := map[string]any{
		"id": "pipeline_rework_checklist",
		"run": map[string]any{"events": []any{
			map[string]any{"type": "gate.rejected", "message": "Human Review rejected: 颜色需要更清楚"},
		}},
	}
	attempt := map[string]any{
		"id":                    "attempt_rework_checklist",
		"pipelineId":            "pipeline_rework_checklist",
		"status":                "failed",
		"humanChangeRequest":    "把按钮文案改短",
		"failureReason":         "Review agent blocked delivery.",
		"failureReviewFeedback": "Add a loading state before merge.",
		"recommendedActions": []any{
			map[string]any{"type": "checks-failed", "label": "Inspect failed CI checks and route back to rework", "count": 1},
			map[string]any{"type": "merge-conflict", "label": "Resolve merge conflicts before delivery", "count": 1},
		},
		"pullRequestFeedback": []any{
			map[string]any{"kind": "pr-review", "label": "CHANGES_REQUESTED by reviewer", "message": "PR review asked for clearer empty-state copy.", "url": "https://github.com/acme/demo/pull/1#review"},
			map[string]any{"kind": "pr-comment", "label": "designer", "message": "Comment says the loading message is hard to scan."},
			map[string]any{"kind": "pr-review-thread", "label": "unresolved thread src/App.tsx:44", "message": "Inline thread asks for shorter loading message.", "state": "unresolved", "path": "src/App.tsx", "line": "44", "sourceUrl": "https://github.com/acme/demo/pull/1#discussion_r1"},
			map[string]any{"kind": "pr-review-thread", "label": "duplicate unresolved thread src/App.tsx:44", "message": "Inline thread asks for shorter loading message.", "state": "unresolved", "path": "src/App.tsx", "line": "44", "sourceUrl": "https://github.com/acme/demo/pull/1#discussion_r2"},
			map[string]any{"kind": "pr-review-thread", "label": "resolved thread src/Done.tsx:8", "message": "Resolved copy note should not become an action.", "state": "resolved", "resolved": true, "path": "src/Done.tsx", "line": "8"},
		},
		"checkLogFeedback": []any{
			map[string]any{"kind": "ci-check-log", "label": "lint", "message": "Lint failed because the button label overflows on mobile.", "runId": "123", "url": "https://github.com/acme/demo/actions/runs/123", "sourceUrl": "https://github.com/acme/demo/actions/runs/123?check_suite_focus=true"},
		},
	}
	database.Tables.Operations = append(database.Tables.Operations, map[string]any{
		"id": "operation_review", "pipelineId": "pipeline_rework_checklist", "stageId": "code_review_round_1", "agentId": "review", "summary": "Review found missing validation proof.",
	})

	checklist := buildReworkChecklist(database, pipeline, attempt)
	if text(checklist, "status") != "needs-rework" {
		t.Fatalf("checklist status = %+v", checklist)
	}
	body := strings.Join(stringSlice(checklist["checklist"]), "\n")
	for _, expected := range []string{"把按钮文案改短", "Add a loading state", "失败的 CI/check", "merge conflict", "missing validation proof", "clearer empty-state copy", "loading message", "button label overflows", "src/App.tsx:44", "2 条相关信号"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("checklist missing %q in:\n%s", expected, body)
		}
	}
	if strings.Contains(body, "Resolved copy note") {
		t.Fatalf("resolved review thread should stay in sources but not become a checklist item:\n%s", body)
	}
	prompt := text(checklist, "prompt")
	if !strings.Contains(prompt, "Rework checklist:") || !strings.Contains(prompt, "Source feedback:") {
		t.Fatalf("prompt not actionable: %s", prompt)
	}
	groups := arrayMaps(checklist["groups"])
	foundGroupedThread := false
	for _, group := range groups {
		if strings.Contains(text(group, "item"), "src/App.tsx:44") && intValue(group["count"]) == 2 {
			foundGroupedThread = true
		}
	}
	if !foundGroupedThread {
		t.Fatalf("expected duplicate thread feedback to be grouped: %+v", groups)
	}
	sources := arrayMaps(checklist["sources"])
	if len(sources) == 0 || text(sources[len(sources)-1], "runId") != "123" || text(sources[len(sources)-1], "sourceUrl") == "" {
		t.Fatalf("source metadata was not preserved: %+v", sources)
	}
}
