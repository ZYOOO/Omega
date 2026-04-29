package omegalocal

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type ReviewRoundProfile struct {
	StageID            string `json:"stageId"`
	Artifact           string `json:"artifact"`
	Focus              string `json:"focus"`
	DiffSource         string `json:"diffSource"`
	ChangesRequestedTo string `json:"changesRequestedTo,omitempty"`
	NeedsHumanInfoTo   string `json:"needsHumanInfoTo,omitempty"`
}

type WorkflowTransitionProfile struct {
	From string `json:"from"`
	On   string `json:"on"`
	To   string `json:"to"`
}

type WorkflowRuntimeProfile struct {
	MaxReviewCycles         int      `json:"maxReviewCycles,omitempty"`
	RunnerHeartbeatSeconds  int      `json:"runnerHeartbeatSeconds,omitempty"`
	AttemptTimeoutMinutes   int      `json:"attemptTimeoutMinutes,omitempty"`
	MaxRetryAttempts        int      `json:"maxRetryAttempts,omitempty"`
	RetryBackoffSeconds     int      `json:"retryBackoffSeconds,omitempty"`
	CleanupRetentionSeconds int      `json:"cleanupRetentionSeconds,omitempty"`
	MaxContinuationTurns    int      `json:"maxContinuationTurns,omitempty"`
	RequiredChecks          []string `json:"requiredChecks,omitempty"`
}

func workflowPipelineTemplates() []PipelineTemplate {
	templates := []PipelineTemplate{}
	if template, ok := loadWorkflowPipelineTemplate("devflow-pr"); ok {
		templates = append(templates, template)
	}
	return templates
}

func loadWorkflowPipelineTemplate(templateID string) (PipelineTemplate, bool) {
	for _, path := range workflowTemplateCandidatePaths(templateID) {
		if raw, err := os.ReadFile(path); err == nil {
			template, parseErr := parseWorkflowTemplateMarkdown(string(raw), path)
			if parseErr == nil && template.ID != "" {
				return template, true
			}
		}
	}
	return PipelineTemplate{}, false
}

func workflowTemplateCandidatePaths(templateID string) []string {
	filename := templateID + ".md"
	candidates := []string{
		filepath.Join(".omega", "workflows", filename),
		filepath.Join("workflows", filename),
		filepath.Join("services", "local-runtime", "workflows", filename),
	}
	if cwd, err := os.Getwd(); err == nil {
		for _, relative := range []string{
			filepath.Join(".omega", "workflows", filename),
			filepath.Join("workflows", filename),
			filepath.Join("services", "local-runtime", "workflows", filename),
		} {
			for _, path := range upwardWorkflowCandidates(cwd, relative) {
				candidates = append(candidates, path)
			}
		}
	}
	return candidates
}

