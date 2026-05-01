package omegalocal

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const pagePilotPreviewRuntimeSettingPrefix = "page-pilot-preview-runtime:"

type previewRuntimeSession struct {
	key        string
	profile    map[string]any
	command    *exec.Cmd
	stdoutTail []string
	stderrTail []string
	startedAt  string
}

type pagePilotPreviewRuntimeRequest struct {
	ProjectID          string `json:"projectId"`
	RepositoryTargetID string `json:"repositoryTargetId"`
	PreviewURL         string `json:"previewUrl"`
	Intent             string `json:"intent"`
	DevCommand         string `json:"devCommand"`
	Restart            bool   `json:"restart"`
}

type pagePilotPreviewRuntimePlan struct {
	RepoPath       string
	PreviewURL     string
	Command        string
	Args           []string
	Shell          bool
	Source         string
	ReloadStrategy string
	Evidence       []string
	Reason         string
}

func (server *Server) pagePilotPreviewRuntimeResolve(response http.ResponseWriter, request *http.Request) {
	var payload pagePilotPreviewRuntimeRequest
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	result, err := server.resolvePagePilotPreviewRuntime(request.Context(), payload)
	if err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	writeJSON(response, http.StatusOK, result)
}

func (server *Server) pagePilotPreviewRuntimeStart(response http.ResponseWriter, request *http.Request) {
	var payload pagePilotPreviewRuntimeRequest
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	result, err := server.startPagePilotPreviewRuntime(request.Context(), payload)
	if err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	writeJSON(response, http.StatusOK, result)
}

func (server *Server) pagePilotPreviewRuntimeRestart(response http.ResponseWriter, request *http.Request) {
	var payload pagePilotPreviewRuntimeRequest
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	payload.Restart = true
	result, err := server.startPagePilotPreviewRuntime(request.Context(), payload)
	if err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	writeJSON(response, http.StatusOK, result)
}

func (server *Server) resolvePagePilotPreviewRuntime(ctx context.Context, payload pagePilotPreviewRuntimeRequest) (map[string]any, error) {
	if strings.TrimSpace(payload.RepositoryTargetID) == "" {
		return nil, errors.New("repositoryTargetId is required")
	}
	repoPath, target, _, err := server.resolvePagePilotWorkspace(ctx, payload.ProjectID, payload.RepositoryTargetID, "")
	if err != nil {
		return nil, err
	}
	plan := detectPagePilotPreviewRuntimePlan(repoPath, payload)
	profile := pagePilotPreviewRuntimeProfileFromPlan(plan, payload, target)
	result := map[string]any{
		"ok":                 plan.Reason == "",
		"status":             stringOr(statusFromBool(plan.Reason == ""), "resolved"),
		"agentId":            "preview-runtime-agent",
		"stageId":            "preview_runtime",
		"repositoryTargetId": payload.RepositoryTargetID,
		"repositoryPath":     repoPath,
		"previewUrl":         plan.PreviewURL,
		"profile":            profile,
		"plan":               pagePilotPreviewRuntimePlanRecord(plan),
	}
	if plan.Reason != "" {
		result["error"] = plan.Reason
	}
	_ = server.persistPagePilotPreviewRuntime(ctx, payload.RepositoryTargetID, result)
	return result, nil
}

