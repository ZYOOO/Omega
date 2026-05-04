import { useState } from "react";
import type {
  AttemptRecordInfo,
  AttemptActionPlanInfo,
  AttemptTimelineItemInfo,
  CheckpointRecordInfo,
  GitHubPullRequestStatusResult,
  OperationRecordInfo,
  PipelineRecordInfo,
  ProofPreviewInfo
} from "../omegaControlApiClient";

export interface DetailProofCard {
  id: string;
  kind: string;
  label: string;
  stage?: string;
  path?: string;
  url?: string;
}

export interface DetailReviewEventCard {
  id: string;
  type: string;
  message: string;
  stageId?: string;
  createdAt?: string;
}

type AttemptStage = NonNullable<AttemptRecordInfo["stages"]>[number];
type PipelineStage = NonNullable<NonNullable<PipelineRecordInfo["run"]>["stages"]>[number];

type LabelHelpers = {
  agentShortLabel: (agentId: string) => string;
  attemptStatusLabel: (status: string) => string;
  displayText: (value: string) => string;
  operationStatusLabel: (status: string) => string;
  pipelineStageClassName: (status: string) => string;
  pipelineStageLabel: (status: string) => string;
};

type FailedStageSummary = {
  id: string;
  title?: string;
  status: string;
};

interface WorkItemAttemptPanelProps extends LabelHelpers {
  attempt?: AttemptRecordInfo;
  actionPlan?: AttemptActionPlanInfo | null;
  pipeline?: PipelineRecordInfo;
  checkpoint?: CheckpointRecordInfo;
  checkpointActionable?: boolean;
  failedStages: FailedStageSummary[];
  failureOperations: OperationRecordInfo[];
  failureProofCards: DetailProofCard[];
  humanReviewArtifacts: DetailProofCard[];
  humanReviewEvents: DetailReviewEventCard[];
  onApproveCheckpoint: (checkpointId: string) => void;
  onFetchProofPreview?: (proofId: string) => Promise<ProofPreviewInfo>;
  onRequestCheckpointChanges: (checkpointId: string, note?: string) => void;
  onRetryAttempt?: (attemptId: string) => void;
  pullRequestStatus?: GitHubPullRequestStatusResult | null;
  timelineItems?: AttemptTimelineItemInfo[];
}

