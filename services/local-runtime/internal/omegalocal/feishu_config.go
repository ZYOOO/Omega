package omegalocal

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const feishuConfigSettingKey = "feishu-config"

type feishuConfigRecord struct {
	Mode                    string `json:"mode"`
	ChatID                  string `json:"chatId"`
	AssigneeID              string `json:"assigneeId"`
	AssigneeLabel           string `json:"assigneeLabel"`
	TasklistID              string `json:"tasklistId"`
	FollowerID              string `json:"followerId"`
	Due                     string `json:"due"`
	WebhookURL              string `json:"webhookUrl"`
	WebhookSecretCiphertext string `json:"webhookSecretCiphertext,omitempty"`
	ReviewTokenCiphertext   string `json:"reviewTokenCiphertext,omitempty"`
	CreateDoc               bool   `json:"createDoc"`
	DocFolderToken          string `json:"docFolderToken"`
	TaskBridgeEnabled       bool   `json:"taskBridgeEnabled"`
	UpdatedAt               string `json:"updatedAt"`
}

type feishuConfigPublic struct {
	Mode                    string `json:"mode"`
	ChatID                  string `json:"chatId"`
	AssigneeID              string `json:"assigneeId"`
	AssigneeLabel           string `json:"assigneeLabel"`
	TasklistID              string `json:"tasklistId"`
	FollowerID              string `json:"followerId"`
	Due                     string `json:"due"`
	WebhookURL              string `json:"webhookUrl"`
	WebhookSecretConfigured bool   `json:"webhookSecretConfigured"`
	WebhookSecretMasked     string `json:"webhookSecretMasked,omitempty"`
	ReviewTokenConfigured   bool   `json:"reviewTokenConfigured"`
	ReviewTokenMasked       string `json:"reviewTokenMasked,omitempty"`
	CreateDoc               bool   `json:"createDoc"`
	DocFolderToken          string `json:"docFolderToken"`
	TaskBridgeEnabled       bool   `json:"taskBridgeEnabled"`
	LarkCLIAvailable        bool   `json:"larkCliAvailable"`
	LarkCLIVersion          string `json:"larkCliVersion,omitempty"`
	UpdatedAt               string `json:"updatedAt"`
}

type feishuConfigUpdateRequest struct {
	Mode              string `json:"mode"`
	ChatID            string `json:"chatId"`
	AssigneeID        string `json:"assigneeId"`
	AssigneeLabel     string `json:"assigneeLabel"`
	TasklistID        string `json:"tasklistId"`
	FollowerID        string `json:"followerId"`
	Due               string `json:"due"`
	WebhookURL        string `json:"webhookUrl"`
	WebhookSecret     string `json:"webhookSecret"`
	ReviewToken       string `json:"reviewToken"`
	CreateDoc         bool   `json:"createDoc"`
	DocFolderToken    string `json:"docFolderToken"`
	TaskBridgeEnabled bool   `json:"taskBridgeEnabled"`
}

type feishuUserSearchRequest struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

type feishuUserCandidate struct {
	OpenID    string `json:"openId,omitempty"`
	UserID    string `json:"userId,omitempty"`
	UnionID   string `json:"unionId,omitempty"`
	Name      string `json:"name,omitempty"`
	Email     string `json:"email,omitempty"`
	Mobile    string `json:"mobile,omitempty"`
	AvatarURL string `json:"avatarUrl,omitempty"`
	Source    string `json:"source,omitempty"`
}

func (server *Server) getFeishuConfig(response http.ResponseWriter, request *http.Request) {
	record, _ := server.feishuConfig(request.Context())
	writeJSON(response, http.StatusOK, server.publicFeishuConfig(request.Context(), record))
}

func (server *Server) putFeishuConfig(response http.ResponseWriter, request *http.Request) {
	var input feishuConfigUpdateRequest
	if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	record, err := server.saveFeishuConfig(request.Context(), input)
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, server.publicFeishuConfig(request.Context(), record))
}

