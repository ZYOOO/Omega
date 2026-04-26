import { mkdtemp, readFile, rm } from "fs/promises";
import { join } from "path";
import { tmpdir } from "os";
import { describe, expect, it } from "vitest";
import { FileTokenStore } from "../tokenStore";

describe("FileTokenStore", () => {
  it("persists provider tokens to disk", async () => {
    const root = await mkdtemp(join(tmpdir(), "omega-token-store-"));
    const store = new FileTokenStore(root);

    try {
      await store.saveToken({
        provider: "github",
        accountId: "omega",
        accessToken: "gho_token",
        scopes: ["repo"]
      });

      expect(await store.getToken("github", "omega")).toMatchObject({
        accessToken: "gho_token",
        scopes: ["repo"]
      });
      expect(JSON.parse(await readFile(join(root, "github__omega.json"), "utf8")).provider).toBe("github");

      await store.revokeToken("github", "omega");
      expect(await store.getToken("github", "omega")).toBeUndefined();
    } finally {
      await rm(root, { recursive: true, force: true });
    }
  });
});
