import { describe, expect, it } from "vitest";
import { GitHubRestClient } from "../githubRestClient";

describe("GitHubRestClient", () => {
  it("calls GitHub REST endpoints with bearer token", async () => {
    const calls: Array<{ url: string; init: RequestInit }> = [];
    const client = new GitHubRestClient({
      owner: "acme",
      repo: "omega",
      token: "gho_token",
      fetchImpl: async (url, init) => {
        calls.push({ url: String(url), init: init ?? {} });
        return new Response(JSON.stringify([]), {
          status: 200,
          headers: { "content-type": "application/json" }
        });
      }
    });

    await client.listIssues();
    await client.listCheckRuns("main");

    expect(calls[0].url).toBe("https://api.github.com/repos/acme/omega/issues?state=open");
    expect(calls[1].url).toBe("https://api.github.com/repos/acme/omega/commits/main/check-runs");
    expect(calls[0].init.headers).toMatchObject({
      authorization: "Bearer gho_token",
      accept: "application/vnd.github+json"
    });
  });

  it("posts issue comments", async () => {
    const calls: Array<{ url: string; init: RequestInit }> = [];
    const client = new GitHubRestClient({
      owner: "acme",
      repo: "omega",
      token: "gho_token",
      fetchImpl: async (url, init) => {
        calls.push({ url: String(url), init: init ?? {} });
        return new Response(JSON.stringify({ id: 1 }), { status: 201 });
      }
    });

    await client.createIssueComment({ issueNumber: 7, body: "Proof attached" });

    expect(calls[0].url).toBe("https://api.github.com/repos/acme/omega/issues/7/comments");
    expect(calls[0].init.method).toBe("POST");
    expect(JSON.parse(String(calls[0].init.body))).toEqual({ body: "Proof attached" });
  });
});
