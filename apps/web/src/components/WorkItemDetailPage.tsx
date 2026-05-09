import type { FormEvent, ReactNode } from "react";
import { useEffect, useMemo, useState } from "react";
import { retryReasonForAttempt } from "../attemptRetryReason";
import type { RepositoryTarget, WorkItem, WorkItemStatus } from "../core";
import { useI18n } from "../i18n";
import type {
  AttemptRecordInfo,
  AttemptActionPlanInfo,
  AttemptTimelineInfo,
  CheckpointRecordInfo,
  GitHubPullRequestStatusResult,
  OperationRecordInfo,
  PatchRunWorkpadInput,
  PipelineRecordInfo,
  ProofPreviewInfo,
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

type StageAgentRunSummary = {
  launchedCount: number;
  agents: string[];
  details: string[];
  operationDetails: StageAgentRunDetail[];
  eventCount: number;
  totalDurationMs: number;
  durationSampleCount: number;
  totalTokens: number;
  inputTokens: number;
  outputTokens: number;
  statusCounts: Record<string, number>;
};

type StageAgentRunDetail = {
  id: string;
  stageId: string;
  agentId?: string;
  agentLabel: string;
  roleLabel: string;
  status: string;
  runner?: string;
  provider?: string;
  model?: string;
  processStatus?: string;
  durationMs?: number;
  tokenUsage?: AgentTokenUsage;
  summary: string;
  startedAt?: string;
  finishedAt?: string;
  source: "operation" | "event";
};

type AgentTokenUsage = {
  input?: number;
  output?: number;
  total: number;
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
  onFetchProofPreview?: (proofId: string) => Promise<ProofPreviewInfo>;
  onPatchRunWorkpad?: (runWorkpadId: string, input: PatchRunWorkpadInput) => Promise<void>;
  onRequestCheckpointChanges: (checkpointId: string, note?: string) => void;
  onRetryAttempt: (attemptId: string) => void;
}

function visibleExternalReference(value?: string): string {
  const raw = value?.trim() ?? "";
  if (!raw) return "";
  if (/^(?:page-pilot:)?item_(?:manual|page_pilot)_/i.test(raw)) return "";
  if (/^req_item_manual_/i.test(raw)) return "";
  if (/^pipeline_item_(?:manual|page_pilot)_/i.test(raw)) return "";
  return raw;
}

function isPagePilotWorkItem(item: WorkItem): boolean {
  return item.source === "page_pilot" || item.labels.includes("page-pilot") || item.sourceExternalRef?.startsWith("page-pilot:") === true;
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
  onFetchProofPreview,
  onPatchRunWorkpad,
  onRequestCheckpointChanges,
  onRetryAttempt
}: WorkItemDetailPageProps) {
  const { t } = useI18n();
  const requirement = workItem.requirementId
    ? requirements.find((candidate) => candidate.id === workItem.requirementId)
    : undefined;
  const siblingItems = requirement ? workItems.filter((item) => item.requirementId === requirement.id) : [];
  const attempt = attempts[0];
  const runWorkpad = useMemo(
    () => latestRunWorkpadForDetail({ attempt, pipeline, runWorkpads, workItem }),
    [attempt, pipeline, runWorkpads, workItem]
  );
  const humanReviewCheckpoints = useMemo(() => {
    if (!pipeline) return [];
    return checkpoints
      .filter((candidate) => {
        if (candidate.pipelineId !== pipeline.id) return false;
        if (!isHumanReviewCheckpoint(candidate)) return false;
        if (attempt?.id && candidate.attemptId && candidate.attemptId !== attempt.id) return false;
        return true;
      })
      .sort((left, right) => checkpointTime(right) - checkpointTime(left));
  }, [attempt?.id, checkpoints, pipeline]);
  const checkpoint = displayCheckpointForCurrentRun(humanReviewCheckpoints[0], pipeline, attempt);
  const checkpointActionable =
    checkpoint?.status === "pending" &&
    (attempt ? attempt.status === "waiting-human" && attempt.currentStageId === "human_review" : true) &&
    (pipeline ? pipeline.status === "waiting-human" : true);
  const detailOperations = useMemo(
    () => operationsForWorkItem({ attempt, operations, pipeline, workItem }),
    [attempt, operations, pipeline, workItem]
  );
  const stageAgentRuns = useMemo(
    () => summarizeStageAgentRuns({ agentShortLabel, attempt, operations: detailOperations, pipeline }),
    [agentShortLabel, attempt, detailOperations, pipeline]
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
        actionPlan: attemptActionPlan,
        attempt,
        checkpoint,
        checkpointActionable,
        operations: detailOperations,
        pipeline,
        proofCards,
        pullRequestStatus,
        requirement,
        reviewEvents,
        runWorkpad,
        t,
        workItem
      }),
    [attempt, attemptActionPlan, checkpoint, checkpointActionable, detailOperations, pipeline, proofCards, pullRequestStatus, requirement, reviewEvents, runWorkpad, t, workItem]
  );
  const timelineItems =
    attemptTimeline && attempt?.id && attemptTimeline.attempt?.id === attempt.id
      ? attemptTimeline.items ?? []
      : [];
  const workItemExternalRef = visibleExternalReference(workItem.sourceExternalRef);
  const requirementExternalRef = visibleExternalReference(requirement?.sourceExternalRef);
  const canOpenPagePilot = isPagePilotWorkItem(workItem);
  const blockedReason = workItem.status === "Blocked"
    ? blockedReasonForDetail({ attempt, failedStages, runWorkpad })
    : "";

  return (
    <section className="issue-detail-view work-item-detail-page" aria-label={t("Work item detail")}>
      <article className="issue-detail-document">
        <nav className="detail-breadcrumb" aria-label={t("Requirement")}>
          <span>{repositoryLabel || t("Workspace")}</span>
          <span>{t("Requirement")}</span>
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
            {workItemExternalRef ? <span>{workItemExternalRef}</span> : null}
            {repositoryLabel ? <span>{repositoryLabel}</span> : null}
            <span>{agentShortLabel(workItem.assignee)}</span>
          </div>
          {canOpenPagePilot ? (
            <div className="issue-detail-actions">
              <button type="button" onClick={onOpenPagePilot} disabled={!workItem.repositoryTargetId}>
                {t("Open in Page Pilot")}
              </button>
            </div>
          ) : null}
          {blockedReason ? (
            <div className="detail-blocked-callout" role="status">
              <span>{t("Blocked reason")}</span>
              <strong>{blockedReason}</strong>
              {attempt?.id ? <small>{t("Attempt")}: {attempt.id}</small> : null}
            </div>
          ) : null}
        </header>

        <section className="issue-detail-section detail-flow-priority">
          <h3>{t("Delivery flow")}</h3>
          <DeliveryFlowGrid
            actionPlan={attemptActionPlan}
            agentShortLabel={agentShortLabel}
            operationStatusLabel={operationStatusLabel}
            pipeline={pipeline}
            pipelineStageClassName={pipelineStageClassName}
            pipelineStageLabel={pipelineStageLabel}
            stageAgentRuns={stageAgentRuns}
          />
          <ReworkReturnSignal actionPlan={attemptActionPlan} attempt={attempt} pipeline={pipeline} runWorkpad={runWorkpad} />
        </section>

        <RunWorkpad onPatch={onPatchRunWorkpad} record={runWorkpad} sections={workpadSections} />

        <section className="issue-detail-section">
          <h3>{t("Requirement source")}</h3>
          <div className="requirement-source-card">
            <div>
              <span>{t(requirement?.source === "github_issue" ? "GitHub issue" : "Manual requirement")}</span>
              <strong>{requirement?.title ?? workItem.title}</strong>
            </div>
            <div className="requirement-source-meta">
              {requirementExternalRef ? <span>{requirementExternalRef}</span> : null}
              {requirement?.status ? <span>{requirement.status}</span> : null}
              <span>{t("{count} work items", { count: siblingItems.length || 1 })}</span>
            </div>
          </div>
          {(requirement?.rawText || workItem.description) && workItem.description !== "No description provided." ? (
            <div className="issue-detail-copy requirement-source-scroll markdown-content">
              {renderMarkdown(requirement?.rawText ?? workItem.description)}
            </div>
          ) : (
            <p className="muted-copy">{t("No description provided.")}</p>
          )}
        </section>

        <section className="issue-detail-section">
          <h3>{t("Current attempt")}</h3>
          <WorkItemAttemptPanel
            agentShortLabel={agentShortLabel}
            actionPlan={attemptActionPlan}
            attempt={attempt}
            attemptStatusLabel={attemptStatusLabel}
            checkpoint={checkpoint}
            checkpointActionable={checkpointActionable}
            displayText={displayText}
            failedStages={failedStages}
            failureOperations={failureOperations}
            failureProofCards={failureProofCards}
            humanReviewArtifacts={humanReviewArtifacts}
            humanReviewEvents={reviewEvents}
            onApproveCheckpoint={onApproveCheckpoint}
            onFetchProofPreview={onFetchProofPreview}
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
          <h3>{t("Agent operations")}</h3>
          <AgentTraceList
            agentShortLabel={agentShortLabel}
            operations={detailOperations}
            operationStatusLabel={operationStatusLabel}
            pipelineStageClassName={pipelineStageClassName}
          />
        </section>

        <section className="issue-detail-section">
          <h3>{t("Artifacts")}</h3>
          <ArtifactGrid onFetchProofPreview={onFetchProofPreview} proofs={proofCards} />
        </section>

        <section className="issue-detail-section">
          <h3>{t("Attempt history")}</h3>
          <AttemptHistory attempts={attempts} attemptStatusLabel={attemptStatusLabel} />
        </section>

        <section className="issue-detail-section">
          <h3>{t("Target")}</h3>
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
  const { t } = useI18n();
  const [activeSectionId, setActiveSectionId] = useState<string | null>(null);
  const [expanded, setExpanded] = useState(true);
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
        reason: draftReason.trim() || t("Operator edited {field}.", { field: selectedOption.label }),
        source: {
          kind: "ui",
          label: t("Run Workpad editor"),
          field: selectedField,
          attemptId: record.attemptId,
          workItemId: record.workItemId
        }
      });
      setEditorOpen(false);
      setDraftReason("");
    } catch (error) {
      setPatchError(error instanceof Error ? error.message : t("Failed to patch Run Workpad."));
    } finally {
      setPatchSaving(false);
    }
  }

  return (
    <section className="run-workpad" aria-label={t("Run workpad")}>
      <header>
        <div>
          <span className="section-label">Run Workpad</span>
          <h3>{t("Execution brief")}</h3>
        </div>
        <div className="run-workpad-actions">
          <small>{workpadSignalSummary(sections)}</small>
          <button type="button" aria-expanded={expanded} onClick={() => setExpanded((current) => !current)}>
            {expanded ? t("Collapse") : t("Expand")}
          </button>
          {canEdit ? (
            <button
              type="button"
              onClick={() => {
                setActiveSectionId(null);
                setEditorOpen(true);
              }}
            >
              {t("Edit fields")}
            </button>
          ) : null}
        </div>
      </header>
      {expanded ? (
        <div className="run-workpad-grid">
          {sections.map((section) => (
            <button
              key={section.id}
              type="button"
              className={section.tone ? `workpad-card workpad-${section.tone}` : "workpad-card"}
              onClick={() => setActiveSectionId(section.id)}
            >
              <span>{t(section.label)}</span>
              <strong>{t(section.title)}</strong>
              <p>{t(section.preview)}</p>
            </button>
          ))}
        </div>
      ) : null}
      {activeSection ? (
        <section className="detail-popover-backdrop" role="presentation" onClick={() => setActiveSectionId(null)}>
          <article
            className="detail-popover"
            role="dialog"
            aria-modal="true"
            aria-label={`${t(activeSection.label)} detail`}
            onClick={(event) => event.stopPropagation()}
          >
            <header>
              <div>
                <span>{t(activeSection.label)}</span>
                <strong>{t(activeSection.title)}</strong>
              </div>
              <button type="button" onClick={() => setActiveSectionId(null)}>{t("Close")}</button>
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
            aria-label={t("Edit Run Workpad field")}
            onClick={(event) => event.stopPropagation()}
          >
            <header>
              <div>
                <span>{t("Run Workpad patch")}</span>
                <strong>{t("Field editor")}</strong>
              </div>
              <button type="button" onClick={() => setEditorOpen(false)}>{t("Close")}</button>
            </header>
            <form className="workpad-edit-form" onSubmit={submitPatch}>
              <label>
                <span>{t("Field")}</span>
                <select
                  value={selectedField}
                  onChange={(event) => setSelectedField(event.target.value as WorkpadEditableField)}
                >
                  {WORKPAD_EDITABLE_FIELDS.map((field) => (
                    <option key={field.id} value={field.id}>{t(field.label)}</option>
                  ))}
                </select>
              </label>
              <p>{t(selectedOption.description)}</p>
              <label>
                <span>{t("Patch value")}</span>
                <textarea
                  value={draftValue}
                  placeholder={t(selectedOption.placeholder)}
                  onChange={(event) => setDraftValue(event.target.value)}
                />
              </label>
              <label>
                <span>{t("Reason")}</span>
                <input
                  value={draftReason}
                  placeholder={t("Why this field is being patched")}
                  onChange={(event) => setDraftReason(event.target.value)}
                />
              </label>
              {patchError ? <p className="workpad-edit-error">{patchError}</p> : null}
              <div className="workpad-edit-actions">
                <button type="button" onClick={() => setEditorOpen(false)} disabled={patchSaving}>{t("Cancel")}</button>
                <button type="submit" disabled={patchSaving || !record}>
                  {patchSaving ? t("Saving...") : t("Save patch")}
                </button>
              </div>
            </form>
          </article>
        </section>
      ) : null}
    </section>
  );
}

