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
	taskOptions := server.mergeFeishuReviewOptions(ctx, feishuReviewSendOptions{})
	database, err := mustLoad(server, ctx)
	if err != nil {
		server.logError(ctx, "feishu.review.load_failed", err.Error(), map[string]any{"pipelineId": pipelineID})
		return
	}
	for _, checkpoint := range database.Tables.Checkpoints {
		if text(checkpoint, "pipelineId") == pipelineID && text(checkpoint, "status") == "pending" && text(checkpoint, "stageId") == "human_review" {
			if taskOptions.ChatID == "" && taskOptions.WebhookURL == "" && taskOptions.DirectUserID == "" && !feishuReviewTaskModeEnabled(taskOptions) {
				server.logInfo(ctx, "feishu.review.needs_target", "Feishu review notification needs a chat, task assignee, tasklist, webhook target, or current-user lark-cli auth.", map[string]any{"pipelineId": pipelineID, "checkpointId": text(checkpoint, "id")})
			}
			_, _, _ = server.sendFeishuReviewForCheckpointWithOptions(ctx, text(checkpoint, "id"), taskOptions, false)
			return
		}
	}
}

func (server *Server) sendFeishuAttemptFailureIfConfigured(ctx context.Context, pipelineID string, attemptID string) {
	database, err := mustLoad(server, ctx)
	if err != nil {
		server.logError(ctx, "feishu.failure.load_failed", err.Error(), map[string]any{"pipelineId": pipelineID, "attemptId": attemptID})
		return
	}
	attemptIndex := findByID(database.Tables.Attempts, attemptID)
	if attemptIndex < 0 {
		return
	}
	attempt := database.Tables.Attempts[attemptIndex]
	if text(attempt, "feishuFailureNotifiedAt") != "" {
		return
	}
	pipeline := pipelineByID(database, pipelineID)
	item := findWorkItem(database, text(attempt, "itemId"))
	options := server.mergeFeishuReviewOptions(ctx, feishuReviewSendOptions{})
	result, err := sendFeishuFailurePacket(ctx, map[string]any{"pipeline": pipeline, "attempt": attempt, "item": item}, options)
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
		_ = server.Repo.Save(ctx, database)
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
}

