import { exchangeGitHubOAuthCode } from "./githubOAuth";
import type { TokenStore } from "./tokenStore";

export interface GitHubOAuthCallbackInput {
  code: string;
  state: string;
  clientId: string;
  clientSecret: string;
  redirectUri: string;
  tokenStore: TokenStore;
  fetchImpl?: typeof fetch;
}

export interface GitHubOAuthCallbackResult {
  connected: boolean;
  accountId: string;
  scopes: string[];
}

export async function handleGitHubOAuthCallback(
  input: GitHubOAuthCallbackInput
): Promise<GitHubOAuthCallbackResult> {
  const token = await exchangeGitHubOAuthCode({
    clientId: input.clientId,
    clientSecret: input.clientSecret,
    code: input.code,
    redirectUri: input.redirectUri,
    accountId: input.state,
    fetchImpl: input.fetchImpl
  });

  await input.tokenStore.saveToken(token);

  return {
    connected: true,
    accountId: token.accountId,
    scopes: token.scopes
  };
}
