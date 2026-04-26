import type { Capability, PipelineStageId } from "./types";

export const capabilityCatalog: Capability[] = [
  {
    id: "mcp.workboard",
    kind: "mcp",
    name: "Workboard MCP",
    category: "Tracker",
    description: "Read work items, sync comments, and move workflow states.",
    risk: "write",
    recommendedStages: ["intake", "solution", "review", "delivery"],
    scopes: ["work_items:read", "work_items:write", "comments:write"]
  },
  {
    id: "mcp.github",
    kind: "mcp",
    name: "GitHub MCP",
    category: "Code",
    description: "Read repositories, pull requests, diffs, reviews, and checks.",
    risk: "write",
    recommendedStages: ["coding", "testing", "review", "delivery"],
    scopes: ["repo:read", "pull_requests:write", "checks:read"]
  },
  {
    id: "mcp.feishu",
    kind: "mcp",
    name: "Feishu MCP",
    category: "IM",
    description: "Send approval cards, collect human decisions, and notify rooms.",
    risk: "write",
    recommendedStages: ["intake", "solution", "testing", "review", "delivery"],
    scopes: ["messages:write", "cards:write", "approval:read"]
  },
  {
    id: "mcp.ci",
    kind: "mcp",
    name: "CI MCP",
    category: "Quality",
    description: "Read CI runs, job logs, coverage reports, and retry safe checks.",
    risk: "write",
    recommendedStages: ["testing", "review", "delivery"],
    scopes: ["checks:read", "logs:read", "checks:rerun"]
  },
  {
    id: "mcp.release",
    kind: "mcp",
    name: "Release MCP",
    category: "Delivery",
    description: "Create releases, attach notes, and coordinate rollout windows.",
    risk: "admin",
    recommendedStages: ["delivery"],
    scopes: ["releases:write", "deployments:write"]
  },
  {
    id: "skill.requirement-analysis",
    kind: "skill",
    name: "Requirement Analysis",
    category: "Product",
    description: "Convert raw asks into user stories, boundaries, and acceptance criteria.",
    risk: "read",
    recommendedStages: ["intake"],
    scopes: ["analysis"]
  },
  {
    id: "skill.architecture",
    kind: "skill",
    name: "Architecture Design",
    category: "Design",
    description: "Produce interfaces, data contracts, risks, and implementation slices.",
    risk: "read",
    recommendedStages: ["solution"],
    scopes: ["design"]
  },
  {
    id: "skill.code",
    kind: "skill",
    name: "Coding Harness",
    category: "Implementation",
    description: "Guide an implementation agent through scoped code changes.",
    risk: "write",
    recommendedStages: ["coding"],
    scopes: ["files:write", "commands:run"]
  },
  {
    id: "skill.test-authoring",
    kind: "skill",
    name: "Test Authoring",
    category: "Quality",
    description: "Design unit, integration, and regression tests from acceptance criteria.",
    risk: "write",
    recommendedStages: ["testing"],
    scopes: ["tests:write", "coverage:read"]
  },
  {
    id: "skill.pr-review",
    kind: "skill",
    name: "PR Review",
    category: "Quality",
    description: "Inspect diffs for regressions, missing tests, maintainability, and security.",
    risk: "read",
    recommendedStages: ["review"],
    scopes: ["diffs:read", "reviews:read"]
  },
  {
    id: "skill.release-notes",
    kind: "skill",
    name: "Release Notes",
    category: "Delivery",
    description: "Generate release notes, rollout plans, and rollback playbooks.",
    risk: "write",
    recommendedStages: ["delivery"],
    scopes: ["docs:write", "notifications:write"]
  },
  {
    id: "skill.security-review",
    kind: "skill",
    name: "Security Review",
    category: "Risk",
    description: "Check permission changes, secrets, auth flows, and high-risk operations.",
    risk: "read",
    recommendedStages: ["solution", "review", "delivery"],
    scopes: ["security:read"]
  }
];

export function findCapability(id: string): Capability | undefined {
  return capabilityCatalog.find((capability) => capability.id === id);
}

export function recommendedCapabilitiesForStage(stageId: PipelineStageId): Capability[] {
  return capabilityCatalog.filter((capability) =>
    capability.recommendedStages.includes(stageId)
  );
}
