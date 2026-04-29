export function workItemDetailHash(itemId: string): string {
  return `#/work-items/${encodeURIComponent(itemId)}`;
}

export function parseWorkItemDetailHash(hash: string): string {
  const match = hash.match(/^#\/work-items\/([^/?#]+)$/);
  return match ? decodeURIComponent(match[1]) : "";
}

export function isWorkItemDetailHash(hash: string): boolean {
  return Boolean(parseWorkItemDetailHash(hash));
}

