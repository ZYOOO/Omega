package omegalocal

import (
	"fmt"
	"strings"
)

func buildReworkChecklist(database WorkspaceDatabase, pipeline map[string]any, attempt map[string]any) map[string]any {
	sources := []map[string]any{}
	addSourceRecord := func(kind string, label string, message string, metadata map[string]any) {
		message = strings.TrimSpace(message)
		if message == "" {
			return
		}
		source := map[string]any{
			"kind":    kind,
			"label":   label,
			"message": truncateForProof(message, 1800),
		}
		for _, key := range []string{"url", "createdAt", "state", "runId"} {
			if value := strings.TrimSpace(stringOr(metadata[key], "")); value != "" {
				source[key] = value
			}
		}
		sources = append(sources, source)
	}
	addSource := func(kind string, label string, message string) {
		addSourceRecord(kind, label, message, nil)
	}

	addSource("human", "Human requested changes", text(attempt, "humanChangeRequest"))
	addSource("review", "Review feedback", text(attempt, "failureReviewFeedback"))
	addSource("failure", "Failure reason", text(attempt, "failureReason"))
	addSource("failure", "Status reason", text(attempt, "statusReason"))
	addSource("failure", "Error message", text(attempt, "errorMessage"))
	addSource("failure", "Failure detail", text(attempt, "failureDetail"))
	addSource("runner", "Runner stderr", text(attempt, "stderrSummary"))
	addSource("human", "Latest human change request", latestHumanChangeRequestFromPipeline(pipeline))

	for _, operation := range database.Tables.Operations {
		if text(operation, "pipelineId") != "" && text(operation, "pipelineId") != text(pipeline, "id") {
			continue
		}
		haystack := strings.ToLower(text(operation, "stageId") + " " + text(operation, "agentId") + " " + text(operation, "summary"))
		if !strings.Contains(haystack, "review") && !strings.Contains(haystack, "rework") && text(operation, "status") != "failed" {
			continue
		}
		addSource("operation", firstNonEmpty(text(operation, "stageId"), text(operation, "agentId"), "operation"), text(operation, "summary"))
	}

	for _, event := range arrayMaps(mapValue(pipeline["run"])["events"]) {
		eventType := text(event, "type")
		if !containsAny(strings.ToLower(eventType), []string{"rejected", "changes", "failed", "rework"}) {
			continue
		}
		addSource("event", eventType, text(event, "message"))
	}

	for _, action := range reworkChecklistRecommendedActions(attempt) {
		addSource("delivery-gate", text(action, "type"), text(action, "label"))
	}
	for _, feedback := range reworkChecklistPullRequestFeedback(attempt) {
		addSourceRecord(stringOr(text(feedback, "kind"), "pr-feedback"), text(feedback, "label"), text(feedback, "message"), feedback)
	}
	for _, feedback := range reworkChecklistCheckLogFeedback(attempt) {
		addSourceRecord(stringOr(text(feedback, "kind"), "ci-check-log"), text(feedback, "label"), text(feedback, "message"), feedback)
	}

	checklist := checklistItemsFromSources(sources)
	reason := reworkChecklistReason(attempt, sources)
	if len(checklist) == 0 && reason == "" {
		return map[string]any{
			"status":    "clear",
			"sources":   []map[string]any{},
			"checklist": []string{},
			"createdAt": nowISO(),
		}
	}
	return map[string]any{
		"status":      "needs-rework",
		"retryReason": reason,
		"checklist":   checklist,
		"sources":     sources,
		"prompt":      reworkChecklistPrompt(reason, checklist, sources),
		"createdAt":   nowISO(),
	}
}

func reworkChecklistCheckLogFeedback(attempt map[string]any) []map[string]any {
	for _, key := range []string{"checkLogFeedback", "ciCheckLogFeedback"} {
		if feedback := arrayMaps(attempt[key]); len(feedback) > 0 {
			return feedback
		}
	}
	for _, key := range []string{"prStatus", "pullRequestStatus", "githubPullRequestStatus"} {
		if feedback := arrayMaps(mapValue(attempt[key])["checkLogFeedback"]); len(feedback) > 0 {
			return feedback
		}
	}
	return nil
}

