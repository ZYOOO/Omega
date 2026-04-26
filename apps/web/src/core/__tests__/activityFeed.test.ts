import { describe, expect, it } from "vitest";
import { createActivityFeed } from "../activityFeed";

describe("createActivityFeed", () => {
  it("creates readable activity items from mission events and sync intents", () => {
    const feed = createActivityFeed({
      events: [
        {
          type: "operation.started",
          missionId: "mission_1",
          workItemId: "item_intake",
          operationId: "operation_intake",
          operationTitle: "Intake",
          timestamp: "2026-04-22T00:00:00.000Z"
        },
        {
          type: "operation.completed",
          missionId: "mission_1",
          workItemId: "item_intake",
          operationId: "operation_intake",
          operationTitle: "Intake",
          timestamp: "2026-04-22T00:01:00.000Z"
        }
      ],
      syncIntents: [
        {
          provider: "workboard",
          action: "update-status",
          targetId: "item_intake",
          payload: { status: "Done" }
        }
      ]
    });

    expect(feed).toEqual([
      {
        id: "event_0",
        kind: "event",
        title: "Intake started",
        detail: "operation_intake",
        timestamp: "2026-04-22T00:00:00.000Z"
      },
      {
        id: "event_1",
        kind: "event",
        title: "Intake completed",
        detail: "operation_intake",
        timestamp: "2026-04-22T00:01:00.000Z"
      },
      {
        id: "sync_0",
        kind: "sync",
        title: "Work item status updated",
        detail: "item_intake"
      }
    ]);
  });
});
