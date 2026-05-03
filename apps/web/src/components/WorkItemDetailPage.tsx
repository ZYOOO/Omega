import type { FormEvent, ReactNode } from "react";
import { useEffect, useMemo, useState } from "react";
import { retryReasonForAttempt } from "../attemptRetryReason";
import type { RepositoryTarget, WorkItem, WorkItemStatus } from "../core";
import type {
  AttemptRecordInfo,
  AttemptActionPlanInfo,
  AttemptTimelineInfo,
  CheckpointRecordInfo,
  GitHubPullRequestStatusResult,
  OperationRecordInfo,
  PatchRunWorkpadInput,
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
  preview: string;
  body: ReactNode;
  tone?: "default" | "warning" | "success";
};

type WorkpadSourceRecord = {
  kind: string;
  label: string;
  message: string;
  path?: string;
  line?: string;
  state?: string;
  url?: string;
};

type WorkpadPatchHistoryEntry = {
  id: string;
  updatedAt: string;
  updatedBy: string;
  fields: string[];
  reason: string;
  sourceLabel: string;
};

type WorkpadEditableField = "validation" | "notes" | "blockers" | "reviewFeedback" | "retryReason" | "reworkChecklist" | "reworkAssessment";

type WorkpadEditableFieldOption = {
  id: WorkpadEditableField;
  label: string;
  description: string;
  placeholder: string;
};

const WORKPAD_EDITABLE_FIELDS: WorkpadEditableFieldOption[] = [
  {
    id: "notes",
    label: "Notes",
    description: "Capture operator observations, review context, or follow-up notes.",
    placeholder: "One note per line"
  },
  {
    id: "blockers",
    label: "Blockers",
    description: "Record the real reasons still blocking delivery.",
    placeholder: "One blocker per line"
  },
  {
    id: "reviewFeedback",
    label: "Review Feedback",
    description: "Archive feedback that should be reused by rework.",
    placeholder: "One feedback item per line"
  },
  {
    id: "retryReason",
    label: "Retry Reason",
    description: "State why retry is needed in product terms.",
    placeholder: "Why this attempt needs retry"
  },
  {
    id: "validation",
    label: "Validation",
    description: "Add validation status, commands, or manual inspection results.",
    placeholder: "Validation status, command, or conclusion"
  },
  {
    id: "reworkChecklist",
    label: "Rework Checklist",
    description: "Capture the checklist that the next rework should execute.",
    placeholder: "One rework item per line"
  },
  {
    id: "reworkAssessment",
    label: "Rework Assessment",
    description: "Record whether rework should be quick-fix or replanned.",
    placeholder: "Assessment and reason"
  }
];

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
  attemptActionPlan?: AttemptActionPlanInfo | null;
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
  onOpenPagePilot: () => void;
  onApproveCheckpoint: (checkpointId: string) => void;
  onPatchRunWorkpad?: (runWorkpadId: string, input: PatchRunWorkpadInput) => Promise<void>;
  onRequestCheckpointChanges: (checkpointId: string, note?: string) => void;
  onRetryAttempt: (attemptId: string) => void;
}

