package omegalocal

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDevFlowRemoteGateFeedbackUsesFailedChecksAndLogs(t *testing.T) {
	summary := githubCheckSummaryWithRequired([]map[string]any{
		{"name": "unit", "state": "FAILURE", "link": "https://github.com/acme/repo/actions/runs/123"},
	}, []string{"unit", "integration"})
	feedback, needsRework := devFlowRemoteGateFeedback(summary, []map[string]any{{
		"kind":    "ci-check-log",
		"label":   "unit",
		"message": "npm test failed at user-card.test.tsx",
	}})
	if !needsRework {
		t.Fatalf("expected remote gate to request rework")
	}
	for _, want := range []string{"Failed checks: 1", "integration: missing", "npm test failed"} {
		if !strings.Contains(feedback, want) {
			t.Fatalf("feedback missing %q:\n%s", want, feedback)
		}
	}
}

func TestWorkflowTemplateFirstClassAPIValidateSaveRestore(t *testing.T) {
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)
	defaultTemplate := findPipelineTemplate("devflow-pr")
	if defaultTemplate == nil || strings.TrimSpace(defaultTemplate.WorkflowMarkdown) == "" {
		t.Fatal("default workflow markdown missing")
	}

	var validation map[string]any
	decode(t, postJSON(t, api.URL+"/workflow-templates/validate", map[string]any{
		"templateId": "devflow-pr",
		"markdown":   defaultTemplate.WorkflowMarkdown,
	}), &validation)
	if text(mapValue(validation["validation"]), "status") != "passed" {
		t.Fatalf("validation = %+v", validation)
	}

	var saved map[string]any
	decode(t, requestJSON(t, http.MethodPut, api.URL+"/workflow-templates/devflow-pr", map[string]any{
		"projectId":  "project_omega",
		"templateId": "devflow-pr",
		"markdown":   defaultTemplate.WorkflowMarkdown,
	}), &saved)
	if text(saved, "source") != "workspace" || intValue(saved["version"]) != 1 {
		t.Fatalf("saved workflow template = %+v", saved)
	}

	database, err := repo.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(database.Tables.WorkflowTemplates) != 1 {
		t.Fatalf("workflow template records = %+v", database.Tables.WorkflowTemplates)
	}

	var restored map[string]any
	decode(t, postJSON(t, api.URL+"/workflow-templates/devflow-pr/restore-default", map[string]any{
		"projectId":  "project_omega",
		"templateId": "devflow-pr",
	}), &restored)
	if text(restored, "source") != "restored-default" || intValue(restored["version"]) != 2 {
		t.Fatalf("restored workflow template = %+v", restored)
	}
}

func TestJobSupervisorPollsRemoteChecksAndRefreshesHeartbeat(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeGH := filepath.Join(bin, "gh")
	if err := os.WriteFile(fakeGH, []byte("#!/bin/sh\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"checks\" ]; then echo '[{\"name\":\"unit\",\"state\":\"SUCCESS\",\"link\":\"https://github.com/acme/repo/actions/runs/1\"}]'; exit 0; fi\necho unexpected >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	sqlitePath, err := exec.LookPath("sqlite3")
	if err != nil {
		t.Skip("sqlite3 is required for runtime log writes in this test")
	}
	if err := os.Symlink(sqlitePath, filepath.Join(bin, "sqlite3")); err != nil {
		t.Fatal(err)
	}
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+oldPath)

	repoDir := filepath.Join(root, "workspace", "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	server := NewServer(filepath.Join(root, "omega.db"), filepath.Join(root, "workspace-root"), filepath.Join(root, "openapi.yaml"))
	database := WorkspaceDatabase{
		SchemaVersion: 1,
		SavedAt:       nowISO(),
		Tables: WorkspaceTables{
			Projects: []map[string]any{{"id": "project_omega", "name": "Omega", "status": "Active", "createdAt": nowISO(), "updatedAt": nowISO()}},
			Pipelines: []map[string]any{{
				"id": "pipeline_1", "workItemId": "item_1", "templateId": "devflow-pr", "status": "running",
				"run": map[string]any{"workflow": map[string]any{"runtime": map[string]any{"requiredChecks": []any{"unit"}}}},
			}},
			Attempts: []map[string]any{{
				"id":             "attempt_1",
				"itemId":         "item_1",
				"pipelineId":     "pipeline_1",
				"status":         "running",
				"workspacePath":  filepath.Join(root, "workspace"),
				"pullRequestUrl": "https://github.com/acme/repo/pull/7",
				"lastSeenAt":     "2026-05-01T00:00:00Z",
				"createdAt":      nowISO(),
				"updatedAt":      nowISO(),
			}},
		},
	}

	summary := server.scanRemoteAttemptSignals(context.Background(), &database, jobSupervisorTickOptions{Limit: 5})
	if intValue(summary["polledPullRequests"]) != 1 || intValue(summary["changed"]) != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	signals := mapValue(database.Tables.Attempts[0]["remoteSignals"])
	checkSummary := mapValue(signals["checkSummary"])
	if intValue(checkSummary["passed"]) != 1 || text(database.Tables.Attempts[0], "lastSeenAt") == "2026-05-01T00:00:00Z" {
		t.Fatalf("attempt remote signals = %+v attempt=%+v", signals, database.Tables.Attempts[0])
	}
}
