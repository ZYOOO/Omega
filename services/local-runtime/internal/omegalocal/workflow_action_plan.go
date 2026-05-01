package omegalocal

import (
	"net/http"
	"strings"
)

func (server *Server) attemptActionPlan(response http.ResponseWriter, request *http.Request) {
	attemptID := strings.TrimSuffix(pathID(request.URL.Path), "/action-plan")
	database, err := mustLoad(server, request.Context())
	if err != nil {
		writeJSON(response, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	plan, status := buildAttemptActionPlan(database, attemptID)
	writeJSON(response, status, plan)
}

func buildAttemptActionPlan(database WorkspaceDatabase, attemptID string) (map[string]any, int) {
	attemptIndex := findByID(database.Tables.Attempts, attemptID)
	if attemptIndex < 0 {
		return map[string]any{"error": "attempt not found"}, http.StatusNotFound
	}
	attempt := cloneMap(database.Tables.Attempts[attemptIndex])
	pipeline := findPipeline(database, text(attempt, "pipelineId"))
	if pipeline == nil {
		return map[string]any{"error": "pipeline not found", "attemptId": attemptID}, http.StatusNotFound
	}

	run := mapValue(pipeline["run"])
	workflow := mapValue(run["workflow"])
	stateID := workflowPlanCurrentStateID(attempt, pipeline)
	stage := workflowPlanStage(run, stateID)
	actions := workflowPlanActionsForState(workflow, stateID)
	actionRecords := workflowPlanActionRecords(actions, text(stage, "status"), text(attempt, "status"))
	currentAction := workflowPlanCurrentAction(actionRecords)

	plan := map[string]any{
		"attemptId":           attemptID,
		"pipelineId":          text(pipeline, "id"),
		"workItemId":          text(pipeline, "workItemId"),
		"templateId":          text(pipeline, "templateId"),
		"workflowId":          stringOr(workflow["id"], text(pipeline, "templateId")),
		"executionMode":       stringOr(workflow["executionMode"], "legacy-stage-plan"),
		"attemptStatus":       text(attempt, "status"),
		"pipelineStatus":      text(pipeline, "status"),
		"currentState":        workflowPlanStateRecord(workflow, run, stateID, text(stage, "status")),
		"currentAction":       currentAction,
		"actions":             actionRecords,
		"transitions":         workflowPlanTransitions(workflow, stateID),
		"states":              workflowPlanStateRecords(workflow, run),
		"taskClasses":         workflow["taskClasses"],
		"hooks":               workflow["hooks"],
		"retry":               workflowPlanRetryRecord(attempt, stateID),
		"source":              "pipeline.workflow.snapshot",
		"contractActionCount": len(arrayMaps(workflow["actions"])),
	}
	return plan, http.StatusOK
}

func workflowPlanCurrentStateID(attempt map[string]any, pipeline map[string]any) string {
	if stageID := text(attempt, "currentStageId"); stageID != "" {
		return stageID
	}
	run := mapValue(pipeline["run"])
	for _, wanted := range []string{"running", "needs-human", "waiting-human", "ready", "failed", "stalled"} {
		for _, stage := range arrayMaps(run["stages"]) {
			if text(stage, "status") == wanted {
				return text(stage, "id")
			}
		}
	}
	stages := arrayMaps(run["stages"])
	if len(stages) == 0 {
		return ""
	}
	return text(stages[0], "id")
}

func workflowPlanStage(run map[string]any, stateID string) map[string]any {
	for _, stage := range arrayMaps(run["stages"]) {
		if text(stage, "id") == stateID {
			return stage
		}
	}
	return map[string]any{"id": stateID, "status": ""}
}

func workflowPlanActionsForState(workflow map[string]any, stateID string) []map[string]any {
	actions := []map[string]any{}
	for _, action := range arrayMaps(workflow["actions"]) {
		if text(action, "stateId") == stateID {
			actions = append(actions, action)
		}
	}
	if len(actions) > 0 {
		return actions
	}
	for _, state := range arrayMaps(workflow["states"]) {
		if text(state, "id") != stateID {
			continue
		}
		for index, action := range arrayMaps(state["actions"]) {
			action["stateId"] = stateID
			action["stateTitle"] = text(state, "title")
			action["order"] = index + 1
			actions = append(actions, action)
		}
	}
	return actions
}

func workflowPlanActionRecords(actions []map[string]any, stageStatus string, attemptStatus string) []map[string]any {
	records := make([]map[string]any, 0, len(actions))
	for index, action := range actions {
		record := cloneMap(action)
		record["status"] = workflowPlanActionStatus(record, index, stageStatus, attemptStatus)
		record["runtimeState"] = stageStatus
		records = append(records, record)
	}
	return records
}

func workflowPlanActionStatus(action map[string]any, index int, stageStatus string, attemptStatus string) string {
	switch stageStatus {
	case "passed", "done":
		return "passed"
	case "failed", "stalled":
		if index == 0 {
			return "blocked"
		}
		return "planned"
	case "needs-human", "waiting-human":
		if text(action, "type") == "human_gate" || index == 0 {
			return "waiting-human"
		}
		return "planned"
	case "running":
		if index == 0 {
			return "running"
		}
		return "planned"
	case "ready":
		if index == 0 {
			return "ready"
		}
		return "planned"
	}
	if attemptStatus == "failed" || attemptStatus == "stalled" || attemptStatus == "canceled" {
		if index == 0 {
			return "blocked"
		}
		return "planned"
	}
	if index == 0 {
		return "ready"
	}
	return "planned"
}

func workflowPlanCurrentAction(actions []map[string]any) map[string]any {
	for _, action := range actions {
		switch text(action, "status") {
		case "running", "ready", "waiting-human", "blocked":
			return action
		}
	}
	return nil
}

func workflowPlanStateRecords(workflow map[string]any, run map[string]any) []map[string]any {
	records := []map[string]any{}
	for _, state := range arrayMaps(workflow["states"]) {
		stage := workflowPlanStage(run, text(state, "id"))
		records = append(records, workflowPlanStateRecord(workflow, run, text(state, "id"), text(stage, "status")))
	}
	if len(records) > 0 {
		return records
	}
	for _, stage := range arrayMaps(run["stages"]) {
		records = append(records, workflowPlanStateRecord(workflow, run, text(stage, "id"), text(stage, "status")))
	}
	return records
}

func workflowPlanStateRecord(workflow map[string]any, run map[string]any, stateID string, status string) map[string]any {
	record := map[string]any{
		"id":          stateID,
		"status":      status,
		"actionCount": len(workflowPlanActionsForState(workflow, stateID)),
	}
	for _, state := range arrayMaps(workflow["states"]) {
		if text(state, "id") == stateID {
			record["title"] = text(state, "title")
			record["agent"] = text(state, "agent")
			record["humanGate"] = boolValue(state["humanGate"])
			return record
		}
	}
	stage := workflowPlanStage(run, stateID)
	record["title"] = text(stage, "title")
	record["agent"] = text(stage, "agentId")
	record["humanGate"] = boolValue(stage["humanGate"])
	return record
}

func workflowPlanTransitions(workflow map[string]any, stateID string) []map[string]any {
	seen := map[string]bool{}
	transitions := []map[string]any{}
	add := func(event string, to string, source string) {
		if event == "" || to == "" {
			return
		}
		key := event + "\x00" + to
		if seen[key] {
			return
		}
		seen[key] = true
		transitions = append(transitions, map[string]any{"on": event, "to": to, "source": source})
	}
	for _, transition := range arrayMaps(workflow["transitions"]) {
		if text(transition, "from") == stateID {
			add(text(transition, "on"), text(transition, "to"), "workflow.transition")
		}
	}
	for _, action := range workflowPlanActionsForState(workflow, stateID) {
		for event, to := range mapValue(action["transitions"]) {
			add(event, stringOr(to, ""), "action.transition")
		}
		for event, to := range mapValue(action["verdicts"]) {
			add(event, stringOr(to, ""), "action.verdict")
		}
	}
	return transitions
}

func workflowPlanRetryRecord(attempt map[string]any, stateID string) map[string]any {
	available := retryableAttemptStatus(text(attempt, "status"))
	record := map[string]any{
		"available":     available,
		"targetStateId": stateID,
		"action":        "retry_attempt",
		"reason":        workflowPlanRetryReason(attempt),
	}
	if available {
		policy := supervisorRecoveryPolicyForAttempt(attempt)
		record["policy"] = supervisorRecoveryPolicyMap(policy)
		record["recommendedAction"] = policy.Action
	}
	return record
}

func workflowPlanRetryReason(attempt map[string]any) string {
	if reason := text(attempt, "retryReason"); reason != "" {
		return reason
	}
	checklist := mapValue(attempt["reworkChecklist"])
	if reason := text(checklist, "retryReason"); reason != "" {
		return reason
	}
	for _, key := range []string{"failureReason", "failureReviewFeedback", "statusReason", "errorMessage", "failureDetail", "stderrSummary"} {
		if reason := text(attempt, key); reason != "" {
			return reason
		}
	}
	return "No retry reason is recorded for this attempt."
}
