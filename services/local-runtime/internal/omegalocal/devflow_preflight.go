package omegalocal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type DevFlowPreflightCheck struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type DevFlowPreflightResult struct {
	Status string                  `json:"status"`
	Checks []DevFlowPreflightCheck `json:"checks"`
	Errors []string                `json:"errors"`
}

func (result *DevFlowPreflightResult) add(id string, status string, message string) {
	result.Checks = append(result.Checks, DevFlowPreflightCheck{ID: id, Status: status, Message: message})
	if status == "failed" {
		result.Errors = append(result.Errors, message)
	}
}

func (result DevFlowPreflightResult) ok() bool {
	return len(result.Errors) == 0
}

func (server *Server) preflightDevFlowRun(ctx context.Context, database WorkspaceDatabase, item map[string]any, target map[string]any, profile ProjectAgentProfile) DevFlowPreflightResult {
	result := DevFlowPreflightResult{Status: "passed", Checks: []DevFlowPreflightCheck{}, Errors: []string{}}
	if text(item, "repositoryTargetId") == "" {
		result.add("repository-target", "failed", "Work Item has no repository target.")
	} else if target == nil {
		result.add("repository-target", "failed", fmt.Sprintf("Repository target %s was not found.", text(item, "repositoryTargetId")))
	} else if repositoryTargetCloneTarget(target) == "" {
		result.add("repository-target", "failed", fmt.Sprintf("Repository target %s has no clone target.", text(target, "id")))
	} else {
		result.add("repository-target", "passed", repositoryTargetLabel(target))
	}

	workspaceRoot := server.localWorkspaceRoot(ctx)
	if normalized, err := normalizeWorkspaceRoot(workspaceRoot); err != nil {
		result.add("workspace-root", "failed", err.Error())
	} else if err := os.MkdirAll(normalized, 0o755); err != nil {
		result.add("workspace-root", "failed", err.Error())
	} else {
		result.add("workspace-root", "passed", normalized)
	}
	if _, workspacePath, repoWorkspace, err := devFlowWorkspacePaths(ctx, server, item); err != nil {
		result.add("workspace-lifecycle", "failed", err.Error())
	} else if target != nil {
		scope := devFlowExecutionScope(item, target)
		if lock := existingExecutionLock(ctx, server, scope); lock != nil {
			result.add("workspace-lifecycle", "failed", fmt.Sprintf("Workspace is locked by attempt %s.", text(lock, "attemptId")))
		} else {
			result.add("workspace-lifecycle", "passed", fmt.Sprintf("workspace=%s repo=%s", workspacePath, repoWorkspace))
		}
	}

	if err := executableAvailable("git"); err != nil {
		result.add("git", "failed", err.Error())
	} else {
		result.add("git", "passed", "git is available.")
	}

	if target != nil && text(target, "kind") == "github" {
		if err := executableAvailable("gh"); err != nil {
			result.add("github-cli", "failed", "gh is required for GitHub PR delivery.")
		} else {
			result.add("github-cli", "passed", "gh is available.")
			for _, check := range githubDeliveryContractPreflight(ctx, target) {
				result.add(check.ID, check.Status, check.Message)
			}
		}
	}

	for _, agentID := range []string{"coding", "review"} {
		if runnerID, err := preflightAgentRunner("profile", profile, agentID); err != nil {
			result.add("runner:"+agentID, "failed", err.Error())
		} else {
			result.add("runner:"+agentID, "passed", fmt.Sprintf("%s uses %s.", agentID, runnerID))
		}
	}
	if _, validation, exists := loadProfileWorkflowTemplate(profile, "devflow-pr"); exists {
		if !validation.ok() {
			result.add("workflow-contract", "failed", strings.Join(validation.Errors, "; "))
		} else {
			result.add("workflow-contract", "passed", "agent profile workflow override")
		}
	}

	if target != nil && text(target, "kind") == "local" {
		path := repositoryTargetCloneTarget(target)
		if strings.TrimSpace(path) == "" {
			result.add("local-repository", "failed", "Local repository path is empty.")
		} else if info, err := os.Stat(path); err != nil || !info.IsDir() {
			result.add("local-repository", "failed", fmt.Sprintf("Local repository path is not readable: %s", path))
		} else if !pathExists(filepath.Join(path, ".git")) {
			result.add("local-repository", "failed", fmt.Sprintf("Local repository is not a git worktree: %s", path))
		} else if output, err := runCommand(path, "git", "status", "--short"); err != nil {
			result.add("local-repository", "failed", err.Error())
		} else if strings.TrimSpace(output) != "" {
			result.add("local-repository", "failed", "Local repository has uncommitted changes.")
		} else {
			result.add("local-repository", "passed", "Local repository is clean.")
		}
		if _, validation, exists := loadRepositoryWorkflowTemplate(path, "devflow-pr"); exists {
			if !validation.ok() {
				result.add("workflow-contract", "failed", strings.Join(validation.Errors, "; "))
			} else {
				result.add("workflow-contract", "passed", filepath.Join(path, ".omega", "WORKFLOW.md"))
			}
		}
	}

	if !result.ok() {
		result.Status = "failed"
	}
	return result
}

