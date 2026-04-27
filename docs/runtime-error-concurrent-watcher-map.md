# Runtime Error: Concurrent Watcher Map Mutation

Date: 2026-04-28

## Summary

Go local runtime tests exposed a crash:

```text
fatal error: concurrent map iteration and map write
```

The crash happened while `putOrchestratorWatcher` was writing the HTTP JSON response for an orchestrator watcher. At the same time, the background watcher goroutine was mutating the same `map[string]any`.

## Impact

- The runtime could panic during watcher updates.
- The failure was nondeterministic because it depended on the response encoder and background watcher timing.
- It affected the local runtime stability around auto processing / orchestrator watcher configuration.

## Root Cause

`putOrchestratorWatcher` reused the same mutable watcher map for two paths:

- `writeJSON(response, watcher)` encoded the watcher response.
- `go server.runOrchestratorWatcher(..., watcher, true)` started a background goroutine that mutates fields such as `lastTickAt`, `lastTickStatus`, `lastTickReason`, and `updatedAt`.

Go maps are not safe for concurrent read/write. JSON encoding iterates over the map while the goroutine may write to it, which can trigger a runtime panic.

## Fix

Clone the watcher map before crossing concurrency boundaries:

- `responseWatcher := cloneMap(watcher)` is used for the HTTP response.
- `cloneMap(watcher)` is passed into the background goroutine.

This prevents the response encoder and background watcher from sharing the same mutable map instance.

## Verification

Validated with:

```bash
go test ./services/local-runtime/...
```

The Go local runtime test suite passed after the fix.

## Follow-Up

- Avoid passing raw mutable `map[string]any` values into goroutines.
- Prefer typed structs or cloned immutable snapshots at async boundaries.
- Audit other background jobs that capture map values from request handlers.
