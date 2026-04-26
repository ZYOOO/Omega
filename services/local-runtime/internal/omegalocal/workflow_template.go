package omegalocal

import (
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
	MaxReviewCycles int `json:"maxReviewCycles,omitempty"`
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
	template := PipelineTemplate{Source: sourcePath, PromptTemplate: strings.TrimSpace(prompt), WorkflowMarkdown: markdown}
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
