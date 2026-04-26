export type AuthProvider =
  | "google"
  | "github"
  | "feishu"
  | "wechat-work"
  | "dingtalk";

export interface ConnectedAccount {
  provider: AuthProvider;
  displayName: string;
  connected: boolean;
  scopes: string[];
  expiresAt?: string;
}

export const defaultConnectedAccounts: ConnectedAccount[] = [
  {
    provider: "google",
    displayName: "Google Login",
    connected: true,
    scopes: ["openid", "email", "profile"]
  },
  {
    provider: "github",
    displayName: "GitHub",
    connected: true,
    scopes: ["repo", "read:org", "workflow"]
  },
  {
    provider: "feishu",
    displayName: "Feishu",
    connected: false,
    scopes: ["im:message", "card:write"]
  },
  {
    provider: "wechat-work",
    displayName: "WeCom",
    connected: false,
    scopes: ["chat:write", "approval:read"]
  },
  {
    provider: "dingtalk",
    displayName: "DingTalk",
    connected: false,
    scopes: ["robot:send", "approval:read"]
  }
];

export function connectedProviders(accounts: ConnectedAccount[]): AuthProvider[] {
  return accounts.filter((account) => account.connected).map((account) => account.provider);
}