function blockedReasonForDetail({
  attempt,
  failedStages,
  runWorkpad
}: {
  attempt?: AttemptRecordInfo;
  failedStages: StageSummary[];
  runWorkpad?: RunWorkpadRecordInfo;
}): string {
  const blockers = runWorkpad?.workpad?.blockers?.map((item) => item.trim()).filter(Boolean) ?? [];
  if (blockers.length > 0) return blockers[0];
  const retryReason = runWorkpad?.workpad?.retryReason?.trim();
  if (retryReason) return retryReason;
  const attemptReason = [
    attempt?.failureReason,
    attempt?.errorMessage,
    attempt?.statusReason
  ].find((value) => value && value.trim());
  if (attemptReason) return attemptReason.trim();
  const failedStage = failedStages[0];
  if (failedStage) return `${failedStage.title ?? failedStage.id} ${failedStage.status}`;
  if (attempt?.status === "failed") return "The last attempt failed before a detailed reason was captured.";
  if (attempt?.status === "cancelled") return "The last attempt was cancelled.";
  return "This item is blocked. Open the Run Workpad and attempt history for the latest recovery context.";
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
  operationStatusLabel,
  pipeline,
  pipelineStageClassName,
  pipelineStageLabel,
  stageAgentRuns
}: Pick<DetailHelpers, "agentShortLabel" | "operationStatusLabel" | "pipelineStageClassName" | "pipelineStageLabel"> & {
  actionPlan?: AttemptActionPlanInfo | null;
  pipeline?: PipelineRecordInfo;
  stageAgentRuns: Map<string, StageAgentRunSummary>;
}) {
  const { t } = useI18n();
  const [activeStage, setActiveStage] = useState<{
    agentIds: string[];
    status: string;
    stageId: string;
    title: string;
  } | null>(null);
  const pipelineStages = pipeline?.run?.stages ?? [];
  const planStates = actionPlan?.states?.length ? actionPlan.states : [];
  const stages = pipelineStages.length ? pipelineStages : planStates;
  const reworkRunning = stages.some((stage) => {
    const stageRecord = stage as Record<string, unknown>;
    return recordString(stageRecord, "id") === "rework" && recordString(stageRecord, "status") === "running";
  });
  if (!stages.length) {
    return <p className="muted-copy">Delivery stages will appear after a pipeline is created.</p>;
  }
  return (
    <div className="detail-stage-grid">
      {stages.map((stage, index) => {
        const stageRecord = stage as Record<string, unknown>;
        const agent = recordString(stageRecord, "agent") || recordString(stageRecord, "agentId");
        const agentIds = Array.isArray(stageRecord.agentIds) ? stageRecord.agentIds.map(String) : agent ? [agent] : [];
        const stageId = recordString(stageRecord, "id") || String(index);
        const rawStatus = recordString(stageRecord, "status");
        const startedAt = recordString(stageRecord, "startedAt");
        const completedAt = recordString(stageRecord, "completedAt");
        const hasRunningAgent = stageAgentRuns.get(stageId)?.details.some((detail) => /running/i.test(detail)) ?? false;
        const implementationReworkActive =
          reworkRunning &&
          (stageId === "in_progress" || stageId === "implementation") &&
          (rawStatus === "passed" || rawStatus === "waiting");
        const status =
          implementationReworkActive
            ? "running"
            : rawStatus === "waiting" && !completedAt && (hasRunningAgent || (pipeline?.status === "running" && startedAt))
            ? "running"
            : rawStatus;
        const participantLabel = stageParticipantLabel(stageRecord, agentIds, agentShortLabel);
        const summary = stageAgentRuns.get(stageId);
        const runtimeLabel = stageAgentRuntimeLabel(summary, agentIds, agentShortLabel, t);
        const stageTitle = recordString(stageRecord, "title") || recordString(stageRecord, "id") || stageId;
        return (
          <article key={stageId} className={`detail-stage-card ${pipelineStageClassName(status)}`}>
            <span className="stage-card-index">{index + 1}</span>
            <div className="stage-card-content">
              <strong>{stageTitle}</strong>
              {participantLabel ? <small>{participantLabel}</small> : null}
              {runtimeLabel ? <small className="stage-agent-runtime">{runtimeLabel}</small> : null}
              <StageAgentMetrics summary={summary} plannedAgentIds={agentIds} />
            </div>
            <div className="stage-card-actions">
              <span className="stage-card-status">{pipelineStageLabel(status)}</span>
              <button
                type="button"
                className="stage-agent-detail-button"
                aria-label={`${t("View agents")}: ${stageTitle}`}
                onClick={() => setActiveStage({ agentIds, stageId, status, title: stageTitle })}
              >
                {t("View agents")}
              </button>
            </div>
          </article>
        );
      })}
      {activeStage ? (
        <StageAgentDetailDialog
          agentShortLabel={agentShortLabel}
          onClose={() => setActiveStage(null)}
          operationStatusLabel={operationStatusLabel}
          plannedAgentIds={activeStage.agentIds}
          pipelineStageLabel={pipelineStageLabel}
          stageId={activeStage.stageId}
          status={activeStage.status}
          summary={stageAgentRuns.get(activeStage.stageId)}
          title={activeStage.title}
        />
      ) : null}
    </div>
  );
}

