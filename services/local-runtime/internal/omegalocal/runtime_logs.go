package omegalocal

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (writer *loggingResponseWriter) WriteHeader(status int) {
	writer.status = status
	writer.ResponseWriter.WriteHeader(status)
}

func (writer *loggingResponseWriter) Write(data []byte) (int, error) {
	if writer.status == 0 {
		writer.status = http.StatusOK
	}
	count, err := writer.ResponseWriter.Write(data)
	writer.bytes += count
	return count, err
}

func (server *Server) runtimeLogs(response http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	limit, _ := strconv.Atoi(query.Get("limit"))
	filters := map[string]string{
		"level":              strings.ToUpper(query.Get("level")),
		"eventType":          query.Get("eventType"),
		"entityType":         query.Get("entityType"),
		"entityId":           query.Get("entityId"),
		"projectId":          query.Get("projectId"),
		"repositoryTargetId": query.Get("repositoryTargetId"),
		"workItemId":         query.Get("workItemId"),
		"pipelineId":         query.Get("pipelineId"),
		"attemptId":          query.Get("attemptId"),
		"stageId":            query.Get("stageId"),
		"agentId":            query.Get("agentId"),
		"requestId":          query.Get("requestId"),
		"createdAfter":       stringOr(query.Get("createdAfter"), query.Get("from")),
		"createdBefore":      stringOr(query.Get("createdBefore"), query.Get("to")),
	}
	logs, err := server.Repo.ListRuntimeLogs(request.Context(), filters, limit)
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, logs)
}

func (server *Server) logInfo(ctx context.Context, eventType string, message string, fields map[string]any) {
	server.logRuntime(ctx, "INFO", eventType, message, fields)
}

func (server *Server) logDebug(ctx context.Context, eventType string, message string, fields map[string]any) {
	server.logRuntime(ctx, "DEBUG", eventType, message, fields)
}

func (server *Server) logError(ctx context.Context, eventType string, message string, fields map[string]any) {
	server.logRuntime(ctx, "ERROR", eventType, message, fields)
}

func (server *Server) logRuntime(ctx context.Context, level string, eventType string, message string, fields map[string]any) {
	if server == nil || server.Repo == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if _, err := os.Stat(filepath.Dir(server.Repo.Path)); err != nil {
		return
	}
	fields = cloneLogFields(fields)
	record := RuntimeLogRecord{
		ID:                 fmt.Sprintf("log_%d", time.Now().UnixNano()),
		Level:              strings.ToUpper(stringOr(level, "INFO")),
		EventType:          stringOr(eventType, "runtime.event"),
		Message:            message,
		EntityType:         text(fields, "entityType"),
		EntityID:           text(fields, "entityId"),
		ProjectID:          text(fields, "projectId"),
		RepositoryTargetID: text(fields, "repositoryTargetId"),
		WorkItemID:         text(fields, "workItemId"),
		PipelineID:         text(fields, "pipelineId"),
		AttemptID:          text(fields, "attemptId"),
		StageID:            text(fields, "stageId"),
		AgentID:            text(fields, "agentId"),
		RequestID:          text(fields, "requestId"),
		Details:            fields,
		CreatedAt:          nowISO(),
	}
	if err := server.Repo.AppendRuntimeLog(ctx, record); err != nil {
		fmt.Printf("Omega runtime log write failed: %s\n", err)
	}
}

func cloneLogFields(fields map[string]any) map[string]any {
	if fields == nil {
		return map[string]any{}
	}
	output := map[string]any{}
	for key, value := range fields {
		output[key] = value
	}
	return output
}

func runtimeLogLevelForStatus(status int, method string) string {
	if status >= 500 {
		return "ERROR"
	}
	if status >= 400 {
		return "ERROR"
	}
	if method == http.MethodGet || method == http.MethodOptions {
		return "DEBUG"
	}
	return "INFO"
}
