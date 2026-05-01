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
	if !feishuReviewTokenAllowed(request, payload) {
		writeJSON(response, http.StatusUnauthorized, map[string]any{"error": "invalid Feishu review token"})
		return
	}
	checkpointID := strings.TrimSpace(stringOr(payload["checkpointId"], text(mapValue(payload["value"]), "checkpointId")))
	action := strings.ToLower(strings.TrimSpace(stringOr(payload["action"], text(mapValue(payload["value"]), "action"))))
	reviewer := stringOr(payload["reviewer"], stringOr(payload["operator"], "feishu-reviewer"))
	reason := stringOr(payload["reason"], stringOr(payload["comment"], "changes requested from Feishu"))
	if checkpointID == "" {
		writeJSON(response, http.StatusBadRequest, map[string]any{"error": "checkpointId is required"})
		return
	}
	decisionPayload := map[string]any{"reviewer": reviewer, "reason": reason, "asyncDelivery": boolValueDefault(payload["asyncDelivery"], true)}
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
	}
	for _, candidate := range candidates {
		if hmac.Equal([]byte(strings.TrimSpace(candidate)), []byte(expected)) {
			return true
		}
	}
	return false
}

func (server *Server) sendFeishuReviewForPipelineIfConfigured(ctx context.Context, pipelineID string) {
	chatID := strings.TrimSpace(os.Getenv("OMEGA_FEISHU_REVIEW_CHAT_ID"))
	webhook := strings.TrimSpace(os.Getenv("OMEGA_FEISHU_WEBHOOK_URL"))
	if webhook == "" {
		webhook = strings.TrimSpace(os.Getenv("FEISHU_BOT_WEBHOOK"))
	}
	taskOptions := feishuReviewSendOptions{ChatID: chatID}
	if chatID == "" && webhook == "" && !feishuReviewTaskModeEnabled(taskOptions) {
		server.logDebug(ctx, "feishu.review.skipped", "Feishu review notification skipped because no chat or webhook is configured.", map[string]any{"pipelineId": pipelineID})
		return
	}
	database, err := mustLoad(server, ctx)
	if err != nil {
		server.logError(ctx, "feishu.review.load_failed", err.Error(), map[string]any{"pipelineId": pipelineID})
		return
	}
	for _, checkpoint := range database.Tables.Checkpoints {
		if text(checkpoint, "pipelineId") == pipelineID && text(checkpoint, "status") == "pending" && text(checkpoint, "stageId") == "human_review" {
			_, _, _ = server.sendFeishuReviewForCheckpointWithOptions(ctx, text(checkpoint, "id"), taskOptions, false)
			return
		}
	}
}

func (server *Server) sendFeishuReviewForCheckpoint(ctx context.Context, checkpointID string, chatID string, manual bool) (map[string]any, int, error) {
	return server.sendFeishuReviewForCheckpointWithOptions(ctx, checkpointID, feishuReviewSendOptions{ChatID: chatID}, manual)
}

type feishuReviewSendOptions struct {
	ChatID     string
	Mode       string
	AssigneeID string
	TasklistID string
	FollowerID string
	Due        string
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
	result, err := sendFeishuReviewPacket(ctx, packet, options)
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
	card := buildFeishuReviewCard(packet)
	docMarkdown := buildFeishuReviewDocMarkdown(packet)
	webhook := strings.TrimSpace(os.Getenv("OMEGA_FEISHU_WEBHOOK_URL"))
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
	if webhook != "" {
		result, err := sendFeishuWebhookInteractiveCard(ctx, webhook, card)
		result["docMode"] = "card-summary"
		result["docPreview"] = truncateForProof(docMarkdown, 1200)
		return result, err
	}
	chatID := strings.TrimSpace(options.ChatID)
	if chatID != "" {
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
	return map[string]any{
		"status":       "needs-configuration",
		"provider":     "feishu",
		"reason":       "Set OMEGA_FEISHU_WEBHOOK_URL, OMEGA_FEISHU_REVIEW_CHAT_ID, or task review options with lark-cli installed.",
		"cardPreview":  card,
		"docPreview":   truncateForProof(docMarkdown, 2000),
		"checkpointId": text(mapValue(packet["checkpoint"]), "id"),
	}, nil
}

func buildFeishuReviewCard(packet map[string]any) map[string]any {
	checkpoint := mapValue(packet["checkpoint"])
	item := mapValue(packet["item"])
	attempt := mapValue(packet["attempt"])
	reviewPacket := mapValue(packet["reviewPacket"])
	requirement := mapValue(packet["requirement"])
	title := stringOr(text(item, "title"), text(checkpoint, "title"))
	checkpointID := text(checkpoint, "id")
	publicAppURL := strings.TrimRight(strings.TrimSpace(os.Getenv("OMEGA_PUBLIC_APP_URL")), "/")
	publicAPIURL := strings.TrimRight(strings.TrimSpace(os.Getenv("OMEGA_PUBLIC_API_URL")), "/")
	token := strings.TrimSpace(os.Getenv("OMEGA_FEISHU_REVIEW_TOKEN"))
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
	approveValue := map[string]any{"action": "approve", "checkpointId": checkpointID, "token": token}
	requestChangesValue := map[string]any{"action": "request_changes", "checkpointId": checkpointID, "token": token}
	actions = append(actions,
		map[string]any{"tag": "button", "text": map[string]any{"tag": "plain_text", "content": "Approve"}, "type": "primary", "value": approveValue},
		map[string]any{"tag": "button", "text": map[string]any{"tag": "plain_text", "content": "Request changes"}, "type": "danger", "value": requestChangesValue},
	)
	if callbackURL != "" {
		elements = append(elements, map[string]any{"tag": "note", "elements": []any{map[string]any{"tag": "plain_text", "content": "Interactive buttons require the Feishu callback URL to point to " + callbackURL}}})
	}
	elements = append(elements, map[string]any{"tag": "action", "actions": actions})
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
	payload := map[string]any{"msg_type": "interactive", "card": card}
	if secret := strings.TrimSpace(os.Getenv("OMEGA_FEISHU_WEBHOOK_SECRET")); secret != "" {
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
