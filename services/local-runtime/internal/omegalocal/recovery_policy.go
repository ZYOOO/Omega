package omegalocal

import "strings"

type supervisorRecoveryPolicy struct {
	Class      string
	Action     string
	Label      string
	AutoRetry  bool
	Confidence string
}

func supervisorRecoveryPolicyForAttempt(attempt map[string]any) supervisorRecoveryPolicy {
	haystack := supervisorRecoveryHaystack(attempt)
	recommendedActions := reworkChecklistRecommendedActions(attempt)
	checkLogs := reworkChecklistCheckLogFeedback(attempt)
	if containsAny(haystack, []string{
		"viewerpermission", "permission denied", "not authorized", "unauthorized", "forbidden", "resource not accessible by integration",
		"authentication required", "gh auth", "bad credentials", "requires authentication", "403", "cannot be trusted for delivery branch push",
		"cannot be trusted for pull request creation", "insufficient permission", "missing permission", "permission failure",
	}) {
		return supervisorRecoveryPolicy{
			Class:      "permission_failure",
			Action:     "manual-fix-permission",
			Label:      "Fix credentials, repository permissions, or branch policy before retrying.",
			AutoRetry:  false,
			Confidence: "high",
		}
	}
	if containsAny(haystack, []string{
		"github api", "gh api", "api rate limit", "secondary rate limit", "rate limit exceeded", "bad gateway", "service unavailable",
		"gateway timeout", "http 500", "http 502", "http 503", "http 504", "github unavailable", "gh pr checks failed",
	}) {
		return supervisorRecoveryPolicy{
			Class:      "github_api_transient",
			Action:     "wait-and-retry",
			Label:      "Wait for GitHub API recovery, then retry the same attempt.",
			AutoRetry:  true,
			Confidence: "medium",
		}
	}
	if containsAny(haystack, []string{
		"temporary network", "network failure", "connection reset", "connection refused", "econnreset", "etimedout",
		"i/o timeout", "tls handshake timeout", "no such host", "dns", "timeout awaiting response headers", "context deadline exceeded",
	}) {
		return supervisorRecoveryPolicy{
			Class:      "transient_network",
			Action:     "wait-and-retry",
			Label:      "Wait for the local or remote network to recover, then retry.",
			AutoRetry:  true,
			Confidence: "medium",
		}
	}
	if containsAny(haystack, []string{
		"runner crash", "signal: killed", "signal: segmentation fault", "exit status 137", "exit status 143", "broken pipe",
		"runner process exited", "runner host", "worker host", "no active local worker host lease", "process killed",
	}) {
		return supervisorRecoveryPolicy{
			Class:      "runner_crash",
			Action:     "retry-with-clean-worker",
			Label:      "Start a fresh local worker and retry with the same repository workspace.",
			AutoRetry:  true,
			Confidence: "medium",
		}
	}
	if hasRecommendedAction(recommendedActions, "checks-failed") || hasRecommendedAction(recommendedActions, "checks-error") || len(checkLogs) > 0 {
		if supervisorCheckLogsLookFlaky(checkLogs, haystack) {
			return supervisorRecoveryPolicy{
				Class:      "ci_flaky_failure",
				Action:     "retry-validation",
				Label:      "Retry validation once; if it fails again, route the check log into rework.",
				AutoRetry:  true,
				Confidence: "medium",
			}
		}
		return supervisorRecoveryPolicy{
			Class:      "ci_failure",
			Action:     "rework-required",
			Label:      "Route failed check output into the rework checklist before retrying delivery.",
			AutoRetry:  false,
			Confidence: "medium",
		}
	}
	return supervisorRecoveryPolicy{
		Class:      "unknown_failure",
		Action:     "retry-with-context",
		Label:      "Retry with the captured failure context; inspect manually if the next attempt fails.",
		AutoRetry:  true,
		Confidence: "low",
	}
}

func supervisorRecoveryPolicyMap(policy supervisorRecoveryPolicy) map[string]any {
	return map[string]any{
		"class":      policy.Class,
		"action":     policy.Action,
		"label":      policy.Label,
		"autoRetry":  policy.AutoRetry,
		"confidence": policy.Confidence,
	}
}

func supervisorRecoveryHaystack(attempt map[string]any) string {
	values := []string{
		text(attempt, "status"),
		text(attempt, "statusReason"),
		text(attempt, "errorMessage"),
		text(attempt, "failureReason"),
		text(attempt, "failureDetail"),
		text(attempt, "failureReviewFeedback"),
		text(attempt, "stderrSummary"),
		text(attempt, "retryReason"),
	}
	for _, action := range reworkChecklistRecommendedActions(attempt) {
		values = append(values, text(action, "type"), text(action, "label"))
	}
	for _, feedback := range reworkChecklistCheckLogFeedback(attempt) {
		values = append(values, text(feedback, "kind"), text(feedback, "label"), text(feedback, "message"))
	}
	return strings.ToLower(strings.Join(values, "\n"))
}

func hasRecommendedAction(actions []map[string]any, actionType string) bool {
	for _, action := range actions {
		if strings.EqualFold(text(action, "type"), actionType) {
			return true
		}
	}
	return false
}

func supervisorCheckLogsLookFlaky(checkLogs []map[string]any, haystack string) bool {
	if containsAny(haystack, []string{
		"flaky", "flake", "intermittent", "rerun", "re-run", "retry succeeded", "timed out", "timeout", "canceled",
		"cancelled", "infrastructure", "runner lost", "hosted runner", "rate limit", "network",
	}) {
		return true
	}
	for _, feedback := range checkLogs {
		message := strings.ToLower(text(feedback, "message") + " " + text(feedback, "label"))
		if containsAny(message, []string{"flaky", "intermittent", "timed out", "timeout", "runner lost", "infrastructure", "network"}) {
			return true
		}
	}
	return false
}
