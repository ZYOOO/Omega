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
	PlanOutput          string
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
	todoCompletion := markdownDevFlowTodoCompletion(packet["todoCompletion"])
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
- Basis:
%s

## Recommended Actions

%s

## Plan / TODO Completion

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
		markdownDevFlowRiskBasis(mapValue(packet["risk"])["basis"]),
		markdownPacketActions(packet["recommendedActions"]),
		todoCompletion,
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
		packet := cloneMap(input.ReviewPacket)
		if _, ok := packet["todoCompletion"]; !ok {
			packet["todoCompletion"] = devFlowTodoCompletion(input, mapValue(packet["testPreview"]), mapValue(packet["checkPreview"]))
		}
		return packet
	}
	diffPreview := devFlowDiffPreview(input.ChangedFiles, input.DiffText)
	testPreview := devFlowTestPreview(input.TestOutput)
	checkPreview := devFlowCheckPreview(input.ChecksOutput, input.PullRequestFeedback, input.CheckLogFeedback)
	risk := devFlowRiskSummary(input, testPreview, checkPreview)
	actions := devFlowRecommendedActions(input, testPreview, checkPreview, risk)
	todoCompletion := devFlowTodoCompletion(input, testPreview, checkPreview)
	solutionPlan := devFlowSolutionPlanPreview(input.PlanOutput)
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
		"solutionPlan":       solutionPlan,
		"todoCompletion":     todoCompletion,
		"recommendedActions": actions,
		"reviewFeedback":     devFlowPacketReviewFeedback(input),
	}
}

func devFlowSolutionPlanPreview(plan string) map[string]any {
	plan = strings.TrimSpace(plan)
	if plan == "" {
		return map[string]any{}
	}
	functional, project := devFlowParsePlanTodos(plan)
	return map[string]any{
		"source":          "solution-plan",
		"summary":         devFlowSolutionPlanSummary(plan),
		"excerpt":         truncateForProof(plan, 2400),
		"functionalTodos": functional,
		"projectTodos":    project,
	}
}

func devFlowSolutionPlanSummary(plan string) string {
	lines := []string{}
	for _, line := range strings.Split(plan, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "- [") || strings.HasPrefix(trimmed, "* [") {
			continue
		}
		lines = append(lines, trimmed)
		if len(strings.Join(lines, " ")) >= 260 {
			break
		}
	}
	if len(lines) == 0 {
		return "Solution plan captured for Human Review."
	}
	return truncateForProof(oneLine(strings.Join(lines, " ")), 420)
}

func attachDevFlowHumanReviewBrief(reviewPacket map[string]any, path string) map[string]any {
	packet := cloneMap(reviewPacket)
	raw, err := os.ReadFile(path)
	if err != nil || strings.TrimSpace(string(raw)) == "" {
		return packet
	}
	brief := devFlowHumanReviewBriefPreview(string(raw))
	brief["sourcePath"] = path
	packet["humanReviewBrief"] = brief
	return packet
}

func devFlowHumanReviewBriefPreview(markdown string) map[string]any {
	markdown = strings.TrimSpace(markdown)
	if markdown == "" {
		return map[string]any{}
	}
	return map[string]any{
		"source":  "delivery-agent",
		"summary": devFlowSolutionPlanSummary(markdown),
		"excerpt": truncateForProof(markdown, 2200),
	}
}

