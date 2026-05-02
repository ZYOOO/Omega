package omegalocal

import "strings"

type workflowActionRouteResult struct {
	StageStatus string
	NextStageID string
	Event       string
	ActionID    string
	ActionType  string
	Handler     string
	Source      string
}

func workflowActionRouteFromPipeline(pipeline map[string]any, stageID string, agentID string, status string) workflowActionRouteResult {
	workflow := mapValue(mapValue(pipeline["run"])["workflow"])
	template := findPipelineTemplate(text(pipeline, "templateId"))
	return workflowActionRoute(workflow, template, stageID, agentID, status)
}

func workflowActionRoute(workflow map[string]any, template *PipelineTemplate, stageID string, agentID string, status string) workflowActionRouteResult {
	result := workflowActionRouteResult{
		StageStatus: "running",
		Handler:     "workflow-action-handler",
		Source:      "workflow.action",
	}
	normalizedStatus := strings.ToLower(strings.TrimSpace(status))
	switch normalizedStatus {
	case "running":
		result.StageStatus = "running"
		return result
	case "failed":
		result.StageStatus = "failed"
		result.Event = "failed"
		result.NextStageID = workflowActionTransitionTo(workflow, template, stageID, result.Event, "")
		return result
	case "changes-requested", "changes_requested":
		result.StageStatus = "passed"
		result.Event = "changes_requested"
	case "needs-human", "waiting-human", "needs_human_info":
		result.StageStatus = "needs-human"
		result.Event = "needs_human_info"
		result.NextStageID = workflowActionTransitionTo(workflow, template, stageID, result.Event, "")
		return result
	case "passed", "done":
		if stageID == "in_progress" && agentID != "testing" {
			result.StageStatus = "running"
			return result
		}
		result.StageStatus = "passed"
		result.Event = "passed"
	default:
		return result
	}

	action := workflowActionForStage(workflow, template, stageID, agentID, result.Event)
	result.ActionID = text(action, "id")
	result.ActionType = text(action, "type")
	if result.Event == "passed" && result.ActionType == "run_review" {
		result.Event = "approved"
	}

	fallback := ""
	if result.Event == "passed" || result.Event == "approved" {
		fallback = devFlowNextStageAfter(stageID)
	}
	if result.Event == "changes_requested" {
		fallback = "rework"
	}
	result.NextStageID = workflowActionTransitionTo(workflow, template, stageID, result.Event, fallback)
	return result
}

func workflowActionForStage(workflow map[string]any, template *PipelineTemplate, stageID string, agentID string, event string) map[string]any {
	actions := workflowPlanActionsForState(workflow, stageID)
	if len(actions) == 0 && template != nil {
		actions = workflowActionMapsFromTemplate(template, stageID)
	}
	if len(actions) == 0 {
		return nil
	}
	best := map[string]any{}
	for _, action := range actions {
		if actionSupportsEvent(action, event) {
			best = action
			if text(action, "agent") == agentID {
				return action
			}
		}
	}
	if len(best) > 0 {
		return best
	}
	for _, action := range actions {
		if text(action, "agent") == agentID {
			return action
		}
	}
	return actions[0]
}

func actionSupportsEvent(action map[string]any, event string) bool {
	if event == "" {
		return false
	}
	if _, ok := mapValue(action["transitions"])[event]; ok {
		return true
	}
	if _, ok := mapValue(action["verdicts"])[event]; ok {
		return true
	}
	if event == "passed" && text(action, "type") != "run_review" {
		return true
	}
	if event == "approved" && text(action, "type") == "run_review" {
		return true
	}
	return false
}

func workflowActionTransitionTo(workflow map[string]any, template *PipelineTemplate, from string, event string, fallback string) string {
	if event == "" {
		return fallback
	}
	for _, action := range workflowPlanActionsForState(workflow, from) {
		if to := transitionTargetFromAction(action, event); to != "" {
			return to
		}
	}
	for _, state := range arrayMaps(workflow["states"]) {
		if text(state, "id") != from {
			continue
		}
		if to := stringOr(mapValue(state["transitions"])[event], ""); to != "" {
			return to
		}
	}
	for _, transition := range arrayMaps(workflow["transitions"]) {
		if text(transition, "from") == from && text(transition, "on") == event && text(transition, "to") != "" {
			return text(transition, "to")
		}
	}
	if template != nil {
		for _, state := range template.StateProfiles {
			if state.ID != from {
				continue
			}
			for _, action := range state.Actions {
				if to := transitionTargetFromAction(workflowActionMapFromProfile(action, from, state.Title), event); to != "" {
					return to
				}
			}
			if to := state.Transitions[event]; to != "" {
				return to
			}
		}
		for _, transition := range template.Transitions {
			if transition.From == from && transition.On == event && transition.To != "" {
				return transition.To
			}
		}
	}
	return fallback
}

func transitionTargetFromAction(action map[string]any, event string) string {
	if to := stringOr(mapValue(action["verdicts"])[event], ""); to != "" {
		return to
	}
	if to := stringOr(mapValue(action["transitions"])[event], ""); to != "" {
		return to
	}
	return ""
}

func workflowActionMapsFromTemplate(template *PipelineTemplate, stageID string) []map[string]any {
	if template == nil {
		return nil
	}
	actions := []map[string]any{}
	for _, state := range template.StateProfiles {
		if state.ID != stageID {
			continue
		}
		for index, action := range state.Actions {
			actionMap := workflowActionMapFromProfile(action, state.ID, state.Title)
			actionMap["order"] = index + 1
			actions = append(actions, actionMap)
		}
		break
	}
	return actions
}

func workflowActionMapFromProfile(action WorkflowActionProfile, stateID string, stateTitle string) map[string]any {
	return map[string]any{
		"id":              action.ID,
		"type":            action.Type,
		"agent":           action.Agent,
		"prompt":          action.Prompt,
		"mode":            action.Mode,
		"diffSource":      action.DiffSource,
		"requiresDiff":    action.RequiresDiff,
		"inputArtifacts":  action.InputArtifacts,
		"outputArtifacts": action.OutputArtifacts,
		"transitions":     stringMapToAnyMap(action.Transitions),
		"verdicts":        stringMapToAnyMap(action.Verdicts),
		"stateId":         stateID,
		"stateTitle":      stateTitle,
	}
}

func stringMapToAnyMap(values map[string]string) map[string]any {
	if len(values) == 0 {
		return nil
	}
	output := map[string]any{}
	for key, value := range values {
		output[key] = value
	}
	return output
}

func workflowActionRouteMap(route workflowActionRouteResult) map[string]any {
	output := map[string]any{
		"handler":     route.Handler,
		"stageStatus": route.StageStatus,
		"event":       route.Event,
		"source":      route.Source,
	}
	if route.NextStageID != "" {
		output["nextStageId"] = route.NextStageID
	}
	if route.ActionID != "" {
		output["actionId"] = route.ActionID
	}
	if route.ActionType != "" {
		output["actionType"] = route.ActionType
	}
	return output
}
