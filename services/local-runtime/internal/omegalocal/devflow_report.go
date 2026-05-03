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
	DiffText            string
	TestOutput          string
	ChecksOutput        string
	PullRequestFeedback []map[string]any
	CheckLogFeedback    []map[string]any
	StageArtifacts      []map[string]any
	AgentInvocations    []map[string]any
	ReviewPacket        map[string]any
}

func writeDevFlowRunReport(proofDir string, input devFlowRunReportInput) (string, error) {
	path := filepath.Join(proofDir, "attempt-run-report.md")
	packet := ensureDevFlowReviewPacket(input)
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

## Diff Preview

- Changed files: %d
- Preview: %s

~~~diff
%s
~~~

## Test Preview

- Status: %s
- Summary: %s

## Check Preview

- Status: %s
- Summary: %s

## Risk

- Level: %s
- Reasons:
%s

## Recommended Actions

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
		len(input.ChangedFiles),
		text(mapValue(packet["diffPreview"]), "summary"),
		truncateForProof(text(mapValue(packet["diffPreview"]), "patchExcerpt"), 2400),
		text(mapValue(packet["testPreview"]), "status"),
		text(mapValue(packet["testPreview"]), "summary"),
		text(mapValue(packet["checkPreview"]), "status"),
		text(mapValue(packet["checkPreview"]), "summary"),
		text(mapValue(packet["risk"]), "level"),
		markdownAnyList(mapValue(packet["risk"])["reasons"]),
		markdownPacketActions(packet["recommendedActions"]),
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

func writeDevFlowReviewPacket(proofDir string, input devFlowRunReportInput) (map[string]any, string, error) {
	packet := ensureDevFlowReviewPacket(input)
	path := filepath.Join(proofDir, "attempt-review-packet.json")
	if err := writeJSONFile(path, packet); err != nil {
		return nil, "", err
	}
	return packet, path, nil
}

func ensureDevFlowReviewPacket(input devFlowRunReportInput) map[string]any {
	if len(input.ReviewPacket) > 0 {
		return cloneMap(input.ReviewPacket)
	}
	diffPreview := devFlowDiffPreview(input.ChangedFiles, input.DiffText)
	testPreview := devFlowTestPreview(input.TestOutput)
	checkPreview := devFlowCheckPreview(input.ChecksOutput, input.PullRequestFeedback, input.CheckLogFeedback)
	risk := devFlowRiskSummary(input, testPreview, checkPreview)
	actions := devFlowRecommendedActions(input, testPreview, checkPreview, risk)
	return map[string]any{
		"schemaVersion":      1,
		"generatedAt":        nowISO(),
		"workItemKey":        text(input.Item, "key"),
		"workItemTitle":      text(input.Item, "title"),
		"repository":         input.Repository,
		"branchName":         input.BranchName,
		"pullRequestUrl":     input.PullRequestURL,
		"summary":            devFlowPacketSummary(input, risk),
		"diffPreview":        diffPreview,
		"testPreview":        testPreview,
		"checkPreview":       checkPreview,
		"risk":               risk,
		"recommendedActions": actions,
		"reviewFeedback":     devFlowPacketReviewFeedback(input),
	}
}

func devFlowDiffPreview(changedFiles []string, diffText string) map[string]any {
	additions := strings.Count(diffText, "\n+")
	deletions := strings.Count(diffText, "\n-")
	if additions > 0 {
		additions--
	}
	if deletions > 0 {
		deletions--
	}
	summary := "No source diff captured."
	if len(changedFiles) > 0 {
		summary = fmt.Sprintf("%d changed file(s), +%d/-%d lines in captured diff.", len(changedFiles), additions, deletions)
	}
	return map[string]any{
		"changedFiles": changedFiles,
		"fileCount":    len(changedFiles),
		"additions":    additions,
		"deletions":    deletions,
		"summary":      summary,
		"patchExcerpt": truncateForProof(diffText, 8000),
	}
}

func devFlowTestPreview(testOutput string) map[string]any {
	output := strings.TrimSpace(testOutput)
	lower := strings.ToLower(output)
	status := "unknown"
	switch {
	case output == "":
		status = "missing"
	case strings.Contains(lower, "fail") || strings.Contains(lower, "error"):
		status = "attention"
	default:
		status = "passed"
	}
	return map[string]any{
		"status":        status,
		"summary":       devFlowFirstUsefulLine(output, "No validation output captured."),
		"outputExcerpt": truncateForProof(output, 3000),
	}
}

func devFlowCheckPreview(checksOutput string, pullRequestFeedback []map[string]any, checkLogFeedback []map[string]any) map[string]any {
	output := strings.TrimSpace(checksOutput)
	lower := strings.ToLower(output)
	status := "unknown"
	switch {
	case output == "":
		status = "missing"
	case strings.Contains(lower, "fail") || strings.Contains(lower, "error") || len(checkLogFeedback) > 0:
		status = "attention"
	case strings.Contains(lower, "pending") || strings.Contains(lower, "queued") || strings.Contains(lower, "in progress"):
		status = "pending"
	default:
		status = "passed"
	}
	return map[string]any{
		"status":              status,
		"summary":             devFlowFirstUsefulLine(output, "No remote check output captured."),
		"outputExcerpt":       truncateForProof(output, 3000),
		"pullRequestFeedback": pullRequestFeedback,
		"checkLogFeedback":    checkLogFeedback,
	}
}

func devFlowRiskSummary(input devFlowRunReportInput, testPreview map[string]any, checkPreview map[string]any) map[string]any {
	level := "low"
	reasons := []any{}
	if len(input.ChangedFiles) == 0 {
		level = "high"
		reasons = append(reasons, "No changed files were captured for review.")
	}
	if len(input.ChangedFiles) >= 8 {
		level = "medium"
		reasons = append(reasons, "Large diff footprint; reviewer should inspect changed areas carefully.")
	}
	if status := text(testPreview, "status"); status == "missing" || status == "attention" {
		level = "high"
		reasons = append(reasons, "Validation output is missing or needs attention.")
	}
	if status := text(checkPreview, "status"); status == "missing" || status == "attention" {
		if level != "high" {
			level = "medium"
		}
		reasons = append(reasons, "Remote check output is missing or needs attention.")
	}
	if len(input.CheckLogFeedback) > 0 || len(input.PullRequestFeedback) > 0 {
		if level != "high" {
			level = "medium"
		}
		reasons = append(reasons, "PR review, comment, or failed check feedback exists.")
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "Diff, validation, and check previews have no blocking signal in local records.")
	}
	return map[string]any{"level": level, "reasons": reasons}
}