func (server *Server) testFeishuConfig(response http.ResponseWriter, request *http.Request) {
	record, _ := server.feishuConfig(request.Context())
	result := map[string]any{
		"provider": "feishu",
		"status":   "failed",
	}
	if path, err := exec.LookPath("lark-cli"); err == nil {
		result["tool"] = "lark-cli"
		result["path"] = path
		ctx, cancel := context.WithTimeout(request.Context(), 8*time.Second)
		defer cancel()
		output, runErr := exec.CommandContext(ctx, path, "config", "show").CombinedOutput()
		raw := strings.TrimSpace(string(output))
		result["raw"] = truncateForProof(raw, 1200)
		if runErr == nil && strings.Contains(raw, `"appId"`) && strings.Contains(raw, `"appSecret"`) {
			result["status"] = "ready"
			result["message"] = "lark-cli profile is configured. Human Review can use the local Feishu app profile."
			writeJSON(response, http.StatusOK, result)
			return
		}
		result["message"] = "lark-cli is installed, but no app profile is configured. Run lark-cli config init, then test again."
		writeJSON(response, http.StatusServiceUnavailable, result)
		return
	}
	if record.WebhookURL != "" {
		result["status"] = "ready"
		result["tool"] = "webhook"
		result["message"] = "Webhook URL is configured. Sending a real review card will happen at Human Review time."
		writeJSON(response, http.StatusOK, result)
		return
	}
	result["message"] = "Install and configure lark-cli, or configure a webhook URL for Feishu review delivery."
	writeJSON(response, http.StatusServiceUnavailable, result)
}

func (server *Server) searchFeishuUsers(response http.ResponseWriter, request *http.Request) {
	var input feishuUserSearchRequest
	if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	query := strings.TrimSpace(input.Query)
	if query == "" {
		writeError(response, http.StatusBadRequest, errors.New("query is required"))
		return
	}
	limit := input.Limit
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	path, err := exec.LookPath("lark-cli")
	if err != nil {
		writeJSON(response, http.StatusServiceUnavailable, map[string]any{
			"status":  "missing-tool",
			"message": "Install and configure lark-cli before searching Feishu reviewers.",
			"users":   []feishuUserCandidate{},
		})
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 12*time.Second)
	defer cancel()
	if isFeishuSelfQuery(query) {
		users, raw, err := getCurrentFeishuUserWithLarkCLI(ctx, path)
		if err != nil {
			writeJSON(response, http.StatusServiceUnavailable, map[string]any{
				"status":  "failed",
				"message": "Feishu current user lookup failed. Run lark-cli auth login, then try again.",
				"error":   err.Error(),
				"raw":     truncateForProof(raw, 1200),
				"users":   []feishuUserCandidate{},
			})
			return
		}
		writeJSON(response, http.StatusOK, map[string]any{
			"status":  "ready",
			"query":   query,
			"users":   users,
			"message": "Current Feishu user resolved from local lark-cli login.",
		})
		return
	}
	users, raw, searchErr := searchFeishuUsersWithLarkCLI(ctx, path, query, limit)
	if searchErr != nil && shouldTryFeishuIDLookup(query) {
		fallbackUsers, fallbackRaw, fallbackErr := lookupFeishuUserIDWithLarkCLI(ctx, path, query)
		if fallbackErr == nil && len(fallbackUsers) > 0 {
			users = fallbackUsers
			raw = fallbackRaw
			searchErr = nil
		}
	}
	if searchErr != nil {
		writeJSON(response, http.StatusServiceUnavailable, map[string]any{
			"status":  "failed",
			"message": "Feishu reviewer search failed. Confirm lark-cli is logged in, or search by enterprise email/mobile with contact ID permissions.",
			"error":   searchErr.Error(),
			"raw":     truncateForProof(raw, 1200),
			"users":   []feishuUserCandidate{},
		})
		return
	}
	if len(users) > limit {
		users = users[:limit]
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"status":  "ready",
		"query":   query,
		"users":   users,
		"message": "Select a reviewer to save the assignee for Feishu Task review.",
	})
}

