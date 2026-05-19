package omegalocal

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
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

type AgentSkillBundle struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Source        string `json:"source"`
	SourcePath    string `json:"sourcePath,omitempty"`
	WorkspacePath string `json:"workspacePath"`
	Installed     bool   `json:"installed"`
	Bundled       bool   `json:"bundled"`
}

func defaultAgentProfileConfigs() []AgentProfileConfig {
	return []AgentProfileConfig{
		{ID: "master", Label: "Master", Runner: "codex", Model: "", Skills: "bb-browser\nopenai-docs", MCP: "omega-filesystem\nomega-memory\nomega-sequential-thinking", StageNotes: "Classify the requirement, choose workflow route, and dispatch stage agents with repository boundaries.", CodexPolicy: "orchestrate only; do not edit repository source", ClaudePolicy: "focus on dispatch, ownership, and stage handoff"},
		{ID: "requirement", Label: "Requirement", Runner: "codex", Model: "", Skills: "bb-browser\nopenai-docs", MCP: "omega-filesystem\nomega-memory\nomega-sequential-thinking", StageNotes: "Clarify acceptance criteria and repository boundary.", CodexPolicy: "write requirement artifact only", ClaudePolicy: "focus on ambiguity and acceptance criteria"},
		{ID: "architect", Label: "Architect", Runner: "codex", Model: "", Skills: "security-threat-model\nopenai-docs", MCP: "omega-filesystem\nomega-sequential-thinking", StageNotes: "Map affected files and risk.", CodexPolicy: "prefer read-only analysis", ClaudePolicy: "produce file-level impact notes"},
		{ID: "coding", Label: "Coding", Runner: "codex", Model: "", Skills: "playwright\nsecurity-best-practices", MCP: "omega-filesystem\nomega-git\nomega-puppeteer", StageNotes: "Edit only inside the locked repository workspace.", CodexPolicy: "workspace-write only; emit diff and summary", ClaudePolicy: "preserve existing project style"},
		{ID: "testing", Label: "Testing", Runner: "codex", Model: "", Skills: "playwright\ngh-fix-ci", MCP: "omega-filesystem\nomega-puppeteer\nomega-git", StageNotes: "Run focused tests and capture output.", CodexPolicy: "capture test-report.md", ClaudePolicy: "summarize validation evidence"},
		{ID: "review", Label: "Review", Runner: "codex", Model: "", Skills: "gh-address-comments\ngh-fix-ci\nsecurity-best-practices", MCP: "omega-git\nomega-filesystem", StageNotes: "Review correctness, safety, tests, and contract drift.", CodexPolicy: "do not edit files; issue explicit verdict", ClaudePolicy: "return verdict and required fixes"},
		{ID: "git_recovery", Label: "Git Recovery", Runner: "codex", Model: "", Skills: "gh-address-comments\ngh-fix-ci", MCP: "omega-git\nomega-filesystem\nomega-sequential-thinking", StageNotes: "Diagnose and repair Git/GitHub delivery failures only when a PR publish/update action invokes recovery.", CodexPolicy: "danger-full-access is used only for git metadata and gh repair inside the locked repository workspace; keep the same Omega branch; no product edits except conflict resolution; never merge", ClaudePolicy: "repair branch/PR delivery inside the locked repository workspace only"},
		{ID: "delivery", Label: "Delivery", Runner: "codex", Model: "", Skills: "yeet\ngh-address-comments", MCP: "omega-git\nomega-filesystem", StageNotes: "Prepare handoff after human approval.", CodexPolicy: "require human gate approval before delivery action", ClaudePolicy: "summarize shipped changes and caveats"},
	}
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
			"Git Recovery: only the git_recovery Agent may repair Git/GitHub delivery failures, and only for in_progress/publish_pull_request or rework/update_pull_request.",
			"Human Review: stop delivery until explicit approval; request changes becomes first-class feedback for the next rework attempt.",
			"Delivery: after approval, run merge/check actions separately and record PR/check/proof output in the Run Workpad.",
		}, "\n"),
		SkillAllowlist: "bb-browser\nplaywright\ngh-address-comments\ngh-fix-ci\nyeet\nsecurity-best-practices\nsecurity-threat-model\nopenai-docs",
		MCPAllowlist:   "omega-filesystem\nomega-git\nomega-puppeteer\nomega-memory\nomega-sequential-thinking\nx-mcp",
		CodexPolicy:    "sandbox: workspace-write\napproval: never inside automated stage\nrepo-scope: require repositoryTargetId match",
		ClaudePolicy:   "workspace: repository target only\nhandoff: keep Omega artifact names stable",
		AgentProfiles:  defaultAgentProfileConfigs(),
		Source:         "default",
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
		if template, ok := loadWorkflowPipelineTemplate(profile.WorkflowTemplate); ok {
			profile.WorkflowMarkdown = template.WorkflowMarkdown
		} else {
			profile.WorkflowMarkdown = defaultAgentProfile(profile.ProjectID, profile.RepositoryTargetID).WorkflowMarkdown
		}
	} else if template, ok := loadWorkflowPipelineTemplate(profile.WorkflowTemplate); ok {
		parsedTemplate, err := parseWorkflowTemplateMarkdown(profile.WorkflowMarkdown, "agent-profile:"+profile.ProjectID+":"+profile.RepositoryTargetID)
		if err == nil && ((parsedTemplate.ID == "" && profile.WorkflowTemplate != "devflow-pr") || (parsedTemplate.ID != "" && parsedTemplate.ID != profile.WorkflowTemplate)) {
			profile.WorkflowMarkdown = template.WorkflowMarkdown
		}
	}
	if len(profile.AgentProfiles) == 0 {
		profile.AgentProfiles = defaultAgentProfileConfigs()
	} else {
		seen := map[string]bool{}
		for _, agent := range profile.AgentProfiles {
			seen[agent.ID] = true
		}
		for _, defaultAgent := range defaultAgentProfileConfigs() {
			if !seen[defaultAgent.ID] {
				profile.AgentProfiles = append(profile.AgentProfiles, defaultAgent)
			}
		}
	}
	for index := range profile.AgentProfiles {
		if profile.AgentProfiles[index].Runner == "" {
			profile.AgentProfiles[index].Runner = "codex"
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
	database, err := server.Repo.LoadWorkspaceSession(ctx)
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

func agentSkillCatalog() map[string]string {
	return map[string]string{
		"bb-browser":              "Browser inspection and authenticated browsing for requirement discovery, UI debugging, and private/local page checks.",
		"playwright":              "Browser automation, screenshots, interaction checks, and UI regression verification.",
		"gh-address-comments":     "Read and address GitHub pull request review comments and unresolved review feedback.",
		"gh-fix-ci":               "Inspect GitHub Actions failures, summarize failing logs, and guide targeted CI fixes.",
		"yeet":                    "Prepare delivery commits, pushes, and pull requests after human approval.",
		"security-best-practices": "Review implementation changes for common security pitfalls in supported languages and frameworks.",
		"security-threat-model":   "Produce architecture-stage trust boundary, asset, abuse path, and mitigation notes.",
		"openai-docs":             "Use current OpenAI product/API documentation when implementation depends on OpenAI behavior.",
	}
}

func knownSkillTitle(skillID string) string {
	title := strings.TrimSpace(strings.ReplaceAll(skillID, "-", " "))
	if title == "" {
		return "Omega skill"
	}
	return strings.ToUpper(title[:1]) + title[1:]
}

func fallbackSkillMarkdown(skillID string) string {
	description := agentSkillCatalog()[skillID]
	if description == "" {
		description = "Project-provided Omega skill guidance for this stage."
	}
	return fmt.Sprintf(`# %s

Use this project-bundled Omega skill when the stage profile lists %q.

Purpose:
- %s

Rules:
- Treat this file as the local skill instruction source when a host-level skill install is unavailable.
- Keep all reads and writes inside the locked Repository Workspace or runner workspace.
- Record evidence in Omega proof files when this skill changes implementation, validation, review, or delivery behavior.
- If a richer host-installed skill exists, prefer its detailed workflow, but keep this project copy as the auditable fallback.
`, knownSkillTitle(skillID), skillID, description)
}

func localSkillSearchRoots() []string {
	roots := []string{}
	if codexHome := strings.TrimSpace(os.Getenv("CODEX_HOME")); codexHome != "" {
		roots = append(roots,
			filepath.Join(codexHome, "skills"),
			filepath.Join(codexHome, "skills", ".system"),
		)
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		roots = append(roots,
			filepath.Join(home, ".codex", "skills"),
			filepath.Join(home, ".codex", "skills", ".system"),
			filepath.Join(home, ".codex", "superpowers", "skills"),
		)
	}
	return roots
}

func localSkillFile(skillID string) string {
	for _, root := range localSkillSearchRoots() {
		candidate := filepath.Join(root, skillID, "SKILL.md")
		if stat, err := os.Stat(candidate); err == nil && !stat.IsDir() {
			return candidate
		}
	}
	return ""
}

func writeSkillBundleFile(destination string, skillID string, sourcePath string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	if sourcePath != "" {
		source, err := os.Open(sourcePath)
		if err == nil {
			defer source.Close()
			target, createErr := os.Create(destination)
			if createErr != nil {
				return createErr
			}
			if _, copyErr := io.Copy(target, source); copyErr != nil {
				_ = target.Close()
				return copyErr
			}
			return target.Close()
		}
	}
	return os.WriteFile(destination, []byte(fallbackSkillMarkdown(skillID)), 0o644)
}

func materializeAgentSkillBundles(root string, profile ProjectAgentProfile, agentID string) ([]AgentSkillBundle, error) {
	agent := agentProfileForRole(profile, agentID)
	skills := compactLines(agent.Skills)
	bundles := make([]AgentSkillBundle, 0, len(skills))
	for _, skillID := range skills {
		sourcePath := localSkillFile(skillID)
		relativePath := filepath.Join(".omega", "skills", safeSegment(skillID), "SKILL.md")
		destination := filepath.Join(root, relativePath)
		if err := writeSkillBundleFile(destination, skillID, sourcePath); err != nil {
			return nil, err
		}
		source := "project-bundled-fallback"
		if sourcePath != "" {
			source = "host-install-copy"
		}
		bundles = append(bundles, AgentSkillBundle{
			ID:            skillID,
			Title:         knownSkillTitle(skillID),
			Source:        source,
			SourcePath:    sourcePath,
			WorkspacePath: filepath.ToSlash(relativePath),
			Installed:     sourcePath != "",
			Bundled:       true,
		})
	}
	sort.SliceStable(bundles, func(left, right int) bool {
		return bundles[left].ID < bundles[right].ID
	})
	return bundles, nil
}

func agentSkillBundlesForEnv(profile ProjectAgentProfile, agentID string) []AgentSkillBundle {
	agent := agentProfileForRole(profile, agentID)
	skills := compactLines(agent.Skills)
	bundles := make([]AgentSkillBundle, 0, len(skills))
	for _, skillID := range skills {
		relativePath := filepath.Join(".omega", "skills", safeSegment(skillID), "SKILL.md")
		sourcePath := localSkillFile(skillID)
		source := "project-bundled-fallback"
		if sourcePath != "" {
			source = "host-install-copy"
		}
		bundles = append(bundles, AgentSkillBundle{
			ID:            skillID,
			Title:         knownSkillTitle(skillID),
			Source:        source,
			SourcePath:    sourcePath,
			WorkspacePath: filepath.ToSlash(relativePath),
			Installed:     sourcePath != "",
			Bundled:       true,
		})
	}
	sort.SliceStable(bundles, func(left, right int) bool {
		return bundles[left].ID < bundles[right].ID
	})
	return bundles
}

func skillBundlePaths(bundles []AgentSkillBundle) []string {
	paths := make([]string, 0, len(bundles))
	for _, bundle := range bundles {
		if bundle.WorkspacePath != "" {
			paths = append(paths, bundle.WorkspacePath)
		}
	}
	return paths
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
	return AgentProfileConfig{ID: agentID, Label: agentID, Runner: "codex", Model: ""}
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
	skillBundles := agentSkillBundlesForEnv(profile, agentID)
	parts := []string{
		"Omega Agent Profile:",
		"- workflow: " + profile.WorkflowTemplate,
		"- agent: " + stringOr(agent.Label, agent.ID),
		"- runner: " + stringOr(agent.Runner, "codex"),
	}
	if strings.TrimSpace(agent.Model) != "" {
		parts = append(parts, "- model: "+strings.TrimSpace(agent.Model))
	}
	if strings.TrimSpace(agent.StageNotes) != "" {
		parts = append(parts, "- stage notes: "+strings.TrimSpace(agent.StageNotes))
	}
	if strings.TrimSpace(agent.Skills) != "" {
		parts = append(parts, "- skills: "+strings.ReplaceAll(strings.TrimSpace(agent.Skills), "\n", ", "))
	}
	if len(skillBundles) > 0 {
		paths := make([]string, 0, len(skillBundles))
		for _, bundle := range skillBundles {
			paths = append(paths, bundle.ID+"="+bundle.WorkspacePath)
		}
		parts = append(parts, "- project skill files: "+strings.Join(paths, ", "))
		parts = append(parts, "Before using a configured skill, read the matching project skill file from .omega/skills in this runner workspace.")
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
		"runtimeFiles": []string{".omega/agent-runtime.json", ".omega/agent-capabilities.json", ".omega/agent-capabilities.md", ".omega/agent-skill-manifest.json", ".omega/skills", ".codex/OMEGA.md", ".claude/CLAUDE.md"},
	}
}

func agentCapabilityManifest(profile ProjectAgentProfile, agentID string) map[string]any {
	agent := agentProfileForRole(profile, agentID)
	skillBundles := agentSkillBundlesForEnv(profile, agentID)
	return map[string]any{
		"agentId":             stringOr(agent.ID, agentID),
		"label":               stringOr(agent.Label, agentID),
		"runner":              stringOr(agent.Runner, "codex"),
		"model":               strings.TrimSpace(agent.Model),
		"skills":              compactLines(agent.Skills),
		"skillBundleRoot":     ".omega/skills",
		"skillBundles":        skillBundles,
		"mcp":                 compactLines(agent.MCP),
		"skillAllowlist":      compactLines(profile.SkillAllowlist),
		"mcpAllowlist":        compactLines(profile.MCPAllowlist),
		"stageNotes":          agent.StageNotes,
		"workflowTemplate":    profile.WorkflowTemplate,
		"profileSource":       profile.Source,
		"repositoryTargetId":  profile.RepositoryTargetID,
		"capabilityFileUsage": "Runner prompts and policy files must treat skills/mcp as the allowed capability surface for this stage.",
	}
}

func agentCapabilityEnv(profile ProjectAgentProfile, agentID string) map[string]string {
	agent := agentProfileForRole(profile, agentID)
	skillBundles := agentSkillBundlesForEnv(profile, agentID)
	return map[string]string{
		"OMEGA_AGENT_ID":              stringOr(agent.ID, agentID),
		"OMEGA_AGENT_LABEL":           stringOr(agent.Label, agentID),
		"OMEGA_AGENT_RUNNER":          stringOr(agent.Runner, "codex"),
		"OMEGA_AGENT_MODEL":           strings.TrimSpace(agent.Model),
		"OMEGA_AGENT_SKILLS":          strings.Join(compactLines(agent.Skills), ","),
		"OMEGA_AGENT_SKILL_ROOT":      ".omega/skills",
		"OMEGA_AGENT_SKILL_PATHS":     strings.Join(skillBundlePaths(skillBundles), ","),
		"OMEGA_AGENT_MCP":             strings.Join(compactLines(agent.MCP), ","),
		"OMEGA_AGENT_SKILL_ALLOWLIST": strings.Join(compactLines(profile.SkillAllowlist), ","),
		"OMEGA_AGENT_MCP_ALLOWLIST":   strings.Join(compactLines(profile.MCPAllowlist), ","),
		"OMEGA_WORKFLOW_TEMPLATE":     stringOr(profile.WorkflowTemplate, "devflow-pr"),
	}
}

func agentCapabilitiesMarkdown(profile ProjectAgentProfile, agentID string) string {
	agent := agentProfileForRole(profile, agentID)
	skillBundles := agentSkillBundlesForEnv(profile, agentID)
	section := func(title string, values []string) string {
		if len(values) == 0 {
			return "## " + title + "\n\n- none\n"
		}
		lines := []string{"## " + title, ""}
		for _, value := range values {
			lines = append(lines, "- "+value)
		}
		return strings.Join(lines, "\n") + "\n"
	}
	skillDirectorySection := func(bundles []AgentSkillBundle) string {
		if len(bundles) == 0 {
			return "## Project Skill Directory\n\n- none\n"
		}
		lines := []string{
			"## Project Skill Directory",
			"",
			"Before using a configured skill, read the project-bundled skill file in this runner workspace:",
		}
		for _, bundle := range bundles {
			source := bundle.Source
			if source == "" {
				source = "project-bundled"
			}
			lines = append(lines, fmt.Sprintf("- `%s`: `%s` (%s)", bundle.ID, bundle.WorkspacePath, source))
		}
		return strings.Join(lines, "\n") + "\n"
	}
	parts := []string{
		"# Omega Agent Capabilities",
		"",
		"Agent: " + stringOr(agent.Label, agent.ID),
		"Runner: " + stringOr(agent.Runner, "codex"),
		"Workflow: " + stringOr(profile.WorkflowTemplate, "devflow-pr"),
		"",
		section("Stage Skills", compactLines(agent.Skills)),
		section("Stage MCP", compactLines(agent.MCP)),
		section("Project Skill Allowlist", compactLines(profile.SkillAllowlist)),
		section("Project MCP Allowlist", compactLines(profile.MCPAllowlist)),
		skillDirectorySection(skillBundles),
	}
	if notes := strings.TrimSpace(agent.StageNotes); notes != "" {
		parts = append(parts, "## Stage Notes\n\n"+notes+"\n")
	}
	if model := strings.TrimSpace(agent.Model); model != "" {
		parts = append(parts[:4], append([]string{"Model: " + model}, parts[4:]...)...)
	}
	return strings.Join(parts, "\n")
}

func writeRunnerPolicyFiles(root string, profile ProjectAgentProfile, agentID string) error {
	agent := agentProfileForRole(profile, agentID)
	if err := os.MkdirAll(filepath.Join(root, ".omega"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, ".codex"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
		return err
	}
	skillBundles, err := materializeAgentSkillBundles(root, profile, agentID)
	if err != nil {
		return err
	}
	if err := writeJSONFile(filepath.Join(root, ".omega", "agent-skill-manifest.json"), skillBundles); err != nil {
		return err
	}
	if err := writeJSONFile(filepath.Join(root, ".omega", "agent-capabilities.json"), agentCapabilityManifest(profile, agentID)); err != nil {
		return err
	}
	capabilitiesMarkdown := agentCapabilitiesMarkdown(profile, agentID)
	if err := os.WriteFile(filepath.Join(root, ".omega", "agent-capabilities.md"), []byte(capabilitiesMarkdown), 0o644); err != nil {
		return err
	}
	modelLine := ""
	if model := strings.TrimSpace(agent.Model); model != "" {
		modelLine = "\nModel: " + model
	}
	codex := fmt.Sprintf("# Omega Agent Policy\n\nAgent: %s\nWorkflow: %s\nRunner: %s%s\n\n%s\n\n%s\n", stringOr(agent.Label, agent.ID), profile.WorkflowTemplate, stringOr(agent.Runner, "codex"), modelLine, capabilitiesMarkdown, stringOr(agent.CodexPolicy, profile.CodexPolicy))
	claude := fmt.Sprintf("# Omega Agent Policy\n\nAgent: %s\nWorkflow: %s\nRunner: %s%s\n\n%s\n\n%s\n", stringOr(agent.Label, agent.ID), profile.WorkflowTemplate, stringOr(agent.Runner, "codex"), modelLine, capabilitiesMarkdown, stringOr(agent.ClaudePolicy, profile.ClaudePolicy))
	if err := os.WriteFile(filepath.Join(root, ".codex", "OMEGA.md"), []byte(codex), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, ".claude", "CLAUDE.md"), []byte(claude), 0o644)
}

func (server *Server) getAgentProfile(response http.ResponseWriter, request *http.Request) {
	projectID := request.URL.Query().Get("projectId")
	repositoryTargetID := request.URL.Query().Get("repositoryTargetId")
	database, err := server.Repo.LoadWorkspaceSession(request.Context())
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
		if database, err := server.Repo.LoadWorkspaceSession(request.Context()); err == nil {
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
	if err == nil {
		return false
	}
	return err == sql.ErrNoRows || strings.Contains(err.Error(), "no rows")
}
