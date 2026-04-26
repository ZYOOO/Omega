import { agentDefinitions, hasCapability } from "./agents";
import { capabilityCatalog, findCapability } from "./catalog";
import type {
  AgentId,
  DeliveryRequirement,
  PipelineRun,
  PipelineStage,
  PipelineStageId,
  RunEvent,
  StageCompletionResult
} from "./types";

const stageOrder: PipelineStageId[] = [
  "intake",
  "solution",
  "coding",
  "testing",
  "review",
  "delivery"
];

function now(): string {
  return new Date().toISOString();
}

function makeEvent(
  type: RunEvent["type"],
  message: string,
  stageId?: PipelineStageId,
  agentId?: AgentId
): RunEvent {
  return {
    id: crypto.randomUUID(),
    type,
    message,
    timestamp: now(),
    stageId,
    agentId
  };
}

export function createDefaultStages(): PipelineStage[] {
  return [
    {
      id: "intake",
      title: "Intake",
      description: "Clarify problem, value, scope, non-goals, and acceptance criteria.",
      ownerAgentId: "requirement",
      status: "ready",
      humanGate: true,
      acceptanceCriteria: ["Scope is clear", "Acceptance criteria are testable", "Open questions are recorded"],
      evidence: []
    },
    {
      id: "solution",
      title: "Solution",
      description: "Define architecture, interfaces, modules, risks, and test strategy.",
      ownerAgentId: "architect",
      status: "waiting",
      humanGate: true,
      acceptanceCriteria: ["Interfaces are explicit", "Work is sliceable", "Risks have mitigations"],
      evidence: []
    },
    {
      id: "coding",
      title: "Implementation",
      description: "Implement the approved change inside an isolated workspace and prepare a PR.",
      ownerAgentId: "coding",
      status: "waiting",
      humanGate: false,
      acceptanceCriteria: ["Implementation matches design", "Local checks pass", "Diff is reviewable"],
      evidence: []
    },
    {
      id: "testing",
      title: "Testing",
      description: "Create a test plan, run checks and coverage, and explain failures.",
      ownerAgentId: "testing",
      status: "waiting",
      humanGate: true,
      acceptanceCriteria: ["Criteria map to tests", "CI is green", "Coverage meets threshold"],
      evidence: []
    },
    {
      id: "review",
      title: "Review",
      description: "Inspect PR diff, review comments, risk, security, and maintainability.",
      ownerAgentId: "review",
      status: "waiting",
      humanGate: true,
      acceptanceCriteria: ["No blocking review comments", "Security risk is acceptable", "CI remains green"],
      evidence: []
    },
    {
      id: "delivery",
      title: "Delivery",
      description: "Prepare release notes, rollback strategy, and stakeholder notification.",
      ownerAgentId: "delivery",
      status: "waiting",
      humanGate: true,
      acceptanceCriteria: ["Release notes are complete", "Rollback plan is clear", "Stakeholders are notified"],
      evidence: []
    }
  ];
}

export function createPipelineRun(requirement: DeliveryRequirement): PipelineRun {
  const selectedCapabilities = Object.fromEntries(
    agentDefinitions.map((agent) => [agent.id, [...agent.defaultCapabilities]])
  ) as PipelineRun["selectedCapabilities"];

  const createdAt = now();

  return {
    id: crypto.randomUUID(),
    requirement,
    goal: `Deliver ${requirement.identifier}: ${requirement.title}`,
    successCriteria: [
      "All pipeline stages are passed",
      "All human gates are approved",
      "Testing and review evidence is attached",
      "Delivery notes and rollback plan are attached"
    ],
    stages: createDefaultStages(),
    agents: agentDefinitions,
    selectedCapabilities,
    events: [
      makeEvent(
        "run.created",
        `Pipeline created for ${requirement.identifier}`,
        "intake",
        "master"
      )
    ],
    createdAt,
    updatedAt: createdAt
  };
}

export class PipelineEngine {
  constructor(private readonly run: PipelineRun) {}

  snapshot(): PipelineRun {
    return structuredClone(this.run);
  }

  startStage(stageId: PipelineStageId): PipelineRun {
    const stage = this.findStage(stageId);

    if (stage.status !== "ready") {
      throw new Error(`Stage ${stageId} is not ready`);
    }

    stage.status = "running";
    stage.startedAt = now();
    this.pushEvent("stage.started", `${stage.title} started`, stageId, stage.ownerAgentId);
    return this.touch();
  }

