package omegalocal

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitHubIssueRefFromWorkItemParsesImportRefAndURL(t *testing.T) {
	repo, number, ok := githubIssueRefFromWorkItem(map[string]any{"sourceExternalRef": "ZYOOO/TestRepo#29"}, "")
	if !ok || repo != "ZYOOO/TestRepo" || number != 29 {
		t.Fatalf("import ref parsed as repo=%q number=%d ok=%v", repo, number, ok)
	}
	repo, number, ok = githubIssueRefFromWorkItem(map[string]any{"target": "https://github.com/ZYOOO/TestRepo/issues/30"}, "")
	if !ok || repo != "ZYOOO/TestRepo" || number != 30 {
		t.Fatalf("issue URL parsed as repo=%q number=%d ok=%v", repo, number, ok)
	}
	repo, number, ok = githubIssueRefFromWorkItem(map[string]any{"issueNumber": 31}, "ZYOOO/TestRepo")
	if !ok || repo != "ZYOOO/TestRepo" || number != 31 {
		t.Fatalf("fallback issue number parsed as repo=%q number=%d ok=%v", repo, number, ok)
	}
}

func TestSyncGitHubIssueOutboundPostsCommentAndLabels(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "gh.log")
	bodyPath := filepath.Join(tempDir, "body.md")
	prBodyPath := filepath.Join(tempDir, "pr-body.md")
	binDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeGH := filepath.Join(binDir, "gh")
	script := `#!/bin/sh
printf '%s\n' "$PWD :: $*" >> "$OMEGA_FAKE_GH_LOG"
if [ "$1" = "issue" ] && [ "$2" = "comment" ]; then
  while [ "$#" -gt 0 ]; do
    if [ "$1" = "--body-file" ]; then
      cat "$2" > "$OMEGA_FAKE_GH_BODY"
    fi
    shift
  done
  echo "commented"
  exit 0
fi
if [ "$1" = "pr" ] && [ "$2" = "comment" ]; then
  while [ "$#" -gt 0 ]; do
    if [ "$1" = "--body-file" ]; then
      cat "$2" > "$OMEGA_FAKE_GH_PR_BODY"
    fi
    shift
  done
  echo "pr commented"
  exit 0
fi
echo "ok"
`
	if err := os.WriteFile(fakeGH, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	repoDir := filepath.Join(tempDir, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("OMEGA_FAKE_GH_LOG", logPath)
	t.Setenv("OMEGA_FAKE_GH_BODY", bodyPath)
	t.Setenv("OMEGA_FAKE_GH_PR_BODY", prBodyPath)

	report := (&Server{}).syncGitHubIssueOutbound(context.Background(), githubOutboundSyncInput{
		RepositoryPath: repoDir,
		Repository:     "ZYOOO/TestRepo",
		WorkItem:       map[string]any{"id": "item_29", "key": "OMG-29", "title": "Sync issue", "sourceExternalRef": "ZYOOO/TestRepo#29"},
		Pipeline:       map[string]any{"id": "pipeline_29"},
		AttemptID:      "attempt_29",
		Event:          "human_review.waiting",
		Status:         "waiting-human",
		StageID:        "human_review",
		Summary:        "Pull request is ready for human review.",
		PullRequestURL: "https://github.com/ZYOOO/TestRepo/pull/44",
		BranchName:     "omega/OMG-29-devflow",
		ChangedFiles:   []string{"index.html", "src/App.tsx"},
		ChecksOutput:   "npm test passed",
		CheckLogFeedback: []map[string]any{
			{"label": "unit", "message": "all checks passed"},
		},
	})
	if text(report, "state") != "synced" {
		t.Fatalf("sync state = %+v", report)
	}
	body, err := os.ReadFile(bodyPath)
	if err != nil {
		t.Fatal(err)
	}
	bodyText := string(body)
	for _, expected := range []string{"Omega delivery update", "OMG-29", "human_review.waiting", "https://github.com/ZYOOO/TestRepo/pull/44", "npm test passed", "index.html"} {
		if !strings.Contains(bodyText, expected) {
			t.Fatalf("comment body missing %q:\n%s", expected, bodyText)
		}
	}
	prBody, err := os.ReadFile(prBodyPath)
	if err != nil {
		t.Fatal(err)
	}
	prBodyText := string(prBody)
	for _, expected := range []string{"Omega review packet", "OMG-29", "human_review.waiting", "npm test passed", "index.html"} {
		if !strings.Contains(prBodyText, expected) {
			t.Fatalf("PR comment body missing %q:\n%s", expected, prBodyText)
		}
	}
	logRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	logText := string(logRaw)
	for _, expected := range []string{"issue comment 29 --repo ZYOOO/TestRepo", "label create omega:managed", "label create omega:review", "issue edit 29 --repo ZYOOO/TestRepo", "pr comment https://github.com/ZYOOO/TestRepo/pull/44"} {
		if !strings.Contains(logText, expected) {
			t.Fatalf("gh command log missing %q:\n%s", expected, logText)
		}
	}
}

func TestSyncGitHubIssueOutboundPostsPRCommentWithoutIssue(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "gh.log")
	prBodyPath := filepath.Join(tempDir, "pr-body.md")
	binDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeGH := filepath.Join(binDir, "gh")
	script := `#!/bin/sh
printf '%s\n' "$PWD :: $*" >> "$OMEGA_FAKE_GH_LOG"
if [ "$1" = "pr" ] && [ "$2" = "comment" ]; then
  while [ "$#" -gt 0 ]; do
    if [ "$1" = "--body-file" ]; then
      cat "$2" > "$OMEGA_FAKE_GH_PR_BODY"
    fi
    shift
  done
  echo "pr commented"
  exit 0
fi
echo "ok"
`
	if err := os.WriteFile(fakeGH, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	repoDir := filepath.Join(tempDir, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("OMEGA_FAKE_GH_LOG", logPath)
	t.Setenv("OMEGA_FAKE_GH_PR_BODY", prBodyPath)

	report := (&Server{}).syncGitHubIssueOutbound(context.Background(), githubOutboundSyncInput{
		RepositoryPath: repoDir,
		WorkItem:       map[string]any{"id": "item_manual", "key": "OMG-30", "title": "Manual item"},
		Pipeline:       map[string]any{"id": "pipeline_30"},
		AttemptID:      "attempt_30",
		Event:          "human_review.waiting",
		Status:         "waiting-human",
		PullRequestURL: "https://github.com/ZYOOO/TestRepo/pull/45",
	})
	if text(report, "state") != "partial" || !boolValue(report["prSynced"]) {
		t.Fatalf("PR-only sync should partially succeed: %+v", report)
	}
	raw, err := os.ReadFile(prBodyPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "Omega review packet") {
		t.Fatalf("PR body = %s", string(raw))
	}
}

func TestSyncGitHubIssueOutboundSkipsUnlinkedWorkItem(t *testing.T) {
	report := (&Server{}).syncGitHubIssueOutbound(context.Background(), githubOutboundSyncInput{
		WorkItem: map[string]any{"id": "item_manual"},
		Pipeline: map[string]any{"id": "pipeline_manual"},
		Event:    "attempt.started",
		Status:   "running",
	})
	if text(report, "state") != "skipped" {
		t.Fatalf("unlinked work item should skip sync: %+v", report)
	}
}

func TestGitHubCITriggerRerunsFailedRuns(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "gh.log")
	binDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeGH := filepath.Join(binDir, "gh")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$OMEGA_FAKE_GH_LOG"
echo "rerun queued"
`
	if err := os.WriteFile(fakeGH, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	repoDir := filepath.Join(tempDir, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("OMEGA_FAKE_GH_LOG", logPath)
	t.Setenv("OMEGA_GITHUB_CI_TRIGGER", "rerun-failed")

	report := (&Server{}).triggerGitHubCIIfConfigured(context.Background(), repoDir, githubOutboundSyncInput{
		CheckLogFeedback: []map[string]any{{"runId": "123"}, {"run_id": "123"}, {"workflowRunId": "456"}},
	})
	if text(report, "state") != "triggered" {
		t.Fatalf("CI rerun report = %+v", report)
	}
	logRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	logText := string(logRaw)
	for _, expected := range []string{"run rerun 123 --failed", "run rerun 456 --failed"} {
		if !strings.Contains(logText, expected) {
			t.Fatalf("gh command log missing %q:\n%s", expected, logText)
		}
	}
}

func TestGitHubCITriggerWorkflowDispatch(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "gh.log")
	binDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeGH := filepath.Join(binDir, "gh")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$OMEGA_FAKE_GH_LOG"
echo "workflow dispatched"
`
	if err := os.WriteFile(fakeGH, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	repoDir := filepath.Join(tempDir, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("OMEGA_FAKE_GH_LOG", logPath)
	t.Setenv("OMEGA_GITHUB_CI_TRIGGER", "workflow-dispatch")
	t.Setenv("OMEGA_GITHUB_CI_WORKFLOW", "ci.yml")
	t.Setenv("OMEGA_GITHUB_CI_INPUTS", `{"reason":"review","attempt":"attempt_1"}`)

	report := (&Server{}).triggerGitHubCIIfConfigured(context.Background(), repoDir, githubOutboundSyncInput{BranchName: "omega/OMG-1-devflow"})
	if text(report, "state") != "triggered" {
		t.Fatalf("workflow dispatch report = %+v", report)
	}
	logRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	logText := string(logRaw)
	for _, expected := range []string{"workflow run ci.yml --ref omega/OMG-1-devflow", "-f reason=review", "-f attempt=attempt_1"} {
		if !strings.Contains(logText, expected) {
			t.Fatalf("gh command log missing %q:\n%s", expected, logText)
		}
	}
}

func TestGitHubIssueOutboundLabelsMapCompletionAndFailures(t *testing.T) {
	completed := githubIssueOutboundLabels("done", "delivery.completed")
	if len(completed.add) < 2 || completed.add[1].name != "omega:done" {
		t.Fatalf("completion label = %+v", completed.add)
	}
	failed := githubIssueOutboundLabels("failed", "delivery.merge_failed")
	if len(failed.add) < 2 || failed.add[1].name != "omega:blocked" {
		t.Fatalf("failed label = %+v", failed.add)
	}
	review := githubIssueOutboundLabels("waiting-human", "human_review.waiting")
	if len(review.add) < 2 || review.add[1].name != "omega:review" {
		t.Fatalf("review label = %+v", review.add)
	}
}
