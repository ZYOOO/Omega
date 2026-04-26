import { describe, expect, it } from "vitest";
import {
  buildAuthorizeUrl,
  connectionProviders,
  createInitialConnectionState,
  grantProviderConnection,
  providerHasPermission
} from "../connections";

describe("connection providers", () => {
  it("models GitHub permissions required for repo, pull request, and CI access", () => {
    const github = connectionProviders.find((provider) => provider.id === "github");

    expect(github?.authMethod).toBe("oauth");
    expect(github?.scopes).toEqual(expect.arrayContaining(["repo", "read:org", "workflow"]));
    expect(github?.permissions.map((permission) => permission.id)).toEqual(
      expect.arrayContaining(["repo:read", "pull_request:write", "checks:read"])
    );
  });

  it("builds an OAuth authorize URL with encoded scopes and state", () => {
    const url = buildAuthorizeUrl("github", {
      clientId: "omega-client",
      redirectUri: "http://localhost:5173/auth/callback",
      state: "run_123"
    });

    expect(url).toBe(
      "https://github.com/login/oauth/authorize?client_id=omega-client&redirect_uri=http%3A%2F%2Flocalhost%3A5173%2Fauth%2Fcallback&scope=repo+read%3Aorg+workflow&state=run_123"
    );
  });

  it("grants provider permissions after a connection is approved", () => {
    const initial = createInitialConnectionState();

    expect(providerHasPermission(initial, "github", "repo:read")).toBe(false);

    const connected = grantProviderConnection(initial, "github");

    expect(connected.github.status).toBe("connected");
    expect(providerHasPermission(connected, "github", "repo:read")).toBe(true);
    expect(providerHasPermission(connected, "github", "deployments:write")).toBe(false);
  });
});