export function WorkItemAttemptPanel({
  actionPlan,
  agentShortLabel,
  attempt,
  attemptStatusLabel,
  checkpoint,
  checkpointActionable = false,
  displayText,
  failedStages,
  failureOperations,
  failureProofCards,
  humanReviewArtifacts,
  humanReviewEvents,
  onApproveCheckpoint,
  onFetchProofPreview,
  onRequestCheckpointChanges,
  onRetryAttempt,
  operationStatusLabel,
  pipeline,
  pipelineStageClassName,
  pipelineStageLabel,
  pullRequestStatus,
  timelineItems = []
}: WorkItemAttemptPanelProps) {
  if (!attempt) {
    if (checkpoint) {
      return (
        <article className="attempt-card review-ready-card">
          <header>
            <div>
              <strong>Waiting for human review</strong>
              <span>Review the delivery packet, then approve or request changes.</span>
            </div>
          </header>
          <HumanGateCard
            checkpoint={checkpoint}
            actionable={checkpointActionable}
            displayText={displayText}
            humanReviewArtifacts={humanReviewArtifacts}
            humanReviewEvents={humanReviewEvents}
            onApproveCheckpoint={onApproveCheckpoint}
            onFetchProofPreview={onFetchProofPreview}
            onRequestCheckpointChanges={onRequestCheckpointChanges}
          />
        </article>
      );
    }
    return <p className="muted-copy">No execution attempt yet. Run this item to create a traceable attempt.</p>;
  }

  const planStates = actionPlan?.states?.length ? actionPlan.states : [];
  const stages = planStates.length ? planStates : attempt.stages?.length ? attempt.stages : pipeline?.run?.stages ?? [];
  const retryable = ["failed", "stalled", "canceled"].includes(attempt.status);

  return (
    <article className="attempt-card">
      <header>
        <div>
          <strong>{attemptStatusLabel(attempt.status)}</strong>
          <span>
            {attempt.runner ?? "runner"}
            {typeof attempt.durationMs === "number" && attempt.durationMs > 0 ? ` · ${attempt.durationMs}ms` : ""}
          </span>
        </div>
        {attempt.pullRequestUrl ? (
          <a href={attempt.pullRequestUrl} target="_blank" rel="noreferrer">PR</a>
        ) : attempt.branchName ? (
          <span>{attempt.branchName}</span>
        ) : null}
        {retryable && onRetryAttempt ? (
          <button type="button" className="attempt-retry-action" onClick={() => onRetryAttempt(attempt.id)}>
            Retry attempt
          </button>
        ) : null}
      </header>

      <details className="attempt-stage-details">
        <summary>Stage details</summary>
        <ActionPlanSummary actionPlan={actionPlan} />
        <div className="attempt-stage-flow" aria-label={`Attempt ${attempt.id} stages`}>
          {stages.map((stage) => (
            <AttemptStageCard
              key={`${attempt.id}-${String(stage.id)}`}
              agentShortLabel={agentShortLabel}
              pipelineStageClassName={pipelineStageClassName}
              pipelineStageLabel={pipelineStageLabel}
              stage={stage}
            />
          ))}
        </div>
      </details>

      {checkpoint ? (
        <HumanGateCard
          attempt={attempt}
          checkpoint={checkpoint}
          actionable={checkpointActionable}
          displayText={displayText}
          humanReviewArtifacts={humanReviewArtifacts}
          humanReviewEvents={humanReviewEvents}
          onApproveCheckpoint={onApproveCheckpoint}
          onFetchProofPreview={onFetchProofPreview}
          onRequestCheckpointChanges={onRequestCheckpointChanges}
        />
      ) : null}

      {pullRequestStatus ? <PullRequestLifecycleCard status={pullRequestStatus} /> : null}

      <RunTimeline items={timelineItems} />

      {attempt.status === "failed" ? (
        <AttemptFailureReport
          agentShortLabel={agentShortLabel}
          attempt={attempt}
          failedStages={failedStages}
          failureOperations={failureOperations}
          failureProofCards={failureProofCards}
          operationStatusLabel={operationStatusLabel}
        />
      ) : null}

      {attempt.status !== "failed" && attempt.errorMessage ? <p className="attempt-error">{attempt.errorMessage}</p> : null}
      {attempt.workspacePath ? <p className="attempt-path">Workspace: {attempt.workspacePath}</p> : null}
    </article>
  );
}

function ActionPlanSummary({ actionPlan }: { actionPlan?: AttemptActionPlanInfo | null }) {
  if (!actionPlan?.currentAction && !actionPlan?.retry && !actionPlan?.transitions?.length) {
    return null;
  }
  const action = actionPlan.currentAction ?? {};
  const retry = actionPlan.retry ?? {};
  const state = actionPlan.currentState ?? {};
  const transitionLabels = (actionPlan.transitions ?? [])
    .map((transition) => `${recordText(transition, "on")} -> ${recordText(transition, "to")}`.trim())
    .filter((value) => value !== "->")
    .slice(0, 3);
  return (
    <section className="action-plan-summary" aria-label="Attempt action plan">
      <div>
        <span>Action plan</span>
        <strong>{recordText(action, "title") || recordText(action, "id") || recordText(state, "title") || "Runtime contract"}</strong>
        <small>{recordText(action, "status") || recordText(state, "status") || actionPlan.attemptStatus || "ready"}</small>
      </div>
      {recordBool(retry, "available") ? (
        <p>
          Retry: {recordText(retry, "recommendedAction") || "retry_attempt"} · {recordText(retry, "reason")}
        </p>
      ) : transitionLabels.length ? (
        <p>{transitionLabels.join(" · ")}</p>
      ) : null}
    </section>
  );
}

