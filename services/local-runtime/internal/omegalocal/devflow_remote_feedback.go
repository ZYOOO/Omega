package omegalocal

import (
	"fmt"
	"strings"
)

func devFlowRemoteGateFeedback(checkSummary map[string]any, checkLogFeedback []map[string]any) (string, bool) {
	if len(checkSummary) == 0 {
		return "", false
	}
	failed := intValue(checkSummary["failed"])
	missing := intValue(checkSummary["missingRequired"])
	if failed == 0 && missing == 0 {
		return "", false
	}
	lines := []string{"Remote delivery checks require rework before human review."}
	if failed > 0 {
		lines = append(lines, fmt.Sprintf("Failed checks: %d.", failed))
		for _, check := range arrayMaps(checkSummary["failedChecks"]) {
			lines = append(lines, fmt.Sprintf("- %s: %s", stringOr(text(check, "name"), "check"), stringOr(text(check, "state"), "failed")))
		}
	}
	if missing > 0 {
		lines = append(lines, fmt.Sprintf("Missing required checks: %d.", missing))
		for _, check := range arrayMaps(checkSummary["missingRequiredChecks"]) {
			lines = append(lines, fmt.Sprintf("- %s: missing", stringOr(text(check, "name"), "required check")))
		}
	}
	if feedback := githubPullRequestFeedbackPrompt(checkLogFeedback); strings.TrimSpace(feedback) != "" {
		lines = append(lines, "Check log excerpts:", feedback)
	}
	return strings.Join(lines, "\n"), true
}
