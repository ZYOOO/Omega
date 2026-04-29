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
			writeTestJSON(t, response, map[string]any{
				"tables": map[string]any{
					"workItems": []map[string]any{{
						"id": "item_1", "key": "OMG-1", "title": "Ship CLI", "status": "Ready", "repositoryTargetId": "repo_1",
					}},
					"pipelines": []map[string]any{},
				},
			})
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
