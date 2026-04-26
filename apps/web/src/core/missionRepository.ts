import type { MissionEvent } from "./missionEvents";
import type { MissionControlState } from "./missionControlState";
import { applyMissionControlEvents } from "./missionControlState";

export interface MissionRepository {
  saveState(state: MissionControlState): Promise<void>;
  getState(runId: string): Promise<MissionControlState | undefined>;
  appendEvent(runId: string, event: MissionEvent): Promise<MissionControlState>;
}

function cloneState(state: MissionControlState): MissionControlState {
  return structuredClone(state);
}

export class InMemoryMissionRepository implements MissionRepository {
  private readonly states = new Map<string, MissionControlState>();

  async saveState(state: MissionControlState): Promise<void> {
    this.states.set(state.runId, cloneState(state));
  }

  async getState(runId: string): Promise<MissionControlState | undefined> {
    const state = this.states.get(runId);
    return state ? cloneState(state) : undefined;
  }

  async appendEvent(runId: string, event: MissionEvent): Promise<MissionControlState> {
    const current = this.states.get(runId);
    if (!current) {
      throw new Error(`Mission Control state not found: ${runId}`);
    }

    const next = applyMissionControlEvents(current, [event]);
    this.states.set(runId, cloneState(next));
    return cloneState(next);
  }
}
