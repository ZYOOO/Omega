package omegalocal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type devFlowCIResult struct {
	Status           string
	Summary          string
	ChecksOutput     string
	RemoteChecks     []map[string]any
	RemoteChecksRaw  string
	CheckSummary     map[string]any
	CheckLogFeedback []map[string]any
	ReportPath       string
}

func collectDevFlowGitHubActionsCI(ctx context.Context, repoWorkspace string, repoSlug string, prURL string, requiredChecks []string, proofDir string, artifactName string) devFlowCIResult {
	result := devFlowCIResult{Status: "missing", Summary: "No pull request was available for CI checks."}
	if strings.TrimSpace(prURL) == "" {
		return result
	}
	checksOutput, _ := runCommand(repoWorkspace, "gh", "pr", "checks", prURL)
	remoteChecks, remoteChecksRaw := githubPullRequestChecks(ctx, repoWorkspace, prURL, repoSlug)
	if strings.TrimSpace(remoteChecksRaw) != "" {
		checksOutput = remoteChecksRaw
	}
	checkSummary := githubCheckSummaryWithRequired(remoteChecks, requiredChecks)
	checkLogFeedback := githubPullRequestCheckLogFeedback(ctx, repoWorkspace, prURL, repoSlug, remoteChecks)
	result.ChecksOutput = checksOutput
	result.RemoteChecks = remoteChecks
	result.RemoteChecksRaw = remoteChecksRaw
	result.CheckSummary = checkSummary
	result.CheckLogFeedback = checkLogFeedback
	result.Status = devFlowCIStatus(checkSummary, checkLogFeedback, checksOutput)
	result.Summary = devFlowCISummary(checkSummary, checksOutput)
	if strings.TrimSpace(proofDir) != "" {
		if strings.TrimSpace(artifactName) == "" {
			artifactName = "ci-checks.md"
		}
		result.ReportPath = filepath.Join(proofDir, artifactName)
		_ = os.WriteFile(result.ReportPath, []byte(devFlowCIReportMarkdown(prURL, requiredChecks, result)), 0o644)
	}
	return result
}

func devFlowCIStatus(summary map[string]any, checkLogFeedback []map[string]any, checksOutput string) string {
	switch {
	case len(summary) == 0 && strings.TrimSpace(checksOutput) == "":
		return "missing"
	case intValue(summary["failed"]) > 0 || intValue(summary["missingRequired"]) > 0 || len(checkLogFeedback) > 0:
		return "attention"
	case intValue(summary["pending"]) > 0:
		return "pending"
	case intValue(summary["total"]) == 0:
		return "missing"
	default:
		return "passed"
	}
}

func devFlowCISummary(summary map[string]any, checksOutput string) string {
	if len(summary) == 0 {
		return devFlowFirstUsefulLine(checksOutput, "No GitHub Actions check output captured.")
	}
	return fmt.Sprintf("GitHub Actions checks: %d total, %d passed, %d pending, %d failed, %d missing required.",
		intValue(summary["total"]),
		intValue(summary["passed"]),
		intValue(summary["pending"]),
		intValue(summary["failed"]),
		intValue(summary["missingRequired"]),
	)
}

func devFlowCIReportMarkdown(prURL string, requiredChecks []string, result devFlowCIResult) string {
	lines := []string{
		"# GitHub Actions CI",
		"",
		"- Pull request: " + stringOr(prURL, "not recorded"),
		"- Refreshed at: " + nowISO(),
		"- Status: " + stringOr(result.Status, "unknown"),
		"- Summary: " + stringOr(result.Summary, "No summary captured."),
	}
	if len(requiredChecks) > 0 {
		lines = append(lines, "- Required checks: "+strings.Join(requiredChecks, ", "))
	}
	lines = append(lines,
		"",
		"## Check Summary",
		"",
		fmt.Sprintf("- Total: %d", intValue(result.CheckSummary["total"])),
		fmt.Sprintf("- Passed: %d", intValue(result.CheckSummary["passed"])),
		fmt.Sprintf("- Pending: %d", intValue(result.CheckSummary["pending"])),
		fmt.Sprintf("- Failed: %d", intValue(result.CheckSummary["failed"])),
		fmt.Sprintf("- Missing required: %d", intValue(result.CheckSummary["missingRequired"])),
		"",
		"## gh pr checks",
		"",
		"```text",
		stringOr(strings.TrimSpace(result.ChecksOutput), "No GitHub Actions checks were reported."),
		"```",
	)
	if len(result.CheckLogFeedback) > 0 {
		lines = append(lines, "", "## Failed Check Logs", "")
		for _, feedback := range result.CheckLogFeedback {
			lines = append(lines,
				"### "+stringOr(text(feedback, "label"), "Failed check"),
				"",
				"- State: "+stringOr(text(feedback, "state"), "failed"),
				"- Run: "+stringOr(text(feedback, "url"), "not recorded"),
				"",
				"```text",
				stringOr(text(feedback, "message"), "No log excerpt captured."),
				"```",
				"",
			)
		}
	}
	return strings.Join(lines, "\n") + "\n"
}