function RunTimeline({ items }: { items: AttemptTimelineItemInfo[] }) {
  if (!items.length) {
    return null;
  }
  const visibleItems = [...items].reverse();
  const latestItem = visibleItems[0];
  return (
    <details className="run-timeline" aria-label="Run timeline">
      <summary>
        <div>
          <span>Run timeline</span>
          <strong>{items.length} recent event{items.length === 1 ? "" : "s"}</strong>
          {latestItem ? <small>{latestItem.eventType}: {latestItem.message}</small> : null}
        </div>
      </summary>
      <div className="run-timeline-list">
        {visibleItems.map((item) => (
          <article key={item.id} className={`run-timeline-row ${item.level.toLowerCase() === "error" ? "is-error" : ""}`}>
            <span className="run-timeline-dot" />
            <div>
              <p>
                <strong>{item.eventType}</strong>
                {item.stageId ? <small>{item.stageId}</small> : null}
              </p>
              <span>{item.message}</span>
              <footer>
                <time>{formatBeijingTimestamp(item.time)}</time>
                <em>{item.source}</em>
                {item.agentId ? <em>{item.agentId}</em> : null}
              </footer>
            </div>
          </article>
        ))}
      </div>
    </details>
  );
}

function PullRequestLifecycleCard({ status }: { status: GitHubPullRequestStatusResult }) {
  return (
    <section className="pr-lifecycle-card" aria-label="Pull request lifecycle">
      <header>
        <div>
          <span>PR lifecycle</span>
          <strong>{status.title ?? `Pull request #${status.number ?? ""}`}</strong>
        </div>
        <small>{status.deliveryGate}</small>
      </header>
      <div className="pr-lifecycle-grid">
        <article>
          <span>Review</span>
          <strong>{status.reviewDecision || "No decision"}</strong>
        </article>
        <article>
          <span>Mergeable</span>
          <strong>{status.mergeable || "Unknown"}</strong>
        </article>
        <article>
          <span>Branch</span>
          <strong>{status.headRefName || "Unknown"}</strong>
          {status.baseRefName ? <small>into {status.baseRefName}</small> : null}
        </article>
      </div>
      {status.checks.length ? (
        <div className="pr-check-list">
          {status.checks.map((check) => (
            <a key={`${check.name}:${check.link ?? check.state}`} href={check.link} target="_blank" rel="noreferrer">
              <span>{check.name}</span>
              <strong>{check.state}</strong>
            </a>
          ))}
        </div>
      ) : null}
    </section>
  );
}

function AttemptStageCard({
  agentShortLabel,
  pipelineStageClassName,
  pipelineStageLabel,
  stage
}: Pick<LabelHelpers, "agentShortLabel" | "pipelineStageClassName" | "pipelineStageLabel"> & {
  stage: AttemptStage | PipelineStage | Record<string, unknown>;
}) {
  const status = recordText(stage, "status");
  const title = recordText(stage, "title") || recordText(stage, "id");
  const agent = recordText(stage, "agent") || recordText(stage, "agentId");
  const stageRecord = stage as Record<string, unknown>;
  const agentIds = Array.isArray(stageRecord.agentIds) ? stageRecord.agentIds.map(String) : agent ? [agent] : [];
  const evidence = Array.isArray(stageRecord.evidence) ? stageRecord.evidence.map(String) : [];
  const artifactLabels = [
    ...(Array.isArray(stageRecord.outputArtifacts) ? stageRecord.outputArtifacts.map(String) : []),
    ...evidence.map((item) => item.split("/").pop() ?? item)
  ];

  return (
    <article className={pipelineStageClassName(status)}>
      <span>{pipelineStageLabel(status)}</span>
      <strong>{title}</strong>
      <small>{stageParticipantLabel(stageRecord, agentIds, agentShortLabel)}</small>
      {artifactLabels.length ? <em>{artifactLabels.slice(0, 2).join(", ")}</em> : null}
    </article>
  );
}

