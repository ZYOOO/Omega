package omegalocal

import (
	"context"
	"os"
)

func localWorkerHostRecord(attemptID string) map[string]any {
	hostname, _ := os.Hostname()
	return map[string]any{
		"id":        "local:" + hostname,
		"kind":      "local-runtime",
		"hostname":  hostname,
		"pid":       os.Getpid(),
		"attemptId": attemptID,
		"claimedAt": nowISO(),
		"updatedAt": nowISO(),
	}
}

func (server *Server) markAttemptWorkerHost(ctx context.Context, pipelineID string, attemptID string, worker map[string]any) {
	database, err := mustLoad(server, ctx)
	if err != nil {
		return
	}
	index := findByID(database.Tables.Attempts, attemptID)
	if index < 0 {
		return
	}
	attempt := cloneMap(database.Tables.Attempts[index])
	attempt["workerHost"] = worker
	attempt["lastSeenAt"] = nowISO()
	attempt["updatedAt"] = nowISO()
	database.Tables.Attempts[index] = attempt
	if pipelineIndex := findByID(database.Tables.Pipelines, pipelineID); pipelineIndex >= 0 {
		pipeline := cloneMap(database.Tables.Pipelines[pipelineIndex])
		run := mapValue(pipeline["run"])
		orchestrator := mapValue(run["orchestrator"])
		orchestrator["workerHost"] = worker
		run["orchestrator"] = orchestrator
		pipeline["run"] = run
		pipeline["updatedAt"] = nowISO()
		database.Tables.Pipelines[pipelineIndex] = pipeline
	}
	_ = server.Repo.Save(ctx, database)
}

func (server *Server) hasRegisteredAttemptJob(attemptID string) bool {
	server.jobMu.Lock()
	defer server.jobMu.Unlock()
	return server.attemptCancels[attemptID] != nil
}

func intOrDefault(value any, fallback int) int {
	parsed := intValue(value)
	if parsed == 0 {
		return fallback
	}
	return parsed
}
