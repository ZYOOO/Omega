import type { ReactNode } from "react";
import { useMemo } from "react";
import { retryReasonForAttempt } from "../attemptRetryReason";
import type { RepositoryTarget, WorkItem, WorkItemStatus } from "../core";
import type {
  AttemptRecordInfo,
  AttemptTimelineInfo,
  CheckpointRecordInfo,
  GitHubPullRequestStatusResult,
  OperationRecordInfo,
  PipelineRecordInfo,
  ProofRecordInfo,
  RequirementRecordInfo,
  RunWorkpadRecordInfo
} from "../omegaControlApiClient";
import {
  AgentTraceList,
  ArtifactGrid,
  AttemptHistory,
  type DetailProofCard,
  type DetailReviewEventCard,
  WorkItemAttemptPanel
} from "./WorkItemDetailPanels";

type StageSummary = {
  id: string;
  title?: string;
  status: string;
};

type WorkpadSection = {
  id: string;
  label: string;
  title: string;
  body: ReactNode;
  tone?: "default" | "warning" | "success";
};

type DetailHelpers = {
  agentShortLabel: (agentId: string) => string;
  attemptStatusLabel: (status: string) => string;
  operationStatusLabel: (status: string) => string;
  pipelineStageClassName: (status: string) => string;
  pipelineStageLabel: (status: string) => string;
  sourceLabel: (item: WorkItem) => string;
  statusClassName: (status: WorkItemStatus) => string;
  workItemStatusLabel: (status: WorkItemStatus) => string;
};

export interface WorkItemDetailPageProps extends DetailHelpers {
  attemptTimeline?: AttemptTimelineInfo | null;
  attempts: AttemptRecordInfo[];
  checkpoints: CheckpointRecordInfo[];
  operations: OperationRecordInfo[];
  pipeline?: PipelineRecordInfo;
  proofRecords: ProofRecordInfo[];
  pullRequestStatus?: GitHubPullRequestStatusResult | null;
  repositoryLabel: string;
  repositoryTargets: RepositoryTarget[];
  requirements: RequirementRecordInfo[];
  runWorkpads: RunWorkpadRecordInfo[];
  workItem: WorkItem;
  workItems: WorkItem[];
  onApproveCheckpoint: (checkpointId: string) => void;
  onRequestCheckpointChanges: (checkpointId: string, note?: string) => void;
  onRetryAttempt: (attemptId: string) => void;
}

