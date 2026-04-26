import type { WorkItemStatus } from "./workboard";

export type MissionEvent =
  | {
      type: "operation.started";
      missionId: string;
      workItemId: string;
      operationId: string;
      operationTitle: string;
      timestamp: string;
    }
  | {
      type: "operation.proof-attached";
      missionId: string;
      workItemId: string;
      operationId: string;
      operationTitle: string;
      proofFiles: string[];
      summary: string;
      timestamp: string;
    }
  | {
      type: "checkpoint.requested";
      missionId: string;
      workItemId: string;
      operationId: string;
      operationTitle: string;
      checkpointReason: string;
      timestamp: string;
    }
  | {
      type: "operation.completed";
      missionId: string;
      workItemId: string;
      operationId: string;
      operationTitle: string;
      timestamp: string;
    }
  | {
      type: "operation.failed";
      missionId: string;
      workItemId: string;
      operationId: string;
      operationTitle: string;
      error: string;
      timestamp: string;
    };

export type SyncProvider = "workboard" | "github" | "feishu" | "ci";
export type SyncAction =
  | "update-status"
  | "attach-proof"
  | "annotate-branch"
  | "comment"
  | "request-approval";

export interface SyncIntent {
  provider: SyncProvider;
  action: SyncAction;
  targetId: string;
  payload: Record<string, unknown>;
}

function proofCommentBody(operationTitle: string, summary: string, proofFiles: string[]): string {
  return [
    `Mission Control proof for ${operationTitle}`,
    "",
    summary,
    "",
    "Proof files:",
    ...proofFiles.map((proofFile) => `- ${proofFile}`)
  ].join("\n");
}

function workboardStatusIntent(
  workItemId: string,
  status: WorkItemStatus
): SyncIntent {
  return {
    provider: "workboard",
    action: "update-status",
    targetId: workItemId,
    payload: { status }
  };
}

export function planSyncIntents(event: MissionEvent): SyncIntent[] {
  if (event.type === "operation.started") {
    return [
      workboardStatusIntent(event.workItemId, "In Review"),
      {
        provider: "github",
        action: "annotate-branch",
        targetId: event.missionId,
        payload: { operationId: event.operationId, status: "started" }
      }
    ];
  }

  if (event.type === "operation.proof-attached") {
    return [
      {
        provider: "workboard",
        action: "attach-proof",
        targetId: event.workItemId,
        payload: {
          operationTitle: event.operationTitle,
          proofFiles: event.proofFiles,
          summary: event.summary
        }
      },
      {
        provider: "github",
        action: "comment",
        targetId: event.missionId,
        payload: {
          body: proofCommentBody(event.operationTitle, event.summary, event.proofFiles)
        }
      }
    ];
  }

  if (event.type === "checkpoint.requested") {
    return [
      workboardStatusIntent(event.workItemId, "Ready"),
      {
        provider: "feishu",
        action: "request-approval",
        targetId: event.missionId,
        payload: {
          title: `${event.operationTitle} checkpoint`,
          reason: event.checkpointReason
        }
      }
    ];
  }

  if (event.type === "operation.completed") {
    return [workboardStatusIntent(event.workItemId, "Done")];
  }

  return [workboardStatusIntent(event.workItemId, "Blocked")];
}
