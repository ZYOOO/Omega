import { describe, expect, it } from "vitest";
import { agentDefinitions, capabilitiesForAgent, capabilityCatalog, getAgent } from "..";

describe("agent definitions", () => {
  it("keeps every agent backed by selectable capabilities", () => {
    for (const agent of agentDefinitions) {
      const capabilities = capabilitiesForAgent(agent, capabilityCatalog);

      expect(capabilities.length).toBeGreaterThanOrEqual(agent.defaultCapabilities.length);
      expect(capabilities.map((capability) => capability.id)).toEqual(
        expect.arrayContaining(agent.defaultCapabilities)
      );
    }
  });

  it("models testing, review, and delivery as first-class agents", () => {
    expect(getAgent("testing").defaultCapabilities).toContain("skill.test-authoring");
    expect(getAgent("review").defaultCapabilities).toContain("skill.pr-review");
    expect(getAgent("delivery").defaultCapabilities).toContain("skill.release-notes");
  });
});