func upwardWorkflowCandidates(start string, relative string) []string {
	paths := []string{}
	current := start
	for {
		paths = append(paths, filepath.Join(current, relative))
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return paths
}

func parseWorkflowTemplateMarkdown(markdown string, sourcePath string) (PipelineTemplate, error) {
	frontMatter, prompt := splitWorkflowFrontMatter(markdown)
	promptTemplate, promptSections := parseWorkflowPromptSections(prompt)
	template := PipelineTemplate{Source: sourcePath, PromptTemplate: strings.TrimSpace(promptTemplate), WorkflowMarkdown: markdown, PromptSections: promptSections}
	currentSection := ""
	var currentStage *StageProfile
	var currentReview *ReviewRoundProfile
	var currentTransition *WorkflowTransitionProfile

	flushStage := func() {
		if currentStage != nil && currentStage.ID != "" {
			template.StageProfiles = append(template.StageProfiles, *currentStage)
		}
		currentStage = nil
	}
	flushReview := func() {
		if currentReview != nil && currentReview.StageID != "" {
			template.ReviewRounds = append(template.ReviewRounds, *currentReview)
		}
		currentReview = nil
	}
	flushTransition := func() {
		if currentTransition != nil && currentTransition.From != "" && currentTransition.On != "" && currentTransition.To != "" {
			template.Transitions = append(template.Transitions, *currentTransition)
		}
		currentTransition = nil
	}

	for _, rawLine := range strings.Split(frontMatter, "\n") {
		if strings.TrimSpace(rawLine) == "" || strings.HasPrefix(strings.TrimSpace(rawLine), "#") {
			continue
		}
		trimmed := strings.TrimSpace(rawLine)
		if !strings.HasPrefix(rawLine, " ") && strings.HasSuffix(trimmed, ":") {
			flushStage()
			flushReview()
			flushTransition()
			currentSection = strings.TrimSuffix(trimmed, ":")
			continue
		}
		if strings.HasPrefix(trimmed, "- ") {
			switch currentSection {
			case "stages":
				flushStage()
				flushReview()
				flushTransition()
				currentStage = &StageProfile{}
				applyWorkflowStageField(currentStage, strings.TrimPrefix(trimmed, "- "))
			case "reviewRounds":
				flushReview()
				flushStage()
				flushTransition()
				currentReview = &ReviewRoundProfile{}
				applyWorkflowReviewField(currentReview, strings.TrimPrefix(trimmed, "- "))
			case "transitions":
				flushTransition()
				flushStage()
				flushReview()
				currentTransition = &WorkflowTransitionProfile{}
				applyWorkflowTransitionField(currentTransition, strings.TrimPrefix(trimmed, "- "))
			}
			continue
		}
		if currentSection == "stages" && currentStage != nil {
			applyWorkflowStageField(currentStage, trimmed)
			continue
		}
		if currentSection == "reviewRounds" && currentReview != nil {
			applyWorkflowReviewField(currentReview, trimmed)
			continue
		}
		if currentSection == "transitions" && currentTransition != nil {
			applyWorkflowTransitionField(currentTransition, trimmed)
			continue
		}
		if currentSection == "runtime" {
			applyWorkflowRuntimeField(&template.Runtime, trimmed)
			continue
		}
		if currentSection == "" {
			key, value, ok := splitWorkflowKeyValue(trimmed)
			if !ok {
				continue
			}
			switch key {
			case "id":
				template.ID = value
			case "name":
				template.Name = value
			case "description":
				template.Description = value
			}
		}
	}
	flushStage()
	flushReview()
	flushTransition()
	return template, nil
}

type WorkflowValidationResult struct {
	Status   string   `json:"status"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}

func (result WorkflowValidationResult) ok() bool {
	return len(result.Errors) == 0
}

func validateWorkflowTemplate(template PipelineTemplate) WorkflowValidationResult {
	result := WorkflowValidationResult{Status: "passed", Errors: []string{}, Warnings: []string{}}
	if strings.TrimSpace(template.ID) == "" {
		result.Errors = append(result.Errors, "workflow id is required")
	}
	stageIDs := map[string]bool{}
	for _, stage := range template.StageProfiles {
		if strings.TrimSpace(stage.ID) == "" {
			result.Errors = append(result.Errors, "stage id is required")
			continue
		}
		if stageIDs[stage.ID] {
			result.Errors = append(result.Errors, "duplicate stage id: "+stage.ID)
		}
		stageIDs[stage.ID] = true
		if strings.TrimSpace(stage.Agent) == "" && len(stage.AgentIDs) == 0 {
			result.Errors = append(result.Errors, "stage "+stage.ID+" has no agent")
		}
	}
	if len(stageIDs) == 0 {
		result.Errors = append(result.Errors, "at least one stage is required")
	}
	for _, transition := range template.Transitions {
		if !stageIDs[transition.From] {
			result.Errors = append(result.Errors, "transition from unknown stage: "+transition.From)
		}
		if !stageIDs[transition.To] {
			result.Errors = append(result.Errors, "transition to unknown stage: "+transition.To)
		}
		if strings.TrimSpace(transition.On) == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("transition %s -> %s has no event", transition.From, transition.To))
		}
	}
	for _, review := range template.ReviewRounds {
		if !stageIDs[review.StageID] {
			result.Errors = append(result.Errors, "review round references unknown stage: "+review.StageID)
		}
		if strings.TrimSpace(review.Artifact) == "" {
			result.Errors = append(result.Errors, "review round "+review.StageID+" has no artifact")
		}
	}
	if template.Runtime.MaxReviewCycles < 0 || template.Runtime.AttemptTimeoutMinutes < 0 || template.Runtime.MaxRetryAttempts < 0 || template.Runtime.RetryBackoffSeconds < 0 {
		result.Errors = append(result.Errors, "runtime values must not be negative")
	}
	if len(result.Errors) > 0 {
		result.Status = "failed"
	}
	return result
}

func loadRepositoryWorkflowTemplate(repositoryPath string, fallbackTemplateID string) (PipelineTemplate, WorkflowValidationResult, bool) {
	path := filepath.Join(repositoryPath, ".omega", "WORKFLOW.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		return PipelineTemplate{}, WorkflowValidationResult{Status: "missing"}, false
	}
	template, parseErr := parseWorkflowTemplateMarkdown(string(raw), path)
	if parseErr != nil {
		return PipelineTemplate{}, WorkflowValidationResult{Status: "failed", Errors: []string{parseErr.Error()}}, true
	}
	if template.ID == "" {
		template.ID = fallbackTemplateID
	}
	validation := validateWorkflowTemplate(template)
	return template, validation, true
}

func loadProfileWorkflowTemplate(profile ProjectAgentProfile, fallbackTemplateID string) (PipelineTemplate, WorkflowValidationResult, bool) {
	if !strings.HasPrefix(strings.TrimSpace(profile.WorkflowMarkdown), "---") {
		return PipelineTemplate{}, WorkflowValidationResult{Status: "missing"}, false
	}
	template, err := parseWorkflowTemplateMarkdown(profile.WorkflowMarkdown, "agent-profile:"+profile.ProjectID+":"+profile.RepositoryTargetID)
	if err != nil {
		return PipelineTemplate{}, WorkflowValidationResult{Status: "failed", Errors: []string{err.Error()}}, true
	}
	if template.ID == "" {
		template.ID = fallbackTemplateID
	}
	validation := validateWorkflowTemplate(template)
	return template, validation, true
}

func parseWorkflowPromptSections(markdown string) (string, map[string]string) {
	lines := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")
	clean := []string{}
	sections := map[string][]string{}
	current := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## Prompt:") {
			current = strings.TrimSpace(strings.TrimPrefix(trimmed, "## Prompt:"))
			if current != "" {
				sections[current] = []string{}
			}
			continue
		}
		if current != "" {
			sections[current] = append(sections[current], line)
			continue
		}
		clean = append(clean, line)
	}
	output := map[string]string{}
	for key, values := range sections {
		if text := strings.TrimSpace(strings.Join(values, "\n")); text != "" {
			output[key] = text
		}
	}
	if len(output) == 0 {
		return markdown, nil
	}
	return strings.TrimSpace(strings.Join(clean, "\n")), output
}

func splitWorkflowFrontMatter(markdown string) (string, string) {
	normalized := strings.ReplaceAll(markdown, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", normalized
	}
	for index := 1; index < len(lines); index++ {
		if strings.TrimSpace(lines[index]) == "---" {
			return strings.Join(lines[1:index], "\n"), strings.Join(lines[index+1:], "\n")
		}
	}
	return strings.Join(lines[1:], "\n"), ""
}

func applyWorkflowStageField(stage *StageProfile, line string) {
	key, value, ok := splitWorkflowKeyValue(line)
	if !ok {
		return
	}
	switch key {
	case "id":
		stage.ID = value
	case "title":
		stage.Title = value
	case "agentId", "agent":
		stage.Agent = value
	case "humanGate":
		stage.HumanGate = value == "true"
	case "agents":
		stage.AgentIDs = parseWorkflowStringList(value)
	case "inputArtifacts":
		stage.InputArtifacts = parseWorkflowStringList(value)
	case "outputArtifacts":
		stage.OutputArtifacts = parseWorkflowStringList(value)
	}
}

func applyWorkflowReviewField(review *ReviewRoundProfile, line string) {
	key, value, ok := splitWorkflowKeyValue(line)
	if !ok {
		return
	}
	switch key {
	case "stageId":
		review.StageID = value
	case "artifact":
		review.Artifact = value
	case "focus":
		review.Focus = value
	case "diffSource":
		review.DiffSource = value
	case "changesRequestedTo":
		review.ChangesRequestedTo = value
	case "needsHumanInfoTo":
		review.NeedsHumanInfoTo = value
	}
}

func applyWorkflowTransitionField(transition *WorkflowTransitionProfile, line string) {
	key, value, ok := splitWorkflowKeyValue(line)
	if !ok {
		return
	}
	switch key {
	case "from":
		transition.From = value
	case "on":
		transition.On = value
	case "to":
		transition.To = value
	}
}

func applyWorkflowRuntimeField(runtime *WorkflowRuntimeProfile, line string) {
	key, value, ok := splitWorkflowKeyValue(line)
	if !ok {
		return
	}
	switch key {
	case "maxReviewCycles":
		if parsed, err := strconv.Atoi(value); err == nil {
			runtime.MaxReviewCycles = parsed
		}
	case "runnerHeartbeatSeconds":
		if parsed, err := strconv.Atoi(value); err == nil {
			runtime.RunnerHeartbeatSeconds = parsed
		}
	case "attemptTimeoutMinutes":
		if parsed, err := strconv.Atoi(value); err == nil {
			runtime.AttemptTimeoutMinutes = parsed
		}
	case "maxRetryAttempts":
		if parsed, err := strconv.Atoi(value); err == nil {
			runtime.MaxRetryAttempts = parsed
		}
	case "retryBackoffSeconds":
		if parsed, err := strconv.Atoi(value); err == nil {
			runtime.RetryBackoffSeconds = parsed
		}
	case "cleanupRetentionSeconds":
		if parsed, err := strconv.Atoi(value); err == nil {
			runtime.CleanupRetentionSeconds = parsed
		}
	case "maxContinuationTurns":
		if parsed, err := strconv.Atoi(value); err == nil {
			runtime.MaxContinuationTurns = parsed
		}
	case "requiredChecks":
		runtime.RequiredChecks = parseWorkflowStringList(value)
	}
}

func splitWorkflowKeyValue(line string) (string, string, bool) {
	before, after, ok := strings.Cut(line, ":")
	if !ok {
		return "", "", false
	}
	return strings.TrimSpace(before), strings.Trim(strings.TrimSpace(after), `"`), true
}

func parseWorkflowStringList(value string) []string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.TrimPrefix(trimmed, "[")
	trimmed = strings.TrimSuffix(trimmed, "]")
	if strings.TrimSpace(trimmed) == "" {
		return nil
	}
	parts := strings.Split(trimmed, ",")
	output := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.Trim(strings.TrimSpace(part), `"`)
		if item != "" {
			output = append(output, item)
		}
	}
	return output
}

func anyListFromStrings(values []string) []any {
	if len(values) == 0 {
		return nil
	}
	output := make([]any, 0, len(values))
	for _, value := range values {
		output = append(output, value)
	}
	return output
}
