package omegalocal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

func (server *Server) decomposeRequirement(response http.ResponseWriter, request *http.Request) {
	var payload struct {
		Title            string `json:"title"`
		Description      string `json:"description"`
		RepositoryTarget string `json:"repositoryTarget"`
		Source           string `json:"source"`
	}
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	title := strings.TrimSpace(payload.Title)
	description := strings.TrimSpace(payload.Description)
	if title == "" && description == "" {
		writeJSON(response, http.StatusBadRequest, map[string]any{"error": "title or description is required"})
		return
	}
	if title == "" {
		title = firstSentence(description)
	}

	writeJSON(response, http.StatusOK, decomposeRequirementPayload(title, description, strings.TrimSpace(payload.RepositoryTarget), strings.TrimSpace(payload.Source)))
}

func decomposeRequirementPayload(title string, description string, repositoryTarget string, source string) map[string]any {
	if source == "" {
		source = "manual"
	}
	criteria := []any{
		fmt.Sprintf("Requirement %q has a clear user-visible outcome.", title),
		"Implementation can be verified by automated tests or explicit proof.",
		"Delivery output includes a diff summary and human-reviewable evidence.",
	}
	if repositoryTarget != "" {
		criteria = append(criteria, "Repository target is resolved before coding starts.")
	}

	risks := []any{
		"Requirement may need additional clarification before solution design.",
		"Repository impact may be broader than the initial request suggests.",
	}
	lowerDescription := strings.ToLower(description)
	if strings.Contains(lowerDescription, "pr") || strings.Contains(lowerDescription, "github") {
		risks = append(risks, "GitHub permissions, branch policy, or CI status may block final delivery.")
	}

	stages := []struct {
		id       string
		agent    string
		title    string
		criteria string
	}{
		{"intake", "requirement", "Clarify requirement", "Structured requirement artifact is ready."},
		{"solution", "architect", "Design technical approach", "Technical plan identifies affected files, APIs, and tests."},
		{"coding", "coding", "Implement code change", "Code change is committed on an Omega-managed branch."},
		{"testing", "testing", "Verify behavior", "Required tests or proof commands have passed."},
		{"review", "review", "Review delivery quality", "Review report covers correctness, risk, and regressions."},
		{"delivery", "delivery", "Prepare GitHub delivery", "Delivery summary is ready for PR or issue update."},
	}
	items := make([]any, 0, len(stages))
	for index, stage := range stages {
		items = append(items, map[string]any{
			"id":                 fmt.Sprintf("decomposed_%s_%d", safeSegment(stage.id), index+1),
			"key":                fmt.Sprintf("REQ-%d", index+1),
			"title":              stage.title + ": " + title,
			"description":        descriptionOrDefault(description, title),
			"stageId":            stage.id,
			"assignee":           stage.agent,
			"priority":           "High",
			"status":             "Ready",
			"acceptanceCriteria": []any{stage.criteria},
			"target":             stringOr(repositoryTarget, "No target"),
			"source":             "ai_generated",
		})
	}

	return map[string]any{
		"id":                 "requirement_" + safeSegment(title),
		"summary":            title,
		"description":        descriptionOrDefault(description, title),
		"source":             source,
		"repositoryTarget":   repositoryTarget,
		"acceptanceCriteria": criteria,
		"risks":              risks,
		"assumptions": []any{
			"Omega can access the target repository locally or through GitHub.",
			"Human reviewers will approve at configured checkpoints.",
		},
		"suggestedWorkItems": items,
		"pipelineStages":     []any{"intake", "solution", "coding", "testing", "review", "delivery"},
		"createdAt":          nowISO(),
	}
}

func firstSentence(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "Untitled requirement"
	}
	for _, delimiter := range []string{".", "\n", "。"} {
		if index := strings.Index(trimmed, delimiter); index > 0 {
			return strings.TrimSpace(trimmed[:index])
		}
	}
	if len(trimmed) > 80 {
		return strings.TrimSpace(trimmed[:80])
	}
	return trimmed
}

func descriptionOrDefault(description string, title string) string {
	if strings.TrimSpace(description) != "" {
		return strings.TrimSpace(description)
	}
	return title
}
