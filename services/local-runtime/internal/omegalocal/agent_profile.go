package omegalocal

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const agentProfileSettingPrefix = "agent-profile:"

type AgentProfileConfig struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Runner       string `json:"runner"`
	Model        string `json:"model"`
	Skills       string `json:"skills"`
	MCP          string `json:"mcp"`
	StageNotes   string `json:"stageNotes"`
	CodexPolicy  string `json:"codexPolicy"`
	ClaudePolicy string `json:"claudePolicy"`
}

type ProjectAgentProfile struct {
	ProjectID          string               `json:"projectId"`
	RepositoryTargetID string               `json:"repositoryTargetId,omitempty"`
	WorkflowTemplate   string               `json:"workflowTemplate"`
	WorkflowMarkdown   string               `json:"workflowMarkdown"`
	StagePolicy        string               `json:"stagePolicy"`
	SkillAllowlist     string               `json:"skillAllowlist"`
	MCPAllowlist       string               `json:"mcpAllowlist"`
	CodexPolicy        string               `json:"codexPolicy"`
	ClaudePolicy       string               `json:"claudePolicy"`
	AgentProfiles      []AgentProfileConfig `json:"agentProfiles"`
	Source             string               `json:"source,omitempty"`
	UpdatedAt          string               `json:"updatedAt,omitempty"`
}

func defaultAgentProfile(projectID string, repositoryTargetID string) ProjectAgentProfile {
	return normalizeAgentProfile(ProjectAgentProfile{
		ProjectID:          stringOr(projectID, "project_omega"),
		RepositoryTargetID: repositoryTargetID,
		WorkflowTemplate:   "devflow-pr",
		WorkflowMarkdown: `workflow: devflow-pr
stages:
  - requirement: requirement
  - implementation: architect + coding + testing
  - code_review: review
  - rework: coding + testing, then code_review
  - human_review: human gate
  - delivery: delivery`,
		StagePolicy: strings.Join([]string{
			"Requirement: clarify acceptance criteria, repository target, open questions, and acceptance risks before planning.",
			"Architecture: list affected files, integration boundaries, risky assumptions, and validation strategy before coding.",
			"Coding: edit only inside the bound repository workspace and keep the diff reviewable for a single Work Item.",
			"Testing: run focused validation first, then broader checks when shared contracts, delivery, or UI behavior changed.",
			"Review: changes_requested must route to Rework with a checklist; review feedback should not be treated as an infrastructure failure.",
			"Rework: reuse the existing implementation workspace, apply the checklist, update PR notes when the behavior changed, and return to review.",
			"Human Review: stop delivery until explicit approval; request changes becomes first-class feedback for the next rework attempt.",
			"Delivery: after approval, run merge/check actions separately and record PR/check/proof output in the Run Workpad.",
		}, "\n"),
		SkillAllowlist: "browser-use\ngithub:github\ngithub:gh-fix-ci\ngithub:yeet",
		MCPAllowlist:   "github\nfilesystem:repository-workspace\nbrowser:localhost-preview",
		CodexPolicy:    "sandbox: workspace-write\napproval: never inside automated stage\nrepo-scope: require repositoryTargetId match",
		ClaudePolicy:   "workspace: repository target only\nhandoff: keep Omega artifact names stable",
		AgentProfiles: []AgentProfileConfig{
			{ID: "requirement", Label: "Requirement", Runner: "codex", Model: "gpt-5.4-mini", Skills: "github:github\nbrowser-use", MCP: "github\nfilesystem:repository-workspace", StageNotes: "Clarify acceptance criteria and repository boundary.", CodexPolicy: "write requirement artifact only", ClaudePolicy: "focus on ambiguity and acceptance criteria"},
			{ID: "architect", Label: "Architect", Runner: "codex", Model: "gpt-5.4-mini", Skills: "github:github", MCP: "filesystem:repository-workspace", StageNotes: "Map affected files and risk.", CodexPolicy: "prefer read-only analysis", ClaudePolicy: "produce file-level impact notes"},
			{ID: "coding", Label: "Coding", Runner: "codex", Model: "gpt-5.4-mini", Skills: "github:gh-fix-ci\nbrowser-use", MCP: "filesystem:repository-workspace\nbrowser:localhost-preview", StageNotes: "Edit only inside the locked repository workspace.", CodexPolicy: "workspace-write only; emit diff and summary", ClaudePolicy: "preserve existing project style"},
			{ID: "testing", Label: "Testing", Runner: "codex", Model: "gpt-5.4-mini", Skills: "browser-use", MCP: "filesystem:repository-workspace\nbrowser:localhost-preview", StageNotes: "Run focused tests and capture output.", CodexPolicy: "capture test-report.md", ClaudePolicy: "summarize validation evidence"},
			{ID: "review", Label: "Review", Runner: "codex", Model: "gpt-5.4-mini", Skills: "github:github\ngithub:gh-fix-ci", MCP: "github\nfilesystem:repository-workspace", StageNotes: "Review correctness, safety, tests, and contract drift.", CodexPolicy: "do not edit files; issue explicit verdict", ClaudePolicy: "return verdict and required fixes"},
			{ID: "delivery", Label: "Delivery", Runner: "codex", Model: "gpt-5.4-mini", Skills: "github:yeet\ngithub:github", MCP: "github\nfilesystem:repository-workspace", StageNotes: "Prepare handoff after human approval.", CodexPolicy: "require human gate approval before delivery action", ClaudePolicy: "summarize shipped changes and caveats"},
		},
		Source: "default",
	})
}