func devFlowRecommendedActions(input devFlowRunReportInput, testPreview map[string]any, checkPreview map[string]any, risk map[string]any) []any {
	actions := []any{}
	if input.PullRequestURL != "" {
		actions = append(actions, map[string]any{"type": "open-pr", "label": "Review the pull request diff and discussion.", "url": input.PullRequestURL})
	}
	if text(testPreview, "status") != "passed" {
		actions = append(actions, map[string]any{"type": "validation", "label": "Confirm validation output or run focused tests before approval."})
	}
	if text(checkPreview, "status") != "passed" {
		actions = append(actions, map[string]any{"type": "checks", "label": "Inspect remote checks and failed check log feedback."})
	}
	if text(risk, "level") == "high" {
		actions = append(actions, map[string]any{"type": "risk", "label": "Treat this packet as high risk until blockers are resolved."})
	}
	if len(actions) == 0 {
		actions = append(actions, map[string]any{"type": "human-review", "label": "Approve if the visible behavior and PR diff match the requirement."})
	}
	return actions
}

func devFlowPacketReviewFeedback(input devFlowRunReportInput) []any {
	feedback := []any{}
	for _, entry := range input.PullRequestFeedback {
		if text(entry, "message") != "" {
			feedback = append(feedback, entry)
		}
	}
	for _, entry := range input.CheckLogFeedback {
		if text(entry, "message") != "" {
			feedback = append(feedback, entry)
		}
	}
	return feedback
}

func devFlowPacketSummary(input devFlowRunReportInput, risk map[string]any) string {
	return fmt.Sprintf("%s has %d changed file(s), risk %s, PR %s.", text(input.Item, "key"), len(input.ChangedFiles), text(risk, "level"), stringOr(input.PullRequestURL, "not created"))
}

func devFlowFirstUsefulLine(output string, fallback string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return truncateForProof(line, 240)
		}
	}
	return fallback
}

func markdownPacketActions(value any) string {
	actions := arrayMaps(value)
	if len(actions) == 0 {
		return "- none\n"
	}
	lines := []string{}
	for _, action := range actions {
		label := stringOr(text(action, "label"), text(action, "type"))
		if url := text(action, "url"); url != "" {
			label += " " + url
		}
		lines = append(lines, "- "+label)
	}
	return strings.Join(lines, "\n")
}

func fencedOrFallback(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	return "```text\n" + truncateForProof(value, 4000) + "\n```"
}
