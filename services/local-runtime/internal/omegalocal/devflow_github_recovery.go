package omegalocal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const devFlowGitRecoveryAgentID = "git_recovery"
const devFlowGitRecoverySandbox = "danger-full-access"

type devFlowGitHubRecoveryInput struct {
	Server              *Server
	Context             context.Context
	Template            *PipelineTemplate
	Profile             ProjectAgentProfile
	Agent               AgentProfileConfig
	Runner              AgentRunner
	RunnerID            string
	HeartbeatInterval   time.Duration
	Pipeline            map[string]any
	Item                map[string]any
	RepositoryWorkspace string
	Repository          string
	BranchName          string
	BaseBranch          string
	PullRequestTitle    string
	PullRequestBody     string
	PullRequestURL      string
	ChangedFiles        []string
	TestOutput          string
	ProofDir            string
	AttemptID           string
	StageID             string
	ActionID            string
	ActionType          string
	InitialError        error
	PromptVariables     map[string]string
}

type devFlowGitHubRecoveryResult struct {
	PullRequestURL string
	Prompt         string
	ArtifactPath   string
	PromptPath     string
	Summary        string
	Process        map[string]any
}

func devFlowGitHubRecoveryAllowed(stageID string, actionID string, actionType string, agentID string) bool {
	if agentID != devFlowGitRecoveryAgentID {
		return false
	}
	actionType = strings.TrimSpace(actionType)
	if actionType != "" && actionType != "ensure_pr" {
		return false
	}
	switch stageID + "/" + actionID {
	case "in_progress/publish_pull_request", "rework/update_pull_request":
		return true
	default:
		return false
	}
}

func runDevFlowGitHubRecoveryAgent(input devFlowGitHubRecoveryInput) (devFlowGitHubRecoveryResult, error) {
	result := devFlowGitHubRecoveryResult{}
	if !devFlowGitHubRecoveryAllowed(input.StageID, input.ActionID, stringOr(input.ActionType, "ensure_pr"), devFlowGitRecoveryAgentID) {
		return result, fmt.Errorf("git recovery agent is not allowed for stage=%s action=%s type=%s", input.StageID, input.ActionID, input.ActionType)
	}
	ctx := input.Context
	if ctx == nil {
		ctx = context.Background()
	}
	if input.Server == nil {
		return result, fmt.Errorf("git recovery agent missing runtime server")
	}
	if input.Runner == nil {
		return result, fmt.Errorf("git recovery agent runner is not configured")
	}
	artifactName := fmt.Sprintf("git-recovery-%s.md", safeSegment(stringOr(input.ActionID, input.StageID)))
	promptName := fmt.Sprintf("git-recovery-%s-prompt.md", safeSegment(stringOr(input.ActionID, input.StageID)))
	if err := os.MkdirAll(input.ProofDir, 0o755); err != nil {
		return result, err
	}
	artifactPath := filepath.Join(input.ProofDir, artifactName)
	promptPath := filepath.Join(input.ProofDir, promptName)
	prompt := buildDevFlowGitHubRecoveryPrompt(input, artifactPath)
	result.Prompt = prompt
	result.ArtifactPath = artifactPath
	result.PromptPath = promptPath
	_ = os.WriteFile(promptPath, []byte(prompt), 0o644)

	if err := writeRunnerPolicyFiles(input.RepositoryWorkspace, input.Profile, devFlowGitRecoveryAgentID); err != nil {
		process := runnerProcessNotAvailable(input.RunnerID, input.RunnerID, input.RepositoryWorkspace, err)
		process["agentId"] = devFlowGitRecoveryAgentID
		result.Process = process
		return result, err
	}
	model, credentialEnv := input.Server.runnerCredentialModelAndEnv(ctx, input.RunnerID, input.Agent.Model)
	turn := input.Runner.RunTurn(ctx, AgentTurnRequest{
		Role:              devFlowGitRecoveryAgentID,
		StageID:           input.StageID,
		Runner:            input.RunnerID,
		Workspace:         input.RepositoryWorkspace,
		Prompt:            prompt,
		OutputPath:        artifactPath,
		Sandbox:           devFlowGitRecoverySandbox,
		Model:             model,
		Effort:            "high",
		Env:               mergeEnvMaps(agentCapabilityEnv(input.Profile, devFlowGitRecoveryAgentID), credentialEnv),
		HeartbeatInterval: input.HeartbeatInterval,
		OnProcessEvent:    input.Server.runnerHeartbeatRecorder(text(input.Pipeline, "id"), text(input.Item, "id"), input.AttemptID, input.StageID, devFlowGitRecoveryAgentID, input.RunnerID),
	})
	result.Process = cloneMap(turn.Process)
	if result.Process == nil {
		result.Process = map[string]any{}
	}
	result.Process["agentId"] = devFlowGitRecoveryAgentID
	result.Process["recoveryStageId"] = input.StageID
	result.Process["recoveryActionId"] = input.ActionID
	result.Process["recoveryActionType"] = stringOr(input.ActionType, "ensure_pr")
	result.Process["initialError"] = truncateForProof(errorString(input.InitialError), 1200)

	verifiedURL, verifyErr := ensureDevFlowPullRequest(input.RepositoryWorkspace, input.Repository, input.BranchName, input.BaseBranch, input.PullRequestTitle, input.PullRequestBody)
	if verifyErr == nil && strings.TrimSpace(verifiedURL) != "" {
		result.PullRequestURL = strings.TrimSpace(verifiedURL)
		result.Process["verifiedPullRequestUrl"] = result.PullRequestURL
		result.Summary = "Git Recovery Agent repaired pull request delivery and Omega verified the PR."
		return result, nil
	}
	if turn.Error != nil {
		return result, fmt.Errorf("git recovery agent failed: %w; pull request verification failed: %v", turn.Error, verifyErr)
	}
	return result, fmt.Errorf("git recovery agent finished but pull request verification failed: %w", verifyErr)
}

