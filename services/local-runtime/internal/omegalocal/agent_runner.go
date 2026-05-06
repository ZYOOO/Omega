package omegalocal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type AgentTurnRequest struct {
	Role              string
	StageID           string
	Runner            string
	Workspace         string
	Prompt            string
	OutputPath        string
	Sandbox           string
	Model             string
	Effort            string
	Env               map[string]string
	HeartbeatInterval time.Duration
	OnProcessEvent    func(SupervisedCommandEvent)
}

type AgentTurnResult struct {
	Status  string
	Process map[string]any
	Error   error
}

type AgentRunner interface {
	RunTurn(ctx context.Context, request AgentTurnRequest) AgentTurnResult
}

type AgentRunnerRegistry struct {
	runners map[string]AgentRunner
}

func NewAgentRunnerRegistry() AgentRunnerRegistry {
	return AgentRunnerRegistry{runners: map[string]AgentRunner{
		"codex":       CodexExecAgentRunner{},
		"opencode":    OpenCodeAgentRunner{},
		"trae":        TraeAgentRunner{},
		"trae-agent":  TraeAgentRunner{},
		"claude-code": ClaudeCodeAgentRunner{},
		"claude":      ClaudeCodeAgentRunner{},
	}}
}

func (registry AgentRunnerRegistry) Resolve(runnerID string) (AgentRunner, string) {
	id := strings.TrimSpace(strings.ToLower(runnerID))
	if id == "" || id == "profile" || id == "auto" {
		id = "codex"
	}
	if runner, ok := registry.runners[id]; ok {
		return runner, id
	}
	return UnsupportedAgentRunner{RunnerID: id}, id
}

func isAIRunnerID(runnerID string) bool {
	switch strings.ToLower(strings.TrimSpace(runnerID)) {
	case "codex", "opencode", "trae", "trae-agent", "claude", "claude-code", "profile", "auto":
		return true
	default:
		return false
	}
}

