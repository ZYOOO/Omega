package omegalocal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
)

func upsertRunWorkpad(database *WorkspaceDatabase, attemptID string) map[string]any {
	if database == nil || attemptID == "" {
		return nil
	}
	attemptIndex := findByID(database.Tables.Attempts, attemptID)
	if attemptIndex < 0 {
		return nil
	}
	attempt := cloneMap(database.Tables.Attempts[attemptIndex])
	pipelineIndex := findByID(database.Tables.Pipelines, text(attempt, "pipelineId"))
	var pipeline map[string]any
	if pipelineIndex >= 0 {
		pipeline = cloneMap(database.Tables.Pipelines[pipelineIndex])
	}
	item := findWorkItem(*database, text(attempt, "itemId"))
	requirement := workpadRequirement(*database, item)
	timestamp := nowISO()
	recordID := fmt.Sprintf("%s:workpad", attemptID)
	createdAt := timestamp
	fieldPatches := map[string]any{}
	fieldPatchSources := map[string]any{}
	fieldPatchHistory := []map[string]any{}
	if existingIndex := findByID(database.Tables.RunWorkpads, recordID); existingIndex >= 0 {
		createdAt = stringOr(database.Tables.RunWorkpads[existingIndex]["createdAt"], timestamp)
		fieldPatches = mapValue(database.Tables.RunWorkpads[existingIndex]["fieldPatches"])
		fieldPatchSources = mapValue(database.Tables.RunWorkpads[existingIndex]["fieldPatchSources"])
		fieldPatchHistory = arrayMaps(database.Tables.RunWorkpads[existingIndex]["fieldPatchHistory"])
	}
	reworkChecklist := buildReworkChecklist(*database, pipeline, attempt)
	workpad := map[string]any{
		"plan":               workpadPlan(pipeline, attempt),
		"acceptanceCriteria": workpadAcceptanceCriteria(item, requirement),
		"validation":         workpadValidation(*database, pipeline, attempt),
		"notes":              workpadNotes(attempt),
		"blockers":           workpadBlockers(*database, pipeline, attempt),
		"pr":                 workpadPullRequest(attempt),
		"reviewFeedback":     workpadReviewFeedback(*database, pipeline, attempt),
		"reviewPacket":       workpadReviewPacket(attempt),
		"retryReason":        workpadRetryReason(attempt, reworkChecklist),
		"reworkChecklist":    reworkChecklist,
		"reworkAssessment":   mapValue(attempt["reworkAssessment"]),
		"updatedBy":          "runtime",
	}
	workpad = applyRunWorkpadFieldPatches(workpad, fieldPatches)
	record := map[string]any{
		"id":                 recordID,
		"attemptId":          attemptID,
		"pipelineId":         text(attempt, "pipelineId"),
		"workItemId":         text(attempt, "itemId"),
		"repositoryTargetId": text(attempt, "repositoryTargetId"),
		"status":             text(attempt, "status"),
		"workpad":            workpad,
		"fieldPatches":       fieldPatches,
		"fieldPatchSources":  fieldPatchSources,
		"fieldPatchHistory":  fieldPatchHistory,
		"createdAt":          createdAt,
		"updatedAt":          timestamp,
	}
	database.Tables.RunWorkpads = appendOrReplace(database.Tables.RunWorkpads, record)
	return record
}

func upsertLatestRunWorkpadForPipeline(database *WorkspaceDatabase, pipelineID string) map[string]any {
	if database == nil || pipelineID == "" {
		return nil
	}
	if attemptIndex := latestAttemptIndexForPipeline(*database, pipelineID); attemptIndex >= 0 {
		return upsertRunWorkpad(database, text(database.Tables.Attempts[attemptIndex], "id"))
	}
	return nil
}