func normalizeAgentProfile(profile ProjectAgentProfile) ProjectAgentProfile {
	if profile.ProjectID == "" {
		profile.ProjectID = "project_omega"
	}
	if profile.WorkflowTemplate == "" {
		profile.WorkflowTemplate = "devflow-pr"
	}
	if profile.WorkflowMarkdown == "" {
		profile.WorkflowMarkdown = defaultAgentProfile(profile.ProjectID, profile.RepositoryTargetID).WorkflowMarkdown
	}
	if len(profile.AgentProfiles) == 0 {
		profile.AgentProfiles = defaultAgentProfile(profile.ProjectID, profile.RepositoryTargetID).AgentProfiles
	}
	for index := range profile.AgentProfiles {
		if profile.AgentProfiles[index].Runner == "" {
			profile.AgentProfiles[index].Runner = "codex"
		}
		if profile.AgentProfiles[index].Model == "" {
			profile.AgentProfiles[index].Model = "gpt-5.4-mini"
		}
		if profile.AgentProfiles[index].Label == "" {
			profile.AgentProfiles[index].Label = profile.AgentProfiles[index].ID
		}
	}
	return profile
}

func agentProfileSettingKey(projectID string, repositoryTargetID string) string {
	if repositoryTargetID != "" {
		return agentProfileSettingPrefix + "repository:" + repositoryTargetID
	}
	return agentProfileSettingPrefix + "project:" + stringOr(projectID, "project_omega")
}

func agentProfileRecordID(projectID string, repositoryTargetID string) string {
	if repositoryTargetID != "" {
		return "agent_profile_repository_" + safeSegment(repositoryTargetID)
	}
	return "agent_profile_project_" + safeSegment(stringOr(projectID, "project_omega"))
}

func profileToMap(profile ProjectAgentProfile) map[string]any {
	raw, _ := json.Marshal(profile)
	var value map[string]any
	_ = json.Unmarshal(raw, &value)
	return value
}

func profileFromMap(value map[string]any) ProjectAgentProfile {
	raw, _ := json.Marshal(value)
	var profile ProjectAgentProfile
	_ = json.Unmarshal(raw, &profile)
	return normalizeAgentProfile(profile)
}

func firstProjectIDFromDatabase(database WorkspaceDatabase) string {
	if len(database.Tables.Projects) == 0 {
		return "project_omega"
	}
	return stringOr(text(database.Tables.Projects[0], "id"), "project_omega")
}

func (server *Server) resolveAgentProfile(ctx context.Context, database WorkspaceDatabase, item map[string]any, target map[string]any) ProjectAgentProfile {
	projectID := stringOr(text(item, "projectId"), firstProjectIDFromDatabase(database))
	repositoryTargetID := text(target, "id")
	if repositoryTargetID == "" {
		repositoryTargetID = text(item, "repositoryTargetId")
	}
	applyWorkflowOverride := func(profile ProjectAgentProfile) ProjectAgentProfile {
		templateID := stringOr(profile.WorkflowTemplate, "devflow-pr")
		if record := workflowTemplateOverride(database, projectID, repositoryTargetID, templateID); record != nil && strings.TrimSpace(text(record, "markdown")) != "" {
			profile.WorkflowTemplate = stringOr(text(record, "templateId"), templateID)
			profile.WorkflowMarkdown = text(record, "markdown")
		}
		return normalizeAgentProfile(profile)
	}
	if repositoryTargetID != "" {
		if record, err := server.Repo.GetAgentProfile(ctx, projectID, repositoryTargetID); err == nil {
			profile := profileFromMap(record)
			profile.Source = "repository"
			return applyWorkflowOverride(profile)
		} else if !errorsIsNoRows(err) {
			// Keep running with older settings-backed profiles if the first-class table is not readable.
		}
		if record, err := server.Repo.GetSetting(ctx, agentProfileSettingKey(projectID, repositoryTargetID)); err == nil {
			profile := profileFromMap(record)
			profile.Source = "repository"
			_ = server.Repo.SetAgentProfile(ctx, profile)
			return applyWorkflowOverride(profile)
		}
	}
	if record, err := server.Repo.GetAgentProfile(ctx, projectID, ""); err == nil {
		profile := profileFromMap(record)
		profile.Source = "project"
		return applyWorkflowOverride(profile)
	}
	if record, err := server.Repo.GetSetting(ctx, agentProfileSettingKey(projectID, "")); err == nil {
		profile := profileFromMap(record)
		profile.Source = "project"
		_ = server.Repo.SetAgentProfile(ctx, profile)
		return applyWorkflowOverride(profile)
	}
	return applyWorkflowOverride(defaultAgentProfile(projectID, repositoryTargetID))
}

