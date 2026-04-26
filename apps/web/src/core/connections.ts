export type ProviderId = "github" | "feishu" | "google" | "ci";
export type ProviderStatus = "not-connected" | "connected" | "needs-config";
export type AuthMethod = "oauth" | "github-app" | "mcp" | "webhook";

export interface ProviderPermission {
  id: string;
  label: string;
  risk: "read" | "write" | "admin";
}

export interface ConnectionProvider {
  id: ProviderId;
  name: string;
  authMethod: AuthMethod;
  authorizeUrl?: string;
  scopes: string[];
  permissions: ProviderPermission[];
  description: string;
}

export type ConnectionState = Record<
  ProviderId,
  {
    status: ProviderStatus;
    grantedPermissions: string[];
    connectedAs?: string;
  }
>;

export interface AuthorizeUrlInput {
  clientId: string;
  redirectUri: string;
  state: string;
}

export const connectionProviders: ConnectionProvider[] = [
  {
    id: "github",
    name: "GitHub",
    authMethod: "oauth",
    authorizeUrl: "https://github.com/login/oauth/authorize",
    scopes: ["repo", "read:org", "workflow"],
    permissions: [
      { id: "repo:read", label: "Read repositories and branches", risk: "read" },
      { id: "pull_request:write", label: "Open PRs and reply to review comments", risk: "write" },
      { id: "checks:read", label: "Read checks, jobs, and CI logs", risk: "read" },
      { id: "issues:write", label: "Sync delivery events back to issues", risk: "write" }
    ],
    description: "Source, pull request, review, and CI access for workspace runners."
  },
  {
    id: "feishu",
    name: "Feishu",
    authMethod: "mcp",
    scopes: ["im:message", "card:write", "approval:read"],
    permissions: [
      { id: "messages:write", label: "Send delivery notifications", risk: "write" },
      { id: "approval:read", label: "Read human gate card decisions", risk: "read" },
      { id: "cards:write", label: "Create approve or request-changes cards", risk: "write" }
    ],
    description: "Human approvals, team notifications, and delivery status cards."
  },
  {
    id: "google",
    name: "Google",
    authMethod: "oauth",
    authorizeUrl: "https://accounts.google.com/o/oauth2/v2/auth",
    scopes: ["openid", "email", "profile"],
    permissions: [
      { id: "identity:read", label: "Confirm workspace identity", risk: "read" }
    ],
    description: "Primary identity provider for sign-in and account linking."
  },
  {
    id: "ci",
    name: "CI",
    authMethod: "webhook",
    scopes: ["checks:read", "logs:read"],
    permissions: [
      { id: "checks:read", label: "Read check state", risk: "read" },
      { id: "logs:read", label: "Read job logs for test failures", risk: "read" }
    ],
    description: "Continuous integration evidence for testing, review, and delivery."
  }
];

export function createInitialConnectionState(): ConnectionState {
  return Object.fromEntries(
    connectionProviders.map((provider) => [
      provider.id,
      {
        status: provider.id === "google" ? "connected" : "not-connected",
        grantedPermissions: provider.id === "google"
          ? provider.permissions.map((permission) => permission.id)
          : []
      }
    ])
  ) as ConnectionState;
}

export function getConnectionProvider(providerId: ProviderId): ConnectionProvider {
  const provider = connectionProviders.find((candidate) => candidate.id === providerId);

  if (!provider) {
    throw new Error(`Unknown connection provider: ${providerId}`);
  }

  return provider;
}

export function buildAuthorizeUrl(providerId: ProviderId, input: AuthorizeUrlInput): string {
  const provider = getConnectionProvider(providerId);

  if (!provider.authorizeUrl) {
    throw new Error(`${provider.name} does not support OAuth authorization URLs`);
  }

  const url = new URL(provider.authorizeUrl);
  url.searchParams.set("client_id", input.clientId);
  url.searchParams.set("redirect_uri", input.redirectUri);
  url.searchParams.set("scope", provider.scopes.join(" "));
  url.searchParams.set("state", input.state);

  return url.toString();
}

export function grantProviderConnection(
  state: ConnectionState,
  providerId: ProviderId,
  connectedAs = "workspace-admin"
): ConnectionState {
  const provider = getConnectionProvider(providerId);

  return {
    ...state,
    [providerId]: {
      status: "connected",
      connectedAs,
      grantedPermissions: provider.permissions.map((permission) => permission.id)
    }
  };
}

export function revokeProviderConnection(state: ConnectionState, providerId: ProviderId): ConnectionState {
  return {
    ...state,
    [providerId]: {
      status: "not-connected",
      grantedPermissions: []
    }
  };
}

export function providerHasPermission(
  state: ConnectionState,
  providerId: ProviderId,
  permissionId: string
): boolean {
  return state[providerId].grantedPermissions.includes(permissionId);
}
