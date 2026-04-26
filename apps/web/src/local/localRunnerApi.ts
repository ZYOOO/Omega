import { createServer } from "http";
import { readFile } from "fs/promises";
import { LocalRunnerBridge, type RunOperationRequest, type RunOperationResponse } from "./localRunnerBridge";
import type { LocalRunnerCommand } from "./localMissionRunner";
import { createCodexCommand } from "./codexCommand";
import { SqliteWorkspaceRepository } from "./sqliteWorkspaceRepository";
import { getGhStatus, getGitHubRepoInfo, importGitHubIssuesAsWorkItems } from "./ghCli";
import {
  appendWorkItemToDatabase,
  applyMissionEventsToDatabase,
  appendMissionToDatabase,
  appendPipelineToDatabase,
  appendProofRecordToDatabase,
  createPipelineRun,
  createMissionFromRun,
  createSampleRun,
  nowIso,
  type Mission,
  type MissionOperation,
  type MissionRecord,
  type OperationRecord,
  type ProofRecord,
  type CheckpointRecord,
  type PipelineRecord,
  type PipelineLifecycleStatus,
  type PipelineRun,
  upsertCheckpointInDatabase,
  updateWorkItemInDatabase,
  updatePipelineInDatabase,
  upsertOperationInDatabase,
  type WorkItem,
  type WorkspaceDatabase
} from "../core";
import { join, resolve } from "path";

export interface LocalRunnerApiOptions {
  workspaceRoot: string;
  port: number;
  host?: string;
  databasePath?: string;
}

export interface LocalRunnerApiServer {
  url: string;
  close(): Promise<void>;
}

type RunnerPreset = "local-proof" | "codex";

type ApiRunOperationRequest = Omit<RunOperationRequest, "command"> & {
  command?: LocalRunnerCommand;
  runner?: RunnerPreset;
};

export function commandFromRunnerPreset(runner: RunnerPreset): LocalRunnerCommand {
  if (runner === "local-proof") {
    return {
      executable: process.execPath,
      args: [
        "-e",
        "const fs=require('fs'); fs.writeFileSync('.omega/proof/local-proof.txt','local proof'); console.log('local proof complete')"
      ]
    };
  }
  if (runner === "codex") {
    return createCodexCommand({
      promptFilePath: ".omega/prompt.md",
      model: "gpt-5.4-mini",
      reasoningEffort: "medium"
    });
  }
  throw new Error(`Unknown runner preset: ${runner}`);
}

function readRequestBody(request: { on(event: string, listener: (chunk?: unknown) => void): void }): Promise<string> {
  return new Promise((resolve) => {
    let body = "";
    request.on("data", (chunk) => {
      body += String(chunk);
    });
    request.on("end", () => resolve(body));
  });
}

function setCors(response: { setHeader(name: string, value: string): void }): void {
  response.setHeader("access-control-allow-origin", "*");
  response.setHeader("access-control-allow-methods", "GET,POST,PUT,PATCH,OPTIONS");
  response.setHeader("access-control-allow-headers", "content-type");
}

function sendJson(response: { statusCode: number; setHeader(name: string, value: string): void; end(body: string): void }, statusCode: number, body: unknown): void {
  response.statusCode = statusCode;
  setCors(response);
  response.setHeader("content-type", "application/json");
  response.end(JSON.stringify(body));
}

function requirementFromWorkItem(item: WorkItem): PipelineRun["requirement"] {
  return {
    id: `req_${item.id}`,
    identifier: item.key,
    title: item.title,
    description: item.description,
    source: "manual",
    priority: item.priority === "Urgent" || item.priority === "High"
      ? "high"
      : item.priority === "Medium"
        ? "medium"
        : "low",
    requester: item.assignee,
    labels: item.labels,
    createdAt: nowIso()
  };
}

function makePipelineRecord(item: WorkItem): PipelineRecord {
  const timestamp = nowIso();
  const run = createPipelineRun(requirementFromWorkItem(item));
  return {
    id: `pipeline_${item.id}`,
    workItemId: item.id,
    runId: run.id,
    status: "draft",
    run,
    createdAt: timestamp,
    updatedAt: timestamp
  };
}

