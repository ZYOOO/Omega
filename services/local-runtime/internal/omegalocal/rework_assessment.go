package omegalocal

import (
	"fmt"
	"strings"
)

const (
	reworkStrategyFastRework     = "fast_rework"
	reworkStrategyReplanRework   = "replan_rework"
	reworkStrategyNeedsHumanInfo = "needs_human_info"
)

func assessHumanRequestedRework(reason string, item map[string]any, previousAttempt map[string]any, pipeline map[string]any) map[string]any {
	normalized := strings.TrimSpace(reason)
	lower := strings.ToLower(normalized)
	strategy := reworkStrategyFastRework
	entryStageID := "rework"
	rationale := "人工意见可以在现有分支和 PR 上做局部修改。"
	signals := []string{"复用上一轮分支和 PR", "二次 review 核对人工意见与增量 diff"}
	checklist := []string{
		"基于上一轮 delivery branch 继续修改。",
		"只提交本轮人工意见要求的最小增量 diff。",
		"更新验证结果和 PR 描述。",
		"二次 review 重点核对人工意见是否被满足。",
	}

	switch {
	case normalized == "":
		strategy = reworkStrategyNeedsHumanInfo
		entryStageID = "human_review"
		rationale = "人工意见为空，不能让 Agent 猜测要修改什么。"
		signals = []string{"缺少明确人工反馈"}
		checklist = []string{"请补充具体需要修改的行为、页面、文件或验收口径。"}
	case containsAny(lower, []string{"不确定", "你看着办", "随便", "再看看", "不清楚", "？", "?"}):
		strategy = reworkStrategyNeedsHumanInfo
		entryStageID = "human_review"
		rationale = "人工意见包含不确定表达，需要继续补齐决策。"
		signals = append(signals, "人工意见需要补充业务判断")
		checklist = []string{"请补充明确期望结果或允许的实现边界。"}
	case containsAny(lower, []string{"重新设计", "重做架构", "架构", "数据模型", "数据库", "schema", "migration", "接口", "api", "权限", "认证", "状态机", "流程", "多模块", "跨页面", "重构", "改需求", "新增功能", "后端"}):
		strategy = reworkStrategyReplanRework
		entryStageID = "todo"
		rationale = "人工意见可能改变需求、架构或跨模块边界，需要重新规划后再实现。"
		signals = append(signals, "需要重新评估 requirement / architecture")
		checklist = []string{
			"重新整理 requirement 和 acceptance criteria。",
			"更新 solution plan，标明架构或接口影响。",
			"基于上一轮 PR branch 继续修改，避免丢失已完成工作。",
			"二次 review 同时核对新规划和增量 diff。",
		}
	case containsAny(lower, []string{"文案", "文字", "标题", "名称", "改成", "颜色", "样式", "css", "间距", "布局", "按钮", "显示", "隐藏", "折叠", "展开", "对齐", "行高", "快一点", "慢", "copy"}):
		strategy = reworkStrategyFastRework
		entryStageID = "rework"
		rationale = "人工意见属于局部 UI、文案、样式或行为修正，适合直接快速 rework。"
		signals = append(signals, "局部修改信号")
	}

	return map[string]any{
		"strategy":          strategy,
		"entryStageId":      entryStageID,
		"rationale":         rationale,
		"humanFeedback":     normalized,
		"signals":           signals,
		"checklist":         checklist,
		"previousAttemptId": text(previousAttempt, "id"),
		"previousBranch":    text(previousAttempt, "branchName"),
		"previousPR":        text(previousAttempt, "pullRequestUrl"),
		"workItemId":        text(item, "id"),
		"pipelineId":        text(pipeline, "id"),
		"createdAt":         nowISO(),
	}
}

func reworkAssessmentMarkdown(assessment map[string]any) string {
	return fmt.Sprintf("# Rework Assessment\n\n- Strategy: `%s`\n- Entry stage: `%s`\n- Previous attempt: `%s`\n- Previous branch: `%s`\n- Previous PR: %s\n\n## Rationale\n\n%s\n\n## Human feedback\n\n%s\n\n## Checklist\n\n%s\n",
		text(assessment, "strategy"),
		text(assessment, "entryStageId"),
		text(assessment, "previousAttemptId"),
		text(assessment, "previousBranch"),
		stringOr(text(assessment, "previousPR"), "Not recorded."),
		text(assessment, "rationale"),
		stringOr(text(assessment, "humanFeedback"), "Not provided."),
		markdownAnyList(assessment["checklist"]))
}

func containsAny(value string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(value, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}
