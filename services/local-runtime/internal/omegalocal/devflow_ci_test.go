package omegalocal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDevFlowCIReportMarksFailedActionsAsAttention(t *testing.T) {
	result := devFlowCIResult{
		Status:       "attention",
		Summary:      "GitHub Actions checks: 2 total, 1 passed, 0 pending, 1 failed, 0 missing required.",
		ChecksOutput: `[{"name":"test","state":"FAILURE","link":"https://github.com/acme/demo/actions/runs/123"}]`,
		CheckSummary: map[string]any{
			"total": 1, "passed": 0, "pending": 0, "failed": 1, "missingRequired": 0,
		},
		CheckLogFeedback: []map[string]any{{
			"label":   "test",
			"state":   "FAILURE",
			"url":     "https://github.com/acme/demo/actions/runs/123",
			"message": "npm test failed\nExpected Dashboard, got Settings",
		}},
	}
	report := devFlowCIReportMarkdown("https://github.com/acme/demo/pull/7", []string{"test"}, result)
	if !strings.Contains(report, "# GitHub Actions CI") || !strings.Contains(report, "Status: attention") || !strings.Contains(report, "Failed Check Logs") || !strings.Contains(report, "Expected Dashboard") {
		t.Fatalf("ci report missing failure context:\n%s", report)
	}
	if status := devFlowCIStatus(result.CheckSummary, result.CheckLogFeedback, result.ChecksOutput); status != "attention" {
		t.Fatalf("status = %s", status)
	}
}

func TestDevFlowCIReportWritesProofFile(t *testing.T) {
	proofDir := t.TempDir()
	result := collectDevFlowGitHubActionsCI(nil, "", "", "", nil, proofDir, "ci-checks.md")
	if result.Status != "missing" || result.ReportPath != "" {
		t.Fatalf("missing PR should not write a proof report: %+v", result)
	}

	manual := devFlowCIResult{
		Status:       "passed",
		Summary:      "GitHub Actions checks: 1 total, 1 passed, 0 pending, 0 failed, 0 missing required.",
		ChecksOutput: "test\tSUCCESS",
		CheckSummary: map[string]any{"total": 1, "passed": 1, "pending": 0, "failed": 0, "missingRequired": 0},
		ReportPath:   filepath.Join(proofDir, "ci-checks.md"),
	}
	if err := os.WriteFile(manual.ReportPath, []byte(devFlowCIReportMarkdown("https://github.com/acme/demo/pull/8", nil, manual)), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(manual.ReportPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "GitHub Actions checks: 1 total") {
		t.Fatalf("proof report = %s", raw)
	}
}
