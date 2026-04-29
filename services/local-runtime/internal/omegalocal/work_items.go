package omegalocal

import (
	"fmt"
	"strings"
)

func appendWorkItem(database WorkspaceDatabase, item map[string]any) WorkspaceDatabase {
	timestamp := nowISO()
	normalized := normalizeWorkItem(item)
	normalized["id"] = uniqueWorkItemID(database, text(normalized, "id"))
	normalized["key"] = uniqueWorkItemKey(database, text(normalized, "key"))
	database, normalized = ensureRequirementForWorkItem(database, normalized, timestamp)
	record := cloneMap(normalized)
	record["projectId"] = firstProjectID(database)
	record["createdAt"] = timestamp
	record["updatedAt"] = timestamp
	database.Tables.WorkItems = append(database.Tables.WorkItems, record)
	for index, state := range database.Tables.MissionControlStates {
		nextState := cloneMap(state)
		nextState["workItems"] = append(arrayMaps(nextState["workItems"]), normalized)
		nextState["updatedAt"] = timestamp
		database.Tables.MissionControlStates[index] = nextState
	}
	touch(&database)
	return database
}

func canDeleteWorkItem(database WorkspaceDatabase, item map[string]any) (bool, string) {
	status := text(item, "status")
	if status != "Ready" && status != "Backlog" {
		return false, "only not-started work items can be deleted"
	}
	itemID := text(item, "id")
	if itemID == "" {
		return false, "work item id is missing"
	}
	for _, record := range database.Tables.Pipelines {
		if recordReferencesWorkItem(record, itemID) {
			return false, "work item has pipeline history"
		}
	}
	for _, record := range database.Tables.Attempts {
		if recordReferencesWorkItem(record, itemID) {
			return false, "work item has attempt history"
		}
	}
	for _, record := range database.Tables.Checkpoints {
		if recordReferencesWorkItem(record, itemID) {
			return false, "work item has checkpoint history"
		}
	}
	for _, record := range database.Tables.Missions {
		if recordReferencesWorkItem(record, itemID) {
			return false, "work item has mission history"
		}
	}
	for _, record := range database.Tables.Operations {
		if recordReferencesWorkItem(record, itemID) {
			return false, "work item has operation history"
		}
	}
	for _, record := range database.Tables.ProofRecords {
		if recordReferencesWorkItem(record, itemID) {
			return false, "work item has proof history"
		}
	}
	return true, ""
}

func deleteWorkItemRecord(database WorkspaceDatabase, itemID string) (WorkspaceDatabase, bool, string) {
	item := findWorkItem(database, itemID)
	if item == nil {
		return database, false, "work item not found"
	}
	if ok, reason := canDeleteWorkItem(database, item); !ok {
		return database, false, reason
	}
	requirementID := text(item, "requirementId")
	nextItems := make([]map[string]any, 0, len(database.Tables.WorkItems))
	for _, candidate := range database.Tables.WorkItems {
		if text(candidate, "id") != itemID {
			nextItems = append(nextItems, candidate)
		}
	}
	database.Tables.WorkItems = nextItems
	for index, state := range database.Tables.MissionControlStates {
		nextState := cloneMap(state)
		items := arrayMaps(nextState["workItems"])
		nextStateItems := make([]map[string]any, 0, len(items))
		for _, candidate := range items {
			if text(candidate, "id") != itemID {
				nextStateItems = append(nextStateItems, candidate)
			}
		}
		nextState["workItems"] = nextStateItems
		nextState["updatedAt"] = nowISO()
		database.Tables.MissionControlStates[index] = nextState
	}
	if requirementID != "" && !workItemRequirementStillReferenced(database.Tables.WorkItems, requirementID) {
		requirements := make([]map[string]any, 0, len(database.Tables.Requirements))
		for _, requirement := range database.Tables.Requirements {
			if text(requirement, "id") != requirementID {
				requirements = append(requirements, requirement)
			}
		}
		database.Tables.Requirements = requirements
	}
	touch(&database)
	return database, true, ""
}

