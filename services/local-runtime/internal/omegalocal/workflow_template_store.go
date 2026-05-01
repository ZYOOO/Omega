package omegalocal

import (
	"encoding/json"
	"net/http"
	"strings"
)

func workflowTemplateRecordID(projectID, repositoryTargetID, templateID string) string {
	scope := "project"
	target := ""
	if strings.TrimSpace(repositoryTargetID) != "" {
		scope = "repository"
		target = ":" + safeSegment(repositoryTargetID)
	}
	return "workflow_template:" + safeSegment(stringOr(projectID, "project_omega")) + ":" + scope + target + ":" + safeSegment(stringOr(templateID, "devflow-pr"))
}

func workflowTemplateValidationMap(result WorkflowValidationResult) map[string]any {
	return map[string]any{
		"status":   result.Status,
		"errors":   stringAnyList(result.Errors),
		"warnings": stringAnyList(result.Warnings),
	}
}

func stringAnyList(values []string) []any {
	output := make([]any, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			output = append(output, value)
		}
	}
	return output
}

func validateWorkflowMarkdownRecord(markdown, fallbackTemplateID string) (PipelineTemplate, WorkflowValidationResult) {
	template, err := parseWorkflowTemplateMarkdown(markdown, "api:workflow-template")
	if err != nil {
		return PipelineTemplate{}, WorkflowValidationResult{Status: "failed", Errors: []string{err.Error()}}
	}
	if template.ID == "" {
		template.ID = fallbackTemplateID
	}
	return template, validateWorkflowTemplate(template)
}

func upsertWorkflowTemplateRecord(database WorkspaceDatabase, record map[string]any) WorkspaceDatabase {
	if text(record, "id") == "" {
		record["id"] = workflowTemplateRecordID(text(record, "projectId"), text(record, "repositoryTargetId"), text(record, "templateId"))
	}
	if text(record, "createdAt") == "" {
		record["createdAt"] = nowISO()
	}
	record["updatedAt"] = nowISO()
	if intValue(record["version"]) <= 0 {
		record["version"] = 1
	}
	for index, existing := range database.Tables.WorkflowTemplates {
		if text(existing, "id") == text(record, "id") {
			next := cloneMap(record)
			next["createdAt"] = stringOr(text(existing, "createdAt"), text(record, "createdAt"))
			next["version"] = intValue(existing["version"]) + 1
			database.Tables.WorkflowTemplates[index] = next
			return database
		}
	}
	database.Tables.WorkflowTemplates = append(database.Tables.WorkflowTemplates, record)
	return database
}

func workflowTemplateOverride(database WorkspaceDatabase, projectID, repositoryTargetID, templateID string) map[string]any {
	targetID := strings.TrimSpace(repositoryTargetID)
	templateID = stringOr(templateID, "devflow-pr")
	for _, record := range database.Tables.WorkflowTemplates {
		if text(record, "projectId") == projectID && text(record, "repositoryTargetId") == targetID && text(record, "templateId") == templateID {
			return record
		}
	}
	if targetID != "" {
		for _, record := range database.Tables.WorkflowTemplates {
			if text(record, "projectId") == projectID && text(record, "repositoryTargetId") == "" && text(record, "templateId") == templateID {
				return record
			}
		}
	}
	return nil
}

func workflowTemplatesResponse(database *WorkspaceDatabase, projectID, repositoryTargetID string) []map[string]any {
	records := []map[string]any{}
	for _, template := range pipelineTemplates() {
		base := map[string]any{
			"id":               template.ID,
			"templateId":       template.ID,
			"name":             template.Name,
			"description":      template.Description,
			"source":           stringOr(template.Source, "default"),
			"workflowMarkdown": template.WorkflowMarkdown,
			"validation":       workflowTemplateValidationMap(validateWorkflowTemplate(template)),
			"default":          true,
		}
		records = append(records, base)
	}
	if database == nil {
		return records
	}
	for _, record := range database.Tables.WorkflowTemplates {
		if projectID != "" && text(record, "projectId") != projectID {
			continue
		}
		if repositoryTargetID != "" && text(record, "repositoryTargetId") != repositoryTargetID {
			continue
		}
		records = append(records, cloneMap(record))
	}
	return records
}