func devFlowTodoCompletion(input devFlowRunReportInput, testPreview map[string]any, checkPreview map[string]any) map[string]any {
	functional, project := devFlowParsePlanTodos(input.PlanOutput)
	total := len(functional) + len(project)
	counts := map[string]any{"total": total, "verified": 0, "pending": 0, "attention": 0}
	if total == 0 {
		return map[string]any{
			"status":     "not_captured",
			"summary":    "No plan TODO checklist was captured in the solution plan.",
			"counts":     counts,
			"functional": []any{},
			"project":    []any{},
			"source":     "solution-plan",
		}
	}
	reviewStatus := devFlowLatestReviewStatus(input.AgentInvocations)
	testStatus := text(testPreview, "status")
	checkStatus := text(checkPreview, "status")
	functionalItems, verified, pending, attention := devFlowTodoCompletionItems(functional, reviewStatus, testStatus, checkStatus)
	counts["verified"] = int(counts["verified"].(int)) + verified
	counts["pending"] = int(counts["pending"].(int)) + pending
	counts["attention"] = int(counts["attention"].(int)) + attention
	projectItems, verified, pending, attention := devFlowTodoCompletionItems(project, reviewStatus, testStatus, checkStatus)
	counts["verified"] = int(counts["verified"].(int)) + verified
	counts["pending"] = int(counts["pending"].(int)) + pending
	counts["attention"] = int(counts["attention"].(int)) + attention
	status := "verified"
	if int(counts["attention"].(int)) > 0 {
		status = "attention"
	} else if int(counts["pending"].(int)) > 0 {
		status = "pending"
	}
	return map[string]any{
		"status":       status,
		"summary":      fmt.Sprintf("%d/%d plan TODO(s) verified for Human Review.", counts["verified"], total),
		"counts":       counts,
		"functional":   functionalItems,
		"project":      projectItems,
		"source":       "solution-plan + automated-review + validation/check previews",
		"reviewStatus": reviewStatus,
		"testStatus":   testStatus,
		"checkStatus":  checkStatus,
	}
}

func devFlowTodoCompletionItems(items []map[string]any, reviewStatus string, testStatus string, checkStatus string) ([]any, int, int, int) {
	output := make([]any, 0, len(items))
	verified := 0
	pending := 0
	attention := 0
	for _, item := range items {
		status, evidence := devFlowTodoItemStatus(item["checked"] == true, reviewStatus, testStatus, checkStatus)
		switch status {
		case "verified":
			verified++
		case "attention":
			attention++
		default:
			pending++
		}
		output = append(output, map[string]any{
			"text":     text(item, "text"),
			"status":   status,
			"evidence": evidence,
		})
	}
	return output, verified, pending, attention
}

func devFlowTodoItemStatus(checked bool, reviewStatus string, testStatus string, checkStatus string) (string, string) {
	if checked {
		return "verified", "The plan checklist item was already marked complete."
	}
	reviewStatus = strings.ToLower(strings.TrimSpace(reviewStatus))
	testStatus = strings.ToLower(strings.TrimSpace(testStatus))
	checkStatus = strings.ToLower(strings.TrimSpace(checkStatus))
	if strings.Contains(reviewStatus, "failed") || strings.Contains(reviewStatus, "changes") || strings.Contains(reviewStatus, "needs") || testStatus == "attention" || checkStatus == "attention" {
		return "attention", "Review, validation, or remote checks recorded an attention signal."
	}
	if (reviewStatus == "passed" || reviewStatus == "approved") && testStatus == "passed" && checkStatus == "passed" {
		return "verified", "Automated review approved the diff and validation/check previews passed."
	}
	return "pending", "Waiting for complete review, validation, and check evidence."
}

func devFlowParsePlanTodos(plan string) ([]map[string]any, []map[string]any) {
	functional := []map[string]any{}
	project := []map[string]any{}
	section := ""
	for _, line := range strings.Split(plan, "\n") {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "## ") {
			switch {
			case strings.Contains(lower, "functional") && strings.Contains(lower, "todo"):
				section = "functional"
			case strings.Contains(lower, "project") && strings.Contains(lower, "todo"):
				section = "project"
			default:
				section = ""
			}
			continue
		}
		checked, label, ok := devFlowMarkdownCheckbox(trimmed)
		if !ok || label == "" {
			continue
		}
		item := map[string]any{"text": label, "checked": checked}
		switch section {
		case "project":
			project = append(project, item)
		default:
			functional = append(functional, item)
		}
	}
	return functional, project
}

func devFlowMarkdownCheckbox(line string) (bool, string, bool) {
	if len(line) < 6 {
		return false, "", false
	}
	if !(strings.HasPrefix(line, "- [") || strings.HasPrefix(line, "* [")) || line[4] != ']' {
		return false, "", false
	}
	marker := strings.ToLower(strings.TrimSpace(line[3:4]))
	if marker != "" && marker != "x" {
		return false, "", false
	}
	return marker == "x", strings.TrimSpace(line[5:]), true
}

