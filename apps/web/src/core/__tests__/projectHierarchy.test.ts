import { describe, expect, it } from "vitest";
import {
  addItemToWorkspace,
  createProject,
  createWorkspace,
  listWorkspaceItems
} from "../projectHierarchy";

describe("project hierarchy", () => {
  it("creates a project bound to a repository target", () => {
    const project = createProject({
      name: "omega-mission-test",
      repositoryTargets: [{
        id: "repo_zyong_omega-mission-test",
        kind: "github",
        owner: "zyong",
        repo: "omega-mission-test",
        defaultBranch: "main"
      }]
    });

    expect(project).toMatchObject({
      key: "PRJ-1",
      name: "omega-mission-test",
      repositoryTargets: [{
        id: "repo_zyong_omega-mission-test",
        kind: "github",
        owner: "zyong",
        repo: "omega-mission-test",
        defaultBranch: "main"
      }],
      defaultRepositoryTargetId: "repo_zyong_omega-mission-test"
    });
  });

  it("creates a workspace under a project and attaches work items", () => {
    const project = createProject({
      name: "omega-mission-test",
      repositoryTargets: [{
        id: "repo_local_target",
        kind: "local",
        path: "/Users/zyong/Projects/target",
        defaultBranch: "main"
      }]
    });
    const workspace = createWorkspace(project, {
      name: "proof-readme",
      branch: "omega/proof-readme"
    });
    const withItem = addItemToWorkspace(workspace, {
      title: "Add proof collection to README",
      description: "Update README with proof section.",
      assignee: "coding",
      priority: "High"
    });

    expect(withItem.projectId).toBe(project.id);
    expect(withItem.items[0]).toMatchObject({
      key: "ITM-1",
      title: "Add proof collection to README",
      status: "Todo",
      assignee: "coding"
    });
    expect(listWorkspaceItems(withItem)).toHaveLength(1);
  });
});
