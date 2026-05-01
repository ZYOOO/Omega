package omegalocal

import "testing"

func TestSupervisorRecoveryPolicyClassifiesFailureModes(t *testing.T) {
	tests := []struct {
		name       string
		attempt    map[string]any
		wantClass  string
		wantAction string
		wantAuto   bool
	}{
		{
			name:      "runner crash",
			attempt:   map[string]any{"status": "stalled", "statusReason": "No active local worker host lease for running attempt."},
			wantClass: "runner_crash", wantAction: "retry-with-clean-worker", wantAuto: true,
		},
		{
			name:      "transient network",
			attempt:   map[string]any{"status": "failed", "errorMessage": "temporary network failure: connection reset by peer"},
			wantClass: "transient_network", wantAction: "wait-and-retry", wantAuto: true,
		},
		{
			name:      "github api transient",
			attempt:   map[string]any{"status": "failed", "failureDetail": "GitHub API returned HTTP 502 Bad Gateway while reading PR checks."},
			wantClass: "github_api_transient", wantAction: "wait-and-retry", wantAuto: true,
		},
		{
			name: "ci flaky failure",
			attempt: map[string]any{
				"status": "failed",
				"recommendedActions": []any{
					map[string]any{"type": "checks-failed", "label": "Inspect failed CI checks and route back to rework"},
				},
				"checkLogFeedback": []any{
					map[string]any{"kind": "ci-check-log", "label": "e2e", "message": "Test timed out on hosted runner; likely flaky infrastructure."},
				},
			},
			wantClass: "ci_flaky_failure", wantAction: "retry-validation", wantAuto: true,
		},
		{
			name: "ci non flaky failure",
			attempt: map[string]any{
				"status": "failed",
				"recommendedActions": []any{
					map[string]any{"type": "checks-failed", "label": "Inspect failed CI checks and route back to rework"},
				},
				"checkLogFeedback": []any{
					map[string]any{"kind": "ci-check-log", "label": "unit", "message": "Expected heading text to equal Dashboard, got Settings."},
				},
			},
			wantClass: "ci_failure", wantAction: "rework-required", wantAuto: false,
		},
		{
			name:      "permission failure",
			attempt:   map[string]any{"status": "failed", "errorMessage": "GitHub viewerPermission=READ cannot be trusted for delivery branch push."},
			wantClass: "permission_failure", wantAction: "manual-fix-permission", wantAuto: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := supervisorRecoveryPolicyForAttempt(tt.attempt)
			if got.Class != tt.wantClass || got.Action != tt.wantAction || got.AutoRetry != tt.wantAuto {
				t.Fatalf("policy=%+v", got)
			}
		})
	}
}