func recordReferencesWorkItem(record map[string]any, itemID string) bool {
	for _, key := range []string{"workItemId", "itemId", "sourceWorkItemId"} {
		if text(record, key) == itemID {
			return true
		}
	}
	return false
}

func workItemRequirementStillReferenced(items []map[string]any, requirementID string) bool {
	for _, item := range items {
		if text(item, "requirementId") == requirementID {
			return true
		}
	}
	return false
}

func normalizeRequirementLinks(database WorkspaceDatabase) WorkspaceDatabase {
	timestamp := nowISO()
	for index, item := range database.Tables.WorkItems {
		database, item = ensureRequirementForWorkItem(database, item, timestamp)
		database.Tables.WorkItems[index] = item
	}
	for stateIndex, state := range database.Tables.MissionControlStates {
		nextState := cloneMap(state)
		items := arrayMaps(nextState["workItems"])
		for itemIndex, item := range items {
			database, item = ensureRequirementForWorkItem(database, item, timestamp)
			items[itemIndex] = item
		}
		nextState["workItems"] = items
		database.Tables.MissionControlStates[stateIndex] = nextState
	}
	return database
}

func ensureRequirementForWorkItem(database WorkspaceDatabase, item map[string]any, timestamp string) (WorkspaceDatabase, map[string]any) {
	nextItem := normalizeWorkItem(item)
	if text(nextItem, "requirementId") != "" {
		requirementIndex := findByID(database.Tables.Requirements, text(nextItem, "requirementId"))
		if requirementIndex >= 0 {
			generated := requirementFromWorkItem(database, nextItem, timestamp)
			database.Tables.Requirements[requirementIndex] = enrichRequirementRecord(database.Tables.Requirements[requirementIndex], generated, timestamp)
			return database, nextItem
		}
	}

	requirement := requirementFromWorkItem(database, nextItem, timestamp)
	if existingID := findRequirementID(database, requirement); existingID != "" {
		nextItem["requirementId"] = existingID
		if requirementIndex := findByID(database.Tables.Requirements, existingID); requirementIndex >= 0 {
			database.Tables.Requirements[requirementIndex] = enrichRequirementRecord(database.Tables.Requirements[requirementIndex], requirement, timestamp)
		}
		return database, nextItem
	}
	nextItem["requirementId"] = text(requirement, "id")
	database.Tables.Requirements = append(database.Tables.Requirements, requirement)
	return database, nextItem
}

func enrichRequirementRecord(existing map[string]any, generated map[string]any, timestamp string) map[string]any {
	next := cloneMap(existing)
	changed := false
	for _, key := range []string{"projectId", "repositoryTargetId", "source", "sourceExternalRef", "rawText"} {
		if text(next, key) == "" && text(generated, key) != "" {
			next[key] = generated[key]
			changed = true
		}
	}
	if len(anySlice(next["acceptanceCriteria"])) == 0 && len(anySlice(generated["acceptanceCriteria"])) > 0 {
		next["acceptanceCriteria"] = generated["acceptanceCriteria"]
		changed = true
	}
	if len(anySlice(next["risks"])) == 0 && len(anySlice(generated["risks"])) > 0 {
		next["risks"] = generated["risks"]
		changed = true
	}
	structured := mapValue(next["structured"])
	generatedStructured := mapValue(generated["structured"])
	for _, key := range []string{"summary", "sourceWorkItemKey", "repositoryTargetId", "sourceExternalRef", "initialExecutorHint", "masterAgentId", "dispatchStatus", "dispatchPlan", "suggestedWorkItems", "assumptions"} {
		if isEmptyStructuredValue(structured[key]) && !isEmptyStructuredValue(generatedStructured[key]) {
			structured[key] = generatedStructured[key]
			changed = true
		}
	}
	if text(structured, "masterAgentId") == "" {
		structured["masterAgentId"] = "master"
		changed = true
	}
	if text(structured, "dispatchStatus") == "" {
		structured["dispatchStatus"] = "ready"
		changed = true
	}
	next["structured"] = structured
	if changed {
		next["updatedAt"] = timestamp
	}
	return next
}

