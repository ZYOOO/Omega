import type { WorkItem, WorkItemPriority, WorkItemStatus } from "./workboard";

export type WorkItemDisplayField = keyof Pick<
  WorkItem,
  "key" | "title" | "status" | "priority" | "assignee" | "target" | "team"
>;

export interface WorkboardViewFilters {
  status?: WorkItemStatus[];
  assignee?: string[];
  labels?: string[];
}

export interface WorkboardViewSort {
  field: "priority" | "status" | "target" | "key";
  direction: "asc" | "desc";
}

export interface WorkboardViewOptions {
  filters: WorkboardViewFilters;
  sort: WorkboardViewSort;
  display: WorkItemDisplayField[];
}

export interface WorkboardView {
  items: WorkItem[];
  display: WorkItemDisplayField[];
}

const priorityRank: Record<WorkItemPriority, number> = {
  "No priority": 0,
  Low: 1,
  Medium: 2,
  High: 3,
  Urgent: 4
};

function matchesFilters(item: WorkItem, filters: WorkboardViewFilters): boolean {
  if (filters.status?.length && !filters.status.includes(item.status)) return false;
  if (filters.assignee?.length && !filters.assignee.includes(item.assignee)) return false;
  if (filters.labels?.length && !filters.labels.every((label) => item.labels.includes(label))) return false;
  return true;
}

function sortValue(item: WorkItem, field: WorkboardViewSort["field"]): string | number {
  if (field === "priority") return priorityRank[item.priority];
  return item[field];
}

export function createWorkboardView(items: WorkItem[], options: WorkboardViewOptions): WorkboardView {
  const direction = options.sort.direction === "asc" ? 1 : -1;
  return {
    display: options.display,
    items: items
      .filter((item) => matchesFilters(item, options.filters))
      .sort((a, b) => {
        const left = sortValue(a, options.sort.field);
        const right = sortValue(b, options.sort.field);
        if (left < right) return -1 * direction;
        if (left > right) return 1 * direction;
        return a.key.localeCompare(b.key);
      })
  };
}
