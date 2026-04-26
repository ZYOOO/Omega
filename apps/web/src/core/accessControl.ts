export type UserRole = "viewer" | "operator" | "reviewer" | "admin";
export type MissionAction =
  | "view"
  | "run-operation"
  | "run-codex"
  | "approve-checkpoint"
  | "manage-provider";

export interface MissionUser {
  role: UserRole;
}

const rolePermissions: Record<UserRole, MissionAction[]> = {
  viewer: ["view"],
  operator: ["view", "run-operation"],
  reviewer: ["view", "run-operation", "approve-checkpoint"],
  admin: ["view", "run-operation", "run-codex", "approve-checkpoint", "manage-provider"]
};

export function canPerformAction(user: MissionUser, action: MissionAction): boolean {
  return rolePermissions[user.role].includes(action);
}
