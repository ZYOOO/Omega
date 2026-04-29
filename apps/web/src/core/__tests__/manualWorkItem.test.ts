import { describe, expect, it } from "vitest";
import { createManualWorkItem, titleFromMarkdownDescription } from "../manualWorkItem";

describe("manual work item helpers", () => {
  it("derives a title from markdown description", () => {
    expect(titleFromMarkdownDescription("\n## Add user profile page\n\n- Include tabs.")).toBe("Add user profile page");
  });

  it("creates a repository-scoped manual work item", () => {
    expect(
      createManualWorkItem(3, "Add settings", "Build settings page", "coding", "github:acme/app", "repo_acme_app")
    ).toMatchObject({
      id: "item_manual_3",
      key: "OMG-3",
      title: "Add settings",
      status: "Ready",
      source: "manual",
      repositoryTargetId: "repo_acme_app",
      acceptanceCriteria: ["The requested change can be verified by a human reviewer."]
    });
  });
});
