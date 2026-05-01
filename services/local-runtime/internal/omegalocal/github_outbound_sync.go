package omegalocal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const githubOutboundSyncMarker = "<!-- omega:github-outbound-sync -->"
const githubPullRequestOutboundSyncMarker = "<!-- omega:github-pr-outbound-sync -->"

type githubOutboundSyncInput struct {
	RepositoryPath      string
	Repository          string
	WorkItem            map[string]any
	Pipeline            map[string]any
	Attempt             map[string]any
	AttemptID           string
	Event               string
	Status              string
	StageID             string
	Summary             string
	PullRequestURL      string
	BranchName          string
	ChangedFiles        []string
	ChecksOutput        string
	PullRequestFeedback []map[string]any
	CheckLogFeedback    []map[string]any
	ReviewPacket        map[string]any
	FailureReason       string
	FailureDetail       string
}

func (server *Server) syncGitHubIssueOutbound(ctx context.Context, input githubOutboundSyncInput) map[string]any {
	repo, number, ok := githubIssueRefFromWorkItem(input.WorkItem, input.Repository)
	report := map[string]any{
		"event":           input.Event,
		"status":          input.Status,
		"pipelineId":      text(input.Pipeline, "id"),
		"attemptId":       stringOr(input.AttemptID, text(input.Attempt, "id")),
		"workItemId":      text(input.WorkItem, "id"),
		"createdAt":       nowISO(),
		"issueSynced":     false,
		"prSynced":        false,
		"commentPosted":   false,
		"prCommentPosted": false,
		"labelsSynced":    false,
	}

	repositoryPath := strings.TrimSpace(input.RepositoryPath)
	if repositoryPath == "" {
		repositoryPath = "."
	}
	if ok {
		report["repository"] = repo
		report["issueNumber"] = number
		_, bodyPath, err := githubIssueOutboundCommentBody(input)
		if err != nil {
			report["commentError"] = err.Error()
			server.logError(ctx, "github.issue.outbound_sync_failed", err.Error(), githubOutboundSyncLogFields(input, report))
		} else {
			defer os.Remove(bodyPath)
			if output, err := runGitHubOutboundCommand(ctx, repositoryPath, "issue", "comment", strconv.Itoa(number), "--repo", repo, "--body-file", bodyPath); err != nil {
				report["commentError"] = truncateForProof(output+"\n"+err.Error(), 1200)
			} else {
				report["commentPosted"] = true
				report["commentOutput"] = truncateForProof(output, 800)
			}
		}
	} else {
		report["issueSkipped"] = true
		report["issueSkipReason"] = "work item is not linked to a GitHub issue"
	}

	if ok {
		labels := githubIssueOutboundLabels(input.Status, input.Event)
		labelErrors := []string{}
		for _, label := range githubOutboundLabelCatalog() {
			if output, err := runGitHubOutboundCommand(ctx, repositoryPath, "label", "create", label.name, "--repo", repo, "--color", label.color, "--description", label.description, "--force"); err != nil {
				labelErrors = append(labelErrors, truncateForProof(output+"\n"+err.Error(), 600))
			}
		}
		removeLabels := strings.Join(labels.remove, ",")
		addNames := []string{}
		for _, label := range labels.add {
			addNames = append(addNames, label.name)
		}
		editArgs := []string{"issue", "edit", strconv.Itoa(number), "--repo", repo}
		if len(addNames) > 0 {
			editArgs = append(editArgs, "--add-label", strings.Join(addNames, ","))
		}
		if removeLabels != "" {
			editArgs = append(editArgs, "--remove-label", removeLabels)
		}
		if output, err := runGitHubOutboundCommand(ctx, repositoryPath, editArgs...); err != nil {
			labelErrors = append(labelErrors, truncateForProof(output+"\n"+err.Error(), 1200))
		} else {
			report["labelOutput"] = truncateForProof(output, 800)
		}
		if len(labelErrors) == 0 {
			report["labelsSynced"] = true
		} else {
			report["labelErrors"] = labelErrors
		}
	}

	if prReport := server.syncGitHubPullRequestOutbound(ctx, repositoryPath, input); len(prReport) > 0 {
		report["pullRequest"] = prReport
		report["prSynced"] = text(prReport, "state") == "synced"
		report["prCommentPosted"] = boolValue(prReport["commentPosted"])
	}
	if ciReport := server.triggerGitHubCIIfConfigured(ctx, repositoryPath, input); len(ciReport) > 0 {
		report["ciTrigger"] = ciReport
	}

	report["issueSynced"] = boolValue(report["commentPosted"]) && boolValue(report["labelsSynced"])
	if boolValue(report["issueSynced"]) && (input.PullRequestURL == "" || boolValue(report["prSynced"])) {
		report["state"] = "synced"
		server.logInfo(ctx, "github.issue.outbound_synced", "GitHub issue comment and labels synced.", githubOutboundSyncLogFields(input, report))
	} else if boolValue(report["issueSynced"]) || boolValue(report["prSynced"]) || boolValue(report["commentPosted"]) || boolValue(report["labelsSynced"]) {
		report["state"] = "partial"
		server.logError(ctx, "github.issue.outbound_partial", "GitHub issue outbound sync partially completed.", githubOutboundSyncLogFields(input, report))
	} else {
		if !ok && input.PullRequestURL == "" {
			report["state"] = "skipped"
			report["reason"] = "work item is not linked to a GitHub issue and no pull request URL is available"
		} else {
			report["state"] = "failed"
			server.logError(ctx, "github.issue.outbound_failed", "GitHub issue outbound sync failed.", githubOutboundSyncLogFields(input, report))
		}
	}
	return report
}

