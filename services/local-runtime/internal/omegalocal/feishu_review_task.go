package omegalocal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func feishuReviewTaskModeEnabled(options feishuReviewSendOptions) bool {
	mode := strings.ToLower(strings.TrimSpace(stringOr(options.Mode, os.Getenv("OMEGA_FEISHU_REVIEW_MODE"))))
	if mode == "task" || mode == "task-bridge" || mode == "task_bridge" {
		return true
	}
	return strings.TrimSpace(stringOr(options.AssigneeID, os.Getenv("OMEGA_FEISHU_REVIEW_ASSIGNEE_ID"))) != "" ||
		strings.TrimSpace(stringOr(options.TasklistID, os.Getenv("OMEGA_FEISHU_REVIEW_TASKLIST_ID"))) != ""
}

func feishuReviewTaskBridgeEnabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("OMEGA_FEISHU_TASK_BRIDGE_ENABLED")))
	return value == "1" || value == "true" || value == "yes"
}

func (server *Server) feishuReviewTaskBridgeEnabled(ctx context.Context) bool {
	record, err := server.feishuConfig(ctx)
	if err == nil && record.TaskBridgeEnabled {
		return true
	}
	return feishuReviewTaskBridgeEnabled()
}

func sendFeishuReviewTask(ctx context.Context, packet map[string]any, options feishuReviewSendOptions) (map[string]any, error) {
	checkpoint := mapValue(packet["checkpoint"])
	item := mapValue(packet["item"])
	attempt := mapValue(packet["attempt"])
	checkpointID := text(checkpoint, "id")
	nonce := feishuReviewNonce(checkpointID, text(attempt, "id"))
	doc := createFeishuReviewDocIfConfigured(ctx, packet, nonce, options)
	description := renderFeishuReviewTaskDescriptionForLanguage(packet, nonce, doc, options.Language)
	summaryMiddle := feishuLabel(options.Language, "Human review", "人工审核")
	summary := fmt.Sprintf("%s · %s · %s", stringOr(text(item, "key"), text(item, "id")), summaryMiddle, stringOr(text(item, "title"), text(checkpoint, "title")))

	args := []string{"task", "+create", "--as", "bot", "--summary", summary, "--description", description, "--idempotency-key", "omega-review-" + safeSegment(checkpointID)}
	if assignee := strings.TrimSpace(stringOr(options.AssigneeID, os.Getenv("OMEGA_FEISHU_REVIEW_ASSIGNEE_ID"))); assignee != "" {
		args = append(args, "--assignee", assignee)
	}
	if follower := strings.TrimSpace(stringOr(options.FollowerID, os.Getenv("OMEGA_FEISHU_REVIEW_FOLLOWER_ID"))); follower != "" {
		args = append(args, "--follower", follower)
	}
	if tasklist := strings.TrimSpace(stringOr(options.TasklistID, os.Getenv("OMEGA_FEISHU_REVIEW_TASKLIST_ID"))); tasklist != "" {
		args = append(args, "--tasklist-id", tasklist)
	}
	if due := strings.TrimSpace(stringOr(options.Due, os.Getenv("OMEGA_FEISHU_REVIEW_DUE"))); due != "" {
		args = append(args, "--due", due)
	}
	output, err := runLarkCLI(ctx, args...)
	result := map[string]any{
		"status":       "sent",
		"provider":     "feishu",
		"tool":         "lark-cli",
		"format":       "task-review",
		"checkpointId": checkpointID,
		"attemptId":    text(attempt, "id"),
		"nonce":        nonce,
		"docMode":      text(doc, "mode"),
		"doc":          doc,
		"raw":          strings.TrimSpace(output),
	}
	if err != nil {
		result["status"] = "failed"
		result["error"] = err.Error()
		return result, err
	}
	task := extractLarkTask(output)
	for key, value := range task {
		result[key] = value
	}
	taskGuid := stringOr(text(task, "taskGuid"), text(task, "taskId"))
	if taskGuid != "" {
		_, _ = sendFeishuTaskComment(ctx, taskGuid, renderFeishuReviewTaskInitialCommentForLanguage(packet, nonce, options.Language))
	}
	return result, nil
}

