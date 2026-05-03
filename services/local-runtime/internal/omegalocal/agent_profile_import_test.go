package omegalocal

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportAgentProfileTemplateFromPathPersistsProfile(t *testing.T) {
	api, repo := newTestAPI(t)
	seedWorkspace(t, repo)
	templateDir := t.TempDir()
	writeImportFixture(t, templateDir)

	var imported agentProfileImportResponse
	decode(t, requestJSON(t, http.MethodPost, api.URL+"/agent-profile/import-template", map[string]any{
		"projectId": "project_omega",
		"source":    "path",
		"basePath":  templateDir,
	}), &imported)

	if imported.Profile.WorkflowTemplate != "custom-devflow" {
		t.Fatalf("workflow template = %s", imported.Profile.WorkflowTemplate)
	}
	if !strings.Contains(imported.Profile.WorkflowMarkdown, "## Prompt: coding") {
		t.Fatalf("coding prompt was not merged into workflow markdown:\n%s", imported.Profile.WorkflowMarkdown)
	}
	if !strings.Contains(imported.Profile.StagePolicy, "Coding must stay inside the repository workspace") {
		t.Fatalf("stage policy was not imported: %s", imported.Profile.StagePolicy)
	}
	if imported.Profile.AgentProfiles[0].StageNotes == "" {
		t.Fatalf("agent prompt summaries were not applied: %+v", imported.Profile.AgentProfiles[0])
	}
	stored, err := repo.GetAgentProfile(context.Background(), "project_omega", "")
	if err != nil {
		t.Fatal(err)
	}
	if text(stored, "workflowTemplate") != "custom-devflow" {
		t.Fatalf("stored profile = %+v", stored)
	}
	if files, ok := imported.Summary["files"].([]any); !ok || len(files) != 3 {
		t.Fatalf("summary files = %+v", imported.Summary["files"])
	}
}

func TestImportAgentProfileTemplateFromRepositoryOmega(t *testing.T) {
	api, repo := newTestAPI(t)
	repositoryPath := t.TempDir()
	templateDir := filepath.Join(repositoryPath, ".omega")
	if err := os.MkdirAll(templateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeImportFixture(t, templateDir)
	database := WorkspaceDatabase{
		SchemaVersion: 1,
		SavedAt:       nowISO(),
		Tables: WorkspaceTables{
			Projects: []map[string]any{{
				"id":                        "project_omega",
				"name":                      "Omega",
				"status":                    "Active",
				"defaultRepositoryTargetId": "repo_local",
				"repositoryTargets": []any{map[string]any{
					"id":        "repo_local",
					"projectId": "project_omega",
					"kind":      "local",
					"path":      repositoryPath,
					"label":     "Local Repo",
				}},
			}},
		},
	}
	if err := repo.Save(context.Background(), database); err != nil {
		t.Fatal(err)
	}

	var imported agentProfileImportResponse
	decode(t, requestJSON(t, http.MethodPost, api.URL+"/agent-profile/import-template", map[string]any{
		"projectId":          "project_omega",
		"repositoryTargetId": "repo_local",
		"source":             "repository",
	}), &imported)

	if imported.Profile.Source != "repository" || imported.Profile.RepositoryTargetID != "repo_local" {
		t.Fatalf("imported profile scope = %+v", imported.Profile)
	}
	if !strings.Contains(imported.Profile.WorkflowMarkdown, "custom-devflow") {
		t.Fatalf("repository workflow was not imported: %s", imported.Profile.WorkflowMarkdown)
	}
}

func writeImportFixture(t *testing.T, dir string) {
	t.Helper()
	files := map[string]string{
		"workflow.md": `workflow: custom-devflow
stages:
  - requirement: requirement
  - implementation: architect + coding + testing
  - review: review`,
		"stage-policy.md": "Coding: Coding must stay inside the repository workspace.\nReview: Return an explicit verdict.",
		"prompts.md": `# Prompt template

## Requirement

### 角色
Clarify requirement scope and acceptance criteria.

### 必须遵守
Only write the requirement artifact.

## Coding

### 角色
Apply the requested change in the locked workspace.

### 必须遵守
Keep the diff focused and reviewable.`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
