package omegalocal

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var diagnosticLogMu sync.Mutex

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
	filters := runtimeLogFiltersFromQuery(query)
	pageMode := query.Get("page") == "1" || query.Get("cursor") != "" || query.Get("pageSize") != ""
	if pageSize, _ := strconv.Atoi(query.Get("pageSize")); pageSize > 0 {
		limit = pageSize
	}
	if pageMode {
		page, err := server.Repo.ListRuntimeLogsPage(request.Context(), filters, limit)
		if err != nil {
			writeError(response, http.StatusInternalServerError, err)
			return
		}
		writeJSON(response, http.StatusOK, page)
		return
	}
	logs, err := server.Repo.ListRuntimeLogs(request.Context(), filters, limit)
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, logs)
}

func (server *Server) runtimeLogsExport(response http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	limit, _ := strconv.Atoi(query.Get("limit"))
	if limit <= 0 {
		limit = 5000
	}
	filters := runtimeLogFiltersFromQuery(query)
	filters["export"] = "1"
	logs, err := server.Repo.ListRuntimeLogs(request.Context(), filters, limit)
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	format := strings.ToLower(stringOr(query.Get("format"), "jsonl"))
	if format != "csv" && format != "jsonl" {
		format = "jsonl"
	}
	filename := "omega-runtime-logs." + format
	response.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	switch format {
	case "csv":
		response.Header().Set("Content-Type", "text/csv; charset=utf-8")
		writer := csv.NewWriter(response)
		_ = writer.Write([]string{"createdAt", "level", "eventType", "message", "projectId", "repositoryTargetId", "requirementId", "workItemId", "pipelineId", "attemptId", "stageId", "agentId", "requestId"})
		for _, log := range logs {
			_ = writer.Write([]string{log.CreatedAt, log.Level, log.EventType, log.Message, log.ProjectID, log.RepositoryTargetID, log.RequirementID, log.WorkItemID, log.PipelineID, log.AttemptID, log.StageID, log.AgentID, log.RequestID})
		}
		writer.Flush()
	default:
		response.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
		encoder := json.NewEncoder(response)
		for _, log := range logs {
			_ = encoder.Encode(log)
		}
	}
}

func runtimeLogFiltersFromQuery(query url.Values) map[string]string {
	filters := map[string]string{
		"level":              strings.ToUpper(query.Get("level")),
		"eventType":          query.Get("eventType"),
		"entityType":         query.Get("entityType"),
		"entityId":           query.Get("entityId"),
		"projectId":          query.Get("projectId"),
		"repositoryTargetId": query.Get("repositoryTargetId"),
		"requirementId":      query.Get("requirementId"),
		"workItemId":         query.Get("workItemId"),
		"pipelineId":         query.Get("pipelineId"),
		"attemptId":          query.Get("attemptId"),
		"stageId":            query.Get("stageId"),
		"agentId":            query.Get("agentId"),
		"requestId":          query.Get("requestId"),
		"createdAfter":       stringOr(query.Get("createdAfter"), query.Get("from")),
		"createdBefore":      stringOr(query.Get("createdBefore"), query.Get("to")),
		"cursor":             query.Get("cursor"),
		"query":              stringOr(query.Get("q"), query.Get("search")),
	}
	return filters
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
		RequirementID:      text(fields, "requirementId"),
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

func (server *Server) logRuntimeDiagnosticFile(level string, eventType string, message string, fields map[string]any) {
	if server == nil || server.Repo == nil {
		return
	}
	dir := filepath.Dir(server.Repo.Path)
	if _, err := os.Stat(dir); err != nil {
		return
	}
	fields = cloneLogFields(fields)
	record := RuntimeLogRecord{
		ID:                 fmt.Sprintf("diag_%d", time.Now().UnixNano()),
		Level:              strings.ToUpper(stringOr(level, "DEBUG")),
		EventType:          stringOr(eventType, "runtime.diagnostic"),
		Message:            message,
		EntityType:         text(fields, "entityType"),
		EntityID:           text(fields, "entityId"),
		ProjectID:          text(fields, "projectId"),
		RepositoryTargetID: text(fields, "repositoryTargetId"),
		RequirementID:      text(fields, "requirementId"),
		WorkItemID:         text(fields, "workItemId"),
		PipelineID:         text(fields, "pipelineId"),
		AttemptID:          text(fields, "attemptId"),
		StageID:            text(fields, "stageId"),
		AgentID:            text(fields, "agentId"),
		RequestID:          text(fields, "requestId"),
		Details:            fields,
		CreatedAt:          nowISO(),
	}
	if err := appendDiagnosticRuntimeLog(dir, record); err != nil {
		fmt.Printf("Omega diagnostic log write failed: %s\n", err)
	}
}

func appendDiagnosticRuntimeLog(root string, record RuntimeLogRecord) error {
	diagnosticLogMu.Lock()
	defer diagnosticLogMu.Unlock()
	logDir := filepath.Join(root, "logs")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return err
	}
	now := time.Now()
	cleanupDiagnosticRuntimeLogs(logDir, now)
	path := filepath.Join(logDir, "omega-runtime-diagnostics."+now.Format("2006-01-02")+".jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	return encoder.Encode(record)
}

func cleanupDiagnosticRuntimeLogs(logDir string, now time.Time) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return
	}
	cutoff := now.Add(-24 * time.Hour)
	today := "omega-runtime-diagnostics." + now.Format("2006-01-02") + ".jsonl"
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "omega-runtime-diagnostics.") || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		if entry.Name() == today {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(logDir, entry.Name()))
		}
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
