import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ObservabilityDashboard } from "../ObservabilityDashboard";

describe("ObservabilityDashboard", () => {
  afterEach(() => cleanup());

  it("renders delivery movement with explicit trend meaning", () => {
    const onRefresh = vi.fn();
    render(
      <ObservabilityDashboard
        windowDays={14}
        groupBy="stage"
        onWindowDaysChange={vi.fn()}
        onGroupByChange={vi.fn()}
        onRefresh={onRefresh}
        observability={{
          counts: { workItems: 7, pipelines: 7, checkpoints: 0, missions: 0, operations: 20, proofRecords: 226, events: 0 },
          pipelineStatus: {},
          checkpointStatus: {},
          operationStatus: {},
          workItemStatus: {},
          attention: { waitingHuman: 2, failed: 2, blocked: 0 },
          dashboard: {
            groupBy: "stage",
            groups: [{ key: "in_progress", label: "Implementation", total: 6, done: 4, failed: 2, waiting: 0 }],
            recentFailures: [{ attemptId: "attempt_failed", stageId: "in_progress", message: "runner failed", createdAt: "2026-05-05T15:22:00Z" }],
            slowStageDrilldown: [{ stageId: "human_review", averageDurationMs: 42000, count: 2 }],
            trends: [
              { date: "2026-05-05", attemptsStarted: 2, attemptsCompleted: 1, attemptsFailed: 1, pullRequestsMerged: 0, checkpointsResolved: 1 },
              { date: "2026-05-06", attemptsStarted: 1, attemptsCompleted: 1, attemptsFailed: 0, pullRequestsMerged: 1, checkpointsResolved: 0 }
            ]
          }
        }}
      />
    );

    expect(screen.getByText("Delivery movement")).toBeInTheDocument();
    expect(screen.getByText("Signals from recent attempts, checkpoints, PR proof, and runner operations.")).toBeInTheDocument();
    expect(screen.getByLabelText("Delivery movement totals")).toHaveTextContent("3");
    expect(screen.getByLabelText("Daily delivery movement")).toBeInTheDocument();
    expect(screen.getByLabelText("Trend legend")).toHaveTextContent("Height = daily movement");
    fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
    expect(onRefresh).toHaveBeenCalledTimes(1);
  });
});
