import { describe, expect, it } from "vitest";
import { buildGhIssueListCommand, mapGhIssueJsonToWorkItem } from "../ghAdapter";

describe("ghAdapter", () => {
  it("builds a gh issue list command for a repository", () => {
    expect(
      buildGhIssueListCommand({
        owner: "zyong",
        repo: "omega-mission-test"
      })
    ).toEqual({
      executable: "gh",
      args: [
        "issue",
        "list",
        "--repo",
        "zyong/omega-mission-test",
        "--state",
        "all",
        "--json",
        "number,title,body,assignees,labels,state"
      ]
    });
  });

  it("maps gh issue JSON into a WorkItem", () => {
    expect(
      mapGhIssueJsonToWorkItem({
        number: 12,
        title: "Refine README proof flow",
        body: "Need a cleaner README proof section.",
        state: "OPEN",
        assignees: [{ login: "zyong" }],
        labels: [{ name: "documentation" }]
      })
    ).toMatchObject({
      key: "GH-12",
      title: "Refine README proof flow",
      description: "Need a cleaner README proof section.",
      status: "Ready",
      assignee: "zyong",
      labels: ["documentation"],
      team: "GitHub"
    });
  });
});
