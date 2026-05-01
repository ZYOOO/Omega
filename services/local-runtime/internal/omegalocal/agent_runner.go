package omegalocal

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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
	model := stringOr(request.Model, "gpt-5.4-mini")
	effort := stringOr(request.Effort, "medium")
	sandbox := stringOr(request.Sandbox, "workspace-write")
	if err := executableAvailable("codex"); err != nil {
		process := runnerProcessNotAvailable("codex", "codex", request.Workspace, err)
		return AgentTurnResult{Status: "failed", Process: process, Error: err}
	}
	process, err := runSupervisedCommandContextWithOptions(
		ctx,
		SupervisedCommandOptions{HeartbeatInterval: request.HeartbeatInterval, OnEvent: request.OnProcessEvent, Env: request.Env},
		request.Workspace,
		request.Prompt,
		"codex",
		"--ask-for-approval", "never",
		"exec",
		"--model", model,
		"-c", "model_reasoning_effort=\""+effort+"\"",
		"--skip-git-repo-check",
		"--sandbox", sandbox,
		"--output-last-message", request.OutputPath,
		"-",
	)
	if request.OutputPath != "" {
		ensureAgentOutputFile(request.OutputPath, process)
	}
	status := "passed"
	if err != nil {
		status = "failed"
	}
	process["runner"] = "codex"
	return AgentTurnResult{Status: status, Process: process, Error: err}
}

type OpenCodeAgentRunner struct{}

func (runner OpenCodeAgentRunner) RunTurn(ctx context.Context, request AgentTurnRequest) AgentTurnResult {
	model := stringOr(request.Model, "gpt-5.4-mini")
	if err := executableAvailable("opencode"); err != nil {
		process := runnerProcessNotAvailable("opencode", "opencode", request.Workspace, err)
		return AgentTurnResult{Status: "failed", Process: process, Error: err}
	}
	args := []string{"run", "--model", model, "-"}
	process, err := runSupervisedCommandContextWithOptions(ctx, SupervisedCommandOptions{HeartbeatInterval: request.HeartbeatInterval, OnEvent: request.OnProcessEvent, Env: request.Env}, request.Workspace, request.Prompt, "opencode", args...)
	if request.OutputPath != "" {
		ensureAgentOutputFile(request.OutputPath, process)
	}
	status := "passed"
	if err != nil {
		status = "failed"
	}
	process["runner"] = "opencode"
	return AgentTurnResult{Status: status, Process: process, Error: err}
}

type TraeAgentRunner struct{}

func (runner TraeAgentRunner) RunTurn(ctx context.Context, request AgentTurnRequest) AgentTurnResult {
	if err := executableAvailable("trae-cli"); err != nil {
		process := runnerProcessNotAvailable("trae-agent", "trae-cli", request.Workspace, err)
		return AgentTurnResult{Status: "failed", Process: process, Error: err}
	}
	args := []string{"run", request.Prompt, "--working-dir", request.Workspace}
	provider, model := traeProviderAndModel(request.Model)
	if model != "" {
		if provider != "" {
			args = append(args, "--provider", provider)
		}
		args = append(args, "--model", model)
	}
	process, err := runSupervisedCommandContextWithOptions(
		ctx,
		SupervisedCommandOptions{HeartbeatInterval: request.HeartbeatInterval, OnEvent: request.OnProcessEvent, Env: mergeEnvMaps(traeProviderEnv(provider), request.Env)},
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
	return AgentTurnResult{Status: status, Process: process, Error: err}
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

type ClaudeCodeAgentRunner struct{}

func (runner ClaudeCodeAgentRunner) RunTurn(ctx context.Context, request AgentTurnRequest) AgentTurnResult {
	model := stringOr(request.Model, "claude-sonnet-4-5")
	executable := "claude"
	if err := executableAvailable(executable); err != nil {
		if fallbackErr := executableAvailable("claude-code"); fallbackErr != nil {
			process := runnerProcessNotAvailable("claude-code", "claude", request.Workspace, err)
			return AgentTurnResult{Status: "failed", Process: process, Error: err}
		}
		executable = "claude-code"
	}
	args := []string{"-p", "-", "--model", model}
	process, err := runSupervisedCommandContextWithOptions(ctx, SupervisedCommandOptions{HeartbeatInterval: request.HeartbeatInterval, OnEvent: request.OnProcessEvent, Env: request.Env}, request.Workspace, request.Prompt, executable, args...)
	if request.OutputPath != "" {
		ensureAgentOutputFile(request.OutputPath, process)
	}
	status := "passed"
	if err != nil {
		status = "failed"
	}
	process["runner"] = "claude-code"
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
