package omegalocal

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const runnerCredentialSettingPrefix = "runnerCredential:"
const credentialKeyFilename = "credentials.key"

type runnerCredentialRecord struct {
	ID               string `json:"id"`
	Runner           string `json:"runner"`
	Provider         string `json:"provider"`
	Label            string `json:"label"`
	Model            string `json:"model"`
	BaseURL          string `json:"baseUrl"`
	SecretCiphertext string `json:"secretCiphertext,omitempty"`
	UpdatedAt        string `json:"updatedAt"`
}

type runnerCredentialPublic struct {
	ID               string `json:"id"`
	Runner           string `json:"runner"`
	Provider         string `json:"provider"`
	Label            string `json:"label"`
	Model            string `json:"model"`
	BaseURL          string `json:"baseUrl"`
	SecretConfigured bool   `json:"secretConfigured"`
	SecretMasked     string `json:"secretMasked,omitempty"`
	UpdatedAt        string `json:"updatedAt"`
}

type runnerCredentialUpdateRequest struct {
	ID       string `json:"id"`
	Runner   string `json:"runner"`
	Provider string `json:"provider"`
	Label    string `json:"label"`
	Model    string `json:"model"`
	BaseURL  string `json:"baseUrl"`
	Secret   string `json:"secret"`
	APIKey   string `json:"apiKey"`
}

func (server *Server) listRunnerCredentials(response http.ResponseWriter, request *http.Request) {
	records, err := server.runnerCredentialRecords(request.Context())
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	output := make([]runnerCredentialPublic, 0, len(records))
	for _, record := range records {
		output = append(output, publicRunnerCredential(record))
	}
	writeJSON(response, http.StatusOK, output)
}

func (server *Server) putRunnerCredential(response http.ResponseWriter, request *http.Request) {
	var input runnerCredentialUpdateRequest
	if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	record, err := server.saveRunnerCredential(request.Context(), input)
	if err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	writeJSON(response, http.StatusOK, publicRunnerCredential(record))
}

func publicRunnerCredential(record runnerCredentialRecord) runnerCredentialPublic {
	secretConfigured := strings.TrimSpace(record.SecretCiphertext) != ""
	masked := ""
	if secretConfigured {
		masked = "********"
	}
	return runnerCredentialPublic{
		ID:               record.ID,
		Runner:           record.Runner,
		Provider:         record.Provider,
		Label:            record.Label,
		Model:            record.Model,
		BaseURL:          record.BaseURL,
		SecretConfigured: secretConfigured,
		SecretMasked:     masked,
		UpdatedAt:        record.UpdatedAt,
	}
}

func (server *Server) saveRunnerCredential(ctx context.Context, input runnerCredentialUpdateRequest) (runnerCredentialRecord, error) {
	runner := normalizeRunnerCredentialRunner(input.Runner)
	provider := strings.ToLower(strings.TrimSpace(input.Provider))
	if runner == "" {
		return runnerCredentialRecord{}, errors.New("runner is required")
	}
	if provider == "" {
		return runnerCredentialRecord{}, errors.New("provider is required")
	}
	id := strings.TrimSpace(input.ID)
	if id == "" {
		id = runner + "-" + safeSegment(provider)
	}
	existing, _ := server.runnerCredentialRecord(ctx, id)
	record := runnerCredentialRecord{
		ID:               id,
		Runner:           runner,
		Provider:         provider,
		Label:            stringOr(strings.TrimSpace(input.Label), defaultRunnerCredentialLabel(runner, provider)),
		Model:            strings.TrimSpace(input.Model),
		BaseURL:          strings.TrimSpace(input.BaseURL),
		SecretCiphertext: existing.SecretCiphertext,
		UpdatedAt:        nowISO(),
	}
	if secret := firstNonEmpty(strings.TrimSpace(input.Secret), strings.TrimSpace(input.APIKey)); secret != "" && secret != "********" {
		ciphertext, err := server.encryptRunnerSecret(secret, record.ID)
		if err != nil {
			return runnerCredentialRecord{}, err
		}
		record.SecretCiphertext = ciphertext
	}
	if err := server.Repo.SetSetting(ctx, runnerCredentialSettingPrefix+record.ID, runnerCredentialRecordToMap(record)); err != nil {
		return runnerCredentialRecord{}, err
	}
	return record, nil
}

