export type ProjectRepositoryTarget =
  | {
      id: string;
      kind: "github";
      owner: string;
      repo: string;
      defaultBranch: string;
      url?: string;
    }
  | {
      id: string;
      kind: "local";
      path: string;
      defaultBranch: string;
    };

export type WorkspaceItemPriority = "Low" | "Medium" | "High" | "Urgent";
export type WorkspaceItemStatus = "Todo" | "In Progress" | "In Review" | "Done" | "Blocked";

export interface Project {
  id: string;
  key: string;
  name: string;
  repositoryTargets: ProjectRepositoryTarget[];
  defaultRepositoryTargetId?: string;
}

export interface WorkspaceItem {
  id: string;
  key: string;
  title: string;
  description: string;
  assignee: string;
  priority: WorkspaceItemPriority;
  status: WorkspaceItemStatus;
}

export interface Workspace {
  id: string;
  key: string;
  projectId: string;
  name: string;
  branch: string;
  items: WorkspaceItem[];
}

let projectCounter = 1;
let workspaceCounter = 1;
let itemCounter = 1;

export function createProject(input: {
  name: string;
  repositoryTargets?: ProjectRepositoryTarget[];
  defaultRepositoryTargetId?: string;
}): Project {
  const number = projectCounter++;
  const repositoryTargets = input.repositoryTargets ?? [];
  return {
    id: `project_${number}`,
    key: `PRJ-${number}`,
    name: input.name,
    repositoryTargets,
    defaultRepositoryTargetId: input.defaultRepositoryTargetId ?? repositoryTargets[0]?.id
  };
}

export function createWorkspace(
  project: Project,
  input: {
    name: string;
    branch: string;
  }
): Workspace {
  const number = workspaceCounter++;
  return {
    id: `workspace_${number}`,
    key: `WKS-${number}`,
    projectId: project.id,
    name: input.name,
    branch: input.branch,
    items: []
  };
}

export function addItemToWorkspace(
  workspace: Workspace,
  input: {
    title: string;
    description: string;
    assignee: string;
    priority: WorkspaceItemPriority;
  }
): Workspace {
  const number = itemCounter++;
  return {
    ...workspace,
    items: [
      ...workspace.items,
      {
        id: `workspace_item_${number}`,
        key: `ITM-${number}`,
        title: input.title,
        description: input.description,
        assignee: input.assignee,
        priority: input.priority,
        status: "Todo"
      }
    ]
  };
}

export function listWorkspaceItems(workspace: Workspace): WorkspaceItem[] {
  return workspace.items;
}