func runnerAvailabilityError(runnerID string) error {
	normalized := strings.ToLower(strings.TrimSpace(runnerID))
	switch normalized {
	case "", "local-proof":
		return nil
	case "demo-code":
		if err := executableAvailable("git"); err != nil {
			return fmt.Errorf("runner %q requires git: %w", normalized, err)
		}
		return nil
	case "codex":
		if err := executableAvailable("codex"); err != nil {
			return fmt.Errorf("runner %q is not installed or not on PATH: %w", normalized, err)
		}
		return nil
	case "opencode":
		if err := executableAvailable("opencode"); err != nil {
			return fmt.Errorf("runner %q is not installed or not on PATH: %w", normalized, err)
		}
		return nil
	case "trae", "trae-agent":
		if err := executableAvailable("trae-cli"); err != nil {
			return fmt.Errorf("runner %q is not installed or not on PATH: %w", normalized, err)
		}
		return nil
	case "claude", "claude-code":
		if err := executableAvailable("claude"); err == nil {
			return nil
		}
		if err := executableAvailable("claude-code"); err != nil {
			return fmt.Errorf("runner %q is not installed or not on PATH: %w", normalized, err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported agent runner %q", runnerID)
	}
}

func preflightAgentRunner(requested string, profile ProjectAgentProfile, agentID string) (string, error) {
	runnerID := effectiveAgentRunnerID(requested, profile, agentID)
	if err := runnerAvailabilityError(runnerID); err != nil {
		return runnerID, fmt.Errorf("agent %q cannot start with runner %q: %w", agentID, runnerID, err)
	}
	return runnerID, nil
}

func effectiveAgentRunnerID(requested string, profile ProjectAgentProfile, agentID string) string {
	normalized := strings.ToLower(strings.TrimSpace(requested))
	if normalized == "" || normalized == "profile" || normalized == "auto" {
		return stringOr(agentProfileForRole(profile, agentID).Runner, "codex")
	}
	return normalized
}

func runnerProcessNotAvailable(runnerID string, executable string, workspace string, err error) map[string]any {
	return map[string]any{
		"runner":   runnerID,
		"command":  executable,
		"cwd":      workspace,
		"status":   "failed",
		"exitCode": -1,
		"stderr":   err.Error(),
	}
}

func executableAvailable(name string) error {
	_, err := exec.LookPath(name)
	return err
}

type CodexExecAgentRunner struct{}

func (runner CodexExecAgentRunner) RunTurn(ctx context.Context, request AgentTurnRequest) AgentTurnResult {
	model := strings.TrimSpace(request.Model)
	effort := stringOr(request.Effort, "medium")
	sandbox := stringOr(request.Sandbox, "workspace-write")
	if err := executableAvailable("codex"); err != nil {
		process := runnerProcessNotAvailable("codex", "codex", request.Workspace, err)
		process["model"] = model
		process["effort"] = effort
		return AgentTurnResult{Status: "failed", Process: process, Error: err}
	}
	args := []string{"--ask-for-approval", "never", "exec"}
	if model != "" {
		args = append(args, "--model", model)
	}
	args = append(args,
		"-c", "model_reasoning_effort=\""+effort+"\"",
		"--skip-git-repo-check",
		"--sandbox", sandbox,
		"--output-last-message", request.OutputPath,
		"-",
	)
	process, err := runSupervisedCommandContextWithOptions(
		ctx,
		SupervisedCommandOptions{HeartbeatInterval: request.HeartbeatInterval, OnEvent: request.OnProcessEvent, Env: request.Env},
		request.Workspace,
		request.Prompt,
		"codex",
		args...,
	)
	if request.OutputPath != "" {
		ensureAgentOutputFile(request.OutputPath, process)
	}
	status := "passed"
	if err != nil {
		status = "failed"
	}
	process["runner"] = "codex"
	process["model"] = model
	process["effort"] = effort
	return AgentTurnResult{Status: status, Process: process, Error: err}
}

type OpenCodeAgentRunner struct{}

func (runner OpenCodeAgentRunner) RunTurn(ctx context.Context, request AgentTurnRequest) AgentTurnResult {
	model := strings.TrimSpace(request.Model)
	if err := executableAvailable("opencode"); err != nil {
		process := runnerProcessNotAvailable("opencode", "opencode", request.Workspace, err)
		process["model"] = model
		return AgentTurnResult{Status: "failed", Process: process, Error: err}
	}
	provider, providerModel := opencodeProviderAndModel(model)
	env := cloneStringMap(request.Env)
	if configPath, configErr := writeOpenCodeRunnerConfig(request.Workspace, provider, providerModel, env); configErr == nil && configPath != "" {
		env["OPENCODE_CONFIG"] = configPath
	} else if configErr != nil {
		process := runnerProcessNotAvailable("opencode", "opencode", request.Workspace, configErr)
		process["model"] = model
		process["provider"] = provider
		return AgentTurnResult{Status: "failed", Process: process, Error: configErr}
	}
	args := []string{"run"}
	if model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, "--dangerously-skip-permissions", request.Prompt)
	process, err := runSupervisedCommandContextWithOptions(ctx, SupervisedCommandOptions{HeartbeatInterval: request.HeartbeatInterval, OnEvent: request.OnProcessEvent, Env: env}, request.Workspace, "", "opencode", args...)
	if request.OutputPath != "" {
		ensureAgentOutputFile(request.OutputPath, process)
	}
	status := "passed"
	if err != nil {
		status = "failed"
	}
	process["runner"] = "opencode"
	process["model"] = model
	if provider != "" {
		process["provider"] = provider
	}
	return AgentTurnResult{Status: status, Process: process, Error: err}
}

func writeOpenCodeRunnerConfig(workspace string, provider string, model string, env map[string]string) (string, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	model = strings.TrimSpace(model)
	if provider == "" || model == "" {
		return "", nil
	}
	prefix := credentialEnvPrefix(provider)
	baseURL := firstNonEmpty(strings.TrimSpace(env[prefix+"_BASE_URL"]), defaultRunnerProviderBaseURL(provider))
	if baseURL == "" {
		return "", nil
	}
	configDir := filepath.Join(workspace, ".omega")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return "", err
	}
	configPath := filepath.Join(configDir, "opencode-runner-config.json")
	apiKey := ""
	if strings.TrimSpace(env[prefix+"_API_KEY"]) != "" {
		apiKey = "{env:" + prefix + "_API_KEY}"
	}
	payload := map[string]any{
		"$schema": "https://opencode.ai/config.json",
		"provider": map[string]any{
			provider: map[string]any{
				"npm":  "@ai-sdk/openai-compatible",
				"name": runnerProviderDisplayName(provider),
				"options": map[string]any{
					"baseURL": baseURL,
					"apiKey":  apiKey,
				},
				"models": map[string]any{
					model: map[string]any{
						"name":      model,
						"tool_call": true,
						"reasoning": true,
						"limit": map[string]any{
							"context": 128000,
							"output":  8192,
						},
					},
				},
			},
		},
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(configPath, raw, 0o600); err != nil {
		return "", err
	}
	return configPath, nil
}

type TraeAgentRunner struct{}

func (runner TraeAgentRunner) RunTurn(ctx context.Context, request AgentTurnRequest) AgentTurnResult {
	if err := executableAvailable("trae-cli"); err != nil {
		process := runnerProcessNotAvailable("trae-agent", "trae-cli", request.Workspace, err)
		return AgentTurnResult{Status: "failed", Process: process, Error: err}
	}
	args := []string{"run", request.Prompt, "--working-dir", request.Workspace}
	provider, model := traeProviderAndModel(request.Model)
	if model == "" {
		model = strings.TrimSpace(os.Getenv("OMEGA_TRAE_MODEL"))
	}
	cliProvider := traeCLIProvider(provider)
	env := mergeEnvMaps(traeProviderEnv(provider), request.Env)
	env = addTraeProviderCompatibilityEnv(provider, cliProvider, env)
	modelBaseURL := traeModelBaseURL(provider, cliProvider, env)
	configPath, configErr := writeTraeRunnerConfig(request.Workspace, cliProvider, model)
	if configErr != nil {
		process := runnerProcessNotAvailable("trae-agent", "trae-cli", request.Workspace, configErr)
		process["provider"] = provider
		process["model"] = model
		return AgentTurnResult{Status: "failed", Process: process, Error: configErr}
	}
	if configPath != "" {
		args = append(args, "--config-file", configPath)
	}
	if model != "" {
		if cliProvider != "" {
			args = append(args, "--provider", cliProvider)
		}
		args = append(args, "--model", model)
	}
	if modelBaseURL != "" {
		args = append(args, "--model-base-url", modelBaseURL)
	}
	process, err := runSupervisedCommandContextWithOptions(
		ctx,
		SupervisedCommandOptions{HeartbeatInterval: request.HeartbeatInterval, OnEvent: request.OnProcessEvent, Env: env},
		request.Workspace,
		"",
		"trae-cli",
		args...,
	)
	if request.OutputPath != "" {
		ensureAgentOutputFile(request.OutputPath, process)
	}
	status := "passed"
	if err != nil {
		status = "failed"
	}
	process["runner"] = "trae-agent"
	if provider != "" {
		process["provider"] = provider
	}
	if cliProvider != "" && cliProvider != provider {
		process["cliProvider"] = cliProvider
	}
	if model != "" {
		process["model"] = model
	}
	return AgentTurnResult{Status: status, Process: process, Error: err}
}

func writeTraeRunnerConfig(workspace string, provider string, model string) (string, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	model = strings.TrimSpace(model)
	if provider == "" || model == "" {
		return "", nil
	}
	configDir := filepath.Join(workspace, ".omega")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return "", err
	}
	configPath := filepath.Join(configDir, "trae-runner-config.yaml")
	raw := strings.Join([]string{
		"agents:",
		"  trae_agent:",
		"    enable_lakeview: false",
		"    model: omega_model",
		"    max_steps: 200",
		"    tools:",
		"      - bash",
		"      - str_replace_based_edit_tool",
		"      - sequentialthinking",
		"      - task_done",
		"model_providers:",
		"  " + yamlKey(provider) + ":",
		"    api_key: \"\"",
		"    provider: " + yamlString(provider),
		"    base_url: \"\"",
		"models:",
		"  omega_model:",
		"    model_provider: " + yamlString(provider),
		"    model: " + yamlString(model),
		"    max_tokens: 4096",
		"    temperature: 0.2",
		"    top_p: 1",
		"    top_k: 0",
		"    max_retries: 3",
		"    parallel_tool_calls: true",
		"",
	}, "\n")
	if err := os.WriteFile(configPath, []byte(raw), 0o600); err != nil {
		return "", err
	}
	return configPath, nil
}