function stageParticipantLabel(stage: Record<string, unknown>, agentIds: string[], agentShortLabel: (agentId: string) => string): string {
  if (agentIds.length) return agentIds.map(agentShortLabel).join(" + ");
  const raw = `${recordText(stage, "id")} ${recordText(stage, "title")}`.toLowerCase();
  if (/todo|intake|requirement/.test(raw)) return "Requirement";
  if (/implementation|architect|coding|test/.test(raw)) return "Architecture + Code + Test";
  if (/code_review|review round|review/.test(raw)) return "Review";
  if (/rework/.test(raw)) return "Code + Test";
  if (/human/.test(raw)) return "Human Review";
  if (/merg|deliver|done|ship/.test(raw)) return "Delivery";
  return "Workflow stage";
}

function recordText(record: Record<string, unknown> | undefined | null, key: string): string {
  const value = record?.[key];
  return typeof value === "string" ? value : typeof value === "number" ? String(value) : "";
}

function recordBool(record: Record<string, unknown> | undefined | null, key: string): boolean {
  return record?.[key] === true;
}

interface HumanGateCardProps {
  actionable: boolean;
  attempt?: AttemptRecordInfo;
  checkpoint: CheckpointRecordInfo;
  displayText: (value: string) => string;
  humanReviewArtifacts: DetailProofCard[];
  humanReviewEvents: DetailReviewEventCard[];
  onApproveCheckpoint: (checkpointId: string) => void;
  onFetchProofPreview?: (proofId: string) => Promise<ProofPreviewInfo>;
  onRequestCheckpointChanges: (checkpointId: string, note?: string) => void;
}