func (server *Server) syncGitHubPullRequestOutbound(ctx context.Context, repositoryPath string, input githubOutboundSyncInput) map[string]any {
	repo, number, ok := githubPullRequestRefFromURL(input.PullRequestURL)
	report := map[string]any{
		"state":         "skipped",
		"commentPosted": false,
		"createdAt":     nowISO(),
	}
	if !ok {
		report["reason"] = "pull request URL is missing or unsupported"
		return report
	}
	report["repository"] = repo
	report["pullRequestNumber"] = number
	_, bodyPath, err := githubPullRequestOutboundCommentBody(input)
	if err != nil {
		report["state"] = "failed"
		report["commentError"] = err.Error()
		return report
	}
	defer os.Remove(bodyPath)
	output, err := runGitHubOutboundCommand(ctx, repositoryPath, "pr", "comment", input.PullRequestURL, "--body-file", bodyPath, "--edit-last", "--create-if-none")
	if err != nil {
		report["state"] = "failed"
		report["commentError"] = truncateForProof(output+"\n"+err.Error(), 1200)
		server.logError(ctx, "github.pr.outbound_failed", text(report, "commentError"), githubOutboundSyncLogFields(input, report))
		return report
	}
	report["state"] = "synced"
	report["commentPosted"] = true
	report["commentOutput"] = truncateForProof(output, 800)
	server.logInfo(ctx, "github.pr.outbound_synced", "GitHub pull request comment synced.", githubOutboundSyncLogFields(input, report))
	return report
}

func githubPullRequestRefFromURL(raw string) (string, int, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.EqualFold(parsed.Host, "github.com") {
		return "", 0, false
	}
	segments := compactStringList(strings.Split(strings.Trim(parsed.Path, "/"), "/"))
	if len(segments) < 4 || segments[2] != "pull" {
		return "", 0, false
	}
	number, err := strconv.Atoi(segments[3])
	if err != nil || number <= 0 {
		return "", 0, false
	}
	return segments[0] + "/" + segments[1], number, true
}

func githubIssueRefFromWorkItem(item map[string]any, fallbackRepo string) (string, int, bool) {
	if ref := strings.TrimSpace(text(item, "sourceExternalRef")); ref != "" {
		if repo, number, ok := parseGitHubIssueRef(ref); ok {
			return repo, number, true
		}
	}
	for _, key := range []string{"target", "url"} {
		if repo, number, ok := parseGitHubIssueRef(text(item, key)); ok {
			return repo, number, true
		}
	}
	repo := strings.TrimSpace(fallbackRepo)
	if repo == "" || !strings.Contains(repo, "/") {
		return "", 0, false
	}
	for _, key := range []string{"githubIssueNumber", "issueNumber", "number"} {
		if number := intValue(item[key]); number > 0 {
			return repo, number, true
		}
	}
	if ref := strings.TrimSpace(text(item, "sourceExternalRef")); ref != "" {
		parts := strings.Split(ref, "#")
		if len(parts) == 2 {
			if number, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil && number > 0 {
				return repo, number, true
			}
		}
	}
	return "", 0, false
}

