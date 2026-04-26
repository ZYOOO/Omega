import { describe, expect, it } from "vitest";
import { runOperationViaMissionControlApi } from "../missionControlApiClient";
import { createMissionFromRun, createSampleRun, createWorkItems } from "../core";

describe("runOperationViaMissionControlApi", () => {
  it("posts a mission operation to the local Mission Control API", async () => {
    const run = createSampleRun();
    const mission = createMissionFromRun(run, createWorkItems(run)[0]);
    const calls: Array<{ url: string; init: RequestInit }> = [];

    const response = await runOperationViaMissionControlApi({
      apiUrl: "http://127.0.0.1:3999",
      mission,
      operationId: "operation_intake",
      runner: "codex",
      fetchImpl: async (url, init) => {
        calls.push({ url: String(url), init: init ?? {} });
        return new Response(JSON.stringify({ status: "passed", proofFiles: ["proof.txt"] }), {
          status: 200,
          headers: { "content-type": "application/json" }
        });
      }
    });

    expect(response).toEqual({ status: "passed", proofFiles: ["proof.txt"] });
    expect(calls[0].url).toBe("http://127.0.0.1:3999/run-operation");
    expect(calls[0].init.method).toBe("POST");
    expect(JSON.parse(String(calls[0].init.body))).toMatchObject({
      operationId: "operation_intake",
      runner: "codex"
    });
  });
});