func createFeishuReviewDocIfConfigured(ctx context.Context, packet map[string]any, nonce string, options feishuReviewSendOptions) map[string]any {
	enabled := strings.ToLower(strings.TrimSpace(os.Getenv("OMEGA_FEISHU_REVIEW_CREATE_DOC")))
	folderToken := strings.TrimSpace(firstNonEmpty(options.DocFolderToken, os.Getenv("OMEGA_FEISHU_REVIEW_DOC_FOLDER_TOKEN")))
	docEnabled := options.CreateDoc || enabled == "1" || enabled == "true" || enabled == "yes" || folderToken != ""
	if !docEnabled {
		return map[string]any{"mode": "preview-only"}
	}
	item := mapValue(packet["item"])
	titleSuffix := feishuLabel(options.Language, "Human review", "人工审核")
	title := fmt.Sprintf("%s %s", stringOr(text(item, "key"), nonce), titleSuffix)
	content := buildFeishuReviewDocMarkdown(packet, options.Language) + "\n\n---\n\nReview token: `" + nonce + "`\n"
	tempDir := os.TempDir()
	path := filepath.Join(tempDir, "omega-feishu-review-"+safeSegment(nonce)+".md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return map[string]any{"mode": "failed", "error": err.Error()}
	}
	defer os.Remove(path)
	args := []string{"docs", "+create", "--as", "bot", "--title", title, "--markdown", "@" + path}
	if folderToken != "" {
		args = append(args, "--folder-token", folderToken)
	}
	output, err := runLarkCLI(ctx, args...)
	doc := extractLarkDoc(output)
	if err != nil {
		doc["mode"] = "failed"
		doc["error"] = err.Error()
		doc["raw"] = strings.TrimSpace(output)
		return doc
	}
	doc["mode"] = "created"
	doc["raw"] = strings.TrimSpace(output)
	return doc
}

func renderFeishuReviewTaskDescription(packet map[string]any, nonce string, doc map[string]any) string {
	return renderFeishuReviewTaskDescriptionForLanguage(packet, nonce, doc, "zh-CN")
}

func renderFeishuReviewTaskDescriptionForLanguage(packet map[string]any, nonce string, doc map[string]any, language string) string {
	lang := normalizeUILanguage(language)
	item := mapValue(packet["item"])
	attempt := mapValue(packet["attempt"])
	reviewPacket := mapValue(packet["reviewPacket"])
	requirement := mapValue(packet["requirement"])
	requirementText := stringOr(text(requirement, "description"), text(item, "description"))
	lines := []string{}
	if lang == "zh-CN" {
		lines = append(lines,
			"✅ Omega 人工审核",
			fmt.Sprintf("审核标识: %s", nonce),
			fmt.Sprintf("工作项: %s (%s) · %s", stringOr(text(item, "key"), text(item, "id")), stringOr(text(item, "id"), "unknown"), text(item, "title")),
			fmt.Sprintf("PR: %s", stringOr(text(attempt, "pullRequestUrl"), "not created")),
			fmt.Sprintf("分支: %s", text(attempt, "branchName")),
			"",
			"🛠️ 审核方式",
			"完成这条任务: 审核通过",
			"评论明确修改意见: Omega 同步为 request changes",
			"评论问题或阻塞原因: Omega 记录为 need-info",
			"",
			"📋 需求摘要",
			truncateForProof(requirementText, 900),
		)
	} else {
		lines = append(lines,
			"✅ Omega human review",
			fmt.Sprintf("Review token: %s", nonce),
			fmt.Sprintf("Work item: %s (%s) · %s", stringOr(text(item, "key"), text(item, "id")), stringOr(text(item, "id"), "unknown"), text(item, "title")),
			fmt.Sprintf("PR: %s", stringOr(text(attempt, "pullRequestUrl"), "not created")),
			fmt.Sprintf("Branch: %s", text(attempt, "branchName")),
			"",
			"🛠️ How to review",
			"Complete this task: approve",
			"Comment with specific requested changes: Omega records request changes",
			"Comment with questions or blockers: Omega records need-info",
			"",
			"📋 Requirement summary",
			truncateForProof(requirementText, 900),
		)
	}
	if url := stringOr(text(doc, "url"), text(doc, "docUrl")); url != "" {
		lines = append(lines, "", feishuLabel(lang, "📄 Review doc", "📄 审核文档"), url)
	}
	if summary := text(reviewPacket, "summary"); summary != "" {
		lines = append(lines, "", feishuLabel(lang, "🧾 Review packet", "🧾 审核包摘要"), truncateForProof(summary, 700))
	}
	return strings.Join(lines, "\n")
}