func parseGitHubIssueRef(raw string) (string, int, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", 0, false
	}
	if parsed, err := url.Parse(value); err == nil && strings.EqualFold(parsed.Host, "github.com") {
		segments := compactStringList(strings.Split(strings.Trim(parsed.Path, "/"), "/"))
		if len(segments) >= 4 && segments[2] == "issues" {
			number, err := strconv.Atoi(segments[3])
			if err == nil && number > 0 {
				return segments[0] + "/" + segments[1], number, true
			}
		}
	}
	re := regexp.MustCompile(`^([A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+)#([0-9]+)$`)
	matches := re.FindStringSubmatch(value)
	if len(matches) == 3 {
		number, err := strconv.Atoi(matches[2])
		if err == nil && number > 0 {
			return matches[1], number, true
		}
	}
	return "", 0, false
}

func repositoryFromPullRequestURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.EqualFold(parsed.Host, "github.com") {
		return ""
	}
	segments := compactStringList(strings.Split(strings.Trim(parsed.Path, "/"), "/"))
	if len(segments) < 2 {
		return ""
	}
	return segments[0] + "/" + segments[1]
}

type githubOutboundLabel struct {
	name        string
	color       string
	description string
}

func githubIssueOutboundLabels(status string, event string) struct {
	add    []githubOutboundLabel
	remove []string
} {
	statusLabel := "omega:running"
	description := "Pipeline is running in Omega."
	lower := strings.ToLower(strings.TrimSpace(status + " " + event))
	switch {
	case strings.Contains(lower, "done") || strings.Contains(lower, "completed"):
		statusLabel = "omega:done"
		description = "Pipeline delivery completed in Omega."
	case strings.Contains(lower, "failed") || strings.Contains(lower, "blocked") || strings.Contains(lower, "stalled"):
		statusLabel = "omega:blocked"
		description = "Pipeline needs attention in Omega."
	case strings.Contains(lower, "review") || strings.Contains(lower, "waiting-human") || strings.Contains(lower, "pr-ready"):
		statusLabel = "omega:review"
		description = "Pipeline is waiting for review in Omega."
	case strings.Contains(lower, "merge"):
		statusLabel = "omega:merging"
		description = "Pipeline is merging delivery changes in Omega."
	}
	allStatusLabels := []string{"omega:running", "omega:review", "omega:blocked", "omega:merging", "omega:done"}
	remove := []string{}
	for _, label := range allStatusLabels {
		if label != statusLabel {
			remove = append(remove, label)
		}
	}
	return struct {
		add    []githubOutboundLabel
		remove []string
	}{
		add: []githubOutboundLabel{
			{name: "omega:managed", color: "2563eb", description: "Managed by Omega."},
			{name: statusLabel, color: githubOutboundStatusColor(statusLabel), description: description},
		},
		remove: remove,
	}
}

func githubOutboundStatusColor(label string) string {
	switch label {
	case "omega:done":
		return "22c55e"
	case "omega:review":
		return "f59e0b"
	case "omega:blocked":
		return "ef4444"
	case "omega:merging":
		return "06b6d4"
	default:
		return "3b82f6"
	}
}

func githubOutboundLabelCatalog() []githubOutboundLabel {
	return []githubOutboundLabel{
		{name: "omega:managed", color: "2563eb", description: "Managed by Omega."},
		{name: "omega:running", color: githubOutboundStatusColor("omega:running"), description: "Pipeline is running in Omega."},
		{name: "omega:review", color: githubOutboundStatusColor("omega:review"), description: "Pipeline is waiting for review in Omega."},
		{name: "omega:blocked", color: githubOutboundStatusColor("omega:blocked"), description: "Pipeline needs attention in Omega."},
		{name: "omega:merging", color: githubOutboundStatusColor("omega:merging"), description: "Pipeline is merging delivery changes in Omega."},
		{name: "omega:done", color: githubOutboundStatusColor("omega:done"), description: "Pipeline delivery completed in Omega."},
	}
}