func yamlKey(value string) string {
	return strings.Trim(strconv.Quote(strings.TrimSpace(value)), `"`)
}

func yamlString(value string) string {
	return strconv.Quote(strings.TrimSpace(value))
}

func traeCLIProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "kimi", "moonshot", "moonshotai":
		return "openai"
	default:
		return strings.ToLower(strings.TrimSpace(provider))
	}
}

func addTraeProviderCompatibilityEnv(provider string, cliProvider string, env map[string]string) map[string]string {
	providerPrefix := credentialEnvPrefix(provider)
	cliPrefix := credentialEnvPrefix(cliProvider)
	if providerPrefix == "" || cliPrefix == "" || providerPrefix == cliPrefix {
		return env
	}
	next := cloneStringMap(env)
	if next[cliPrefix+"_API_KEY"] == "" && next[providerPrefix+"_API_KEY"] != "" {
		next[cliPrefix+"_API_KEY"] = next[providerPrefix+"_API_KEY"]
	}
	if next[cliPrefix+"_BASE_URL"] == "" && next[providerPrefix+"_BASE_URL"] != "" {
		next[cliPrefix+"_BASE_URL"] = next[providerPrefix+"_BASE_URL"]
	}
	return next
}

func traeModelBaseURL(provider string, cliProvider string, env map[string]string) string {
	for _, candidate := range []string{provider, cliProvider} {
		prefix := credentialEnvPrefix(candidate)
		if prefix != "" && strings.TrimSpace(env[prefix+"_BASE_URL"]) != "" {
			return strings.TrimSpace(env[prefix+"_BASE_URL"])
		}
	}
	for _, candidate := range []string{provider, cliProvider} {
		if baseURL := defaultRunnerProviderBaseURL(candidate); baseURL != "" {
			return baseURL
		}
	}
	return ""
}