func isEmptyStructuredValue(value any) bool {
	if value == nil {
		return true
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) == ""
	case []any:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	default:
		return false
	}
}

func requirementFromWorkItem(database WorkspaceDatabase, item map[string]any, timestamp string) map[string]any {
	source := stringOr(text(item, "source"), "manual")
	externalRef := text(item, "sourceExternalRef")
	repositoryTargetID := text(item, "repositoryTargetId")
	idSource := text(item, "id")
	if externalRef != "" {
		idSource = externalRef
	}
	if idSource == "" {
		idSource = text(item, "title")
	}
	requirementID := text(item, "requirementId")
	if requirementID == "" {
		requirementID = fmt.Sprintf("req_%s", safeSegment(idSource))
	}
	rawText := text(item, "description")
	if rawText == "" {
		rawText = text(item, "title")
	}
	decomposition := decomposeRequirementPayload(
		stringOr(text(item, "title"), "Untitled requirement"),
		rawText,
		text(item, "target"),
		source,
	)
	criteria := anySlice(decomposition["acceptanceCriteria"])
	if len(criteria) == 0 {
		criteria = anySlice(item["acceptanceCriteria"])
	}
	risks := anySlice(decomposition["risks"])
	structured := map[string]any{
		"summary":             stringOr(text(item, "title"), "Untitled requirement"),
		"sourceWorkItemKey":   text(item, "key"),
		"repositoryTargetId":  repositoryTargetID,
		"sourceExternalRef":   externalRef,
		"initialExecutorHint": text(item, "assignee"),
		"masterAgentId":       "master",
		"dispatchStatus":      "ready",
		"dispatchPlan": map[string]any{
			"templateId":          "feature",
			"repositoryTargetId":  repositoryTargetID,
			"repositoryTarget":    text(item, "target"),
			"stageOrder":          decomposition["pipelineStages"],
			"assignedBy":          "master",
			"requiresHumanReview": true,
		},
		"suggestedWorkItems": decomposition["suggestedWorkItems"],
		"assumptions":        decomposition["assumptions"],
	}
	return map[string]any{
		"id":                 requirementID,
		"projectId":          firstProjectID(database),
		"repositoryTargetId": repositoryTargetID,
		"source":             source,
		"sourceExternalRef":  externalRef,
		"title":              stringOr(text(item, "title"), "Untitled requirement"),
		"rawText":            rawText,
		"structured":         structured,
		"acceptanceCriteria": criteria,
		"risks":              risks,
		"status":             "converted",
		"createdAt":          timestamp,
		"updatedAt":          timestamp,
	}
}

func findRequirementID(database WorkspaceDatabase, requirement map[string]any) string {
	if ref := text(requirement, "sourceExternalRef"); ref != "" {
		for _, candidate := range database.Tables.Requirements {
			if text(candidate, "sourceExternalRef") == ref && text(candidate, "source") == text(requirement, "source") {
				return text(candidate, "id")
			}
		}
	}
	id := text(requirement, "id")
	if findByID(database.Tables.Requirements, id) < 0 {
		return ""
	}
	for suffix := 2; ; suffix++ {
		next := fmt.Sprintf("%s_%d", id, suffix)
		if findByID(database.Tables.Requirements, next) < 0 {
			requirement["id"] = next
			return ""
		}
	}
}