func (server *Server) saveFeishuConfig(ctx context.Context, input feishuConfigUpdateRequest) (feishuConfigRecord, error) {
	existing, _ := server.feishuConfig(ctx)
	mode := strings.ToLower(strings.TrimSpace(input.Mode))
	if mode == "" {
		mode = "chat"
	}
	record := feishuConfigRecord{
		Mode:                    mode,
		ChatID:                  strings.TrimSpace(input.ChatID),
		AssigneeID:              strings.TrimSpace(input.AssigneeID),
		AssigneeLabel:           strings.TrimSpace(input.AssigneeLabel),
		TasklistID:              strings.TrimSpace(input.TasklistID),
		FollowerID:              strings.TrimSpace(input.FollowerID),
		Due:                     strings.TrimSpace(input.Due),
		WebhookURL:              strings.TrimSpace(input.WebhookURL),
		WebhookSecretCiphertext: existing.WebhookSecretCiphertext,
		ReviewTokenCiphertext:   existing.ReviewTokenCiphertext,
		CreateDoc:               input.CreateDoc,
		DocFolderToken:          strings.TrimSpace(input.DocFolderToken),
		TaskBridgeEnabled:       input.TaskBridgeEnabled,
		UpdatedAt:               nowISO(),
	}
	if secret := strings.TrimSpace(input.WebhookSecret); secret != "" && secret != "********" {
		ciphertext, err := server.encryptRunnerSecret(secret, "feishu-webhook-secret")
		if err != nil {
			return record, err
		}
		record.WebhookSecretCiphertext = ciphertext
	}
	if token := strings.TrimSpace(input.ReviewToken); token != "" && token != "********" {
		ciphertext, err := server.encryptRunnerSecret(token, "feishu-review-token")
		if err != nil {
			return record, err
		}
		record.ReviewTokenCiphertext = ciphertext
	}
	if err := server.Repo.SetSetting(ctx, feishuConfigSettingKey, feishuConfigToMap(record)); err != nil {
		return record, err
	}
	return record, nil
}

func (server *Server) feishuConfig(ctx context.Context) (feishuConfigRecord, error) {
	record, err := server.Repo.GetSetting(ctx, feishuConfigSettingKey)
	if err != nil {
		return feishuConfigFromEnv(), err
	}
	return feishuConfigFromMap(record), nil
}

func feishuConfigFromEnv() feishuConfigRecord {
	webhookURL := strings.TrimSpace(firstNonEmpty(os.Getenv("OMEGA_FEISHU_WEBHOOK_URL"), os.Getenv("FEISHU_BOT_WEBHOOK")))
	return feishuConfigRecord{
		Mode:              stringOr(os.Getenv("OMEGA_FEISHU_REVIEW_MODE"), "chat"),
		ChatID:            strings.TrimSpace(os.Getenv("OMEGA_FEISHU_REVIEW_CHAT_ID")),
		AssigneeID:        strings.TrimSpace(os.Getenv("OMEGA_FEISHU_REVIEW_ASSIGNEE_ID")),
		TasklistID:        strings.TrimSpace(os.Getenv("OMEGA_FEISHU_REVIEW_TASKLIST_ID")),
		FollowerID:        strings.TrimSpace(os.Getenv("OMEGA_FEISHU_REVIEW_FOLLOWER_ID")),
		Due:               strings.TrimSpace(os.Getenv("OMEGA_FEISHU_REVIEW_DUE")),
		WebhookURL:        webhookURL,
		CreateDoc:         strings.ToLower(strings.TrimSpace(os.Getenv("OMEGA_FEISHU_REVIEW_CREATE_DOC"))) == "true",
		DocFolderToken:    strings.TrimSpace(os.Getenv("OMEGA_FEISHU_REVIEW_DOC_FOLDER_TOKEN")),
		TaskBridgeEnabled: strings.ToLower(strings.TrimSpace(os.Getenv("OMEGA_FEISHU_TASK_BRIDGE_ENABLED"))) == "true",
	}
}

func feishuConfigToMap(record feishuConfigRecord) map[string]any {
	return map[string]any{
		"mode":                    record.Mode,
		"chatId":                  record.ChatID,
		"assigneeId":              record.AssigneeID,
		"assigneeLabel":           record.AssigneeLabel,
		"tasklistId":              record.TasklistID,
		"followerId":              record.FollowerID,
		"due":                     record.Due,
		"webhookUrl":              record.WebhookURL,
		"webhookSecretCiphertext": record.WebhookSecretCiphertext,
		"reviewTokenCiphertext":   record.ReviewTokenCiphertext,
		"createDoc":               record.CreateDoc,
		"docFolderToken":          record.DocFolderToken,
		"taskBridgeEnabled":       record.TaskBridgeEnabled,
		"updatedAt":               record.UpdatedAt,
	}
}

