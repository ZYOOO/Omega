import { useState } from "react";
import type {
  AttemptRecordInfo,
  CheckpointRecordInfo,
  OperationRecordInfo,
  PipelineRecordInfo
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
  pipeline?: PipelineRecordInfo;
  checkpoint?: CheckpointRecordInfo;
  failedStages: FailedStageSummary[];
  failureOperations: OperationRecordInfo[];
  failureProofCards: DetailProofCard[];
  humanReviewArtifacts: DetailProofCard[];
  humanReviewEvents: DetailReviewEventCard[];
  onApproveCheckpoint: (checkpointId: string) => void;
  onRequestCheckpointChanges: (checkpointId: string, note?: string) => void;
}

export function WorkItemAttemptPanel({
  agentShortLabel,
  attempt,
  attemptStatusLabel,
  checkpoint,
  displayText,
  failedStages,
  failureOperations,
  failureProofCards,
  humanReviewArtifacts,
  humanReviewEvents,
  onApproveCheckpoint,
  onRequestCheckpointChanges,
  operationStatusLabel,
  pipeline,
  pipelineStageClassName,
  pipelineStageLabel
}: WorkItemAttemptPanelProps) {
  if (!attempt) {
    return <p className="muted-copy">No execution attempt yet. Run this item to create a traceable attempt.</p>;
  }

  const stages = attempt.stages?.length ? attempt.stages : pipeline?.run?.stages ?? [];

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
      </header>

      <div className="attempt-stage-flow" aria-label={`Attempt ${attempt.id} stages`}>
        {stages.map((stage) => (
          <AttemptStageCard
            key={`${attempt.id}-${stage.id}`}
            agentShortLabel={agentShortLabel}
            pipelineStageClassName={pipelineStageClassName}
            pipelineStageLabel={pipelineStageLabel}
            stage={stage}
          />
        ))}
      </div>

      {checkpoint ? (
        <HumanGateCard
          attempt={attempt}
          checkpoint={checkpoint}
          displayText={displayText}
          humanReviewArtifacts={humanReviewArtifacts}
          humanReviewEvents={humanReviewEvents}
          onApproveCheckpoint={onApproveCheckpoint}
          onRequestCheckpointChanges={onRequestCheckpointChanges}
        />
      ) : null}

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

function AttemptStageCard({
  agentShortLabel,
  pipelineStageClassName,
  pipelineStageLabel,
  stage
}: Pick<LabelHelpers, "agentShortLabel" | "pipelineStageClassName" | "pipelineStageLabel"> & {
  stage: AttemptStage | PipelineStage;
}) {
  const agentIds = stage.agentIds ?? [];
  const evidence = "evidence" in stage && Array.isArray(stage.evidence) ? stage.evidence : [];
  const artifactLabels = [
    ...(stage.outputArtifacts ?? []),
    ...evidence.map((item: string) => item.split("/").pop() ?? item)
  ];

  return (
    <article className={pipelineStageClassName(stage.status)}>
      <span>{pipelineStageLabel(stage.status)}</span>
      <strong>{stage.title ?? stage.id}</strong>
      <small>{agentIds.length ? agentIds.map(agentShortLabel).join(" + ") : "Agent pending"}</small>
      {artifactLabels.length ? <em>{artifactLabels.slice(0, 2).join(", ")}</em> : null}
    </article>
  );
}

interface HumanGateCardProps {
  attempt: AttemptRecordInfo;
  checkpoint: CheckpointRecordInfo;
  displayText: (value: string) => string;
  humanReviewArtifacts: DetailProofCard[];
  humanReviewEvents: DetailReviewEventCard[];
  onApproveCheckpoint: (checkpointId: string) => void;
  onRequestCheckpointChanges: (checkpointId: string, note?: string) => void;
}

function HumanGateCard({
  attempt,
  checkpoint,
  displayText,
  humanReviewArtifacts,
  humanReviewEvents,
  onApproveCheckpoint,
  onRequestCheckpointChanges
}: HumanGateCardProps) {
  const [reviewNote, setReviewNote] = useState("");
  const defaultRequestChangesNote = "Please address the review notes before delivery.";
  const changedMaterials = humanReviewArtifacts.filter((proof) =>
    /diff|implementation|change|changed|summary|handoff/i.test(`${proof.kind} ${proof.label} ${proof.path ?? ""}`)
  );
  const validationMaterials = humanReviewArtifacts.filter((proof) =>
    /test|check|validation|review/i.test(`${proof.kind} ${proof.label} ${proof.path ?? ""}`)
  );
  const displayMaterials = humanReviewArtifacts.slice(0, 6);

  return (
    <section className="human-gate-card human-review-thread" aria-label="Human review checkpoint">
      <header className="human-review-thread-header">
        <span className="human-review-avatar">Ω</span>
        <div>
          <span>Omega review</span>
          <strong>{checkpoint.title}</strong>
          <p>{checkpoint.summary}</p>
        </div>
        <small>{attempt.status}</small>
      </header>

      <article className="human-review-pr-card">
        <section>
          <h4>PR</h4>
          {attempt.pullRequestUrl ? (
            <a href={attempt.pullRequestUrl} target="_blank" rel="noreferrer">
              {attempt.pullRequestUrl}
            </a>
          ) : (
            <p>PR link will appear after delivery creates the pull request.</p>
          )}
          <div className="human-review-links">
            {attempt.branchName ? <span>Branch {attempt.branchName}</span> : null}
            {attempt.workspacePath ? <code>{attempt.workspacePath}</code> : null}
          </div>
        </section>

        <section>
          <h4>Changed</h4>
          {changedMaterials.length ? (
            <ul className="human-review-bullets">
              {changedMaterials.slice(0, 4).map((proof) => (
                <li key={proof.id}>
                  <ArtifactInline proof={proof} />
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
                  <ArtifactInline proof={proof} />
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
                <ArtifactSummary key={proof.id} proof={proof} />
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
        <span className="human-review-avatar small">H</span>
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
      </div>
    </section>
  );
}

function ArtifactInline({ proof }: { proof: DetailProofCard }) {
  if (proof.url) {
    return (
      <a href={proof.url} target="_blank" rel="noreferrer">
        {proof.label}
      </a>
    );
  }

  return (
    <>
      <span>{proof.label}</span>
      {proof.path ? <code>{proof.path}</code> : null}
    </>
  );
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
  return (
    <section className="attempt-failure-report" aria-label="Attempt failure report">
      <div>
        <span>Failure report</span>
        <strong>{failedStages[0]?.title ?? attempt.currentStageId ?? "Pipeline"} blocked this attempt</strong>
        <p>{attempt.errorMessage ?? attempt.stderrSummary ?? "The run failed before a detailed reason was captured."}</p>
      </div>
      {failureOperations.length ? (
        <div className="failure-agent-list">
          {failureOperations.map((operation) => (
            <article key={operation.id}>
              <span>{operation.stageId ?? "stage"} · {agentShortLabel(operation.agentId ?? "agent")}</span>
              <strong>{operationStatusLabel(operation.status)}</strong>
              {operation.summary ? <p>{operation.summary}</p> : null}
              {operation.runnerProcess?.stderr ? <code>{operation.runnerProcess.stderr.slice(0, 420)}</code> : null}
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
  if (!operations.length) {
    return <p className="muted-copy">Agent trace will appear as soon as the local orchestrator starts assigning stage work.</p>;
  }

  return (
    <div className="agent-trace-list">
      {operations.map((operation) => (
        <article key={operation.id} className={`agent-trace-row ${pipelineStageClassName(operation.status)}`}>
          <header>
            <div>
              <span>{operation.stageId ?? "stage"}</span>
              <strong>{agentShortLabel(operation.agentId ?? "agent")}</strong>
            </div>
            <small>{operationStatusLabel(operation.status)}</small>
          </header>
          {operation.summary ? <p>{operation.summary}</p> : null}
          {operation.prompt ? <code>{operation.prompt.slice(0, 260)}</code> : null}
          {operation.runnerProcess ? (
            <div className="agent-runner-meta">
              <span>{operation.runnerProcess.runner ?? "runner"}</span>
              {operation.runnerProcess.status ? <span>{operation.runnerProcess.status}</span> : null}
              {typeof operation.runnerProcess.exitCode === "number" ? <span>exit {operation.runnerProcess.exitCode}</span> : null}
              {typeof operation.runnerProcess.durationMs === "number" ? <span>{operation.runnerProcess.durationMs}ms</span> : null}
            </div>
          ) : null}
        </article>
      ))}
    </div>
  );
}

export function ArtifactGrid({ proofs }: { proofs: DetailProofCard[] }) {
  if (!proofs.length) {
    return <p className="muted-copy">No artifact has been collected yet.</p>;
  }

  return (
    <div className="proof-grid">
      {proofs.map((proof) => (
        <article key={proof.id} className="proof-card">
          <span className="proof-kind">{proof.kind}</span>
          <strong>{proof.label}</strong>
          {proof.stage ? <small>{proof.stage}</small> : null}
          {proof.url ? (
            <a href={proof.url} target="_blank" rel="noreferrer">
              Open URL
            </a>
          ) : proof.path ? (
            <code>{proof.path}</code>
          ) : null}
        </article>
      ))}
    </div>
  );
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
            <time>{attempt.finishedAt ?? attempt.startedAt ?? ""}</time>
          </article>
        ))
      )}
    </div>
  );
}

function ArtifactSummary({
  openLabel = "Open",
  proof
}: {
  openLabel?: string;
  proof: DetailProofCard;
}) {
  return (
    <article>
      <span>{proof.kind}</span>
      <strong>{proof.label}</strong>
      {proof.stage ? <small>{proof.stage}</small> : null}
      {proof.url ? (
        <a href={proof.url} target="_blank" rel="noreferrer">{openLabel}</a>
      ) : proof.path ? (
        <code>{proof.path}</code>
      ) : null}
    </article>
  );
}
