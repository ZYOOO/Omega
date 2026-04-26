import { mkdir, readFile, writeFile } from "fs/promises";
import { join } from "path";
import type { MissionEvent, MissionControlState, MissionRepository } from "../core";
import { applyMissionControlEvents } from "../core";

export class FileMissionRepository implements MissionRepository {
  constructor(private readonly root: string) {}

  async saveState(state: MissionControlState): Promise<void> {
    await mkdir(this.root, { recursive: true });
    await writeFile(this.pathFor(state.runId), JSON.stringify(state, null, 2));
  }

  async getState(runId: string): Promise<MissionControlState | undefined> {
    try {
      return JSON.parse(await readFile(this.pathFor(runId), "utf8")) as MissionControlState;
    } catch {
      return undefined;
    }
  }

  async appendEvent(runId: string, event: MissionEvent): Promise<MissionControlState> {
    const current = await this.getState(runId);
    if (!current) {
      throw new Error(`Mission Control state not found: ${runId}`);
    }

    const next = applyMissionControlEvents(current, [event]);
    await this.saveState(next);
    return next;
  }

  private pathFor(runId: string): string {
    return join(this.root, `${runId}.json`);
  }
}