func feishuConfigFromMap(value map[string]any) feishuConfigRecord {
	return feishuConfigRecord{
		Mode:                    stringOr(value["mode"], "chat"),
		ChatID:                  text(value, "chatId"),
		AssigneeID:              text(value, "assigneeId"),
		AssigneeLabel:           text(value, "assigneeLabel"),
		TasklistID:              text(value, "tasklistId"),
		FollowerID:              text(value, "followerId"),
		Due:                     text(value, "due"),
		WebhookURL:              text(value, "webhookUrl"),
		WebhookSecretCiphertext: text(value, "webhookSecretCiphertext"),
		ReviewTokenCiphertext:   text(value, "reviewTokenCiphertext"),
		CreateDoc:               boolValue(value["createDoc"]),
		DocFolderToken:          text(value, "docFolderToken"),
		TaskBridgeEnabled:       boolValue(value["taskBridgeEnabled"]),
		UpdatedAt:               text(value, "updatedAt"),
	}
}

func (server *Server) publicFeishuConfig(ctx context.Context, record feishuConfigRecord) feishuConfigPublic {
	lark := detectLocalCapability(ctx, capabilityProbe{ID: "lark-cli", Command: "lark-cli", Category: "feishu", VersionArgs: []string{"--version"}})
	return feishuConfigPublic{
		Mode:                    stringOr(record.Mode, "chat"),
		ChatID:                  record.ChatID,
		AssigneeID:              record.AssigneeID,
		AssigneeLabel:           record.AssigneeLabel,
		TasklistID:              record.TasklistID,
		FollowerID:              record.FollowerID,
		Due:                     record.Due,
		WebhookURL:              record.WebhookURL,
		WebhookSecretConfigured: strings.TrimSpace(record.WebhookSecretCiphertext) != "",
		WebhookSecretMasked:     maskedIfConfigured(record.WebhookSecretCiphertext),
		ReviewTokenConfigured:   strings.TrimSpace(record.ReviewTokenCiphertext) != "",
		ReviewTokenMasked:       maskedIfConfigured(record.ReviewTokenCiphertext),
		CreateDoc:               record.CreateDoc,
		DocFolderToken:          record.DocFolderToken,
		TaskBridgeEnabled:       record.TaskBridgeEnabled,
		LarkCLIAvailable:        lark.Available,
		LarkCLIVersion:          lark.Version,
		UpdatedAt:               record.UpdatedAt,
	}
}

func maskedIfConfigured(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return "********"
}

func (server *Server) decryptFeishuSecret(record feishuConfigRecord, id string) string {
	ciphertext := record.WebhookSecretCiphertext
	if id == "feishu-review-token" {
		ciphertext = record.ReviewTokenCiphertext
	}
	if ciphertext == "" {
		return ""
	}
	secret, err := server.decryptRunnerSecret(runnerCredentialRecord{ID: id, SecretCiphertext: ciphertext})
	if err != nil {
		return ""
	}
	return secret
}

func searchFeishuUsersWithLarkCLI(ctx context.Context, path string, query string, limit int) ([]feishuUserCandidate, string, error) {
	command := exec.CommandContext(ctx, path, "contact", "+search-user", "--as", "user", "--query", query, "--page-size", strconv.Itoa(limit), "--format", "json")
	output, err := command.CombinedOutput()
	raw := strings.TrimSpace(string(output))
	if err != nil {
		return nil, raw, err
	}
	users := uniqueFeishuUserCandidates(feishuUserCandidatesFromJSON(parseLarkCLIJSON(raw), "contact-search"))
	if len(users) == 0 {
		return []feishuUserCandidate{}, raw, nil
	}
	return users, raw, nil
}

func getCurrentFeishuUserWithLarkCLI(ctx context.Context, path string) ([]feishuUserCandidate, string, error) {
	command := exec.CommandContext(ctx, path, "contact", "+get-user", "--as", "user", "--format", "json")
	output, err := command.CombinedOutput()
	raw := strings.TrimSpace(string(output))
	if err != nil {
		return nil, raw, err
	}
	users := uniqueFeishuUserCandidates(feishuUserCandidatesFromJSON(parseLarkCLIJSON(raw), "current-user"))
	if len(users) == 0 {
		return nil, raw, errors.New("current Feishu user was not returned")
	}
	return users, raw, nil
}

