import { mkdir, readFile, unlink, writeFile } from "fs/promises";
import { join } from "path";

export type TokenProvider = "github" | "google" | "feishu" | "ci";

export interface ProviderToken {
  provider: TokenProvider;
  accountId: string;
  accessToken: string;
  scopes: string[];
  expiresAt?: string;
}

export interface TokenStore {
  saveToken(token: ProviderToken): Promise<void>;
  getToken(provider: TokenProvider, accountId: string): Promise<ProviderToken | undefined>;
  revokeToken(provider: TokenProvider, accountId: string): Promise<void>;
}

function tokenKey(provider: TokenProvider, accountId: string): string {
  return `${provider}:${accountId}`;
}

export class InMemoryTokenStore implements TokenStore {
  private readonly tokens = new Map<string, ProviderToken>();

  async saveToken(token: ProviderToken): Promise<void> {
    this.tokens.set(tokenKey(token.provider, token.accountId), { ...token });
  }

  async getToken(provider: TokenProvider, accountId: string): Promise<ProviderToken | undefined> {
    const token = this.tokens.get(tokenKey(provider, accountId));
    return token ? { ...token } : undefined;
  }

  async revokeToken(provider: TokenProvider, accountId: string): Promise<void> {
    this.tokens.delete(tokenKey(provider, accountId));
  }
}

function safeTokenSegment(input: string): string {
  return input.replace(/[^a-zA-Z0-9._-]+/g, "_");
}

export class FileTokenStore implements TokenStore {
  constructor(private readonly root: string) {}

  async saveToken(token: ProviderToken): Promise<void> {
    await mkdir(this.root, { recursive: true });
    await writeFile(this.pathFor(token.provider, token.accountId), JSON.stringify(token, null, 2));
  }

  async getToken(provider: TokenProvider, accountId: string): Promise<ProviderToken | undefined> {
    try {
      return JSON.parse(await readFile(this.pathFor(provider, accountId), "utf8")) as ProviderToken;
    } catch {
      return undefined;
    }
  }

  async revokeToken(provider: TokenProvider, accountId: string): Promise<void> {
    try {
      await unlink(this.pathFor(provider, accountId));
    } catch {
      // Already revoked.
    }
  }

  private pathFor(provider: TokenProvider, accountId: string): string {
    return join(this.root, `${safeTokenSegment(provider)}__${safeTokenSegment(accountId)}.json`);
  }
}
