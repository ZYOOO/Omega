import { describe, expect, it } from "vitest";
import { InMemoryFeishuClient } from "../feishuClient";

describe("InMemoryFeishuClient", () => {
  it("stores sent cards", async () => {
    const client = new InMemoryFeishuClient();

    await client.sendCard({
      cardType: "approval",
      title: "Review checkpoint",
      missionId: "mission_1",
      actions: ["Approve", "Request changes", "Pause"],
      body: "Needs approval."
    });

    expect(client.cards).toEqual([
      {
        cardType: "approval",
        title: "Review checkpoint",
        missionId: "mission_1",
        actions: ["Approve", "Request changes", "Pause"],
        body: "Needs approval."
      }
    ]);
  });
});
