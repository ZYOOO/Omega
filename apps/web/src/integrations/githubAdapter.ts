import type { StageEvidence } from "../core";
import type { WorkItem } from "../core/workboard";

export interface GitHubIssuePayload {
  id: number;
  number: number;
  title: string;
  body?: string | null;
  state: "open" | "closed";
  labels: Array<{ name: string }>;
  assignees: Array<{ login: string }>;
}

export interface GitHubPullRequestPayload {
  number: number;
  title: string;
  html_url: string;
  mergeable_state?: string | null;
}

export interface GitHubCheckRunPayload {
  name: string;
  status: string;
  conclusion?: string | null;
  html_url: string;
}

export interface GitHubProofCommentInput {
  operationTitle: string;
  summary: string;
  proofFiles: string[];
}

export function mapGitHubIssueToWorkItem(issue: GitHubIssuePayload): WorkItem {
  return {
    id: `github_issue_${issue.id}`,
    key: `GH-${issue.number}`,
    title: issue.title,
    description: issue.body ?? "",
    status: issue.state === "closed" ? "Done" : "Ready",
    priority: "No priority",
    assignee: issue.assignees[0]?.login ?? "Unassigned",
    labels: issue.labels.map((label) => label.name),
    team: "GitHub",
    stageId: "intake",
    target: "No target",
    source: "github_issue",
    sourceExternalRef: `#${issue.number}`,
    acceptanceCriteria: ["Imported GitHub issue is understood", "Implementation resolves the issue"],
    blockedByItemIds: []
  };
}

export function mapGitHubPullRequestToProof(pr: GitHubPullRequestPayload): StageEvidence {
  return {
    id: `github_pr_${pr.number}`,
    label: "Pull Request",
    value: `#${pr.number} ${pr.title} (${pr.mergeable_state ?? "unknown"})`,
    url: pr.html_url
  };
}

export function mapGitHubCheckRunToProof(checkRun: GitHubCheckRunPayload): StageEvidence {
  return {
    id: `github_check_${checkRun.name}`,
    label: "Check Run",
    value: `${checkRun.name}: ${checkRun.status} / ${checkRun.conclusion ?? "unknown"}`,
    url: checkRun.html_url
  };
}

export function createGitHubProofComment(input: GitHubProofCommentInput): string {
  return [
    `Mission Control proof for ${input.operationTitle}`,
    "",
    input.summary,
    "",
    "Proof files:",
    ...input.proofFiles.map((proofFile) => `- ${proofFile}`)
  ].join("\n");
}