func lookupFeishuUserIDWithLarkCLI(ctx context.Context, path string, query string) ([]feishuUserCandidate, string, error) {
	payloadKey := "emails"
	value := query
	if looksLikeMobile(query) {
		payloadKey = "mobiles"
		value = normalizePhoneLike(query)
	}
	body, _ := json.Marshal(map[string][]string{payloadKey: []string{value}})
	command := exec.CommandContext(ctx, path, "api", "POST", "/open-apis/contact/v3/users/batch_get_id", "--as", "bot", "--params", `{"user_id_type":"open_id"}`, "--data", string(body))
	output, err := command.CombinedOutput()
	raw := strings.TrimSpace(string(output))
	if err != nil {
		return nil, raw, err
	}
	users := uniqueFeishuUserCandidates(feishuUserCandidatesFromJSON(parseLarkCLIJSON(raw), "batch-get-id"))
	if len(users) == 0 {
		return nil, raw, errors.New("no Feishu user matched the email or mobile")
	}
	return users, raw, nil
}

func parseLarkCLIJSON(raw string) any {
	trimmed := strings.TrimSpace(raw)
	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start >= 0 && end > start {
		trimmed = trimmed[start : end+1]
	}
	var value any
	_ = json.Unmarshal([]byte(trimmed), &value)
	return value
}

func feishuUserCandidatesFromJSON(value any, source string) []feishuUserCandidate {
	switch typed := value.(type) {
	case []any:
		var users []feishuUserCandidate
		for _, entry := range typed {
			users = append(users, feishuUserCandidatesFromJSON(entry, source)...)
		}
		return users
	case map[string]any:
		var users []feishuUserCandidate
		if candidate := feishuUserCandidateFromMap(typed, source); candidate.OpenID != "" || candidate.UserID != "" || candidate.Name != "" || candidate.Email != "" {
			users = append(users, candidate)
		}
		for _, key := range []string{"data", "user", "users", "user_list", "items", "email_users", "mobile_users", "result"} {
			if child, ok := typed[key]; ok {
				users = append(users, feishuUserCandidatesFromJSON(child, source)...)
			}
		}
		return users
	default:
		return nil
	}
}

func feishuUserCandidateFromMap(value map[string]any, source string) feishuUserCandidate {
	localizedName := mapValue(value["localized_name"])
	avatar := mapValue(value["avatar"])
	candidate := feishuUserCandidate{
		OpenID:    firstNonEmpty(text(value, "open_id"), text(value, "openId")),
		UserID:    firstNonEmpty(text(value, "user_id"), text(value, "userId")),
		UnionID:   firstNonEmpty(text(value, "union_id"), text(value, "unionId")),
		Name:      firstNonEmpty(text(value, "name"), text(value, "display_name"), text(value, "en_name"), text(localizedName, "zh_cn"), text(localizedName, "en_us")),
		Email:     text(value, "email"),
		Mobile:    firstNonEmpty(text(value, "mobile"), text(value, "mobile_phone")),
		AvatarURL: firstNonEmpty(text(avatar, "avatar_72"), text(avatar, "avatar_url"), text(value, "avatar_url")),
		Source:    source,
	}
	if candidate.OpenID == "" && source == "batch-get-id" {
		candidate.OpenID = candidate.UserID
	}
	return candidate
}

func uniqueFeishuUserCandidates(users []feishuUserCandidate) []feishuUserCandidate {
	seen := map[string]bool{}
	var output []feishuUserCandidate
	for _, user := range users {
		key := firstNonEmpty(user.OpenID, user.UserID, user.UnionID, user.Email, user.Name)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		output = append(output, user)
	}
	return output
}

func shouldTryFeishuIDLookup(query string) bool {
	return strings.Contains(query, "@") || looksLikeMobile(query)
}

func isFeishuSelfQuery(query string) bool {
	switch strings.ToLower(strings.TrimSpace(query)) {
	case "me", "self", "myself", "current", "current user", "我", "自己", "本人", "当前用户":
		return true
	default:
		return false
	}
}

func looksLikeMobile(query string) bool {
	digits := normalizePhoneLike(query)
	return len(digits) >= 7 && len(digits) <= 15
}

func normalizePhoneLike(query string) string {
	var builder strings.Builder
	for _, char := range query {
		if char >= '0' && char <= '9' {
			builder.WriteRune(char)
		}
	}
	return builder.String()
}
