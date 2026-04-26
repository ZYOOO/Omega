export interface ReleaseAuditInput {
  missionId: string;
  workItemKey: string;
  title: string;
  summary: string;
  proofFiles: string[];
  rollbackPlan: string;
  releasedBy: string;
}

export interface ReleaseAuditEntry extends ReleaseAuditInput {
  id: string;
  createdAt: string;
}

export function createReleaseAuditEntry(input: ReleaseAuditInput): ReleaseAuditEntry {
  return {
    id: `release_${input.missionId}`,
    createdAt: new Date().toISOString(),
    ...input
  };
}
