export type CleanupWorkspaceStatus = "idle" | "active" | "waiting-human" | "failed" | "completed";

export interface CleanupWorkspace {
  path: string;
  status: CleanupWorkspaceStatus;
  updatedAt: string;
}

export interface WorkspaceCleanupInput {
  now: string;
  maxAgeHours: number;
  workspaces: CleanupWorkspace[];
}

export interface WorkspaceCleanupItem {
  path: string;
  reason: string;
}

export interface WorkspaceCleanupPlan {
  candidates: WorkspaceCleanupItem[];
  retained: WorkspaceCleanupItem[];
}

function ageHours(now: string, updatedAt: string): number {
  return (Date.parse(now) - Date.parse(updatedAt)) / (1000 * 60 * 60);
}

export function planWorkspaceCleanup(input: WorkspaceCleanupInput): WorkspaceCleanupPlan {
  return input.workspaces.reduce<WorkspaceCleanupPlan>(
    (plan, workspace) => {
      if (workspace.status === "active" || workspace.status === "waiting-human") {
        plan.retained.push({ path: workspace.path, reason: `workspace is ${workspace.status}` });
        return plan;
      }

      if (ageHours(input.now, workspace.updatedAt) < input.maxAgeHours) {
        plan.retained.push({ path: workspace.path, reason: "workspace age is below threshold" });
        return plan;
      }

      plan.candidates.push({
        path: workspace.path,
        reason: `${workspace.status} workspace older than ${input.maxAgeHours}h`
      });
      return plan;
    },
    { candidates: [], retained: [] }
  );
}