func normalizeWorkItem(item map[string]any) map[string]any {
	next := cloneMap(item)
	if text(next, "source") == "" {
		next["source"] = "manual"
	}
	if _, ok := next["acceptanceCriteria"]; !ok || arrayLength(next["acceptanceCriteria"]) == 0 {
		next["acceptanceCriteria"] = []any{"Request is described clearly", "Human can verify the result"}
	}
	if _, ok := next["blockedByItemIds"]; !ok {
		next["blockedByItemIds"] = []any{}
	}
	if text(next, "team") == "" {
		next["team"] = "Omega"
	}
	if text(next, "stageId") == "" {
		next["stageId"] = "intake"
	}
	if text(next, "target") == "" {
		next["target"] = "No target"
	}
	return next
}

func uniqueWorkItemID(database WorkspaceDatabase, candidate string) string {
	base := strings.TrimSpace(candidate)
	if base == "" {
		base = fmt.Sprintf("item_manual_%d", len(database.Tables.WorkItems)+1)
	}
	used := map[string]bool{}
	for _, item := range database.Tables.WorkItems {
		if id := text(item, "id"); id != "" {
			used[id] = true
		}
	}
	for _, state := range database.Tables.MissionControlStates {
		for _, item := range arrayMaps(state["workItems"]) {
			if id := text(item, "id"); id != "" {
				used[id] = true
			}
		}
	}
	if !used[base] {
		return base
	}
	for suffix := 2; ; suffix++ {
		next := fmt.Sprintf("%s_%d", base, suffix)
		if !used[next] {
			return next
		}
	}
}

func uniqueWorkItemKey(database WorkspaceDatabase, candidate string) string {
	base := strings.TrimSpace(candidate)
	if base == "" {
		base = nextOmegaWorkItemKey(database)
	}
	used := usedWorkItemKeys(database)
	if !used[base] {
		return base
	}
	if strings.HasPrefix(base, "OMG-") {
		for next := nextOmegaWorkItemNumber(database); ; next++ {
			key := fmt.Sprintf("OMG-%d", next)
			if !used[key] {
				return key
			}
		}
	}
	for suffix := 2; ; suffix++ {
		key := fmt.Sprintf("%s-%d", base, suffix)
		if !used[key] {
			return key
		}
	}
}

func usedWorkItemKeys(database WorkspaceDatabase) map[string]bool {
	used := map[string]bool{}
	for _, item := range database.Tables.WorkItems {
		if key := text(item, "key"); key != "" {
			used[key] = true
		}
	}
	for _, state := range database.Tables.MissionControlStates {
		for _, item := range arrayMaps(state["workItems"]) {
			if key := text(item, "key"); key != "" {
				used[key] = true
			}
		}
	}
	return used
}

func nextOmegaWorkItemKey(database WorkspaceDatabase) string {
	return fmt.Sprintf("OMG-%d", nextOmegaWorkItemNumber(database))
}

func nextOmegaWorkItemNumber(database WorkspaceDatabase) int {
	maxNumber := 0
	for key := range usedWorkItemKeys(database) {
		var number int
		if _, err := fmt.Sscanf(key, "OMG-%d", &number); err == nil && number > maxNumber {
			maxNumber = number
		}
	}
	return maxNumber + 1
}

func updateWorkItem(database WorkspaceDatabase, itemID string, patch map[string]any) WorkspaceDatabase {
	timestamp := nowISO()
	for index, item := range database.Tables.WorkItems {
		if text(item, "id") == itemID {
			next := cloneMap(item)
			for key, value := range patch {
				next[key] = value
			}
			next["updatedAt"] = timestamp
			database.Tables.WorkItems[index] = next
		}
	}
	for stateIndex, state := range database.Tables.MissionControlStates {
		nextState := cloneMap(state)
		items := arrayMaps(nextState["workItems"])
		for itemIndex, item := range items {
			if text(item, "id") == itemID {
				for key, value := range patch {
					item[key] = value
				}
				items[itemIndex] = item
			}
		}
		nextState["workItems"] = items
		nextState["updatedAt"] = timestamp
		database.Tables.MissionControlStates[stateIndex] = nextState
	}
	touch(&database)
	return database
}
