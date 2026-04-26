import { describe, expect, it } from "vitest";
import { InMemoryTokenStore } from "../tokenStore";

describe("InMemoryTokenStore", () => {
  it("stores, reads, and revokes provider tokens by account", async () => {
    const store = new InMemoryTokenStore();

    await store.saveToken({
      provider: "github",
      accountId: "omega",
      accessToken: "gho_token",
      scopes: ["repo", "workflow"]
    });

    expect(await store.getToken("github", "omega")).toEqual({
      provider: "github",
      accountId: "omega",
      accessToken: "gho_token",
      scopes: ["repo", "workflow"]
    });

    await store.revokeToken("github", "omega");
    expect(await store.getToken("github", "omega")).toBeUndefined();
  });
});
