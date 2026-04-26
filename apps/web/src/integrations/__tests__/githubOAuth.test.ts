import { describe, expect, it } from "vitest";
import { exchangeGitHubOAuthCode } from "../githubOAuth";

describe("exchangeGitHubOAuthCode", () => {
  it("exchanges an OAuth code for a token", async () => {
    const calls: Array<{ url: string; init: RequestInit }> = [];
    const token = await exchangeGitHubOAuthCode({
      clientId: "client",
      clientSecret: "secret",
      code: "code_123",
      redirectUri: "http://localhost:5173/auth/github/callback",
      fetchImpl: async (url, init) => {
        calls.push({ url: String(url), init: init ?? {} });
        return new Response(
          JSON.stringify({
            access_token: "gho_token",
            scope: "repo,workflow",
            token_type: "bearer"
          }),
          { status: 200, headers: { "content-type": "application/json" } }
        );
      }
    });

    expect(token).toEqual({
      provider: "github",
      accountId: "default",
      accessToken: "gho_token",
      scopes: ["repo", "workflow"]
    });
    expect(calls[0].url).toBe("https://github.com/login/oauth/access_token");
    expect(calls[0].init.method).toBe("POST");
    expect(calls[0].init.headers).toMatchObject({ accept: "application/json" });
  });
});
