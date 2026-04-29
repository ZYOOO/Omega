package omegalocal

import (
	"context"
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
		RepositoryOwner string   `json:"repositoryOwner"`
		RepositoryName  string   `json:"repositoryName"`
		RepositoryPath  string   `json:"repositoryPath"`
		WorkspacePath   string   `json:"workspacePath"`
		RequiredChecks  []string `json:"requiredChecks"`
		Number          int      `json:"number"`
		URL             string   `json:"url"`
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

	checkSummary := githubCheckSummaryWithRequired(checks, payload.RequiredChecks)
	deliveryGate := githubDeliveryGateForSummary(pr, checks, checkSummary)
	proofs := githubPRProofRecords(pr, checks)
	repositoryPath := strings.TrimSpace(payload.RepositoryPath)
	if repositoryPath == "" && strings.TrimSpace(payload.WorkspacePath) != "" {
		repositoryPath = filepath.Join(strings.TrimSpace(payload.WorkspacePath), "repo")
	}
	branchSync := githubBranchSyncStatus(request.Context(), repositoryPath, text(pr, "baseRefName"), text(pr, "headRefName"))
	actions := githubDeliveryRecommendedActions(pr, checkSummary, branchSync)
	result := cloneMap(pr)
	result["checks"] = checks
	result["checkSummary"] = checkSummary
	result["deliveryGate"] = deliveryGate
	result["proofRecords"] = proofs
	result["branchSync"] = branchSync
	result["recommendedActions"] = actions
	writeJSON(response, http.StatusOK, result)
}

func githubDeliveryGate(pr map[string]any, checks []map[string]any) string {
	return githubDeliveryGateForSummary(pr, checks, githubCheckSummary(checks))
}

func githubDeliveryGateForSummary(pr map[string]any, checks []map[string]any, summary map[string]any) string {
	if text(pr, "state") == "MERGED" || text(pr, "state") == "CLOSED" {
		return "closed"
	}
	if intValue(summary["failed"]) > 0 || intValue(summary["missingRequired"]) > 0 {
		return "blocked"
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

func githubCheckSummary(checks []map[string]any) map[string]any {
	return githubCheckSummaryWithRequired(checks, nil)
}

func githubCheckSummaryWithRequired(checks []map[string]any, requiredChecks []string) map[string]any {
	summary := map[string]any{
		"total":                 len(checks),
		"passed":                0,
		"pending":               0,
		"failed":                0,
		"neutral":               0,
		"failedChecks":          []map[string]any{},
		"pendingChecks":         []map[string]any{},
		"requiredChecks":        requiredChecks,
		"missingRequired":       0,
		"missingRequiredChecks": []map[string]any{},
	}
	seen := map[string]bool{}
	for _, check := range checks {
		seen[strings.ToLower(strings.TrimSpace(text(check, "name")))] = true
		state := strings.ToUpper(strings.TrimSpace(text(check, "state")))
		switch state {
		case "SUCCESS", "COMPLETED", "PASSED":
			summary["passed"] = intValue(summary["passed"]) + 1
		case "FAILURE", "FAILED", "ERROR", "CANCELLED", "CANCELED", "TIMED_OUT", "ACTION_REQUIRED":
			summary["failed"] = intValue(summary["failed"]) + 1
			summary["failedChecks"] = append(arrayMaps(summary["failedChecks"]), check)
		case "SKIPPED", "NEUTRAL":
			summary["neutral"] = intValue(summary["neutral"]) + 1
		default:
			summary["pending"] = intValue(summary["pending"]) + 1
			summary["pendingChecks"] = append(arrayMaps(summary["pendingChecks"]), check)
		}
	}
	for _, name := range requiredChecks {
		normalized := strings.ToLower(strings.TrimSpace(name))
		if normalized == "" || seen[normalized] {
			continue
		}
		missing := map[string]any{"name": strings.TrimSpace(name), "state": "MISSING"}
		summary["missingRequired"] = intValue(summary["missingRequired"]) + 1
		summary["missingRequiredChecks"] = append(arrayMaps(summary["missingRequiredChecks"]), missing)
	}
	return summary
}

func githubDeliveryRecommendedActions(pr map[string]any, checkSummary map[string]any, branchSync map[string]any) []map[string]any {
	actions := []map[string]any{}
	if intValue(checkSummary["failed"]) > 0 {
		actions = append(actions, map[string]any{"type": "checks-failed", "label": "Inspect failed CI checks and route back to rework", "count": intValue(checkSummary["failed"])})
	}
	if intValue(checkSummary["missingRequired"]) > 0 {
		actions = append(actions, map[string]any{"type": "required-checks-missing", "label": "Configure or wait for required checks", "count": intValue(checkSummary["missingRequired"])})
	}
	if intValue(checkSummary["pending"]) > 0 {
		actions = append(actions, map[string]any{"type": "checks-pending", "label": "Wait for pending CI checks", "count": intValue(checkSummary["pending"])})
	}
	switch text(branchSync, "status") {
	case "behind":
		actions = append(actions, map[string]any{"type": "branch-sync", "label": "Rebase or sync PR branch with base branch", "count": 1})
	case "conflict":
		actions = append(actions, map[string]any{"type": "merge-conflict", "label": "Resolve merge conflicts before delivery", "count": 1})
	}
	if text(pr, "reviewDecision") != "" && text(pr, "reviewDecision") != "APPROVED" {
		actions = append(actions, map[string]any{"type": "review", "label": "Address PR review decision before delivery", "count": 1})
	}
	return actions
}

func githubBranchSyncStatus(ctx context.Context, repositoryPath string, baseRef string, headRef string) map[string]any {
	status := map[string]any{"status": "unknown"}
	if strings.TrimSpace(repositoryPath) == "" {
		status["reason"] = "repositoryPath not provided"
		return status
	}
	if strings.TrimSpace(baseRef) == "" || strings.TrimSpace(headRef) == "" {
		status["reason"] = "baseRefName or headRefName missing"
		return status
	}
	_, _ = exec.CommandContext(ctx, "git", "-C", repositoryPath, "fetch", "--quiet", "origin", baseRef, headRef).CombinedOutput()
	baseRemote := "origin/" + baseRef
	headRemote := "origin/" + headRef
	ancestor := exec.CommandContext(ctx, "git", "-C", repositoryPath, "merge-base", "--is-ancestor", baseRemote, headRemote)
	if err := ancestor.Run(); err == nil {
		status["status"] = "current"
		status["baseRefName"] = baseRef
		status["headRefName"] = headRef
		return status
	}
	mergeBaseOutput, err := exec.CommandContext(ctx, "git", "-C", repositoryPath, "merge-base", baseRemote, headRemote).CombinedOutput()
	if err != nil {
		status["status"] = "unknown"
		status["reason"] = "merge-base failed: " + strings.TrimSpace(string(mergeBaseOutput))
		return status
	}
	mergeBase := strings.TrimSpace(string(mergeBaseOutput))
	treeOutput, _ := exec.CommandContext(ctx, "git", "-C", repositoryPath, "merge-tree", mergeBase, baseRemote, headRemote).CombinedOutput()
	status["status"] = "behind"
	status["baseRefName"] = baseRef
	status["headRefName"] = headRef
	status["mergeBase"] = mergeBase
	if strings.Contains(string(treeOutput), "<<<<<<<") || strings.Contains(strings.ToLower(string(treeOutput)), "changed in both") {
		status["status"] = "conflict"
		status["conflictSummary"] = truncateForProof(string(treeOutput), 4000)
	}
	return status
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
