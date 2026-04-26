import { createServer } from "http";
import { handleGitHubOAuthCallback } from "../integrations/githubCallbackHandler";
import type { TokenStore } from "../integrations/tokenStore";

export interface LocalAuthApiOptions {
  workspaceRoot: string;
  tokenStore: TokenStore;
  github: {
    clientId: string;
    clientSecret: string;
    redirectUri: string;
  };
  fetchImpl?: typeof fetch;
  port: number;
  host?: string;
}

export interface LocalAuthApiServer {
  url: string;
  close(): Promise<void>;
}

function sendJson(response: { statusCode: number; setHeader(name: string, value: string): void; end(body: string): void }, statusCode: number, body: unknown): void {
  response.statusCode = statusCode;
  response.setHeader("content-type", "application/json");
  response.end(JSON.stringify(body));
}

export function startLocalAuthApi(options: LocalAuthApiOptions): Promise<LocalAuthApiServer> {
  const host = options.host ?? "127.0.0.1";

  return new Promise((resolve) => {
    const server = createServer(async (request, response) => {
      if (request.method !== "GET" || !request.url?.startsWith("/auth/github/callback")) {
        sendJson(response, 404, { error: "not found" });
        return;
      }

      try {
        const url = new URL(request.url, `http://${host}`);
        const code = url.searchParams.get("code");
        const state = url.searchParams.get("state");
        if (!code || !state) {
          sendJson(response, 400, { error: "code and state are required" });
          return;
        }

        const result = await handleGitHubOAuthCallback({
          code,
          state,
          clientId: options.github.clientId,
          clientSecret: options.github.clientSecret,
          redirectUri: options.github.redirectUri,
          tokenStore: options.tokenStore,
          fetchImpl: options.fetchImpl
        });
        sendJson(response, 200, result);
      } catch (error) {
        sendJson(response, 500, { error: error instanceof Error ? error.message : "unknown error" });
      }
    });

    server.listen(options.port, host, () => {
      const address = server.address();
      const port = typeof address === "object" && address ? address.port : options.port;
      resolve({
        url: `http://${host}:${port}`,
        close: () => new Promise((closeResolve) => server.close(() => closeResolve()))
      });
    });
  });
}
