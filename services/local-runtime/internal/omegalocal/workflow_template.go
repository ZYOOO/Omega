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

type WorkflowActionProfile struct {
	ID              string            `json:"id"`
	Type            string            `json:"type"`
	Agent           string            `json:"agent,omitempty"`
	Prompt          string            `json:"prompt,omitempty"`
	Mode            string            `json:"mode,omitempty"`
	DiffSource      string            `json:"diffSource,omitempty"`
	RequiresDiff    bool              `json:"requiresDiff,omitempty"`
	InputArtifacts  []string          `json:"inputArtifacts,omitempty"`
	OutputArtifacts []string          `json:"outputArtifacts,omitempty"`
	Transitions     map[string]string `json:"transitions,omitempty"`
	Verdicts        map[string]string `json:"verdicts,omitempty"`
}

type WorkflowStateProfile struct {
	ID              string                  `json:"id"`
	Title           string                  `json:"title"`
	Agent           string                  `json:"agentId,omitempty"`
	AgentIDs        []string                `json:"agents,omitempty"`
	HumanGate       bool                    `json:"humanGate,omitempty"`
	InputArtifacts  []string                `json:"inputArtifacts,omitempty"`
	OutputArtifacts []string                `json:"outputArtifacts,omitempty"`
	Actions         []WorkflowActionProfile `json:"actions,omitempty"`
	Transitions     map[string]string       `json:"transitions,omitempty"`
}

type WorkflowTaskClassProfile struct {
	ID              string   `json:"id"`
	Title           string   `json:"title,omitempty"`
	WorkpadMode     string   `json:"workpadMode,omitempty"`
	PlanningMode    string   `json:"planningMode,omitempty"`
	ValidationMode  string   `json:"validationMode,omitempty"`
	MaxChangedFiles int      `json:"maxChangedFiles,omitempty"`
	Signals         []string `json:"signals,omitempty"`
}