func (server *Server) startPagePilotPreviewRuntime(ctx context.Context, payload pagePilotPreviewRuntimeRequest) (map[string]any, error) {
	resolved, err := server.resolvePagePilotPreviewRuntime(ctx, payload)
	if err != nil {
		return nil, err
	}
	planRecord := mapValue(resolved["plan"])
	plan := pagePilotPreviewRuntimePlan{
		RepoPath:       text(planRecord, "repoPath"),
		PreviewURL:     text(planRecord, "previewUrl"),
		Command:        text(planRecord, "command"),
		Args:           stringSlice(planRecord["args"]),
		Shell:          boolValue(planRecord["shell"]),
		Source:         text(planRecord, "source"),
		ReloadStrategy: text(planRecord, "reloadStrategy"),
		Evidence:       stringSlice(planRecord["evidence"]),
		Reason:         text(resolved, "error"),
	}
	if plan.Reason != "" {
		return resolved, nil
	}
	key := pagePilotPreviewRuntimeKey(payload.RepositoryTargetID)
	if payload.Restart {
		server.stopPagePilotPreviewRuntimeSession(key)
	}
	if health := probePagePilotPreviewURL(ctx, server, plan.PreviewURL, 900*time.Millisecond); health["ok"] == true && !payload.Restart {
		resolved["ok"] = true
		resolved["status"] = "external"
		resolved["health"] = health
		_ = server.persistPagePilotPreviewRuntime(ctx, payload.RepositoryTargetID, resolved)
		return resolved, nil
	}
	if strings.TrimSpace(plan.Command) == "" {
		resolved["ok"] = false
		resolved["status"] = "failed"
		resolved["error"] = "Preview Runtime Agent could not detect a dev server command."
		_ = server.persistPagePilotPreviewRuntime(ctx, payload.RepositoryTargetID, resolved)
		return resolved, nil
	}
	session, err := server.spawnPagePilotPreviewRuntime(key, plan)
	if err != nil {
		resolved["ok"] = false
		resolved["status"] = "failed"
		resolved["error"] = err.Error()
		_ = server.persistPagePilotPreviewRuntime(ctx, payload.RepositoryTargetID, resolved)
		return resolved, nil
	}
	health := waitForPagePilotPreviewURL(ctx, server, plan.PreviewURL, 45*time.Second)
	resolved["ok"] = health["ok"] == true
	resolved["status"] = map[bool]string{true: "running", false: "failed"}[health["ok"] == true]
	resolved["health"] = health
	resolved["pid"] = session.command.Process.Pid
	resolved["stdoutTail"] = session.stdoutTail
	resolved["stderrTail"] = session.stderrTail
	if resolved["ok"] != true {
		resolved["error"] = "Preview Runtime health check failed."
	}
	_ = server.persistPagePilotPreviewRuntime(ctx, payload.RepositoryTargetID, resolved)
	return resolved, nil
}

func detectPagePilotPreviewRuntimePlan(repoPath string, payload pagePilotPreviewRuntimeRequest) pagePilotPreviewRuntimePlan {
	previewURL := strings.TrimSpace(payload.PreviewURL)
	if previewURL == "" {
		previewURL = "http://127.0.0.1:3009/"
	}
	plan := pagePilotPreviewRuntimePlan{
		RepoPath:       repoPath,
		PreviewURL:     previewURL,
		ReloadStrategy: "hmr-wait",
		Evidence:       pagePilotPreviewRuntimeEvidence(repoPath),
	}
	if command := strings.TrimSpace(payload.DevCommand); command != "" {
		plan.Command = command
		plan.Shell = true
		plan.Source = "manual-command"
		return plan
	}
	packageJSON := map[string]any{}
	if raw, err := os.ReadFile(filepath.Join(repoPath, "package.json")); err == nil {
		_ = json.Unmarshal(raw, &packageJSON)
	}
	scripts := mapValue(packageJSON["scripts"])
	script := ""
	for _, candidate := range []string{"dev", "start", "preview"} {
		if strings.TrimSpace(text(scripts, candidate)) != "" {
			script = candidate
			break
		}
	}
	if script != "" {
		port := pagePilotPreviewPort(previewURL)
		manager := pagePilotPreviewPackageManager(repoPath)
		plan.Command = manager
		plan.Args = pagePilotPreviewPackageArgs(manager, script, text(scripts, script), port)
		plan.Source = manager + ":" + script
		return plan
	}
	if pathExists(filepath.Join(repoPath, "index.html")) {
		plan.Command = "python3"
		plan.Args = []string{"-m", "http.server", pagePilotPreviewPort(previewURL), "--bind", "127.0.0.1"}
		plan.Source = "static-index"
		plan.ReloadStrategy = "browser-reload"
		return plan
	}
	plan.Reason = "no preview command could be detected"
	return plan
}

