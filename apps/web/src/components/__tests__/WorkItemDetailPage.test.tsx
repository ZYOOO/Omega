import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { WorkItem } from "../../core";
import { parseWorkItemDetailHash, workItemDetailHash } from "../../workItemRoutes";
import { WorkItemDetailPage } from "../WorkItemDetailPage";

describe("WorkItemDetailPage", () => {
  afterEach(() => cleanup());

  const helpers = {
    agentShortLabel: (agentId: string) => agentId,
    attemptStatusLabel: (status: string) => status,
    operationStatusLabel: (status: string) => status,
    pipelineStageClassName: (status: string) => `stage-${status}`,
    pipelineStageLabel: (status: string) => status,
    sourceLabel: () => "Omega",
    statusClassName: (status: string) => `status-${status.toLowerCase()}`,
    workItemStatusLabel: (status: string) => status
  };

  it("renders a workpad-first detail page with compact flow and expandable records", () => {
    const workItem: WorkItem = {
      id: "item_manual_21",
      key: "OMG-21",
      title: "添加用户详情页面",
      description: "# 添加用户详情页面\n\n需要展示用户资料、账号状态和最近活动。",
      status: "In Review" as const,
      priority: "High" as const,
      assignee: "coding",
      labels: ["manual"],
      team: "Omega",
      stageId: "coding",
      target: "ZYOOO/TestRepo",
      source: "manual" as const,
      requirementId: "req_21",
      repositoryTargetId: "repo_test",
      acceptanceCriteria: ["展示用户姓名", "展示账号状态"],
      blockedByItemIds: []
    };

    const { container } = render(
      <WorkItemDetailPage
        {...helpers}
        workItem={workItem}
        workItems={[workItem]}
        requirements={[{
          id: "req_21",
          projectId: "project_omega",
          repositoryTargetId: "repo_test",
          source: "manual",
          title: "添加用户详情页面",
          rawText: "# 添加用户详情页面\n\n".repeat(30),
          acceptanceCriteria: ["展示用户姓名"],
          risks: [],
          status: "converted",
          createdAt: "2026-04-29T00:00:00Z",
          updatedAt: "2026-04-29T00:00:00Z"
        }]}
        repositoryTargets={[{ id: "repo_test", kind: "github", owner: "ZYOOO", repo: "TestRepo", defaultBranch: "main" }]}
        repositoryLabel="ZYOOO/TestRepo"
        runWorkpads={[]}
        pipeline={{
          id: "pipeline_21",
          workItemId: "item_manual_21",
          runId: "run_1",
          status: "running",
          run: {
            stages: [
              { id: "implementation", title: "Implementation and PR", status: "running", agentIds: ["coding", "testing"] },
              { id: "human_review", title: "Human Review", status: "waiting-human", agentIds: ["review"] }
            ]
          }
        }}
        attempts={[{
          id: "attempt_21",
          itemId: "item_manual_21",
          pipelineId: "pipeline_21",
          status: "waiting-human",
          currentStageId: "human_review",
          branchName: "omega/OMG-21-devflow",
          pullRequestUrl: "https://github.com/ZYOOO/TestRepo/pull/21"
        }]}
        checkpoints={[]}
        operations={[{
          id: "pipeline_21:agent:implementation:coding",
          missionId: "mission_pipeline_21",
          stageId: "implementation",
          agentId: "coding",
          status: "passed",
          prompt: "Implement user detail page.",
          summary: "Coding agent produced changed files.",
          runnerProcess: { runner: "codex", status: "passed", stdout: "ok" }
        }]}
        proofRecords={[{
          id: "proof_1",
          operationId: "pipeline_21:agent:implementation:coding",
          label: "implementation-summary",
          value: "implementation-summary.md",
          sourcePath: "/tmp/implementation-summary.md"
        }]}
        attemptTimeline={null}
        pullRequestStatus={null}
        onApproveCheckpoint={vi.fn()}
        onRequestCheckpointChanges={vi.fn()}
        onRetryAttempt={vi.fn()}
      />
    );

    expect(screen.getByLabelText("Run workpad")).toBeInTheDocument();
    expect(container.querySelector(".requirement-source-scroll")).toBeTruthy();
    expect(container.querySelector(".detail-stage-grid .stage-running")).toBeTruthy();
    expect(screen.getByText("Agent operations")).toBeInTheDocument();
    expect(screen.getByText("Prompt")).toBeInTheDocument();
    expect(screen.getByText("/tmp/implementation-summary.md")).toBeInTheDocument();
  });

  it("round-trips work item detail hash routes", () => {
    expect(workItemDetailHash("item/manual 21")).toBe("#/work-items/item%2Fmanual%2021");
    expect(parseWorkItemDetailHash("#/work-items/item%2Fmanual%2021")).toBe("item/manual 21");
  });
});
