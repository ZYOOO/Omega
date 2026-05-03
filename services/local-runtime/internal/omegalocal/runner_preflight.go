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
	AgentID string `json:"agentId"`
	Label   string `json:"label"`
	Runner  string `json:"runner"`
	Model   string `json:"model"`
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
	if model != "" {
		result["effectiveModel"] = model
	}
	if runner == "trae-agent" || runner == "opencode" {
		credential, ok := server.runnerCredentialFor(ctx, runner, providerFromModelOrDefault(runner, model))
		if ok {
			result["credentialConfigured"] = strings.TrimSpace(credential.SecretCiphertext) != ""
			result["credentialProvider"] = credential.Provider
			result["credentialModel"] = credential.Model
		} else {
			result["credentialConfigured"] = false
		}
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
	result["status"] = "ready"
	result["message"] = "Runner command and local account preflight passed."
	return result, http.StatusOK
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
	if runner == "trae-agent" {
		provider, _ := traeProviderAndModel(model)
		return provider
	}
	return provider
}
