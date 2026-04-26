package omegalocal

import (
	"context"
	"os"
	"strings"
)

type AgentTurnRequest struct {
	Role       string
	StageID    string
	Runner     string
	Workspace  string
	Prompt     string
	OutputPath string
	Sandbox    string
	Model      string
	Effort     string
}

type AgentTurnResult struct {
	Status  string
	Process map[string]any
	Error   error
}

type AgentRunner interface {
	RunTurn(ctx context.Context, request AgentTurnRequest) AgentTurnResult
}

type CodexExecAgentRunner struct{}

func (runner CodexExecAgentRunner) RunTurn(ctx context.Context, request AgentTurnRequest) AgentTurnResult {
	model := stringOr(request.Model, "gpt-5.4-mini")
	effort := stringOr(request.Effort, "medium")
	sandbox := stringOr(request.Sandbox, "workspace-write")
	process, err := runSupervisedCommand(
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
	return AgentTurnResult{Status: status, Process: process, Error: err}
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
