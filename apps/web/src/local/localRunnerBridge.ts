import type { Mission, MissionEvent } from "../core";
import { dispatchMissionOperation } from "./missionDispatcher";
import type { LocalRunnerCommand } from "./localMissionRunner";

export interface RunOperationRequest {
  mission: Mission;
  operationId: string;
  command: LocalRunnerCommand;
}

export interface RunOperationResponse {
  operationId: string;
  status: "passed" | "failed";
  workspacePath: string;
  proofFiles: string[];
  stdout: string;
  stderr: string;
  events: MissionEvent[];
}

export class LocalRunnerBridge {
  constructor(private readonly workspaceRoot: string) {}

  async runOperation(request: RunOperationRequest): Promise<RunOperationResponse> {
    const operation = request.mission.operations.find(
      (candidate) => candidate.id === request.operationId
    );

    if (!operation) {
      throw new Error(`Unknown operation: ${request.operationId}`);
    }

    const started: MissionEvent = {
      type: "operation.started",
      missionId: request.mission.id,
      workItemId: request.mission.sourceWorkItemId,
      operationId: operation.id,
      operationTitle: request.mission.title,
      timestamp: new Date().toISOString()
    };

    const result = await dispatchMissionOperation({
      mission: request.mission,
      operation,
      workspaceRoot: this.workspaceRoot,
      command: request.command
    });

    const proofAttached: MissionEvent = {
      type: "operation.proof-attached",
      missionId: request.mission.id,
      workItemId: request.mission.sourceWorkItemId,
      operationId: operation.id,
      operationTitle: request.mission.title,
      proofFiles: result.proofFiles,
      summary: result.status === "passed" ? "Proof collected." : "Runner failed before proof completed.",
      timestamp: new Date().toISOString()
    };

    const terminal: MissionEvent =
      result.status === "passed"
        ? {
            type: "operation.completed",
            missionId: request.mission.id,
            workItemId: request.mission.sourceWorkItemId,
            operationId: operation.id,
            operationTitle: request.mission.title,
            timestamp: new Date().toISOString()
          }
        : {
            type: "operation.failed",
            missionId: request.mission.id,
            workItemId: request.mission.sourceWorkItemId,
            operationId: operation.id,
            operationTitle: request.mission.title,
            error: result.stderr,
            timestamp: new Date().toISOString()
          };

    return {
      operationId: operation.id,
      status: result.status,
      workspacePath: result.workspacePath,
      proofFiles: result.proofFiles,
      stdout: result.stdout,
      stderr: result.stderr,
      events: [started, proofAttached, terminal]
    };
  }
}
