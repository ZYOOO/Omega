export type RequirementSource = "workboard" | "github" | "feishu" | "manual";

export type Priority = "urgent" | "high" | "medium" | "low";

export interface DeliveryRequirement {
  id: string;
  identifier: string;
  title: string;
  description: string;
  source: RequirementSource;
  priority: Priority;
  requester: string;
  labels: string[];
  createdAt: string;
}

export type CapabilityKind = "mcp" | "skill";
export type CapabilityRisk = "read" | "write" | "admin";
export type PipelineStageId =
  | "intake"
  | "solution"
  | "coding"
  | "testing"
  | "review"
  | "delivery";

export interface Capability {
  id: string;
  kind: CapabilityKind;
  name: string;
  category: string;
  description: string;
  risk: CapabilityRisk;
  recommendedStages: PipelineStageId[];
  scopes: string[];
}

export type AgentId =
  | "master"
  | "requirement"
  | "architect"
  | "coding"
  | "testing"
  | "review"
  | "delivery";

export interface AgentDefinition {
  id: AgentId;
  name: string;
  role: string;
  mission: string;
  defaultCapabilities: string[];
  optionalCapabilities: string[];
  outputContract: string[];
  handoffCriteria: string[];
}

export type StageStatus =
  | "waiting"
  | "ready"
  | "running"
  | "needs-human"
  | "passed"
  | "failed"
  | "blocked";

export interface StageEvidence {
  id: string;
  label: string;
  value: string;
  url?: string;
}

export interface PipelineStage {
  id: PipelineStageId;
  title: string;
  description: string;
  ownerAgentId: AgentId;
  status: StageStatus;
  humanGate: boolean;
  acceptanceCriteria: string[];
  evidence: StageEvidence[];
  notes?: string;
  startedAt?: string;
  completedAt?: string;
  approvedBy?: string;
}

export type RunEventType =
  | "run.created"
  | "stage.started"
  | "stage.completed"
  | "stage.failed"
  | "gate.requested"
  | "gate.approved"
  | "capability.selected"
  | "master.decision";

export interface RunEvent {
  id: string;
  type: RunEventType;
  message: string;
  timestamp: string;
  stageId?: PipelineStageId;
  agentId?: AgentId;
}

export interface PipelineRun {
  id: string;
  requirement: DeliveryRequirement;
  goal: string;
  successCriteria: string[];
  stages: PipelineStage[];
  agents: AgentDefinition[];
  selectedCapabilities: Record<AgentId, string[]>;
  events: RunEvent[];
  createdAt: string;
  updatedAt: string;
}

export type StageCompletionResult =
  | {
      passed: true;
      notes: string;
      evidence: StageEvidence[];
    }
  | {
      passed: false;
      notes: string;
      evidence: StageEvidence[];
    };

export interface MasterDecision {
  action: "continue" | "request-human" | "rework" | "succeed";
  reason: string;
  nextStageId?: PipelineStageId;
  nextAgentId?: AgentId;
}