type WorkflowHookProfile struct {
	AfterCreate    string `json:"afterCreate,omitempty"`
	BeforeRun      string `json:"beforeRun,omitempty"`
	AfterRun       string `json:"afterRun,omitempty"`
	BeforeRemove   string `json:"beforeRemove,omitempty"`
	TimeoutSeconds int    `json:"timeoutSeconds,omitempty"`
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
	template.StateProfiles = parseWorkflowStateProfiles(frontMatter)
	template.TaskClasses = parseWorkflowTaskClasses(frontMatter)
	template.Hooks = parseWorkflowHooks(frontMatter)
	if len(template.StateProfiles) > 0 {
		template.StageProfiles = stageProfilesFromWorkflowStates(template.StateProfiles)
		template.Transitions = append(template.Transitions, transitionsFromWorkflowStates(template.StateProfiles)...)
	}
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
				if len(template.StateProfiles) > 0 {
					continue
				}
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
		if currentSection == "stages" && currentStage != nil && len(template.StateProfiles) == 0 {
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
	template.Transitions = uniqueWorkflowTransitions(template.Transitions)
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
	actionIDs := map[string]bool{}
	for _, state := range template.StateProfiles {
		if strings.TrimSpace(state.ID) == "" {
			result.Errors = append(result.Errors, "workflow state id is required")
			continue
		}
		if !stageIDs[state.ID] {
			result.Errors = append(result.Errors, "workflow state has no matching stage: "+state.ID)
		}
		for event, target := range state.Transitions {
			if strings.TrimSpace(event) == "" {
				result.Errors = append(result.Errors, "state "+state.ID+" has a blank transition event")
			}
			if !stageIDs[target] {
				result.Errors = append(result.Errors, "state "+state.ID+" transition points to unknown stage: "+target)
			}
		}
		for _, action := range state.Actions {
			if strings.TrimSpace(action.ID) == "" {
				result.Errors = append(result.Errors, "state "+state.ID+" action id is required")
				continue
			}
			actionKey := state.ID + "/" + action.ID
			if actionIDs[actionKey] {
				result.Errors = append(result.Errors, "duplicate workflow action id in state "+state.ID+": "+action.ID)
			}
			actionIDs[actionKey] = true
			if strings.TrimSpace(action.Type) == "" {
				result.Errors = append(result.Errors, "action "+action.ID+" has no type")
			} else if workflowActionHandlerName(action.Type) == "" {
				result.Errors = append(result.Errors, "action "+action.ID+" uses unsupported action type: "+action.Type)
			}
			for event, target := range action.Transitions {
				if strings.TrimSpace(event) == "" {
					result.Errors = append(result.Errors, "action "+action.ID+" has a blank transition event")
				}
				if !stageIDs[target] {
					result.Errors = append(result.Errors, "action "+action.ID+" transition points to unknown stage: "+target)
				}
			}
			for verdict, target := range action.Verdicts {
				if strings.TrimSpace(verdict) == "" {
					result.Errors = append(result.Errors, "action "+action.ID+" has a blank verdict")
				}
				if !stageIDs[target] {
					result.Errors = append(result.Errors, "action "+action.ID+" verdict points to unknown stage: "+target)
				}
			}
		}
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

func parseWorkflowStateProfiles(frontMatter string) []WorkflowStateProfile {
	lines := strings.Split(strings.ReplaceAll(frontMatter, "\r\n", "\n"), "\n")
	states := []WorkflowStateProfile{}
	inStates := false
	var currentState *WorkflowStateProfile
	var currentAction *WorkflowActionProfile
	currentMap := ""

	flushAction := func() {
		if currentState != nil && currentAction != nil && currentAction.ID != "" {
			currentState.Actions = append(currentState.Actions, *currentAction)
		}
		currentAction = nil
		currentMap = ""
	}
	flushState := func() {
		flushAction()
		if currentState != nil && currentState.ID != "" {
			states = append(states, *currentState)
		}
		currentState = nil
	}

	for _, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := leadingSpaces(raw)
		if indent == 0 {
			if trimmed == "states:" {
				flushState()
				inStates = true
				continue
			}
			if inStates {
				flushState()
			}
			inStates = false
			continue
		}
		if !inStates {
			continue
		}
		switch {
		case indent == 2 && strings.HasPrefix(trimmed, "- "):
			flushState()
			currentState = &WorkflowStateProfile{}
			applyWorkflowStateField(currentState, strings.TrimPrefix(trimmed, "- "))
		case currentState != nil && indent == 4 && trimmed == "actions:":
			flushAction()
			currentMap = ""
		case currentState != nil && indent == 6 && strings.HasPrefix(trimmed, "- "):
			flushAction()
			currentAction = &WorkflowActionProfile{}
			applyWorkflowActionField(currentAction, strings.TrimPrefix(trimmed, "- "))
		case currentState != nil && currentAction != nil && indent == 8 && (trimmed == "transitions:" || trimmed == "verdicts:"):
			currentMap = trimmed[:len(trimmed)-1]
		case currentState != nil && currentAction != nil && indent >= 10 && currentMap != "":
			applyWorkflowActionMapField(currentAction, currentMap, trimmed)
		case currentState != nil && currentAction != nil && indent >= 8:
			currentMap = ""
			applyWorkflowActionField(currentAction, trimmed)
		case currentState != nil && indent == 4 && trimmed == "transitions:":
			currentMap = "state.transitions"
		case currentState != nil && indent >= 6 && currentMap == "state.transitions":
			applyWorkflowStateTransitionField(currentState, trimmed)
		case currentState != nil && indent >= 4:
			currentMap = ""
			applyWorkflowStateField(currentState, trimmed)
		}
	}
	flushState()
	return states
}

func parseWorkflowTaskClasses(frontMatter string) []WorkflowTaskClassProfile {
	lines := strings.Split(strings.ReplaceAll(frontMatter, "\r\n", "\n"), "\n")
	classes := []WorkflowTaskClassProfile{}
	inClasses := false
	var current *WorkflowTaskClassProfile
	flush := func() {
		if current != nil && current.ID != "" {
			classes = append(classes, *current)
		}
		current = nil
	}
	for _, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := leadingSpaces(raw)
		if indent == 0 {
			if trimmed == "taskClasses:" {
				flush()
				inClasses = true
				continue
			}
			if inClasses {
				flush()
			}
			inClasses = false
			continue
		}
		if !inClasses {
			continue
		}
		if indent == 2 && strings.HasPrefix(trimmed, "- ") {
			flush()
			current = &WorkflowTaskClassProfile{}
			applyWorkflowTaskClassField(current, strings.TrimPrefix(trimmed, "- "))
			continue
		}
		if current != nil && indent >= 4 {
			applyWorkflowTaskClassField(current, trimmed)
		}
	}
	flush()
	return classes
}

func parseWorkflowHooks(frontMatter string) WorkflowHookProfile {
	lines := strings.Split(strings.ReplaceAll(frontMatter, "\r\n", "\n"), "\n")
	hooks := WorkflowHookProfile{}
	inHooks := false
	currentBlockKey := ""
	blockIndent := 0
	blockLines := []string{}
	flushBlock := func() {
		if currentBlockKey == "" {
			return
		}
		value := strings.TrimRight(strings.Join(blockLines, "\n"), "\n")
		switch currentBlockKey {
		case "after_create", "afterCreate":
			hooks.AfterCreate = value
		case "before_run", "beforeRun":
			hooks.BeforeRun = value
		case "after_run", "afterRun":
			hooks.AfterRun = value
		case "before_remove", "beforeRemove":
			hooks.BeforeRemove = value
		}
		currentBlockKey = ""
		blockLines = nil
	}
	for _, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" && currentBlockKey != "" {
			blockLines = append(blockLines, "")
			continue
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := leadingSpaces(raw)
		if currentBlockKey != "" {
			if indent > blockIndent {
				blockLines = append(blockLines, trimWorkflowBlockLine(raw, blockIndent+2))
				continue
			}
			flushBlock()
		}
		if indent == 0 {
			if trimmed == "hooks:" {
				inHooks = true
				continue
			}
			inHooks = false
			continue
		}
		if !inHooks || indent != 2 {
			continue
		}
		key, value, ok := splitWorkflowKeyValue(trimmed)
		if !ok {
			continue
		}
		if value == "|" {
			currentBlockKey = key
			blockIndent = indent
			blockLines = []string{}
			continue
		}
		applyWorkflowHookField(&hooks, key, value)
	}
	flushBlock()
	return hooks
}