func (server *Server) patchRunWorkpad(response http.ResponseWriter, request *http.Request) {
	recordID, err := url.PathUnescape(pathID(request.URL.Path))
	if err != nil || strings.TrimSpace(recordID) == "" {
		writeJSON(response, http.StatusBadRequest, map[string]any{"error": "run workpad id is required"})
		return
	}
	var payload struct {
		Workpad   map[string]any `json:"workpad"`
		UpdatedBy string         `json:"updatedBy"`
		Reason    string         `json:"reason"`
		Source    map[string]any `json:"source"`
	}
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	database, err := mustLoad(server, request.Context())
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	index := findByID(database.Tables.RunWorkpads, recordID)
	if index < 0 {
		writeJSON(response, http.StatusNotFound, map[string]any{"error": "run workpad not found"})
		return
	}
	updatedBy := normalizeRunWorkpadPatchActor(payload.UpdatedBy)
	if updatedBy == "" {
		writeJSON(response, http.StatusBadRequest, map[string]any{"error": "invalid workpad patch actor", "allowed": runWorkpadPatchActors()})
		return
	}
	patch, invalid := sanitizeRunWorkpadPatch(payload.Workpad, updatedBy)
	if len(invalid) > 0 {
		writeJSON(response, http.StatusBadRequest, map[string]any{"error": "invalid workpad fields", "fields": invalid})
		return
	}
	if len(patch) == 0 && strings.TrimSpace(payload.Reason) == "" && len(payload.Source) == 0 {
		writeJSON(response, http.StatusBadRequest, map[string]any{"error": "workpad patch is empty"})
		return
	}
	record := cloneMap(database.Tables.RunWorkpads[index])
	fieldPatches := mapValue(record["fieldPatches"])
	fieldPatches = mergeRunWorkpadMaps(fieldPatches, patch)
	workpad := applyRunWorkpadFieldPatches(mapValue(record["workpad"]), fieldPatches)
	workpad["updatedBy"] = updatedBy
	fieldPatches["updatedBy"] = updatedBy
	timestamp := nowISO()
	source := sanitizeRunWorkpadPatchSource(payload.Source, updatedBy, payload.Reason, timestamp)
	fieldPatchSources := mapValue(record["fieldPatchSources"])
	patchedFields := sortedRunWorkpadPatchFields(patch)
	for _, field := range patchedFields {
		if field == "updatedBy" {
			continue
		}
		fieldPatchSources[field] = source
	}
	fieldPatchHistory := appendRunWorkpadPatchHistory(arrayMaps(record["fieldPatchHistory"]), recordID, updatedBy, patchedFields, source, payload.Reason, timestamp)
	record["workpad"] = workpad
	record["fieldPatches"] = fieldPatches
	record["fieldPatchSources"] = fieldPatchSources
	record["fieldPatchHistory"] = fieldPatchHistory
	record["updatedAt"] = timestamp
	database.Tables.RunWorkpads[index] = record
	if err := server.Repo.Save(request.Context(), database); err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, record)
}

func sanitizeRunWorkpadPatch(input map[string]any, updatedBy string) (map[string]any, []string) {
	allowed := map[string]bool{
		"plan":               true,
		"acceptanceCriteria": true,
		"validation":         true,
		"notes":              true,
		"blockers":           true,
		"pr":                 true,
		"reviewFeedback":     true,
		"reviewPacket":       true,
		"retryReason":        true,
		"reworkChecklist":    true,
		"reworkAssessment":   true,
		"updatedBy":          true,
	}
	patch := map[string]any{}
	invalid := []string{}
	for key, value := range input {
		if !allowed[key] || !runWorkpadActorCanPatchField(updatedBy, key) {
			invalid = append(invalid, key)
			continue
		}
		patch[key] = value
	}
	return patch, invalid
}

func normalizeRunWorkpadPatchActor(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "operator"
	}
	if runWorkpadPatchActors()[value] {
		return value
	}
	return ""
}

func runWorkpadPatchActors() map[string]bool {
	return map[string]bool{
		"operator":       true,
		"agent":          true,
		"job-supervisor": true,
		"human-review":   true,
		"review-agent":   true,
		"delivery-agent": true,
		"test":           true,
	}
}

