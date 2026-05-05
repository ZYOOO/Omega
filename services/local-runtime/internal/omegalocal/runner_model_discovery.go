package omegalocal

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type runnerModelDiscoveryRequest struct {
	Runner   string `json:"runner"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	BaseURL  string `json:"baseUrl"`
	Secret   string `json:"secret"`
	APIKey   string `json:"apiKey"`
}

type runnerModelDiscoveryResult struct {
	Runner               string   `json:"runner"`
	Provider             string   `json:"provider"`
	Status               string   `json:"status"`
	Message              string   `json:"message,omitempty"`
	Source               string   `json:"source,omitempty"`
	Models               []string `json:"models"`
	CredentialConfigured bool     `json:"credentialConfigured"`
	BaseURL              string   `json:"baseUrl,omitempty"`
}

func (server *Server) discoverRunnerModels(response http.ResponseWriter, request *http.Request) {
	var input runnerModelDiscoveryRequest
	if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	result, status := server.runnerModelDiscovery(request.Context(), input)
	writeJSON(response, status, result)
}

func (server *Server) runnerModelDiscovery(ctx context.Context, input runnerModelDiscoveryRequest) (runnerModelDiscoveryResult, int) {
	runner := normalizeRunnerCredentialRunner(input.Runner)
	provider := strings.ToLower(strings.TrimSpace(input.Provider))
	result := runnerModelDiscoveryResult{
		Runner:   runner,
		Provider: provider,
		Status:   "failed",
		Models:   []string{},
	}
	if runner == "codex" && provider == "" {
		provider = "openai"
		result.Provider = provider
	}
	if runner != "codex" && runner != "opencode" && runner != "trae-agent" {
		result.Message = "Model discovery is only available for Codex, opencode and Trae Agent."
		return result, http.StatusBadRequest
	}
	credential, secret, configured := server.runnerModelDiscoveryCredential(ctx, runner, provider, input)
	if provider == "" {
		provider = credential.Provider
		result.Provider = provider
	}
	result.CredentialConfigured = configured
	result.BaseURL = firstNonEmpty(strings.TrimSpace(input.BaseURL), credential.BaseURL, defaultRunnerProviderBaseURL(provider))

	if runner == "codex" {
		models, source, err := discoverCodexModelsFromLocalConfig(firstNonEmpty(input.Model, credential.Model))
		if err != nil {
			result.Message = err.Error()
			return result, http.StatusServiceUnavailable
		}
		result.Status = "ready"
		result.Source = source
		result.Models = models
		result.Message = "Discovered Codex models from the local Codex configuration."
		result.BaseURL = ""
		return result, http.StatusOK
	}

	if runner == "opencode" {
		if models, message := discoverOpenCodeModelsByCommand(ctx, provider); len(models) > 0 {
			result.Status = "ready"
			result.Source = "opencode models"
			result.Message = message
			result.Models = models
			return result, http.StatusOK
		}
	}

	models, source, err := discoverModelsByProviderAPI(ctx, provider, result.BaseURL, secret)
	if err != nil {
		result.Message = err.Error()
		if runner == "opencode" {
			result.Message = "opencode model command returned no provider models; " + result.Message
		}
		return result, http.StatusServiceUnavailable
	}
	if runner == "opencode" {
		models = qualifyOpenCodeModels(provider, models)
	}
	result.Status = "ready"
	result.Source = source
	result.Models = models
	result.Message = "Discovered available models from the configured provider."
	return result, http.StatusOK
}

func (server *Server) runnerModelDiscoveryCredential(ctx context.Context, runner string, provider string, input runnerModelDiscoveryRequest) (runnerCredentialRecord, string, bool) {
	record := runnerCredentialRecord{
		Runner:   runner,
		Provider: provider,
		Model:    strings.TrimSpace(input.Model),
		BaseURL:  strings.TrimSpace(input.BaseURL),
	}
	secret := firstNonEmpty(strings.TrimSpace(input.Secret), strings.TrimSpace(input.APIKey))
	configured := secret != ""
	if stored, ok := server.runnerCredentialFor(ctx, runner, provider); ok {
		if record.Provider == "" {
			record.Provider = stored.Provider
		}
		if record.Model == "" {
			record.Model = stored.Model
		}
		if record.BaseURL == "" {
			record.BaseURL = stored.BaseURL
		}
		if secret == "" {
			if decrypted, err := server.decryptRunnerSecret(stored); err == nil {
				secret = decrypted
			}
		}
		configured = configured || strings.TrimSpace(stored.SecretCiphertext) != ""
	}
	return record, secret, configured
}

func discoverCodexModelsFromLocalConfig(preferredModel string) ([]string, string, error) {
	home := codexHomeDir()
	if home == "" {
		return nil, "", errors.New("Codex home directory could not be resolved")
	}
	models := []string{}
	seen := map[string]bool{}
	add := func(model string) {
		model = strings.TrimSpace(model)
		if model != "" && !seen[model] {
			seen[model] = true
			models = append(models, model)
		}
	}
	add(preferredModel)
	configPath := filepath.Join(home, "config.toml")
	if raw, err := os.ReadFile(configPath); err == nil {
		add(parseCodexConfigModel(string(raw)))
	}
	cachePath := filepath.Join(home, "models_cache.json")
	source := "Codex local config"
	if raw, err := os.ReadFile(cachePath); err == nil {
		source = "Codex local model cache"
		for _, model := range parseCodexModelCache(raw) {
			add(model)
		}
	}
	if len(models) == 0 {
		return nil, "", errors.New("no local Codex model cache was found; run Codex once or enter a model manually")
	}
	return models, source, nil
}

func codexHomeDir() string {
	if configured := strings.TrimSpace(os.Getenv("CODEX_HOME")); configured != "" {
		return configured
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ""
	}
	return filepath.Join(home, ".codex")
}

var codexConfigModelPattern = regexp.MustCompile(`(?m)^\s*model\s*=\s*"([^"]+)"`)