func stageProfilesFromWorkflowStates(states []WorkflowStateProfile) []StageProfile {
	stages := make([]StageProfile, 0, len(states))
	for _, state := range states {
		stage := StageProfile{
			ID:              state.ID,
			Title:           state.Title,
			Agent:           state.Agent,
			AgentIDs:        state.AgentIDs,
			HumanGate:       state.HumanGate,
			InputArtifacts:  state.InputArtifacts,
			OutputArtifacts: state.OutputArtifacts,
		}
		if stage.Agent == "" && len(stage.AgentIDs) > 0 {
			stage.Agent = stage.AgentIDs[0]
		}
		stages = append(stages, stage)
	}
	return stages
}

func transitionsFromWorkflowStates(states []WorkflowStateProfile) []WorkflowTransitionProfile {
	transitions := []WorkflowTransitionProfile{}
	for _, state := range states {
		for event, to := range state.Transitions {
			transitions = append(transitions, WorkflowTransitionProfile{From: state.ID, On: event, To: to})
		}
		for _, action := range state.Actions {
			for event, to := range action.Transitions {
				transitions = append(transitions, WorkflowTransitionProfile{From: state.ID, On: event, To: to})
			}
			for event, to := range action.Verdicts {
				transitions = append(transitions, WorkflowTransitionProfile{From: state.ID, On: event, To: to})
			}
		}
	}
	return transitions
}

func workflowActionPlan(template *PipelineTemplate) []map[string]any {
	if template == nil {
		return nil
	}
	actions := []map[string]any{}
	for _, state := range template.StateProfiles {
		for index, action := range state.Actions {
			actions = append(actions, map[string]any{
				"id":              action.ID,
				"type":            action.Type,
				"handler":         workflowActionHandlerName(action.Type),
				"stateId":         state.ID,
				"stateTitle":      state.Title,
				"order":           index + 1,
				"agent":           action.Agent,
				"prompt":          action.Prompt,
				"mode":            action.Mode,
				"diffSource":      action.DiffSource,
				"requiresDiff":    action.RequiresDiff,
				"inputArtifacts":  action.InputArtifacts,
				"outputArtifacts": action.OutputArtifacts,
				"transitions":     action.Transitions,
				"verdicts":        action.Verdicts,
			})
		}
	}
	return actions
}