export function WorkItemDetailPage({
  agentShortLabel,
  attemptStatusLabel,
  attemptTimeline,
  attempts,
  checkpoints,
  operationStatusLabel,
  operations,
  pipeline,
  pipelineStageClassName,
  pipelineStageLabel,
  proofRecords,
  pullRequestStatus,
  repositoryLabel,
  repositoryTargets,
  requirements,
  runWorkpads,
  sourceLabel,
  statusClassName,
  workItem,
  workItemStatusLabel,
  workItems,
  onApproveCheckpoint,
  onRequestCheckpointChanges,
  onRetryAttempt
}: WorkItemDetailPageProps) {
  const requirement = workItem.requirementId
    ? requirements.find((candidate) => candidate.id === workItem.requirementId)
    : undefined;
  const siblingItems = requirement ? workItems.filter((item) => item.requirementId === requirement.id) : [];
  const attempt = attempts[0];
  const runWorkpad = useMemo(
    () => latestRunWorkpadForDetail({ attempt, pipeline, runWorkpads, workItem }),
    [attempt, pipeline, runWorkpads, workItem]
  );
  const checkpoint = pipeline
    ? checkpoints.find((candidate) => candidate.pipelineId === pipeline.id && candidate.status === "pending")
    : undefined;
  const detailOperations = useMemo(
    () => operationsForWorkItem({ attempt, operations, pipeline, workItem }),
    [attempt, operations, pipeline, workItem]
  );
  const proofCards = useMemo(
    () => proofCardsForWorkItem({ attempt, operations: detailOperations, pipeline, proofRecords, workItem }),
    [attempt, detailOperations, pipeline, proofRecords, workItem]
  );
  const failedStages = useMemo(() => {
    const stages = attempt?.stages?.length ? attempt.stages : pipeline?.run?.stages ?? [];
    return stages.filter((stage) => stage.status === "failed" || stage.status === "blocked");
  }, [attempt, pipeline]);
  const failureOperations = useMemo(() => {
    const failureStageIds = new Set(failedStages.map((stage) => stage.id));
    if (attempt?.failureStageId) failureStageIds.add(attempt.failureStageId);
    if (!failureStageIds.size && attempt?.currentStageId) failureStageIds.add(attempt.currentStageId);
    return detailOperations.filter((operation) => {
      const failed = operation.status === "failed" || operation.runnerProcess?.status === "failed";
      return failed && (!failureStageIds.size || (operation.stageId ? failureStageIds.has(operation.stageId) : true));
    });
  }, [attempt, detailOperations, failedStages]);
  const failureProofCards = useMemo(() => {
    if (!failedStages.length) return proofCards.filter((proof) => proof.kind === "Review").slice(0, 3);
    const failedStageNames = new Set(failedStages.flatMap((stage) => [stage.id, stage.title].filter(Boolean) as string[]));
    return proofCards
      .filter((proof) => proof.kind === "Review" || (proof.stage ? failedStageNames.has(proof.stage) : false))
      .slice(0, 4);
  }, [failedStages, proofCards]);
  const humanReviewArtifacts = useMemo(() => {
    const preferred = /human-review|code-review|review|git-diff|diff|test-report|implementation-summary|rework-summary|solution-plan|changed-files/i;
    return proofCards
      .filter((proof) => preferred.test(`${proof.label} ${proof.path ?? ""} ${proof.stage ?? ""} ${proof.kind}`))
      .sort((left, right) => {
        const leftReview = /review|human/i.test(`${left.label} ${left.kind} ${left.stage ?? ""}`) ? 0 : 1;
        const rightReview = /review|human/i.test(`${right.label} ${right.kind} ${right.stage ?? ""}`) ? 0 : 1;
        return leftReview - rightReview;
      })
      .slice(0, 8);
  }, [proofCards]);
  const reviewEvents = useMemo<DetailReviewEventCard[]>(
    () => reviewEventsForWorkItem(attempt, pipeline),
    [attempt, pipeline]
  );
  const workpadSections = useMemo(
    () =>
      buildRunWorkpadSections({
        attempt,
        checkpoint,
        operations: detailOperations,
        pipeline,
        proofCards,
        pullRequestStatus,
        requirement,
        reviewEvents,
        runWorkpad,
        workItem
      }),
    [attempt, checkpoint, detailOperations, pipeline, proofCards, pullRequestStatus, requirement, reviewEvents, runWorkpad, workItem]
  );
  const timelineItems =
    attemptTimeline && attempt?.id && attemptTimeline.attempt?.id === attempt.id
      ? attemptTimeline.items ?? []
      : [];

  return (
    <section className="issue-detail-view work-item-detail-page" aria-label="Work item detail">
      <article className="issue-detail-document">
        <nav className="detail-breadcrumb" aria-label="Requirement hierarchy">
          <span>{repositoryLabel || "Workspace"}</span>
          <span>Requirement</span>
          <strong>{workItem.key}</strong>
        </nav>
        <header className="issue-detail-title">
          <div className="issue-detail-state">
            <span className={`issue-state ${statusClassName(workItem.status)}`} aria-hidden="true" />
            <span>{workItemStatusLabel(workItem.status)}</span>
          </div>
          <h2>{workItem.title}</h2>
          <div className="issue-detail-meta">
            <span>{workItem.key}</span>
            <span>{sourceLabel(workItem)}</span>
            {workItem.sourceExternalRef ? <span>{workItem.sourceExternalRef}</span> : null}
            {repositoryLabel ? <span>{repositoryLabel}</span> : null}
            <span>{agentShortLabel(workItem.assignee)}</span>
          </div>
        </header>

        <section className="issue-detail-section detail-flow-priority">
          <h3>Delivery flow</h3>
          <DeliveryFlowGrid
            agentShortLabel={agentShortLabel}
            pipeline={pipeline}
            pipelineStageClassName={pipelineStageClassName}
            pipelineStageLabel={pipelineStageLabel}
          />
        </section>

        <RunWorkpad sections={workpadSections} />

        <section className="issue-detail-section">
          <h3>Requirement source</h3>
          <div className="requirement-source-card">
            <div>
              <span>{requirement?.source === "github_issue" ? "GitHub issue" : "Manual requirement"}</span>
              <strong>{requirement?.title ?? workItem.title}</strong>
            </div>
            <div className="requirement-source-meta">
              {requirement?.sourceExternalRef ? <span>{requirement.sourceExternalRef}</span> : null}
              {requirement?.status ? <span>{requirement.status}</span> : null}
              <span>{siblingItems.length || 1} item{(siblingItems.length || 1) === 1 ? "" : "s"}</span>
            </div>
          </div>
          {(requirement?.rawText || workItem.description) && workItem.description !== "No description provided." ? (
            <div className="issue-detail-copy requirement-source-scroll markdown-content">
              {renderMarkdown(requirement?.rawText ?? workItem.description)}
            </div>
          ) : (
            <p className="muted-copy">No description provided yet.</p>
          )}
        </section>

        <section className="issue-detail-section">
          <h3>Current attempt</h3>
          <WorkItemAttemptPanel
            agentShortLabel={agentShortLabel}
            attempt={attempt}
            attemptStatusLabel={attemptStatusLabel}
            checkpoint={checkpoint}
            displayText={displayText}
            failedStages={failedStages}
            failureOperations={failureOperations}
            failureProofCards={failureProofCards}
            humanReviewArtifacts={humanReviewArtifacts}
            humanReviewEvents={reviewEvents}
            onApproveCheckpoint={onApproveCheckpoint}
            onRequestCheckpointChanges={onRequestCheckpointChanges}
            onRetryAttempt={onRetryAttempt}
            operationStatusLabel={operationStatusLabel}
            pipeline={pipeline}
            pipelineStageClassName={pipelineStageClassName}
            pipelineStageLabel={pipelineStageLabel}
            pullRequestStatus={pullRequestStatus?.url === attempt?.pullRequestUrl ? pullRequestStatus : null}
            timelineItems={timelineItems}
          />
        </section>

        <section className="issue-detail-section">
          <h3>Agent operations</h3>
          <AgentTraceList
            agentShortLabel={agentShortLabel}
            operations={detailOperations}
            operationStatusLabel={operationStatusLabel}
            pipelineStageClassName={pipelineStageClassName}
          />
        </section>

        <section className="issue-detail-section">
          <h3>Artifacts</h3>
          <ArtifactGrid proofs={proofCards} />
        </section>

        <section className="issue-detail-section">
          <h3>Attempt history</h3>
          <AttemptHistory attempts={attempts} attemptStatusLabel={attemptStatusLabel} />
        </section>

        <section className="issue-detail-section">
          <h3>Target</h3>
          <div className="detail-target-box">
            <span>{workItem.target}</span>
            {repositoryTargetLabel(repositoryTargets, workItem.repositoryTargetId) ? (
              <small>{repositoryTargetLabel(repositoryTargets, workItem.repositoryTargetId)}</small>
            ) : null}
          </div>
        </section>
      </article>
    </section>
  );
}

