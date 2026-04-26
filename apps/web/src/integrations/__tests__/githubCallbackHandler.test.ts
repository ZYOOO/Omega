import { describe, expect, it } from "vitest";
import { handleGitHubOAuthCallback } from "../githubCallbackHandler";
import { InMemoryTokenStore } from "../tokenStore";

describe("handleGitHubOAuthCallback", () => {
  it("exchanges code and stores the GitHub token", async () => {
    const store = new InMemoryTokenStore();
    const result = await handleGitHubOAuthCallback({
      code: "code_123",
      state: "omega",
      clientId: "client",
      clientSecret: "secret",
      redirectUri: "http://localhost:5173/auth/github/callback",
      tokenStore: store,
      fetchImpl: async () =>
        new Response(
          JSON.stringify({
            access_token: "gho_token",
            scope: "repo,workflow",
            token_type: "bearer"
          }),
          { status: 200, headers: { "content-type": "application/json" } }
        )
    });

    expect(result.connected).toBe(true);
    expect(await store.getToken("github", "omega")).toMatchObject({
      accessToken: "gho_token",
      scopes: ["repo", "workflow"]
    });
  });
});