func runWorkpadActorCanPatchField(actor string, field string) bool {
	if field == "updatedBy" {
		return true
	}
	agentFields := map[string]bool{
		"plan": true, "acceptanceCriteria": true, "validation": true, "notes": true, "blockers": true,
		"pr": true, "reviewFeedback": true, "retryReason": true, "reworkChecklist": true, "reworkAssessment": true,
		"reviewPacket": true,
	}
	operatorFields := map[string]bool{
		"validation": true, "notes": true, "blockers": true, "reviewFeedback": true,
		"retryReason": true, "reworkChecklist": true, "reworkAssessment": true,
	}
	supervisorFields := map[string]bool{
		"validation": true, "notes": true, "blockers": true, "pr": true, "reviewFeedback": true,
		"reviewPacket": true, "retryReason": true, "reworkChecklist": true, "reworkAssessment": true,
	}
	switch actor {
	case "agent", "review-agent", "delivery-agent", "test":
		return agentFields[field]
	case "job-supervisor":
		return supervisorFields[field]
	case "operator", "human-review":
		return operatorFields[field]
	default:
		return false
	}
}

func sanitizeRunWorkpadPatchSource(input map[string]any, updatedBy string, reason string, timestamp string) map[string]any {
	source := map[string]any{
		"kind":      "api",
		"updatedBy": updatedBy,
		"updatedAt": timestamp,
	}
	for _, key := range []string{"kind", "id", "label", "url", "attemptId", "pipelineId", "operationId", "checkpointId", "proofId"} {
		if value := strings.TrimSpace(stringOr(input[key], "")); value != "" {
			source[key] = value
		}
	}
	if trimmed := strings.TrimSpace(reason); trimmed != "" {
		source["reason"] = trimmed
	}
	return source
}

func sortedRunWorkpadPatchFields(patch map[string]any) []string {
	fields := []string{}
	for key := range patch {
		fields = append(fields, key)
	}
	sort.Strings(fields)
	return fields
}

func appendRunWorkpadPatchHistory(history []map[string]any, recordID string, updatedBy string, fields []string, source map[string]any, reason string, timestamp string) []map[string]any {
	entry := map[string]any{
		"id":        fmt.Sprintf("%s:patch:%d", recordID, len(history)+1),
		"updatedAt": timestamp,
		"updatedBy": updatedBy,
		"fields":    fields,
		"source":    source,
	}
	if trimmed := strings.TrimSpace(reason); trimmed != "" {
		entry["reason"] = trimmed
	}
	history = append(history, entry)
	if len(history) > 100 {
		history = history[len(history)-100:]
	}
	return history
}

func applyRunWorkpadFieldPatches(workpad map[string]any, patches map[string]any) map[string]any {
	return mergeRunWorkpadMaps(cloneMap(workpad), patches)
}

func mergeRunWorkpadMaps(base map[string]any, patch map[string]any) map[string]any {
	output := cloneMap(base)
	for key, value := range patch {
		if value == nil {
			continue
		}
		switch key {
		case "plan", "validation", "pr", "reworkChecklist", "reworkAssessment":
			output[key] = mergeGenericMap(mapValue(output[key]), mapValue(value))
		default:
			output[key] = value
		}
	}
	return output
}

func mergeGenericMap(base map[string]any, patch map[string]any) map[string]any {
	output := cloneMap(base)
	for key, value := range patch {
		if value == nil {
			continue
		}
		output[key] = value
	}
	return output
}

func workpadRequirement(database WorkspaceDatabase, item map[string]any) map[string]any {
	if item == nil {
		return nil
	}
	requirementID := text(item, "requirementId")
	if requirementID == "" {
		return nil
	}
	if index := findByID(database.Tables.Requirements, requirementID); index >= 0 {
		return database.Tables.Requirements[index]
	}
	return nil
}