function RunWorkpad({ sections }: { sections: WorkpadSection[] }) {
  return (
    <section className="run-workpad" aria-label="Run workpad">
      <header>
        <div>
          <span className="section-label">Run workpad</span>
          <h3>Execution brief</h3>
        </div>
        <small>{sections.length} live sections</small>
      </header>
      <div className="run-workpad-grid">
        {sections.map((section) => (
          <details key={section.id} className={section.tone ? `workpad-${section.tone}` : undefined}>
            <summary>
              <div>
                <span>{section.label}</span>
                <strong>{section.title}</strong>
              </div>
            </summary>
            <div className="workpad-detail-body">{section.body}</div>
          </details>
        ))}
      </div>
    </section>
  );
}

function DeliveryFlowGrid({
  agentShortLabel,
  pipeline,
  pipelineStageClassName,
  pipelineStageLabel
}: Pick<DetailHelpers, "agentShortLabel" | "pipelineStageClassName" | "pipelineStageLabel"> & {
  pipeline?: PipelineRecordInfo;
}) {
  const stages = pipeline?.run?.stages ?? [];
  if (!stages.length) {
    return <p className="muted-copy">Delivery stages will appear after a pipeline is created.</p>;
  }
  return (
    <div className="detail-stage-grid">
      {stages.map((stage, index) => {
        const agentIds = stage.agentIds ?? (stage.agentId ? [stage.agentId] : []);
        return (
          <article key={stage.id} className={pipelineStageClassName(stage.status)}>
            <span>{index + 1}</span>
            <div>
              <strong>{stage.title ?? stage.id}</strong>
              <small>{agentIds.length ? agentIds.map(agentShortLabel).join(" + ") : "Agent pending"}</small>
            </div>
            <em>{pipelineStageLabel(stage.status)}</em>
          </article>
        );
      })}
    </div>
  );
}