func githubIssueOutboundCommentBody(input githubOutboundSyncInput) (string, string, error) {
	status := stringOr(input.Status, "running")
	event := stringOr(input.Event, "pipeline.updated")
	title := stringOr(text(input.WorkItem, "title"), text(input.WorkItem, "key"))
	lines := []string{
		githubOutboundSyncMarker,
		"## Omega delivery update",
		"",
		fmt.Sprintf("- Work item: `%s` %s", stringOr(text(input.WorkItem, "key"), text(input.WorkItem, "id")), title),
		fmt.Sprintf("- Pipeline: `%s`", text(input.Pipeline, "id")),
		fmt.Sprintf("- Attempt: `%s`", stringOr(input.AttemptID, text(input.Attempt, "id"))),
		fmt.Sprintf("- Event: `%s`", event),
		fmt.Sprintf("- Status: `%s`", status),
	}
	if input.StageID != "" {
		lines = append(lines, fmt.Sprintf("- Stage: `%s`", input.StageID))
	}
	if input.BranchName != "" {
		lines = append(lines, fmt.Sprintf("- Branch: `%s`", input.BranchName))
	}
	if input.PullRequestURL != "" {
		lines = append(lines, fmt.Sprintf("- Pull request: %s", input.PullRequestURL))
	}
	if strings.TrimSpace(input.Summary) != "" {
		lines = append(lines, "", "### Summary", "", strings.TrimSpace(input.Summary))
	}
	if len(input.ChangedFiles) > 0 {
		lines = append(lines, "", "### Changed files", "")
		for _, file := range input.ChangedFiles {
			lines = append(lines, "- `"+file+"`")
		}
	}
	if strings.TrimSpace(input.ChecksOutput) != "" || len(input.CheckLogFeedback) > 0 {
		lines = append(lines, "", "### CI / checks", "")
		if strings.TrimSpace(input.ChecksOutput) != "" {
			lines = append(lines, "```text", truncateForProof(strings.TrimSpace(input.ChecksOutput), 3000), "```")
		}
		for _, feedback := range input.CheckLogFeedback {
			lines = append(lines, "- "+strings.TrimSpace(text(feedback, "label")+": "+text(feedback, "message")))
		}
	}
	if len(input.PullRequestFeedback) > 0 {
		lines = append(lines, "", "### Review feedback", "")
		for _, feedback := range input.PullRequestFeedback {
			lines = append(lines, "- "+strings.TrimSpace(text(feedback, "label")+": "+text(feedback, "message")))
		}
	}
	if input.FailureReason != "" || input.FailureDetail != "" {
		lines = append(lines, "", "### Attention needed", "")
		if input.FailureReason != "" {
			lines = append(lines, "- Reason: "+input.FailureReason)
		}
		if input.FailureDetail != "" {
			lines = append(lines, "```text", truncateForProof(input.FailureDetail, 3000), "```")
		}
	}
	if len(input.ReviewPacket) > 0 {
		risk := text(mapValue(input.ReviewPacket["risk"]), "level")
		if risk != "" {
			lines = append(lines, "", "### Risk", "", "- Level: `"+risk+"`")
		}
		for _, action := range arrayMaps(input.ReviewPacket["recommendedActions"]) {
			lines = append(lines, "- "+stringOr(text(action, "label"), text(action, "type")))
		}
	}
	lines = append(lines, "", fmt.Sprintf("_Synced at %s._", nowISO()))

	path := filepath.Join(os.TempDir(), "omega-github-issue-sync-"+safeSegment(text(input.Pipeline, "id"))+"-"+safeSegment(stringOr(input.AttemptID, text(input.Attempt, "id")))+".md")
	body := strings.Join(lines, "\n")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return body, "", err
	}
	return body, path, nil
}

