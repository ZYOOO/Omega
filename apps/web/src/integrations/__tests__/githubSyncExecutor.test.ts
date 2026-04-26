import { describe, expect, it } from "vitest";
import { InMemoryGitHubClient } from "../githubClient";
import { executeGitHubSyncIntent } from "../githubSyncExecutor";

describe("executeGitHubSyncIntent", () => {
  it("creates an issue comment for GitHub comment intents", async () => {
    const client = new InMemoryGitHubClient({
      issues: [{ id: 1, number: 7, title: "Issue", state: "open", labels: [], assignees: [] }],
      pullRequests: [],
      checkRuns: []
    });

    await executeGitHubSyncIntent(client, {
      provider: "github",
      action: "comment",
      targetId: "GH-7",
      payload: { body: "Proof attached" }
    });

    expect(client.comments).toEqual([{ issueNumber: 7, body: "Proof attached" }]);
  });

  it("ignores non-GitHub intents", async () => {
    const client = new InMemoryGitHubClient({ issues: [], pullRequests: [], checkRuns: [] });

    await executeGitHubSyncIntent(client, {
      provider: "workboard",
      action: "update-status",
      targetId: "item_1",
      payload: { status: "Done" }
    });

    expect(client.comments).toEqual([]);
  });
});