function HumanGateCard({
  actionable,
  attempt,
  checkpoint,
  displayText,
  humanReviewArtifacts,
  humanReviewEvents,
  onApproveCheckpoint,
  onFetchProofPreview,
  onRequestCheckpointChanges
}: HumanGateCardProps) {
  const [reviewNote, setReviewNote] = useState("");
  const previewState = useProofPreviewDialog(onFetchProofPreview);
  const defaultRequestChangesNote = "Please address the review notes before delivery.";
  const changedMaterials = humanReviewArtifacts.filter((proof) =>
    /diff|implementation|change|changed|summary|handoff/i.test(`${proof.kind} ${proof.label} ${proof.path ?? ""}`)
  );
  const validationMaterials = humanReviewArtifacts.filter((proof) =>
    /test|check|validation|review/i.test(`${proof.kind} ${proof.label} ${proof.path ?? ""}`)
  );
  const displayMaterials = humanReviewArtifacts.slice(0, 6);
  const decisionTitle =
    checkpoint.status === "approved"
      ? "Human review approved"
      : checkpoint.status === "rejected"
        ? "Changes requested"
        : "Human review is no longer waiting for input";
  const decisionSummary =
    checkpoint.decisionNote ||
    (checkpoint.status === "approved"
      ? "Delivery has been approved and the workflow can continue."
      : checkpoint.status === "rejected"
        ? "The item was sent back for rework."
        : "This checkpoint is not actionable for the current run.");

  return (
    <section className="human-gate-card human-review-thread" aria-label="Human review checkpoint">
      <header className="human-review-thread-header">
        <div>
          <strong>{checkpoint.title}</strong>
          <p>{checkpoint.summary}</p>
        </div>
        <small>{attempt?.status ?? checkpoint.status}</small>
      </header>

      <article className="human-review-pr-card">
        <section>
          <h4>PR</h4>
          {attempt?.pullRequestUrl ? (
            <a href={attempt.pullRequestUrl} target="_blank" rel="noreferrer">
              {attempt.pullRequestUrl}
            </a>
          ) : (
            <p>PR link will appear after delivery creates the pull request.</p>
          )}
          <div className="human-review-links">
            {attempt?.branchName ? <span>Branch {attempt.branchName}</span> : null}
            {attempt?.workspacePath ? <code>{attempt.workspacePath}</code> : null}
          </div>
        </section>

        <section>
          <h4>Changed</h4>
          {changedMaterials.length ? (
            <ul className="human-review-bullets">
              {changedMaterials.slice(0, 4).map((proof) => (
                <li key={proof.id}>
                  <ArtifactInline onOpen={previewState.open} proof={proof} />
                </li>
              ))}
            </ul>
          ) : (
            <p>Changed-file and diff summaries will appear here once the agent records them.</p>
          )}
        </section>

        <section>
          <h4>Validation</h4>
          {validationMaterials.length ? (
            <ul className="human-review-bullets">
              {validationMaterials.slice(0, 4).map((proof) => (
                <li key={proof.id}>
                  <ArtifactInline onOpen={previewState.open} proof={proof} />
                </li>
              ))}
            </ul>
          ) : humanReviewEvents.length ? (
            <ul className="human-review-bullets">
              {humanReviewEvents.slice(0, 3).map((event) => (
                <li key={event.id}>
                  <span>{event.type}</span>
                  <p>{displayText(event.message)}</p>
                </li>
              ))}
            </ul>
          ) : (
            <p>Review verdicts and test checks will appear here after agent review.</p>
          )}
        </section>

        {displayMaterials.length ? (
          <section>
            <h4>Artifacts</h4>
            <div className="human-review-artifacts compact">
              {displayMaterials.map((proof) => (
                <ArtifactSummary key={proof.id} onOpen={previewState.open} proof={proof} />
              ))}
            </div>
          </section>
        ) : null}
      </article>

      <div className="human-review-activity">
        {humanReviewEvents.slice(0, 4).map((event) => (
          <article key={event.id}>
            <span />
            <div>
              <strong>{event.type}</strong>
              <p>{displayText(event.message)}</p>
            </div>
          </article>
        ))}
      </div>

      <div className="human-review-composer">
        {actionable ? (
          <div>
            <textarea
              aria-label="Human review comment"
              value={reviewNote}
              onChange={(event) => setReviewNote(event.currentTarget.value)}
              placeholder="Leave a comment for the agent before approving or requesting changes..."
            />
            <div className="human-gate-actions">
              <button type="button" className="primary-action" onClick={() => onApproveCheckpoint(checkpoint.id)}>
                Approve delivery
              </button>
              <button
                type="button"
                onClick={() => onRequestCheckpointChanges(checkpoint.id, reviewNote.trim() || defaultRequestChangesNote)}
              >
                Request changes
              </button>
            </div>
          </div>
        ) : (
          <article className={`human-review-decision checkpoint-${checkpoint.status}`}>
            <strong>{decisionTitle}</strong>
            <p>{decisionSummary}</p>
          </article>
        )}
      </div>
      <ProofPreviewDialog state={previewState} />
    </section>
  );
}

function ArtifactInline({ onOpen, proof }: { onOpen: (proof: DetailProofCard) => void; proof: DetailProofCard }) {
  if (proof.url) {
    return (
      <a href={proof.url} target="_blank" rel="noreferrer">
        {proof.label}
      </a>
    );
  }

  return (
    <button type="button" className="artifact-inline-button" onClick={() => onOpen(proof)} title={proof.path ?? proof.label}>
      <span>{artifactFileName(proof)}</span>
      {proof.stage ? <small>{proof.stage}</small> : null}
    </button>
  );
}

function compactArtifactPath(path: string): string {
  const normalized = path.replace(/\\/g, "/");
  const parts = normalized.split("/").filter(Boolean);
  if (parts.length <= 3) return path;
  return `.../${parts.slice(-3).join("/")}`;
}

interface AttemptFailureReportProps {
  agentShortLabel: (agentId: string) => string;
  attempt: AttemptRecordInfo;
  failedStages: FailedStageSummary[];
  failureOperations: OperationRecordInfo[];
  failureProofCards: DetailProofCard[];
  operationStatusLabel: (status: string) => string;
}

