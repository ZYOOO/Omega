import type { ReactNode } from "react";
import type { ObservabilitySummary } from "../omegaControlApiClient";

interface ObservabilityDashboardProps {
  observability: ObservabilitySummary;
  windowDays: number;
  groupBy: string;
  onWindowDaysChange: (value: number) => void;
  onGroupByChange: (value: string) => void;
  onRefresh: () => void;
}

type TrendTotals = {
  activity: number;
  completed: number;
  failed: number;
  prMerged: number;
  started: number;
};

export function ObservabilityDashboard({
  groupBy,
  observability,
  onGroupByChange,
  onRefresh,
  onWindowDaysChange,
  windowDays
}: ObservabilityDashboardProps) {
  const dashboard = observability.dashboard ?? {};
  const groups = dashboard.groups ?? [];
  const recentFailures = dashboard.recentFailures ?? [];
  const slowStageDrilldown = dashboard.slowStageDrilldown ?? [];
  const trends = dashboard.trends ?? [];
  const trendTotals = summarizeTrends(trends);
  const hasTrendSignal = trendTotals.activity > 0;

  return (
    <section className="observability-dashboard">
      <header className="observability-dashboard-header">
        <div>
          <span className="section-label">Observability</span>
          <h2>Runtime health</h2>
          <p>Signals from recent attempts, checkpoints, PR proof, and runner operations.</p>
        </div>
        <div className="observability-controls">
          <label>
            <span>Window</span>
            <select value={windowDays} onChange={(event) => onWindowDaysChange(Number(event.currentTarget.value))}>
              <option value={1}>24h</option>
              <option value={7}>7d</option>
              <option value={14}>14d</option>
              <option value={30}>30d</option>
            </select>
          </label>
          <label>
            <span>Group</span>
            <select value={groupBy} onChange={(event) => onGroupByChange(event.currentTarget.value)}>
              <option value="stage">Stage</option>
              <option value="runner">Runner</option>
              <option value="repository">Repository</option>
              <option value="status">Status</option>
            </select>
          </label>
          <button type="button" onClick={onRefresh}>Refresh</button>
        </div>
      </header>

      <div className="observability-grid">
        <DashboardPanel title={`Grouped by ${dashboard.groupBy ?? groupBy}`} count={groups.length}>
          {groups.length ? (
            <div className="observability-bars">
              {groups.slice(0, 6).map((group, index) => (
                <article key={`${recordText(group, "key") || recordText(group, "label") || "group"}-${index}`}>
                  <span>
                    <strong>{recordText(group, "label") || recordText(group, "key") || "Unknown"}</strong>
                    <small>{recordText(group, "failed")} failed · {recordText(group, "waiting")} waiting</small>
                  </span>
                  <meter min={0} max={Math.max(1, recordNumber(group, "total"))} value={recordNumber(group, "done")} />
                </article>
              ))}
            </div>
          ) : (
            <p>No grouped data in this window.</p>
          )}
        </DashboardPanel>

        <DashboardPanel title="Recent failures" count={recentFailures.length} tone={recentFailures.length ? "danger" : "ready"}>
          {recentFailures.length ? (
            <div className="observability-list">
              {recentFailures.slice(0, 5).map((failure, index) => (
                <article key={`${recordText(failure, "attemptId")}-${index}`}>
                  <strong>{recordText(failure, "stageId") || recordText(failure, "eventType") || "Failure"}</strong>
                  <span>{recordText(failure, "message") || recordText(failure, "reason") || "No reason captured"}</span>
                  <small>{formatTime(recordText(failure, "createdAt") || recordText(failure, "updatedAt"))}</small>
                </article>
              ))}
            </div>
          ) : (
            <p>No recent failures.</p>
          )}
        </DashboardPanel>

        <DashboardPanel title="Slow stage drilldown" count={slowStageDrilldown.length}>
          {slowStageDrilldown.length ? (
            <div className="observability-list">
              {slowStageDrilldown.slice(0, 5).map((stage, index) => (
                <article key={`${recordText(stage, "stageId")}-${index}`}>
                  <strong>{recordText(stage, "stageId") || "stage"}</strong>
                  <span>{formatDuration(recordNumber(stage, "averageDurationMs"))} avg · {recordText(stage, "count")} run(s)</span>
                  <small>{recordText(stage, "runner") || recordText(stage, "repository")}</small>
                </article>
              ))}
            </div>
          ) : (
            <p>No slow-stage signal yet.</p>
          )}
        </DashboardPanel>

        <DashboardPanel title="Delivery movement" count={trendTotals.activity}>
          {hasTrendSignal ? (
            <div className="observability-trend-panel">
              <div className="observability-trend-summary" aria-label="Delivery movement totals">
                <span><strong>{trendTotals.started}</strong> started</span>
                <span><strong>{trendTotals.completed}</strong> completed</span>
                <span><strong>{trendTotals.failed}</strong> failed</span>
                <span><strong>{trendTotals.prMerged}</strong> PR merged</span>
              </div>
              <div className="observability-trend" aria-label="Daily delivery movement">
                {trends.slice(-10).map((trend, index) => {
                  const date = recordText(trend, "date") || recordText(trend, "day");
                  const started = recordNumber(trend, "attemptsStarted");
                  const completed = recordNumber(trend, "attemptsCompleted");
                  const failed = recordNumber(trend, "attemptsFailed");
                  const prMerged = recordNumber(trend, "pullRequestsMerged");
                  const total = started + completed + failed + prMerged + recordNumber(trend, "checkpointsResolved");
                  const height = Math.max(10, Math.min(72, total * 10));
                  return (
                    <span
                      key={`${date || "trend"}-${index}`}
                      className={failed ? "has-failure" : ""}
                      title={`${date}: ${started} started, ${completed} completed, ${failed} failed, ${prMerged} PR merged`}
                    >
                      <i style={{ height: `${height}px` }} />
                      <small>{shortDate(date)}</small>
                    </span>
                  );
                })}
              </div>
              <div className="observability-trend-legend" aria-label="Trend legend">
                <span>Height = daily movement</span>
                <span>Red = failures present</span>
              </div>
            </div>
          ) : (
            <p>No movement trend in this window yet.</p>
          )}
        </DashboardPanel>
      </div>
    </section>
  );
}

