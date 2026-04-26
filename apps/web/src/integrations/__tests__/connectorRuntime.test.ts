import { describe, expect, it } from "vitest";
import { createSampleRun, createWorkItems } from "../../core";
import { ConnectorRuntime } from "../connectorRuntime";
import { InMemoryFeishuClient } from "../feishuClient";
import { InMemoryGitHubClient } from "../githubClient";

describe("ConnectorRuntime", () => {
  it("applies workboard and GitHub sync intents", async () => {
    const github = new InMemoryGitHubClient({
      issues: [{ id: 1, number: 1, title: "Intake", state: "open", labels: [], assignees: [] }],
      pullRequests: [],
      checkRuns: []
    });
    const runtime = new ConnectorRuntime({
      workItems: createWorkItems(createSampleRun()),
      github
    });

    const report = await runtime.execute([
      {
        provider: "workboard",
        action: "update-status",
        targetId: "item_intake",
        payload: { status: "Done" }
      },
      {
        provider: "github",
        action: "comment",
        targetId: "GH-1",
        payload: { body: "Proof attached" }
      },
      {
        provider: "workboard",
        action: "attach-proof",
        targetId: "item_intake",
        payload: {
          operationTitle: "Intake",
          proofFiles: [".omega/proof/a.txt"],
          summary: "Proof collected."
        }
      }
    ]);

    expect(report.applied).toHaveLength(3);
    expect(report.failed).toHaveLength(0);
    expect(runtime.workItems[0].status).toBe("Done");
    expect(runtime.workItems[0].labels).toEqual(expect.arrayContaining(["proof-attached"]));
    expect(github.comments).toEqual([{ issueNumber: 1, body: "Proof attached" }]);
  });

  it("records failed intents without stopping later intents", async () => {
    const runtime = new ConnectorRuntime({
      workItems: createWorkItems(createSampleRun()),
      github: new InMemoryGitHubClient({ issues: [], pullRequests: [], checkRuns: [] })
    });

    const report = await runtime.execute([
      {
        provider: "github",
        action: "comment",
        targetId: "missing",
        payload: { body: "No issue number" }
      },
      {
        provider: "workboard",
        action: "update-status",
        targetId: "item_intake",
        payload: { status: "Done" }
      }
    ]);

    expect(report.applied).toHaveLength(1);
    expect(report.failed).toHaveLength(1);
    expect(runtime.workItems[0].status).toBe("Done");
  });

  it("executes Feishu approval intents", async () => {
    const feishu = new InMemoryFeishuClient();
    const runtime = new ConnectorRuntime({
      workItems: createWorkItems(createSampleRun()),
      feishu
    });

    const report = await runtime.execute([
      {
        provider: "feishu",
        action: "request-approval",
        targetId: "mission_1",
        payload: {
          title: "Review checkpoint",
          reason: "Needs approval."
        }
      }
    ]);

    expect(report.failed).toHaveLength(0);
    expect(feishu.cards[0]).toMatchObject({
      cardType: "approval",
      title: "Review checkpoint",
      missionId: "mission_1"
    });
  });
});
