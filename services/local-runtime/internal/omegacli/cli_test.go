package omegacli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGlobalHelpPrintsUsageWithoutError(t *testing.T) {
	var stdout bytes.Buffer
	cli := CLI{Client: http.DefaultClient, Stdout: &stdout, Stderr: ioDiscard{}}
	if err := cli.Run(context.Background(), []string{"--help"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "Omega CLI") || !strings.Contains(stdout.String(), "work-items run") {
		t.Fatalf("help output = %q", stdout.String())
	}
}

func TestStatusPrintsObservabilitySummary(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/observability" {
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
		writeTestJSON(t, response, map[string]any{
			"counts":    map[string]any{"workItems": 3, "pipelines": 2, "attempts": 1, "checkpoints": 4, "runtimeLogs": 5},
			"attention": map[string]any{"waitingHuman": 1, "failed": 2, "blocked": 3},
			"dashboard": map[string]any{
				"attempts":           map[string]any{"total": 1, "active": 0, "terminal": 1, "successRate": 1},
				"recommendedActions": []map[string]any{{"label": "Review pending human gates", "count": 1}},
			},
		})
	}))
	defer api.Close()

	var stdout bytes.Buffer
	cli := CLI{Client: api.Client(), Stdout: &stdout, Stderr: ioDiscard{}}
	if err := cli.Run(context.Background(), []string{"--api-url", api.URL, "status"}); err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	if !strings.Contains(output, "workItems=3") || !strings.Contains(output, "waitingHuman=1") || !strings.Contains(output, "successRate=1.00") {
		t.Fatalf("status output = %q", output)
	}
}

func TestWorkItemsRunCreatesPipelineAndStartsDevFlow(t *testing.T) {
	createdPipeline := false
	startedRun := false
	api := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/workspace":
			if request.URL.Query().Get("scope") != "session" {
				t.Fatalf("workspace query = %s", request.URL.RawQuery)
			}
			writeTestJSON(t, response, map[string]any{
				"tables": map[string]any{
					"workItems": []map[string]any{{
						"id": "item_1", "key": "OMG-1", "title": "Ship CLI", "status": "Ready", "repositoryTargetId": "repo_1",
					}},
				},
			})
		case request.Method == http.MethodGet && request.URL.Path == "/pipelines":
			if request.URL.Query().Get("workItemId") != "item_1" {
				t.Fatalf("pipelines query = %s", request.URL.RawQuery)
			}
			writeTestJSON(t, response, []map[string]any{})
		case request.Method == http.MethodPost && request.URL.Path == "/pipelines/from-template":
			var payload map[string]any
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["templateId"] != "devflow-pr" {
				t.Fatalf("payload = %+v", payload)
			}
			createdPipeline = true
			writeTestJSON(t, response, map[string]any{"id": "pipeline_item_1_devflow", "workItemId": "item_1", "templateId": "devflow-pr"})
		case request.Method == http.MethodPost && request.URL.Path == "/pipelines/pipeline_item_1_devflow/run-devflow-cycle":
			var payload map[string]any
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["wait"] != true {
				t.Fatalf("run payload = %+v", payload)
			}
			startedRun = true
			writeTestJSON(t, response, map[string]any{"status": "accepted", "attempt": map[string]any{"id": "attempt_1", "status": "running"}})
		default:
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
		}
	}))
	defer api.Close()

	var stdout bytes.Buffer
	cli := CLI{Client: api.Client(), Stdout: &stdout, Stderr: ioDiscard{}}
	if err := cli.Run(context.Background(), []string{"--api-url", api.URL, "work-items", "run", "OMG-1", "--wait"}); err != nil {
		t.Fatal(err)
	}
	if !createdPipeline || !startedRun {
		t.Fatalf("createdPipeline=%v startedRun=%v", createdPipeline, startedRun)
	}
	if !strings.Contains(stdout.String(), "attempt=attempt_1") {
		t.Fatalf("run output = %q", stdout.String())
	}
}

func TestWorkItemsListTreatsMissingWorkspaceAsEmpty(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/workspace" || request.URL.Query().Get("scope") != "session" {
			t.Fatalf("unexpected request %s %s?%s", request.Method, request.URL.Path, request.URL.RawQuery)
		}
		response.WriteHeader(http.StatusNotFound)
		writeTestJSON(t, response, map[string]any{"error": "workspace not found"})
	}))
	defer api.Close()

	var stdout bytes.Buffer
	cli := CLI{Client: api.Client(), Stdout: &stdout, Stderr: ioDiscard{}}
	if err := cli.Run(context.Background(), []string{"--api-url", api.URL, "work-items", "list"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "KEY") {
		t.Fatalf("work-items output = %q", stdout.String())
	}
}

func TestCheckpointApprovePostsReviewer(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/checkpoints/checkpoint_1/approve" {
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["reviewer"] != "alice" {
			t.Fatalf("payload = %+v", payload)
		}
		writeTestJSON(t, response, map[string]any{"id": "checkpoint_1", "status": "approved"})
	}))
	defer api.Close()

	var stdout bytes.Buffer
	cli := CLI{Client: api.Client(), Stdout: &stdout, Stderr: ioDiscard{}}
	if err := cli.Run(context.Background(), []string{"--api-url", api.URL, "checkpoints", "approve", "checkpoint_1", "--reviewer", "alice"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "status=approved") {
		t.Fatalf("approve output = %q", stdout.String())
	}
}