func devFlowLatestReviewStatus(invocations []map[string]any) string {
	status := ""
	for _, invocation := range invocations {
		if text(invocation, "agentId") != "review" {
			continue
		}
		if next := text(invocation, "status"); next != "" {
			status = next
		}
	}
	return status
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
	signals := []map[string]any{}
	if len(input.ChangedFiles) == 0 {
		signals = append(signals, devFlowRiskSignal("high", "No changed files were captured, so the reviewer cannot verify implementation scope.", "changedFiles=0", "diff"))
	}
	if len(input.ChangedFiles) >= 8 {
		signals = append(signals, devFlowRiskSignal("medium", "Large diff footprint; reviewer should inspect changed areas carefully.", fmt.Sprintf("%d changed file(s)", len(input.ChangedFiles)), "diff"))
	}
	if status := text(testPreview, "status"); status == "attention" {
		signals = append(signals, devFlowRiskSignal("high", "Validation output contains a failure or error signal.", stringOr(text(testPreview, "summary"), "Validation preview status is attention."), "validation"))
	} else if status == "missing" {
		signals = append(signals, devFlowRiskSignal("medium", "Validation output is missing; approval should confirm focused tests or documented validation.", stringOr(text(testPreview, "summary"), "No validation output captured."), "validation"))
	} else if status != "" && status != "passed" {
		signals = append(signals, devFlowRiskSignal("medium", "Validation output is not fully passed yet.", stringOr(text(testPreview, "summary"), status), "validation"))
	}
	if status := text(checkPreview, "status"); status == "attention" {
		signals = append(signals, devFlowRiskSignal("high", "Remote checks contain a failed or error signal.", stringOr(text(checkPreview, "summary"), "Remote check preview status is attention."), "checks"))
	} else if status == "missing" {
		signals = append(signals, devFlowRiskSignal("medium", "Remote check output is missing; CI state has not been confirmed in this packet.", stringOr(text(checkPreview, "summary"), "No remote check output captured."), "checks"))
	} else if status == "pending" {
		signals = append(signals, devFlowRiskSignal("medium", "Remote checks are still pending.", stringOr(text(checkPreview, "summary"), "Remote checks pending."), "checks"))
	}
	if len(input.CheckLogFeedback) > 0 {
		signals = append(signals, devFlowRiskSignal("high", "Failed check log feedback was captured.", devFlowRiskFeedbackEvidence(input.CheckLogFeedback), "check-log-feedback"))
	}
	if len(input.PullRequestFeedback) > 0 {
		level := "medium"
		reason := "PR review or comment feedback exists."
		if devFlowFeedbackHasBlockingSignal(input.PullRequestFeedback) {
			level = "high"
			reason = "PR feedback contains a blocking or requested-changes signal."
		}
		signals = append(signals, devFlowRiskSignal(level, reason, devFlowRiskFeedbackEvidence(input.PullRequestFeedback), "pull-request-feedback"))
	}
	level := "low"
	reasons := []any{}
	if len(signals) == 0 {
		reasons = append(reasons, "Diff, validation, and check previews have no blocking signal in local records.")
		return map[string]any{
			"level":   level,
			"reasons": reasons,
			"basis": []any{
				devFlowRiskSignal("low", "No blocking risk signal was found.", "changed files, validation preview, and check preview are present in local records.", "review-packet"),
			},
			"policy": "High risk requires an explicit blocker such as no reviewable diff, validation failure, failed remote checks, failed check logs, or requested changes. Missing evidence is medium risk.",
		}
	}
	for _, signal := range signals {
		if text(signal, "level") == "high" {
			level = "high"
		} else if level != "high" && text(signal, "level") == "medium" {
			level = "medium"
		}
		reasons = append(reasons, text(signal, "reason"))
	}
	return map[string]any{
		"level":   level,
		"reasons": reasons,
		"basis":   anyRiskSignals(signals),
		"policy":  "High risk requires an explicit blocker such as no reviewable diff, validation failure, failed remote checks, failed check logs, or requested changes. Missing evidence is medium risk.",
	}
}