func parseCodexConfigModel(raw string) string {
	match := codexConfigModelPattern.FindStringSubmatch(raw)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func parseCodexModelCache(raw []byte) []string {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	models := []string{}
	for _, item := range arrayMaps(payload["models"]) {
		model := firstNonEmpty(text(item, "slug"), text(item, "model"), text(item, "id"), text(item, "name"))
		if strings.TrimSpace(model) != "" {
			models = append(models, strings.TrimSpace(model))
		}
	}
	return models
}

func discoverOpenCodeModelsByCommand(ctx context.Context, provider string) ([]string, string) {
	path, err := exec.LookPath("opencode")
	if err != nil {
		return nil, ""
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	args := []string{"models"}
	if strings.TrimSpace(provider) != "" {
		args = append(args, strings.TrimSpace(provider))
	}
	output, err := exec.CommandContext(timeoutCtx, path, args...).CombinedOutput()
	models := parseRunnerModelLines(string(output))
	if err != nil || len(models) == 0 {
		return nil, strings.TrimSpace(string(output))
	}
	return qualifyOpenCodeModels(provider, models), "Discovered models with opencode models."
}

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

func parseRunnerModelLines(raw string) []string {
	seen := map[string]bool{}
	models := []string{}
	for _, line := range strings.Split(ansiEscapePattern.ReplaceAllString(raw, ""), "\n") {
		model := strings.TrimSpace(line)
		if model == "" || strings.HasPrefix(strings.ToLower(model), "error:") || strings.Contains(model, "Provider not found") {
			continue
		}
		if strings.ContainsAny(model, " \t│┌└") {
			continue
		}
		if !seen[model] {
			seen[model] = true
			models = append(models, model)
		}
	}
	sort.Strings(models)
	return models
}

func qualifyOpenCodeModels(provider string, models []string) []string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		return models
	}
	output := make([]string, 0, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if strings.Contains(model, "/") {
			output = append(output, model)
		} else {
			output = append(output, provider+"/"+model)
		}
	}
	return output
}

func discoverModelsByProviderAPI(ctx context.Context, provider string, baseURL string, apiKey string) ([]string, string, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	baseURL = strings.TrimRight(firstNonEmpty(strings.TrimSpace(baseURL), defaultRunnerProviderBaseURL(provider)), "/")
	if provider == "" {
		return nil, "", errors.New("provider is required for API model discovery")
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, "", errors.New("a saved API key is required to discover provider models")
	}
	if baseURL == "" {
		return nil, "", errors.New("base URL is required for this provider")
	}
	requestURL := baseURL + "/models"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, "", err
	}
	request.Header.Set("accept", "application/json")
	switch provider {
	case "anthropic":
		request.Header.Set("x-api-key", apiKey)
		request.Header.Set("anthropic-version", "2023-06-01")
	case "google":
		q := request.URL.Query()
		q.Set("key", apiKey)
		request.URL.RawQuery = q.Encode()
	default:
		request.Header.Set("authorization", "Bearer "+apiKey)
	}
	client := http.Client{Timeout: 12 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return nil, "", err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, "", errors.New("provider model discovery failed with HTTP " + response.Status)
	}
	models := parseProviderModelResponse(provider, body)
	if len(models) == 0 {
		return nil, "", errors.New("provider returned no model ids")
	}
	return models, request.URL.String(), nil
}

func parseProviderModelResponse(provider string, body []byte) []string {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}
	seen := map[string]bool{}
	models := []string{}
	add := func(value string) {
		value = strings.TrimSpace(strings.TrimPrefix(value, "models/"))
		if value != "" && !seen[value] {
			seen[value] = true
			models = append(models, value)
		}
	}
	for _, item := range arrayMaps(payload["data"]) {
		add(firstNonEmpty(text(item, "id"), text(item, "name")))
	}
	if provider == "google" {
		for _, item := range arrayMaps(payload["models"]) {
			add(firstNonEmpty(text(item, "name"), text(item, "id")))
		}
	}
	sort.Strings(models)
	return models
}

func defaultRunnerProviderBaseURL(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "openai":
		return "https://api.openai.com/v1"
	case "openrouter":
		return "https://openrouter.ai/api/v1"
	case "deepseek":
		return "https://api.deepseek.com"
	case "qwen":
		return "https://dashscope.aliyuncs.com/compatible-mode/v1"
	case "kimi", "moonshot", "moonshotai":
		return "https://api.moonshot.ai/v1"
	case "doubao":
		return "https://ark.cn-beijing.volces.com/api/v3"
	case "anthropic":
		return "https://api.anthropic.com/v1"
	case "google":
		return "https://generativelanguage.googleapis.com/v1beta"
	default:
		return ""
	}
}
