import type {
  GitHubCheckRunPayload,
  GitHubIssuePayload,
  GitHubPullRequestPayload
} from "./githubAdapter";

export interface GitHubAuthorizeUrlInput {
  clientId: string;
  redirectUri: string;
  state: string;
  scopes: string[];
}

export interface GitHubIssueCommentPayload {
  issueNumber: number;
  body: string;
}

export interface GitHubClient {
  listIssues(): Promise<GitHubIssuePayload[]>;
  getPullRequest(number: number): Promise<GitHubPullRequestPayload | undefined>;
  listCheckRuns(ref?: string): Promise<GitHubCheckRunPayload[]>;
  createIssueComment(payload: GitHubIssueCommentPayload): Promise<void>;
}

export interface InMemoryGitHubClientSeed {
  issues: GitHubIssuePayload[];
  pullRequests: GitHubPullRequestPayload[];
  checkRuns: GitHubCheckRunPayload[];
}

export function buildGitHubAuthorizeUrl(input: GitHubAuthorizeUrlInput): string {
  const url = new URL("https://github.com/login/oauth/authorize");
  url.searchParams.set("client_id", input.clientId);
  url.searchParams.set("redirect_uri", input.redirectUri);
  url.searchParams.set("scope", input.scopes.join(" "));
  url.searchParams.set("state", input.state);
  return url.toString();
}

export class InMemoryGitHubClient implements GitHubClient {
  public comments: GitHubIssueCommentPayload[] = [];

  constructor(private readonly seed: InMemoryGitHubClientSeed) {}

  async listIssues(): Promise<GitHubIssuePayload[]> {
    return this.seed.issues.map((issue) => ({ ...issue }));
  }

  async getPullRequest(number: number): Promise<GitHubPullRequestPayload | undefined> {
    const pullRequest = this.seed.pullRequests.find((candidate) => candidate.number === number);
    return pullRequest ? { ...pullRequest } : undefined;
  }

  async listCheckRuns(_ref?: string): Promise<GitHubCheckRunPayload[]> {
    return this.seed.checkRuns.map((checkRun) => ({ ...checkRun }));
  }

  async createIssueComment(payload: GitHubIssueCommentPayload): Promise<void> {
    this.comments.push(payload);
  }
}
