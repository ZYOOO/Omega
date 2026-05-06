package omegalocal

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func (server *Server) feishuReviewRequest(response http.ResponseWriter, request *http.Request) {
	var payload struct {
		CheckpointID string `json:"checkpointId"`
		ChatID       string `json:"chatId"`
		Mode         string `json:"mode"`
		AssigneeID   string `json:"assigneeId"`
		TasklistID   string `json:"tasklistId"`
		FollowerID   string `json:"followerId"`
		Due          string `json:"due"`
	}
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	result, status, err := server.sendFeishuReviewForCheckpointWithOptions(request.Context(), payload.CheckpointID, feishuReviewSendOptions{
		ChatID:     payload.ChatID,
		Mode:       payload.Mode,
		AssigneeID: payload.AssigneeID,
		TasklistID: payload.TasklistID,
		FollowerID: payload.FollowerID,
		Due:        payload.Due,
	}, true)
	if err != nil {
		writeJSON(response, status, map[string]any{"error": err.Error(), "result": result})
		return
	}
	writeJSON(response, status, result)
}

func (server *Server) feishuReviewCallback(response http.ResponseWriter, request *http.Request) {
	var payload map[string]any
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	if challenge := strings.TrimSpace(text(payload, "challenge")); challenge != "" {
		writeJSON(response, http.StatusOK, map[string]any{"challenge": challenge})
		return
	}
	if !feishuReviewTokenAllowed(request, payload) {
		writeJSON(response, http.StatusUnauthorized, map[string]any{"error": "invalid Feishu review token"})
		return
	}
	value := feishuReviewCallbackValue(payload)
	event := mapValue(payload["event"])
	operator := firstNonEmpty(
		text(mapValue(event["operator"]), "open_id"),
		text(mapValue(event["operator"]), "user_id"),
		text(mapValue(payload["operator"]), "open_id"),
		stringOr(payload["operator"], ""),
	)
	checkpointID := strings.TrimSpace(stringOr(payload["checkpointId"], text(value, "checkpointId")))
	action := strings.ToLower(strings.TrimSpace(stringOr(payload["action"], text(value, "action"))))
	reviewer := stringOr(payload["reviewer"], stringOr(operator, "feishu-reviewer"))
	reason := stringOr(payload["reason"], stringOr(payload["comment"], "changes requested from Feishu"))
	if checkpointID == "" {
		writeJSON(response, http.StatusBadRequest, map[string]any{"error": "checkpointId is required"})
		return
	}
	asyncDeliveryValue := payload["asyncDelivery"]
	if asyncDeliveryValue == nil {
		asyncDeliveryValue = value["asyncDelivery"]
	}
	decisionPayload := map[string]any{"reviewer": reviewer, "reason": reason, "asyncDelivery": boolValueDefault(asyncDeliveryValue, true)}
	switch action {
	case "approve", "approved":
		checkpoint, status, err := server.applyCheckpointDecision(request.Context(), checkpointID, "approved", decisionPayload)
		if err != nil {
			writeJSON(response, status, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(response, http.StatusOK, map[string]any{"status": "approved", "checkpoint": checkpoint})
	case "request_changes", "request-changes", "changes_requested", "reject", "rejected":
		checkpoint, status, err := server.applyCheckpointDecision(request.Context(), checkpointID, "rejected", decisionPayload)
		if err != nil {
			writeJSON(response, status, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(response, http.StatusOK, map[string]any{"status": "rejected", "checkpoint": checkpoint})
	default:
		writeJSON(response, http.StatusBadRequest, map[string]any{"error": "unsupported review action"})
	}
}

func feishuReviewCallbackValue(payload map[string]any) map[string]any {
	if value := mapValue(payload["value"]); len(value) > 0 {
		return value
	}
	event := mapValue(payload["event"])
	action := mapValue(event["action"])
	if value := mapValue(action["value"]); len(value) > 0 {
		return value
	}
	if value := mapValue(payload["action"]); len(value) > 0 {
		if nested := mapValue(value["value"]); len(nested) > 0 {
			return nested
		}
		return value
	}
	return map[string]any{}
}

func feishuReviewTokenAllowed(request *http.Request, payload map[string]any) bool {
	expected := strings.TrimSpace(os.Getenv("OMEGA_FEISHU_REVIEW_TOKEN"))
	if expected == "" {
		return true
	}
	candidates := []string{
		request.Header.Get("X-Omega-Feishu-Token"),
		request.URL.Query().Get("token"),
		stringOr(payload["token"], ""),
		text(mapValue(payload["value"]), "token"),
		text(feishuReviewCallbackValue(payload), "token"),
		text(mapValue(payload["event"]), "token"),
	}
	for _, candidate := range candidates {
		if hmac.Equal([]byte(strings.TrimSpace(candidate)), []byte(expected)) {
			return true
		}
	}
	return false
}

func (server *Server) sendFeishuReviewForPipelineIfConfigured(ctx context.Context, pipelineID string) {
	if feishuAutoDeliveryDisabled() {
		server.logDebug(ctx, "feishu.review.auto_delivery_disabled", "Feishu automatic review delivery is disabled.", map[string]any{"pipelineId": pipelineID})
		return
	}
	taskOptions := server.mergeFeishuReviewOptions(ctx, feishuReviewSendOptions{})
	checkpoints, err := server.Repo.ListCheckpoints(ctx, map[string]string{
		"pipelineId": pipelineID,
		"stageId":    "human_review",
		"status":     "pending",
		"limit":      "1",
	})
	if err != nil {
		server.logError(ctx, "feishu.review.checkpoint_lookup_failed", err.Error(), map[string]any{"pipelineId": pipelineID})
		return
	}
	if len(checkpoints) == 0 {
		return
	}
	checkpoint := checkpoints[0]
	if checkpointFeishuReviewAlreadySent(checkpoint) {
		server.logInfo(ctx, "feishu.review.skipped_already_sent", "Feishu review notification was already recorded for this checkpoint.", map[string]any{"checkpointId": text(checkpoint, "id"), "pipelineId": pipelineID, "format": text(mapValue(checkpoint["feishuReview"]), "format")})
		return
	}
	if taskOptions.ChatID == "" && taskOptions.WebhookURL == "" && taskOptions.DirectUserID == "" && !feishuReviewTaskModeEnabled(taskOptions) {
		server.logInfo(ctx, "feishu.review.needs_target", "Feishu review notification needs a chat, task assignee, tasklist, webhook target, or current-user lark-cli auth.", map[string]any{"pipelineId": pipelineID, "checkpointId": text(checkpoint, "id")})
	}
	_, _, _ = server.sendFeishuReviewForCheckpointWithOptions(ctx, text(checkpoint, "id"), taskOptions, false)
}

func checkpointFeishuReviewAlreadySent(checkpoint map[string]any) bool {
	review := mapValue(checkpoint["feishuReview"])
	if feishuReviewAlreadySent(review) {
		return true
	}
	status := strings.ToLower(strings.TrimSpace(text(checkpoint, "feishuReviewStatus")))
	if status == "sent" {
		return true
	}
	if status == "failed" || status == "needs-configuration" || status == "skipped" {
		return false
	}
	return firstNonEmpty(text(checkpoint, "feishuTaskGuid"), text(checkpoint, "feishuTaskId"), text(checkpoint, "feishuMessageId")) != ""
}

func checkpointFeishuReviewSentResult(checkpoint map[string]any) map[string]any {
	result := cloneMap(mapValue(checkpoint["feishuReview"]))
	for key, value := range map[string]string{
		"status":    "feishuReviewStatus",
		"taskGuid":  "feishuTaskGuid",
		"taskId":    "feishuTaskId",
		"messageId": "feishuMessageId",
	} {
		if strings.TrimSpace(text(result, key)) == "" {
			if candidate := text(checkpoint, value); candidate != "" {
				result[key] = candidate
			}
		}
	}
	result["status"] = stringOr(text(result, "status"), "sent")
	result["skippedReason"] = "already-sent"
	return result
}

func (server *Server) sendFeishuAttemptFailureIfConfigured(ctx context.Context, pipelineID string, attemptID string) {
	if feishuAutoDeliveryDisabled() {
		server.logDebug(ctx, "feishu.failure.auto_delivery_disabled", "Feishu automatic failure delivery is disabled.", map[string]any{"pipelineId": pipelineID, "attemptId": attemptID})
		return
	}
	attempts, err := server.Repo.ListAttempts(ctx, map[string]string{"id": attemptID, "limit": "1"})
	if err != nil {
		server.logError(ctx, "feishu.failure.attempt_lookup_failed", err.Error(), map[string]any{"pipelineId": pipelineID, "attemptId": attemptID})
		return
	}
	if len(attempts) == 0 {
		return
	}
	if attemptFeishuFailureAlreadySent(attempts[0]) {
		return
	}
	databasePtr, err := server.Repo.LoadSupervisorExecutionState(ctx)
	if err != nil {
		server.logError(ctx, "feishu.failure.load_failed", err.Error(), map[string]any{"pipelineId": pipelineID, "attemptId": attemptID})
		return
	}
	database := *databasePtr
	attemptIndex := findByID(database.Tables.Attempts, attemptID)
	if attemptIndex < 0 {
		return
	}
	attempt := database.Tables.Attempts[attemptIndex]
	if attemptFeishuFailureAlreadySent(attempt) {
		return
	}
	pipeline := pipelineByID(database, pipelineID)
	item := findWorkItem(database, text(attempt, "itemId"))
	requirement := workpadRequirement(database, item)
	repositoryTargetID := firstNonEmpty(text(attempt, "repositoryTargetId"), text(item, "repositoryTargetId"))
	repositoryTarget := findRepositoryTarget(database, repositoryTargetID)
	options := server.mergeFeishuReviewOptions(ctx, feishuReviewSendOptions{})
	result, err := sendFeishuFailurePacket(ctx, map[string]any{
		"pipeline":         pipeline,
		"attempt":          attempt,
		"item":             item,
		"requirement":      requirement,
		"repositoryTarget": repositoryTarget,
	}, options)
	if err != nil {
		server.logError(ctx, "feishu.failure.send_failed", err.Error(), map[string]any{"pipelineId": pipelineID, "attemptId": attemptID, "workItemId": text(attempt, "itemId")})
		return
	}
	if text(result, "status") == "sent" {
		nextAttempt := cloneMap(attempt)
		nextAttempt["feishuFailureNotifiedAt"] = nowISO()
		nextAttempt["feishuFailure"] = result
		delete(nextAttempt, "feishuFailureNotifyPending")
		database.Tables.Attempts[attemptIndex] = nextAttempt
		touch(&database)
		_ = server.Repo.SaveSupervisorExecutionState(ctx, database)
	}
	server.logInfo(ctx, "feishu.failure.synced", "Feishu failure notification state recorded.", map[string]any{
		"pipelineId": pipelineID,
		"attemptId":  attemptID,
		"workItemId": text(attempt, "itemId"),
		"status":     text(result, "status"),
		"provider":   text(result, "provider"),
		"route":      text(result, "route"),
		"messageId":  text(result, "messageId"),
	})
}

func attemptFeishuFailureAlreadySent(attempt map[string]any) bool {
	if text(attempt, "feishuFailureNotifiedAt") != "" {
		return true
	}
	failure := mapValue(attempt["feishuFailure"])
	status := strings.ToLower(strings.TrimSpace(firstNonEmpty(text(attempt, "feishuFailureStatus"), text(failure, "status"))))
	if status == "sent" {
		return true
	}
	if status == "failed" || status == "needs-configuration" || status == "skipped" {
		return false
	}
	return firstNonEmpty(text(attempt, "feishuFailureMessageId"), text(failure, "messageId")) != ""
}

func (server *Server) sendFeishuReviewForCheckpoint(ctx context.Context, checkpointID string, chatID string, manual bool) (map[string]any, int, error) {
	return server.sendFeishuReviewForCheckpointWithOptions(ctx, checkpointID, feishuReviewSendOptions{ChatID: chatID}, manual)
}

type feishuReviewSendOptions struct {
	ChatID         string
	DirectUserID   string
	Mode           string
	AssigneeID     string
	TasklistID     string
	FollowerID     string
	Due            string
	WebhookURL     string
	WebhookSecret  string
	ReviewToken    string
	CreateDoc      bool
	DocFolderToken string
	Language       string
}

func feishuAutoDeliveryDisabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("OMEGA_FEISHU_AUTO_DELIVERY_DISABLED")))
	return value == "1" || value == "true" || value == "yes"
}

func (server *Server) sendFeishuReviewForCheckpointWithOptions(ctx context.Context, checkpointID string, options feishuReviewSendOptions, manual bool) (map[string]any, int, error) {
	checkpointID = strings.TrimSpace(checkpointID)
	if checkpointID == "" {
		return nil, http.StatusBadRequest, fmt.Errorf("checkpointId is required")
	}
	if !manual {
		if receipt, ok := server.sentFeishuReviewDeliveryReceipt(ctx, checkpointID); ok {
			server.logInfo(ctx, "feishu.review.skipped_already_sent", "Feishu review delivery receipt already exists for this checkpoint.", map[string]any{"checkpointId": checkpointID, "pipelineId": text(receipt, "pipelineId"), "format": text(receipt, "format")})
			return feishuReviewSentResultFromReceipt(receipt), http.StatusOK, nil
		}
		checkpoints, err := server.Repo.ListCheckpoints(ctx, map[string]string{"id": checkpointID, "limit": "1"})
		if err == nil && len(checkpoints) > 0 && checkpointFeishuReviewAlreadySent(checkpoints[0]) {
			existing := checkpointFeishuReviewSentResult(checkpoints[0])
			server.logInfo(ctx, "feishu.review.skipped_already_sent", "Feishu review notification was already recorded for this checkpoint.", map[string]any{"checkpointId": checkpointID, "pipelineId": text(checkpoints[0], "pipelineId"), "format": text(existing, "format")})
			return existing, http.StatusOK, nil
		}
		if err != nil {
			server.logError(ctx, "feishu.review.checkpoint_lookup_failed", err.Error(), map[string]any{"checkpointId": checkpointID})
		}
	}
	databasePtr, err := server.Repo.LoadSupervisorExecutionState(ctx)
	if err != nil {
		return nil, http.StatusNotFound, err
	}
	database := *databasePtr
	checkpointIndex := findByID(database.Tables.Checkpoints, checkpointID)
	if checkpointIndex < 0 {
		return nil, http.StatusNotFound, fmt.Errorf("checkpoint not found")
	}
	checkpoint := cloneMap(database.Tables.Checkpoints[checkpointIndex])
	if !manual {
		if checkpointFeishuReviewAlreadySent(checkpoint) {
			existing := checkpointFeishuReviewSentResult(checkpoint)
			server.logInfo(ctx, "feishu.review.skipped_already_sent", "Feishu review notification was already recorded for this checkpoint.", map[string]any{"checkpointId": checkpointID, "pipelineId": text(checkpoint, "pipelineId"), "format": text(existing, "format")})
			return existing, http.StatusOK, nil
		}
	}
	pipeline := map[string]any{}
	if index := findByID(database.Tables.Pipelines, text(checkpoint, "pipelineId")); index >= 0 {
		pipeline = cloneMap(database.Tables.Pipelines[index])
	}
	item := findWorkItem(database, text(pipeline, "workItemId"))
	attempt := map[string]any{}
	if attemptIndex := attemptIndexForCheckpoint(database, checkpoint); attemptIndex >= 0 {
		attempt = cloneMap(database.Tables.Attempts[attemptIndex])
	}
	packet := feishuReviewPacketFromRecords(database, checkpoint, pipeline, item, attempt)
	result, err := sendFeishuReviewPacket(ctx, packet, server.mergeFeishuReviewOptions(ctx, options))
	if err != nil {
		server.logError(ctx, "feishu.review.send_failed", err.Error(), map[string]any{"checkpointId": checkpointID, "pipelineId": text(pipeline, "id"), "attemptId": text(attempt, "id")})
		if manual {
			return result, http.StatusServiceUnavailable, err
		}
	}
	if result == nil {
		result = map[string]any{"status": "skipped"}
	}
	server.recordFeishuReviewDeliveryReceipt(ctx, checkpointID, result, checkpoint, pipeline, item, attempt)
	checkpoint["feishuReview"] = result
	checkpoint["updatedAt"] = nowISO()
	database.Tables.Checkpoints[checkpointIndex] = checkpoint
	touch(&database)
	if saveErr := server.Repo.SaveSupervisorExecutionState(ctx, database); saveErr != nil {
		return result, http.StatusInternalServerError, saveErr
	}
	server.logInfo(ctx, "feishu.review.synced", "Feishu review notification state recorded.", map[string]any{
		"checkpointId": checkpointID,
		"pipelineId":   text(pipeline, "id"),
		"attemptId":    text(attempt, "id"),
		"status":       text(result, "status"),
		"provider":     text(result, "provider"),
	})
	if text(result, "status") == "needs-configuration" {
		return result, http.StatusAccepted, nil
	}
	return result, http.StatusOK, err
}

func feishuReviewDeliveryKey(checkpointID string) string {
	return "human_review:" + strings.TrimSpace(checkpointID)
}

func (server *Server) sentFeishuReviewDeliveryReceipt(ctx context.Context, checkpointID string) (map[string]any, bool) {
	receipt, err := server.Repo.GetFeishuDeliveryReceipt(ctx, feishuReviewDeliveryKey(checkpointID))
	if err != nil {
		return nil, false
	}
	status := strings.ToLower(strings.TrimSpace(text(receipt, "status")))
	if status != "sent" {
		return nil, false
	}
	return receipt, true
}

func feishuReviewSentResultFromReceipt(receipt map[string]any) map[string]any {
	result := cloneMap(mapValue(receipt["payload"]))
	if len(result) == 0 {
		result = map[string]any{}
	}
	for key, value := range map[string]string{
		"status":       "status",
		"provider":     "provider",
		"format":       "format",
		"route":        "route",
		"taskGuid":     "taskGuid",
		"taskId":       "taskId",
		"messageId":    "messageId",
		"checkpointId": "entityId",
		"attemptId":    "attemptId",
	} {
		if strings.TrimSpace(text(result, key)) == "" {
			if candidate := text(receipt, value); candidate != "" {
				result[key] = candidate
			}
		}
	}
	result["status"] = "sent"
	result["provider"] = stringOr(text(result, "provider"), "feishu")
	result["skippedReason"] = "delivery-receipt-already-sent"
	return result
}

func (server *Server) recordFeishuReviewDeliveryReceipt(ctx context.Context, checkpointID string, result map[string]any, checkpoint map[string]any, pipeline map[string]any, item map[string]any, attempt map[string]any) {
	if strings.ToLower(strings.TrimSpace(text(result, "status"))) != "sent" {
		return
	}
	receipt := map[string]any{
		"dedupeKey":  feishuReviewDeliveryKey(checkpointID),
		"provider":   stringOr(text(result, "provider"), "feishu"),
		"kind":       "human_review",
		"entityId":   checkpointID,
		"pipelineId": firstNonEmpty(text(pipeline, "id"), text(checkpoint, "pipelineId")),
		"attemptId":  firstNonEmpty(text(attempt, "id"), text(checkpoint, "attemptId")),
		"workItemId": firstNonEmpty(text(item, "id"), text(pipeline, "workItemId")),
		"status":     "sent",
		"route":      text(result, "route"),
		"format":     text(result, "format"),
		"taskGuid":   text(result, "taskGuid"),
		"taskId":     text(result, "taskId"),
		"messageId":  text(result, "messageId"),
		"payload":    result,
		"createdAt":  nowISO(),
	}
	if err := server.Repo.UpsertFeishuDeliveryReceipt(ctx, receipt); err != nil {
		server.logError(ctx, "feishu.review.delivery_receipt_failed", err.Error(), map[string]any{"checkpointId": checkpointID})
	}
}

func feishuReviewAlreadySent(review map[string]any) bool {
	status := strings.ToLower(strings.TrimSpace(text(review, "status")))
	if status == "sent" {
		return true
	}
	if status == "failed" || status == "needs-configuration" || status == "skipped" {
		return false
	}
	return firstNonEmpty(text(review, "taskGuid"), text(review, "taskId"), text(review, "messageId")) != ""
}

func (server *Server) mergeFeishuReviewOptions(ctx context.Context, options feishuReviewSendOptions) feishuReviewSendOptions {
	config, _ := server.feishuConfig(ctx)
	if strings.TrimSpace(options.ChatID) == "" {
		options.ChatID = config.ChatID
	}
	if strings.TrimSpace(options.Mode) == "" {
		options.Mode = config.Mode
	}
	if strings.TrimSpace(options.AssigneeID) == "" {
		options.AssigneeID = config.AssigneeID
	}
	if strings.TrimSpace(options.TasklistID) == "" {
		options.TasklistID = config.TasklistID
	}
	if strings.TrimSpace(options.FollowerID) == "" {
		options.FollowerID = config.FollowerID
	}
	if strings.TrimSpace(options.Due) == "" {
		options.Due = config.Due
	}
	if strings.TrimSpace(options.WebhookURL) == "" {
		options.WebhookURL = config.WebhookURL
	}
	if strings.TrimSpace(options.WebhookSecret) == "" {
		options.WebhookSecret = firstNonEmpty(server.decryptFeishuSecret(config, "feishu-webhook-secret"), os.Getenv("OMEGA_FEISHU_WEBHOOK_SECRET"))
	}
	if strings.TrimSpace(options.ReviewToken) == "" {
		options.ReviewToken = firstNonEmpty(server.decryptFeishuSecret(config, "feishu-review-token"), os.Getenv("OMEGA_FEISHU_REVIEW_TOKEN"))
	}
	if !options.CreateDoc {
		options.CreateDoc = config.CreateDoc
	}
	if strings.TrimSpace(options.DocFolderToken) == "" {
		options.DocFolderToken = config.DocFolderToken
	}
	if strings.TrimSpace(options.Language) == "" {
		options.Language = server.currentUILanguage(ctx)
	}
	if strings.TrimSpace(options.DirectUserID) == "" && !feishuReviewTaskModeEnabled(options) && !feishuCardCallbackReady() {
		options.DirectUserID = server.currentFeishuUserID(ctx)
	}
	if strings.TrimSpace(options.DirectUserID) == "" && strings.TrimSpace(options.ChatID) == "" && strings.TrimSpace(options.WebhookURL) == "" && !feishuReviewTaskModeEnabled(options) {
		options.DirectUserID = server.currentFeishuUserID(ctx)
	}
	return options
}

func (server *Server) currentFeishuUserID(ctx context.Context) string {
	path, err := exec.LookPath("lark-cli")
	if err != nil {
		return ""
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	users, _, err := getCurrentFeishuUserWithLarkCLI(timeoutCtx, path)
	if err != nil || len(users) == 0 {
		return ""
	}
	for _, user := range users {
		if strings.TrimSpace(user.OpenID) != "" {
			return strings.TrimSpace(user.OpenID)
		}
	}
	for _, user := range users {
		if strings.TrimSpace(user.UserID) != "" {
			return strings.TrimSpace(user.UserID)
		}
	}
	return ""
}

func feishuReviewPacketFromRecords(database WorkspaceDatabase, checkpoint map[string]any, pipeline map[string]any, item map[string]any, attempt map[string]any) map[string]any {
	reviewPacket := mapValue(attempt["reviewPacket"])
	runWorkpad := map[string]any{}
	for _, workpad := range database.Tables.RunWorkpads {
		if text(workpad, "attemptId") == text(attempt, "id") {
			runWorkpad = workpad
			break
		}
	}
	requirement := map[string]any{}
	if requirementID := text(item, "requirementId"); requirementID != "" {
		if index := findByID(database.Tables.Requirements, requirementID); index >= 0 {
			requirement = database.Tables.Requirements[index]
		}
	}
	return map[string]any{
		"checkpoint":   checkpoint,
		"pipeline":     pipeline,
		"item":         item,
		"attempt":      attempt,
		"reviewPacket": reviewPacket,
		"runWorkpad":   runWorkpad,
		"requirement":  requirement,
	}
}

func sendFeishuReviewPacket(ctx context.Context, packet map[string]any, options feishuReviewSendOptions) (map[string]any, error) {
	card := buildFeishuReviewCardWithOptions(packet, options)
	docMarkdown := buildFeishuReviewDocMarkdown(packet, options.Language)
	callbackReady := feishuCardCallbackReady()
	webhook := strings.TrimSpace(options.WebhookURL)
	if webhook == "" {
		webhook = strings.TrimSpace(os.Getenv("OMEGA_FEISHU_WEBHOOK_URL"))
	}
	if webhook == "" {
		webhook = strings.TrimSpace(os.Getenv("FEISHU_BOT_WEBHOOK"))
	}
	if feishuReviewTaskModeEnabled(options) {
		result, err := sendFeishuReviewTask(ctx, packet, options)
		if result != nil {
			result["docPreview"] = truncateForProof(docMarkdown, 1200)
		}
		return result, err
	}
	if !callbackReady {
		directUserID := strings.TrimSpace(options.DirectUserID)
		if directUserID != "" {
			taskOptions := options
			taskOptions.AssigneeID = firstNonEmpty(taskOptions.AssigneeID, directUserID)
			result, err := sendFeishuReviewTask(ctx, packet, taskOptions)
			if result != nil {
				result["route"] = "direct-user"
				result["fallback"] = "current-user-task"
				result["docPreview"] = truncateForProof(docMarkdown, 1200)
				if !feishuReviewTaskBridgeEnabled() {
					result["syncHint"] = "Complete the Feishu task, then run /feishu/review-task/sync or enable OMEGA_FEISHU_TASK_BRIDGE_ENABLED=true."
				}
			}
			if err == nil && result != nil {
				return result, nil
			}
		}
	}
	if webhook != "" {
		result, err := sendFeishuWebhookInteractiveCardWithSecret(ctx, webhook, options.WebhookSecret, card)
		result["docMode"] = "card-summary"
		result["docPreview"] = truncateForProof(docMarkdown, 1200)
		return result, err
	}
	chatID := strings.TrimSpace(options.ChatID)
	if chatID != "" {
		if !callbackReady {
			result, err := sendFeishuText(ctx, chatID, renderFeishuReviewText(packet, options.Language))
			if result != nil {
				result["format"] = "text-fallback"
				result["docMode"] = "text-summary"
				result["fallback"] = "card-callback-unavailable"
				result["syncHint"] = "No public Feishu Card Request URL is configured. Approve in Omega Web or configure Task review mode."
			}
			return result, err
		}
		result, cardErr := sendFeishuInteractiveCard(ctx, chatID, card)
		if result != nil {
			result["docMode"] = "card-summary"
			result["docPreview"] = truncateForProof(docMarkdown, 1200)
			return result, cardErr
		}
		result, err := sendFeishuText(ctx, chatID, renderFeishuReviewText(packet, options.Language))
		if result != nil {
			result["format"] = "text-fallback"
			result["docMode"] = "text-summary"
			if cardErr != nil {
				result["interactiveCardError"] = cardErr.Error()
			}
		}
		return result, err
	}
	directUserID := strings.TrimSpace(options.DirectUserID)
	if directUserID != "" {
		if !callbackReady {
			taskOptions := options
			taskOptions.AssigneeID = firstNonEmpty(taskOptions.AssigneeID, directUserID)
			result, err := sendFeishuReviewTask(ctx, packet, taskOptions)
			if err == nil && result != nil {
				result["route"] = "direct-user"
				result["fallback"] = "current-user-task"
				result["docPreview"] = truncateForProof(docMarkdown, 1200)
				if !feishuReviewTaskBridgeEnabled() {
					result["syncHint"] = "Complete the Feishu task, then run /feishu/review-task/sync or enable OMEGA_FEISHU_TASK_BRIDGE_ENABLED=true."
				}
				return result, nil
			}
			result, cardErr := sendFeishuInteractiveCardToUser(ctx, directUserID, card)
			if result != nil {
				result["docMode"] = "card-summary"
				result["docPreview"] = truncateForProof(docMarkdown, 1200)
				result["fallback"] = "current-user"
				if err != nil {
					result["taskReviewError"] = err.Error()
				}
				return result, cardErr
			}
			return result, err
		}
		result, cardErr := sendFeishuInteractiveCardToUser(ctx, directUserID, card)
		if result != nil {
			result["docMode"] = "card-summary"
			result["docPreview"] = truncateForProof(docMarkdown, 1200)
			result["fallback"] = "current-user"
			return result, cardErr
		}
		result, err := sendFeishuTextToUser(ctx, directUserID, renderFeishuReviewText(packet, options.Language))
		if result != nil {
			result["format"] = "text-fallback"
			result["docMode"] = "text-summary"
			result["fallback"] = "current-user"
			if cardErr != nil {
				result["interactiveCardError"] = cardErr.Error()
			}
		}
		return result, err
	}
	return map[string]any{
		"status":       "needs-configuration",
		"provider":     "feishu",
		"reason":       "Set OMEGA_FEISHU_WEBHOOK_URL, OMEGA_FEISHU_REVIEW_CHAT_ID, task review options, or run lark-cli auth login for current-user direct delivery.",
		"cardPreview":  card,
		"docPreview":   truncateForProof(docMarkdown, 2000),
		"checkpointId": text(mapValue(packet["checkpoint"]), "id"),
	}, nil
}

func feishuCardCallbackEnabled() bool {
	for _, key := range []string{"OMEGA_FEISHU_CARD_CALLBACK_ENABLED", "OMEGA_FEISHU_INTERACTIVE_CALLBACK_ENABLED"} {
		value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
		if value == "1" || value == "true" || value == "yes" {
			return true
		}
	}
	return false
}

func feishuCardCallbackReady() bool {
	return feishuCardCallbackEnabled() && strings.TrimSpace(os.Getenv("OMEGA_PUBLIC_API_URL")) != ""
}

func sendFeishuFailurePacket(ctx context.Context, packet map[string]any, options feishuReviewSendOptions) (map[string]any, error) {
	message := renderFeishuFailureText(packet, options.Language)
	title := "Omega run needs attention"
	if normalizeUILanguage(options.Language) == "zh-CN" {
		title = "Omega 运行需要处理"
	}
	if webhook := strings.TrimSpace(firstNonEmpty(options.WebhookURL, os.Getenv("OMEGA_FEISHU_WEBHOOK_URL"), os.Getenv("FEISHU_BOT_WEBHOOK"))); webhook != "" {
		card := map[string]any{
			"config": map[string]any{"wide_screen_mode": true},
			"header": map[string]any{
				"template": "red",
				"title":    map[string]any{"tag": "plain_text", "content": title},
			},
			"elements": []any{
				map[string]any{"tag": "markdown", "content": message},
			},
		}
		result, err := sendFeishuWebhookInteractiveCardWithSecret(ctx, webhook, options.WebhookSecret, card)
		if result != nil {
			result["format"] = "failure-card"
		}
		return result, err
	}
	if chatID := strings.TrimSpace(options.ChatID); chatID != "" {
		return sendFeishuText(ctx, chatID, message)
	}
	if directUserID := strings.TrimSpace(options.DirectUserID); directUserID != "" {
		result, err := sendFeishuTextToUser(ctx, directUserID, message)
		if result != nil {
			result["fallback"] = "current-user"
		}
		return result, err
	}
	return map[string]any{
		"status":   "needs-configuration",
		"provider": "feishu",
		"reason":   "Set a Feishu chat/webhook/task route, or run lark-cli auth login for current-user direct delivery.",
	}, nil
}

func renderFeishuFailureText(packet map[string]any, language ...string) string {
	lang := normalizeUILanguage(firstNonEmpty(append(language, "zh-CN")...))
	item := mapValue(packet["item"])
	attempt := mapValue(packet["attempt"])
	pipeline := mapValue(packet["pipeline"])
	requirement := mapValue(packet["requirement"])
	repositoryTarget := mapValue(packet["repositoryTarget"])
	reason := firstNonEmpty(text(attempt, "failureReason"), text(attempt, "statusReason"), text(attempt, "errorMessage"), "Run failed or stalled.")
	detail := firstNonEmpty(text(attempt, "failureDetail"), text(attempt, "stderrSummary"))
	stageID := firstNonEmpty(text(attempt, "failureStageId"), text(attempt, "currentStageId"), text(item, "stageId"))
	publicAppURL := strings.TrimRight(strings.TrimSpace(os.Getenv("OMEGA_PUBLIC_APP_URL")), "/")
	lines := []string{}
	if lang == "zh-CN" {
		lines = append(lines,
			"🚨 Omega 运行需要处理",
			fmt.Sprintf("类型: %s", feishuFailureAlertType(attempt, lang)),
			fmt.Sprintf("工作项: %s (%s) · %s", firstNonEmpty(text(item, "key"), text(item, "id"), "unknown"), stringOr(text(item, "id"), "unknown"), stringOr(text(item, "title"), "Untitled work item")),
		)
	} else {
		lines = append(lines,
			"🚨 Omega run needs attention",
			fmt.Sprintf("Type: %s", feishuFailureAlertType(attempt, lang)),
			fmt.Sprintf("Work item: %s (%s) · %s", firstNonEmpty(text(item, "key"), text(item, "id"), "unknown"), stringOr(text(item, "id"), "unknown"), stringOr(text(item, "title"), "Untitled work item")),
		)
	}
	if requirementLine := feishuFailureRequirementLine(requirement, item); requirementLine != "" {
		lines = append(lines, fmt.Sprintf("%s: %s", feishuLabel(lang, "Requirement", "需求"), requirementLine))
	}
	if contextLine := feishuFailureItemContextLine(item); contextLine != "" {
		lines = append(lines, fmt.Sprintf("%s: %s", feishuLabel(lang, "Context", "上下文"), contextLine))
	}
	if repositoryLine := feishuFailureRepositoryLine(repositoryTarget, item, attempt); repositoryLine != "" {
		lines = append(lines, fmt.Sprintf("%s: %s", feishuLabel(lang, "Repository", "仓库"), repositoryLine))
	}
	lines = append(lines,
		fmt.Sprintf("Pipeline: %s · %s · %s", stringOr(text(pipeline, "id"), "unknown"), stringOr(firstNonEmpty(text(pipeline, "templateId"), text(pipeline, "kind")), "unknown"), stringOr(text(pipeline, "status"), "unknown")),
		fmt.Sprintf("Attempt: %s · %s", stringOr(text(attempt, "id"), "unknown"), feishuFailureRunnerLine(attempt)),
		fmt.Sprintf("%s: %s", feishuLabel(lang, "Stage", "阶段"), feishuFailureStageLine(pipeline, attempt, stageID)),
	)
	if lastSeenAt := firstNonEmpty(text(attempt, "lastSeenAt"), text(attempt, "updatedAt")); lastSeenAt != "" {
		lines = append(lines, fmt.Sprintf("%s: %s", feishuLabel(lang, "Last heartbeat", "最后心跳"), lastSeenAt))
	}
	lines = append(lines,
		fmt.Sprintf("%s: %s", feishuLabel(lang, "Reason", "原因"), reason),
		"",
		feishuLabel(lang, "📦 Delivery context", "📦 交付上下文"),
		fmt.Sprintf("PR: %s", stringOr(text(attempt, "pullRequestUrl"), "not created")),
		fmt.Sprintf("Branch: %s", stringOr(text(attempt, "branchName"), "not recorded")),
		fmt.Sprintf("Workspace: %s", stringOr(text(attempt, "workspacePath"), "not recorded")),
		"",
		feishuLabel(lang, "🛠️ Suggested action", "🛠️ 建议处理"),
		feishuLabel(lang, "Open Omega, inspect the attempt logs, then Retry, Cancel, or recover the runner.", "打开 Omega 查看 attempt 日志，然后 Retry、Cancel，或恢复对应 runner。"),
	)
	if publicAppURL != "" && text(item, "id") != "" {
		lines = append(lines, fmt.Sprintf("%s: %s/#/work-items/%s", feishuLabel(lang, "Open", "打开"), publicAppURL, text(item, "id")))
	}
	if detail != "" {
		lines = append(lines, "", "🧾 Runner detail", truncateForProof(detail, 900))
	}
	return strings.Join(lines, "\n")
}

func feishuFailureAlertType(attempt map[string]any, language ...string) string {
	switch strings.ToLower(strings.TrimSpace(text(attempt, "status"))) {
	case "stalled":
		return "Stalled attempt"
	case "failed":
		return "Failed attempt"
	case "canceled", "cancelled":
		return "Canceled attempt"
	case "timed_out", "timeout":
		return "Timed out attempt"
	default:
		return "Run attention required"
	}
}

func feishuLabel(language string, english string, chinese string) string {
	if normalizeUILanguage(language) == "zh-CN" {
		return chinese
	}
	return english
}

func feishuFailureRequirementLine(requirement map[string]any, item map[string]any) string {
	title := firstNonEmpty(text(requirement, "title"), text(item, "title"))
	description := firstNonEmpty(text(requirement, "description"), text(requirement, "rawText"), text(item, "description"))
	if title == "" {
		return truncateForProof(oneLine(description), 220)
	}
	if description == "" || strings.EqualFold(strings.TrimSpace(title), strings.TrimSpace(description)) {
		return title
	}
	return title + " — " + truncateForProof(oneLine(description), 220)
}

func feishuFailureItemContextLine(item map[string]any) string {
	parts := []string{}
	for _, part := range []string{
		labelValue("source", text(item, "source")),
		labelValue("status", text(item, "status")),
		labelValue("priority", text(item, "priority")),
		labelValue("assignee", text(item, "assignee")),
	} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	if labels := joinAnyStrings(item["labels"]); labels != "" {
		parts = append(parts, "labels "+labels)
	}
	return strings.Join(parts, " · ")
}

func feishuFailureRepositoryLine(target map[string]any, item map[string]any, attempt map[string]any) string {
	if label := strings.TrimSpace(repositoryTargetLabel(target)); label != "" {
		if targetID := text(target, "id"); targetID != "" {
			return fmt.Sprintf("%s (%s)", label, targetID)
		}
		return label
	}
	return firstNonEmpty(text(item, "repositoryTargetLabel"), text(attempt, "repositoryTargetLabel"), text(item, "target"), text(attempt, "repositoryTargetId"), text(item, "repositoryTargetId"))
}

func feishuFailureRunnerLine(attempt map[string]any) string {
	runner := firstNonEmpty(text(attempt, "runner"), text(attempt, "agent"), text(attempt, "profileId"))
	model := text(attempt, "model")
	switch {
	case runner != "" && model != "":
		return fmt.Sprintf("runner %s · model %s", runner, model)
	case runner != "":
		return fmt.Sprintf("runner %s", runner)
	case model != "":
		return fmt.Sprintf("model %s", model)
	default:
		return "runner not recorded"
	}
}

func feishuFailureStageLine(pipeline map[string]any, attempt map[string]any, stageID string) string {
	title := feishuFailureStageTitle(pipeline, attempt, stageID)
	if stageID == "" {
		return stringOr(title, "not recorded")
	}
	if title == "" || title == stageID {
		return stageID
	}
	return fmt.Sprintf("%s (%s)", title, stageID)
}

func feishuFailureStageTitle(pipeline map[string]any, attempt map[string]any, stageID string) string {
	if stageID == "" {
		return ""
	}
	for _, stage := range arrayMaps(mapValue(pipeline["run"])["stages"]) {
		if text(stage, "id") == stageID {
			return firstNonEmpty(text(stage, "title"), text(stage, "name"))
		}
	}
	for _, stage := range arrayMaps(attempt["stages"]) {
		if text(stage, "id") == stageID {
			return firstNonEmpty(text(stage, "title"), text(stage, "name"))
		}
	}
	return ""
}

func labelValue(label string, value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return label + " " + value
}

func joinAnyStrings(value any) string {
	values := []string{}
	switch typed := value.(type) {
	case []any:
		for _, entry := range typed {
			if label := strings.TrimSpace(fmt.Sprint(entry)); label != "" {
				values = append(values, label)
			}
		}
	case []string:
		for _, entry := range typed {
			if label := strings.TrimSpace(entry); label != "" {
				values = append(values, label)
			}
		}
	}
	return strings.Join(values, ", ")
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func buildFeishuReviewCard(packet map[string]any) map[string]any {
	return buildFeishuReviewCardWithOptions(packet, feishuReviewSendOptions{})
}

func buildFeishuReviewCardWithOptions(packet map[string]any, options feishuReviewSendOptions) map[string]any {
	lang := normalizeUILanguage(options.Language)
	checkpoint := mapValue(packet["checkpoint"])
	item := mapValue(packet["item"])
	attempt := mapValue(packet["attempt"])
	reviewPacket := mapValue(packet["reviewPacket"])
	requirement := mapValue(packet["requirement"])
	title := stringOr(text(item, "title"), text(checkpoint, "title"))
	checkpointID := text(checkpoint, "id")
	publicAppURL := strings.TrimRight(strings.TrimSpace(os.Getenv("OMEGA_PUBLIC_APP_URL")), "/")
	publicAPIURL := strings.TrimRight(strings.TrimSpace(os.Getenv("OMEGA_PUBLIC_API_URL")), "/")
	token := strings.TrimSpace(firstNonEmpty(options.ReviewToken, os.Getenv("OMEGA_FEISHU_REVIEW_TOKEN")))
	reviewURL := ""
	if publicAppURL != "" && text(item, "id") != "" {
		reviewURL = publicAppURL + "/#/work-items/" + text(item, "id")
	}
	callbackURL := ""
	if publicAPIURL != "" {
		callbackURL = publicAPIURL + "/feishu/review-callback"
	}
	requirementText := text(requirement, "description")
	if requirementText == "" {
		requirementText = text(item, "description")
	}
	risk := text(mapValue(reviewPacket["risk"]), "level")
	if risk == "" {
		risk = "pending"
	}
	elements := []any{
		map[string]any{"tag": "markdown", "content": fmt.Sprintf("**%s**: `%s` %s\n**%s**: %s\n**%s**: `%s`",
			feishuLabel(lang, "Work item", "工作项"),
			stringOr(text(item, "key"), text(item, "id")),
			title,
			feishuLabel(lang, "Status", "状态"),
			feishuLabel(lang, "waiting for human review", "等待人工审核"),
			feishuLabel(lang, "Risk", "风险"),
			risk,
		)},
		map[string]any{"tag": "hr"},
		map[string]any{"tag": "markdown", "content": "**" + feishuLabel(lang, "Requirement", "需求") + "**\n" + truncateForProof(requirementText, 900)},
	}
	if prURL := text(attempt, "pullRequestUrl"); prURL != "" {
		elements = append(elements, map[string]any{"tag": "markdown", "content": "**Pull request**\n" + prURL})
	}
	if summary := text(reviewPacket, "summary"); summary != "" {
		elements = append(elements, map[string]any{"tag": "markdown", "content": "**" + feishuLabel(lang, "Review packet", "审核包") + "**\n" + truncateForProof(summary, 900)})
	}
	if riskBasis := renderFeishuRiskBasisMarkdown(reviewPacket, lang, 4); riskBasis != "" {
		elements = append(elements, map[string]any{"tag": "markdown", "content": riskBasis})
	}
	if todoMarkdown := renderFeishuTodoCompletionMarkdown(reviewPacket, lang, 6); todoMarkdown != "" {
		elements = append(elements, map[string]any{"tag": "markdown", "content": todoMarkdown})
	}
	actions := []any{}
	if reviewURL != "" {
		actions = append(actions, map[string]any{"tag": "button", "text": map[string]any{"tag": "plain_text", "content": feishuLabel(lang, "Open review", "打开审核")}, "type": "default", "url": reviewURL})
	}
	if feishuCardCallbackReady() {
		approveValue := map[string]any{"action": "approve", "checkpointId": checkpointID, "token": token}
		requestChangesValue := map[string]any{"action": "request_changes", "checkpointId": checkpointID, "token": token}
		actions = append(actions,
			map[string]any{"tag": "button", "text": map[string]any{"tag": "plain_text", "content": feishuLabel(lang, "Approve", "通过")}, "type": "primary", "value": approveValue},
			map[string]any{"tag": "button", "text": map[string]any{"tag": "plain_text", "content": feishuLabel(lang, "Request changes", "要求修改")}, "type": "danger", "value": requestChangesValue},
		)
		if callbackURL != "" {
			elements = append(elements, map[string]any{"tag": "note", "elements": []any{map[string]any{"tag": "plain_text", "content": feishuLabel(lang, "Card buttons require Feishu Card Request URL to point to ", "卡片按钮需要 Feishu Card Request URL 指向 ") + callbackURL}}})
		}
	} else {
		elements = append(elements, map[string]any{"tag": "note", "elements": []any{map[string]any{"tag": "plain_text", "content": feishuLabel(lang, "Card buttons are disabled until a public Feishu Card Request URL is configured. Omega will prefer Task review when a reviewer is available.", "配置公开 Feishu Card Request URL 前，卡片按钮不可用；如果有审核人，Omega 会优先使用任务审核。")}}})
	}
	if len(actions) == 0 {
		actions = append(actions, map[string]any{"tag": "button", "text": map[string]any{"tag": "plain_text", "content": feishuLabel(lang, "Open Omega", "打开 Omega")}, "type": "default", "url": firstNonEmpty(reviewURL, publicAppURL)})
	}
	if firstNonEmpty(reviewURL, publicAppURL) != "" {
		elements = append(elements, map[string]any{"tag": "action", "actions": actions})
	}
	return map[string]any{
		"config": map[string]any{"wide_screen_mode": true},
		"header": map[string]any{
			"title":    map[string]any{"tag": "plain_text", "content": feishuLabel(lang, "Omega Human Review", "Omega 人工审核")},
			"template": "orange",
		},
		"elements": elements,
	}
}

func buildFeishuReviewDocMarkdown(packet map[string]any, language ...string) string {
	lang := normalizeUILanguage(firstNonEmpty(language...))
	item := mapValue(packet["item"])
	attempt := mapValue(packet["attempt"])
	reviewPacket := mapValue(packet["reviewPacket"])
	requirement := mapValue(packet["requirement"])
	description := text(requirement, "description")
	if description == "" {
		description = text(item, "description")
	}
	lines := []string{
		"# " + feishuLabel(lang, "Omega Human Review", "Omega 人工审核"),
		"",
		fmt.Sprintf("- %s: `%s` %s", feishuLabel(lang, "Work item", "工作项"), stringOr(text(item, "key"), text(item, "id")), text(item, "title")),
		fmt.Sprintf("- PR: %s", stringOr(text(attempt, "pullRequestUrl"), "not created")),
		fmt.Sprintf("- Branch: `%s`", text(attempt, "branchName")),
		"",
		"## " + feishuLabel(lang, "Requirement", "需求"),
		"",
		description,
	}
	if summary := text(reviewPacket, "summary"); summary != "" {
		lines = append(lines, "", "## "+feishuLabel(lang, "Review packet", "审核包"), "", summary)
	}
	if risk := mapValue(reviewPacket["risk"]); len(risk) > 0 {
		lines = append(lines, "", "## "+feishuLabel(lang, "Risk", "风险"), "", "- "+feishuLabel(lang, "Level", "等级")+": `"+text(risk, "level")+"`")
		for _, reason := range stringSlice(risk["reasons"]) {
			lines = append(lines, "- "+reason)
		}
		if basis := renderFeishuRiskBasisMarkdown(reviewPacket, lang, 12); basis != "" {
			lines = append(lines, "", basis)
		}
	}
	if todoMarkdown := renderFeishuTodoCompletionMarkdown(reviewPacket, lang, 24); todoMarkdown != "" {
		lines = append(lines, "", todoMarkdown)
	}
	if diff := mapValue(reviewPacket["diffPreview"]); len(diff) > 0 {
		lines = append(lines, "", "## "+feishuLabel(lang, "Diff preview", "Diff 预览"), "", "```diff", truncateForProof(text(diff, "patchExcerpt"), 5000), "```")
	}
	return strings.Join(lines, "\n")
}

func renderFeishuReviewText(packet map[string]any, language ...string) string {
	lang := normalizeUILanguage(firstNonEmpty(append(language, "zh-CN")...))
	item := mapValue(packet["item"])
	attempt := mapValue(packet["attempt"])
	reviewPacket := mapValue(packet["reviewPacket"])
	requirement := mapValue(packet["requirement"])
	risk := text(mapValue(reviewPacket["risk"]), "level")
	requirementText := firstNonEmpty(text(requirement, "description"), text(requirement, "rawText"), text(item, "description"))
	lines := []string{}
	if lang == "zh-CN" {
		lines = append(lines,
			"✅ Omega 人工审核",
			fmt.Sprintf("工作项: %s (%s) · %s", stringOr(text(item, "key"), text(item, "id")), stringOr(text(item, "id"), "unknown"), text(item, "title")),
			"状态: 等待人工审核",
		)
	} else {
		lines = append(lines,
			"✅ Omega human review",
			fmt.Sprintf("Work item: %s (%s) · %s", stringOr(text(item, "key"), text(item, "id")), stringOr(text(item, "id"), "unknown"), text(item, "title")),
			"Status: waiting for human review",
		)
	}
	if risk != "" {
		lines = append(lines, feishuLabel(lang, "Risk", "风险")+": "+risk)
	}
	if riskLines := renderFeishuRiskBasisPlain(reviewPacket, lang, 4); len(riskLines) > 0 {
		lines = append(lines, riskLines...)
	}
	if prURL := text(attempt, "pullRequestUrl"); prURL != "" {
		lines = append(lines, "PR: "+prURL)
	}
	if branch := text(attempt, "branchName"); branch != "" {
		lines = append(lines, "Branch: "+branch)
	}
	if requirementText != "" {
		lines = append(lines, "", feishuLabel(lang, "📋 Requirement summary", "📋 需求摘要"), truncateForProof(oneLine(requirementText), 700))
	}
	if summary := text(reviewPacket, "summary"); summary != "" {
		lines = append(lines, "", feishuLabel(lang, "🧾 Review packet", "🧾 Review packet"), truncateForProof(summary, 700))
	}
	if todoLines := renderFeishuTodoCompletionPlain(reviewPacket, lang, 6); len(todoLines) > 0 {
		lines = append(lines, "")
		lines = append(lines, todoLines...)
	}
	lines = append(lines, "", feishuLabel(lang, "🛠️ Review actions", "🛠️ 审核动作"), feishuLabel(lang, "Approve or Request changes in Omega. In Feishu task mode, completing the task approves delivery; comments with requested changes route to rework.", "在 Omega 中 Approve 或 Request changes；如果是飞书任务模式，完成任务表示审核通过；评论修改意见会进入 rework。"))
	return strings.Join(lines, "\n")
}

func renderFeishuRiskBasisMarkdown(reviewPacket map[string]any, lang string, limit int) string {
	risk := mapValue(reviewPacket["risk"])
	basis := arrayMaps(risk["basis"])
	if len(basis) == 0 {
		return ""
	}
	lines := []string{"**" + feishuLabel(lang, "Risk basis", "风险依据") + "**"}
	for index, entry := range basis {
		if index >= limit {
			lines = append(lines, fmt.Sprintf("- ... %d more", len(basis)-index))
			break
		}
		lines = append(lines, fmt.Sprintf("- `%s` %s: %s", stringOr(text(entry, "level"), "unknown"), stringOr(text(entry, "source"), "source"), truncateForProof(oneLine(stringOr(text(entry, "evidence"), text(entry, "reason"))), 180)))
	}
	return strings.Join(lines, "\n")
}

func renderFeishuRiskBasisPlain(reviewPacket map[string]any, lang string, limit int) []string {
	risk := mapValue(reviewPacket["risk"])
	basis := arrayMaps(risk["basis"])
	if len(basis) == 0 {
		return nil
	}
	lines := []string{feishuLabel(lang, "Risk basis:", "风险依据:")}
	for index, entry := range basis {
		if index >= limit {
			lines = append(lines, fmt.Sprintf("... %d more", len(basis)-index))
			break
		}
		lines = append(lines, fmt.Sprintf("- %s/%s: %s", stringOr(text(entry, "level"), "unknown"), stringOr(text(entry, "source"), "source"), truncateForProof(oneLine(stringOr(text(entry, "evidence"), text(entry, "reason"))), 180)))
	}
	return lines
}

func renderFeishuTodoCompletionMarkdown(reviewPacket map[string]any, lang string, limit int) string {
	completion := mapValue(reviewPacket["todoCompletion"])
	if len(completion) == 0 || text(completion, "status") == "not_captured" {
		return ""
	}
	lines := []string{"**" + feishuLabel(lang, "Plan / TODO verification", "Plan / TODO 复核") + "**"}
	if summary := text(completion, "summary"); summary != "" {
		lines = append(lines, truncateForProof(summary, 500))
	}
	lines = append(lines, renderFeishuTodoCompletionMarkdownGroup(feishuLabel(lang, "Functional TODO", "功能 TODO"), arrayMaps(completion["functional"]), limit)...)
	remaining := limit - (len(lines) - 2)
	if remaining < 0 {
		remaining = 0
	}
	lines = append(lines, renderFeishuTodoCompletionMarkdownGroup(feishuLabel(lang, "Project TODO", "项目 TODO"), arrayMaps(completion["project"]), remaining)...)
	return strings.Join(lines, "\n")
}

func renderFeishuTodoCompletionMarkdownGroup(title string, items []map[string]any, limit int) []string {
	if len(items) == 0 || limit <= 0 {
		return []string{}
	}
	lines := []string{"", title}
	for index, item := range items {
		if index >= limit {
			lines = append(lines, fmt.Sprintf("- ... %d more", len(items)-index))
			break
		}
		lines = append(lines, fmt.Sprintf("- %s %s", feishuTodoStatusIcon(text(item, "status")), truncateForProof(text(item, "text"), 180)))
	}
	return lines
}

func renderFeishuTodoCompletionPlain(reviewPacket map[string]any, lang string, limit int) []string {
	completion := mapValue(reviewPacket["todoCompletion"])
	if len(completion) == 0 || text(completion, "status") == "not_captured" {
		return nil
	}
	lines := []string{feishuLabel(lang, "✅ Plan / TODO verification", "✅ Plan / TODO 复核")}
	if summary := text(completion, "summary"); summary != "" {
		lines = append(lines, truncateForProof(oneLine(summary), 500))
	}
	appendPlainGroup := func(title string, items []map[string]any) {
		if len(items) == 0 || len(lines) >= limit+2 {
			return
		}
		lines = append(lines, title)
		for index, item := range items {
			if len(lines) >= limit+2 {
				lines = append(lines, fmt.Sprintf("... %d more", len(items)-index))
				return
			}
			lines = append(lines, fmt.Sprintf("%s %s", feishuTodoStatusIcon(text(item, "status")), truncateForProof(oneLine(text(item, "text")), 180)))
		}
	}
	appendPlainGroup(feishuLabel(lang, "Functional TODO", "功能 TODO"), arrayMaps(completion["functional"]))
	appendPlainGroup(feishuLabel(lang, "Project TODO", "项目 TODO"), arrayMaps(completion["project"]))
	return lines
}

func feishuTodoStatusIcon(status string) string {
	switch status {
	case "verified":
		return "✅"
	case "attention":
		return "⚠️"
	default:
		return "○"
	}
}

func sendFeishuWebhookInteractiveCard(ctx context.Context, webhook string, card map[string]any) (map[string]any, error) {
	return sendFeishuWebhookInteractiveCardWithSecret(ctx, webhook, os.Getenv("OMEGA_FEISHU_WEBHOOK_SECRET"), card)
}

func sendFeishuWebhookInteractiveCardWithSecret(ctx context.Context, webhook string, secret string, card map[string]any) (map[string]any, error) {
	payload := map[string]any{"msg_type": "interactive", "card": card}
	if secret := strings.TrimSpace(secret); secret != "" {
		timestamp := fmt.Sprint(time.Now().Unix())
		payload["timestamp"] = timestamp
		payload["sign"] = signFeishuWebhook(timestamp, secret)
	}
	raw, _ := json.Marshal(payload)
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(timeoutCtx, http.MethodPost, webhook, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	var result map[string]any
	_ = json.NewDecoder(response.Body).Decode(&result)
	if response.StatusCode >= 300 {
		return result, fmt.Errorf("Feishu webhook failed with HTTP %d", response.StatusCode)
	}
	if result == nil {
		result = map[string]any{}
	}
	result["status"] = "sent"
	result["provider"] = "feishu"
	result["tool"] = "webhook"
	result["format"] = "interactive-card"
	return result, nil
}

func signFeishuWebhook(timestamp string, secret string) string {
	key := []byte(timestamp + "\n" + secret)
	mac := hmac.New(sha256.New, key)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func writeFeishuReviewDocArtifact(proofDir string, packet map[string]any) (string, error) {
	if proofDir == "" {
		return "", nil
	}
	if err := os.MkdirAll(proofDir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(proofDir, "feishu-human-review.md")
	return path, os.WriteFile(path, []byte(buildFeishuReviewDocMarkdown(packet)), 0o644)
}

func larkCLIAvailable() bool {
	_, err := exec.LookPath("lark-cli")
	return err == nil
}