func pagePilotPreviewRuntimeProfileFromPlan(plan pagePilotPreviewRuntimePlan, payload pagePilotPreviewRuntimeRequest, target map[string]any) map[string]any {
	return map[string]any{
		"agentId":            "preview-runtime-agent",
		"stageId":            "preview_runtime",
		"repositoryTargetId": payload.RepositoryTargetID,
		"repositoryTarget":   repositoryTargetLabel(target),
		"workingDirectory":   plan.RepoPath,
		"devCommand":         strings.TrimSpace(strings.Join(append([]string{plan.Command}, plan.Args...), " ")),
		"previewUrl":         plan.PreviewURL,
		"reloadStrategy":     plan.ReloadStrategy,
		"source":             plan.Source,
		"evidence":           plan.Evidence,
		"intent":             payload.Intent,
		"healthCheck": map[string]any{
			"url":            plan.PreviewURL,
			"expectedStatus": 200,
		},
		"responsibilities": []any{
			"Resolve the selected repository workspace before preview.",
			"Choose a dev server command or static file server from repository evidence.",
			"Start and health-check only the selected repository workspace.",
			"Persist pid, stdout/stderr tail, preview URL, and reload strategy for Page Pilot.",
		},
		"createdAt": nowISO(),
	}
}

func pagePilotPreviewRuntimePlanRecord(plan pagePilotPreviewRuntimePlan) map[string]any {
	return map[string]any{
		"repoPath":       plan.RepoPath,
		"previewUrl":     plan.PreviewURL,
		"command":        plan.Command,
		"args":           plan.Args,
		"shell":          plan.Shell,
		"source":         plan.Source,
		"reloadStrategy": plan.ReloadStrategy,
		"evidence":       plan.Evidence,
	}
}

func pagePilotPreviewRuntimeEvidence(repoPath string) []string {
	files := []string{"package.json", "vite.config.ts", "vite.config.js", "next.config.js", "astro.config.mjs", "index.html", "README.md", "pnpm-lock.yaml", "yarn.lock", "bun.lockb", "bun.lock"}
	evidence := []string{}
	for _, file := range files {
		if pathExists(filepath.Join(repoPath, file)) {
			evidence = append(evidence, file)
		}
	}
	return evidence
}

func pagePilotPreviewPackageManager(repoPath string) string {
	switch {
	case pathExists(filepath.Join(repoPath, "pnpm-lock.yaml")):
		return "pnpm"
	case pathExists(filepath.Join(repoPath, "yarn.lock")):
		return "yarn"
	case pathExists(filepath.Join(repoPath, "bun.lockb")), pathExists(filepath.Join(repoPath, "bun.lock")):
		return "bun"
	default:
		return "npm"
	}
}

func pagePilotPreviewPackageArgs(manager string, script string, scriptCommand string, port string) []string {
	args := map[string][]string{
		"npm":  {"run", script},
		"pnpm": {"run", script},
		"yarn": {script},
		"bun":  {"run", script},
	}[manager]
	if len(args) == 0 {
		args = []string{"run", script}
	}
	lower := strings.ToLower(scriptCommand)
	if script == "dev" && strings.Contains(lower, "vite") {
		if manager == "yarn" {
			return append(args, "--host", "127.0.0.1", "--port", port)
		}
		return append(args, "--", "--host", "127.0.0.1", "--port", port)
	}
	if script == "dev" && strings.Contains(lower, "next") {
		if manager == "yarn" {
			return append(args, "--hostname", "127.0.0.1", "-p", port)
		}
		return append(args, "--", "--hostname", "127.0.0.1", "-p", port)
	}
	return args
}

func pagePilotPreviewPort(previewURL string) string {
	parsed, err := http.NewRequest(http.MethodGet, previewURL, nil)
	if err != nil || parsed.URL == nil {
		return "3009"
	}
	if port := parsed.URL.Port(); port != "" {
		return port
	}
	if parsed.URL.Scheme == "https" {
		return "443"
	}
	return "80"
}

