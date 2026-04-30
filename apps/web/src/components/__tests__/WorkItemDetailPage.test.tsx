import { cleanup, fireEvent, render, screen } from "@testing-library/react";
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
        runWorkpads={[{
          id: "attempt_21:workpad",
          attemptId: "attempt_21",
          pipelineId: "pipeline_21",
          workItemId: "item_manual_21",
          repositoryTargetId: "repo_test",
          status: "waiting-human",
          workpad: {
            reworkChecklist: {
              status: "needs-rework",
              retryReason: "Review agent requested a clearer loading state.",
              checklist: ["处理 Review Agent 指出的阻塞问题：Add loading feedback before merge."],
              sources: [
                { kind: "review", label: "Review feedback", message: "Add loading feedback before merge." },
                { kind: "ci-check-log", label: "lint", message: "Expected visible loading text before approval.", url: "https://github.com/ZYOOO/TestRepo/actions/runs/21" }
              ]
            }
          },
          fieldPatchHistory: [{
            id: "attempt_21:workpad:patch:1",
            updatedAt: "2026-04-29T14:40:00Z",
            updatedBy: "job-supervisor",
            fields: ["blockers", "reviewFeedback"],
            reason: "Required check changed state.",
            source: { kind: "ci-check", label: "lint" }
          }]
        }]}
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
    expect(screen.getByText("Rework checklist")).toBeInTheDocument();
    expect(screen.getByText("1 action")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /Rework checklist/i }));
    expect(screen.getByLabelText("Checklist sources")).toBeInTheDocument();
    expect(screen.getByText("Check log")).toBeInTheDocument();
    expect(screen.getByText("Expected visible loading text before approval.")).toBeInTheDocument();
    expect(screen.getByText("Open source")).toHaveAttribute("href", "https://github.com/ZYOOO/TestRepo/actions/runs/21");
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    fireEvent.click(screen.getByRole("button", { name: /Patch history/i }));
    expect(screen.getByText("Required check changed state.")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    expect(container.querySelector(".requirement-source-scroll")).toBeTruthy();
    expect(container.querySelector(".detail-stage-grid .stage-running")).toBeTruthy();
    expect(screen.getByText("Agent operations")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /implementation.*coding/i }));
    expect(screen.getByText("Prompt")).toBeInTheDocument();
    expect(screen.getByText("/tmp/implementation-summary.md")).toBeInTheDocument();
  });

  it("round-trips work item detail hash routes", () => {
    expect(workItemDetailHash("item/manual 21")).toBe("#/work-items/item%2Fmanual%2021");
    expect(parseWorkItemDetailHash("#/work-items/item%2Fmanual%2021")).toBe("item/manual 21");
  });
});