func devFlowRiskSignal(level string, reason string, evidence string, source string) map[string]any {
	return map[string]any{
		"level":    level,
		"reason":   reason,
		"evidence": truncateForProof(strings.TrimSpace(evidence), 300),
		"source":   source,
	}
}

func anyRiskSignals(signals []map[string]any) []any {
	output := make([]any, 0, len(signals))
	for _, signal := range signals {
		output = append(output, signal)
	}
	return output
}

func devFlowRiskFeedbackEvidence(feedback []map[string]any) string {
	values := []string{}
	for _, entry := range feedback {
		label := strings.TrimSpace(strings.Join([]string{text(entry, "kind"), text(entry, "label"), text(entry, "message")}, " "))
		if label != "" {
			values = append(values, label)
		}
	}
	if len(values) == 0 {
		return "Feedback was captured without a message."
	}
	return strings.Join(values, " | ")
}

func devFlowFeedbackHasBlockingSignal(feedback []map[string]any) bool {
	for _, entry := range feedback {
		lower := strings.ToLower(strings.Join([]string{text(entry, "kind"), text(entry, "label"), text(entry, "message"), text(entry, "state"), text(entry, "status")}, " "))
		if strings.Contains(lower, "request changes") || strings.Contains(lower, "changes requested") || strings.Contains(lower, "requested_changes") || strings.Contains(lower, "block") || strings.Contains(lower, "fail") || strings.Contains(lower, "error") {
			return true
		}
	}
	return false
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

func markdownDevFlowRiskBasis(value any) string {
	basis := arrayMaps(value)
	if len(basis) == 0 {
		return "- No structured risk basis captured."
	}
	lines := []string{}
	for _, entry := range basis {
		lines = append(lines, fmt.Sprintf("- `%s` / `%s`: %s Evidence: %s", stringOr(text(entry, "level"), "unknown"), stringOr(text(entry, "source"), "unknown"), text(entry, "reason"), stringOr(text(entry, "evidence"), "not captured")))
	}
	return strings.Join(lines, "\n")
}

func markdownDevFlowTodoCompletion(value any) string {
	completion := mapValue(value)
	if len(completion) == 0 {
		return "- No plan TODO completion data captured."
	}
	lines := []string{}
	if summary := text(completion, "summary"); summary != "" {
		lines = append(lines, "- Summary: "+summary)
	}
	if status := text(completion, "status"); status != "" {
		lines = append(lines, "- Status: `"+status+"`")
	}
	counts := mapValue(completion["counts"])
	if len(counts) > 0 {
		lines = append(lines, fmt.Sprintf("- Counts: %s verified / %s total, %s pending, %s attention", text(counts, "verified"), text(counts, "total"), text(counts, "pending"), text(counts, "attention")))
	}
	appendGroup := func(title string, entries []map[string]any) {
		if len(entries) == 0 {
			return
		}
		lines = append(lines, "", "### "+title)
		for _, entry := range entries {
			lines = append(lines, fmt.Sprintf("- [%s] (%s) %s — %s", devFlowTodoMarkdownMark(text(entry, "status")), stringOr(text(entry, "status"), "pending"), text(entry, "text"), text(entry, "evidence")))
		}
	}
	appendGroup("Functional TODO", arrayMaps(completion["functional"]))
	appendGroup("Project TODO", arrayMaps(completion["project"]))
	if len(lines) == 0 {
		return "- No plan TODO completion data captured."
	}
	return strings.Join(lines, "\n")
}

func appendDevFlowTodoCompletionSection(markdown string, packet map[string]any) string {
	section := "## Plan / TODO Verification\n\n" + markdownDevFlowTodoCompletion(packet["todoCompletion"]) + "\n\n"
	if strings.Contains(markdown, "## Plan / TODO Verification") {
		return markdown
	}
	if strings.Contains(markdown, "\n## Proof") {
		return strings.Replace(markdown, "\n## Proof", "\n"+section+"## Proof", 1)
	}
	return strings.TrimRight(markdown, "\n") + "\n\n" + section
}

func devFlowTodoMarkdownMark(status string) string {
	switch status {
	case "verified":
		return "x"
	default:
		return " "
	}
}

func fencedOrFallback(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	return "```text\n" + truncateForProof(value, 4000) + "\n```"
}
