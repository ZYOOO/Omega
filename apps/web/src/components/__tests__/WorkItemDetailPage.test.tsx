import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
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
      sourceExternalRef: "item_manual_21",
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
          sourceExternalRef: "req_item_manual_21",
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
            },
            reviewPacket: {
              summary: "OMG-21 has 2 changed files and needs validation attention.",
              diffPreview: {
                summary: "2 changed file(s), +24/-4 lines.",
                fileCount: 2,
                changedFiles: ["src/UserDetail.tsx", "src/UserDetail.test.tsx"],
                patchExcerpt: "diff --git a/src/UserDetail.tsx b/src/UserDetail.tsx"
              },
              testPreview: { status: "attention", summary: "Lint failed." },
              checkPreview: { status: "missing", summary: "No remote checks captured." },
              risk: { level: "high", reasons: ["Validation output needs attention."] },
              recommendedActions: [{ type: "validation", label: "Run focused validation before approval." }]
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
          status: "waiting-human",
          run: {
            stages: [
              { id: "implementation", title: "Implementation and PR", status: "passed", agentIds: ["coding", "testing"] },
              { id: "human_review", title: "Human Review", status: "needs-human", agentIds: ["review"] }
            ],
            events: [{ type: "checkpoint.rejected", message: "Add loading feedback before merge." }]
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
        onOpenPagePilot={vi.fn()}
        onApproveCheckpoint={vi.fn()}
        onRequestCheckpointChanges={vi.fn()}
        onRetryAttempt={vi.fn()}
      />
    );

    expect(screen.getByLabelText("Run workpad")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Open in Page Pilot" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Collapse" }));
    expect(screen.queryByRole("button", { name: /Review packet/i })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Expand" }));
    expect(screen.queryByText("item_manual_21")).not.toBeInTheDocument();
    expect(screen.queryByText("req_item_manual_21")).not.toBeInTheDocument();
    expect(screen.getByText("Rework checklist")).toBeInTheDocument();
    expect(screen.getByText("Review packet")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /Review packet/i }));
    expect(screen.getByLabelText("Review packet preview")).toBeInTheDocument();
    expect(screen.getByText("Run focused validation before approval.")).toBeInTheDocument();
    expect(screen.getByText("src/UserDetail.tsx")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
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
    expect(container.querySelectorAll(".detail-stage-grid .stage-needs-human")).toHaveLength(1);
    expect(container.querySelectorAll(".detail-stage-grid .stage-running")).toHaveLength(0);
    expect(screen.getByText("Agent operations")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /implementation.*coding/i }));
    expect(screen.getByText("Prompt")).toBeInTheDocument();
    expect(screen.getByText("/tmp/implementation-summary.md")).toBeInTheDocument();
  });

  it("does not show a rework route just because the workflow contains a rework stage", () => {
    const workItem: WorkItem = {
      id: "item_manual_28",
      key: "OMG-28",
      title: "新增页面",
      description: "Need a new page.",
      status: "In Review" as const,
      priority: "Medium" as const,
      assignee: "coding",
      labels: [],
      team: "Omega",
      stageId: "review",
      target: "ZYOOO/TestRepo",
      source: "manual" as const,
      repositoryTargetId: "repo_test",
      acceptanceCriteria: ["页面可人工验收"],
      blockedByItemIds: []
    };

    render(
      <WorkItemDetailPage
        {...helpers}
        workItem={workItem}
        workItems={[workItem]}
        requirements={[]}
        repositoryTargets={[{ id: "repo_test", kind: "github", owner: "ZYOOO", repo: "TestRepo", defaultBranch: "main" }]}
        repositoryLabel="ZYOOO/TestRepo"
        runWorkpads={[{
          id: "attempt_28:workpad",
          attemptId: "attempt_28",
          pipelineId: "pipeline_28",
          workItemId: "item_manual_28",
          repositoryTargetId: "repo_test",
          status: "waiting-human",
          workpad: {
            blockers: ["Human Review 审批"],
            reworkChecklist: {
              checklist: ["历史运行记录里捕获过一条返工建议。"],
              retryReason: "Agent stage artifact recorded."
            },
            retryReason: "Agent stage artifact recorded."
          }
        }]}
        pipeline={{
          id: "pipeline_28",
          workItemId: "item_manual_28",
          runId: "run_28",
          status: "waiting-human",
          run: {
            stages: [
              { id: "implementation", title: "Implementation and PR", status: "done", agentIds: ["coding", "testing"] },
              { id: "rework", title: "Rework", status: "done", agentIds: ["coding", "testing"] },
              { id: "human_review", title: "Human Review", status: "waiting-human", agentIds: ["review"] }
            ]
          }
        }}
        attempts={[{
          id: "attempt_28",
          itemId: "item_manual_28",
          pipelineId: "pipeline_28",
          status: "waiting-human",
          currentStageId: "human_review",
          pullRequestUrl: "https://github.com/ZYOOO/TestRepo/pull/37"
        }]}
        checkpoints={[{
          id: "pipeline_28:human_review",
          pipelineId: "pipeline_28",
          attemptId: "attempt_28",
          stageId: "human_review",
          status: "pending",
          title: "Human Review 审批",
          summary: "Human Review 需要人工确认后才能继续"
        }]}
        operations={[]}
        proofRecords={[]}
        attemptTimeline={null}
        pullRequestStatus={null}
        onOpenPagePilot={vi.fn()}
        onApproveCheckpoint={vi.fn()}
        onRequestCheckpointChanges={vi.fn()}
        onRetryAttempt={vi.fn()}
      />
    );

    expect(screen.queryByText("Feedback route")).not.toBeInTheDocument();
    expect(screen.queryByText("Rework checklist")).not.toBeInTheDocument();
    expect(screen.getByText("No active blockers")).toBeInTheDocument();
    expect(screen.getByText("No retry needed")).toBeInTheDocument();
  });

  it("renders delivery flow from the canonical backend pipeline snapshot instead of action plan states", () => {
    const workItem: WorkItem = {
      id: "item_manual_32",
      key: "OMG-32",
      title: "新增加一个静夜思在md文档中",
      description: "Need a markdown update.",
      status: "Human Review" as const,
      priority: "High" as const,
      assignee: "coding",
      labels: [],
      team: "Omega",
      stageId: "review",
      target: "ZYOOO/TestRepo",
      source: "manual" as const,
      repositoryTargetId: "repo_test",
      acceptanceCriteria: [],
      blockedByItemIds: []
    };

    const { container } = render(
      <WorkItemDetailPage
        {...helpers}
        workItem={workItem}
        workItems={[workItem]}
        requirements={[]}
        repositoryTargets={[{ id: "repo_test", kind: "github", owner: "ZYOOO", repo: "TestRepo", defaultBranch: "main" }]}
        repositoryLabel="ZYOOO/TestRepo"
        runWorkpads={[]}
        pipeline={{
          id: "pipeline_32",
          workItemId: "item_manual_32",
          runId: "run_32",
          status: "waiting-human",
          run: {
            stages: [
              { id: "todo", title: "Todo intake", status: "passed", agentIds: ["requirement"] },
              { id: "in_progress", title: "Implementation and PR", status: "passed", agentIds: ["architect", "coding", "testing"] },
              { id: "code_review_round_1", title: "Code Review Round 1", status: "passed", agentIds: ["review"] },
              { id: "code_review_round_2", title: "Code Review Round 2", status: "passed", agentIds: ["review"] },
              { id: "rework", title: "Rework", status: "waiting", agentIds: ["coding", "testing"] },
              { id: "human_review", title: "Human Review", status: "needs-human", agentIds: ["human"] },
              { id: "merging", title: "Merging", status: "waiting", agentIds: ["delivery"] },
              { id: "done", title: "Done", status: "waiting", agentIds: ["delivery"] }
            ]
          }
        }}
        attemptActionPlan={{
          attemptId: "attempt_32",
          pipelineId: "pipeline_32",
          states: [
            { id: "todo", title: "Todo intake", status: "passed" },
            { id: "in_progress", title: "Implementation and PR", status: "running" },
            { id: "human_review", title: "Human Review", status: "needs-human" },
            { id: "done", title: "Done", status: "needs-human" }
          ]
        }}
        attempts={[{
          id: "attempt_32",
          itemId: "item_manual_32",
          pipelineId: "pipeline_32",
          status: "waiting-human",
          currentStageId: "human_review"
        }]}
        checkpoints={[{
          id: "pipeline_32:human_review",
          pipelineId: "pipeline_32",
          attemptId: "attempt_32",
          stageId: "human_review",
          status: "pending",
          title: "Human review",
          summary: "Waiting for approval."
        }]}
        operations={[]}
        proofRecords={[]}
        attemptTimeline={null}
        pullRequestStatus={null}
        onOpenPagePilot={vi.fn()}
        onApproveCheckpoint={vi.fn()}
        onRequestCheckpointChanges={vi.fn()}
        onRetryAttempt={vi.fn()}
      />
    );

    expect(container.querySelectorAll(".detail-stage-grid .stage-needs-human")).toHaveLength(1);
    expect(container.querySelectorAll(".detail-stage-grid .stage-running")).toHaveLength(0);
  });

  it("hides human approval actions after the current human review checkpoint is approved", () => {
    const workItem: WorkItem = {
      id: "item_manual_30",
      key: "OMG-30",
      title: "新增客户健康度工作台页面",
      description: "Need final approval.",
      status: "Done" as const,
      priority: "High" as const,
      assignee: "coding",
      labels: [],
      team: "Omega",
      stageId: "delivery",
      target: "ZYOOO/TestRepo",
      source: "manual" as const,
      repositoryTargetId: "repo_test",
      acceptanceCriteria: [],
      blockedByItemIds: []
    };

    render(
      <WorkItemDetailPage
        {...helpers}
        workItem={workItem}
        workItems={[workItem]}
        requirements={[]}
        repositoryTargets={[{ id: "repo_test", kind: "github", owner: "ZYOOO", repo: "TestRepo", defaultBranch: "main" }]}
        repositoryLabel="ZYOOO/TestRepo"
        runWorkpads={[]}
        pipeline={{
          id: "pipeline_30",
          workItemId: "item_manual_30",
          runId: "run_30",
          status: "done",
          run: {
            stages: [
              { id: "human_review", title: "Human Review", status: "passed", agentIds: ["human"] },
              { id: "done", title: "Done", status: "passed", agentIds: ["delivery"] }
            ],
            events: [
              { type: "gate.approved", message: "Human review approved by human." },
              { type: "devflow.cycle.completed", message: "DevFlow PR cycle completed after human approval." }
            ]
          }
        }}
        attempts={[{
          id: "attempt_30",
          itemId: "item_manual_30",
          pipelineId: "pipeline_30",
          status: "done",
          currentStageId: "done",
          pullRequestUrl: "https://github.com/ZYOOO/TestRepo/pull/40"
        }]}
        checkpoints={[{
          id: "pipeline_30:human_review",
          pipelineId: "pipeline_30",
          attemptId: "attempt_30",
          stageId: "human_review",
          status: "approved",
          title: "Human Review",
          summary: "Human Review is approved.",
          decisionNote: "approved by human",
          updatedAt: "2026-05-04T01:00:00Z"
        }]}
        operations={[]}
        proofRecords={[]}
        attemptTimeline={null}
        pullRequestStatus={null}
        onOpenPagePilot={vi.fn()}
        onApproveCheckpoint={vi.fn()}
        onRequestCheckpointChanges={vi.fn()}
        onRetryAttempt={vi.fn()}
      />
    );

    expect(screen.queryByRole("button", { name: "Approve delivery" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Request changes" })).not.toBeInTheDocument();
    expect(screen.getByText("Human review approved")).toBeInTheDocument();
    expect(screen.getByText("approved by human")).toBeInTheDocument();
  });

  it("opens proof artifact previews from the artifact grid", async () => {
    const workItem: WorkItem = {
      id: "item_manual_preview",
      key: "OMG-preview",
      title: "查看审核产物",
      description: "Need artifact preview.",
      status: "In Review" as const,
      priority: "Medium" as const,
      assignee: "review",
      labels: [],
      team: "Omega",
      stageId: "review",
      target: "ZYOOO/TestRepo",
      source: "manual" as const,
      repositoryTargetId: "repo_test",
      acceptanceCriteria: [],
      blockedByItemIds: []
    };
    const onFetchProofPreview = vi.fn().mockResolvedValue({
      available: true,
      proof: { id: "proof_preview", label: "code-review-round-1" },
      sourcePath: "/tmp/proof/code-review-round-1.md",
      previewType: "markdown",
      content: "# Review\n\nNo blocking findings."
    });

    render(
      <WorkItemDetailPage
        {...helpers}
        workItem={workItem}
        workItems={[workItem]}
        requirements={[]}
        repositoryTargets={[{ id: "repo_test", kind: "github", owner: "ZYOOO", repo: "TestRepo", defaultBranch: "main" }]}
        repositoryLabel="ZYOOO/TestRepo"
        runWorkpads={[]}
        pipeline={{
          id: "pipeline_preview",
          workItemId: "item_manual_preview",
          runId: "run_preview",
          status: "waiting-human",
          run: {
            stages: [{
              id: "human_review",
              title: "Human Review",
              status: "waiting-human",
              agentIds: ["review"],
              outputArtifacts: ["/tmp/proof/code-review-round-1.md", "rollback-plan"]
            }]
          }
        }}
        attempts={[]}
        checkpoints={[]}
        operations={[{
          id: "pipeline_preview:agent:human_review:review",
          stageId: "human_review",
          agentId: "review",
          status: "passed",
          prompt: "Review",
          summary: "Review complete."
        }]}
        proofRecords={[{
          id: "proof_preview",
          operationId: "pipeline_preview:agent:human_review:review",
          label: "code-review-round-1",
          value: "code-review-round-1.md",
          sourcePath: "/tmp/proof/code-review-round-1.md"
        }]}
        attemptTimeline={null}
        pullRequestStatus={null}
        onOpenPagePilot={vi.fn()}
        onApproveCheckpoint={vi.fn()}
        onFetchProofPreview={onFetchProofPreview}
        onRequestCheckpointChanges={vi.fn()}
        onRetryAttempt={vi.fn()}
      />
    );

    fireEvent.click(screen.getByRole("button", { name: /code-review-round-1\.md/i }));

    await waitFor(() => expect(onFetchProofPreview).toHaveBeenCalledWith("proof_preview"));
    expect(screen.getByRole("dialog", { name: /code-review-round-1\.md preview/i })).toBeInTheDocument();
    expect(screen.getByText(/No blocking findings/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /rollback-plan/i })).not.toBeInTheDocument();
  });

  it("round-trips work item detail hash routes", () => {
    expect(workItemDetailHash("item/manual 21")).toBe("#/work-items/item%2Fmanual%2021");
    expect(parseWorkItemDetailHash("#/work-items/item%2Fmanual%2021")).toBe("item/manual 21");
  });

  it("submits operator field patches from the Run Workpad editor", async () => {
    const workItem: WorkItem = {
      id: "item_manual_22",
      key: "OMG-22",
      title: "补充验证入口",
      description: "Need validation notes.",
      status: "In Review" as const,
      priority: "Medium" as const,
      assignee: "coding",
      labels: [],
      team: "Omega",
      stageId: "review",
      target: "ZYOOO/TestRepo",
      source: "manual" as const,
      repositoryTargetId: "repo_test",
      acceptanceCriteria: [],
      blockedByItemIds: []
    };
    const onPatchRunWorkpad = vi.fn().mockResolvedValue(undefined);

    render(
      <WorkItemDetailPage
        {...helpers}
        workItem={workItem}
        workItems={[workItem]}
        requirements={[]}
        repositoryTargets={[{ id: "repo_test", kind: "github", owner: "ZYOOO", repo: "TestRepo", defaultBranch: "main" }]}
        repositoryLabel="ZYOOO/TestRepo"
        runWorkpads={[{
          id: "attempt_22:workpad",
          attemptId: "attempt_22",
          pipelineId: "pipeline_22",
          workItemId: "item_manual_22",
          repositoryTargetId: "repo_test",
          status: "running",
          workpad: {
            blockers: ["旧 blocker"],
            notes: ["初始 note"]
          },
          updatedAt: "2026-04-30T12:00:00Z"
        }]}
        pipeline={undefined}
        attempts={[]}
        checkpoints={[]}
        operations={[]}
        proofRecords={[]}
        attemptTimeline={null}
        pullRequestStatus={null}
        onOpenPagePilot={vi.fn()}
        onApproveCheckpoint={vi.fn()}
        onPatchRunWorkpad={onPatchRunWorkpad}
        onRequestCheckpointChanges={vi.fn()}
        onRetryAttempt={vi.fn()}
      />
    );

    fireEvent.click(screen.getByRole("button", { name: "Edit fields" }));
    fireEvent.change(screen.getByLabelText("Field"), { target: { value: "blockers" } });
    fireEvent.change(screen.getByLabelText("Patch value"), {
      target: { value: "需要重新跑 smoke\n等待人工确认" }
    });
    fireEvent.change(screen.getByLabelText("Reason"), { target: { value: "人工复核发现验证缺口" } });
    fireEvent.click(screen.getByRole("button", { name: "Save patch" }));

    await waitFor(() => expect(onPatchRunWorkpad).toHaveBeenCalledTimes(1));
    expect(onPatchRunWorkpad).toHaveBeenCalledWith("attempt_22:workpad", {
      workpad: { blockers: ["需要重新跑 smoke", "等待人工确认"] },
      updatedBy: "operator",
      reason: "人工复核发现验证缺口",
      source: {
        kind: "ui",
        label: "Run Workpad editor",
        field: "blockers",
        attemptId: "attempt_22",
        workItemId: "item_manual_22"
      }
    });
  });
});
