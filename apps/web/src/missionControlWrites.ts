export function requireMissionControlApi(apiUrl: string, action: string): string {
  if (apiUrl.trim()) return apiUrl;
  throw new Error(`${action} requires the local runtime. Mission Control is the only writer for workspace data.`);
}

export function missionControlUnavailableMessage(action: string): string {
  return `${action} requires the local runtime. Start Omega Desktop or the Go local runtime first.`;
}
