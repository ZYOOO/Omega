import type { LocalRunnerCommand } from "./localMissionRunner";

export interface CodexCommandInput {
  promptFilePath: string;
  outputLastMessageFile?: string;
  executable?: string;
  model?: string;
  reasoningEffort?: "low" | "medium" | "high" | "xhigh";
}

export function createCodexCommand(input: CodexCommandInput): LocalRunnerCommand {
  const args = ["--ask-for-approval", "never", "exec"];

  if (input.model) {
    args.push("--model", input.model);
  }

  if (input.reasoningEffort) {
    args.push("-c", `model_reasoning_effort="${input.reasoningEffort}"`);
  }

  args.push(
    "--skip-git-repo-check",
    "--sandbox",
    "workspace-write",
    "--output-last-message",
    input.outputLastMessageFile ?? ".omega/proof/codex-last-message.txt",
    "-"
  );

  return {
    executable: input.executable ?? "codex",
    args,
    stdinFile: input.promptFilePath
  };
}