func workpadPlan(pipeline map[string]any, attempt map[string]any) map[string]any {
	stages := []map[string]any{}
	if pipeline != nil {
		for _, stage := range arrayMaps(mapValue(pipeline["run"])["stages"]) {
			stages = append(stages, map[string]any{
				"id":      text(stage, "id"),
				"title":   text(stage, "title"),
				"status":  text(stage, "status"),
				"agents":  stringSlice(stage["agentIds"]),
				"started": text(stage, "startedAt"),
				"done":    text(stage, "completedAt"),
			})
		}
	}
	return map[string]any{
		"pipelineStatus": stringOr(text(pipeline, "status"), "unknown"),
		"currentStageId": text(attempt, "currentStageId"),
		"stageCount":     len(stages),
		"stages":         stages,
	}
}

func workpadAcceptanceCriteria(item map[string]any, requirement map[string]any) []string {
	values := stringSlice(nil)
	if requirement != nil {
		values = append(values, stringSlice(requirement["acceptanceCriteria"])...)
	}
	if len(values) == 0 && item != nil {
		values = append(values, stringSlice(item["acceptanceCriteria"])...)
	}
	if len(values) == 0 {
		values = append(values, "Human reviewer can verify the requested change against the original requirement.")
	}
	return compactStringList(values)
}

func workpadValidation(database WorkspaceDatabase, pipeline map[string]any, attempt map[string]any) map[string]any {
	proofs := workpadProofSummaries(database, text(pipeline, "id"), "test")
	status := "pending"
	if text(attempt, "status") == "done" {
		status = "passed"
	}
	if text(attempt, "status") == "failed" || text(attempt, "status") == "stalled" || text(attempt, "status") == "canceled" {
		status = "needs-attention"
	}
	return map[string]any{
		"status":     status,
		"proofCount": len(proofs),
		"proofs":     proofs,
	}
}

func workpadNotes(attempt map[string]any) []string {
	notes := []string{}
	for _, event := range arrayMaps(attempt["events"]) {
		message := strings.TrimSpace(text(event, "message"))
		if message == "" {
			continue
		}
		notes = append(notes, message)
	}
	return compactStringList(notes)
}

func workpadBlockers(database WorkspaceDatabase, pipeline map[string]any, attempt map[string]any) []string {
	blockers := []string{}
	for _, value := range []string{text(attempt, "failureReason"), text(attempt, "statusReason"), text(attempt, "errorMessage"), text(attempt, "failureDetail")} {
		if strings.TrimSpace(value) != "" {
			blockers = append(blockers, value)
		}
	}
	if status := text(pipeline, "status"); status == "failed" || status == "stalled" || status == "canceled" {
		blockers = append(blockers, "Pipeline is "+status+".")
	}
	for _, checkpoint := range database.Tables.Checkpoints {
		if text(checkpoint, "pipelineId") != text(attempt, "pipelineId") {
			continue
		}
		if status := text(checkpoint, "status"); status == "rejected" || status == "canceled" || status == "pending" {
			blockers = append(blockers, strings.TrimSpace(text(checkpoint, "title")+" "+text(checkpoint, "decisionNote")))
		}
	}
	return compactStringList(blockers)
}

func workpadPullRequest(attempt map[string]any) map[string]any {
	return map[string]any{
		"url":       text(attempt, "pullRequestUrl"),
		"branch":    text(attempt, "branchName"),
		"workspace": text(attempt, "workspacePath"),
	}
}