function AttemptFailureReport({
  agentShortLabel,
  attempt,
  failedStages,
  failureOperations,
  failureProofCards,
  operationStatusLabel
}: AttemptFailureReportProps) {
  const reason = attempt.failureReason ?? attempt.errorMessage ?? attempt.statusReason ?? "The run failed before a detailed reason was captured.";
  const detail = attempt.failureReviewFeedback ?? attempt.failureDetail;
  const operationDetails = failureOperations
    .map((operation) => ({
      operation,
      stderr: (operation.runnerProcess?.stderr ?? "").trim()
    }));
  return (
    <section className="attempt-failure-report" aria-label="Attempt failure report">
      <div>
        <span>Why retry is needed</span>
        <strong>{failedStages[0]?.title ?? attempt.currentStageId ?? "Pipeline"} blocked this attempt</strong>
        <p>{reason}</p>
        {detail ? <code>{detail.slice(0, 700)}</code> : null}
      </div>
      {failureOperations.length ? (
        <div className="failure-agent-list">
          {operationDetails.map(({ operation, stderr }) => (
            <article key={operation.id}>
              <span>{operation.stageId ?? "stage"} · {agentShortLabel(operation.agentId ?? "agent")}</span>
              <strong>{operationStatusLabel(operation.status)}</strong>
              {operation.summary ? <p>{operation.summary}</p> : null}
              {stderr && !detail ? <code>{stderr.slice(0, 420)}</code> : null}
            </article>
          ))}
        </div>
      ) : null}
      {failureProofCards.length ? (
        <div className="failure-proof-list">
          {failureProofCards.map((proof) => (
            <ArtifactSummary key={proof.id} proof={proof} openLabel="Open artifact" />
          ))}
        </div>
      ) : null}
    </section>
  );
}

export function AgentTraceList({
  agentShortLabel,
  operations,
  operationStatusLabel,
  pipelineStageClassName
}: Pick<LabelHelpers, "agentShortLabel" | "operationStatusLabel" | "pipelineStageClassName"> & {
  operations: OperationRecordInfo[];
}) {
  const [activeOperationId, setActiveOperationId] = useState<string | null>(null);
  const activeOperation = operations.find((operation) => operation.id === activeOperationId);
  if (!operations.length) {
    return <p className="muted-copy">Agent trace will appear as soon as the local orchestrator starts assigning stage work.</p>;
  }

  return (
    <div className="agent-trace-list">
      {operations.map((operation) => (
        <button
          key={operation.id}
          type="button"
          className={`agent-trace-row ${pipelineStageClassName(operation.status)}`}
          onClick={() => setActiveOperationId(operation.id)}
        >
          <span className="agent-trace-card-main">
            <div className="agent-trace-summary-copy">
              <span>{operation.stageId ?? "stage"}</span>
              <strong>{agentShortLabel(operation.agentId ?? "agent")}</strong>
              <em>{agentOperationPreview(operation)}</em>
            </div>
            <small className="agent-trace-status">{operationStatusLabel(operation.status)}</small>
          </span>
          <span className="agent-trace-open" aria-hidden="true">Open</span>
        </button>
      ))}
      {activeOperation ? (
        <section className="detail-popover-backdrop" role="presentation" onClick={() => setActiveOperationId(null)}>
          <article
            className="detail-popover agent-operation-popover"
            role="dialog"
            aria-modal="true"
            aria-label={`${agentShortLabel(activeOperation.agentId ?? "agent")} operation detail`}
            onClick={(event) => event.stopPropagation()}
          >
            <header>
              <div>
                <span>{activeOperation.stageId ?? "stage"}</span>
                <strong>{agentShortLabel(activeOperation.agentId ?? "agent")}</strong>
              </div>
              <button type="button" onClick={() => setActiveOperationId(null)}>Close</button>
            </header>
            <div className="detail-popover-body">
              <div className="agent-operation-dialog-summary">
                <strong>{operationStatusLabel(activeOperation.status)}</strong>
                <p>{activeOperation.summary || agentOperationPreview(activeOperation)}</p>
              </div>
              {activeOperation.runnerProcess ? (
                <div className="agent-runner-meta">
                  <span>{activeOperation.runnerProcess.runner ?? "runner"}</span>
                  {activeOperation.runnerProcess.status ? <span>{activeOperation.runnerProcess.status}</span> : null}
                  {typeof activeOperation.runnerProcess.exitCode === "number" ? <span>exit {activeOperation.runnerProcess.exitCode}</span> : null}
                  {typeof activeOperation.runnerProcess.durationMs === "number" ? <span>{activeOperation.runnerProcess.durationMs}ms</span> : null}
                </div>
              ) : null}
              {activeOperation.prompt ? (
                <section className="agent-detail-block">
                  <strong>Prompt</strong>
                  <code>{activeOperation.prompt}</code>
                </section>
              ) : null}
              {activeOperation.runnerProcess?.stdout ? (
                <section className="agent-detail-block">
                  <strong>Stdout</strong>
                  <code>{activeOperation.runnerProcess.stdout}</code>
                </section>
              ) : null}
              {activeOperation.runnerProcess?.stderr ? (
                <section className="agent-detail-block">
                  <strong>Stderr</strong>
                  <code>{activeOperation.runnerProcess.stderr}</code>
                </section>
              ) : null}
            </div>
          </article>
        </section>
      ) : null}
    </div>
  );
}

