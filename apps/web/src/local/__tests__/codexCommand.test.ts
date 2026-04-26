import { describe, expect, it } from "vitest";
import { createCodexCommand } from "../codexCommand";

describe("createCodexCommand", () => {
  it("creates a local Codex CLI command for a workspace job prompt", () => {
    const command = createCodexCommand({
      promptFilePath: ".omega/prompt.md",
      model: "gpt-5.4",
      reasoningEffort: "medium"
    });

    expect(command).toEqual({
      executable: "codex",
      args: [
        "--ask-for-approval",
        "never",
        "exec",
        "--model",
        "gpt-5.4",
        "-c",
        "model_reasoning_effort=\"medium\"",
        "--skip-git-repo-check",
        "--sandbox",
        "workspace-write",
        "--output-last-message",
        ".omega/proof/codex-last-message.txt",
        "-"
      ],
      stdinFile: ".omega/prompt.md"
    });
  });

  it("allows overriding the executable for app-specific Codex binaries", () => {
    const command = createCodexCommand({
      executable: "/Applications/Codex.app/Contents/Resources/codex",
      promptFilePath: ".omega/prompt.md"
    });

    expect(command.executable).toBe("/Applications/Codex.app/Contents/Resources/codex");
    expect(command.stdinFile).toBe(".omega/prompt.md");
    expect(command.args).toContain(".omega/proof/codex-last-message.txt");
  });
});
