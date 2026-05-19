import type { MissionEvent } from "./core";

export type RunnerPreset = "local-proof" | "demo-code" | "codex" | "opencode" | "claude-code" | "trae-agent";

export interface RunOperationResponse {
  operationId: string;
  status: "passed" | "failed";
  workspacePath: string;
  proofFiles: string[];
  stdout: string;
  stderr: string;
  branchName?: string;
  commitSha?: string;
  changedFiles?: string[];
  events: MissionEvent[];
}