func renderFeishuReviewTaskInitialComment(packet map[string]any, nonce string) string {
	return renderFeishuReviewTaskInitialCommentForLanguage(packet, nonce, "zh-CN")
}

func renderFeishuReviewTaskInitialCommentForLanguage(packet map[string]any, nonce string, language string) string {
	item := mapValue(packet["item"])
	if normalizeUILanguage(language) == "zh-CN" {
		return fmt.Sprintf("✅ 这条飞书任务已绑定 Omega 工作项 %s：%s。完成任务表示审核通过；如需修改，请直接评论具体修改意见。请不要把这个审核标识复制到其他任务：%s。", text(item, "id"), text(item, "title"), nonce)
	}
	return fmt.Sprintf("✅ This Feishu task is bound to Omega work item %s: %s. Complete the task to approve; comment with specific changes to request changes. Do not copy this review token to another task: %s.", text(item, "id"), text(item, "title"), nonce)
}

func feishuReviewNonce(checkpointID string, attemptID string) string {
	return safeSegment(checkpointID + ":" + attemptID)
}

func runLarkCLI(ctx context.Context, args ...string) (string, error) {
	path, err := exec.LookPath("lark-cli")
	if err != nil {
		return "", fmt.Errorf("lark-cli not found")
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	output, err := exec.CommandContext(timeoutCtx, path, args...).CombinedOutput()
	trimmed := strings.TrimSpace(string(output))
	if err != nil {
		if trimmed == "" {
			trimmed = err.Error()
		}
		return trimmed, fmt.Errorf("lark-cli failed: %s", trimmed)
	}
	return trimmed, nil
}

func sendFeishuTaskComment(ctx context.Context, taskGuid string, content string) (map[string]any, error) {
	taskGuid = strings.TrimSpace(taskGuid)
	content = strings.TrimSpace(content)
	if taskGuid == "" || content == "" {
		return nil, nil
	}
	output, err := runLarkCLI(ctx, "task", "+comment", "--as", "bot", "--task-id", taskGuid, "--content", content)
	result := map[string]any{"taskGuid": taskGuid, "raw": strings.TrimSpace(output)}
	if err != nil {
		result["status"] = "failed"
		result["error"] = err.Error()
		return result, err
	}
	result["status"] = "sent"
	return result, nil
}

func (server *Server) feishuReviewTaskSync(response http.ResponseWriter, request *http.Request) {
	var payload struct {
		CheckpointID string `json:"checkpointId"`
	}
	_ = json.NewDecoder(request.Body).Decode(&payload)
	result, status, err := server.syncFeishuReviewTasks(request.Context(), payload.CheckpointID)
	if err != nil {
		writeJSON(response, status, map[string]any{"error": err.Error(), "result": result})
		return
	}
	writeJSON(response, status, result)
}

func (server *Server) feishuReviewTaskBridgeTick(response http.ResponseWriter, request *http.Request) {
	var payload struct {
		CheckpointID string `json:"checkpointId"`
		Limit        int    `json:"limit"`
		DryRun       bool   `json:"dryRun"`
	}
	_ = json.NewDecoder(request.Body).Decode(&payload)
	result, status, err := server.tickFeishuReviewTaskBridge(request.Context(), payload.CheckpointID, payload.Limit, payload.DryRun)
	if err != nil {
		writeJSON(response, status, map[string]any{"error": err.Error(), "result": result})
		return
	}
	writeJSON(response, status, result)
}

func (server *Server) tickFeishuReviewTaskBridge(ctx context.Context, checkpointID string, limit int, dryRun bool) (map[string]any, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if dryRun {
		database, err := server.Repo.LoadSupervisorExecutionState(ctx)
		if err != nil {
			return nil, http.StatusNotFound, err
		}
		pending := []map[string]any{}
		for _, checkpoint := range database.Tables.Checkpoints {
			if checkpointID != "" && text(checkpoint, "id") != checkpointID {
				continue
			}
			review := mapValue(checkpoint["feishuReview"])
			taskGuid := stringOr(text(review, "taskGuid"), text(review, "taskId"))
			if text(checkpoint, "status") == "pending" && taskGuid != "" {
				pending = append(pending, map[string]any{"checkpointId": text(checkpoint, "id"), "taskGuid": taskGuid, "updatedAt": text(checkpoint, "updatedAt")})
			}
			if len(pending) >= limit {
				break
			}
		}
		return map[string]any{"status": "dry-run", "pending": pending, "createdAt": nowISO()}, http.StatusOK, nil
	}
	result, status, err := server.syncFeishuReviewTasks(ctx, checkpointID)
	if err == nil {
		server.logInfo(ctx, "feishu.review_task.bridge_tick", "Feishu review task bridge tick completed.", map[string]any{"checkpointId": checkpointID, "status": text(result, "status")})
	}
	return result, status, err
}

func (server *Server) syncFeishuReviewTasks(ctx context.Context, checkpointID string) (map[string]any, int, error) {
	database, err := server.Repo.LoadSupervisorExecutionState(ctx)
	if err != nil {
		return nil, http.StatusNotFound, err
	}
	synced := []map[string]any{}
	skipped := 0
	for _, checkpoint := range database.Tables.Checkpoints {
		if checkpointID != "" && text(checkpoint, "id") != checkpointID {
			continue
		}
		if text(checkpoint, "status") != "pending" {
			skipped++
			continue
		}
		taskGuid := text(mapValue(checkpoint["feishuReview"]), "taskGuid")
		if taskGuid == "" {
			taskGuid = text(mapValue(checkpoint["feishuReview"]), "taskId")
		}
		if taskGuid == "" {
			skipped++
			continue
		}
		task, err := fetchFeishuTask(ctx, taskGuid)
		entry := map[string]any{"checkpointId": text(checkpoint, "id"), "taskGuid": taskGuid}
		if err != nil {
			entry["state"] = "failed"
			entry["error"] = err.Error()
			synced = append(synced, entry)
			continue
		}
		entry["task"] = task
		if feishuTaskIsDone(task) {
			updated, status, err := server.applyCheckpointDecision(ctx, text(checkpoint, "id"), "approved", map[string]any{"reviewer": "feishu-task", "asyncDelivery": true})
			entry["decision"] = "approved"
			entry["statusCode"] = status
			if err != nil {
				entry["state"] = "failed"
				entry["error"] = err.Error()
			} else {
				entry["state"] = "synced"
				entry["checkpoint"] = updated
				_, _ = sendFeishuTaskComment(ctx, taskGuid, "Omega 已把这条已完成任务同步为审核通过。")
			}
		} else {
			entry["state"] = "pending"
		}
		synced = append(synced, entry)
	}
	return map[string]any{"status": "ok", "synced": synced, "skipped": skipped, "createdAt": nowISO()}, http.StatusOK, nil
}

func fetchFeishuTask(ctx context.Context, taskGuid string) (map[string]any, error) {
	params, _ := json.Marshal(map[string]any{"task_guid": taskGuid, "user_id_type": "open_id"})
	output, err := runLarkCLI(ctx, "task", "tasks", "get", "--as", "bot", "--params", string(params))
	if err != nil {
		return nil, err
	}
	task := extractNestedMap(output, "task")
	if len(task) == 0 {
		task = parseJSONMap(output)
	}
	return task, nil
}

func (server *Server) feishuReviewTaskComment(response http.ResponseWriter, request *http.Request) {
	var payload struct {
		CheckpointID string `json:"checkpointId"`
		TaskGuid     string `json:"taskGuid"`
		TaskID       string `json:"taskId"`
		Comment      string `json:"comment"`
		Reviewer     string `json:"reviewer"`
		EventID      string `json:"eventId"`
	}
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	taskGuid := stringOr(payload.TaskGuid, payload.TaskID)
	result, status, err := server.applyFeishuReviewTaskComment(request.Context(), payload.CheckpointID, taskGuid, payload.Comment, payload.Reviewer, payload.EventID)
	if err != nil {
		writeJSON(response, status, map[string]any{"error": err.Error(), "result": result})
		return
	}
	writeJSON(response, status, result)
}

func (server *Server) applyFeishuReviewTaskComment(ctx context.Context, checkpointID string, taskGuid string, comment string, reviewer string, eventID string) (map[string]any, int, error) {
	comment = strings.TrimSpace(comment)
	if comment == "" {
		return map[string]any{"status": "ignored", "reason": "empty comment"}, http.StatusOK, nil
	}
	database, err := server.Repo.LoadSupervisorExecutionState(ctx)
	if err != nil {
		return nil, http.StatusNotFound, err
	}
	checkpoint, ok := findFeishuTaskCheckpoint(*database, checkpointID, taskGuid)
	if !ok {
		return nil, http.StatusNotFound, fmt.Errorf("linked checkpoint not found")
	}
	if text(checkpoint, "status") != "pending" {
		return map[string]any{"status": "ignored", "reason": "checkpoint is not pending", "checkpointId": text(checkpoint, "id")}, http.StatusOK, nil
	}
	classification := classifyFeishuReviewTaskComment(comment)
	result := map[string]any{
		"status":         "classified",
		"classification": classification,
		"checkpointId":   text(checkpoint, "id"),
		"taskGuid":       text(mapValue(checkpoint["feishuReview"]), "taskGuid"),
		"comment":        comment,
		"eventId":        eventID,
	}
	switch classification {
	case "request_changes":
		updated, status, err := server.applyCheckpointDecision(ctx, text(checkpoint, "id"), "rejected", map[string]any{"reviewer": stringOr(reviewer, "feishu-task"), "reason": comment, "asyncDelivery": true})
		result["decision"] = "rejected"
		result["checkpoint"] = updated
		if err == nil {
			_, _ = sendFeishuTaskComment(ctx, stringOr(taskGuid, text(mapValue(checkpoint["feishuReview"]), "taskGuid")), "Omega 已把这条任务评论同步为请求修改。")
		}
		return result, status, err
	case "need_info":
		updated, err := server.recordFeishuReviewTaskNeedInfo(ctx, text(checkpoint, "id"), comment, reviewer, eventID)
		result["decision"] = "need_info"
		result["checkpoint"] = updated
		if err != nil {
			return result, http.StatusInternalServerError, err
		}
		_, _ = sendFeishuTaskComment(ctx, stringOr(taskGuid, text(mapValue(checkpoint["feishuReview"]), "taskGuid")), "Omega 已把这条评论记录为需要补充信息，暂不改变审核状态。")
		return result, http.StatusOK, nil
	default:
		updated, err := server.recordFeishuReviewTaskComment(ctx, text(checkpoint, "id"), comment, reviewer, eventID, classification)
		result["decision"] = "recorded"
		result["checkpoint"] = updated
		if err != nil {
			return result, http.StatusInternalServerError, err
		}
		return result, http.StatusOK, nil
	}
}

func (server *Server) recordFeishuReviewTaskNeedInfo(ctx context.Context, checkpointID string, comment string, reviewer string, eventID string) (map[string]any, error) {
	return server.patchFeishuReviewTaskComment(ctx, checkpointID, map[string]any{"state": "need-info", "comment": comment, "reviewer": stringOr(reviewer, "feishu-task"), "eventId": eventID, "createdAt": nowISO()})
}

func (server *Server) recordFeishuReviewTaskComment(ctx context.Context, checkpointID string, comment string, reviewer string, eventID string, classification string) (map[string]any, error) {
	return server.patchFeishuReviewTaskComment(ctx, checkpointID, map[string]any{"state": classification, "comment": comment, "reviewer": stringOr(reviewer, "feishu-task"), "eventId": eventID, "createdAt": nowISO()})
}

func (server *Server) patchFeishuReviewTaskComment(ctx context.Context, checkpointID string, entry map[string]any) (map[string]any, error) {
	database, err := server.Repo.LoadSupervisorExecutionState(ctx)
	if err != nil {
		return nil, err
	}
	index := findByID(database.Tables.Checkpoints, checkpointID)
	if index < 0 {
		return nil, fmt.Errorf("checkpoint not found")
	}
	checkpoint := cloneMap(database.Tables.Checkpoints[index])
	review := mapValue(checkpoint["feishuReview"])
	review["lastComment"] = entry
	review["comments"] = append(arrayMaps(review["comments"]), entry)
	checkpoint["feishuReview"] = review
	checkpoint["updatedAt"] = nowISO()
	database.Tables.Checkpoints[index] = checkpoint
	touch(database)
	if err := server.Repo.SaveSupervisorExecutionState(ctx, *database); err != nil {
		return nil, err
	}
	return checkpoint, nil
}

func classifyFeishuReviewTaskComment(comment string) string {
	normalized := strings.ToLower(strings.TrimSpace(comment))
	if normalized == "" {
		return "empty"
	}
	ack := []string{"收到", "我看看", "等一下", "稍等", "ack", "ok", "好的", "看一下"}
	for _, value := range ack {
		if normalized == value {
			return "normal_comment"
		}
	}
	needInfoSignals := []string{"?", "？", "不清楚", "需要确认", "缺少", "怎么", "哪里来", "无法判断", "need info", "question"}
	for _, value := range needInfoSignals {
		if strings.Contains(normalized, value) {
			return "need_info"
		}
	}
	requestSignals := []string{"修改", "改成", "请改", "调整", "不对", "不通过", "request changes", "changes:", "change:", "fix", "revise"}
	for _, value := range requestSignals {
		if strings.Contains(normalized, value) {
			return "request_changes"
		}
	}
	return "request_changes"
}

func findFeishuTaskCheckpoint(database WorkspaceDatabase, checkpointID string, taskGuid string) (map[string]any, bool) {
	for _, checkpoint := range database.Tables.Checkpoints {
		if checkpointID != "" && text(checkpoint, "id") == checkpointID {
			return cloneMap(checkpoint), true
		}
		if taskGuid != "" {
			review := mapValue(checkpoint["feishuReview"])
			if text(review, "taskGuid") == taskGuid || text(review, "taskId") == taskGuid {
				return cloneMap(checkpoint), true
			}
		}
	}
	return nil, false
}

func feishuTaskIsDone(task map[string]any) bool {
	status := strings.ToLower(strings.TrimSpace(text(task, "status")))
	switch status {
	case "done", "completed":
		return true
	case "":
		return meaningfulFeishuCompletedAt(task["completed_at"]) || meaningfulFeishuCompletedAt(task["completedAt"])
	default:
		return false
	}
}

func meaningfulFeishuCompletedAt(value any) bool {
	raw := strings.TrimSpace(fmt.Sprint(value))
	if value == nil || raw == "" || raw == "<nil>" {
		return false
	}
	normalized := strings.Trim(raw, "\"")
	normalized = strings.TrimSpace(normalized)
	return normalized != "" && normalized != "0" && normalized != "null"
}

func extractLarkTask(output string) map[string]any {
	payload := parseJSONMap(output)
	task := extractMapValue(payload, "task")
	if len(task) == 0 {
		task = payload
	}
	guid := firstNestedString(task, "guid", "task_guid", "taskGuid", "task_id", "taskId")
	url := firstNestedString(task, "url", "appLink", "app_link")
	return map[string]any{
		"taskGuid": guid,
		"taskId":   stringOr(firstNestedString(task, "task_id", "taskId"), guid),
		"taskUrl":  url,
		"taskRaw":  task,
	}
}

func extractLarkDoc(output string) map[string]any {
	payload := parseJSONMap(output)
	return map[string]any{
		"url":      firstNestedString(payload, "url", "docUrl", "document_url", "app_link"),
		"token":    firstNestedString(payload, "token", "document_id", "doc_token", "obj_token"),
		"revision": firstNestedString(payload, "revision_id", "revisionId"),
	}
}

func extractNestedMap(output string, key string) map[string]any {
	return extractMapValue(parseJSONMap(output), key)
}

func parseJSONMap(output string) map[string]any {
	var payload map[string]any
	trimmed := strings.TrimSpace(output)
	if err := json.Unmarshal([]byte(trimmed), &payload); err == nil && payload != nil {
		return payload
	}
	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start >= 0 && end > start {
		if err := json.Unmarshal([]byte(trimmed[start:end+1]), &payload); err == nil && payload != nil {
			return payload
		}
	}
	return map[string]any{}
}

func extractMapValue(payload map[string]any, key string) map[string]any {
	if len(payload) == 0 {
		return map[string]any{}
	}
	if value := mapValue(payload[key]); len(value) > 0 {
		return value
	}
	for _, value := range payload {
		switch typed := value.(type) {
		case map[string]any:
			if found := extractMapValue(typed, key); len(found) > 0 {
				return found
			}
		case []any:
			for _, item := range typed {
				if found := extractMapValue(mapValue(item), key); len(found) > 0 {
					return found
				}
			}
		}
	}
	return map[string]any{}
}

func firstNestedString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(stringOr(payload[key], "")); value != "" {
			return value
		}
	}
	for _, value := range payload {
		switch typed := value.(type) {
		case map[string]any:
			if found := firstNestedString(typed, keys...); found != "" {
				return found
			}
		case []any:
			for _, item := range typed {
				if found := firstNestedString(mapValue(item), keys...); found != "" {
					return found
				}
			}
		}
	}
	return ""
}