func reworkChecklistPullRequestFeedback(attempt map[string]any) []map[string]any {
	for _, key := range []string{"pullRequestFeedback", "prReviewFeedback", "reviewFeedback"} {
		if feedback := arrayMaps(attempt[key]); len(feedback) > 0 {
			return feedback
		}
	}
	for _, key := range []string{"prStatus", "pullRequestStatus", "githubPullRequestStatus"} {
		if feedback := arrayMaps(mapValue(attempt[key])["reviewFeedback"]); len(feedback) > 0 {
			return feedback
		}
	}
	return nil
}

func reworkChecklistRecommendedActions(attempt map[string]any) []map[string]any {
	for _, key := range []string{"recommendedActions", "deliveryRecommendedActions"} {
		if actions := arrayMaps(attempt[key]); len(actions) > 0 {
			return actions
		}
	}
	for _, key := range []string{"prStatus", "pullRequestStatus", "githubPullRequestStatus"} {
		if actions := arrayMaps(mapValue(attempt[key])["recommendedActions"]); len(actions) > 0 {
			return actions
		}
	}
	return nil
}

func checklistItemsFromSources(sources []map[string]any) []string {
	items := []string{}
	for _, source := range sources {
		kind := text(source, "kind")
		message := text(source, "message")
		switch kind {
		case "human":
			items = append(items, "按人工反馈完成修改："+message)
		case "review":
			items = append(items, "处理 Review Agent 指出的阻塞问题："+message)
		case "pr-review", "pr-comment", "pr-feedback":
			items = append(items, "处理 PR 评审/评论反馈："+message)
		case "ci-check-log":
			items = append(items, "根据 CI/check 失败日志修复并重新验证："+message)
		case "delivery-gate":
			items = append(items, deliveryGateChecklistItem(source))
		case "operation", "event":
			items = append(items, "复核运行记录并补齐对应修复："+message)
		default:
			items = append(items, "解决导致本次执行不能继续的问题："+message)
		}
	}
	return compactStringList(items)
}

func deliveryGateChecklistItem(source map[string]any) string {
	switch text(source, "label") {
	case "checks-failed":
		return "查看失败的 CI/check 输出，在同一分支上修复后重新验证。"
	case "required-checks-missing":
		return "确认必需 checks 是否缺失；缺失配置或等待完成后再进入交付。"
	case "checks-pending":
		return "等待 pending checks 完成；若转失败则把失败输出纳入 rework。"
	case "branch-sync":
		return "同步 PR 分支到目标基线并重新运行验证。"
	case "merge-conflict":
		return "解决 merge conflict，提交冲突修复后重新 review。"
	case "review":
		return "处理 PR review decision 指出的未解决意见。"
	default:
		return "处理交付门禁建议：" + text(source, "message")
	}
}

func reworkChecklistReason(attempt map[string]any, sources []map[string]any) string {
	for _, value := range []string{text(attempt, "retryReason"), text(attempt, "humanChangeRequest"), text(attempt, "failureReason"), text(attempt, "failureReviewFeedback"), text(attempt, "statusReason"), text(attempt, "errorMessage"), text(attempt, "failureDetail")} {
		if strings.TrimSpace(value) != "" {
			return truncateForProof(value, 1200)
		}
	}
	if len(sources) > 0 {
		return text(sources[0], "message")
	}
	if status := text(attempt, "status"); status == "failed" || status == "stalled" || status == "canceled" {
		return "Attempt is " + status + " and needs a retry decision."
	}
	return ""
}

func reworkChecklistPrompt(reason string, checklist []string, sources []map[string]any) string {
	lines := []string{}
	if strings.TrimSpace(reason) != "" {
		lines = append(lines, "Retry / rework reason:\n"+strings.TrimSpace(reason))
	}
	if len(checklist) > 0 {
		lines = append(lines, "Rework checklist:")
		for _, item := range checklist {
			lines = append(lines, "- "+item)
		}
	}
	if len(sources) > 0 {
		lines = append(lines, "Source feedback:")
		for _, source := range sources {
			lines = append(lines, fmt.Sprintf("- [%s] %s: %s", text(source, "kind"), text(source, "label"), text(source, "message")))
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func reworkChecklistPromptFromAttempt(attempt map[string]any, fallback string) string {
	checklist := mapValue(attempt["reworkChecklist"])
	prompt := strings.TrimSpace(text(checklist, "prompt"))
	if prompt != "" {
		return prompt
	}
	return strings.TrimSpace(fallback)
}