export function WorkItemDetailPage({
  agentShortLabel,
  attemptStatusLabel,
  attemptTimeline,
  attemptActionPlan,
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
  onOpenPagePilot,
  onApproveCheckpoint,
  onPatchRunWorkpad,
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
          <div className="issue-detail-actions">
            <button type="button" onClick={onOpenPagePilot} disabled={!workItem.repositoryTargetId}>
              Open in Page Pilot
            </button>
          </div>
        </header>

        <section className="issue-detail-section detail-flow-priority">
          <h3>Delivery flow</h3>
          <DeliveryFlowGrid
            actionPlan={attemptActionPlan}
            agentShortLabel={agentShortLabel}
            pipeline={pipeline}
            pipelineStageClassName={pipelineStageClassName}
            pipelineStageLabel={pipelineStageLabel}
          />
          <ReworkReturnSignal actionPlan={attemptActionPlan} attempt={attempt} pipeline={pipeline} runWorkpad={runWorkpad} />
        </section>

        <RunWorkpad onPatch={onPatchRunWorkpad} record={runWorkpad} sections={workpadSections} />

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
            actionPlan={attemptActionPlan}
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

function RunWorkpad({
  onPatch,
  record,
  sections
}: {
  onPatch?: (runWorkpadId: string, input: PatchRunWorkpadInput) => Promise<void>;
  record?: RunWorkpadRecordInfo;
  sections: WorkpadSection[];
}) {
  const [activeSectionId, setActiveSectionId] = useState<string | null>(null);
  const [editorOpen, setEditorOpen] = useState(false);
  const [selectedField, setSelectedField] = useState<WorkpadEditableField>("notes");
  const [draftValue, setDraftValue] = useState("");
  const [draftReason, setDraftReason] = useState("");
  const [patchError, setPatchError] = useState("");
  const [patchSaving, setPatchSaving] = useState(false);
  const activeSection = sections.find((section) => section.id === activeSectionId);
  const selectedOption = WORKPAD_EDITABLE_FIELDS.find((field) => field.id === selectedField) ?? WORKPAD_EDITABLE_FIELDS[0];
  const canEdit = Boolean(record && onPatch);

  useEffect(() => {
    if (!editorOpen) return;
    setDraftValue(workpadFieldToDraft(record?.workpad, selectedField));
    setPatchError("");
  }, [editorOpen, record?.id, record?.updatedAt, selectedField]);

  async function submitPatch(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!record || !onPatch) return;
    setPatchSaving(true);
    setPatchError("");
    try {
      await onPatch(record.id, {
        workpad: buildWorkpadFieldPatch(selectedField, draftValue, record.workpad),
        updatedBy: "operator",
        reason: draftReason.trim() || `Operator edited ${selectedOption.label}.`,
        source: {
          kind: "ui",
          label: "Run Workpad editor",
          field: selectedField,
          attemptId: record.attemptId,
          workItemId: record.workItemId
        }
      });
      setEditorOpen(false);
      setDraftReason("");
    } catch (error) {
      setPatchError(error instanceof Error ? error.message : "Failed to patch Run Workpad.");
    } finally {
      setPatchSaving(false);
    }
  }

  return (
    <section className="run-workpad" aria-label="Run workpad">
      <header>
        <div>
          <span className="section-label">Run workpad</span>
          <h3>Execution brief</h3>
        </div>
        <div className="run-workpad-actions">
          <small>{workpadSignalSummary(sections)}</small>
          {canEdit ? (
            <button
              type="button"
              onClick={() => {
                setActiveSectionId(null);
                setEditorOpen(true);
              }}
            >
              Edit fields
            </button>
          ) : null}
        </div>
      </header>
      <div className="run-workpad-grid">
        {sections.map((section) => (
          <button
            key={section.id}
            type="button"
            className={section.tone ? `workpad-card workpad-${section.tone}` : "workpad-card"}
            onClick={() => setActiveSectionId(section.id)}
          >
            <span>{section.label}</span>
            <strong>{section.title}</strong>
            <p>{section.preview}</p>
          </button>
        ))}
      </div>
      {activeSection ? (
        <section className="detail-popover-backdrop" role="presentation" onClick={() => setActiveSectionId(null)}>
          <article
            className="detail-popover"
            role="dialog"
            aria-modal="true"
            aria-label={`${activeSection.label} detail`}
            onClick={(event) => event.stopPropagation()}
          >
            <header>
              <div>
                <span>{activeSection.label}</span>
                <strong>{activeSection.title}</strong>
              </div>
              <button type="button" onClick={() => setActiveSectionId(null)}>Close</button>
            </header>
            <div className="detail-popover-body">{activeSection.body}</div>
          </article>
        </section>
      ) : null}
      {editorOpen ? (
        <section className="detail-popover-backdrop" role="presentation" onClick={() => setEditorOpen(false)}>
          <article
            className="detail-popover workpad-edit-popover"
            role="dialog"
            aria-modal="true"
            aria-label="Edit Run Workpad field"
            onClick={(event) => event.stopPropagation()}
          >
            <header>
              <div>
                <span>Run Workpad patch</span>
                <strong>Field editor</strong>
              </div>
              <button type="button" onClick={() => setEditorOpen(false)}>Close</button>
            </header>
            <form className="workpad-edit-form" onSubmit={submitPatch}>
              <label>
                <span>Field</span>
                <select
                  value={selectedField}
                  onChange={(event) => setSelectedField(event.target.value as WorkpadEditableField)}
                >
                  {WORKPAD_EDITABLE_FIELDS.map((field) => (
                    <option key={field.id} value={field.id}>{field.label}</option>
                  ))}
                </select>
              </label>
              <p>{selectedOption.description}</p>
              <label>
                <span>Patch value</span>
                <textarea
                  value={draftValue}
                  placeholder={selectedOption.placeholder}
                  onChange={(event) => setDraftValue(event.target.value)}
                />
              </label>
              <label>
                <span>Reason</span>
                <input
                  value={draftReason}
                  placeholder="Why this field is being patched"
                  onChange={(event) => setDraftReason(event.target.value)}
                />
              </label>
              {patchError ? <p className="workpad-edit-error">{patchError}</p> : null}
              <div className="workpad-edit-actions">
                <button type="button" onClick={() => setEditorOpen(false)} disabled={patchSaving}>Cancel</button>
                <button type="submit" disabled={patchSaving || !record}>
                  {patchSaving ? "Saving..." : "Save patch"}
                </button>
              </div>
            </form>
          </article>
        </section>
      ) : null}
    </section>
  );
}

function workpadFieldToDraft(workpad: RunWorkpadRecordInfo["workpad"] | undefined, field: WorkpadEditableField): string {
  if (!workpad) return "";
  if (field === "notes" || field === "blockers" || field === "reviewFeedback") {
    const value = workpad[field];
    return Array.isArray(value) ? value.join("\n") : "";
  }
  if (field === "retryReason") return workpad.retryReason ?? "";
  if (field === "validation") {
    const validation = recordValue(workpad.validation) ?? {};
    return [recordString(validation, "summary"), recordString(validation, "message")].filter(Boolean).join("\n");
  }
  if (field === "reworkChecklist") {
    const checklist = recordValue(workpad.reworkChecklist) ?? {};
    const items = asStringArray(checklist.checklist);
    return items.length ? items.join("\n") : recordString(checklist, "retryReason");
  }
  const assessment = recordValue(workpad.reworkAssessment) ?? {};
  return [recordString(assessment, "strategy"), recordString(assessment, "reason")].filter(Boolean).join("\n");
}

function buildWorkpadFieldPatch(
  field: WorkpadEditableField,
  draftValue: string,
  workpad: RunWorkpadRecordInfo["workpad"]
): Partial<RunWorkpadRecordInfo["workpad"]> {
  const trimmed = draftValue.trim();
  if (field === "notes" || field === "blockers" || field === "reviewFeedback") {
    return { [field]: linesFromDraft(draftValue) };
  }
  if (field === "retryReason") return { retryReason: trimmed };
  if (field === "validation") {
    return {
      validation: {
        ...(recordValue(workpad.validation) ?? {}),
        status: "operator-note",
        summary: trimmed || "Operator cleared the validation note."
      }
    };
  }
  if (field === "reworkChecklist") {
    return {
      reworkChecklist: {
        ...(recordValue(workpad.reworkChecklist) ?? {}),
        status: "operator-patched",
        checklist: linesFromDraft(draftValue)
      }
    };
  }
  return {
    reworkAssessment: {
      ...(recordValue(workpad.reworkAssessment) ?? {}),
      strategy: "operator-assessment",
      reason: trimmed
    }
  };
}

function linesFromDraft(value: string): string[] {
  return value.split(/\r?\n/).map((line) => line.trim()).filter(Boolean);
}

function DeliveryFlowGrid({
  actionPlan,
  agentShortLabel,
  pipeline,
  pipelineStageClassName,
  pipelineStageLabel
}: Pick<DetailHelpers, "agentShortLabel" | "pipelineStageClassName" | "pipelineStageLabel"> & {
  actionPlan?: AttemptActionPlanInfo | null;
  pipeline?: PipelineRecordInfo;
}) {
  const planStates = actionPlan?.states?.length ? actionPlan.states : [];
  const stages = planStates.length ? planStates : pipeline?.run?.stages ?? [];
  if (!stages.length) {
    return <p className="muted-copy">Delivery stages will appear after a pipeline is created.</p>;
  }
  return (
    <div className="detail-stage-grid">
      {stages.map((stage, index) => {
        const stageRecord = stage as Record<string, unknown>;
        const agent = recordString(stageRecord, "agent") || recordString(stageRecord, "agentId");
        const agentIds = Array.isArray(stageRecord.agentIds) ? stageRecord.agentIds.map(String) : agent ? [agent] : [];
        const status = recordString(stageRecord, "status");
        return (
          <article key={recordString(stageRecord, "id") || String(index)} className={pipelineStageClassName(status)}>
            <span>{index + 1}</span>
            <div>
              <strong>{recordString(stageRecord, "title") || recordString(stageRecord, "id")}</strong>
              <small>{agentIds.length ? agentIds.map(agentShortLabel).join(" + ") : "Agent pending"}</small>
            </div>
            <em>{pipelineStageLabel(status)}</em>
          </article>
        );
      })}
    </div>
  );
}

function ReworkReturnSignal({
  actionPlan,
  attempt,
  pipeline,
  runWorkpad
}: {
  actionPlan?: AttemptActionPlanInfo | null;
  attempt?: AttemptRecordInfo;
  pipeline?: PipelineRecordInfo;
  runWorkpad?: RunWorkpadRecordInfo;
}) {
  const rejectedEvent = pipeline?.run?.events?.find((event) => /rejected|changes requested|request changes/i.test(`${event.type ?? ""} ${event.message ?? ""}`));
  const assessment = recordValue(runWorkpad?.workpad?.reworkAssessment) || recordValue(attempt?.reworkAssessment);
  const checklist = recordValue(runWorkpad?.workpad?.reworkChecklist) || recordValue(attempt?.reworkChecklist);
  const retry = recordValue(actionPlan?.retry);
  const checklistItems = asStringArray(checklist?.checklist);
  const retryAvailable = recordBool(retry, "available");
  const isRetryableAttempt = attempt ? ["failed", "stalled", "canceled"].includes(attempt.status) : false;
  const hasFeedbackRoute = Boolean(
    rejectedEvent ||
    attempt?.humanChangeRequest ||
    recordString(assessment, "strategy") ||
    retryAvailable ||
    isRetryableAttempt
  );
  if (!hasFeedbackRoute) {
    return null;
  }
  const route = recordString(assessment, "strategy") || "rework";
  const reason =
    recordString(assessment, "rationale") ||
    (retryAvailable || isRetryableAttempt ? recordString(checklist, "retryReason") : "") ||
    recordString(retry, "reason") ||
    attempt?.humanChangeRequest ||
    rejectedEvent?.message ||
    "Human or review feedback will be routed into rework before returning to review.";
  return (
    <aside className="rework-return-signal" aria-label="Rework return signal">
      <span>Feedback route</span>
      <strong>{reworkStrategyLabel(route)}</strong>
      <p>{shortText(reason, 180)}</p>
      {checklistItems.length ? <small>{checklistItems.length} checklist action{checklistItems.length === 1 ? "" : "s"} captured for the next run.</small> : null}
    </aside>
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
  const recordedBlockers = asStringArray(workpad?.blockers).filter((blocker) => {
    if (checkpoint?.status === "pending" && /human review|人工|审批/i.test(blocker)) return false;
    return true;
  });
  const checkpointBlocker = checkpoint && checkpoint.status !== "pending" ? `${checkpoint.title}: ${checkpoint.summary}` : "";
  const blockers = recordedBlockers.length ? recordedBlockers : [
    attempt?.failureReason,
    attempt?.errorMessage,
    checkpointBlocker,
    pullRequestStatus?.deliveryGate && pullRequestStatus.deliveryGate !== "passed" ? `PR gate: ${pullRequestStatus.deliveryGate}` : ""
  ].filter(Boolean) as string[];
  const recordedFeedback = asStringArray(workpad?.reviewFeedback);
  const prFeedback = [
    ...feedbackRecordsToStrings(attempt?.pullRequestFeedback),
    ...feedbackRecordsToStrings(pullRequestStatus?.reviewFeedback)
  ];
  const checkFeedback = [
    ...feedbackRecordsToStrings(attempt?.checkLogFeedback),
    ...feedbackRecordsToStrings(pullRequestStatus?.checkLogFeedback)
  ];
  const reviewFeedback =
    recordedFeedback[0] ||
    attempt?.failureReviewFeedback ||
    prFeedback[0] ||
    checkFeedback[0] ||
    reviewOps.map((operation) => operation.summary || operation.runnerProcess?.stderr || "").find(Boolean) ||
    reviewEvents.map((event) => event.message).find(Boolean);
  const isRetryableAttempt = attempt ? ["failed", "stalled", "canceled"].includes(attempt.status) : false;
  const retryReason = isRetryableAttempt ? (workpad?.retryReason || retryReasonForAttempt(attempt)) : "";
  const acceptanceCriteria = asStringArray(workpad?.acceptanceCriteria);
  const criteria = acceptanceCriteria.length
    ? acceptanceCriteria
    : (workItem.acceptanceCriteria.length ? workItem.acceptanceCriteria : requirement?.acceptanceCriteria ?? ["No acceptance criteria captured."]);
  const validationStatus = recordString(workpad?.validation, "status");
  const runtimeReworkChecklist = recordValue(workpad?.reworkChecklist) || recordValue(attempt?.reworkChecklist);
  const runtimeReworkChecklistItems = asStringArray(runtimeReworkChecklist?.checklist);
  const runtimeReworkSources = sourceRecordsFromValue(runtimeReworkChecklist?.sources);
  const reviewPacket = recordValue(workpad?.reviewPacket) || recordValue(attempt?.reviewPacket);
  const reworkAssessment = recordValue(workpad?.reworkAssessment) || recordValue(attempt?.reworkAssessment);
  const reworkStrategy = recordString(reworkAssessment, "strategy");
  const reworkChecklist = asStringArray(recordValue(reworkAssessment)?.checklist);
  const patchHistory = patchHistoryFromRecord(runWorkpad);
  const rejectedEvent = pipeline?.run?.events?.find((event) => /rejected|changes requested|request changes/i.test(`${event.type ?? ""} ${event.message ?? ""}`));
  const hasFeedbackRoute = Boolean(rejectedEvent || attempt?.humanChangeRequest || reworkStrategy || isRetryableAttempt);
  const shouldShowReworkChecklist = hasFeedbackRoute && runtimeReworkChecklistItems.length > 0;
  const sections: WorkpadSection[] = [
    ...(shouldShowReworkChecklist
      ? [{
          id: "rework-checklist",
          label: "Rework checklist",
          title: `${runtimeReworkChecklistItems.length} action${runtimeReworkChecklistItems.length === 1 ? "" : "s"}`,
          preview: shortText(runtimeReworkChecklistItems[0] ?? recordString(runtimeReworkChecklist, "retryReason"), 120),
          tone: "warning" as const,
          body: (
            <div className="workpad-rework-assessment">
              {recordString(runtimeReworkChecklist, "retryReason") ? <p>{shortText(recordString(runtimeReworkChecklist, "retryReason"), 280)}</p> : null}
              <ul>
                {runtimeReworkChecklistItems.slice(0, 5).map((item) => <li key={item}>{item}</li>)}
              </ul>
              {runtimeReworkSources.length ? <ChecklistSourceList sources={runtimeReworkSources} /> : null}
            </div>
          )
        }]
      : []),
    ...(reworkStrategy
      ? [{
          id: "rework-assessment",
          label: "Rework assessment",
          title: reworkStrategyLabel(reworkStrategy),
          preview: shortText(recordString(reworkAssessment, "rationale") || recordString(reworkAssessment, "humanFeedback") || "Rework route selected.", 120),
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
    ...(patchHistory.length
      ? [{
          id: "patch-history",
          label: "Patch history",
          title: `${patchHistory.length} update${patchHistory.length === 1 ? "" : "s"}`,
          preview: patchHistory[0] ? `${patchHistory[0].updatedBy} updated ${patchHistory[0].fields.join(", ")}` : "No field patch recorded.",
          body: <WorkpadPatchHistory entries={patchHistory.slice(0, 5)} />
        }]
      : []),
    ...(reviewPacket
      ? [{
          id: "review-packet",
          label: "Review packet",
          title: reviewPacketTitle(reviewPacket),
          preview: shortText(recordString(reviewPacket, "summary") || reviewPacketFirstAction(reviewPacket) || "Diff, validation, checks and risk preview.", 120),
          tone: reviewPacketTone(reviewPacket),
          body: <ReviewPacketPreview packet={reviewPacket} />
        }]
      : []),
    {
      id: "plan",
      label: "Plan",
      title: recordString(workpad?.plan, "currentStageId") || planArtifacts[0]?.label || pipeline?.templateId || "Plan pending",
      preview: planArtifacts.length
        ? `${planArtifacts.length} plan artifact${planArtifacts.length === 1 ? "" : "s"} captured.`
        : shortText(requirement?.rawText ?? workItem.description, 120),
      body: planArtifacts.length ? <ArtifactList artifacts={planArtifacts.slice(0, 3)} /> : <p>{shortText(requirement?.rawText ?? workItem.description)}</p>
    },
    {
      id: "acceptance",
      label: "Acceptance criteria",
      title: `${criteria.length} criteria`,
      preview: shortText(criteria[0] ?? "No acceptance criteria captured.", 120),
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
      preview: validationOps[0]?.summary
        ? shortText(validationOps[0].summary, 120)
        : validationStatus
          ? `Validation is ${validationStatus}.`
          : "Waiting for test reports or checks.",
      tone: validationStatus === "passed" || pullRequestStatus?.deliveryGate === "passed" ? "success" : undefined,
      body: validationOps.length ? <OperationSummaryList operations={validationOps.slice(-3)} /> : <p>Test reports and checks will appear here after validation runs.</p>
    },
    {
      id: "pr",
      label: "PR",
      title: attempt?.pullRequestUrl ? "Pull request ready" : attempt?.branchName ? "Branch ready" : "Not created",
      preview: attempt?.pullRequestUrl || attempt?.branchName || "No delivery branch or pull request yet.",
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
      preview: reviewFeedback ? shortText(reviewFeedback, 120) : "No review, PR, or human feedback captured.",
      body: reviewFeedback ? <p>{shortText(reviewFeedback, 360)}</p> : <p>Review agent, PR comments and human requested changes will be merged here.</p>
    },
    {
      id: "blockers",
      label: "Blockers",
      title: blockers.length ? `${blockers.length} active signal${blockers.length === 1 ? "" : "s"}` : "No active blockers",
      preview: blockers.length ? shortText(blockers[0], 120) : "No blocking failure is recorded.",
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
      preview: retryReason ? shortText(retryReason, 120) : "Current run has no retry reason.",
      body: retryReason ? <p>{shortText(retryReason, 420)}</p> : <p>Retry will reuse the captured blocker and review feedback when needed.</p>
    },
    {
      id: "notes",
      label: "Notes",
      title: `${operations.length} operation${operations.length === 1 ? "" : "s"}`,
      preview: operations.length ? shortText(operations[operations.length - 1]?.summary ?? "Latest operation recorded.", 120) : "No operation notes yet.",
      body: operations.length ? <OperationSummaryList operations={operations.slice(-3)} /> : <p>Agent notes will appear as operations are recorded.</p>
    }
  ];

  return sections;
}

function workpadSignalSummary(sections: WorkpadSection[]): string {
  const blocker = sections.find((section) => section.id === "blockers" && section.tone === "warning");
  const retry = sections.find((section) => section.id === "retry" && !/No retry/i.test(section.title));
  const feedback = sections.find((section) => section.id === "feedback" && !/No feedback/i.test(section.title));
  if (blocker) return blocker.title;
  if (retry) return retry.title;
  if (feedback) return feedback.title;
  return `${sections.length} signals`;
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

function ReviewPacketPreview({ packet }: { packet: Record<string, unknown> }) {
  const diff = recordValue(packet.diffPreview);
  const test = recordValue(packet.testPreview);
  const checks = recordValue(packet.checkPreview);
  const risk = recordValue(packet.risk);
  const actions = arrayRecords(packet.recommendedActions);
  const changedFiles = asStringArray(diff?.changedFiles);
  return (
    <div className="review-packet-preview" aria-label="Review packet preview">
      <p>{recordString(packet, "summary") || "Review packet is ready for human review."}</p>
      <div className="review-packet-grid">
        <article>
          <span>Diff</span>
          <strong>{recordString(diff, "summary") || `${changedFiles.length} changed file${changedFiles.length === 1 ? "" : "s"}`}</strong>
          {changedFiles.length ? (
            <ul>{changedFiles.slice(0, 6).map((file) => <li key={file}>{file}</li>)}</ul>
          ) : (
            <small>No changed files captured.</small>
          )}
        </article>
        <article>
          <span>Tests</span>
          <strong>{recordString(test, "status") || "unknown"}</strong>
          <small>{recordString(test, "summary") || "No validation preview captured."}</small>
        </article>
        <article>
          <span>Checks</span>
          <strong>{recordString(checks, "status") || "unknown"}</strong>
          <small>{recordString(checks, "summary") || "No check preview captured."}</small>
        </article>
        <article className={`review-packet-risk review-packet-risk-${recordString(risk, "level") || "low"}`}>
          <span>Risk</span>
          <strong>{recordString(risk, "level") || "low"}</strong>
          <ul>{asStringArray(risk?.reasons).slice(0, 4).map((reason) => <li key={reason}>{reason}</li>)}</ul>
        </article>
      </div>
      {actions.length ? (
        <div className="review-packet-actions">
          <span>Next actions</span>
          <ul>
            {actions.slice(0, 5).map((action, index) => {
              const label = recordString(action, "label") || recordString(action, "type") || `Action ${index + 1}`;
              const url = recordString(action, "url");
              return <li key={`${label}-${index}`}>{url ? <a href={url} target="_blank" rel="noreferrer">{label}</a> : label}</li>;
            })}
          </ul>
        </div>
      ) : null}
      {recordString(diff, "patchExcerpt") ? (
        <pre className="review-packet-diff">{shortText(recordString(diff, "patchExcerpt"), 1800)}</pre>
      ) : null}
    </div>
  );
}

function reviewPacketTitle(packet: Record<string, unknown>): string {
  const risk = recordString(recordValue(packet.risk), "level") || "low";
  const diff = recordValue(packet.diffPreview);
  const count = numberValue(diff?.fileCount) || asStringArray(diff?.changedFiles).length;
  return `${risk} risk · ${count} file${count === 1 ? "" : "s"}`;
}

function reviewPacketTone(packet: Record<string, unknown>): WorkpadSection["tone"] {
  const risk = recordString(recordValue(packet.risk), "level");
  if (risk === "high" || risk === "medium") return "warning";
  return "success";
}

function reviewPacketFirstAction(packet: Record<string, unknown>): string {
  const action = arrayRecords(packet.recommendedActions)[0];
  return action ? recordString(action, "label") : "";
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

function ChecklistSourceList({ sources }: { sources: WorkpadSourceRecord[] }) {
  return (
    <div className="workpad-source-list" aria-label="Checklist sources">
      <span>Sources</span>
      {sources.slice(0, 6).map((source, index) => (
        <article key={`${source.kind}:${source.label}:${index}`}>
          <div>
            <strong>{sourceLabel(source.kind)}</strong>
            <small>{[source.label, source.state, source.path ? `${source.path}${source.line ? `:${source.line}` : ""}` : ""].filter(Boolean).join(" · ")}</small>
          </div>
          <p>{shortText(source.message, 220)}</p>
          {source.url ? <a href={source.url} target="_blank" rel="noreferrer">Open source</a> : null}
        </article>
      ))}
    </div>
  );
}

function WorkpadPatchHistory({ entries }: { entries: WorkpadPatchHistoryEntry[] }) {
  return (
    <div className="workpad-patch-history" aria-label="Workpad patch history">
      {entries.map((entry) => (
        <article key={entry.id}>
          <div>
            <strong>{entry.updatedBy}</strong>
            <span>{entry.fields.join(", ")}</span>
          </div>
          {entry.reason ? <p>{shortText(entry.reason, 180)}</p> : null}
          <small>{[entry.sourceLabel, formatTimestamp(entry.updatedAt)].filter(Boolean).join(" · ")}</small>
        </article>
      ))}
    </div>
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

function arrayRecords(value: unknown): Array<Record<string, unknown>> {
  return Array.isArray(value) ? value.flatMap((item) => {
    const record = recordValue(item);
    return record ? [record] : [];
  }) : [];
}

function feedbackRecordsToStrings(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.flatMap((entry) => {
    const record = recordValue(entry);
    if (!record) return [];
    const message = typeof record.message === "string" ? record.message.trim() : "";
    if (!message) return [];
    const label = typeof record.label === "string" && record.label.trim() ? `${record.label.trim()}: ` : "";
    return `${label}${message}`;
  });
}

function sourceRecordsFromValue(value: unknown): WorkpadSourceRecord[] {
  if (!Array.isArray(value)) return [];
  return value.flatMap((entry) => {
    const record = recordValue(entry);
    if (!record) return [];
    const message = typeof record.message === "string" ? record.message.trim() : "";
    if (!message) return [];
    return [{
      kind: typeof record.kind === "string" && record.kind.trim() ? record.kind.trim() : "source",
      label: typeof record.label === "string" && record.label.trim() ? record.label.trim() : "Captured source",
      message,
      path: recordString(record, "path") || undefined,
      line: recordString(record, "line") || undefined,
      state: recordString(record, "state") || undefined,
      url: recordString(record, "sourceUrl") || recordString(record, "url") || undefined
    }];
  });
}

function patchHistoryFromRecord(record?: RunWorkpadRecordInfo): WorkpadPatchHistoryEntry[] {
  if (!Array.isArray(record?.fieldPatchHistory)) return [];
  return record.fieldPatchHistory.flatMap((entry, index) => {
    const value = recordValue(entry);
    if (!value) return [];
    const source = recordValue(value.source);
    const fields = asStringArray(value.fields);
    return [{
      id: recordString(value, "id") || `${record.id}:patch:${index}`,
      updatedAt: recordString(value, "updatedAt"),
      updatedBy: recordString(value, "updatedBy") || "unknown",
      fields: fields.length ? fields : ["workpad"],
      reason: recordString(value, "reason") || recordString(source, "reason"),
      sourceLabel: recordString(source, "label") || recordString(source, "kind") || "api"
    }];
  }).sort((left, right) => right.updatedAt.localeCompare(left.updatedAt));
}

function recordString(value: unknown, key: string): string {
  if (!value || typeof value !== "object" || Array.isArray(value)) return "";
  const raw = (value as Record<string, unknown>)[key];
  return typeof raw === "string" ? raw : "";
}

function recordBool(value: unknown, key: string): boolean {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  return (value as Record<string, unknown>)[key] === true;
}

function formatTimestamp(value: string): string {
  if (!value) return "";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false
  }).format(parsed);
}

function recordValue(value: unknown): Record<string, unknown> | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
  return value as Record<string, unknown>;
}

function numberValue(value: unknown): number {
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
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

function sourceLabel(kind: string): string {
  switch (kind) {
    case "human":
      return "Human";
    case "review":
      return "Review";
    case "pr-review":
      return "PR review";
    case "pr-comment":
      return "PR comment";
    case "ci-check-log":
      return "Check log";
    case "delivery-gate":
      return "Gate";
    case "runner":
      return "Runner";
    case "operation":
      return "Operation";
    case "event":
      return "Event";
    default:
      return kind;
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