func defaultRunnerCredentialLabel(runner string, provider string) string {
	switch runner {
	case "trae-agent":
		return "Trae " + provider
	case "opencode":
		return "opencode " + provider
	default:
		return runner + " " + provider
	}
}

func normalizeRunnerCredentialRunner(runner string) string {
	switch strings.ToLower(strings.TrimSpace(runner)) {
	case "trae", "trae-agent":
		return "trae-agent"
	case "opencode":
		return "opencode"
	default:
		return strings.ToLower(strings.TrimSpace(runner))
	}
}

func runnerCredentialRecordToMap(record runnerCredentialRecord) map[string]any {
	return map[string]any{
		"id":               record.ID,
		"runner":           record.Runner,
		"provider":         record.Provider,
		"label":            record.Label,
		"model":            record.Model,
		"baseUrl":          record.BaseURL,
		"secretCiphertext": record.SecretCiphertext,
		"updatedAt":        record.UpdatedAt,
	}
}

func runnerCredentialRecordFromMap(value map[string]any) runnerCredentialRecord {
	return runnerCredentialRecord{
		ID:               text(value, "id"),
		Runner:           normalizeRunnerCredentialRunner(text(value, "runner")),
		Provider:         strings.ToLower(strings.TrimSpace(text(value, "provider"))),
		Label:            text(value, "label"),
		Model:            text(value, "model"),
		BaseURL:          text(value, "baseUrl"),
		SecretCiphertext: text(value, "secretCiphertext"),
		UpdatedAt:        text(value, "updatedAt"),
	}
}

func (server *Server) runnerCredentialRecord(ctx context.Context, id string) (runnerCredentialRecord, error) {
	record, err := server.Repo.GetSetting(ctx, runnerCredentialSettingPrefix+id)
	if err != nil {
		return runnerCredentialRecord{}, err
	}
	return runnerCredentialRecordFromMap(record), nil
}

func (server *Server) runnerCredentialRecords(ctx context.Context) ([]runnerCredentialRecord, error) {
	settings, err := server.Repo.ListSettings(ctx, runnerCredentialSettingPrefix)
	if err != nil {
		return nil, err
	}
	records := make([]runnerCredentialRecord, 0, len(settings))
	for _, setting := range settings {
		record := runnerCredentialRecordFromMap(setting)
		if record.ID != "" {
			records = append(records, record)
		}
	}
	return records, nil
}

func (server *Server) runnerCredentialKeyPath() string {
	root := strings.TrimSpace(server.WorkspaceRoot)
	if root == "" {
		if configDir, err := os.UserConfigDir(); err == nil && configDir != "" {
			root = filepath.Join(configDir, "omega")
		} else {
			root = filepath.Join(os.TempDir(), "omega")
		}
	}
	return filepath.Join(root, ".omega", credentialKeyFilename)
}

