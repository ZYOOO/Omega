export function navigateToExternalUrl(url: string): void {
  window.location.assign(url);
}

export function openExternalUrlInNewTab(url: string): void {
  window.open(url, "_blank", "noopener,noreferrer");
}
