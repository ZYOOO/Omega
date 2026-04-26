import type { ProviderToken } from "./tokenStore";

export interface GitHubOAuthExchangeInput {
  clientId: string;
  clientSecret: string;
  code: string;
  redirectUri: string;
  accountId?: string;
  fetchImpl?: typeof fetch;
}

interface GitHubOAuthResponse {
  access_token: string;
  scope: string;
  token_type: string;
}

export async function exchangeGitHubOAuthCode(
  input: GitHubOAuthExchangeInput
): Promise<ProviderToken> {
  const fetcher = input.fetchImpl ?? fetch;
  const response = await fetcher("https://github.com/login/oauth/access_token", {
    method: "POST",
    headers: {
      accept: "application/json",
      "content-type": "application/json"
    },
    body: JSON.stringify({
      client_id: input.clientId,
      client_secret: input.clientSecret,
      code: input.code,
      redirect_uri: input.redirectUri
    })
  });

  if (!response.ok) {
    throw new Error(`GitHub OAuth exchange failed: ${response.status}`);
  }

  const payload = (await response.json()) as GitHubOAuthResponse;

  return {
    provider: "github",
    accountId: input.accountId ?? "default",
    accessToken: payload.access_token,
    scopes: payload.scope.split(",").map((scope) => scope.trim()).filter(Boolean)
  };
}
