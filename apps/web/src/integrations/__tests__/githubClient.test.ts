import { describe, expect, it } from "vitest";
import { buildGitHubAuthorizeUrl, InMemoryGitHubClient } from "../githubClient";

describe("githubClient", () => {
  it("builds a GitHub OAuth authorize URL", () => {
    const url = buildGitHubAuthorizeUrl({
      clientId: "github-client",
      redirectUri: "http://localhost:5173/auth/github/callback",
      state: "mission_1",
      scopes: ["repo", "read:org", "workflow"]
    });

    expect(url).toBe(
      "https://github.com/login/oauth/authorize?client_id=github-client&redirect_uri=http%3A%2F%2Flocalhost%3A5173%2Fauth%2Fgithub%2Fcallback&scope=repo+read%3Aorg+workflow&state=mission_1"
    );
  });

  it("stores fake GitHub issues, pull requests, checks, and comments", async () => {
    const client = new InMemoryGitHubClient({
      issues: [{ id: 1, number: 7, title: "Issue", state: "open", labels: [], assignees: [] }],
      pullRequests: [{ number: 2, title: "PR", html_url: "https://github.com/acme/repo/pull/2", mergeable_state: "clean" }],
      checkRuns: [{ name: "CI", status: "completed", conclusion: "success", html_url: "https://github.com/acme/repo/actions/1" }]
    });

    expect(await client.listIssues()).toHaveLength(1);
    expect(await client.getPullRequest(2)).toMatchObject({ title: "PR" });
    expect(await client.listCheckRuns("main")).toHaveLength(1);

    await client.createIssueComment({ issueNumber: 7, body: "Proof attached" });
    expect(client.comments).toEqual([{ issueNumber: 7, body: "Proof attached" }]);
  });
});
