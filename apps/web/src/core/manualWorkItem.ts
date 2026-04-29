import type { WorkItem } from "./workboard";

export function titleFromMarkdownDescription(description: string): string {
  const firstMeaningfulLine = description
    .split(/\r?\n/)
    .map((line) => line.trim())
    .find((line) => line.length > 0);
  if (!firstMeaningfulLine) return "";
  return firstMeaningfulLine
    .replace(/^#{1,6}\s+/, "")
    .replace(/^[-*]\s+/, "")
    .replace(/^>\s+/, "")
    .replace(/[`*_]+/g, "")
    .trim()
    .slice(0, 120);
}

export function createManualWorkItem(
  index: number,
  title: string,
  description: string,
  assignee: string,
  target: string,
  repositoryTargetId?: string
): WorkItem {
  return {
    id: `item_manual_${index}`,
    key: `OMG-${index}`,
    title,
    description,
    status: "Ready",
    priority: "High",
    assignee,
    labels: ["manual", "ai-delivery"],
    team: "Omega",
    stageId: "intake",
    target,
    source: "manual",
    repositoryTargetId,
    acceptanceCriteria: ["The requested change can be verified by a human reviewer."],
    blockedByItemIds: []
  };
}
