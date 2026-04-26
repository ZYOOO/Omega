import { spawn } from "child_process";
import { mapGhIssueJsonToWorkItem, type GhIssueJson } from "../integrations/ghAdapter";
import type { WorkItem } from "../core";

interface CommandResult {
  stdout: string;
  stderr: string;
  exitCode: number | null;
}

export interface GitHubRepoInfo {
  name: string;
  nameWithOwner: string;
  description: string | null;
  isPrivate: boolean;
  defaultBranchRef?: { name: string } | null;
  url: string;
}

export type CommandRunner = (command: string, args: string[]) => Promise<CommandResult>;

export function createSpawnRunner(env: Record<string, string | undefined> = process.env): CommandRunner {
  return (command, args) =>
    new Promise((resolve) => {
      const child = spawn(command, args, { shell: false, env });
      let stdout = "";
      let stderr = "";
      child.stdout.on("data", (chunk) => {
        stdout += chunk.toString();
      });
      child.stderr.on("data", (chunk) => {
        stderr += chunk.toString();
      });
      child.on("close", (exitCode) => resolve({ stdout, stderr, exitCode }));
      child.on("error", (error) => resolve({ stdout, stderr: error.message, exitCode: null }));
    });
}

async function runJson<T>(runner: CommandRunner, args: string[]): Promise<T> {
  const result = await runner("gh", args);
  if (result.exitCode !== 0) {
    throw new Error(result.stderr || `gh exited with ${result.exitCode}`);
  }
  return JSON.parse(result.stdout) as T;
}

export async function getGhStatus(runner: CommandRunner = createSpawnRunner()): Promise<{
  installed: true;
  authenticated: boolean;
  detail: string;
}> {
  const result = await runner("gh", ["auth", "status"]);
  return {
    installed: true,
    authenticated: result.exitCode === 0,
    detail: result.exitCode === 0 ? "authenticated" : (result.stderr || "not authenticated").trim()
  };
}

export async function getGitHubRepoInfo(
  owner: string,
  repo: string,
  runner: CommandRunner = createSpawnRunner()
): Promise<GitHubRepoInfo> {
  return runJson<GitHubRepoInfo>(runner, [
    "repo",
    "view",
    `${owner}/${repo}`,
    "--json",
    "name,nameWithOwner,description,isPrivate,defaultBranchRef,url"
  ]);
}

export async function importGitHubIssuesAsWorkItems(
  owner: string,
  repo: string,
  runner: CommandRunner = createSpawnRunner()
): Promise<WorkItem[]> {
  const issues = await runJson<GhIssueJson[]>(runner, [
    "issue",
    "list",
    "--repo",
    `${owner}/${repo}`,
    "--state",
    "all",
    "--limit",
    "100",
    "--json",
    "number,title,body,assignees,labels,state"
  ]);

  return issues.map(mapGhIssueJsonToWorkItem);
}