func traeProviderAndModel(rawModel string) (string, string) {
	model := strings.TrimSpace(rawModel)
	if model == "" || model == "gpt-5.4-mini" {
		return strings.TrimSpace(os.Getenv("OMEGA_TRAE_PROVIDER")), strings.TrimSpace(os.Getenv("OMEGA_TRAE_MODEL"))
	}
	if provider, configuredModel, ok := strings.Cut(model, ":"); ok && provider != "" && configuredModel != "" {
		return strings.TrimSpace(provider), strings.TrimSpace(configuredModel)
	}
	provider := strings.TrimSpace(os.Getenv("OMEGA_TRAE_PROVIDER"))
	if provider == "" {
		lower := strings.ToLower(model)
		switch {
		case strings.HasPrefix(lower, "doubao"):
			provider = "doubao"
		case strings.HasPrefix(lower, "claude"):
			provider = "anthropic"
		case strings.HasPrefix(lower, "gpt"):
			provider = "openai"
		case strings.HasPrefix(lower, "gemini"):
			provider = "google"
		case strings.HasPrefix(lower, "kimi"), strings.HasPrefix(lower, "moonshot"):
			provider = "kimi"
		}
	}
	return provider, model
}

func traeProviderEnv(provider string) map[string]string {
	env := map[string]string{}
	normalized := strings.ToUpper(strings.TrimSpace(provider))
	if normalized == "" {
		normalized = strings.ToUpper(strings.TrimSpace(os.Getenv("OMEGA_TRAE_PROVIDER")))
	}
	if normalized == "" {
		return env
	}
	if apiKey := strings.TrimSpace(os.Getenv("OMEGA_TRAE_API_KEY")); apiKey != "" {
		env[normalized+"_API_KEY"] = apiKey
	}
	if baseURL := strings.TrimSpace(os.Getenv("OMEGA_TRAE_BASE_URL")); baseURL != "" {
		env[normalized+"_BASE_URL"] = baseURL
	}
	return env
}