function StageAgentMetrics({ plannedAgentIds, summary }: { plannedAgentIds: string[]; summary?: StageAgentRunSummary }) {
  const { t } = useI18n();
  if (!summary?.launchedCount) {
    if (plannedAgentIds.length <= 1) return null;
    return (
      <div className="stage-agent-metrics" aria-label="Planned stage agents">
        <span>{t("{count} planned", { count: plannedAgentIds.length })}</span>
      </div>
    );
  }
  const duration = stageDurationLabel(summary);
  const tokens = summary.totalTokens > 0 ? formatTokenCount(summary.totalTokens) : "";
  const failedCount = summary.statusCounts.failed ?? 0;
  const runningCount = summary.statusCounts.running ?? 0;
  return (
    <div className="stage-agent-metrics" aria-label="Stage agent statistics">
      <span>{t("{count} runs", { count: summary.launchedCount })}</span>
      {duration ? <span>{duration}</span> : null}
      {tokens ? <span>{tokens}</span> : null}
      {failedCount ? <span className="stage-agent-metric-warning">{t("{count} failed", { count: failedCount })}</span> : null}
      {runningCount ? <span>{t("{count} running", { count: runningCount })}</span> : null}
    </div>
  );
}

function StageAgentDetailDialog({
  agentShortLabel,
  onClose,
  operationStatusLabel,
  pipelineStageLabel,
  plannedAgentIds,
  stageId,
  status,
  summary,
  title
}: {
  agentShortLabel: (agentId: string) => string;
  onClose: () => void;
  operationStatusLabel: (status: string) => string;
  pipelineStageLabel: (status: string) => string;
  plannedAgentIds: string[];
  stageId: string;
  status: string;
  summary?: StageAgentRunSummary;
  title: string;
}) {
  const { t } = useI18n();
  const details = summary?.operationDetails ?? [];
  const duration = summary ? stageDurationLabel(summary) : "";
  const runnerMix = compactAgentRunDetails(summary?.details ?? []);
  const planned = plannedAgentIds.map(agentShortLabel);
  return (
    <section className="detail-popover-backdrop" role="presentation" onClick={onClose}>
      <article
        className="detail-popover stage-agent-popover"
        role="dialog"
        aria-modal="true"
        aria-label={`${title} agent statistics`}
        onClick={(event) => event.stopPropagation()}
      >
        <header>
          <div>
            <span>{stageId}</span>
            <strong>{title}</strong>
          </div>
          <button type="button" onClick={onClose}>{t("Close")}</button>
        </header>
        <div className="detail-popover-body stage-agent-popover-body">
          <div className="stage-agent-stat-grid" aria-label={t("Stage statistics")}>
            <span>
              <strong>{t("Status")}</strong>
              <small>{pipelineStageLabel(status)}</small>
            </span>
            <span>
              <strong>{t("Agent runs")}</strong>
              <small>{summary?.launchedCount ?? 0}</small>
            </span>
            <span>
              <strong>{t("Duration")}</strong>
              <small>{duration || t("Not captured")}</small>
            </span>
            <span>
              <strong>{t("Tokens")}</strong>
              <small>{summary?.totalTokens ? formatTokenUsage(summary) : t("Not captured")}</small>
            </span>
            <span>
              <strong>{t("Runtime")}</strong>
              <small>{runnerMix || planned.join(" + ") || t("Not captured")}</small>
            </span>
          </div>
          {details.length ? (
            <div className="stage-agent-detail-list">
              {details.map((detail) => (
                <article key={detail.id} className={`stage-agent-detail-row ${detail.source === "event" ? "is-event" : ""}`}>
                  <div className="stage-agent-detail-main">
                    <span>{t(detail.roleLabel)}</span>
                    <strong>{detail.agentLabel}</strong>
                    <small>{agentRuntimeMeta(detail) || t("Runtime not captured")}</small>
                    <p>{shortText(detail.summary, 220)}</p>
                  </div>
                  <div className="stage-agent-detail-facts">
                    <span>{operationStatusLabel(detail.status)}</span>
                    {typeof detail.durationMs === "number" && detail.durationMs > 0 ? <span>{formatDurationLabel(detail.durationMs)}</span> : null}
                    {detail.tokenUsage?.total ? <span>{formatTokenUsage(detail.tokenUsage)}</span> : null}
                  </div>
                </article>
              ))}
            </div>
          ) : (
            <p className="muted-copy">
              {planned.length ? `${t("Planned agents")}: ${planned.join(" + ")}` : t("Agent trace will appear as soon as the local orchestrator starts assigning stage work.")}
            </p>
          )}
        </div>
      </article>
    </section>
  );
}

