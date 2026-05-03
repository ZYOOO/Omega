package omegalocal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type agentProfileImportRequest struct {
	ProjectID          string `json:"projectId"`
	RepositoryTargetID string `json:"repositoryTargetId"`
	Source             string `json:"source"`
	BasePath           string `json:"basePath"`
}

type agentProfileImportResponse struct {
	Profile ProjectAgentProfile `json:"profile"`
	Summary map[string]any      `json:"summary"`
}

func (server *Server) importAgentProfileTemplate(response http.ResponseWriter, request *http.Request) {
	var payload agentProfileImportRequest
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	projectID := strings.TrimSpace(payload.ProjectID)
	if projectID == "" {
		if database, err := server.Repo.Load(request.Context()); err == nil {
			projectID = firstProjectIDFromDatabase(*database)
		}
	}
	repositoryTargetID := strings.TrimSpace(payload.RepositoryTargetID)
	source := strings.TrimSpace(payload.Source)
	if source == "" {
		source = "fixtures"
	}
	basePath, err := server.agentProfileImportBasePath(request.Context(), source, projectID, repositoryTargetID, strings.TrimSpace(payload.BasePath))
	if err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	current := defaultAgentProfile(projectID, repositoryTargetID)
	if record, err := server.Repo.GetAgentProfile(request.Context(), projectID, repositoryTargetID); err == nil {
		current = profileFromMap(record)
	} else if repositoryTargetID == "" {
		if record, err := server.Repo.GetAgentProfile(request.Context(), projectID, ""); err == nil {
			current = profileFromMap(record)
		}
	}
	profile, summary, err := importAgentProfileFromDirectory(current, basePath, source)
	if err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	profile.ProjectID = stringOr(projectID, profile.ProjectID)
	profile.RepositoryTargetID = repositoryTargetID
	profile.UpdatedAt = nowISO()
	profile.Source = map[bool]string{true: "repository", false: "project"}[repositoryTargetID != ""]
	if err := server.Repo.SetAgentProfile(request.Context(), profile); err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	_ = server.Repo.SetSetting(request.Context(), agentProfileSettingKey(profile.ProjectID, profile.RepositoryTargetID), profileToMap(profile))
	writeJSON(response, http.StatusOK, agentProfileImportResponse{Profile: profile, Summary: summary})
}

func (server *Server) agentProfileImportBasePath(ctx context.Context, source string, projectID string, repositoryTargetID string, basePath string) (string, error) {
	switch source {
	case "fixtures", "default":
		root := omegaProjectRoot()
		if root == "" {
			return "", fmt.Errorf("Omega project root not found; cannot load built-in workflow fixtures")
		}
		return filepath.Join(root, "docs", "test-workflow-fixtures"), nil
	case "repository":
		database, err := server.Repo.Load(ctx)
		if err != nil {
			return "", err
		}
		target := findRepositoryTarget(*database, repositoryTargetID)
		if target == nil {
			return "", fmt.Errorf("repository target %s not found", repositoryTargetID)
		}
		repoPath := strings.TrimSpace(text(target, "path"))
		if repoPath == "" {
			return "", fmt.Errorf("repository target has no local path; bind or prepare a local repository workspace first")
		}
		return filepath.Join(repoPath, ".omega"), nil
	case "path":
		if basePath == "" {
			return "", fmt.Errorf("basePath is required for path import")
		}
		return basePath, nil
	default:
		return "", fmt.Errorf("unsupported import source: %s", source)
	}
}

func omegaProjectRoot() string {
	if cwd, err := os.Getwd(); err == nil {
		current := cwd
		for {
			if pathExists(filepath.Join(current, "package.json")) && pathExists(filepath.Join(current, "docs", "test-workflow-fixtures", "workflow.md")) {
				return current
			}
			parent := filepath.Dir(current)
			if parent == current {
				break
			}
			current = parent
		}
	}
	return ""
}

func importAgentProfileFromDirectory(current ProjectAgentProfile, basePath string, source string) (ProjectAgentProfile, map[string]any, error) {
	info, err := os.Stat(basePath)
	if err != nil || !info.IsDir() {
		return ProjectAgentProfile{}, nil, fmt.Errorf("template directory is not readable: %s", basePath)
	}
	profile := normalizeAgentProfile(current)
	summary := map[string]any{
		"source":   source,
		"basePath": basePath,
		"files":    []any{},
	}
	addFile := func(name string, path string) {
		files, _ := summary["files"].([]any)
		summary["files"] = append(files, map[string]any{"name": name, "path": path})
	}
	if workflow, path, ok := readFirstMarkdown(basePath, "WORKFLOW.md", "workflow.md"); ok {
		profile.WorkflowMarkdown = workflow
		profile.WorkflowTemplate = workflowIDFromMarkdown(workflow, profile.WorkflowTemplate)
		addFile("workflow", path)
	}
	if stagePolicy, path, ok := readFirstMarkdown(basePath, "STAGE_POLICY.md", "stage-policy.md", "stage_policy.md"); ok {
		profile.StagePolicy = stagePolicy
		addFile("stage-policy", path)
	}
	if prompts, path, ok := readFirstMarkdown(basePath, "PROMPTS.md", "prompts.md"); ok {
		sections := parseAgentPromptTemplateSections(prompts)
		if len(sections) > 0 {
			profile.WorkflowMarkdown = mergeWorkflowPromptSections(profile.WorkflowMarkdown, sections)
			profile.AgentProfiles = applyPromptSummariesToAgentProfiles(profile.AgentProfiles, sections)
		}
		addFile("prompts", path)
	}
	profile = normalizeAgentProfile(profile)
	return profile, summary, nil
}