export function ArtifactGrid({
  onFetchProofPreview,
  proofs
}: {
  onFetchProofPreview?: (proofId: string) => Promise<ProofPreviewInfo>;
  proofs: DetailProofCard[];
}) {
  const previewState = useProofPreviewDialog(onFetchProofPreview);
  if (!proofs.length) {
    return <p className="muted-copy">No artifact has been collected yet.</p>;
  }

  return (
    <>
      <div className="proof-grid">
        {proofs.map((proof) => (
          <button
            key={proof.id}
            type="button"
            className="proof-card proof-card-button"
            onClick={() => previewState.open(proof)}
            title={proof.path ?? proof.url ?? proof.label}
          >
            <span className="proof-kind">{proof.kind}</span>
            <div className="proof-card-copy">
              <strong>{artifactFileName(proof)}</strong>
              {proof.stage ? <small>{proof.stage}</small> : null}
              {proof.path ? <code>{compactArtifactPath(proof.path)}</code> : null}
            </div>
            <span className="proof-open-label">Preview</span>
          </button>
        ))}
      </div>
      <ProofPreviewDialog state={previewState} />
    </>
  );
}

function agentOperationPreview(operation: OperationRecordInfo): string {
  const value =
    operation.summary ||
    operation.runnerProcess?.stderr ||
    operation.runnerProcess?.stdout ||
    operation.prompt ||
    "";
  return shortInline(value, 110) || "Trace details captured for this stage.";
}

function shortInline(value: string, maxLength: number): string {
  const normalized = value.replace(/\s+/g, " ").trim();
  if (normalized.length <= maxLength) return normalized;
  return `${normalized.slice(0, maxLength - 1)}…`;
}

export function AttemptHistory({
  attempts,
  attemptStatusLabel
}: {
  attempts: AttemptRecordInfo[];
  attemptStatusLabel: (status: string) => string;
}) {
  return (
    <div className="attempt-history">
      {attempts.length === 0 ? (
        <p className="muted-copy">No prior attempts.</p>
      ) : (
        attempts.map((attempt, index) => (
          <article key={attempt.id}>
            <span>#{attempts.length - index}</span>
            <div>
              <strong>{attemptStatusLabel(attempt.status)}</strong>
              <small>
                {attempt.runner ?? "runner"}
                {attempt.currentStageId ? ` · ${attempt.currentStageId}` : ""}
                {attempt.pullRequestUrl ? ` · ${attempt.pullRequestUrl}` : ""}
              </small>
            </div>
            <time>{formatBeijingTimestamp(attempt.finishedAt ?? attempt.startedAt)}</time>
          </article>
        ))
      )}
    </div>
  );
}

