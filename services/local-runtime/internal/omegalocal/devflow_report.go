package omegalocal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type devFlowRunReportInput struct {
	Item                map[string]any
	Repository          string
	BranchName          string
	PullRequestURL      string
	ChangedFiles        []string
	TestOutput          string
	ChecksOutput        string
	PullRequestFeedback []map[string]any
	CheckLogFeedback    []map[string]any
	StageArtifacts      []map[string]any
	AgentInvocations    []map[string]any
}

func writeDevFlowRunReport(proofDir string, input devFlowRunReportInput) (string, error) {
	path := filepath.Join(proofDir, "attempt-run-report.md")
	reviewLines := []string{}
	for _, invocation := range input.AgentInvocations {
		if text(invocation, "agentId") != "review" {
			continue
		}
		reviewLines = append(reviewLines, fmt.Sprintf("- `%s`: %s", text(invocation, "stageId"), stringOr(text(invocation, "summary"), text(invocation, "status"))))
	}
	if len(reviewLines) == 0 {
		reviewLines = append(reviewLines, "- No review verdict recorded yet.")
	}
	prFeedbackLines := []string{}
	for _, feedback := range input.PullRequestFeedback {
		if text(feedback, "message") == "" {
			continue
		}
		prFeedbackLines = append(prFeedbackLines, fmt.Sprintf("- `%s` %s: %s", text(feedback, "kind"), text(feedback, "label"), text(feedback, "message")))
	}
	if len(prFeedbackLines) == 0 {
		prFeedbackLines = append(prFeedbackLines, "- No PR review or comment feedback captured.")
	}
	checkLogLines := []string{}
	for _, feedback := range input.CheckLogFeedback {
		if text(feedback, "message") == "" {
			continue
		}
		checkLogLines = append(checkLogLines, fmt.Sprintf("- `%s`: %s", text(feedback, "label"), text(feedback, "message")))
	}
	if len(checkLogLines) == 0 {
		checkLogLines = append(checkLogLines, "- No failed check log captured.")
	}
	artifactLines := []string{}
	for _, artifact := range input.StageArtifacts {
		artifactLines = append(artifactLines, fmt.Sprintf("- `%s` / `%s`: %s", text(artifact, "stageId"), text(artifact, "agentId"), text(artifact, "artifact")))
	}
	if len(artifactLines) == 0 {
		artifactLines = append(artifactLines, "- No stage artifact recorded.")
	}
	body := fmt.Sprintf(`# Attempt Run Report

## Work Item

- Key: %s
- Title: %s
- Repository: %s
- Branch: %s
- Pull request: %s

## Requirement

%s

## Changed Files

%s

## Validation

%s

## Remote Checks

%s

## Review

%s

## Pull Request Feedback

%s

## Failed Check Logs

%s

## Artifacts

%s
`,
		text(input.Item, "key"),
		text(input.Item, "title"),
		input.Repository,
		input.BranchName,
		stringOr(input.PullRequestURL, "Not created."),
		stringOr(text(input.Item, "description"), "No description provided."),
		markdownFileList(input.ChangedFiles),
		fencedOrFallback(input.TestOutput, "No validation output."),
		fencedOrFallback(input.ChecksOutput, "No remote checks captured."),
		strings.Join(reviewLines, "\n"),
		strings.Join(prFeedbackLines, "\n"),
		strings.Join(checkLogLines, "\n"),
		strings.Join(artifactLines, "\n"),
	)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func fencedOrFallback(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	return "```text\n" + truncateForProof(value, 4000) + "\n```"
}