func (server *Server) resolveAgentProfileForMission(ctx context.Context, mission map[string]any) ProjectAgentProfile {
	database, err := server.Repo.Load(ctx)
	if err != nil {
		return defaultAgentProfile("project_omega", text(mission, "repositoryTargetId"))
	}
	item := findWorkItem(*database, text(mission, "sourceWorkItemId"))
	if item == nil {
		item = map[string]any{"projectId": firstProjectIDFromDatabase(*database), "repositoryTargetId": text(mission, "repositoryTargetId")}
	}
	target := findRepositoryTarget(*database, text(mission, "repositoryTargetId"))
	return server.resolveAgentProfile(ctx, *database, item, target)
}

func agentProfileForRole(profile ProjectAgentProfile, agentID string) AgentProfileConfig {
	for _, agent := range profile.AgentProfiles {
		if agent.ID == agentID {
			return agent
		}
	}
	for _, agent := range profile.AgentProfiles {
		if agent.ID == "coding" {
			return agent
		}
	}
	return AgentProfileConfig{ID: agentID, Label: agentID, Runner: "codex", Model: "gpt-5.4-mini"}
}

func attachAgentProfileToPipeline(pipeline map[string]any, profile ProjectAgentProfile) map[string]any {
	next := cloneMap(pipeline)
	run := mapValue(next["run"])
	run["agentProfile"] = map[string]any{
		"source":             profile.Source,
		"projectId":          profile.ProjectID,
		"repositoryTargetId": profile.RepositoryTargetID,
		"workflowTemplate":   profile.WorkflowTemplate,
		"workflowMarkdown":   profile.WorkflowMarkdown,
		"agentCount":         len(profile.AgentProfiles),
	}
	next["run"] = run
	return next
}

func agentPolicyBlock(profile ProjectAgentProfile, agentID string) string {
	agent := agentProfileForRole(profile, agentID)
	parts := []string{
		"Omega Agent Profile:",
		"- workflow: " + profile.WorkflowTemplate,
		"- agent: " + stringOr(agent.Label, agent.ID),
		"- runner: " + stringOr(agent.Runner, "codex"),
		"- model: " + stringOr(agent.Model, "gpt-5.4-mini"),
	}
	if strings.TrimSpace(agent.StageNotes) != "" {
		parts = append(parts, "- stage notes: "+strings.TrimSpace(agent.StageNotes))
	}
	if strings.TrimSpace(agent.Skills) != "" {
		parts = append(parts, "- skills: "+strings.ReplaceAll(strings.TrimSpace(agent.Skills), "\n", ", "))
	}
	if strings.TrimSpace(agent.MCP) != "" {
		parts = append(parts, "- mcp: "+strings.ReplaceAll(strings.TrimSpace(agent.MCP), "\n", ", "))
	}
	if strings.TrimSpace(agent.CodexPolicy) != "" {
		parts = append(parts, "\n.codex policy:\n"+strings.TrimSpace(agent.CodexPolicy))
	}
	return strings.Join(parts, "\n")
}

func agentRuntimeMetadata(profile ProjectAgentProfile, agentID string) map[string]any {
	agent := agentProfileForRole(profile, agentID)
	return map[string]any{
		"source":             profile.Source,
		"projectId":          profile.ProjectID,
		"repositoryTargetId": profile.RepositoryTargetID,
		"workflowTemplate":   profile.WorkflowTemplate,
		"workflowMarkdown":   profile.WorkflowMarkdown,
		"stagePolicy":        profile.StagePolicy,
		"agent": map[string]any{
			"id":           agent.ID,
			"label":        agent.Label,
			"runner":       agent.Runner,
			"model":        agent.Model,
			"skills":       compactLines(agent.Skills),
			"mcp":          compactLines(agent.MCP),
			"stageNotes":   agent.StageNotes,
			"codexPolicy":  agent.CodexPolicy,
			"claudePolicy": agent.ClaudePolicy,
		},
		"runtimeFiles": []string{".omega/agent-runtime.json", ".codex/OMEGA.md", ".claude/CLAUDE.md"},
	}
}