function formatBeijingTimestamp(value?: string): string {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat("zh-CN", {
    timeZone: "Asia/Shanghai",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false
  }).format(date);
}

function ArtifactSummary({
  onOpen,
  openLabel = "Open",
  proof
}: {
  onOpen?: (proof: DetailProofCard) => void;
  openLabel?: string;
  proof: DetailProofCard;
}) {
  const content = (
    <>
      <span>{proof.kind}</span>
      <strong>{artifactFileName(proof)}</strong>
      {proof.stage ? <small>{proof.stage}</small> : null}
      {proof.path ? <code title={proof.path}>{compactArtifactPath(proof.path)}</code> : null}
      {proof.url ? <small>{openLabel}</small> : null}
    </>
  );
  if (onOpen && !proof.url) {
    return (
      <button type="button" className="artifact-summary-button" onClick={() => onOpen(proof)} title={proof.path ?? proof.label}>
        {content}
      </button>
    );
  }
  return (
    <article>
      {proof.url ? (
        <a href={proof.url} target="_blank" rel="noreferrer">{content}</a>
      ) : (
        content
      )}
    </article>
  );
}

function useProofPreviewDialog(onFetchProofPreview?: (proofId: string) => Promise<ProofPreviewInfo>) {
  const [proof, setProof] = useState<DetailProofCard | null>(null);
  const [preview, setPreview] = useState<ProofPreviewInfo | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  const open = (nextProof: DetailProofCard) => {
    if (nextProof.url) {
      window.open(nextProof.url, "_blank", "noreferrer");
      return;
    }
    setProof(nextProof);
    setPreview(null);
    setError("");
    if (!onFetchProofPreview) {
      setError("Artifact preview is not connected for this view.");
      return;
    }
    setLoading(true);
    onFetchProofPreview(nextProof.id)
      .then((result) => {
        setPreview(result);
        setError(result.available === false ? result.error || "Artifact content is not available." : "");
      })
      .catch((err: unknown) => setError(err instanceof Error ? err.message : "Artifact preview failed."))
      .finally(() => setLoading(false));
  };

  return {
    close: () => {
      setProof(null);
      setPreview(null);
      setError("");
      setLoading(false);
    },
    error,
    loading,
    open,
    preview,
    proof
  };
}

function ProofPreviewDialog({ state }: { state: ReturnType<typeof useProofPreviewDialog> }) {
  if (!state.proof) return null;
  const sourcePath = state.preview?.sourcePath || state.proof.path || "";
  const content = state.preview?.content ?? "";
  return (
    <section className="detail-popover-backdrop" role="presentation" onClick={state.close}>
      <article
        className="detail-popover proof-preview-popover"
        role="dialog"
        aria-modal="true"
        aria-label={`${artifactFileName(state.proof)} preview`}
        onClick={(event) => event.stopPropagation()}
      >
        <header>
          <div>
            <span>{state.proof.kind}</span>
            <strong>{artifactFileName(state.proof)}</strong>
          </div>
          <button type="button" onClick={state.close}>Close</button>
        </header>
        <div className="detail-popover-body">
          {sourcePath ? <code className="proof-preview-path">{sourcePath}</code> : null}
          {state.loading ? <p className="muted-copy">Loading artifact preview...</p> : null}
          {state.error ? <p className="attempt-error">{state.error}</p> : null}
          {!state.loading && content ? (
            <pre className={`proof-preview-content proof-preview-${state.preview?.previewType ?? "text"}`}>
              <code>{content}</code>
            </pre>
          ) : null}
          {!state.loading && !state.error && !content ? <p className="muted-copy">No preview content captured for this artifact.</p> : null}
          {state.preview?.truncated ? <small className="muted-copy">Preview truncated to keep the detail view responsive.</small> : null}
        </div>
      </article>
    </section>
  );
}

function artifactFileName(proof: DetailProofCard): string {
  const raw = proof.path || proof.label;
  const normalized = raw.replace(/\\/g, "/");
  return normalized.split("/").filter(Boolean).pop() || proof.label;
}
