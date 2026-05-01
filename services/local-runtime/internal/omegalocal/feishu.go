package omegalocal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

func (server *Server) feishuNotify(response http.ResponseWriter, request *http.Request) {
	var payload struct {
		ChatID string `json:"chatId"`
		Text   string `json:"text"`
	}
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	result, err := sendFeishuText(request.Context(), payload.ChatID, payload.Text)
	if err != nil {
		writeJSON(response, http.StatusServiceUnavailable, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(response, http.StatusOK, result)
}

func sendFeishuText(ctx context.Context, chatID string, text string) (map[string]any, error) {
	chatID = strings.TrimSpace(chatID)
	text = strings.TrimSpace(text)
	if chatID == "" {
		return nil, fmt.Errorf("chatId is required")
	}
	if text == "" {
		return nil, fmt.Errorf("text is required")
	}
	path, err := exec.LookPath("lark-cli")
	if err != nil {
		return nil, fmt.Errorf("lark-cli not found")
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(timeoutCtx, path, "im", "+messages-send", "--chat-id", chatID, "--text", text).CombinedOutput()
	trimmed := strings.TrimSpace(string(output))
	if err != nil {
		if trimmed == "" {
			trimmed = err.Error()
		}
		return nil, fmt.Errorf("lark-cli failed: %s", trimmed)
	}
	messageID := extractMessageID(trimmed)
	return map[string]any{
		"status":    "sent",
		"provider":  "feishu",
		"tool":      "lark-cli",
		"format":    "text",
		"chatId":    chatID,
		"messageId": messageID,
		"raw":       trimmed,
	}, nil
}

func sendFeishuInteractiveCard(ctx context.Context, chatID string, card map[string]any) (map[string]any, error) {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return nil, fmt.Errorf("chatId is required")
	}
	if len(card) == 0 {
		return nil, fmt.Errorf("interactive card is required")
	}
	cardJSON, err := json.Marshal(card)
	if err != nil {
		return nil, err
	}
	path, err := exec.LookPath("lark-cli")
	if err != nil {
		return nil, fmt.Errorf("lark-cli not found")
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(timeoutCtx, path, "im", "+messages-send", "--chat-id", chatID, "--msg-type", "interactive", "--content", string(cardJSON)).CombinedOutput()
	trimmed := strings.TrimSpace(string(output))
	if err != nil {
		if trimmed == "" {
			trimmed = err.Error()
		}
		return nil, fmt.Errorf("lark-cli failed: %s", trimmed)
	}
	messageID := extractMessageID(trimmed)
	return map[string]any{
		"status":    "sent",
		"provider":  "feishu",
		"tool":      "lark-cli",
		"format":    "interactive-card",
		"chatId":    chatID,
		"messageId": messageID,
		"raw":       trimmed,
	}, nil
}

func extractMessageID(output string) string {
	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err == nil {
		if value := stringOr(payload["message_id"], ""); value != "" {
			return value
		}
		if data := mapValue(payload["data"]); len(data) > 0 {
			if value := stringOr(data["message_id"], ""); value != "" {
				return value
			}
		}
	}
	return ""
}
