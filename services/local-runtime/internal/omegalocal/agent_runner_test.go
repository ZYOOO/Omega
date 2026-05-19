package omegalocal

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestCodexReadOnlySandboxCapturesOutputOutsideWorkspace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake codex script uses POSIX sh")
	}
	bin := t.TempDir()
	workspace := t.TempDir()
	outputPath := filepath.Join(workspace, ".omega", "proof", "requirement-handoff.md")
	argsLog := filepath.Join(bin, "codex.args")
	codex := filepath.Join(bin, "codex")
	script := "#!/bin/sh\n" +
		"output=\"\"\n" +
		"while [ \"$#\" -gt 0 ]; do\n" +
		"  if [ \"$1\" = \"--output-last-message\" ]; then\n" +
		"    shift\n" +
		"    output=\"$1\"\n" +
		"  fi\n" +
		"  shift\n" +
		"done\n" +
		"printf '%s\\n' \"$output\" > " + strconv.Quote(argsLog) + "\n" +
		"case \"$output\" in\n" +
		"  " + strconv.Quote(workspace) + "*) echo 'output path should not be inside read-only workspace' >&2; exit 23 ;;\n" +
		"esac\n" +
		"cat >/dev/null\n" +
		"printf '%s\\n' '# Requirement Handoff' '' 'Captured from Codex final answer.' > \"$output\"\n" +
		"printf '%s\\n' 'stdout fallback should not replace captured file'\n"
	if err := os.WriteFile(codex, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	result := CodexExecAgentRunner{}.RunTurn(context.Background(), AgentTurnRequest{
		Workspace:  workspace,
		Prompt:     "Prepare a requirement handoff.",
		OutputPath: outputPath,
		Sandbox:    "read-only",
		Effort:     "medium",
	})
	if result.Error != nil || result.Status != "passed" {
		t.Fatalf("codex turn failed: status=%s error=%v process=%+v", result.Status, result.Error, result.Process)
	}
	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "# Requirement Handoff") {
		t.Fatalf("output file was not copied from capture path: %s", raw)
	}
	argsRaw, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(strings.TrimSpace(string(argsRaw)), workspace) {
		t.Fatalf("read-only capture path should be outside workspace: %s", argsRaw)
	}
}

func TestCodexWorkspaceWriteSandboxCapturesOutputInsideWorkspace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake codex script uses POSIX sh")
	}
	bin := t.TempDir()
	attemptWorkspace := t.TempDir()
	repoWorkspace := filepath.Join(attemptWorkspace, "repo")
	if err := os.MkdirAll(repoWorkspace, 0o755); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(attemptWorkspace, ".omega", "proof", "coding-agent-note.md")
	argsLog := filepath.Join(bin, "codex.args")
	codex := filepath.Join(bin, "codex")
	expectedCapturePrefix := filepath.Join(repoWorkspace, ".omega", "agent-output") + string(os.PathSeparator)
	script := "#!/bin/sh\n" +
		"output=\"\"\n" +
		"while [ \"$#\" -gt 0 ]; do\n" +
		"  if [ \"$1\" = \"--output-last-message\" ]; then\n" +
		"    shift\n" +
		"    output=\"$1\"\n" +
		"  fi\n" +
		"  shift\n" +
		"done\n" +
		"printf '%s\\n' \"$output\" > " + strconv.Quote(argsLog) + "\n" +
		"case \"$output\" in\n" +
		"  " + strconv.Quote(expectedCapturePrefix) + "*) ;;\n" +
		"  *) echo 'output path should be captured inside the writable repo workspace' >&2; exit 24 ;;\n" +
		"esac\n" +
		"cat >/dev/null\n" +
		"printf '%s\\n' '# Coding Note' '' 'Captured inside repo-local runner output.' > \"$output\"\n"
	if err := os.WriteFile(codex, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	result := CodexExecAgentRunner{}.RunTurn(context.Background(), AgentTurnRequest{
		Workspace:  repoWorkspace,
		Prompt:     "Implement the feature and summarize it.",
		OutputPath: outputPath,
		Sandbox:    "workspace-write",
		Effort:     "medium",
	})
	if result.Error != nil || result.Status != "passed" {
		t.Fatalf("codex turn failed: status=%s error=%v process=%+v", result.Status, result.Error, result.Process)
	}
	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "# Coding Note") {
		t.Fatalf("output file was not copied from workspace capture path: %s", raw)
	}
	argsRaw, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatal(err)
	}
	capturePath := strings.TrimSpace(string(argsRaw))
	if capturePath == outputPath {
		t.Fatalf("workspace-write capture path should not be the proof path outside repo workspace")
	}
	if !strings.HasPrefix(capturePath, expectedCapturePrefix) {
		t.Fatalf("workspace-write capture path should be inside repo workspace: %s", capturePath)
	}
}