func runnerProviderDisplayName(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "kimi", "moonshot", "moonshotai":
		return "Kimi (Moonshot)"
	case "qwen":
		return "Qwen"
	case "deepseek":
		return "DeepSeek"
	case "openrouter":
		return "OpenRouter"
	case "openai":
		return "OpenAI"
	default:
		return strings.TrimSpace(provider)
	}
}

type ClaudeCodeAgentRunner struct{}

func (runner ClaudeCodeAgentRunner) RunTurn(ctx context.Context, request AgentTurnRequest) AgentTurnResult {
	model := strings.TrimSpace(request.Model)
	executable := "claude"
	if err := executableAvailable(executable); err != nil {
		if fallbackErr := executableAvailable("claude-code"); fallbackErr != nil {
			process := runnerProcessNotAvailable("claude-code", "claude", request.Workspace, err)
			if model != "" {
				process["model"] = model
			}
			return AgentTurnResult{Status: "failed", Process: process, Error: err}
		}
		executable = "claude-code"
	}
	args := []string{"-p", "-"}
	if model != "" {
		args = append(args, "--model", model)
	}
	process, err := runSupervisedCommandContextWithOptions(ctx, SupervisedCommandOptions{HeartbeatInterval: request.HeartbeatInterval, OnEvent: request.OnProcessEvent, Env: request.Env}, request.Workspace, request.Prompt, executable, args...)
	if request.OutputPath != "" {
		ensureAgentOutputFile(request.OutputPath, process)
	}
	status := "passed"
	if err != nil {
		status = "failed"
	}
	process["runner"] = "claude-code"
	if model != "" {
		process["model"] = model
	} else {
		process["model"] = "local-config"
	}
	return AgentTurnResult{Status: status, Process: process, Error: err}
}

func mergeEnvMaps(values ...map[string]string) map[string]string {
	merged := map[string]string{}
	for _, value := range values {
		for key, item := range value {
			if strings.TrimSpace(key) == "" {
				continue
			}
			merged[key] = item
		}
	}
	return merged
}

type UnsupportedAgentRunner struct {
	RunnerID string
}

func (runner UnsupportedAgentRunner) RunTurn(_ context.Context, request AgentTurnRequest) AgentTurnResult {
	err := fmt.Errorf("unsupported agent runner %q", runner.RunnerID)
	process := runnerProcessNotAvailable(runner.RunnerID, runner.RunnerID, request.Workspace, err)
	return AgentTurnResult{Status: "failed", Process: process, Error: err}
}

func ensureAgentOutputFile(outputPath string, process map[string]any) {
	raw, readErr := os.ReadFile(outputPath)
	if readErr == nil && strings.TrimSpace(string(raw)) != "" {
		return
	}
	fallback := strings.TrimSpace(stringOr(process["stdout"], ""))
	if fallback == "" {
		fallback = strings.TrimSpace(stringOr(process["stderr"], ""))
	}
	if fallback != "" {
		_ = os.WriteFile(outputPath, []byte(fallback), 0o644)
	}
}

func agentArtifactCaptureInstruction(outputPath string) string {
	outputPath = strings.TrimSpace(outputPath)
	if outputPath == "" {
		return "\n\nArtifact capture: return the complete artifact content in your final answer only."
	}
	return fmt.Sprintf("\n\nArtifact capture: return the complete artifact content in your final answer only. Do not attempt to write, patch, or edit `%s`; Omega will persist your final answer to that path after the runner exits.", outputPath)
}
