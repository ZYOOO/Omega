# Omega Data Ownership And Read Model

Omega runtime data must be owned by SQLite first-class tables. The historical full workspace JSON snapshot remains only as a compatibility, export, and emergency recovery layer.

## Goals

- Keep hot UI paths off the full `/workspace` snapshot.
- Make supervisor recovery and external delivery idempotency queryable without deserializing the whole workspace.
- Store large or append-heavy execution output outside the primary snapshot path.
- Use JSON only where the payload is local to one row and does not define cross-entity ownership.

## Ownership Rules

| Data | Owner | JSON role |
| --- | --- | --- |
| Project, repository target, requirement, work item | SQLite first-class tables | `record_json` is compatibility detail only |
| Pipeline and attempt identity/status | SQLite first-class tables | `run_json`, `stages_json`, `events_json` are local execution payloads |
| Attempt failure notification idempotency | `attempts.feishu_failure_notified_at`, `feishu_failure_status`, `feishu_failure_message_id` | `feishu_failure_json` keeps provider-specific delivery payload |
| Checkpoint identity/status/decision | `checkpoints` columns | `feishu_review_json` keeps provider-specific delivery payload |
| Feishu delivery idempotency | `checkpoints.feishu_review_status`, `feishu_task_guid`, `feishu_task_id`, `feishu_message_id` | Provider details remain in `feishu_review_json` |
| Run Workpad summary and user/agent patches | `run_workpads` table | `workpad_json` is the rendered detail payload; `record_json` preserves field patches, sources, and patch history |
| Operations, proof records, handoff bundles, queue | Dedicated SQLite tables | `operations.record_json` preserves row-local runner details; other JSON columns are nested proof/bundle bodies |
| Runtime logs | `runtime_logs` table and `.omega/logs/*.jsonl` | details JSON is per-log metadata |
| Full workspace snapshot | `workspace_snapshots` | Compatibility/export/recovery only |

## Hot Path Guidance

- Session restore must use `GET /workspace?scope=session`.
- Detail pages should fetch dedicated read models: `/run-workpads`, `/pipelines`, `/attempts`, `/checkpoints`, `/proof-records`, `/runtime-logs`.
- Ordinary control-plane refresh should use compact execution lists (`/attempts?compact=true`, `/run-workpads?compact=true`, `/operations?compact=true`). Full prompt/stdout/stderr, attempt events, and Workpad patch history belong to scoped detail reads, not global refresh.
- Page Pilot recent run lists should use `/page-pilot/runs?repositoryTargetId=...&limit=8&compact=true`; full `run_json` payloads with conversation, visual proof, PR preview, diff, and source mapping should be loaded by run id only when a detail view is opened.
- JobSupervisor should update and recover by first-class identifiers: `pipeline_id`, `attempt_id`, `stage_id`, checkpoint status, and Feishu task/message columns.
- JobSupervisor maintenance ticks and DevFlow execution-state maintenance must load `LoadSupervisorExecutionState`, not the full snapshot. This read model contains projects/repository targets, requirements, work items, pipelines, attempts, checkpoints, run workpads, recent missions/operations, and recent proof records.
- JobSupervisor maintenance ticks and DevFlow execution-state maintenance must save with `SaveSupervisorExecutionState`, which upserts execution tables instead of rewriting the full workspace snapshot.
- `SaveSupervisorExecutionState` mirrors the changed execution tables back into `workspace_snapshots` only as a compatibility projection. The mirror is best-effort and must not become a runtime dependency.
- Feishu auto-review send should check checkpoint idempotency from checkpoint columns before loading full context.
- Feishu attempt-failure alert send should check attempt delivery columns before loading full context.
- Run Workpad patch should update `run_workpads` directly and must not require the full snapshot.
- Attempt cancel should use the supervisor execution read/write path so it can update attempt, pipeline, work item, checkpoint, and workpad execution state without full snapshot.
- Attempt retry, DevFlow background job load, worker host marking, completion, failure, cancellation, agent invocation persistence, and runner heartbeat should use the same execution read/write path.
- Attempt action plan, run timeline, manual DevFlow run, run-current-stage, Feishu review task bridge, and workspace cleanup should also use the execution read/write path.
- Work Item create/patch/delete should use the session read model and local work item state writers; these APIs must not require a valid full snapshot.
- Repository target bind/delete/import, GitHub issue import, Page Pilot apply/deliver/discard, workflow template save/restore, Agent Profile lookup/import, proof preview, observability, and orchestrator tick should use normalized table readers/writers.
- Human Review approve delivery and Feishu review/failure delivery state must persist through `SaveSupervisorExecutionState`; repeated sends must not rely on snapshot JSON.
- Full snapshot reads are acceptable for legacy handlers, manual export, one-off recovery, and code paths not on polling or startup idempotency.

## Checkpoint Contract

The checkpoint row is the canonical place for Human Review gate state:

- `id`, `pipeline_id`, `attempt_id`, `stage_id`, `status`
- `decision_note`
- `feishu_review_status`
- `feishu_task_guid`
- `feishu_task_id`
- `feishu_message_id`
- `feishu_review_json`

`feishu_review_json` can contain task URLs, provider names, route details, errors, and callback metadata. It must not be the only place to know whether a notification was already sent.

## Attempt Delivery Contract

Attempt-level external notifications use the attempt row as their canonical idempotency gate:

- `feishu_failure_notified_at`
- `feishu_failure_status`
- `feishu_failure_message_id`
- `feishu_failure_json`
- `record_json`

The provider payload can include route details, message raw output, and failure reason. The columns must remain sufficient to skip repeat sends after process restart.

`record_json` preserves attempt-local extension fields such as `stalledAt`, `statusReason`, `remoteSignals`, retry metadata, and workspace cleanup metadata. Queryable facts still belong in columns.

## Run Workpad Contract

Run Workpad rows carry both the rendered workpad and the patch metadata needed to preserve human/operator edits:

- `workpad_json`
- `record_json`

`record_json` includes `fieldPatches`, `fieldPatchSources`, and `fieldPatchHistory`. Patch handlers must read and write this table directly. Snapshot mirroring is compatibility-only.

## Operation Contract

Operation rows carry status, stage, agent, prompt, required proof, and `record_json`.

`record_json` preserves operation-local details such as `runnerProcess`, summaries, action routes, and Page Pilot metadata. Endpoints like `/operations` must return this merged record so the UI can show stdout/stderr/exit status without reading the full snapshot.

## Work Item Contract

Work Item session APIs use first-class session tables:

- `projects`
- `requirements`
- `work_items`
- `mission_control_states`

Create, patch, and delete operations save through local table writers and mirror into `workspace_snapshots` only for compatibility. Delete must still consult normalized execution/history tables before removing a not-started item.

## Contraction Backlog

- Add long-term retention for stdout/stderr/runner details outside `.omega/omega.db`.
- Continue shrinking remaining legacy handlers; full `/workspace` compatibility, import/export, and a few connection-state mirrors are the only expected full snapshot users on the local runtime hot path.
- Keep `workspace_snapshots` until export/import and backward-compatible recovery have a narrower replacement.
