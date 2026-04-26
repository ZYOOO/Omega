import type { WorkItem } from "../core";
import type { LocalRunnerCommand } from "../local/localMissionRunner";

export interface GhIssueJson {
  number: number;
  title: string;
  body?: string | null;
  state: "OPEN" | "CLOSED";
  assignees: Array<{ login: string }>;
  labels: Array<{ name: string }>;
}

export function buildGhIssueListCommand(input: {
  owner: string;
  repo: string;
}): LocalRunnerCommand {
  return {
    executable: "gh",
    args: [
      "issue",
      "list",
      "--repo",
      `${input.owner}/${input.repo}`,
      "--state",
      "all",
      "--json",
      "number,title,body,assignees,labels,state"
    ]
  };
}

export function mapGhIssueJsonToWorkItem(issue: GhIssueJson): WorkItem {
  return {
    id: `gh_issue_${issue.number}`,
    key: `GH-${issue.number}`,
    title: issue.title,
    description: issue.body ?? "",
    status: issue.state === "OPEN" ? "Ready" : "Done",
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