func githubDeliveryContractPreflight(ctx context.Context, target map[string]any) []DevFlowPreflightCheck {
	owner := strings.TrimSpace(text(target, "owner"))
	repo := strings.TrimSpace(text(target, "repo"))
	if owner == "" || repo == "" {
		return []DevFlowPreflightCheck{{
			ID:      "github-repository",
			Status:  "failed",
			Message: "GitHub repository target must include owner and repo.",
		}}
	}
	repoSlug := owner + "/" + repo
	checks := []DevFlowPreflightCheck{}
	add := func(id string, status string, message string) {
		checks = append(checks, DevFlowPreflightCheck{ID: id, Status: status, Message: message})
	}
	if output, err := exec.CommandContext(ctx, "gh", "auth", "status").CombinedOutput(); err != nil {
		add("github-auth", "failed", "gh auth status failed: "+truncateForProof(string(output), 600))
		return checks
	}
	add("github-auth", "passed", "gh is authenticated.")

	viewOutput, err := exec.CommandContext(ctx, "gh", "repo", "view", repoSlug, "--json", "nameWithOwner,viewerPermission,defaultBranchRef").CombinedOutput()
	if err != nil {
		add("github-repository", "failed", "gh repo view failed: "+truncateForProof(string(viewOutput), 600))
		return checks
	}
	var view map[string]any
	if err := json.Unmarshal(viewOutput, &view); err != nil {
		add("github-repository", "failed", "gh repo view returned invalid JSON: "+err.Error())
		return checks
	}
	add("github-repository", "passed", stringOr(text(view, "nameWithOwner"), repoSlug))
	permission := strings.ToUpper(strings.TrimSpace(text(view, "viewerPermission")))
	switch permission {
	case "ADMIN", "MAINTAIN", "WRITE":
		add("github-branch-permission", "passed", "viewerPermission="+permission+" can push delivery branches.")
		add("github-pr-create-permission", "passed", "viewerPermission="+permission+" can create pull requests.")
	default:
		if permission == "" {
			permission = "UNKNOWN"
		}
		add("github-branch-permission", "failed", "GitHub viewerPermission="+permission+" cannot be trusted for delivery branch push.")
		add("github-pr-create-permission", "failed", "GitHub viewerPermission="+permission+" cannot be trusted for PR creation.")
	}
	listOutput, err := exec.CommandContext(ctx, "gh", "pr", "list", "--repo", repoSlug, "--limit", "1", "--json", "number").CombinedOutput()
	if err != nil {
		add("github-checks-read-permission", "failed", "gh pr list/checks capability failed: "+truncateForProof(string(listOutput), 600))
	} else {
		add("github-checks-read-permission", "passed", "PR and checks metadata are readable.")
	}
	return checks
}