func readFirstMarkdown(basePath string, names ...string) (string, string, bool) {
	for _, name := range names {
		path := filepath.Join(basePath, name)
		raw, err := os.ReadFile(path)
		if err == nil {
			return string(raw), path, true
		}
	}
	return "", "", false
}

func workflowIDFromMarkdown(markdown string, fallback string) string {
	for _, line := range strings.Split(markdown, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "id:") {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "id:")), `"'`)
		}
		if strings.HasPrefix(trimmed, "workflow:") {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "workflow:")), `"'`)
		}
	}
	return stringOr(fallback, "devflow-pr")
}

func parseAgentPromptTemplateSections(markdown string) map[string]string {
	sectionIDs := map[string]string{
		"requirement":  "requirement",
		"architect":    "architect",
		"architecture": "architect",
		"coding":       "coding",
		"testing":      "testing",
		"review":       "review",
		"rework":       "rework",
		"delivery":     "delivery",
	}
	output := map[string]string{}
	lines := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")
	currentID := ""
	current := []string{}
	flush := func() {
		if currentID == "" {
			return
		}
		body := strings.TrimSpace(strings.Join(current, "\n"))
		if body != "" {
			output[currentID] = body
		}
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") && !strings.HasPrefix(trimmed, "### ") {
			heading := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(trimmed, "## ")))
			if id, ok := sectionIDs[heading]; ok {
				flush()
				currentID = id
				current = []string{line}
				continue
			}
			if currentID != "" {
				flush()
				currentID = ""
				current = nil
			}
		}
		if currentID != "" {
			current = append(current, line)
		}
	}
	flush()
	return output
}

func mergeWorkflowPromptSections(markdown string, sections map[string]string) string {
	next := strings.TrimSpace(markdown)
	for id, body := range sections {
		next = replaceMarkdownPromptSection(next, id, body)
	}
	return next
}

func replaceMarkdownPromptSection(markdown string, id string, body string) string {
	lines := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")
	output := []string{}
	skipping := false
	found := false
	header := "## Prompt: " + id
	for index := 0; index < len(lines); index++ {
		line := lines[index]
		trimmed := strings.TrimSpace(line)
		if strings.EqualFold(trimmed, header) {
			found = true
			output = append(output, header, "", strings.TrimSpace(body), "")
			skipping = true
			continue
		}
		if skipping {
			if strings.HasPrefix(trimmed, "## Prompt:") {
				skipping = false
				output = append(output, line)
			}
			continue
		}
		output = append(output, line)
	}
	if !found {
		output = append(output, "", header, "", strings.TrimSpace(body))
	}
	return strings.TrimSpace(strings.Join(output, "\n")) + "\n"
}

func applyPromptSummariesToAgentProfiles(profiles []AgentProfileConfig, sections map[string]string) []AgentProfileConfig {
	next := append([]AgentProfileConfig{}, profiles...)
	for index := range next {
		if body := sections[next[index].ID]; strings.TrimSpace(body) != "" {
			next[index].StageNotes = firstPromptSubsection(body, "角色")
			if next[index].StageNotes == "" {
				next[index].StageNotes = firstNonHeadingParagraph(body)
			}
			if rules := firstPromptSubsection(body, "必须遵守"); rules != "" {
				next[index].CodexPolicy = rules
			}
			if failure := firstPromptSubsection(body, "失败处理"); failure != "" {
				next[index].ClaudePolicy = failure
			}
		}
	}
	return next
}

func firstPromptSubsection(markdown string, title string) string {
	lines := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")
	inSection := false
	values := []string{}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "### ") {
			if inSection {
				break
			}
			inSection = strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(trimmed, "### ")), title)
			continue
		}
		if inSection {
			values = append(values, line)
		}
	}
	return strings.TrimSpace(strings.Join(values, "\n"))
}

func firstNonHeadingParagraph(markdown string) string {
	for _, part := range strings.Split(markdown, "\n\n") {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		return trimmed
	}
	return ""
}