function stageParticipantLabel(stage: Record<string, unknown>, agentIds: string[], agentShortLabel: (agentId: string) => string): string {
  if (agentIds.length) return agentIds.map(agentShortLabel).join(" + ");
  const raw = `${recordString(stage, "id")} ${recordString(stage, "title")}`.toLowerCase();
  if (/todo|intake|requirement/.test(raw)) return "Requirement";
  if (/implementation|architect|coding|test/.test(raw)) return "Architecture + Code + Test";
  if (/code_review|review round|review/.test(raw)) return "Review";
  if (/rework/.test(raw)) return "Code + Test";
  if (/human/.test(raw)) return "Human Review";
  if (/merg|deliver|done|ship/.test(raw)) return "Delivery";
  return "Workflow stage";
}

function stageAgentRuntimeLabel(
  summary: StageAgentRunSummary | undefined,
  plannedAgentIds: string[],
  agentShortLabel: (agentId: string) => string,
  t: (key: string, params?: Record<string, string | number>) => string
): string {
  if (summary?.launchedCount) {
    const detail = compactAgentRunDetails(summary.details.length ? summary.details : summary.agents);
    return detail
      ? `${t("{count} agent run(s)", { count: summary.launchedCount })} · ${detail}`
      : t("{count} agent run(s)", { count: summary.launchedCount });
  }
  if (!plannedAgentIds.length) return "";
  const planned = compactAgentRunDetails(plannedAgentIds.map(agentShortLabel));
  return planned
    ? `${t("{count} planned agent(s)", { count: plannedAgentIds.length })} · ${planned}`
    : t("{count} planned agent(s)", { count: plannedAgentIds.length });
}

function compactAgentRunDetails(details: string[]): string {
  const unique = Array.from(new Set(details.map((detail) => detail.trim()).filter(Boolean)));
  if (unique.length <= 2) return unique.join(" + ");
  return `${unique.slice(0, 2).join(" + ")} +${unique.length - 2}`;
}

function stageDurationLabel(summary: StageAgentRunSummary): string {
  if (summary.totalDurationMs <= 0 || summary.durationSampleCount <= 0) return "";
  const total = formatDurationLabel(summary.totalDurationMs);
  if (summary.durationSampleCount <= 1) return total;
  return `${total} total · ${formatDurationLabel(summary.totalDurationMs / summary.durationSampleCount)} avg`;
}

function formatDurationLabel(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return "0ms";
  if (value < 1000) return `${Math.round(value)}ms`;
  const seconds = value / 1000;
  if (seconds < 60) return `${seconds.toFixed(seconds < 10 ? 1 : 0)}s`;
  const minutes = Math.floor(seconds / 60);
  const remaining = Math.round(seconds % 60);
  return remaining ? `${minutes}m ${remaining}s` : `${minutes}m`;
}

function formatTokenCount(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return "";
  if (value < 1000) return `${Math.round(value)} tokens`;
  if (value < 1000000) return `${(value / 1000).toFixed(value < 10000 ? 1 : 0)}k tokens`;
  return `${(value / 1000000).toFixed(1)}m tokens`;
}

function formatTokenUsage(value: AgentTokenUsage | StageAgentRunSummary): string {
  const input = "inputTokens" in value ? value.inputTokens : value.input;
  const output = "outputTokens" in value ? value.outputTokens : value.output;
  const total = "totalTokens" in value ? value.totalTokens : value.total;
  const pieces = [formatTokenCount(total)].filter(Boolean);
  if (input || output) {
    pieces.push(`in ${formatTokenCount(input ?? 0) || "0 tokens"}`);
    pieces.push(`out ${formatTokenCount(output ?? 0) || "0 tokens"}`);
  }
  return pieces.join(" · ");
}