function missionRecordFromMission(pipelineId: string, workItemId: string, mission: Mission): MissionRecord {
  const timestamp = nowIso();
  return {
    id: mission.id,
    pipelineId,
    workItemId,
    title: mission.title,
    status: mission.status,
    mission,
    createdAt: timestamp,
    updatedAt: timestamp
  };
}

function operationRecordFromMission(missionId: string, operation: MissionOperation): OperationRecord {
  const timestamp = nowIso();
  return {
    id: operation.id,
    missionId,
    stageId: operation.stageId,
    agentId: operation.agentId,
    status: operation.status,
    prompt: operation.prompt,
    requiredProof: operation.requiredProof,
    createdAt: timestamp,
    updatedAt: timestamp
  };
}

function pendingCheckpointFromPipeline(pipeline: PipelineRecord): CheckpointRecord | undefined {
  const stage = pipeline.run.stages.find((candidate) => candidate.status === "needs-human");
  if (!stage) return undefined;
  const timestamp = nowIso();
  return {
    id: `${pipeline.id}:${stage.id}`,
    pipelineId: pipeline.id,
    stageId: stage.id,
    status: "pending",
    title: `${stage.title} 审批`,
    summary: `${stage.title} 需要人工确认后才能继续`,
    createdAt: timestamp,
    updatedAt: timestamp
  };
}

function replacePipelineRun(
  database: WorkspaceDatabase,
  pipelineId: string,
  nextRun: PipelineRun,
  nextStatus: PipelineLifecycleStatus,
  checkpointDecision?: { status: "approved" | "rejected"; note?: string; stageId?: string }
): WorkspaceDatabase {
  const updated = updatePipelineInDatabase(database, pipelineId, (pipeline) => ({
    ...pipeline,
    run: nextRun,
    status: nextStatus,
    updatedAt: nowIso()
  }));

  const pending = pendingCheckpointFromPipeline(updated.tables.pipelines.find((pipeline) => pipeline.id === pipelineId)!);
  let nextDatabase = updated;
  if (pending) {
    nextDatabase = upsertCheckpointInDatabase(nextDatabase, pending);
  }
  if (checkpointDecision && pending) {
    nextDatabase = upsertCheckpointInDatabase(nextDatabase, {
      ...pending,
      status: checkpointDecision.status,
      decisionNote: checkpointDecision.note,
      updatedAt: nowIso()
    });
  }
  if (checkpointDecision && !pending && checkpointDecision.stageId) {
    const existing = (nextDatabase.tables.checkpoints ?? []).find((checkpoint) =>
      checkpoint.pipelineId === pipelineId && checkpoint.stageId === checkpointDecision.stageId
    );
    if (existing) {
      nextDatabase = upsertCheckpointInDatabase(nextDatabase, {
        ...existing,
        status: checkpointDecision.status,
        decisionNote: checkpointDecision.note,
        updatedAt: nowIso()
      });
    }
  }
  return nextDatabase;
}

