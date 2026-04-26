import type { SyncIntent } from "../core";
import type { GitHubClient } from "./githubClient";

function issueNumberFromTarget(targetId: string): number {
  const match = targetId.match(/(\d+)$/);
  if (!match) {
    throw new Error(`Cannot infer GitHub issue number from ${targetId}`);
  }
  return Number(match[1]);
}

export async function executeGitHubSyncIntent(
  client: GitHubClient,
  intent: SyncIntent
): Promise<void> {
  if (intent.provider !== "github") {
    return;
  }

  if (intent.action === "comment") {
    await client.createIssueComment({
      issueNumber: issueNumberFromTarget(intent.targetId),
      body: String(intent.payload.body)
    });
  }
}