func workpadReviewFeedback(database WorkspaceDatabase, pipeline map[string]any, attempt map[string]any) []string {
	feedback := []string{}
	if value := strings.TrimSpace(text(attempt, "humanChangeRequest")); value != "" {
		feedback = append(feedback, value)
	}
	if value := strings.TrimSpace(text(attempt, "failureReviewFeedback")); value != "" {
		feedback = append(feedback, value)
	}
	for _, entry := range arrayMaps(attempt["pullRequestFeedback"]) {
		if message := strings.TrimSpace(text(entry, "message")); message != "" {
			feedback = append(feedback, strings.TrimSpace(text(entry, "label")+": "+message))
		}
	}
	for _, entry := range arrayMaps(attempt["checkLogFeedback"]) {
		if message := strings.TrimSpace(text(entry, "message")); message != "" {
			feedback = append(feedback, strings.TrimSpace(text(entry, "label")+": "+truncateForProof(message, 600)))
		}
	}
	if value := latestHumanChangeRequestFromPipeline(pipeline); value != "" {
		feedback = append(feedback, value)
	}
	for _, proof := range workpadProofSummaries(database, text(pipeline, "id"), "review") {
		feedback = append(feedback, stringOr(proof["label"], stringOr(proof["path"], "")))
	}
	for _, operation := range database.Tables.Operations {
		if !strings.Contains(text(operation, "stageId"), "review") {
			continue
		}
		if summary := strings.TrimSpace(text(operation, "summary")); summary != "" {
			feedback = append(feedback, summary)
		}
	}
	return compactStringList(feedback)
}

func workpadReviewPacket(attempt map[string]any) map[string]any {
	packet := mapValue(attempt["reviewPacket"])
	if len(packet) == 0 {
		return map[string]any{}
	}
	return packet
}

func workpadRetryReason(attempt map[string]any, reworkChecklist map[string]any) string {
	if attempt == nil {
		return ""
	}
	if value := strings.TrimSpace(text(reworkChecklist, "retryReason")); value != "" {
		return truncateForProof(value, 1200)
	}
	for _, value := range []string{text(attempt, "retryReason"), text(attempt, "humanChangeRequest"), text(attempt, "failureReason"), text(attempt, "failureReviewFeedback"), text(attempt, "statusReason"), text(attempt, "errorMessage"), text(attempt, "failureDetail"), text(attempt, "stderrSummary")} {
		if strings.TrimSpace(value) != "" {
			return truncateForProof(value, 1200)
		}
	}
	if status := text(attempt, "status"); status == "failed" || status == "stalled" || status == "canceled" {
		return "Attempt is " + status + " and needs a retry decision."
	}
	return ""
}

func workpadProofSummaries(database WorkspaceDatabase, pipelineID string, kind string) []map[string]any {
	operationIDs := map[string]bool{}
	for _, operation := range database.Tables.Operations {
		operationID := text(operation, "id")
		if pipelineID != "" && !strings.Contains(operationID, pipelineID) {
			continue
		}
		if kind != "" {
			haystack := strings.ToLower(text(operation, "stageId") + " " + text(operation, "agentId") + " " + text(operation, "summary"))
			if !strings.Contains(haystack, strings.ToLower(kind)) {
				continue
			}
		}
		operationIDs[operationID] = true
	}
	proofs := []map[string]any{}
	for _, proof := range database.Tables.ProofRecords {
		if len(operationIDs) > 0 && !operationIDs[text(proof, "operationId")] {
			continue
		}
		if kind != "" {
			haystack := strings.ToLower(text(proof, "label") + " " + text(proof, "value") + " " + text(proof, "sourcePath"))
			if !strings.Contains(haystack, strings.ToLower(kind)) {
				continue
			}
		}
		proofs = append(proofs, map[string]any{
			"id":    text(proof, "id"),
			"label": text(proof, "label"),
			"value": text(proof, "value"),
			"path":  filepath.Base(text(proof, "sourcePath")),
		})
	}
	return proofs
}

func filterRunWorkpads(records []map[string]any, query url.Values) []map[string]any {
	filters := map[string]string{
		"attemptId":          query.Get("attemptId"),
		"pipelineId":         query.Get("pipelineId"),
		"workItemId":         query.Get("workItemId"),
		"repositoryTargetId": query.Get("repositoryTargetId"),
		"status":             query.Get("status"),
	}
	filtered := []map[string]any{}
	for _, record := range records {
		matched := true
		for key, value := range filters {
			if value == "" {
				continue
			}
			if text(record, key) != value {
				matched = false
				break
			}
		}
		if matched {
			filtered = append(filtered, record)
		}
	}
	return filtered
}

func compactStringList(values []string) []string {
	output := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		output = append(output, value)
	}
	return output
}