func githubPullRequestOutboundCommentBody(input githubOutboundSyncInput) (string, string, error) {
	status := stringOr(input.Status, "running")
	event := stringOr(input.Event, "pipeline.updated")
	title := stringOr(text(input.WorkItem, "title"), text(input.WorkItem, "key"))
	lines := []string{
		githubPullRequestOutboundSyncMarker,
		"## Omega review packet",
		"",
		fmt.Sprintf("- Work item: `%s` %s", stringOr(text(input.WorkItem, "key"), text(input.WorkItem, "id")), title),
		fmt.Sprintf("- Pipeline: `%s`", text(input.Pipeline, "id")),
		fmt.Sprintf("- Attempt: `%s`", stringOr(input.AttemptID, text(input.Attempt, "id"))),
		fmt.Sprintf("- Event: `%s`", event),
		fmt.Sprintf("- Status: `%s`", status),
	}
	if input.StageID != "" {
		lines = append(lines, fmt.Sprintf("- Stage: `%s`", input.StageID))
	}
	if input.BranchName != "" {
		lines = append(lines, fmt.Sprintf("- Branch: `%s`", input.BranchName))
	}
	if input.PullRequestURL != "" {
		lines = append(lines, fmt.Sprintf("- Pull request: %s", input.PullRequestURL))
	}
	if summary := strings.TrimSpace(input.Summary); summary != "" {
		lines = append(lines, "", "### Summary", "", summary)
	}
	if len(input.ChangedFiles) > 0 {
		lines = append(lines, "", "### Changed files", "")
		for _, file := range input.ChangedFiles {
			lines = append(lines, "- `"+file+"`")
		}
	}
	if len(input.ReviewPacket) > 0 {
		if summary := text(input.ReviewPacket, "summary"); summary != "" {
			lines = append(lines, "", "### Review packet", "", truncateForProof(summary, 2000))
		}
		if diff := mapValue(input.ReviewPacket["diffPreview"]); len(diff) > 0 {
			lines = append(lines, "", "### Diff preview", "", "```diff", truncateForProof(text(diff, "patchExcerpt"), 4000), "```")
		}
		if risk := mapValue(input.ReviewPacket["risk"]); len(risk) > 0 {
			lines = append(lines, "", "### Risk", "", "- Level: `"+text(risk, "level")+"`")
			for _, reason := range stringSlice(risk["reasons"]) {
				lines = append(lines, "- "+reason)
			}
		}
		if actions := arrayMaps(input.ReviewPacket["recommendedActions"]); len(actions) > 0 {
			lines = append(lines, "", "### Recommended next actions", "")
			for _, action := range actions {
				lines = append(lines, "- "+stringOr(text(action, "label"), text(action, "type")))
			}
		}
	}
	if strings.TrimSpace(input.ChecksOutput) != "" || len(input.CheckLogFeedback) > 0 {
		lines = append(lines, "", "### CI / checks", "")
		if strings.TrimSpace(input.ChecksOutput) != "" {
			lines = append(lines, "```text", truncateForProof(strings.TrimSpace(input.ChecksOutput), 3000), "```")
		}
		for _, feedback := range input.CheckLogFeedback {
			label := stringOr(text(feedback, "label"), text(feedback, "name"))
			message := stringOr(text(feedback, "message"), text(feedback, "summary"))
			if label != "" || message != "" {
				lines = append(lines, "- "+strings.TrimSpace(label+": "+message))
			}
		}
	}
	if len(input.PullRequestFeedback) > 0 {
		lines = append(lines, "", "### Review feedback", "")
		for _, feedback := range input.PullRequestFeedback {
			label := stringOr(text(feedback, "label"), text(feedback, "author"))
			message := stringOr(text(feedback, "message"), text(feedback, "body"))
			lines = append(lines, "- "+strings.TrimSpace(label+": "+message))
		}
	}
	if input.FailureReason != "" || input.FailureDetail != "" {
		lines = append(lines, "", "### Attention needed", "")
		if input.FailureReason != "" {
			lines = append(lines, "- Reason: "+input.FailureReason)
		}
		if input.FailureDetail != "" {
			lines = append(lines, "```text", truncateForProof(input.FailureDetail, 3000), "```")
		}
	}
	lines = append(lines, "", fmt.Sprintf("_Synced at %s._", nowISO()))

	path := filepath.Join(os.TempDir(), "omega-github-pr-sync-"+safeSegment(text(input.Pipeline, "id"))+"-"+safeSegment(stringOr(input.AttemptID, text(input.Attempt, "id")))+".md")
	body := strings.Join(lines, "\n")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return body, "", err
	}
	return body, path, nil
}