function DashboardPanel({
  children,
  count,
  title,
  tone = "neutral"
}: {
  children: ReactNode;
  count: number;
  title: string;
  tone?: "neutral" | "danger" | "ready";
}) {
  return (
    <article className={`observability-panel ${tone}`}>
      <header>
        <span>{title}</span>
        <strong>{count}</strong>
      </header>
      {children}
    </article>
  );
}

function recordText(record: Record<string, unknown>, key: string): string {
  const value = record[key];
  return typeof value === "string" ? value : typeof value === "number" ? String(value) : "";
}

function recordNumber(record: Record<string, unknown>, key: string): number {
  const value = record[key];
  return typeof value === "number" ? value : typeof value === "string" ? Number(value) || 0 : 0;
}

function summarizeTrends(trends: Record<string, unknown>[]): TrendTotals {
  return trends.reduce<TrendTotals>(
    (totals, trend) => {
      const started = recordNumber(trend, "attemptsStarted");
      const completed = recordNumber(trend, "attemptsCompleted");
      const failed = recordNumber(trend, "attemptsFailed");
      const prMerged = recordNumber(trend, "pullRequestsMerged");
      const checkpointsResolved = recordNumber(trend, "checkpointsResolved");
      totals.started += started;
      totals.completed += completed;
      totals.failed += failed;
      totals.prMerged += prMerged;
      totals.activity += started + completed + failed + prMerged + checkpointsResolved;
      return totals;
    },
    { activity: 0, completed: 0, failed: 0, prMerged: 0, started: 0 }
  );
}

function formatDuration(value: number): string {
  if (!value) return "0ms";
  if (value < 1000) return `${Math.round(value)}ms`;
  return `${(value / 1000).toFixed(1)}s`;
}

function shortDate(value: string): string {
  if (!value) return "";
  const date = new Date(`${value}T00:00:00Z`);
  if (Number.isNaN(date.getTime())) return value.slice(5) || value;
  return new Intl.DateTimeFormat("zh-CN", {
    timeZone: "Asia/Shanghai",
    month: "2-digit",
    day: "2-digit"
  }).format(date);
}

function formatTime(value: string): string {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat("zh-CN", {
    timeZone: "Asia/Shanghai",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false
  }).format(date);
}
