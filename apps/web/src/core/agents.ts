import type { AgentDefinition, AgentId, Capability } from "./types";

export const agentDefinitions: AgentDefinition[] = [
  {
    id: "master",
    name: "Master Agent",
    role: "Goal-driven workflow controller",
    mission: "Evaluate evidence against success criteria and decide whether to continue, rework, ask humans, or finish.",
    defaultCapabilities: ["mcp.workboard", "mcp.github", "mcp.ci", "skill.pr-review"],
    optionalCapabilities: ["mcp.feishu", "skill.security-review", "skill.release-notes"],
    outputContract: ["decision", "reason", "next stage or agent"],
    handoffCriteria: ["Every stage is passed", "Required gates are approved", "Delivery evidence exists"]
  },
  {
    id: "requirement",
    name: "Requirement Agent",
    role: "Requirement analyst",
    mission: "Turn messy input into a scoped requirement with clear acceptance criteria.",
    defaultCapabilities: ["mcp.workboard", "mcp.feishu", "skill.requirement-analysis"],
    optionalCapabilities: ["mcp.github"],
    outputContract: ["problem statement", "non-goals", "acceptance criteria"],
    handoffCriteria: ["Scope is testable", "Requester confirms value", "Unknowns are recorded"]
  },
  {
    id: "architect",
    name: "Architect Agent",
    role: "Solution designer",
    mission: "Design the interface, data flow, integration plan, and risk controls.",
    defaultCapabilities: ["mcp.github", "skill.architecture"],
    optionalCapabilities: ["mcp.workboard", "skill.security-review"],
    outputContract: ["architecture notes", "module plan", "risk list"],
    handoffCriteria: ["Interfaces are explicit", "Work can be sliced", "Risks have mitigations"]
  },
  {
    id: "coding",
    name: "Coding Agent",
    role: "Implementation worker",
    mission: "Implement the approved design in an isolated workspace and prepare a pull request.",
    defaultCapabilities: ["mcp.github", "skill.code", "skill.test-authoring"],
    optionalCapabilities: ["mcp.workboard", "mcp.ci"],
    outputContract: ["patch or PR", "implementation notes", "local test result"],
    handoffCriteria: ["Code compiles", "Core tests pass", "PR is ready for quality validation"]
  },
  {
    id: "testing",
    name: "Test Agent",
    role: "Quality validator",
    mission: "Author and run tests that prove the requirement is satisfied and regressions are covered.",
    defaultCapabilities: ["mcp.github", "mcp.ci", "skill.test-authoring"],
    optionalCapabilities: ["mcp.workboard", "skill.security-review"],
    outputContract: ["test plan", "coverage summary", "failing logs or pass evidence"],
    handoffCriteria: ["Acceptance criteria mapped to tests", "CI is green", "Coverage is acceptable"]
  },
  {
    id: "review",
    name: "Review Agent",
    role: "PR reviewer",
    mission: "Inspect implementation, tests, security, maintainability, and review comment closure.",
    defaultCapabilities: ["mcp.github", "mcp.ci", "skill.pr-review"],
    optionalCapabilities: ["mcp.workboard", "skill.security-review"],
    outputContract: ["review findings", "risk assessment", "required fixes"],
    handoffCriteria: ["No blocking findings", "CI is green", "Reviewer gate is approved"]
  },
  {
    id: "delivery",
    name: "Delivery Agent",
    role: "Release coordinator",
    mission: "Prepare release notes, rollout, rollback, and stakeholder notification.",
    defaultCapabilities: ["mcp.github", "mcp.feishu", "skill.release-notes"],
    optionalCapabilities: ["mcp.release", "mcp.workboard"],
    outputContract: ["release notes", "rollback plan", "delivery notification"],
    handoffCriteria: ["Release notes are clear", "Rollback is documented", "Human delivery approval exists"]
  }
];

export function getAgent(agentId: AgentId): AgentDefinition {
  const agent = agentDefinitions.find((candidate) => candidate.id === agentId);

  if (!agent) {
    throw new Error(`Unknown agent: ${agentId}`);
  }

  return agent;
}

export function capabilitiesForAgent(
  agent: AgentDefinition,
  catalog: Capability[]
): Capability[] {
  const allowed = new Set([...agent.defaultCapabilities, ...agent.optionalCapabilities]);
  return catalog.filter((capability) => allowed.has(capability.id));
}

export function hasCapability(agent: AgentDefinition, capabilityId: string): boolean {
  return (
    agent.defaultCapabilities.includes(capabilityId) ||
    agent.optionalCapabilities.includes(capabilityId)
  );
}