func (server *Server) triggerGitHubCIIfConfigured(ctx context.Context, repositoryPath string, input githubOutboundSyncInput) map[string]any {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("OMEGA_GITHUB_CI_TRIGGER")))
	if mode == "" || mode == "none" || mode == "off" || mode == "false" {
		return nil
	}
	report := map[string]any{
		"mode":      mode,
		"state":     "skipped",
		"createdAt": nowISO(),
	}
	switch mode {
	case "rerun-failed", "rerun_failed":
		runIDs := githubOutboundCheckRunIDs(input.CheckLogFeedback)
		if len(runIDs) == 0 {
			report["reason"] = "no failed GitHub Actions run IDs were captured"
			report["requires"] = "check log feedback with runId"
			return report
		}
		results := []map[string]any{}
		failures := []string{}
		for _, runID := range runIDs {
			output, err := runGitHubOutboundCommand(ctx, repositoryPath, "run", "rerun", runID, "--failed")
			entry := map[string]any{"runId": runID, "output": truncateForProof(output, 800)}
			if err != nil {
				entry["state"] = "failed"
				entry["error"] = truncateForProof(output+"\n"+err.Error(), 1000)
				failures = append(failures, text(entry, "error"))
			} else {
				entry["state"] = "triggered"
			}
			results = append(results, entry)
		}
		report["runs"] = results
		if len(failures) > 0 {
			report["state"] = "partial"
			if len(failures) == len(runIDs) {
				report["state"] = "failed"
			}
			report["errors"] = failures
			server.logError(ctx, "github.ci.trigger_partial", "GitHub CI rerun had failures.", githubOutboundSyncLogFields(input, report))
			return report
		}
		report["state"] = "triggered"
		server.logInfo(ctx, "github.ci.triggered", "GitHub failed CI rerun triggered.", githubOutboundSyncLogFields(input, report))
		return report
	case "workflow-dispatch", "workflow_dispatch", "dispatch":
		workflow := strings.TrimSpace(os.Getenv("OMEGA_GITHUB_CI_WORKFLOW"))
		ref := stringOr(strings.TrimSpace(os.Getenv("OMEGA_GITHUB_CI_REF")), input.BranchName)
		if workflow == "" || ref == "" {
			report["state"] = "needs-configuration"
			report["requires"] = "OMEGA_GITHUB_CI_WORKFLOW and a branch/ref"
			return report
		}
		args := []string{"workflow", "run", workflow, "--ref", ref}
		for key, value := range githubOutboundCIInputsFromEnv() {
			args = append(args, "-f", key+"="+value)
		}
		output, err := runGitHubOutboundCommand(ctx, repositoryPath, args...)
		report["workflow"] = workflow
		report["ref"] = ref
		report["output"] = truncateForProof(output, 800)
		if err != nil {
			report["state"] = "failed"
			report["error"] = truncateForProof(output+"\n"+err.Error(), 1200)
			server.logError(ctx, "github.ci.trigger_failed", text(report, "error"), githubOutboundSyncLogFields(input, report))
			return report
		}
		report["state"] = "triggered"
		server.logInfo(ctx, "github.ci.triggered", "GitHub workflow dispatch triggered.", githubOutboundSyncLogFields(input, report))
		return report
	default:
		report["state"] = "needs-configuration"
		report["reason"] = "unsupported OMEGA_GITHUB_CI_TRIGGER value"
		report["supportedModes"] = []any{"rerun-failed", "workflow-dispatch"}
		return report
	}
}

func githubOutboundCheckRunIDs(feedback []map[string]any) []string {
	seen := map[string]bool{}
	runIDs := []string{}
	for _, item := range feedback {
		for _, key := range []string{"runId", "runID", "run_id", "workflowRunId", "workflow_run_id"} {
			value := strings.TrimSpace(stringOr(item[key], ""))
			if value != "" && !seen[value] {
				seen[value] = true
				runIDs = append(runIDs, value)
			}
		}
	}
	return runIDs
}

func githubOutboundCIInputsFromEnv() map[string]string {
	raw := strings.TrimSpace(os.Getenv("OMEGA_GITHUB_CI_INPUTS"))
	if raw == "" {
		return map[string]string{}
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return map[string]string{}
	}
	inputs := map[string]string{}
	for key, value := range payload {
		if strings.TrimSpace(key) != "" {
			inputs[key] = stringOr(value, fmt.Sprint(value))
		}
	}
	return inputs
}

func runGitHubOutboundCommand(ctx context.Context, dir string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "gh", args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("gh %s failed: %w", strings.Join(args, " "), err)
	}
	return string(output), nil
}

func githubOutboundSyncLogFields(input githubOutboundSyncInput, report map[string]any) map[string]any {
	return map[string]any{
		"entityType":     "github_issue",
		"entityId":       fmt.Sprintf("%s#%d", text(report, "repository"), intValue(report["issueNumber"])),
		"workItemId":     text(input.WorkItem, "id"),
		"pipelineId":     text(input.Pipeline, "id"),
		"attemptId":      stringOr(input.AttemptID, text(input.Attempt, "id")),
		"stageId":        input.StageID,
		"event":          input.Event,
		"status":         input.Status,
		"syncState":      text(report, "state"),
		"commentPosted":  boolValue(report["commentPosted"]),
		"labelsSynced":   boolValue(report["labelsSynced"]),
		"pullRequestUrl": input.PullRequestURL,
		"repository":     text(report, "repository"),
		"issueNumber":    intValue(report["issueNumber"]),
		"commentError":   text(report, "commentError"),
		"labelErrors":    report["labelErrors"],
	}
}