function buildRunWorkpadSections({
  attempt,
  checkpoint,
  operations,
  pipeline,
  proofCards,
  pullRequestStatus,
  requirement,
  reviewEvents,
  runWorkpad,
  workItem
}: {
  attempt?: AttemptRecordInfo;
  checkpoint?: CheckpointRecordInfo;
  operations: OperationRecordInfo[];
  pipeline?: PipelineRecordInfo;
  proofCards: DetailProofCard[];
  pullRequestStatus?: GitHubPullRequestStatusResult | null;
  requirement?: RequirementRecordInfo;
  reviewEvents: DetailReviewEventCard[];
  runWorkpad?: RunWorkpadRecordInfo;
  workItem: WorkItem;
}): WorkpadSection[] {
  const workpad = runWorkpad?.workpad;
  const planArtifacts = proofCards.filter((proof) => /plan|solution|implementation|summary/i.test(`${proof.label} ${proof.kind}`));
  const validationOps = operations.filter((operation) => /test|check|validation/i.test(`${operation.stageId ?? ""} ${operation.agentId ?? ""} ${operation.summary ?? ""}`));
  const reviewOps = operations.filter((operation) => /review|rework/i.test(`${operation.stageId ?? ""} ${operation.agentId ?? ""} ${operation.summary ?? ""}`));
  const recordedBlockers = asStringArray(workpad?.blockers);
  const blockers = recordedBlockers.length ? recordedBlockers : [
    attempt?.failureReason,
    attempt?.errorMessage,
    checkpoint ? `${checkpoint.title}: ${checkpoint.summary}` : "",
    pullRequestStatus?.deliveryGate && pullRequestStatus.deliveryGate !== "passed" ? `PR gate: ${pullRequestStatus.deliveryGate}` : ""
  ].filter(Boolean) as string[];
  const recordedFeedback = asStringArray(workpad?.reviewFeedback);
  const reviewFeedback =
    recordedFeedback[0] ||
    attempt?.failureReviewFeedback ||
    reviewOps.map((operation) => operation.summary || operation.runnerProcess?.stderr || "").find(Boolean) ||
    reviewEvents.map((event) => event.message).find(Boolean);
  const retryReason =
    workpad?.retryReason ||
    (attempt && ["failed", "stalled", "canceled"].includes(attempt.status) ? retryReasonForAttempt(attempt) : "");
  const acceptanceCriteria = asStringArray(workpad?.acceptanceCriteria);
  const criteria = acceptanceCriteria.length
    ? acceptanceCriteria
    : (workItem.acceptanceCriteria.length ? workItem.acceptanceCriteria : requirement?.acceptanceCriteria ?? ["No acceptance criteria captured."]);
  const validationStatus = recordString(workpad?.validation, "status");
  const reworkAssessment = recordValue(workpad?.reworkAssessment) || recordValue(attempt?.reworkAssessment);
  const reworkStrategy = recordString(reworkAssessment, "strategy");
  const reworkChecklist = asStringArray(recordValue(reworkAssessment)?.checklist);
  const sections: WorkpadSection[] = [
    ...(reworkStrategy
      ? [{
          id: "rework-assessment",
          label: "Rework assessment",
          title: reworkStrategyLabel(reworkStrategy),
          tone: reworkStrategy === "needs_human_info" ? "warning" as const : undefined,
          body: (
            <div className="workpad-rework-assessment">
              <p>{recordString(reworkAssessment, "rationale") || "Human feedback has been assessed for the next run path."}</p>
              {recordString(reworkAssessment, "humanFeedback") ? <blockquote>{shortText(recordString(reworkAssessment, "humanFeedback"), 360)}</blockquote> : null}
              {reworkChecklist.length ? (
                <ul>
                  {reworkChecklist.slice(0, 5).map((item) => <li key={item}>{item}</li>)}
                </ul>
              ) : null}
            </div>
          )
        }]
      : []),
    {
      id: "plan",
      label: "Plan",
      title: recordString(workpad?.plan, "currentStageId") || planArtifacts[0]?.label || pipeline?.templateId || "Plan pending",
      body: planArtifacts.length ? <ArtifactList artifacts={planArtifacts.slice(0, 3)} /> : <p>{shortText(requirement?.rawText ?? workItem.description)}</p>
    },
    {
      id: "acceptance",
      label: "Acceptance criteria",
      title: `${criteria.length} criteria`,
      body: (
        <ul>
          {criteria.slice(0, 5).map((criterion) => <li key={criterion}>{criterion}</li>)}
        </ul>
      )
    },
    {
      id: "validation",
      label: "Validation status",
      title: validationStatus || pullRequestStatus?.deliveryGate || (validationOps.length ? "Validation captured" : "Pending"),
      tone: validationStatus === "passed" || pullRequestStatus?.deliveryGate === "passed" ? "success" : undefined,
      body: validationOps.length ? <OperationSummaryList operations={validationOps.slice(-3)} /> : <p>Test reports and checks will appear here after validation runs.</p>
    },
    {
      id: "pr",
      label: "PR",
      title: attempt?.pullRequestUrl ? "Pull request ready" : attempt?.branchName ? "Branch ready" : "Not created",
      body: attempt?.pullRequestUrl ? (
        <a href={attempt.pullRequestUrl} target="_blank" rel="noreferrer">Open PR</a>
      ) : (
        <p>{attempt?.branchName ?? "PR link will appear after delivery creates it."}</p>
      )
    },
    {
      id: "feedback",
      label: "Review Feedback",
      title: reviewFeedback ? "Feedback captured" : "No feedback yet",
      body: reviewFeedback ? <p>{shortText(reviewFeedback, 360)}</p> : <p>Review agent, PR comments and human requested changes will be merged here.</p>
    },
    {
      id: "blockers",
      label: "Blockers",
      title: blockers.length ? `${blockers.length} active signal${blockers.length === 1 ? "" : "s"}` : "No active blockers",
      tone: blockers.length ? "warning" : "success",
      body: blockers.length ? (
        <ul>{blockers.slice(0, 4).map((blocker) => <li key={blocker}>{shortText(blocker, 220)}</li>)}</ul>
      ) : (
        <p>No blocking failure is recorded for the current attempt.</p>
      )
    },
    {
      id: "retry",
      label: "Retry Reason",
      title: retryReason ? "Ready for retry" : "No retry needed",
      body: retryReason ? <p>{shortText(retryReason, 420)}</p> : <p>Retry will reuse the captured blocker and review feedback when needed.</p>
    },
    {
      id: "notes",
      label: "Notes",
      title: `${operations.length} operation${operations.length === 1 ? "" : "s"}`,
      body: operations.length ? <OperationSummaryList operations={operations.slice(-3)} /> : <p>Agent notes will appear as operations are recorded.</p>
    }
  ];

  return sections;
}