func TestAttemptsListUsesCompactFilteredAPI(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/attempts" {
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
		}
		query := request.URL.Query()
		if query.Get("compact") != "true" || query.Get("workItemId") != "item_1" || query.Get("status") != "running" || query.Get("limit") != "7" {
			t.Fatalf("attempts query = %s", request.URL.RawQuery)
		}
		writeTestJSON(t, response, []map[string]any{{
			"id": "attempt_1", "status": "running", "itemId": "item_1", "currentStageId": "coding", "updatedAt": "2026-05-06T10:00:00Z",
		}})
	}))
	defer api.Close()

	var stdout bytes.Buffer
	cli := CLI{Client: api.Client(), Stdout: &stdout, Stderr: ioDiscard{}}
	if err := cli.Run(context.Background(), []string{"--api-url", api.URL, "attempts", "list", "--work-item", "item_1", "--status", "running", "--limit", "7"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "attempt_1") || !strings.Contains(stdout.String(), "coding") {
		t.Fatalf("attempts output = %q", stdout.String())
	}
}

func TestOperationsListShowsRunnerStatistics(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/operations" {
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
		}
		query := request.URL.Query()
		if query.Get("compact") != "true" || query.Get("pipelineId") != "pipeline_1" || query.Get("limit") != "3" {
			t.Fatalf("operations query = %s", request.URL.RawQuery)
		}
		writeTestJSON(t, response, []map[string]any{{
			"id": "operation_1", "status": "passed", "stageId": "coding", "agentId": "coder", "summary": "Implemented CLI statistics.",
			"runnerProcess": map[string]any{"runner": "codex", "model": "gpt-5.2", "durationMs": 125000, "totalTokens": 3456},
		}})
	}))
	defer api.Close()

	var stdout bytes.Buffer
	cli := CLI{Client: api.Client(), Stdout: &stdout, Stderr: ioDiscard{}}
	if err := cli.Run(context.Background(), []string{"--api-url", api.URL, "operations", "list", "--pipeline", "pipeline_1", "--limit", "3"}); err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	if !strings.Contains(output, "operation_1") || !strings.Contains(output, "codex") || !strings.Contains(output, "3456") {
		t.Fatalf("operations output = %q", output)
	}
}

func TestWorkpadsListUsesCompactFilteredAPI(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/run-workpads" {
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
		}
		query := request.URL.Query()
		if query.Get("compact") != "true" || query.Get("workItemId") != "item_1" || query.Get("limit") != "5" {
			t.Fatalf("workpads query = %s", request.URL.RawQuery)
		}
		writeTestJSON(t, response, []map[string]any{{
			"id": "attempt_1:workpad", "status": "running", "workItemId": "item_1", "pipelineId": "pipeline_1", "attemptId": "attempt_1",
		}})
	}))
	defer api.Close()

	var stdout bytes.Buffer
	cli := CLI{Client: api.Client(), Stdout: &stdout, Stderr: ioDiscard{}}
	if err := cli.Run(context.Background(), []string{"--api-url", api.URL, "workpads", "list", "--work-item", "item_1", "--limit", "5"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "attempt_1:workpad") {
		t.Fatalf("workpads output = %q", stdout.String())
	}
}

func TestProofPreviewPrintsContent(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/proof-records/proof_1/preview" {
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
		}
		writeTestJSON(t, response, map[string]any{"available": true, "previewType": "markdown", "sourcePath": "/tmp/proof.md", "content": "# Proof\n\nPassed."})
	}))
	defer api.Close()

	var stdout bytes.Buffer
	cli := CLI{Client: api.Client(), Stdout: &stdout, Stderr: ioDiscard{}}
	if err := cli.Run(context.Background(), []string{"--api-url", api.URL, "proof", "preview", "proof_1"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "# Proof") || !strings.Contains(stdout.String(), "available=true") {
		t.Fatalf("proof preview output = %q", stdout.String())
	}
}

func TestPRStatusCanResolveAttempt(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/attempts":
			query := request.URL.Query()
			if query.Get("id") != "attempt_1" || query.Get("compact") != "true" {
				t.Fatalf("attempt query = %s", request.URL.RawQuery)
			}
			writeTestJSON(t, response, []map[string]any{{
				"id": "attempt_1", "pullRequestUrl": "https://github.com/acme/demo/pull/12", "workspacePath": "/tmp/ws",
			}})
		case request.Method == http.MethodPost && request.URL.Path == "/github/pr-status":
			var payload map[string]any
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["url"] != "https://github.com/acme/demo/pull/12" || payload["workspacePath"] != "/tmp/ws" {
				t.Fatalf("payload = %+v", payload)
			}
			writeTestJSON(t, response, map[string]any{
				"number": 12, "state": "OPEN", "reviewDecision": "APPROVED", "deliveryGate": "pending", "url": "https://github.com/acme/demo/pull/12",
				"checkSummary": map[string]any{"passed": 1, "failed": 0, "pending": 1, "missing": 0},
			})
		default:
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
		}
	}))
	defer api.Close()

	var stdout bytes.Buffer
	cli := CLI{Client: api.Client(), Stdout: &stdout, Stderr: ioDiscard{}}
	if err := cli.Run(context.Background(), []string{"--api-url", api.URL, "pr", "status", "attempt_1"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "gate=pending") || !strings.Contains(stdout.String(), "passed=1") {
		t.Fatalf("pr status output = %q", stdout.String())
	}
}

func writeTestJSON(t *testing.T, response http.ResponseWriter, value any) {
	t.Helper()
	response.Header().Set("content-type", "application/json")
	if err := json.NewEncoder(response).Encode(value); err != nil {
		t.Fatal(err)
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}
