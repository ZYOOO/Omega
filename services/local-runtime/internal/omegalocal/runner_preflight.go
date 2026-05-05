package omegalocal

import (
	"context"
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

type agentRunnerPreflightRequest struct {
	AgentID  string `json:"agentId"`
	Label    string `json:"label"`
	Runner   string `json:"runner"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	BaseURL  string `json:"baseUrl"`
	Secret   string `json:"secret"`
	APIKey   string `json:"apiKey"`
}

func (server *Server) testAgentRunner(response http.ResponseWriter, request *http.Request) {
	var input agentRunnerPreflightRequest
	if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	result, status := server.agentRunnerPreflight(request.Context(), input)
	writeJSON(response, status, result)
}

func (server *Server) agentRunnerPreflight(ctx context.Context, input agentRunnerPreflightRequest) (map[string]any, int) {
	runner := normalizeRunnerCredentialRunner(input.Runner)
	if runner == "claude-code" {
		runner = "claude-code"
	}
	command, args := runnerPreflightCommand(runner)
	result := map[string]any{
		"agentId": input.AgentID,
		"label":   input.Label,
		"runner":  runner,
		"model":   input.Model,
		"command": command,
		"status":  "failed",
	}
	if command == "" {
		result["message"] = "Unsupported runner."
		return result, http.StatusBadRequest
	}
	path, err := exec.LookPath(command)
	if err != nil {
		result["message"] = command + " is not installed or not visible in PATH."
		return result, http.StatusServiceUnavailable
	}
	result["path"] = path
	model, env := server.runnerCredentialModelAndEnv(ctx, runner, input.Model)
	env = cloneStringMap(env)
	if model != "" {
		result["effectiveModel"] = model
	}
	accountProvider := strings.ToLower(strings.TrimSpace(input.Provider))
	transientSecret := firstNonEmpty(strings.TrimSpace(input.Secret), strings.TrimSpace(input.APIKey))
	if runner == "trae-agent" || runner == "opencode" {
		model, env = applyTransientRunnerPreflightCredential(runner, model, env, input, transientSecret)
		if model != "" {
			result["effectiveModel"] = model
		}
		if accountProvider != "" {
			result["credentialProvider"] = accountProvider
			result["credentialConfigured"] = transientSecret != ""
			_, credentialModel := runnerProviderAndModel(runner, model)
			if credentialModel != "" {
				result["credentialModel"] = credentialModel
			}
		}
	}
	if runner == "trae-agent" || runner == "opencode" {
		credential, ok := server.runnerCredentialFor(ctx, runner, providerFromModelOrDefault(runner, model))
		if ok {
			if result["credentialConfigured"] != true {
				result["credentialConfigured"] = strings.TrimSpace(credential.SecretCiphertext) != ""
			}
			result["credentialProvider"] = credential.Provider
			result["credentialModel"] = credential.Model
		} else {
			if result["credentialConfigured"] == nil {
				result["credentialConfigured"] = false
			}
		}
	}
	if runner == "trae-agent" && result["credentialConfigured"] == true {
		args = []string{"--version"}
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	process, err := runSupervisedCommandContextWithOptions(
		timeoutCtx,
		SupervisedCommandOptions{Env: env},
		"",
		"",
		path,
		args...,
	)
	result["stdout"] = truncateForProof(redactRunnerPreflightOutput(strings.TrimSpace(text(process, "stdout")), env), 800)
	result["stderr"] = truncateForProof(redactRunnerPreflightOutput(strings.TrimSpace(text(process, "stderr")), env), 800)
	if err != nil {
		result["message"] = err.Error()
		return result, http.StatusServiceUnavailable
	}
	if runner == "trae-agent" || runner == "opencode" {
		provider := strings.ToLower(strings.TrimSpace(text(result, "credentialProvider")))
		if provider != "" {
			secret := transientSecret
			if secret == "" {
				if credential, ok := server.runnerCredentialFor(ctx, runner, provider); ok {
					if decrypted, decryptErr := server.decryptRunnerSecret(credential); decryptErr == nil {
						secret = decrypted
					}
				}
			}
			baseURL := firstNonEmpty(strings.TrimSpace(input.BaseURL), defaultRunnerProviderBaseURL(provider))
			if credential, ok := server.runnerCredentialFor(ctx, runner, provider); ok {
				baseURL = firstNonEmpty(strings.TrimSpace(input.BaseURL), credential.BaseURL, defaultRunnerProviderBaseURL(provider))
			}
			if strings.TrimSpace(secret) == "" {
				result["message"] = "A saved API key or pasted API key is required to test this runner account."
				return result, http.StatusServiceUnavailable
			}
			models, source, discoveryErr := discoverModelsByProviderAPI(ctx, provider, baseURL, secret)
			if discoveryErr != nil {
				result["message"] = discoveryErr.Error()
				return result, http.StatusServiceUnavailable
			}
			result["modelDiscoverySource"] = source
			result["modelCount"] = len(models)
		}
	}
	result["status"] = "ready"
	result["message"] = "Runner command and local account preflight passed."
	return result, http.StatusOK
}

func applyTransientRunnerPreflightCredential(runner string, model string, env map[string]string, input agentRunnerPreflightRequest, secret string) (string, map[string]string) {
	provider := strings.ToLower(strings.TrimSpace(input.Provider))
	if provider == "" {
		return model, env
	}
	configuredModel := strings.TrimSpace(input.Model)
	if configuredModel == "" {
		_, configuredModel = runnerProviderAndModel(runner, model)
	}
	if configuredModel == "" {
		return model, env
	}
	if runner == "trae-agent" {
		_, parsedModel := traeProviderAndModel(configuredModel)
		if parsedModel != "" {
			configuredModel = parsedModel
		}
		model = provider + ":" + strings.TrimSpace(strings.TrimPrefix(configuredModel, provider+":"))
	} else {
		_, parsedModel := opencodeProviderAndModel(configuredModel)
		if parsedModel != "" {
			configuredModel = parsedModel
		}
		model = provider + "/" + strings.TrimSpace(strings.TrimPrefix(configuredModel, provider+"/"))
	}
	prefix := credentialEnvPrefix(provider)
	if prefix != "" {
		if secret != "" {
			env[prefix+"_API_KEY"] = secret
		}
		if strings.TrimSpace(input.BaseURL) != "" {
			env[prefix+"_BASE_URL"] = strings.TrimSpace(input.BaseURL)
		}
	}
	return model, env
}

func runnerProviderAndModel(runner string, model string) (string, string) {
	if runner == "trae-agent" {
		return traeProviderAndModel(model)
	}
	return opencodeProviderAndModel(model)
}

func redactRunnerPreflightOutput(output string, env map[string]string) string {
	redacted := output
	for key, value := range env {
		key = strings.ToUpper(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if strings.Contains(key, "API_KEY") || strings.Contains(key, "TOKEN") || strings.Contains(key, "SECRET") {
			redacted = strings.ReplaceAll(redacted, value, "********")
		}
	}
	return redacted
}

func runnerPreflightCommand(runner string) (string, []string) {
	switch runner {
	case "codex":
		return "codex", []string{"--version"}
	case "opencode":
		return "opencode", []string{"--version"}
	case "trae-agent", "trae":
		return "trae-cli", []string{"show-config"}
	case "claude-code", "claude":
		return "claude", []string{"--version"}
	default:
		return "", nil
	}
}

func providerFromModelOrDefault(runner string, model string) string {
	provider, _, ok := strings.Cut(strings.TrimSpace(model), ":")
	if ok && provider != "" {
		return provider
	}
	if provider, _, ok := strings.Cut(strings.TrimSpace(model), "/"); ok && provider != "" {
		return provider
	}
	if runner == "trae-agent" {
		provider, _ := traeProviderAndModel(model)
		return provider
	}
	return provider
}