func buildDevFlowGitHubRecoveryPrompt(input devFlowGitHubRecoveryInput, artifactPath string) string {
	variables := cloneStringMap(input.PromptVariables)
	variables["repository"] = input.Repository
	variables["repositoryPath"] = input.RepositoryWorkspace
	variables["workItemKey"] = text(input.Item, "key")
	variables["title"] = text(input.Item, "title")
	variables["branchName"] = input.BranchName
	variables["baseBranch"] = input.BaseBranch
	variables["pullRequestUrl"] = input.PullRequestURL
	variables["changedFiles"] = strings.Join(input.ChangedFiles, ", ")
	variables["testOutput"] = input.TestOutput
	variables["failure"] = errorString(input.InitialError)
	variables["actionId"] = input.ActionID
	variables["stageId"] = input.StageID
	variables["artifactPath"] = artifactPath
	variables["diagnostics"] = devFlowGitHubRecoveryDiagnostics(input.RepositoryWorkspace, input.BaseBranch, input.BranchName)
	fallback := fmt.Sprintf(`You are the Git Recovery Agent for Omega.

Repository: %s
Repository path: %s
Work item: %s
Title: %s
Stage/action: %s / %s
Delivery branch: %s
Base branch: %s
Current PR: %s
Changed files: %s

The runtime failed while publishing or updating the pull request:
~~~text
%s
~~~

Runtime diagnostics:
~~~text
%s
~~~

Your job is to recover Git/GitHub delivery only. You may run git and gh commands in this repository checkout to diagnose and fix branch/PR delivery, including fetch, merge-base inspection, rebase, cherry-pick, format-patch/apply, branch recreation from origin/%s, force-with-lease push of the Omega delivery branch, and PR create/update.

Hard boundaries:
- Work only in the repository path above.
- Keep using the delivery branch %s; do not publish a different branch name.
- Do not edit product source files except when resolving git conflicts needed to preserve the already produced implementation diff.
- Do not merge the PR, approve human gates, or touch another repository.
- If credentials, repository permissions, branch protection, or destructive history changes make recovery unsafe, stop and write BLOCKED.

When finished, return a recovery note in your final answer with:
- Status: recovered or blocked
- Pull request: URL if one exists
- Commands run
- What was repaired
- Remaining risk or blocker
`, input.Repository, input.RepositoryWorkspace, text(input.Item, "key"), text(input.Item, "title"), input.StageID, input.ActionID, input.BranchName, input.BaseBranch, stringOr(input.PullRequestURL, "not created yet"), strings.Join(input.ChangedFiles, ", "), errorString(input.InitialError), variables["diagnostics"], input.BaseBranch, input.BranchName)
	return renderWorkflowPromptSection(input.Template, "git_recovery", variables, fallback) + "\n\n" + agentPolicyBlock(input.Profile, devFlowGitRecoveryAgentID) + agentArtifactCaptureInstruction(artifactPath)
}

func devFlowGitHubRecoveryDiagnostics(repoWorkspace string, baseBranch string, branchName string) string {
	commands := [][]string{
		{"git", "status", "--short", "--branch"},
		{"git", "branch", "--show-current"},
		{"git", "rev-parse", "--short", "HEAD"},
		{"git", "rev-parse", "--verify", "origin/" + baseBranch},
		{"git", "merge-base", "HEAD", "origin/" + baseBranch},
		{"git", "log", "--oneline", "--decorate", "--max-count=8"},
		{"gh", "pr", "list", "--head", branchName, "--json", "url,state", "--jq", "."},
	}
	parts := []string{}
	for _, command := range commands {
		if len(command) == 0 {
			continue
		}
		output, err := runCommand(repoWorkspace, command[0], command[1:]...)
		label := strings.Join(command, " ")
		if err != nil {
			parts = append(parts, "$ "+label+"\n"+truncateForProof(err.Error(), 1200))
			continue
		}
		parts = append(parts, "$ "+label+"\n"+truncateForProof(strings.TrimSpace(output), 1200))
	}
	return strings.Join(parts, "\n\n")
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
