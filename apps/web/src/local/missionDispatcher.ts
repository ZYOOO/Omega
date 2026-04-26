import type { Mission, MissionOperation } from "../core/mission";
import { runLocalMissionJob, type LocalMissionJobResult, type LocalRunnerCommand } from "./localMissionRunner";

export interface DispatchMissionOperationInput {
  mission: Mission;
  operation: MissionOperation;
  workspaceRoot: string;
  command: LocalRunnerCommand;
}

export function dispatchMissionOperation(
  input: DispatchMissionOperationInput
): Promise<LocalMissionJobResult> {
  return runLocalMissionJob({
    runId: input.mission.id,
    issueKey: input.mission.sourceIssueKey,
    stageId: input.operation.stageId,
    agentId: input.operation.agentId,
    workspaceRoot: input.workspaceRoot,
    prompt: input.operation.prompt,
    command: input.command
  });
}