function agentRuntimeMeta(detail: StageAgentRunDetail): string {
  return [detail.runner, detail.provider, detail.model, detail.processStatus ? `process ${detail.processStatus}` : ""]
    .map((value) => value?.trim())
    .filter(Boolean)
    .join(" · ");
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
  actionPlan,
  attempt,
  checkpoint,
  checkpointActionable,
  operations,
  pipeline,
  proofCards,
  pullRequestStatus,
  requirement,
  reviewEvents,
  runWorkpad,
  t,
  workItem
}: {
  actionPlan?: AttemptActionPlanInfo | null;
  attempt?: AttemptRecordInfo;
  checkpoint?: CheckpointRecordInfo;
  checkpointActionable?: boolean;
  operations: OperationRecordInfo[];
  pipeline?: PipelineRecordInfo;
  proofCards: DetailProofCard[];
  pullRequestStatus?: GitHubPullRequestStatusResult | null;
  requirement?: RequirementRecordInfo;
  reviewEvents: DetailReviewEventCard[];
  runWorkpad?: RunWorkpadRecordInfo;
  t: (key: string, params?: Record<string, string | number>) => string;
  workItem: WorkItem;
}): WorkpadSection[] {
  const workpad = runWorkpad?.workpad;
  const planArtifacts = proofCards.filter((proof) => /plan|solution|architecture|todo/i.test(`${proof.label} ${proof.kind}`));
  const planProgress = buildPlanProgress({ actionPlan, pipeline, planArtifacts, t });
  const validationOps = operations.filter((operation) => /test|check|validation/i.test(`${operation.stageId ?? ""} ${operation.agentId ?? ""} ${operation.summary ?? ""}`));
  const reviewOps = operations.filter((operation) => /review|rework/i.test(`${operation.stageId ?? ""} ${operation.agentId ?? ""} ${operation.summary ?? ""}`));
  const recordedBlockers = asStringArray(workpad?.blockers).filter((blocker) => {
    if (checkpointActionable && /human review|人工|审批/i.test(blocker)) return false;
    return true;
  });
  const checkpointBlocker =
    checkpoint && checkpoint.status === "rejected"
      ? `${checkpoint.title}: ${checkpoint.summary}`
      : "";
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
          title: t(patchHistory.length === 1 ? "{count} update" : "{count} updates", { count: patchHistory.length }),
          preview: patchHistory[0] ? `${patchHistory[0].updatedBy} updated ${patchHistory[0].fields.join(", ")}` : t("No field patch recorded."),
          body: <WorkpadPatchHistory entries={patchHistory.slice(0, 5)} />
        }]
      : []),
    ...(reviewPacket
      ? [{
          id: "review-packet",
          label: "Review packet",
          title: reviewPacketTitle(reviewPacket),
          preview: shortText(recordString(reviewPacket, "summary") || reviewPacketFirstAction(reviewPacket) || t("Diff, validation, checks and risk preview."), 120),
          tone: reviewPacketTone(reviewPacket),
          body: <ReviewPacketPreview packet={reviewPacket} />
        }]
      : []),
    {
      id: "plan",
      label: "Plan",
      title: recordString(workpad?.plan, "currentStageId") || planProgress.title,
      preview: planProgress.preview,
      tone: planProgress.tone,
      body: (
        <PlanProgressPanel
          artifactCount={planArtifacts.length}
          artifactHint={t("Open the Artifacts section below for the source file.")}
          artifactSummary={planArtifacts.length === 1 ? t("1 plan artifact captured") : t("{count} plan artifacts captured.", { count: planArtifacts.length })}
          fallback={shortText(requirement?.rawText ?? workItem.description)}
          items={planProgress.items}
          title={t("Plan progress")}
        />
      )
    },
    {
      id: "acceptance",
      label: "Acceptance criteria",
      title: t("{count} criteria", { count: criteria.length }),
      preview: shortText(criteria[0] ?? t("No acceptance criteria captured."), 120),
      body: (
        <ul>
          {criteria.slice(0, 5).map((criterion) => <li key={criterion}>{criterion}</li>)}
        </ul>
      )
    },
    {
      id: "validation",
      label: "Validation status",
      title: validationStatus || pullRequestStatus?.deliveryGate || (validationOps.length ? t("Validation captured") : t("Pending")),
      preview: validationOps[0]?.summary
        ? shortText(validationOps[0].summary, 120)
        : validationStatus
          ? t("Validation is {status}.", { status: validationStatus })
          : t("Waiting for test reports or checks."),
      tone: validationStatus === "passed" || pullRequestStatus?.deliveryGate === "passed" ? "success" : undefined,
      body: validationOps.length ? <OperationSummaryList operations={validationOps.slice(-3)} /> : <p>{t("Test reports and checks will appear here after validation runs.")}</p>
    },
    {
      id: "pr",
      label: "PR",
      title: attempt?.pullRequestUrl ? t("Pull request ready") : attempt?.branchName ? t("Branch ready") : t("Not created"),
      preview: attempt?.pullRequestUrl || attempt?.branchName || t("No delivery branch or pull request yet."),
      body: attempt?.pullRequestUrl ? (
        <a href={attempt.pullRequestUrl} target="_blank" rel="noreferrer">{t("Open PR")}</a>
      ) : (
        <p>{attempt?.branchName ?? t("PR link will appear after delivery creates it.")}</p>
      )
    },
    {
      id: "feedback",
      label: "Review Feedback",
      title: reviewFeedback ? t("Feedback captured") : t("No feedback yet"),
      preview: reviewFeedback ? shortText(reviewFeedback, 120) : t("No review, PR, or human feedback captured."),
      body: reviewFeedback ? <p>{shortText(reviewFeedback, 360)}</p> : <p>{t("Review agent, PR comments and human requested changes will be merged here.")}</p>
    },
    {
      id: "blockers",
      label: "Blockers",
      title: blockers.length ? t(blockers.length === 1 ? "{count} active signal" : "{count} active signals", { count: blockers.length }) : t("No active blockers"),
      preview: blockers.length ? shortText(blockers[0], 120) : t("No blocking failure is recorded."),
      tone: blockers.length ? "warning" : "success",
      body: blockers.length ? (
        <ul>{blockers.slice(0, 4).map((blocker) => <li key={blocker}>{shortText(blocker, 220)}</li>)}</ul>
      ) : (
        <p>{t("No blocking failure is recorded for the current attempt.")}</p>
      )
    },
    {
      id: "retry",
      label: "Retry Reason",
      title: retryReason ? t("Ready for retry") : t("No retry needed"),
      preview: retryReason ? shortText(retryReason, 120) : t("Current run has no retry reason."),
      body: retryReason ? <p>{shortText(retryReason, 420)}</p> : <p>{t("Retry will reuse the captured blocker and review feedback when needed.")}</p>
    },
    {
      id: "notes",
      label: "Notes",
      title: t(operations.length === 1 ? "{count} operation" : "{count} operations", { count: operations.length }),
      preview: operations.length ? shortText(operations[operations.length - 1]?.summary ?? t("Latest operation recorded."), 120) : t("No operation notes yet."),
      body: operations.length ? <OperationSummaryList operations={operations.slice(-3)} /> : <p>{t("Agent notes will appear as operations are recorded.")}</p>
    }
  ];

  return sections;
}

function workpadSignalSummary(sections: WorkpadSection[]): string {
  const blocker = sections.find((section) => section.id === "blockers" && section.tone === "warning");
  const retry = sections.find((section) => section.id === "retry" && !/No retry|无需重试/i.test(section.title));
  const feedback = sections.find((section) => section.id === "feedback" && !/No feedback|暂无反馈/i.test(section.title));
  if (blocker) return blocker.title;
  if (retry) return retry.title;
  if (feedback) return feedback.title;
  return `${sections.length} signals`;
}

type PlanProgressItem = {
  label: string;
  status: string;
  detail: string;
  tone: "pending" | "running" | "captured";
};

