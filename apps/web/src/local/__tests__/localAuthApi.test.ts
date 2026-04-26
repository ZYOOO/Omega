import { mkdtemp, rm } from "fs/promises";
import { join } from "path";
import { tmpdir } from "os";
import { describe, expect, it } from "vitest";
import { startLocalAuthApi } from "../localAuthApi";
import { InMemoryTokenStore } from "../../integrations/tokenStore";

describe("startLocalAuthApi", () => {
  it("handles GitHub OAuth callback and stores token", async () => {
    const workspaceRoot = await mkdtemp(join(tmpdir(), "omega-auth-"));
    const tokenStore = new InMemoryTokenStore();
    const api = await startLocalAuthApi({
      workspaceRoot,
      tokenStore,
      github: {
        clientId: "client",
        clientSecret: "secret",
        redirectUri: "http://127.0.0.1:3888/auth/github/callback"
      },
      fetchImpl: async () =>
        new Response(
          JSON.stringify({
            access_token: "gho_token",
            scope: "repo,workflow",
            token_type: "bearer"
          }),
          { status: 200, headers: { "content-type": "application/json" } }
        ),
      port: 0
    });

    try {
      const response = await fetch(`${api.url}/auth/github/callback?code=code_123&state=omega`);
      expect(response.status).toBe(200);
      expect(await response.json()).toEqual({
        connected: true,
        accountId: "omega",
        scopes: ["repo", "workflow"]
      });
      expect(await tokenStore.getToken("github", "omega")).toMatchObject({
        accessToken: "gho_token"
      });
    } finally {
      await api.close();
      await rm(workspaceRoot, { recursive: true, force: true });
    }
  });
});