function ArtifactList({ artifacts }: { artifacts: DetailProofCard[] }) {
  return (
    <ul>
      {artifacts.map((artifact) => (
        <li key={artifact.id}>{artifact.label}</li>
      ))}
    </ul>
  );
}

function OperationSummaryList({ operations }: { operations: OperationRecordInfo[] }) {
  return (
    <ul>
      {operations.map((operation) => (
        <li key={operation.id}>{operation.summary || `${operation.stageId ?? "stage"} ${operation.status}`}</li>
      ))}
    </ul>
  );
}

function latestRunWorkpadForDetail({
  attempt,
  pipeline,
  runWorkpads,
  workItem
}: {
  attempt?: AttemptRecordInfo;
  pipeline?: PipelineRecordInfo;
  runWorkpads: RunWorkpadRecordInfo[];
  workItem: WorkItem;
}) {
  const matches = runWorkpads.filter((record) =>
    (attempt?.id && record.attemptId === attempt.id) ||
    (pipeline?.id && record.pipelineId === pipeline.id) ||
    record.workItemId === workItem.id
  );
  return matches.sort((left, right) => (right.updatedAt ?? "").localeCompare(left.updatedAt ?? ""))[0];
}

function asStringArray(value: unknown): string[] {
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string" && item.trim().length > 0) : [];
}

