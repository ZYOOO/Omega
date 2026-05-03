package omegalocal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type approvedDevFlowDeliveryActionHandler struct {
	server        *Server
	database      *WorkspaceDatabase
	pipeline      map[string]any
	attempt       map[string]any
	item          map[string]any
	workspace     string
	repoWorkspace string
	proofDir      string
	prURL         string
	reviewer      string
	branchName    string

	humanReviewPath string
	mergePath       string
	handoffPath     string
}

func newApprovedDevFlowDeliveryActionHandler(input approvedDevFlowDeliveryActionHandler) *approvedDevFlowDeliveryActionHandler {
	input.humanReviewPath = filepath.Join(input.proofDir, "human-review.md")
	input.mergePath = filepath.Join(input.proofDir, "merge.md")
	input.handoffPath = filepath.Join(input.proofDir, "handoff-bundle.json")
	input.branchName = text(input.attempt, "branchName")
	return &input
}

func (handler *approvedDevFlowDeliveryActionHandler) humanReviewProofPath() string {
	return handler.humanReviewPath
}

func (handler *approvedDevFlowDeliveryActionHandler) mergeProofPath() string {
	return handler.mergePath
}

func (handler *approvedDevFlowDeliveryActionHandler) handoffProofPath() string {
	return handler.handoffPath
}

func (handler *approvedDevFlowDeliveryActionHandler) recordHumanDecision() error {
	return os.WriteFile(handler.humanReviewPath, []byte(fmt.Sprintf("# Human Review\n\n- Reviewer: %s\n- Decision: approved\n- Pull request: %s\n- Approved at: %s\n", handler.reviewer, handler.prURL, nowISO())), 0o644)
}

func (handler *approvedDevFlowDeliveryActionHandler) refreshPullRequestStatus() error {
	checksOutput, _ := runCommand(handler.repoWorkspace, "gh", "pr", "checks", handler.prURL)
	statusPath := filepath.Join(handler.proofDir, "merge-pr-status.md")
	statusReport := fmt.Sprintf("# PR Status Before Merge\n\n- Pull request: %s\n- Refreshed at: %s\n\n```text\n%s\n```\n", handler.prURL, nowISO(), stringOr(strings.TrimSpace(checksOutput), "No check output was returned."))
	if err := os.WriteFile(statusPath, []byte(statusReport), 0o644); err != nil {
		return err
	}
	handler.server.logInfo(context.Background(), "github.pr.status_refreshed", "Pull request status refreshed before merge.", map[string]any{"pipelineId": text(handler.pipeline, "id"), "attemptId": text(handler.attempt, "id"), "pullRequestUrl": handler.prURL})
	return nil
}

func (handler *approvedDevFlowDeliveryActionHandler) mergePullRequest() error {
	if err := handler.server.mergeApprovedDevFlowPullRequest(handler.repoWorkspace, handler.prURL, handler.branchName, text(handler.pipeline, "id"), text(handler.attempt, "id")); err != nil {
		handler.server.logError(context.Background(), "github.pr.merge_failed", err.Error(), map[string]any{"pipelineId": text(handler.pipeline, "id"), "attemptId": text(handler.attempt, "id"), "pullRequestUrl": handler.prURL})
		if handler.item != nil {
			report := handler.server.syncGitHubIssueOutbound(context.Background(), githubOutboundSyncInput{
				RepositoryPath: handler.repoWorkspace,
				Repository:     repositoryFromPullRequestURL(handler.prURL),
				WorkItem:       handler.item,
				Pipeline:       handler.pipeline,
				Attempt:        handler.attempt,
				Event:          "delivery.merge_failed",
				Status:         "failed",
				StageID:        "merging",
				Summary:        "Human review approved, but pull request merge failed.",
				PullRequestURL: handler.prURL,
				BranchName:     handler.branchName,
				FailureReason:  "Pull request merge failed after human approval.",
				FailureDetail:  err.Error(),
			})
			if text(report, "state") != "skipped" {
				_ = writeJSONFile(filepath.Join(handler.proofDir, "github-outbound-sync-merge-failed.json"), report)
			}
		}
		return fmt.Errorf("merge pull request after human approval: %w", err)
	}
	handler.server.logInfo(context.Background(), "github.pr.merged", "Pull request merged after human approval.", map[string]any{"pipelineId": text(handler.pipeline, "id"), "attemptId": text(handler.attempt, "id"), "pullRequestUrl": handler.prURL})
	return os.WriteFile(handler.mergePath, []byte(fmt.Sprintf("# Merge\n\nMerged after human approval: %s\n", handler.prURL)), 0o644)
}

func (handler *approvedDevFlowDeliveryActionHandler) writeHandoff() error {
	if raw, err := os.ReadFile(handler.handoffPath); err == nil {
		var handoff map[string]any
		if json.Unmarshal(raw, &handoff) == nil {
			handoff["merged"] = true
			handoff["humanGate"] = "approved"
			handoff["approvedBy"] = handler.reviewer
			handoff["approvedAt"] = nowISO()
			return writeJSONFile(handler.handoffPath, handoff)
		}
	}
	return writeJSONFile(handler.handoffPath, map[string]any{
		"pipelineId":       text(handler.pipeline, "id"),
		"attemptId":        text(handler.attempt, "id"),
		"workspacePath":    handler.workspace,
		"pullRequestUrl":   handler.prURL,
		"branchName":       handler.branchName,
		"merged":           true,
		"humanGate":        "approved",
		"approvedBy":       handler.reviewer,
		"approvedAt":       nowISO(),
		"handoffGenerated": "approval-continuation",
	})
}