  completeStage(stageId: PipelineStageId, result: StageCompletionResult): PipelineRun {
    const stage = this.findStage(stageId);

    if (stage.status !== "running") {
      throw new Error(`Stage ${stageId} is not running`);
    }

    stage.notes = result.notes;
    stage.evidence = [...stage.evidence, ...result.evidence];
    stage.completedAt = now();

    if (!result.passed) {
      stage.status = "failed";
      this.pushEvent("stage.failed", `${stage.title} failed: ${result.notes}`, stageId, stage.ownerAgentId);
      return this.touch();
    }

    if (stage.humanGate) {
      stage.status = "needs-human";
      this.pushEvent(
        "gate.requested",
        `${stage.title} is waiting for human approval`,
        stageId,
        stage.ownerAgentId
      );
      return this.touch();
    }

    stage.status = "passed";
    this.pushEvent("stage.completed", `${stage.title} passed`, stageId, stage.ownerAgentId);
    this.markNextStageReady(stageId);
    return this.touch();
  }

  approveHumanGate(stageId: PipelineStageId, reviewer: string): PipelineRun {
    const stage = this.findStage(stageId);

    if (stage.status !== "needs-human") {
      throw new Error(`Stage ${stageId} is not waiting for human approval`);
    }

    stage.status = "passed";
    stage.approvedBy = reviewer;
    this.pushEvent("gate.approved", `${reviewer} approved ${stage.title}`, stageId, "master");
    this.markNextStageReady(stageId);
    return this.touch();
  }

  selectCapability(agentId: AgentId, capabilityId: string): PipelineRun {
    const agent = this.run.agents.find((candidate) => candidate.id === agentId);
    const capability = findCapability(capabilityId);

    if (!agent) {
      throw new Error(`Unknown agent: ${agentId}`);
    }

    if (!capability) {
      throw new Error(`Unknown capability: ${capabilityId}`);
    }

    if (!hasCapability(agent, capabilityId)) {
      throw new Error(`${agent.name} cannot use ${capability.name}`);
    }

    const selected = new Set(this.run.selectedCapabilities[agentId] ?? []);
    selected.add(capabilityId);
    this.run.selectedCapabilities[agentId] = [...selected];
    this.pushEvent("capability.selected", `${agent.name} selected ${capability.name}`, undefined, agentId);
    return this.touch();
  }

  deselectCapability(agentId: AgentId, capabilityId: string): PipelineRun {
    const agent = this.run.agents.find((candidate) => candidate.id === agentId);

    if (!agent) {
      throw new Error(`Unknown agent: ${agentId}`);
    }

    if (agent.defaultCapabilities.includes(capabilityId)) {
      throw new Error(`Cannot remove default capability ${capabilityId} from ${agent.name}`);
    }

    this.run.selectedCapabilities[agentId] = (this.run.selectedCapabilities[agentId] ?? []).filter(
      (selectedId) => selectedId !== capabilityId
    );
    return this.touch();
  }

  readyStages(): PipelineStage[] {
    return this.run.stages.filter((stage) => stage.status === "ready");
  }

  private findStage(stageId: PipelineStageId): PipelineStage {
    const stage = this.run.stages.find((candidate) => candidate.id === stageId);

    if (!stage) {
      throw new Error(`Unknown stage: ${stageId}`);
    }

    return stage;
  }

  private markNextStageReady(stageId: PipelineStageId): void {
    const currentIndex = stageOrder.indexOf(stageId);
    const nextStageId = stageOrder[currentIndex + 1];

    if (!nextStageId) {
      return;
    }

    const nextStage = this.findStage(nextStageId);
    if (nextStage.status === "waiting") {
      nextStage.status = "ready";
    }
  }

  private pushEvent(
    type: RunEvent["type"],
    message: string,
    stageId?: PipelineStageId,
    agentId?: AgentId
  ): void {
    this.run.events.unshift(makeEvent(type, message, stageId, agentId));
  }

  private touch(): PipelineRun {
    this.run.updatedAt = now();
    return this.snapshot();
  }
}

export function capabilityRiskCount(run: PipelineRun): Record<string, number> {
  return Object.values(run.selectedCapabilities)
    .flat()
    .reduce<Record<string, number>>(
      (counts, capabilityId) => {
        const capability = capabilityCatalog.find((candidate) => candidate.id === capabilityId);
        if (capability) {
          counts[capability.risk] += 1;
        }
        return counts;
      },
      { read: 0, write: 0, admin: 0 }
    );
}