function buildPlanProgress({
  actionPlan,
  pipeline,
  planArtifacts,
  t
}: {
  actionPlan?: AttemptActionPlanInfo | null;
  pipeline?: PipelineRecordInfo;
  planArtifacts: DetailProofCard[];
  t: (key: string, params?: Record<string, string | number>) => string;
}): { title: string; preview: string; tone?: "success"; items: PlanProgressItem[] } {
  const actions = arrayRecords(actionPlan?.actions);
  const architectureAction = actions.find((action) => /architecture_handoff|architect|plan/i.test(`${recordString(action, "id")} ${recordString(action, "type")}`));
  const reviewAction = actions.find((action) => /review/i.test(`${recordString(action, "id")} ${recordString(action, "type")}`));
  const architectureStage = pipeline?.run?.stages?.find((stage) => /in_progress|architect|implementation/i.test(`${recordString(stage, "id")} ${recordString(stage, "title")}`));
  const architectureStatus = recordString(architectureAction, "status") || recordString(architectureStage, "status");
  const outputArtifacts = artifactNamesFromValue(architectureAction?.outputArtifacts);
  const artifactNames = new Set([
    ...outputArtifacts,
    ...planArtifacts.flatMap((artifact) => [artifact.label, artifact.path ?? ""])
  ].map((value) => value.toLowerCase()));
  const hasPlan = hasAnyArtifact(artifactNames, [/technical-plan/, /solution-plan/, /\bplan\b/]) || planArtifacts.length > 0;
  const hasFunctionalTodo = hasAnyArtifact(artifactNames, [/functional-todo-list/, /functional.*todo/, /solution-plan/]);
  const hasProjectTodo = hasAnyArtifact(artifactNames, [/project-todo-list/, /project.*todo/, /solution-plan/]);
  const reviewStatus = recordString(reviewAction, "status");
  const reviewReady = Boolean(reviewAction || hasPlan || hasFunctionalTodo || hasProjectTodo);
  const architectureTone = progressTone(architectureStatus, hasPlan);
  const capturedCount = [hasPlan, hasFunctionalTodo, hasProjectTodo, reviewReady].filter(Boolean).length;

  return {
    title: capturedCount >= 3 ? t("Plan and TODO captured") : architectureTone === "running" ? t("Planning in progress") : t("Plan pending"),
    preview: capturedCount >= 3
      ? t("Functional TODO, project TODO, and review alignment are visible.")
      : t("Planning progress will appear as the architect action runs."),
    tone: capturedCount >= 3 ? "success" : undefined,
    items: [
      {
        label: t("Plan stage"),
        status: progressStatusLabel(architectureStatus, hasPlan, t),
        detail: recordString(architectureAction, "id") || recordString(architectureAction, "type") || t("Architect action has not started."),
        tone: architectureTone
      },
      {
        label: t("Functional TODO"),
        status: hasFunctionalTodo ? t("Captured") : t("Pending"),
        detail: hasFunctionalTodo ? t("Feature checklist is part of the plan artifacts.") : t("Waiting for the architecture handoff output."),
        tone: hasFunctionalTodo ? "captured" : "pending"
      },
      {
        label: t("Project TODO"),
        status: hasProjectTodo ? t("Captured") : t("Pending"),
        detail: hasProjectTodo ? t("Project file checklist is part of the plan artifacts.") : t("Waiting for the architecture handoff output."),
        tone: hasProjectTodo ? "captured" : "pending"
      },
      {
        label: t("Review alignment"),
        status: reviewReady ? (reviewStatus ? progressStatusLabel(reviewStatus, true, t) : t("Ready")) : t("Pending"),
        detail: reviewReady ? t("Review checks implementation against the plan and TODO list.") : t("Review will use the plan after it is captured."),
        tone: reviewReady ? "captured" : "pending"
      }
    ]
  };
}

function progressTone(status: string, captured: boolean): PlanProgressItem["tone"] {
  if (captured || /passed|done|success|completed/i.test(status)) return "captured";
  if (/running|in_progress|active/i.test(status)) return "running";
  return "pending";
}

function progressStatusLabel(status: string, captured: boolean, t: (key: string, params?: Record<string, string | number>) => string): string {
  if (/running|in_progress|active/i.test(status)) return t("Running");
  if (captured || /passed|done|success|completed/i.test(status)) return t("Captured");
  if (status) return status;
  return t("Pending");
}

function hasAnyArtifact(names: Set<string>, patterns: RegExp[]): boolean {
  return [...names].some((name) => patterns.some((pattern) => pattern.test(name)));
}

function artifactNamesFromValue(value: unknown): string[] {
  if (Array.isArray(value)) {
    return value.flatMap((entry) => {
      if (typeof entry === "string") return [entry];
      const record = recordValue(entry);
      return record ? [recordString(record, "id"), recordString(record, "name"), recordString(record, "label"), recordString(record, "path")] : [];
    }).filter(Boolean);
  }
  if (typeof value === "string") return [value];
  return [];
}