function recordString(value: unknown, key: string): string {
  if (!value || typeof value !== "object" || Array.isArray(value)) return "";
  const raw = (value as Record<string, unknown>)[key];
  return typeof raw === "string" ? raw : "";
}

function recordValue(value: unknown): Record<string, unknown> | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
  return value as Record<string, unknown>;
}

function reworkStrategyLabel(strategy: string): string {
  switch (strategy) {
    case "fast_rework":
      return "Fast rework";
    case "replan_rework":
      return "Replan then rework";
    case "needs_human_info":
      return "Needs human info";
    default:
      return strategy;
  }
}

function operationsForWorkItem({
  attempt,
  operations,
  pipeline,
  workItem
}: {
  attempt?: AttemptRecordInfo;
  operations: OperationRecordInfo[];
  pipeline?: PipelineRecordInfo;
  workItem: WorkItem;
}) {
  const pipelineId = pipeline?.id ?? attempt?.pipelineId ?? "";
  return [...operations]
    .filter((operation) =>
      Boolean(pipelineId && (operation.id.includes(pipelineId) || operation.missionId?.includes(pipelineId))) ||
      Boolean(operation.missionId?.includes(workItem.id)) ||
      Boolean(operation.missionId?.includes(workItem.key)) ||
      Boolean(operation.prompt?.includes(workItem.key))
    )
    .sort((left, right) => (left.createdAt ?? left.updatedAt ?? "").localeCompare(right.createdAt ?? right.updatedAt ?? ""));
}

function proofCardsForWorkItem({
  attempt,
  operations,
  pipeline,
  proofRecords,
  workItem
}: {
  attempt?: AttemptRecordInfo;
  operations: OperationRecordInfo[];
  pipeline?: PipelineRecordInfo;
  proofRecords: ProofRecordInfo[];
  workItem: WorkItem;
}): DetailProofCard[] {
  const cards: DetailProofCard[] = [];
  const seen = new Set<string>();
  const addCard = (input: Omit<DetailProofCard, "id" | "kind"> & { id?: string; kind?: string }) => {
    const label = input.label || (input.path ? fileNameFromPath(input.path) : input.url ?? "proof");
    const key = input.path || input.url || `${input.stage ?? ""}:${label}`;
    if (seen.has(key)) return;
    seen.add(key);
    cards.push({
      id: input.id ?? key,
      kind: input.kind ?? proofKindLabel(`${label} ${input.path ?? ""} ${input.url ?? ""}`),
      label,
      stage: input.stage,
      path: input.path,
      url: input.url
    });
  };

  const operationIds = new Set(operations.map((operation) => operation.id));
  const stageSnapshots = attempt?.stages?.length ? attempt.stages : pipeline?.run?.stages ?? [];
  for (const stage of stageSnapshots) {
    const evidence = "evidence" in stage && Array.isArray(stage.evidence) ? stage.evidence : [];
    for (const proof of evidence) {
      addCard({ label: fileNameFromPath(proof), path: proof, stage: stage.title ?? stage.id });
    }
  }

  const pipelineId = pipeline?.id ?? attempt?.pipelineId ?? "";
  for (const record of proofRecords) {
    const belongsToPipeline = pipelineId && record.operationId?.includes(pipelineId);
    const belongsToAttempt = attempt?.id && record.operationId?.includes(attempt.id);
    const belongsToOperation = record.operationId ? operationIds.has(record.operationId) : false;
    const belongsToWorkItem = record.operationId?.includes(workItem.id) || record.operationId?.includes(workItem.key);
    if (!belongsToPipeline && !belongsToAttempt && !belongsToOperation && !belongsToWorkItem) continue;
    if (!record.sourcePath && !record.sourceUrl) continue;
    addCard({
      id: record.id,
      label: record.value || record.label,
      path: record.sourcePath,
      url: record.sourceUrl,
      stage: record.label
    });
  }
  return cards;
}

