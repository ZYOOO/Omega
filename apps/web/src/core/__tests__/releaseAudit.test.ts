import { describe, expect, it } from "vitest";
import { createReleaseAuditEntry } from "../releaseAudit";

describe("createReleaseAuditEntry", () => {
  it("creates a release audit entry from mission proof", () => {
    const entry = createReleaseAuditEntry({
      missionId: "mission_1",
      workItemKey: "OMG-6",
      title: "Delivery",
      summary: "Release is ready.",
      proofFiles: [".omega/proof/release.txt"],
      rollbackPlan: "Revert the release branch.",
      releasedBy: "delivery"
    });

    expect(entry).toMatchObject({
      id: "release_mission_1",
      missionId: "mission_1",
      workItemKey: "OMG-6",
      title: "Delivery",
      summary: "Release is ready.",
      rollbackPlan: "Revert the release branch.",
      releasedBy: "delivery",
      proofFiles: [".omega/proof/release.txt"]
    });
    expect(entry.createdAt).toBeTruthy();
  });
});