func (server *Server) runnerCredentialKey() ([]byte, error) {
	keyPath := server.runnerCredentialKeyPath()
	if raw, err := os.ReadFile(keyPath); err == nil {
		decoded, decodeErr := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
		if decodeErr == nil && len(decoded) == 32 {
			return decoded, nil
		}
	}
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, []byte(base64.StdEncoding.EncodeToString(key)), 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func (server *Server) encryptRunnerSecret(secret string, credentialID string) (string, error) {
	key, err := server.runnerCredentialKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := aead.Seal(nil, nonce, []byte(secret), []byte(credentialID))
	payload := append(nonce, ciphertext...)
	return "v1:" + base64.StdEncoding.EncodeToString(payload), nil
}

func (server *Server) decryptRunnerSecret(record runnerCredentialRecord) (string, error) {
	raw := strings.TrimSpace(record.SecretCiphertext)
	if raw == "" {
		return "", nil
	}
	if !strings.HasPrefix(raw, "v1:") {
		return "", fmt.Errorf("unsupported runner credential ciphertext")
	}
	key, err := server.runnerCredentialKey()
	if err != nil {
		return "", err
	}
	payload, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(raw, "v1:"))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(payload) < aead.NonceSize() {
		return "", fmt.Errorf("runner credential ciphertext is truncated")
	}
	nonce := payload[:aead.NonceSize()]
	ciphertext := payload[aead.NonceSize():]
	plain, err := aead.Open(nil, nonce, ciphertext, []byte(record.ID))
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func (server *Server) runnerCredentialFor(ctx context.Context, runnerID string, provider string) (runnerCredentialRecord, bool) {
	records, err := server.runnerCredentialRecords(ctx)
	if err != nil {
		return runnerCredentialRecord{}, false
	}
	normalizedRunner := normalizeRunnerCredentialRunner(runnerID)
	normalizedProvider := strings.ToLower(strings.TrimSpace(provider))
	var fallback runnerCredentialRecord
	for _, record := range records {
		if record.Runner != normalizedRunner {
			continue
		}
		if normalizedProvider != "" && record.Provider == normalizedProvider {
			return record, true
		}
		if fallback.ID == "" {
			fallback = record
		}
	}
	return fallback, fallback.ID != ""
}

func credentialEnvPrefix(provider string) string {
	normalized := strings.ToUpper(strings.TrimSpace(provider))
	normalized = strings.NewReplacer("-", "_", " ", "_").Replace(normalized)
	return normalized
}

func (server *Server) runnerCredentialModelAndEnv(ctx context.Context, runnerID string, rawModel string) (string, map[string]string) {
	normalizedRunner := normalizeRunnerCredentialRunner(runnerID)
	switch normalizedRunner {
	case "trae-agent":
		provider, model := traeProviderAndModel(rawModel)
		record, ok := server.runnerCredentialFor(ctx, normalizedRunner, provider)
		env := traeProviderEnv(provider)
		if ok {
			if provider == "" {
				provider = record.Provider
			}
			if model == "" || strings.TrimSpace(rawModel) == "" || strings.TrimSpace(rawModel) == "gpt-5.4-mini" || strings.TrimSpace(rawModel) == "trae-default" {
				model = record.Model
			}
			prefix := credentialEnvPrefix(provider)
			if secret, err := server.decryptRunnerSecret(record); err == nil && secret != "" && prefix != "" {
				env[prefix+"_API_KEY"] = secret
			}
			if record.BaseURL != "" && prefix != "" {
				env[prefix+"_BASE_URL"] = record.BaseURL
			}
		}
		effectiveModel := strings.TrimSpace(rawModel)
		if provider != "" && model != "" {
			effectiveModel = provider + ":" + model
		} else if model != "" {
			effectiveModel = model
		}
		return effectiveModel, env
	case "opencode":
		provider := ""
		if candidate, _, ok := strings.Cut(rawModel, ":"); ok {
			provider = candidate
		}
		record, ok := server.runnerCredentialFor(ctx, normalizedRunner, provider)
		env := map[string]string{}
		if ok {
			prefix := credentialEnvPrefix(record.Provider)
			if secret, err := server.decryptRunnerSecret(record); err == nil && secret != "" && prefix != "" {
				env[prefix+"_API_KEY"] = secret
			}
			if record.BaseURL != "" && prefix != "" {
				env[prefix+"_BASE_URL"] = record.BaseURL
			}
		}
		return rawModel, env
	default:
		return rawModel, nil
	}
}
