package omegalocal

import (
	"context"
	"fmt"
	"os"
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
