import { describe, expect, it } from "vitest";
import { createFeishuApprovalCard, createFeishuDeliveryNotice } from "../feishuAdapter";

describe("feishuAdapter", () => {
  it("creates an approval card payload for checkpoints", () => {
    expect(
      createFeishuApprovalCard({
        missionId: "mission_1",
        operationTitle: "Review",
        reason: "Needs human approval."
      })
    ).toEqual({
      cardType: "approval",
      title: "Review checkpoint",
      missionId: "mission_1",
      actions: ["Approve", "Request changes", "Pause"],
      body: "Needs human approval."
    });
  });

  it("creates a delivery notice payload with proof files", () => {
    expect(
      createFeishuDeliveryNotice({
        missionId: "mission_1",
        summary: "Delivery complete.",
        proofFiles: [".omega/proof/release.txt"]
      })
    ).toEqual({
      cardType: "notice",
      title: "Mission mission_1 delivery update",
      body: "Delivery complete.",
      proofFiles: [".omega/proof/release.txt"]
    });
  });
});