func (server *Server) spawnPagePilotPreviewRuntime(key string, plan pagePilotPreviewRuntimePlan) (*previewRuntimeSession, error) {
	server.stopPagePilotPreviewRuntimeSession(key)
	var command *exec.Cmd
	if plan.Shell {
		command = exec.Command("/bin/sh", "-lc", plan.Command)
	} else {
		command = exec.Command(plan.Command, plan.Args...)
	}
	command.Dir = plan.RepoPath
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := command.Start(); err != nil {
		return nil, err
	}
	session := &previewRuntimeSession{key: key, profile: pagePilotPreviewRuntimePlanRecord(plan), command: command, startedAt: nowISO()}
	go capturePagePilotPreviewTail(stdout, &session.stdoutTail)
	go capturePagePilotPreviewTail(stderr, &session.stderrTail)
	go func() { _ = command.Wait() }()
	server.previewMu.Lock()
	if server.previewRuntime == nil {
		server.previewRuntime = map[string]*previewRuntimeSession{}
	}
	server.previewRuntime[key] = session
	server.previewMu.Unlock()
	return session, nil
}

func capturePagePilotPreviewTail(pipe any, tail *[]string) {
	reader, ok := pipe.(interface{ Read([]byte) (int, error) })
	if !ok {
		return
	}
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		*tail = append(*tail, line)
		if len(*tail) > 80 {
			*tail = (*tail)[len(*tail)-80:]
		}
	}
}

func (server *Server) stopPagePilotPreviewRuntimeSession(key string) {
	server.previewMu.Lock()
	defer server.previewMu.Unlock()
	if server.previewRuntime == nil {
		return
	}
	session := server.previewRuntime[key]
	if session == nil || session.command == nil || session.command.Process == nil {
		return
	}
	_ = session.command.Process.Kill()
	delete(server.previewRuntime, key)
}

func pagePilotPreviewRuntimeKey(repositoryTargetID string) string {
	return pagePilotPreviewRuntimeSettingPrefix + strings.TrimSpace(repositoryTargetID)
}

func (server *Server) persistPagePilotPreviewRuntime(ctx context.Context, repositoryTargetID string, record map[string]any) error {
	if strings.TrimSpace(repositoryTargetID) == "" {
		return nil
	}
	record["updatedAt"] = nowISO()
	return server.Repo.SetSetting(ctx, pagePilotPreviewRuntimeKey(repositoryTargetID), record)
}

func probePagePilotPreviewURL(ctx context.Context, server *Server, rawURL string, timeout time.Duration) map[string]any {
	if strings.TrimSpace(rawURL) == "" {
		return map[string]any{"ok": false, "status": 0, "error": "empty preview URL"}
	}
	client := server.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return map[string]any{"ok": false, "status": 0, "error": err.Error()}
	}
	probeClient := *client
	probeClient.Timeout = timeout
	response, err := probeClient.Do(request)
	if err != nil {
		return map[string]any{"ok": false, "status": 0, "error": err.Error()}
	}
	defer response.Body.Close()
	return map[string]any{"ok": response.StatusCode >= 200 && response.StatusCode < 400, "status": response.StatusCode}
}

func waitForPagePilotPreviewURL(ctx context.Context, server *Server, rawURL string, timeout time.Duration) map[string]any {
	deadline := time.Now().Add(timeout)
	last := map[string]any{"ok": false, "status": 0, "error": "not started"}
	for time.Now().Before(deadline) {
		last = probePagePilotPreviewURL(ctx, server, rawURL, 1200*time.Millisecond)
		if last["ok"] == true {
			return last
		}
		time.Sleep(500 * time.Millisecond)
	}
	last["error"] = stringOr(text(last, "error"), fmt.Sprintf("health check timed out after %s", timeout))
	return last
}

func statusFromBool(ok bool) string {
	if ok {
		return "resolved"
	}
	return "failed"
}
