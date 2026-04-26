import { createPipelineRun } from "./pipeline";
import type { DeliveryRequirement } from "./types";

export const sampleRequirement: DeliveryRequirement = {
  id: "req_omega_001",
  identifier: "OMEGA-1",
  title: "Make testing, review, and delivery explicit AI workflow stages",
  description:
    "The team wants to reduce the manual coordination cost across GitHub, CI, Feishu, and local runners while making testing, review, and delivery auditable.",
  source: "workboard",
  priority: "high",
  requester: "AI project competition team",
  labels: ["ai-first", "delivery", "workflow-engine"],
  createdAt: "2026-04-20T00:00:00.000Z"
};

export function createSampleRun() {
  const run = createPipelineRun(sampleRequirement);
  return {
    ...run,
    id: `run_${sampleRequirement.id}`
  };
}
