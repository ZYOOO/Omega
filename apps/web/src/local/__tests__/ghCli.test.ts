import { describe, expect, it } from "vitest";
import { getGhStatus, getGitHubRepoInfo, importGitHubIssuesAsWorkItems, type CommandRunner } from "../ghCli";

function createRunner(stdout: string, exitCode = 0, stderr = ""): CommandRunner {
  return async () => ({ stdout, stderr, exitCode });
}

describe("ghCli", () => {
  it("reports authentication status from gh auth status", async () => {
    await expect(getGhStatus(createRunner("", 1, "not logged in"))).resolves.toEqual({
      installed: true,
      authenticated: false,
      detail: "not logged in"
    });
  });

  it("reads repository info through gh repo view json", async () => {
    const repo = await getGitHubRepoInfo(
      "openai",
      "openai-openapi",
      createRunner(JSON.stringify({
        name: "openai-openapi",
        nameWithOwner: "openai/openai-openapi",
        description: "specs",
        isPrivate: false,
        defaultBranchRef: { name: "main" },
        url: "https://github.com/openai/openai-openapi"
      }))
    );

    expect(repo).toMatchObject({
      nameWithOwner: "openai/openai-openapi",
      url: "https://github.com/openai/openai-openapi"
    });
  });

  it("imports GitHub issues as Omega work items", async () => {
    const items = await importGitHubIssuesAsWorkItems(
      "openai",
      "omega",
      createRunner(JSON.stringify([
        {
          number: 12,
          title: "Import me",
          body: "Issue body",
          state: "OPEN",
          assignees: [{ login: "alice" }],
          labels: [{ name: "bug" }]
        }
      ]))
    );

    expect(items[0]).toMatchObject({
      id: "gh_issue_12",
      key: "GH-12",
      title: "Import me",
      assignee: "alice",
      team: "GitHub"
    });
  });
});