export function startLocalRunnerApi(options: LocalRunnerApiOptions): Promise<LocalRunnerApiServer> {
  const bridge = new LocalRunnerBridge(options.workspaceRoot);
  const workspaceRepository = options.databasePath ? new SqliteWorkspaceRepository(options.databasePath) : undefined;
  const host = options.host ?? "127.0.0.1";
  const openApiPath = resolve(join(process.cwd(), "docs", "openapi.yaml"));

  return new Promise((resolve) => {
    const server = createServer(async (request, response) => {
      if (request.method === "OPTIONS") {
        response.statusCode = 204;
        setCors(response);
        response.end("");
        return;
      }

      if (request.method === "GET" && request.url === "/health") {
        sendJson(response, 200, {
          ok: true,
          persistence: workspaceRepository ? "sqlite" : "memory"
        });
        return;
      }

      if (request.method === "GET" && request.url === "/openapi.yaml") {
        try {
          const body = await readFile(openApiPath, "utf8");
          response.statusCode = 200;
          setCors(response);
          response.setHeader("content-type", "application/yaml; charset=utf-8");
          response.end(body);
        } catch (error) {
          sendJson(response, 500, { error: error instanceof Error ? error.message : "unknown error" });
        }
        return;
      }

      if (request.method === "GET" && request.url === "/workspace") {
        if (!workspaceRepository) {
          sendJson(response, 404, { error: "workspace persistence is not configured" });
          return;
        }

        try {
          const database = await workspaceRepository.getDatabase();
          if (!database) {
            sendJson(response, 404, { error: "workspace not found" });
            return;
          }

          sendJson(response, 200, database);
        } catch (error) {
          sendJson(response, 500, { error: error instanceof Error ? error.message : "unknown error" });
        }
        return;
      }

      if (request.method === "PUT" && request.url === "/workspace") {
        if (!workspaceRepository) {
          sendJson(response, 404, { error: "workspace persistence is not configured" });
          return;
        }

        try {
          const rawBody = await readRequestBody(request);
          const database = JSON.parse(rawBody) as WorkspaceDatabase;
          await workspaceRepository.saveDatabase(database);
          sendJson(response, 200, { ok: true });
        } catch (error) {
          sendJson(response, 500, { error: error instanceof Error ? error.message : "unknown error" });
        }
        return;
      }

      if (request.method === "GET" && request.url === "/events") {
        if (!workspaceRepository) {
          sendJson(response, 404, { error: "workspace persistence is not configured" });
          return;
        }

        try {
          const database = await workspaceRepository.getDatabase();
          sendJson(response, 200, database?.tables.missionEvents ?? []);
        } catch (error) {
          sendJson(response, 500, { error: error instanceof Error ? error.message : "unknown error" });
        }
        return;
      }

      if (request.method === "GET" && request.url === "/github/status") {
        try {
          sendJson(response, 200, await getGhStatus());
        } catch (error) {
          sendJson(response, 500, { error: error instanceof Error ? error.message : "unknown error" });
        }
        return;
      }

      if (request.method === "POST" && request.url === "/github/repo-info") {
        try {
          const payload = JSON.parse(await readRequestBody(request)) as { owner: string; repo: string };
          sendJson(response, 200, await getGitHubRepoInfo(payload.owner, payload.repo));
        } catch (error) {
          sendJson(response, 500, { error: error instanceof Error ? error.message : "unknown error" });
        }
        return;
      }

      if (request.method === "POST" && request.url === "/github/import-issues") {
        if (!workspaceRepository) {
          sendJson(response, 404, { error: "workspace persistence is not configured" });
          return;
        }

        try {
          const payload = JSON.parse(await readRequestBody(request)) as { owner: string; repo: string };
          const database = await workspaceRepository.getDatabase();
          if (!database) {
            sendJson(response, 404, { error: "workspace not found" });
            return;
          }

          const imported = await importGitHubIssuesAsWorkItems(payload.owner, payload.repo);
          let next = database;
          for (const item of imported.filter((item) => !database.tables.workItems.some((record) => record.id === item.id))) {
            next = appendWorkItemToDatabase(next, item);
          }
          await workspaceRepository.saveDatabase(next);
          sendJson(response, 200, next);
        } catch (error) {
          sendJson(response, 500, { error: error instanceof Error ? error.message : "unknown error" });
        }
        return;
      }

      if (request.method === "GET" && request.url === "/pipelines") {
        if (!workspaceRepository) {
          sendJson(response, 404, { error: "workspace persistence is not configured" });
          return;
        }
        try {
          const database = await workspaceRepository.getDatabase();
          sendJson(response, 200, database?.tables.pipelines ?? []);
        } catch (error) {
          sendJson(response, 500, { error: error instanceof Error ? error.message : "unknown error" });
        }
        return;
      }

      if (request.method === "GET" && request.url === "/checkpoints") {
        if (!workspaceRepository) {
          sendJson(response, 404, { error: "workspace persistence is not configured" });
          return;
        }
        try {
          const database = await workspaceRepository.getDatabase();
          sendJson(response, 200, database?.tables.checkpoints ?? []);
        } catch (error) {
          sendJson(response, 500, { error: error instanceof Error ? error.message : "unknown error" });
        }
        return;
      }

      if (request.method === "POST" && request.url === "/pipelines/from-work-item") {
        if (!workspaceRepository) {
          sendJson(response, 404, { error: "workspace persistence is not configured" });
          return;
        }
        try {
          const database = await workspaceRepository.getDatabase();
          if (!database) {
            sendJson(response, 404, { error: "workspace not found" });
            return;
          }
          const payload = JSON.parse(await readRequestBody(request)) as { item: WorkItem };
          const pipeline = makePipelineRecord(payload.item);
          const next = appendPipelineToDatabase(database, pipeline);
          await workspaceRepository.saveDatabase(next);
          sendJson(response, 200, pipeline);
        } catch (error) {
          sendJson(response, 500, { error: error instanceof Error ? error.message : "unknown error" });
        }
        return;
      }

      if (request.method === "POST" && request.url?.startsWith("/pipelines/") && request.url.endsWith("/start")) {
        if (!workspaceRepository) {
          sendJson(response, 404, { error: "workspace persistence is not configured" });
          return;
        }
        try {
          const pipelineId = request.url.split("/")[2];
          const database = await workspaceRepository.getDatabase();
          const pipeline = database?.tables.pipelines.find((candidate) => candidate.id === pipelineId);
          if (!database || !pipeline) {
            sendJson(response, 404, { error: "pipeline not found" });
            return;
          }
          const { PipelineEngine } = await import("../core/pipeline");
          const engine = new PipelineEngine(structuredClone(pipeline.run));
          const nextStage = engine.readyStages()[0];
          const nextRun = nextStage ? engine.startStage(nextStage.id) : engine.snapshot();
          const next = replacePipelineRun(database, pipelineId, nextRun, "running");
          await workspaceRepository.saveDatabase(next);
          sendJson(response, 200, next.tables.pipelines.find((candidate) => candidate.id === pipelineId));
        } catch (error) {
          sendJson(response, 500, { error: error instanceof Error ? error.message : "unknown error" });
        }
        return;
      }

      if (request.method === "POST" && request.url?.startsWith("/pipelines/") && request.url.endsWith("/complete-stage")) {
        if (!workspaceRepository) {
          sendJson(response, 404, { error: "workspace persistence is not configured" });
          return;
        }
        try {
          const pipelineId = request.url.split("/")[2];
          const database = await workspaceRepository.getDatabase();
          const pipeline = database?.tables.pipelines.find((candidate) => candidate.id === pipelineId);
          if (!database || !pipeline) {
            sendJson(response, 404, { error: "pipeline not found" });
            return;
          }
          const payload = JSON.parse(await readRequestBody(request)) as { passed?: boolean; notes?: string };
          const { PipelineEngine } = await import("../core/pipeline");
          const engine = new PipelineEngine(structuredClone(pipeline.run));
          const runningStage = engine.snapshot().stages.find((stage) => stage.status === "running");
          if (!runningStage) {
            sendJson(response, 400, { error: "no running stage" });
            return;
          }
          const nextRun = engine.completeStage(runningStage.id, {
            passed: payload.passed ?? true,
            notes: payload.notes ?? "Completed by API.",
            evidence: []
          });
          const nextStatus: PipelineLifecycleStatus =
            nextRun.stages.some((stage) => stage.status === "needs-human")
              ? "waiting-human"
              : nextRun.stages.every((stage) => stage.status === "passed")
                ? "completed"
                : (payload.passed ?? true)
                  ? "running"
                  : "failed";
          const next = replacePipelineRun(database, pipelineId, nextRun, nextStatus);
          await workspaceRepository.saveDatabase(next);
          sendJson(response, 200, next.tables.pipelines.find((candidate) => candidate.id === pipelineId));
        } catch (error) {
          sendJson(response, 500, { error: error instanceof Error ? error.message : "unknown error" });
        }
        return;
      }

      if (request.method === "POST" && request.url?.startsWith("/pipelines/") && request.url.endsWith("/pause")) {
        if (!workspaceRepository) {
          sendJson(response, 404, { error: "workspace persistence is not configured" });
          return;
        }
        try {
          const pipelineId = request.url.split("/")[2];
          const database = await workspaceRepository.getDatabase();
          if (!database) {
            sendJson(response, 404, { error: "workspace not found" });
            return;
          }
          const next = updatePipelineInDatabase(database, pipelineId, (pipeline) => ({
            ...pipeline,
            status: "paused",
            updatedAt: nowIso()
          }));
          await workspaceRepository.saveDatabase(next);
          sendJson(response, 200, next.tables.pipelines.find((candidate) => candidate.id === pipelineId));
        } catch (error) {
          sendJson(response, 500, { error: error instanceof Error ? error.message : "unknown error" });
        }
        return;
      }

      if (request.method === "POST" && request.url?.startsWith("/pipelines/") && request.url.endsWith("/resume")) {
        if (!workspaceRepository) {
          sendJson(response, 404, { error: "workspace persistence is not configured" });
          return;
        }
        try {
          const pipelineId = request.url.split("/")[2];
          const database = await workspaceRepository.getDatabase();
          const pipeline = database?.tables.pipelines.find((candidate) => candidate.id === pipelineId);
          if (!database || !pipeline) {
            sendJson(response, 404, { error: "pipeline not found" });
            return;
          }
          const next = updatePipelineInDatabase(database, pipelineId, (current) => ({
            ...current,
            status: "running",
            updatedAt: nowIso()
          }));
          await workspaceRepository.saveDatabase(next);
          sendJson(response, 200, next.tables.pipelines.find((candidate) => candidate.id === pipelineId));
        } catch (error) {
          sendJson(response, 500, { error: error instanceof Error ? error.message : "unknown error" });
        }
        return;
      }

      if (request.method === "POST" && request.url?.startsWith("/pipelines/") && request.url.endsWith("/terminate")) {
        if (!workspaceRepository) {
          sendJson(response, 404, { error: "workspace persistence is not configured" });
          return;
        }
        try {
          const pipelineId = request.url.split("/")[2];
          const database = await workspaceRepository.getDatabase();
          if (!database) {
            sendJson(response, 404, { error: "workspace not found" });
            return;
          }
          const next = updatePipelineInDatabase(database, pipelineId, (pipeline) => ({
            ...pipeline,
            status: "terminated",
            updatedAt: nowIso()
          }));
          await workspaceRepository.saveDatabase(next);
          sendJson(response, 200, next.tables.pipelines.find((candidate) => candidate.id === pipelineId));
        } catch (error) {
          sendJson(response, 500, { error: error instanceof Error ? error.message : "unknown error" });
        }
        return;
      }

      if (request.method === "POST" && request.url?.startsWith("/checkpoints/") && request.url.endsWith("/approve")) {
        if (!workspaceRepository) {
          sendJson(response, 404, { error: "workspace persistence is not configured" });
          return;
        }
        try {
          const checkpointId = request.url.split("/")[2];
          const database = await workspaceRepository.getDatabase();
          const checkpoint = database?.tables.checkpoints.find((candidate) => candidate.id === checkpointId);
          const pipeline = checkpoint ? database?.tables.pipelines.find((candidate) => candidate.id === checkpoint.pipelineId) : undefined;
          if (!database || !checkpoint || !pipeline) {
            sendJson(response, 404, { error: "checkpoint not found" });
            return;
          }
          const payload = JSON.parse(await readRequestBody(request)) as { reviewer?: string };
          const { PipelineEngine } = await import("../core/pipeline");
          const engine = new PipelineEngine(structuredClone(pipeline.run));
          const nextRun = engine.approveHumanGate(checkpoint.stageId, payload.reviewer ?? "reviewer");
          let next = replacePipelineRun(database, pipeline.id, nextRun, "running", { status: "approved", stageId: checkpoint.stageId });
          const readyStage = nextRun.stages.find((stage) => stage.status === "ready");
          if (readyStage) {
            next = updatePipelineInDatabase(next, pipeline.id, (current) => ({ ...current, status: "running", updatedAt: nowIso() }));
          }
          await workspaceRepository.saveDatabase(next);
          sendJson(response, 200, next.tables.checkpoints.find((candidate) => candidate.id === checkpointId));
        } catch (error) {
          sendJson(response, 500, { error: error instanceof Error ? error.message : "unknown error" });
        }
        return;
      }

      if (request.method === "POST" && request.url?.startsWith("/checkpoints/") && request.url.endsWith("/request-changes")) {
        if (!workspaceRepository) {
          sendJson(response, 404, { error: "workspace persistence is not configured" });
          return;
        }
        try {
          const checkpointId = request.url.split("/")[2];
          const database = await workspaceRepository.getDatabase();
          const checkpoint = database?.tables.checkpoints.find((candidate) => candidate.id === checkpointId);
          const pipeline = checkpoint ? database?.tables.pipelines.find((candidate) => candidate.id === checkpoint.pipelineId) : undefined;
          if (!database || !checkpoint || !pipeline) {
            sendJson(response, 404, { error: "checkpoint not found" });
            return;
          }
          const payload = JSON.parse(await readRequestBody(request)) as { reason?: string };
          const nextRun = {
            ...pipeline.run,
            stages: pipeline.run.stages.map((stage) =>
              stage.id === checkpoint.stageId
                ? { ...stage, status: "ready" as const, approvedBy: undefined }
                : stage
            )
          };
          const next = replacePipelineRun(database, pipeline.id, nextRun, "running", {
            status: "rejected",
            note: payload.reason,
            stageId: checkpoint.stageId
          });
          await workspaceRepository.saveDatabase(next);
          sendJson(response, 200, next.tables.checkpoints.find((candidate) => candidate.id === checkpointId));
        } catch (error) {
          sendJson(response, 500, { error: error instanceof Error ? error.message : "unknown error" });
        }
        return;
      }

      if (request.method === "POST" && request.url === "/work-items") {
        if (!workspaceRepository) {
          sendJson(response, 404, { error: "workspace persistence is not configured" });
          return;
        }

        try {
          const database = await workspaceRepository.getDatabase();
          if (!database) {
            sendJson(response, 404, { error: "workspace not found" });
            return;
          }
          const rawBody = await readRequestBody(request);
          const payload = JSON.parse(rawBody) as { item: WorkItem };
          const next = appendWorkItemToDatabase(database, payload.item);
          await workspaceRepository.saveDatabase(next);
          sendJson(response, 200, next);
        } catch (error) {
          sendJson(response, 500, { error: error instanceof Error ? error.message : "unknown error" });
        }
        return;
      }

      if (request.method === "PATCH" && request.url?.startsWith("/work-items/")) {
        if (!workspaceRepository) {
          sendJson(response, 404, { error: "workspace persistence is not configured" });
          return;
        }

        try {
          const itemId = request.url.split("/")[2];
          const database = await workspaceRepository.getDatabase();
          if (!database) {
            sendJson(response, 404, { error: "workspace not found" });
            return;
          }
          const rawBody = await readRequestBody(request);
          const patch = JSON.parse(rawBody) as Partial<Pick<WorkItem, "status" | "priority">>;
          const next = updateWorkItemInDatabase(database, itemId, patch);
          await workspaceRepository.saveDatabase(next);
          sendJson(response, 200, next);
        } catch (error) {
          sendJson(response, 500, { error: error instanceof Error ? error.message : "unknown error" });
        }
        return;
      }

      if (request.method === "POST" && request.url === "/missions/from-work-item") {
        try {
          const rawBody = await readRequestBody(request);
          const payload = JSON.parse(rawBody) as { item: WorkItem };
          const mission = createMissionFromRun(createSampleRun(), payload.item);
          if (workspaceRepository) {
            const database = await workspaceRepository.getDatabase();
            if (database) {
              const pipeline = database.tables.pipelines.find((candidate) => candidate.workItemId === payload.item.id);
              if (!database.tables.missions.some((candidate) => candidate.id === mission.id)) {
                let next = appendMissionToDatabase(
                  database,
                  missionRecordFromMission(
                    pipeline?.id ?? `pipeline_${payload.item.id}`,
                    payload.item.id,
                    mission
                  )
                );
                for (const operation of mission.operations) {
                  next = upsertOperationInDatabase(next, operationRecordFromMission(mission.id, operation));
                }
                await workspaceRepository.saveDatabase(next);
              }
            }
          }
          sendJson(response, 200, mission);
        } catch (error) {
          sendJson(response, 500, { error: error instanceof Error ? error.message : "unknown error" });
        }
        return;
      }

      if (request.method === "POST" && request.url === "/operations/run") {
        try {
          const rawBody = await readRequestBody(request);
          const payload = JSON.parse(rawBody) as ApiRunOperationRequest;
          const command = payload.command ?? (payload.runner ? commandFromRunnerPreset(payload.runner) : undefined);
          if (!command) {
            sendJson(response, 400, { error: "command or runner preset is required" });
            return;
          }
          const result: RunOperationResponse = await bridge.runOperation({ ...payload, command });

          if (workspaceRepository) {
            const database = await workspaceRepository.getDatabase();
            if (database) {
              let next = applyMissionEventsToDatabase(database, result.events);
              const pipeline = next.tables.pipelines.find((candidate) => candidate.workItemId === payload.mission.sourceWorkItemId);
              if (!next.tables.missions.some((candidate) => candidate.id === payload.mission.id)) {
                next = appendMissionToDatabase(
                  next,
                  missionRecordFromMission(
                    pipeline?.id ?? `pipeline_${payload.mission.sourceWorkItemId}`,
                    payload.mission.sourceWorkItemId,
                    payload.mission
                  )
                );
              }
              const operation = payload.mission.operations.find((candidate) => candidate.id === payload.operationId);
              if (operation) {
                next = upsertOperationInDatabase(next, {
                  ...operationRecordFromMission(payload.mission.id, operation),
                  status: result.status === "passed" ? "done" : "blocked",
                  updatedAt: nowIso()
                });
                for (const proofFile of result.proofFiles) {
                  const proofRecord: ProofRecord = {
                    id: `${payload.operationId}:${proofFile}`,
                    operationId: payload.operationId,
                    label: "proof-file",
                    value: proofFile.split("/").pop() ?? proofFile,
                    sourcePath: proofFile,
                    createdAt: nowIso()
                  };
                  next = appendProofRecordToDatabase(next, proofRecord);
                }
              }
              await workspaceRepository.saveDatabase(next);
            }
          }

          sendJson(response, 200, result);
        } catch (error) {
          sendJson(response, 500, { error: error instanceof Error ? error.message : "unknown error" });
        }
        return;
      }

      if (request.method !== "POST" || request.url !== "/run-operation") {
        sendJson(response, 404, { error: "not found" });
        return;
      }

      try {
        const rawBody = await readRequestBody(request);
        const payload = JSON.parse(rawBody) as ApiRunOperationRequest;
        const command = payload.command ?? (payload.runner ? commandFromRunnerPreset(payload.runner) : undefined);
        if (!command) {
          sendJson(response, 400, { error: "command or runner preset is required" });
          return;
        }
        const result: RunOperationResponse = await bridge.runOperation({ ...payload, command });
        sendJson(response, 200, result);
      } catch (error) {
        sendJson(response, 500, { error: error instanceof Error ? error.message : "unknown error" });
      }
    });

    server.listen(options.port, host, () => {
      const address = server.address();
      const port = typeof address === "object" && address ? address.port : options.port;
      resolve({
        url: `http://${host}:${port}`,
        close: () => new Promise((closeResolve) => server.close(() => closeResolve()))
      });
    });
  });
}