function PlanProgressPanel({
  artifactCount,
  artifactHint,
  artifactSummary,
  fallback,
  items,
  title
}: {
  artifactCount: number;
  artifactHint: string;
  artifactSummary: string;
  fallback: string;
  items: PlanProgressItem[];
  title: string;
}) {
  return (
    <div className="workpad-plan-progress" aria-label={title}>
      <ul className="plan-progress-list">
        {items.map((item) => (
          <li key={item.label} className={`plan-progress-${item.tone}`}>
            <span>{item.label}</span>
            <strong>{item.status}</strong>
            <small>{item.detail}</small>
          </li>
        ))}
      </ul>
      {artifactCount ? (
        <div className="plan-progress-artifacts">
          <strong>{artifactSummary}</strong>
          <small>{artifactHint}</small>
        </div>
      ) : (
        <p>{fallback}</p>
      )}
    </div>
  );
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
  const { t } = useI18n();
  const diff = recordValue(packet.diffPreview);
  const test = recordValue(packet.testPreview);
  const checks = recordValue(packet.checkPreview);
  const risk = recordValue(packet.risk);
  const todoCompletion = recordValue(packet.todoCompletion);
  const riskBasis = arrayRecords(risk?.basis);
  const actions = arrayRecords(packet.recommendedActions);
  const changedFiles = asStringArray(diff?.changedFiles);
  return (
    <div className="review-packet-preview" aria-label={t("Review packet preview")}>
      <p>{recordString(packet, "summary") || t("Review packet is ready for human review.")}</p>
      <div className="review-packet-grid">
        <article>
          <span>{t("Diff")}</span>
          <strong>{recordString(diff, "summary") || t("{count} changed files", { count: changedFiles.length })}</strong>
          {changedFiles.length ? (
            <ul>{changedFiles.slice(0, 6).map((file) => <li key={file}>{file}</li>)}</ul>
          ) : (
            <small>{t("No changed files captured.")}</small>
          )}
        </article>
        <article>
          <span>{t("Tests")}</span>
          <strong>{recordString(test, "status") || t("unknown")}</strong>
          <small>{recordString(test, "summary") || t("No validation preview captured.")}</small>
        </article>
        <article>
          <span>{t("Checks")}</span>
          <strong>{recordString(checks, "status") || t("unknown")}</strong>
          <small>{recordString(checks, "summary") || t("No check preview captured.")}</small>
        </article>
        <article className={`review-packet-risk review-packet-risk-${recordString(risk, "level") || "low"}`}>
          <span>{t("Risk")}</span>
          <strong>{recordString(risk, "level") || t("low")}</strong>
          <ul>{asStringArray(risk?.reasons).slice(0, 4).map((reason) => <li key={reason}>{reason}</li>)}</ul>
          {riskBasis.length ? (
            <div className="review-packet-risk-basis" aria-label={t("Risk basis")}>
              <span>{t("Risk basis")}</span>
              {riskBasis.slice(0, 4).map((entry, index) => (
                <small key={`${recordString(entry, "source")}-${index}`}>
                  <b>{recordString(entry, "level") || t("unknown")}</b>
                  {recordString(entry, "source") ? ` · ${recordString(entry, "source")}` : ""}
                  {recordString(entry, "evidence") ? `: ${recordString(entry, "evidence")}` : ""}
                </small>
              ))}
            </div>
          ) : null}
        </article>
      </div>
      {todoCompletion ? <TodoCompletionPreview completion={todoCompletion} /> : null}
      {actions.length ? (
        <div className="review-packet-actions">
          <span>{t("Next actions")}</span>
          <ul>
            {actions.slice(0, 5).map((action, index) => {
              const label = recordString(action, "label") || recordString(action, "type") || t("Action {count}", { count: index + 1 });
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

function TodoCompletionPreview({ completion }: { completion: Record<string, unknown> }) {
  const { t } = useI18n();
  const counts = recordValue(completion.counts);
  const functional = arrayRecords(completion.functional);
  const project = arrayRecords(completion.project);
  const status = recordString(completion, "status") || "pending";
  const verified = numberValue(counts?.verified) ?? 0;
  const total = numberValue(counts?.total) ?? functional.length + project.length;
  const pending = numberValue(counts?.pending) ?? 0;
  const attention = numberValue(counts?.attention) ?? 0;
  return (
    <section className={`todo-completion-preview todo-completion-${status}`} aria-label={t("Plan TODO verification")}>
      <header>
        <span>{t("Plan TODO verification")}</span>
        <strong>{recordString(completion, "summary") || t("{verified}/{total} TODOs verified", { verified, total })}</strong>
      </header>
      <div className="todo-completion-counts" aria-label={t("TODO verification counts")}>
        <span>{t("Verified")}: {verified}</span>
        <span>{t("Pending")}: {pending}</span>
        <span>{t("Attention")}: {attention}</span>
      </div>
      <TodoCompletionGroup title={t("Functional TODO")} items={functional} empty={t("No functional TODO captured.")} />
      <TodoCompletionGroup title={t("Project TODO")} items={project} empty={t("No project TODO captured.")} />
    </section>
  );
}

function TodoCompletionGroup({ empty, items, title }: { empty: string; items: Record<string, unknown>[]; title: string }) {
  const { t } = useI18n();
  if (!items.length) {
    return (
      <div className="todo-completion-group">
        <span>{title}</span>
        <small>{empty}</small>
      </div>
    );
  }
  return (
    <div className="todo-completion-group">
      <span>{title}</span>
      <ul>
        {items.map((item, index) => {
          const status = recordString(item, "status") || "pending";
          return (
            <li key={`${recordString(item, "text")}-${index}`} className={`todo-completion-item todo-completion-item-${status}`}>
              <strong aria-label={t(todoStatusLabel(status))}>{todoStatusMark(status)}</strong>
              <div>
                <b>{recordString(item, "text") || t("Untitled TODO")}</b>
                <small>{recordString(item, "evidence") || t("No evidence captured.")}</small>
              </div>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

function todoStatusMark(status: string): string {
  if (status === "verified") return "✓";
  if (status === "attention") return "!";
  return "·";
}

function todoStatusLabel(status: string): string {
  if (status === "verified") return "Verified";
  if (status === "attention") return "Attention";
  return "Pending";
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

function isHumanReviewCheckpoint(checkpoint: CheckpointRecordInfo): boolean {
  return /human[_-]?review|human review|人工|审批/i.test(`${checkpoint.stageId} ${checkpoint.title} ${checkpoint.summary}`);
}

function checkpointTime(checkpoint: CheckpointRecordInfo): number {
  const raw = checkpoint.updatedAt || checkpoint.createdAt || "";
  const parsed = raw ? Date.parse(raw) : Number.NaN;
  return Number.isFinite(parsed) ? parsed : 0;
}

function displayCheckpointForCurrentRun(
  checkpoint: CheckpointRecordInfo | undefined,
  pipeline: PipelineRecordInfo | undefined,
  attempt: AttemptRecordInfo | undefined
): CheckpointRecordInfo | undefined {
  if (!checkpoint || checkpoint.status !== "pending") return checkpoint;
  const humanReviewStage = pipeline?.run?.stages?.find((stage) => recordString(stage, "id") === "human_review");
  const humanReviewPassed = recordString(humanReviewStage, "status") === "passed";
  const runCompleted = pipeline?.status === "done" || attempt?.status === "done";
  if (!humanReviewPassed || !runCompleted) return checkpoint;
  const reviewer = recordString(humanReviewStage, "approvedBy") || "human";
  return {
    ...checkpoint,
    status: "approved",
    decisionNote: checkpoint.decisionNote || `approved by ${reviewer}`
  };
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

function summarizeStageAgentRuns({
  agentShortLabel,
  attempt,
  operations,
  pipeline
}: {
  agentShortLabel: (agentId: string) => string;
  attempt?: AttemptRecordInfo;
  operations: OperationRecordInfo[];
  pipeline?: PipelineRecordInfo;
}): Map<string, StageAgentRunSummary> {
  const summaries = new Map<string, StageAgentRunSummary>();
  const operationStageAgents = new Set<string>();
  const operationStages = new Set<string>();
  const seenRuns = new Set<string>();

  const ensure = (stageId: string) => {
    const existing = summaries.get(stageId);
    if (existing) return existing;
    const created: StageAgentRunSummary = {
      launchedCount: 0,
      agents: [],
      details: [],
      operationDetails: [],
      eventCount: 0,
      totalDurationMs: 0,
      durationSampleCount: 0,
      totalTokens: 0,
      inputTokens: 0,
      outputTokens: 0,
      statusCounts: {}
    };
    summaries.set(stageId, created);
    return created;
  };

  const addRun = (input: {
    id: string;
    stageId: string;
    agentId?: string;
    runner?: string;
    model?: string;
    provider?: string;
    processStatus?: string;
    durationMs?: number;
    tokenUsage?: AgentTokenUsage;
    status?: string;
    summary?: string;
    startedAt?: string;
    finishedAt?: string;
    fromOperation?: boolean;
  }) => {
    const stageId = input.stageId.trim();
    if (!stageId || seenRuns.has(input.id)) return;
    seenRuns.add(input.id);
    const summary = ensure(stageId);
    summary.launchedCount += 1;
    const agentLabel = input.agentId ? agentShortLabel(input.agentId) : input.runner || "Agent";
    if (!summary.agents.includes(agentLabel)) summary.agents.push(agentLabel);
    const runtime = stageAgentRuntimeDetail(agentLabel, input.runner, input.model, input.provider);
    if (runtime && !summary.details.includes(runtime)) summary.details.push(runtime);
    const status = input.status || input.processStatus || "recorded";
    summary.statusCounts[status] = (summary.statusCounts[status] ?? 0) + 1;
    if (typeof input.durationMs === "number" && input.durationMs > 0) {
      summary.totalDurationMs += input.durationMs;
      summary.durationSampleCount += 1;
    }
    if (input.tokenUsage?.total) {
      summary.totalTokens += input.tokenUsage.total;
      summary.inputTokens += input.tokenUsage.input ?? 0;
      summary.outputTokens += input.tokenUsage.output ?? 0;
    }
    if (input.fromOperation) {
      summary.operationDetails.push({
        id: input.id,
        stageId,
        agentId: input.agentId,
        agentLabel,
        roleLabel: agentRoleLabel(input.agentId, stageId),
        status,
        runner: input.runner,
        provider: input.provider,
        model: input.model,
        processStatus: input.processStatus,
        durationMs: input.durationMs,
        tokenUsage: input.tokenUsage,
        summary: input.summary || "Trace details captured for this stage.",
        startedAt: input.startedAt,
        finishedAt: input.finishedAt,
        source: "operation"
      });
    } else {
      summary.eventCount += 1;
      summary.operationDetails.push({
        id: input.id,
        stageId,
        agentId: input.agentId,
        agentLabel,
        roleLabel: agentRoleLabel(input.agentId, stageId),
        status,
        summary: input.summary || "Agent event captured before operation detail was persisted.",
        startedAt: input.startedAt,
        source: "event"
      });
    }
    if (input.fromOperation && input.agentId) operationStageAgents.add(`${stageId}:${input.agentId}`);
    if (input.fromOperation) operationStages.add(stageId);
  };

  for (const operation of operations) {
    const parsed = parseAgentOperationId(operation.id);
    const stageId = operation.stageId || parsed.stageId;
    if (!stageId) continue;
    const agentId = operation.agentId || parsed.agentId;
    const runner = operation.runnerProcess?.runner;
    const model = operation.runnerProcess?.model;
    const provider = operation.runnerProcess?.provider;
    const runnerProcess = recordValue(operation.runnerProcess);
    addRun({
      id: `operation:${operation.id}`,
      stageId,
      agentId,
      runner,
      model,
      provider,
      processStatus: operation.runnerProcess?.status,
      durationMs: operation.runnerProcess?.durationMs,
      tokenUsage: tokenUsageFromRecord(runnerProcess),
      status: operation.status,
      summary: operation.summary || agentOperationSummary(operation),
      startedAt: operation.runnerProcess?.startedAt || operation.createdAt,
      finishedAt: operation.runnerProcess?.finishedAt || operation.updatedAt,
      fromOperation: true
    });
  }

  const eventSources = [
    ...(attempt?.events ?? []),
    ...(pipeline?.run?.events ?? []).map((event) => ({ ...event, createdAt: event.timestamp }))
  ];
  for (const event of eventSources) {
    const eventRecord = event as Record<string, unknown>;
    const type = recordString(eventRecord, "type");
    if (!type.startsWith("agent.")) continue;
    const stageId = recordString(eventRecord, "stageId");
    const agentId = recordString(eventRecord, "agentId");
    if (!stageId) continue;
    if (!agentId && operationStages.has(stageId)) continue;
    if (agentId && operationStageAgents.has(`${stageId}:${agentId}`)) continue;
    addRun({
      id: `event:${stageId}:${agentId || "agent"}:${type}:${recordString(eventRecord, "createdAt")}`,
      stageId,
      agentId,
      status: agentEventStatus(type),
      summary: recordString(eventRecord, "message") || type,
      startedAt: recordString(eventRecord, "createdAt")
    });
  }

  return summaries;
}

function agentOperationSummary(operation: OperationRecordInfo): string {
  return shortText(
    operation.summary ||
    operation.runnerProcess?.stderr ||
    operation.runnerProcess?.stdout ||
    operation.prompt ||
    "",
    260
  );
}

function agentEventStatus(type: string): string {
  if (/failed|error/i.test(type)) return "failed";
  if (/completed|passed|done/i.test(type)) return "passed";
  if (/started|running|heartbeat/i.test(type)) return "running";
  return "recorded";
}

function agentRoleLabel(agentId?: string, stageId?: string): string {
  const raw = `${agentId ?? ""} ${stageId ?? ""}`.toLowerCase();
  if (/requirement|intake|todo/.test(raw)) return "Requirement";
  if (/master|dispatch|orchestrat/.test(raw)) return "Orchestration";
  if (/architect|plan|solution/.test(raw)) return "Plan";
  if (/coding|code|implement|rework/.test(raw)) return "Code";
  if (/testing|test|validation|check/.test(raw)) return "Test";
  if (/review|human/.test(raw)) return "Review";
  if (/delivery|merge|done|handoff/.test(raw)) return "Delivery";
  return "Agent";
}

function tokenUsageFromRecord(record?: Record<string, unknown>): AgentTokenUsage | undefined {
  if (!record) return undefined;
  const usage = recordValue(record.usage) || recordValue(record.tokenUsage) || recordValue(record.tokens);
  const input =
    firstNumber(record, ["inputTokens", "promptTokens", "input_tokens", "prompt_tokens"]) ||
    firstNumber(usage, ["inputTokens", "promptTokens", "input_tokens", "prompt_tokens"]);
  const output =
    firstNumber(record, ["outputTokens", "completionTokens", "output_tokens", "completion_tokens"]) ||
    firstNumber(usage, ["outputTokens", "completionTokens", "output_tokens", "completion_tokens"]);
  const total =
    firstNumber(record, ["totalTokens", "total_tokens", "tokensTotal"]) ||
    firstNumber(usage, ["totalTokens", "total_tokens", "tokensTotal"]) ||
    ((input || output) ? (input ?? 0) + (output ?? 0) : 0);
  return total > 0 ? { input, output, total } : undefined;
}

function firstNumber(record: Record<string, unknown> | undefined, keys: string[]): number | undefined {
  if (!record) return undefined;
  for (const key of keys) {
    const value = record[key];
    if (typeof value === "number" && Number.isFinite(value) && value > 0) return value;
    if (typeof value === "string") {
      const parsed = Number(value);
      if (Number.isFinite(parsed) && parsed > 0) return parsed;
    }
  }
  return undefined;
}

function parseAgentOperationId(id: string): { stageId?: string; agentId?: string } {
  const match = id.match(/:agent:([^:]+):([^:]+)/);
  if (!match) return {};
  return { stageId: match[1], agentId: match[2] };
}

function stageAgentRuntimeDetail(agentLabel: string, runner?: string, model?: string, provider?: string): string {
  const runtimeParts = [runner, provider, model].map((value) => value?.trim()).filter(Boolean);
  if (!runtimeParts.length) return agentLabel;
  return `${agentLabel} (${runtimeParts.join(" · ")})`;
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
  const proofRecordBySource = new Map<string, ProofRecordInfo>();
  for (const record of proofRecords) {
    if (record.sourcePath) proofRecordBySource.set(record.sourcePath, record);
    if (record.sourceUrl) proofRecordBySource.set(record.sourceUrl, record);
  }
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
    const outputArtifacts = "outputArtifacts" in stage && Array.isArray(stage.outputArtifacts) ? stage.outputArtifacts : [];
    for (const proof of [...evidence, ...outputArtifacts]) {
      const record = proofRecordBySource.get(proof);
      if (!record && !isPreviewableProofReference(proof)) continue;
      addCard({
        id: record?.id,
        label: record?.value || record?.label || fileNameFromPath(proof),
        path: record?.sourcePath ?? proof,
        url: record?.sourceUrl,
        stage: stage.title ?? stage.id
      });
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

function isPreviewableProofReference(value: string): boolean {
  if (!value.trim()) return false;
  if (/^https?:\/\//i.test(value)) return true;
  if (value.startsWith("/") || value.startsWith("~") || value.startsWith(".")) return true;
  if (value.includes("/") || value.includes("\\")) return true;
  return /\.[a-z0-9]{2,8}$/i.test(value);
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
