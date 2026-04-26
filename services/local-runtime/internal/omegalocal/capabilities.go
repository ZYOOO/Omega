package omegalocal

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

type LocalCapability struct {
	ID          string `json:"id"`
	Command     string `json:"command"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Available   bool   `json:"available"`
	Path        string `json:"path,omitempty"`
	Version     string `json:"version,omitempty"`
	Required    bool   `json:"required"`
}

type capabilityProbe struct {
	ID          string
	Command     string
	Category    string
	Description string
	Required    bool
	VersionArgs []string
}

func detectLocalCapabilities(ctx context.Context) []LocalCapability {
	probes := []capabilityProbe{
		{ID: "git", Command: "git", Category: "source-control", Description: "Local repository operations, branches, commits, and diffs.", Required: true, VersionArgs: []string{"--version"}},
		{ID: "gh", Command: "gh", Category: "github", Description: "GitHub auth, issue import, pull request creation, and repository metadata.", Required: false, VersionArgs: []string{"--version"}},
		{ID: "codex", Command: "codex", Category: "ai-runner", Description: "OpenAI Codex local coding agent runner.", Required: false, VersionArgs: []string{"--version"}},
		{ID: "opencode", Command: "opencode", Category: "ai-runner", Description: "OpenCode local coding agent runner.", Required: false, VersionArgs: []string{"--version"}},
		{ID: "lark-cli", Command: "lark-cli", Category: "feishu", Description: "Feishu/Lark notification, review prompt, and collaboration CLI.", Required: false, VersionArgs: []string{"--version"}},
	}
	capabilities := make([]LocalCapability, 0, len(probes))
	for _, probe := range probes {
		capabilities = append(capabilities, detectLocalCapability(ctx, probe))
	}
	return capabilities
}

func detectLocalCapability(ctx context.Context, probe capabilityProbe) LocalCapability {
	capability := LocalCapability{
		ID:          probe.ID,
		Command:     probe.Command,
		Category:    probe.Category,
		Description: probe.Description,
		Required:    probe.Required,
	}
	path, err := exec.LookPath(probe.Command)
	if err != nil {
		return capability
	}
	capability.Available = true
	capability.Path = path
	capability.Version = commandVersion(ctx, path, probe.VersionArgs)
	return capability
}

func commandVersion(ctx context.Context, path string, args []string) string {
	if len(args) == 0 {
		return ""
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	output, err := exec.CommandContext(timeoutCtx, path, args...).CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(output))
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimSpace(lines[0])
}
