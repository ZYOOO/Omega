package omegalocal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func (server *Server) githubCreatePR(response http.ResponseWriter, request *http.Request) {
	var payload struct {
		WorkspacePath   string   `json:"workspacePath"`
		RepositoryPath  string   `json:"repositoryPath"`
		RepositoryOwner string   `json:"repositoryOwner"`
		RepositoryName  string   `json:"repositoryName"`
		Title           string   `json:"title"`
		Body            string   `json:"body"`
		BranchName      string   `json:"branchName"`
		BaseBranch      string   `json:"baseBranch"`
		Draft           bool     `json:"draft"`
		ChangedFiles    []string `json:"changedFiles"`
	}
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	repositoryPath := strings.TrimSpace(payload.RepositoryPath)
	if repositoryPath == "" && strings.TrimSpace(payload.WorkspacePath) != "" {
		repositoryPath = filepath.Join(strings.TrimSpace(payload.WorkspacePath), "repo")
	}
	if repositoryPath == "" {
		writeJSON(response, http.StatusBadRequest, map[string]any{"error": "repositoryPath or workspacePath is required"})
		return
	}
	title := strings.TrimSpace(payload.Title)
	if title == "" {
		writeJSON(response, http.StatusBadRequest, map[string]any{"error": "title is required"})
		return
	}
	branch := strings.TrimSpace(payload.BranchName)
	if branch == "" {
		writeJSON(response, http.StatusBadRequest, map[string]any{"error": "branchName is required"})
		return
	}
	base := strings.TrimSpace(payload.BaseBranch)
	if base == "" {
		base = "main"
	}

	body := strings.TrimSpace(payload.Body)
	if body == "" {
		body = githubPRBodyFromProof(payload.WorkspacePath, payload.ChangedFiles)
	}
	bodyPath := filepath.Join(os.TempDir(), "omega-pr-body-"+safeSegment(title)+".md")
	if err := os.WriteFile(bodyPath, []byte(body), 0o644); err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}

	args := []string{"pr", "create", "--title", title, "--body-file", bodyPath, "--head", branch, "--base", base}
	if payload.RepositoryOwner != "" && payload.RepositoryName != "" {
		args = append(args, "--repo", payload.RepositoryOwner+"/"+payload.RepositoryName)
	}
	if payload.Draft {
		args = append(args, "--draft")
	}
	command := exec.CommandContext(request.Context(), "gh", args...)
	command.Dir = repositoryPath
	output, err := command.CombinedOutput()
	if err != nil {
		writeJSON(response, http.StatusBadGateway, map[string]any{"error": "gh pr create failed", "output": string(output)})
		return
	}
	url := strings.TrimSpace(string(output))
	writeJSON(response, http.StatusOK, map[string]any{
		"status":         "created",
		"url":            url,
		"title":          title,
		"branchName":     branch,
		"baseBranch":     base,
		"repositoryPath": repositoryPath,
		"bodyPath":       bodyPath,
	})
}

func (server *Server) githubPRStatus(response http.ResponseWriter, request *http.Request) {
	var payload struct {
		RepositoryOwner string `json:"repositoryOwner"`
		RepositoryName  string `json:"repositoryName"`
		Number          int    `json:"number"`
		URL             string `json:"url"`
	}
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	repo := strings.TrimSpace(payload.RepositoryOwner + "/" + payload.RepositoryName)
	if payload.RepositoryOwner == "" || payload.RepositoryName == "" {
		repo = ""
	}
	selector := strings.TrimSpace(payload.URL)
	if selector == "" && payload.Number > 0 {
		selector = fmt.Sprint(payload.Number)
	}
	if selector == "" {
		writeJSON(response, http.StatusBadRequest, map[string]any{"error": "number or url is required"})
		return
	}

	viewArgs := []string{"pr", "view", selector, "--json", "number,title,state,mergeable,reviewDecision,headRefName,baseRefName,url"}
	if repo != "" {
		viewArgs = append(viewArgs, "--repo", repo)
	}
	viewOutput, err := exec.CommandContext(request.Context(), "gh", viewArgs...).CombinedOutput()
	if err != nil {
		writeJSON(response, http.StatusBadGateway, map[string]any{"error": "gh pr view failed", "output": string(viewOutput)})
		return
	}
	var pr map[string]any
	if err := json.Unmarshal(viewOutput, &pr); err != nil {
		writeError(response, http.StatusBadGateway, err)
		return
	}

	checkArgs := []string{"pr", "checks", selector, "--json", "name,state,link"}
	if repo != "" {
		checkArgs = append(checkArgs, "--repo", repo)
	}
	checkOutput, err := exec.CommandContext(request.Context(), "gh", checkArgs...).CombinedOutput()
	if err != nil {
		writeJSON(response, http.StatusBadGateway, map[string]any{"error": "gh pr checks failed", "output": string(checkOutput)})
		return
	}
	var checks []map[string]any
	if err := json.Unmarshal(checkOutput, &checks); err != nil {
		writeError(response, http.StatusBadGateway, err)
		return
	}

	deliveryGate := githubDeliveryGate(pr, checks)
	proofs := githubPRProofRecords(pr, checks)
	result := cloneMap(pr)
	result["checks"] = checks
	result["deliveryGate"] = deliveryGate
	result["proofRecords"] = proofs
	writeJSON(response, http.StatusOK, result)
}

func githubDeliveryGate(pr map[string]any, checks []map[string]any) string {
	if text(pr, "state") == "MERGED" || text(pr, "state") == "CLOSED" {
		return "closed"
	}
	for _, check := range checks {
		state := strings.ToUpper(text(check, "state"))
		if state != "SUCCESS" && state != "COMPLETED" {
			return "pending"
		}
	}
	if len(checks) == 0 {
		return "pending"
	}
	if text(pr, "reviewDecision") == "APPROVED" || text(pr, "reviewDecision") == "" {
		return "ready"
	}
	return "pending"
}

func githubPRProofRecords(pr map[string]any, checks []map[string]any) []map[string]any {
	records := []map[string]any{{
		"id":        fmt.Sprintf("github_pr_%v", pr["number"]),
		"label":     "pull-request",
		"value":     fmt.Sprintf("#%v %s", pr["number"], text(pr, "title")),
		"sourceUrl": text(pr, "url"),
		"status":    text(pr, "state"),
	}}
	for _, check := range checks {
		records = append(records, map[string]any{
			"id":        "github_check_" + safeSegment(text(check, "name")),
			"label":     "check",
			"value":     text(check, "name"),
			"sourceUrl": text(check, "link"),
			"status":    text(check, "state"),
		})
	}
	return records
}

func githubPRBodyFromProof(workspacePath string, changedFiles []string) string {
	var builder strings.Builder
	if strings.TrimSpace(workspacePath) != "" {
		summaryPath := filepath.Join(strings.TrimSpace(workspacePath), ".omega", "proof", "change-summary.md")
		if raw, err := os.ReadFile(summaryPath); err == nil && len(raw) > 0 {
			builder.Write(raw)
			builder.WriteString("\n\n")
		}
	}
	if builder.Len() == 0 {
		builder.WriteString("# Omega Delivery\n\nGenerated by Omega Mission Control.\n\n")
	}
	if len(changedFiles) > 0 {
		builder.WriteString("## Changed files\n\n")
		for _, file := range changedFiles {
			builder.WriteString(fmt.Sprintf("- `%s`\n", file))
		}
		builder.WriteString("\n")
	}
	builder.WriteString("## Review notes\n\n- Verify generated diff and proof records before merge.\n")
	return builder.String()
}