func writeRunnerPolicyFiles(root string, profile ProjectAgentProfile, agentID string) error {
	agent := agentProfileForRole(profile, agentID)
	if err := os.MkdirAll(filepath.Join(root, ".codex"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
		return err
	}
	codex := fmt.Sprintf("# Omega Agent Policy\n\nAgent: %s\nWorkflow: %s\nRunner: %s\nModel: %s\n\n%s\n", stringOr(agent.Label, agent.ID), profile.WorkflowTemplate, stringOr(agent.Runner, "codex"), stringOr(agent.Model, "gpt-5.4-mini"), stringOr(agent.CodexPolicy, profile.CodexPolicy))
	claude := fmt.Sprintf("# Omega Agent Policy\n\nAgent: %s\nWorkflow: %s\nRunner: %s\nModel: %s\n\n%s\n", stringOr(agent.Label, agent.ID), profile.WorkflowTemplate, stringOr(agent.Runner, "codex"), stringOr(agent.Model, "gpt-5.4-mini"), stringOr(agent.ClaudePolicy, profile.ClaudePolicy))
	if err := os.WriteFile(filepath.Join(root, ".codex", "OMEGA.md"), []byte(codex), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, ".claude", "CLAUDE.md"), []byte(claude), 0o644)
}

func (server *Server) getAgentProfile(response http.ResponseWriter, request *http.Request) {
	projectID := request.URL.Query().Get("projectId")
	repositoryTargetID := request.URL.Query().Get("repositoryTargetId")
	database, err := server.Repo.Load(request.Context())
	if err == nil && projectID == "" {
		projectID = firstProjectIDFromDatabase(*database)
	}
	if repositoryTargetID != "" {
		if record, err := server.Repo.GetAgentProfile(request.Context(), projectID, repositoryTargetID); err == nil {
			profile := profileFromMap(record)
			profile.Source = "repository"
			writeJSON(response, http.StatusOK, profile)
			return
		} else if err != nil && !errorsIsNoRows(err) {
			writeError(response, http.StatusInternalServerError, err)
			return
		}
		if record, err := server.Repo.GetSetting(request.Context(), agentProfileSettingKey(projectID, repositoryTargetID)); err == nil {
			profile := profileFromMap(record)
			profile.Source = "repository"
			_ = server.Repo.SetAgentProfile(request.Context(), profile)
			writeJSON(response, http.StatusOK, profile)
			return
		}
	}
	if record, err := server.Repo.GetAgentProfile(request.Context(), projectID, ""); err == nil {
		profile := profileFromMap(record)
		profile.Source = "project"
		writeJSON(response, http.StatusOK, profile)
		return
	} else if err != nil && !errorsIsNoRows(err) {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	if record, err := server.Repo.GetSetting(request.Context(), agentProfileSettingKey(projectID, "")); err == nil {
		profile := profileFromMap(record)
		profile.Source = "project"
		_ = server.Repo.SetAgentProfile(request.Context(), profile)
		writeJSON(response, http.StatusOK, profile)
		return
	} else if err != nil && !errorsIsNoRows(err) {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, defaultAgentProfile(projectID, repositoryTargetID))
}

func (server *Server) putAgentProfile(response http.ResponseWriter, request *http.Request) {
	var profile ProjectAgentProfile
	if err := json.NewDecoder(request.Body).Decode(&profile); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	if profile.ProjectID == "" {
		if database, err := server.Repo.Load(request.Context()); err == nil {
			profile.ProjectID = firstProjectIDFromDatabase(*database)
		}
	}
	profile = normalizeAgentProfile(profile)
	profile.UpdatedAt = nowISO()
	profile.Source = map[bool]string{true: "repository", false: "project"}[profile.RepositoryTargetID != ""]
	if err := server.Repo.SetAgentProfile(request.Context(), profile); err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	// Compatibility mirror for older app builds and local debugging tools that still inspect omega_settings.
	_ = server.Repo.SetSetting(request.Context(), agentProfileSettingKey(profile.ProjectID, profile.RepositoryTargetID), profileToMap(profile))
	writeJSON(response, http.StatusOK, profile)
}

func errorsIsNoRows(err error) bool {
	return err == sql.ErrNoRows || strings.Contains(err.Error(), "no rows")
}