function reviewEventsForWorkItem(attempt?: AttemptRecordInfo, pipeline?: PipelineRecordInfo): DetailReviewEventCard[] {
  const attemptEvents =
    attempt?.events?.map((event, index) => ({
      id: `attempt:${attempt.id}:${index}`,
      type: event.type ?? "event",
      message: event.message ?? "",
      stageId: event.stageId,
      createdAt: event.createdAt
    })) ?? [];
  const runEvents =
    pipeline?.run?.events?.map((event, index) => ({
      id: `run:${pipeline.id}:${index}`,
      type: event.type,
      message: event.message,
      stageId: event.stageId,
      createdAt: event.timestamp
    })) ?? [];
  const seen = new Set<string>();
  return [...attemptEvents, ...runEvents]
    .filter((event) => {
      const text = `${event.type} ${event.stageId ?? ""} ${event.message}`.toLowerCase();
      if (!/(review|rework|coding|test|delivery|human|changes|approve|blocked|passed)/.test(text)) return false;
      const key = `${event.type}:${event.stageId ?? ""}:${event.message}:${event.createdAt ?? ""}`;
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    })
    .slice(-8)
    .reverse();
}

function repositoryTargetLabel(repositoryTargets: RepositoryTarget[], repositoryTargetId?: string): string {
  const target = repositoryTargetId ? repositoryTargets.find((candidate) => candidate.id === repositoryTargetId) : undefined;
  if (!target) return "";
  return target.kind === "github" ? `${target.owner}/${target.repo}` : target.path;
}

function fileNameFromPath(value: string): string {
  return value.split(/[\\/]/).pop() ?? value;
}

function proofKindLabel(value: string): string {
  const lower = value.toLowerCase();
  if (lower.includes("requirement")) return "Requirement";
  if (lower.includes("solution") || lower.includes("plan")) return "Solution";
  if (lower.includes("diff") || lower.includes("implementation") || lower.includes("change")) return "Diff";
  if (lower.includes("test") || lower.includes("check")) return "Test";
  if (lower.includes("review")) return "Review";
  if (lower.includes("pr") || lower.includes("pull")) return "PR";
  if (lower.includes("merge") || lower.includes("delivery")) return "Merge";
  if (lower.includes("handoff")) return "Handoff";
  return "Artifact";
}

function displayText(value: string): string {
  return value.replace(/\\n/g, "\n");
}

function shortText(value = "", max = 260): string {
  const normalized = displayText(value).replace(/\s+/g, " ").trim();
  if (normalized.length <= max) return normalized || "No details captured yet.";
  return `${normalized.slice(0, max - 1)}...`;
}

function safeMarkdownHref(href: string): string | undefined {
  return /^(https?:|mailto:)/i.test(href) ? href : undefined;
}

