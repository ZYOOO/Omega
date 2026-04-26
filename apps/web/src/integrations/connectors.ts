import type { DeliveryRequirement, PipelineRun, RunEvent } from "../core/types";

export interface TrackerConnector {
  name: string;
  fetchReadyRequirements(): Promise<DeliveryRequirement[]>;
  writeRunEvent(run: PipelineRun, event: RunEvent): Promise<void>;
}

export interface CodeHostConnector {
  name: string;
  readPullRequest(prUrl: string): Promise<{
    title: string;
    checks: "pending" | "green" | "red";
    unresolvedComments: number;
  }>;
}

export interface HumanGateConnector {
  name: string;
  requestApproval(input: {
    run: PipelineRun;
    title: string;
    summary: string;
  }): Promise<{ approved: boolean; reviewer: string; comment?: string }>;
}

export interface WorkspaceRunner {
  name: string;
  start(input: {
    run: PipelineRun;
    workspacePath: string;
    prompt: string;
  }): Promise<{ sessionId: string }>;
  stop(sessionId: string): Promise<void>;
}

export class InMemoryTrackerConnector implements TrackerConnector {
  name = "in-memory-tracker";

  constructor(private readonly requirements: DeliveryRequirement[]) {}

  async fetchReadyRequirements(): Promise<DeliveryRequirement[]> {
    return this.requirements;
  }

  async writeRunEvent(): Promise<void> {
    return Promise.resolve();
  }
}
