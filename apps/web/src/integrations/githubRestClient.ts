import type {
  GitHubCheckRunPayload,
  GitHubIssuePayload,
  GitHubPullRequestPayload
} from "./githubAdapter";
import type { GitHubClient, GitHubIssueCommentPayload } from "./githubClient";

export interface GitHubRestClientOptions {
  owner: string;
  repo: string;
  token: string;
  fetchImpl?: typeof fetch;
}

export class GitHubRestClient implements GitHubClient {
  private readonly fetcher: typeof fetch;

  constructor(private readonly options: GitHubRestClientOptions) {
    this.fetcher = options.fetchImpl ?? fetch;
  }

  async listIssues(): Promise<GitHubIssuePayload[]> {
    return this.request<GitHubIssuePayload[]>(`/issues?state=open`);
  }

  async getPullRequest(number: number): Promise<GitHubPullRequestPayload | undefined> {
    return this.request<GitHubPullRequestPayload>(`/pulls/${number}`);
  }

  async listCheckRuns(ref = "main"): Promise<GitHubCheckRunPayload[]> {
    const response = await this.request<{ check_runs: GitHubCheckRunPayload[] }>(`/commits/${ref}/check-runs`);
    return response.check_runs ?? [];
  }

  async createIssueComment(payload: GitHubIssueCommentPayload): Promise<void> {
    await this.request(`/issues/${payload.issueNumber}/comments`, {
      method: "POST",
      body: JSON.stringify({ body: payload.body })
    });
  }

  private async request<T>(path: string, init: RequestInit = {}): Promise<T> {
    const response = await this.fetcher(
      `https://api.github.com/repos/${this.options.owner}/${this.options.repo}${path}`,
      {
        ...init,
        headers: {
          authorization: `Bearer ${this.options.token}`,
          accept: "application/vnd.github+json",
          "content-type": "application/json",
          ...(init.headers ?? {})
        }
      }
    );

    if (!response.ok) {
      throw new Error(`GitHub request failed: ${response.status}`);
    }

    return response.json() as Promise<T>;
  }
}
