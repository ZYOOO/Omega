import type { SyncIntent } from "../core";
import { createFeishuApprovalCard } from "./feishuAdapter";
import type { FeishuClient } from "./feishuClient";

export async function executeFeishuSyncIntent(
  client: FeishuClient,
  intent: SyncIntent
): Promise<void> {
  if (intent.provider !== "feishu") {
    return;
  }

  if (intent.action === "request-approval") {
    await client.sendCard(
      createFeishuApprovalCard({
        missionId: intent.targetId,
        operationTitle: String(intent.payload.title).replace(/\s+checkpoint$/, ""),
        reason: String(intent.payload.reason)
      })
    );
  }
}
