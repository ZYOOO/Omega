import { describe, expect, it } from "vitest";
import { InMemoryFeishuClient } from "../feishuClient";
import { executeFeishuSyncIntent } from "../feishuSyncExecutor";

describe("executeFeishuSyncIntent", () => {
  it("sends approval cards for checkpoint intents", async () => {
    const client = new InMemoryFeishuClient();

    await executeFeishuSyncIntent(client, {
      provider: "feishu",
      action: "request-approval",
      targetId: "mission_1",
      payload: {
        title: "Review checkpoint",
        reason: "Review needs human approval."
      }
    });

    expect(client.cards[0]).toEqual({
      cardType: "approval",
      title: "Review checkpoint",
      missionId: "mission_1",
      actions: ["Approve", "Request changes", "Pause"],
      body: "Review needs human approval."
    });
  });

  it("ignores non-Feishu intents", async () => {
    const client = new InMemoryFeishuClient();

    await executeFeishuSyncIntent(client, {
      provider: "github",
      action: "comment",
      targetId: "GH-1",
      payload: { body: "Proof" }
    });

    expect(client.cards).toEqual([]);
  });
});