func (server *Server) sendFeishuReviewForCheckpointWithOptions(ctx context.Context, checkpointID string, options feishuReviewSendOptions, manual bool) (map[string]any, int, error) {
	checkpointID = strings.TrimSpace(checkpointID)
	if checkpointID == "" {
		return nil, http.StatusBadRequest, fmt.Errorf("checkpointId is required")
	}
	database, err := mustLoad(server, ctx)
	if err != nil {
		return nil, http.StatusNotFound, err
	}
	checkpointIndex := findByID(database.Tables.Checkpoints, checkpointID)
	if checkpointIndex < 0 {
		return nil, http.StatusNotFound, fmt.Errorf("checkpoint not found")
	}
	checkpoint := cloneMap(database.Tables.Checkpoints[checkpointIndex])
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
	checkpoint["feishuReview"] = result
	checkpoint["updatedAt"] = nowISO()
	database.Tables.Checkpoints[checkpointIndex] = checkpoint
	touch(&database)
	if saveErr := server.Repo.Save(ctx, database); saveErr != nil {
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
	docMarkdown := buildFeishuReviewDocMarkdown(packet)
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
			result, err := sendFeishuText(ctx, chatID, renderFeishuReviewText(packet))
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
		result, err := sendFeishuText(ctx, chatID, renderFeishuReviewText(packet))
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
		result, err := sendFeishuTextToUser(ctx, directUserID, renderFeishuReviewText(packet))
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
	message := renderFeishuFailureText(packet)
	if webhook := strings.TrimSpace(firstNonEmpty(options.WebhookURL, os.Getenv("OMEGA_FEISHU_WEBHOOK_URL"), os.Getenv("FEISHU_BOT_WEBHOOK"))); webhook != "" {
		card := map[string]any{
			"config": map[string]any{"wide_screen_mode": true},
			"header": map[string]any{
				"template": "red",
				"title":    map[string]any{"tag": "plain_text", "content": "Omega run needs attention"},
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

func renderFeishuFailureText(packet map[string]any) string {
	item := mapValue(packet["item"])
	attempt := mapValue(packet["attempt"])
	pipeline := mapValue(packet["pipeline"])
	reason := firstNonEmpty(text(attempt, "failureReason"), text(attempt, "statusReason"), text(attempt, "errorMessage"), "Run failed or stalled.")
	detail := firstNonEmpty(text(attempt, "failureDetail"), text(attempt, "stderrSummary"))
	lines := []string{
		"**Omega 运行需要处理**",
		fmt.Sprintf("- Work Item: %s %s", firstNonEmpty(text(item, "key"), text(item, "id")), text(item, "title")),
		fmt.Sprintf("- Pipeline: %s", text(pipeline, "id")),
		fmt.Sprintf("- Attempt: %s", text(attempt, "id")),
		fmt.Sprintf("- Stage: %s", firstNonEmpty(text(attempt, "failureStageId"), text(attempt, "currentStageId"))),
		fmt.Sprintf("- Reason: %s", reason),
	}
	if detail != "" {
		lines = append(lines, "", truncateForProof(detail, 900))
	}
	return strings.Join(lines, "\n")
}

func buildFeishuReviewCard(packet map[string]any) map[string]any {
	return buildFeishuReviewCardWithOptions(packet, feishuReviewSendOptions{})
}

func buildFeishuReviewCardWithOptions(packet map[string]any, options feishuReviewSendOptions) map[string]any {
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
		map[string]any{"tag": "markdown", "content": fmt.Sprintf("**Work item**: `%s` %s\n**Status**: waiting for human review\n**Risk**: `%s`", stringOr(text(item, "key"), text(item, "id")), title, risk)},
		map[string]any{"tag": "hr"},
		map[string]any{"tag": "markdown", "content": "**Requirement**\n" + truncateForProof(requirementText, 900)},
	}
	if prURL := text(attempt, "pullRequestUrl"); prURL != "" {
		elements = append(elements, map[string]any{"tag": "markdown", "content": "**Pull request**\n" + prURL})
	}
	if summary := text(reviewPacket, "summary"); summary != "" {
		elements = append(elements, map[string]any{"tag": "markdown", "content": "**Review packet**\n" + truncateForProof(summary, 900)})
	}
	actions := []any{}
	if reviewURL != "" {
		actions = append(actions, map[string]any{"tag": "button", "text": map[string]any{"tag": "plain_text", "content": "Open review"}, "type": "default", "url": reviewURL})
	}
	if feishuCardCallbackReady() {
		approveValue := map[string]any{"action": "approve", "checkpointId": checkpointID, "token": token}
		requestChangesValue := map[string]any{"action": "request_changes", "checkpointId": checkpointID, "token": token}
		actions = append(actions,
			map[string]any{"tag": "button", "text": map[string]any{"tag": "plain_text", "content": "Approve"}, "type": "primary", "value": approveValue},
			map[string]any{"tag": "button", "text": map[string]any{"tag": "plain_text", "content": "Request changes"}, "type": "danger", "value": requestChangesValue},
		)
		if callbackURL != "" {
			elements = append(elements, map[string]any{"tag": "note", "elements": []any{map[string]any{"tag": "plain_text", "content": "Card buttons require Feishu Card Request URL to point to " + callbackURL}}})
		}
	} else {
		elements = append(elements, map[string]any{"tag": "note", "elements": []any{map[string]any{"tag": "plain_text", "content": "Card buttons are disabled until a public Feishu Card Request URL is configured. Omega will prefer Task review when a reviewer is available."}}})
	}
	if len(actions) == 0 {
		actions = append(actions, map[string]any{"tag": "button", "text": map[string]any{"tag": "plain_text", "content": "Open Omega"}, "type": "default", "url": firstNonEmpty(reviewURL, publicAppURL)})
	}
	if firstNonEmpty(reviewURL, publicAppURL) != "" {
		elements = append(elements, map[string]any{"tag": "action", "actions": actions})
	}
	return map[string]any{
		"config": map[string]any{"wide_screen_mode": true},
		"header": map[string]any{
			"title":    map[string]any{"tag": "plain_text", "content": "Omega Human Review"},
			"template": "orange",
		},
		"elements": elements,
	}
}

func buildFeishuReviewDocMarkdown(packet map[string]any) string {
	item := mapValue(packet["item"])
	attempt := mapValue(packet["attempt"])
	reviewPacket := mapValue(packet["reviewPacket"])
	requirement := mapValue(packet["requirement"])
	description := text(requirement, "description")
	if description == "" {
		description = text(item, "description")
	}
	lines := []string{
		"# Omega Human Review",
		"",
		fmt.Sprintf("- Work item: `%s` %s", stringOr(text(item, "key"), text(item, "id")), text(item, "title")),
		fmt.Sprintf("- PR: %s", stringOr(text(attempt, "pullRequestUrl"), "not created")),
		fmt.Sprintf("- Branch: `%s`", text(attempt, "branchName")),
		"",
		"## Requirement",
		"",
		description,
	}
	if summary := text(reviewPacket, "summary"); summary != "" {
		lines = append(lines, "", "## Review packet", "", summary)
	}
	if risk := mapValue(reviewPacket["risk"]); len(risk) > 0 {
		lines = append(lines, "", "## Risk", "", "- Level: `"+text(risk, "level")+"`")
		for _, reason := range stringSlice(risk["reasons"]) {
			lines = append(lines, "- "+reason)
		}
	}
	if diff := mapValue(reviewPacket["diffPreview"]); len(diff) > 0 {
		lines = append(lines, "", "## Diff preview", "", "```diff", truncateForProof(text(diff, "patchExcerpt"), 5000), "```")
	}
	return strings.Join(lines, "\n")
}

func renderFeishuReviewText(packet map[string]any) string {
	item := mapValue(packet["item"])
	attempt := mapValue(packet["attempt"])
	reviewPacket := mapValue(packet["reviewPacket"])
	lines := []string{
		"Omega Human Review",
		fmt.Sprintf("Work item: %s %s", stringOr(text(item, "key"), text(item, "id")), text(item, "title")),
		"Status: waiting for human review",
	}
	if prURL := text(attempt, "pullRequestUrl"); prURL != "" {
		lines = append(lines, "PR: "+prURL)
	}
	if summary := text(reviewPacket, "summary"); summary != "" {
		lines = append(lines, "Review packet: "+truncateForProof(summary, 500))
	}
	return strings.Join(lines, "\n")
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