func workflowExecutionMode(template *PipelineTemplate) string {
	if template != nil && len(template.StateProfiles) > 0 && len(workflowActionPlan(template)) > 0 {
		return "contract-action-executor"
	}
	return "legacy-stage-plan"
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

func applyWorkflowStateField(state *WorkflowStateProfile, line string) {
	key, value, ok := splitWorkflowKeyValue(line)
	if !ok {
		return
	}
	switch key {
	case "id":
		state.ID = value
	case "title":
		state.Title = value
	case "agentId", "agent":
		state.Agent = value
	case "humanGate":
		state.HumanGate = parseWorkflowBool(value)
	case "agents":
		state.AgentIDs = parseWorkflowStringList(value)
	case "inputArtifacts":
		state.InputArtifacts = parseWorkflowStringList(value)
	case "outputArtifacts":
		state.OutputArtifacts = parseWorkflowStringList(value)
	}
}

func applyWorkflowActionField(action *WorkflowActionProfile, line string) {
	key, value, ok := splitWorkflowKeyValue(line)
	if !ok {
		return
	}
	switch key {
	case "id":
		action.ID = value
	case "type":
		action.Type = value
	case "agent":
		action.Agent = value
	case "prompt":
		action.Prompt = value
	case "mode":
		action.Mode = value
	case "diffSource":
		action.DiffSource = value
	case "requiresDiff":
		action.RequiresDiff = parseWorkflowBool(value)
	case "inputArtifacts":
		action.InputArtifacts = parseWorkflowStringList(value)
	case "outputArtifacts":
		action.OutputArtifacts = parseWorkflowStringList(value)
	}
}

func applyWorkflowActionMapField(action *WorkflowActionProfile, mapName string, line string) {
	key, value, ok := splitWorkflowKeyValue(line)
	if !ok || key == "" || value == "" {
		return
	}
	switch mapName {
	case "transitions":
		if action.Transitions == nil {
			action.Transitions = map[string]string{}
		}
		action.Transitions[key] = value
	case "verdicts":
		if action.Verdicts == nil {
			action.Verdicts = map[string]string{}
		}
		action.Verdicts[key] = value
	}
}

func applyWorkflowStateTransitionField(state *WorkflowStateProfile, line string) {
	key, value, ok := splitWorkflowKeyValue(line)
	if !ok || key == "" || value == "" {
		return
	}
	if state.Transitions == nil {
		state.Transitions = map[string]string{}
	}
	state.Transitions[key] = value
}

func applyWorkflowTaskClassField(taskClass *WorkflowTaskClassProfile, line string) {
	key, value, ok := splitWorkflowKeyValue(line)
	if !ok {
		return
	}
	switch key {
	case "id":
		taskClass.ID = value
	case "title":
		taskClass.Title = value
	case "workpadMode":
		taskClass.WorkpadMode = value
	case "planningMode":
		taskClass.PlanningMode = value
	case "validationMode":
		taskClass.ValidationMode = value
	case "maxChangedFiles":
		if parsed, err := strconv.Atoi(value); err == nil {
			taskClass.MaxChangedFiles = parsed
		}
	case "signals":
		taskClass.Signals = parseWorkflowStringList(value)
	}
}

func applyWorkflowHookField(hooks *WorkflowHookProfile, key string, value string) {
	switch key {
	case "after_create", "afterCreate":
		hooks.AfterCreate = value
	case "before_run", "beforeRun":
		hooks.BeforeRun = value
	case "after_run", "afterRun":
		hooks.AfterRun = value
	case "before_remove", "beforeRemove":
		hooks.BeforeRemove = value
	case "timeoutSeconds", "timeout_seconds":
		if parsed, err := strconv.Atoi(value); err == nil {
			hooks.TimeoutSeconds = parsed
		}
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
	return strings.TrimSpace(before), strings.Trim(strings.Trim(strings.TrimSpace(after), `"`), `'`), true
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

func parseWorkflowBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "yes", "1":
		return true
	default:
		return false
	}
}

func leadingSpaces(value string) int {
	count := 0
	for _, char := range value {
		if char != ' ' {
			break
		}
		count++
	}
	return count
}

func trimWorkflowBlockLine(line string, trimIndent int) string {
	if len(line) <= trimIndent {
		return strings.TrimLeft(line, " ")
	}
	return line[trimIndent:]
}

func uniqueWorkflowTransitions(transitions []WorkflowTransitionProfile) []WorkflowTransitionProfile {
	seen := map[string]bool{}
	output := []WorkflowTransitionProfile{}
	for _, transition := range transitions {
		key := transition.From + "\x00" + transition.On + "\x00" + transition.To
		if transition.From == "" || transition.On == "" || transition.To == "" || seen[key] {
			continue
		}
		seen[key] = true
		output = append(output, transition)
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
