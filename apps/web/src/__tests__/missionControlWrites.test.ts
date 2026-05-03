import { describe, expect, it } from "vitest";

import { missionControlUnavailableMessage, requireMissionControlApi } from "../missionControlWrites";

describe("missionControlWrites", () => {
  it("requires the local runtime before canonical workspace writes", () => {
    expect(requireMissionControlApi("/api", "Creating a requirement")).toBe("/api");
    expect(() => requireMissionControlApi("", "Creating a requirement")).toThrow(/Mission Control is the only writer/);
    expect(missionControlUnavailableMessage("Running a work item")).toContain("local runtime");
  });
});