function renderInlineMarkdown(text: string, keyPrefix: string): ReactNode[] {
  const nodes: ReactNode[] = [];
  const pattern = /(`[^`]+`|\*\*[^*]+\*\*|\[[^\]]+\]\([^)]+\))/g;
  let cursor = 0;
  let match: RegExpExecArray | null;

  while ((match = pattern.exec(text))) {
    if (match.index > cursor) nodes.push(text.slice(cursor, match.index));
    const token = match[0];
    const key = `${keyPrefix}-${match.index}`;
    if (token.startsWith("`")) {
      nodes.push(<code key={key}>{token.slice(1, -1)}</code>);
    } else if (token.startsWith("**")) {
      nodes.push(<strong key={key}>{token.slice(2, -2)}</strong>);
    } else {
      const linkMatch = token.match(/^\[([^\]]+)\]\(([^)]+)\)$/);
      const href = linkMatch ? safeMarkdownHref(linkMatch[2]) : undefined;
      nodes.push(
        href ? (
          <a key={key} href={href} target="_blank" rel="noreferrer">
            {linkMatch?.[1]}
          </a>
        ) : (
          <span key={key}>{linkMatch?.[1] ?? token}</span>
        )
      );
    }
    cursor = match.index + token.length;
  }
  if (cursor < text.length) nodes.push(text.slice(cursor));
  return nodes;
}

function renderMarkdown(value: string): ReactNode[] {
  const lines = displayText(value).split("\n");
  const nodes: ReactNode[] = [];
  let index = 0;

  while (index < lines.length) {
    const line = lines[index];
    const trimmed = line.trim();
    if (!trimmed) {
      index += 1;
      continue;
    }

    const heading = trimmed.match(/^(#{1,3})\s+(.+)$/);
    if (heading) {
      const level = heading[1].length;
      const content = renderInlineMarkdown(heading[2], `h-${index}`);
      nodes.push(level === 1 ? <h1 key={index}>{content}</h1> : level === 2 ? <h2 key={index}>{content}</h2> : <h3 key={index}>{content}</h3>);
      index += 1;
      continue;
    }

    if (trimmed.startsWith("```")) {
      const codeLines: string[] = [];
      index += 1;
      while (index < lines.length && !lines[index].trim().startsWith("```")) {
        codeLines.push(lines[index]);
        index += 1;
      }
      if (index < lines.length) index += 1;
      nodes.push(
        <pre key={index}>
          <code>{codeLines.join("\n")}</code>
        </pre>
      );
      continue;
    }

    if (/^[-*]\s+/.test(trimmed)) {
      const items: string[] = [];
      while (index < lines.length && /^[-*]\s+/.test(lines[index].trim())) {
        items.push(lines[index].trim().replace(/^[-*]\s+/, ""));
        index += 1;
      }
      nodes.push(
        <ul key={index}>
          {items.map((item, itemIndex) => (
            <li key={`${index}-${itemIndex}`}>{renderInlineMarkdown(item, `ul-${index}-${itemIndex}`)}</li>
          ))}
        </ul>
      );
      continue;
    }

    if (/^\d+\.\s+/.test(trimmed)) {
      const items: string[] = [];
      while (index < lines.length && /^\d+\.\s+/.test(lines[index].trim())) {
        items.push(lines[index].trim().replace(/^\d+\.\s+/, ""));
        index += 1;
      }
      nodes.push(
        <ol key={index}>
          {items.map((item, itemIndex) => (
            <li key={`${index}-${itemIndex}`}>{renderInlineMarkdown(item, `ol-${index}-${itemIndex}`)}</li>
          ))}
        </ol>
      );
      continue;
    }

    if (trimmed.startsWith(">")) {
      const quoteLines: string[] = [];
      while (index < lines.length && lines[index].trim().startsWith(">")) {
        quoteLines.push(lines[index].trim().replace(/^>\s?/, ""));
        index += 1;
      }
      nodes.push(<blockquote key={index}>{renderInlineMarkdown(quoteLines.join(" "), `quote-${index}`)}</blockquote>);
      continue;
    }

    const paragraphLines: string[] = [];
    while (
      index < lines.length &&
      lines[index].trim() &&
      !/^(#{1,3})\s+/.test(lines[index].trim()) &&
      !lines[index].trim().startsWith("```") &&
      !/^[-*]\s+/.test(lines[index].trim()) &&
      !/^\d+\.\s+/.test(lines[index].trim()) &&
      !lines[index].trim().startsWith(">")
    ) {
      paragraphLines.push(lines[index].trim());
      index += 1;
    }
    nodes.push(<p key={index}>{renderInlineMarkdown(paragraphLines.join(" "), `p-${index}`)}</p>);
  }

  return nodes;
}
