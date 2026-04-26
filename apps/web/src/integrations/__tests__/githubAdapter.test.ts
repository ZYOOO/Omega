import { describe, expect, it } from "vitest";
import {
  createGitHubProofComment,
  mapGitHubCheckRunToProof,
  mapGitHubIssueToWorkItem,
  mapGitHubPullRequestToProof
} from "../githubAdapter";

describe("githubAdapter", () => {
  it("maps a GitHub issue into an Omega WorkItem", () => {
    const item = mapGitHubIssueToWorkItem({
      id: 101,
      number: 7,
      title: "Add delivery proof",
      body: "Need proof collection.",
      state: "open",
      labels: [{ name: "ai-delivery" }],
      assignees: [{ login: "zyong" }]
    });

    expect(item).toMatchObject({
      id: "github_issue_101",
      key: "GH-7",
      title: "Add delivery proof",
      description: "Need proof collection.",
      status: "Ready",
      priority: "No priority",
      assignee: "zyong",
      labels: ["ai-delivery"],
      team: "GitHub"
    });
  });

  it("maps pull request and checks into proof records", () => {
    expect(
      mapGitHubPullRequestToProof({
        number: 12,
        title: "Implement tests",
        html_url: "https://github.com/acme/repo/pull/12",
        mergeable_state: "clean"
      })
    ).toEqual({
      id: "github_pr_12",
      label: "Pull Request",
      value: "#12 Implement tests (clean)",
      url: "https://github.com/acme/repo/pull/12"
    });

    expect(
      mapGitHubCheckRunToProof({
        name: "coverage",
        status: "completed",
        conclusion: "success",
        html_url: "https://github.com/acme/repo/actions/runs/1"
      })
    ).toEqual({
      id: "github_check_coverage",
      label: "Check Run",
      value: "coverage: completed / success",
      url: "https://github.com/acme/repo/actions/runs/1"
    });
  });

  it("creates GitHub proof comments from mission proof files", () => {
    expect(
      createGitHubProofComment({
        operationTitle: "Testing",
        summary: "Coverage passed.",
        proofFiles: [".omega/proof/coverage.txt"]
      })
    ).toBe(
      "Mission Control proof for Testing\n\nCoverage passed.\n\nProof files:\n- .omega/proof/coverage.txt"
    );
  });
});