func (server *Server) listWorkflowTemplates(response http.ResponseWriter, request *http.Request) {
	projectID := request.URL.Query().Get("projectId")
	repositoryTargetID := request.URL.Query().Get("repositoryTargetId")
	database, err := server.Repo.Load(request.Context())
	if err != nil {
		writeJSON(response, http.StatusOK, workflowTemplatesResponse(nil, projectID, repositoryTargetID))
		return
	}
	if projectID == "" {
		projectID = firstProjectIDFromDatabase(*database)
	}
	writeJSON(response, http.StatusOK, workflowTemplatesResponse(database, projectID, repositoryTargetID))
}

func (server *Server) validateWorkflowTemplate(response http.ResponseWriter, request *http.Request) {
	var payload map[string]any
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	template, validation := validateWorkflowMarkdownRecord(text(payload, "markdown"), stringOr(text(payload, "templateId"), "devflow-pr"))
	writeJSON(response, http.StatusOK, map[string]any{"template": template, "validation": workflowTemplateValidationMap(validation)})
}

func (server *Server) putWorkflowTemplate(response http.ResponseWriter, request *http.Request) {
	id := strings.Trim(strings.TrimPrefix(request.URL.Path, "/workflow-templates/"), "/")
	var payload map[string]any
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	database, err := mustLoad(server, request.Context())
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	projectID := stringOr(text(payload, "projectId"), firstProjectIDFromDatabase(database))
	templateID := stringOr(text(payload, "templateId"), stringOr(id, "devflow-pr"))
	markdown := text(payload, "markdown")
	template, validation := validateWorkflowMarkdownRecord(markdown, templateID)
	if template.ID != "" {
		templateID = template.ID
	}
	recordID := id
	if recordID == "" || recordID == templateID {
		recordID = workflowTemplateRecordID(projectID, text(payload, "repositoryTargetId"), templateID)
	}
	record := map[string]any{
		"id":                 recordID,
		"scope":              map[bool]string{true: "repository", false: "project"}[strings.TrimSpace(text(payload, "repositoryTargetId")) != ""],
		"projectId":          projectID,
		"repositoryTargetId": text(payload, "repositoryTargetId"),
		"templateId":         templateID,
		"source":             "workspace",
		"markdown":           markdown,
		"workflowMarkdown":   markdown,
		"validation":         workflowTemplateValidationMap(validation),
		"parsedTemplateName": template.Name,
	}
	database = upsertWorkflowTemplateRecord(database, record)
	if err := server.Repo.Save(request.Context(), database); err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, workflowTemplateOverride(database, projectID, text(payload, "repositoryTargetId"), templateID))
}

func (server *Server) restoreWorkflowTemplateDefault(response http.ResponseWriter, request *http.Request) {
	id := strings.Trim(strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, "/workflow-templates/"), "/restore-default"), "/")
	var payload map[string]any
	_ = json.NewDecoder(request.Body).Decode(&payload)
	database, err := mustLoad(server, request.Context())
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	projectID := stringOr(text(payload, "projectId"), firstProjectIDFromDatabase(database))
	repositoryTargetID := text(payload, "repositoryTargetId")
	templateID := stringOr(text(payload, "templateId"), stringOr(id, "devflow-pr"))
	template := findPipelineTemplate(templateID)
	if template == nil {
		writeJSON(response, http.StatusNotFound, map[string]any{"error": "workflow template not found"})
		return
	}
	markdown := template.WorkflowMarkdown
	_, validation := validateWorkflowMarkdownRecord(markdown, templateID)
	record := map[string]any{
		"id":                 workflowTemplateRecordID(projectID, repositoryTargetID, templateID),
		"scope":              map[bool]string{true: "repository", false: "project"}[repositoryTargetID != ""],
		"projectId":          projectID,
		"repositoryTargetId": repositoryTargetID,
		"templateId":         templateID,
		"source":             "restored-default",
		"markdown":           markdown,
		"workflowMarkdown":   markdown,
		"validation":         workflowTemplateValidationMap(validation),
	}
	database = upsertWorkflowTemplateRecord(database, record)
	if err := server.Repo.Save(request.Context(), database); err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, workflowTemplateOverride(database, projectID, repositoryTargetID, templateID))
}
