import type { Mission } from "./core";
import type { MissionEvent } from "./core";

export type MissionControlRunnerPreset = "local-proof" | "demo-code" | "codex";

export interface RunOperationViaMissionControlApiInput {
  apiUrl: string;
  mission: Mission;
  operationId: string;
  runner: MissionControlRunnerPreset;
  fetchImpl?: typeof fetch;
}

export interface RunOperationViaMissionControlApiResponse {
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

export async function runOperationViaMissionControlApi(
  input: RunOperationViaMissionControlApiInput
): Promise<RunOperationViaMissionControlApiResponse> {
  const fetcher = input.fetchImpl ?? fetch;
  const response = await fetcher(`${input.apiUrl.replace(/\/$/, "")}/run-operation`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({
      mission: input.mission,
      operationId: input.operationId,
      runner: input.runner
    })
  });

  if (!response.ok) {
    throw new Error(`Mission Control API failed: ${response.status}`);
  }

  return response.json() as Promise<RunOperationViaMissionControlApiResponse>;
}
