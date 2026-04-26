import type { SyncIntent, WorkItem } from "../core";
import { applyWorkboardSyncIntent } from "../core";
import type { GitHubClient } from "./githubClient";
import { executeGitHubSyncIntent } from "./githubSyncExecutor";
import type { FeishuClient } from "./feishuClient";
import { executeFeishuSyncIntent } from "./feishuSyncExecutor";

export interface ConnectorRuntimeOptions {
  workItems: WorkItem[];
  github?: GitHubClient;
  feishu?: FeishuClient;
}

export interface ConnectorRuntimeReport {
  applied: SyncIntent[];
  failed: Array<{ intent: SyncIntent; error: string }>;
}

export class ConnectorRuntime {
  public workItems: WorkItem[];
  public readonly github?: GitHubClient;
  public readonly feishu?: FeishuClient;

  constructor(options: ConnectorRuntimeOptions) {
    this.workItems = options.workItems;
    this.github = options.github;
    this.feishu = options.feishu;
  }

  async execute(intents: SyncIntent[]): Promise<ConnectorRuntimeReport> {
    const report: ConnectorRuntimeReport = { applied: [], failed: [] };

    for (const intent of intents) {
      try {
        await this.executeOne(intent);
        report.applied.push(intent);
      } catch (error) {
        report.failed.push({
          intent,
          error: error instanceof Error ? error.message : "unknown error"
        });
      }
    }

    return report;
  }

  private async executeOne(intent: SyncIntent): Promise<void> {
    if (intent.provider === "workboard") {
      this.workItems = applyWorkboardSyncIntent(this.workItems, intent);
      return;
    }

    if (intent.provider === "github") {
      if (!this.github) {
        throw new Error("GitHub client is not configured");
      }
      await executeGitHubSyncIntent(this.github, intent);
      return;
    }

    if (intent.provider === "feishu") {
      if (!this.feishu) {
        throw new Error("Feishu client is not configured");
      }
      await executeFeishuSyncIntent(this.feishu, intent);
    }
  }
}
