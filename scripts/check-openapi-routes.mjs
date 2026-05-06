import { readFileSync } from "node:fs";

const routeSourcePath = "services/local-runtime/internal/omegalocal/server_routes.go";
const openApiPath = "docs/openapi.yaml";
const methods = new Set(["get", "put", "post", "patch", "delete"]);

function routeParamName(prefix) {
  if (prefix.startsWith("/workflow-templates")) return "templateId";
  if (prefix.startsWith("/work-items")) return "itemId";
  if (prefix.startsWith("/pipelines")) return "pipelineId";
  if (prefix.startsWith("/checkpoints")) return "checkpointId";
  return "id";
}

function parseServerRoutes(source) {
  const routes = [];
  for (const line of source.split(/\r?\n/)) {
    const exact = line.match(/request\.Method == http\.Method(\w+) && path == "([^"]+)"/);
    if (exact) {
      routes.push(`${exact[1].toLowerCase()} ${exact[2]}`);
      continue;
    }

    const dynamic = line.match(
      /request\.Method == http\.Method(\w+) && strings\.HasPrefix\(path, "([^"]+)"\)(?: && strings\.HasSuffix\(path, "([^"]+)"\))?/,
    );
    if (dynamic) {
      const method = dynamic[1].toLowerCase();
      const prefix = dynamic[2].replace(/\/$/, "");
      const suffix = dynamic[3] ?? "";
      routes.push(`${method} ${prefix}/{${routeParamName(prefix)}}${suffix}`);
    }
  }
  return new Set(routes);
}

function parseOpenApiRoutes(source) {
  const routes = [];
  let inPaths = false;
  let currentPath = "";
  for (const line of source.split(/\r?\n/)) {
    if (line === "paths:") {
      inPaths = true;
      continue;
    }
    if (!inPaths) continue;
    if (/^[A-Za-z0-9_-]+:/.test(line)) break;

    const pathMatch = line.match(/^  (\/[^:]+):\s*$/);
    if (pathMatch) {
      currentPath = pathMatch[1];
      continue;
    }
    const methodMatch = line.match(/^    ([a-z]+):\s*$/);
    if (currentPath && methodMatch && methods.has(methodMatch[1])) {
      routes.push(`${methodMatch[1]} ${currentPath}`);
    }
  }
  return new Set(routes);
}

function difference(left, right) {
  return [...left].filter((entry) => !right.has(entry)).sort();
}

const serverRoutes = parseServerRoutes(readFileSync(routeSourcePath, "utf8"));
const openApiRoutes = parseOpenApiRoutes(readFileSync(openApiPath, "utf8"));
const missing = difference(serverRoutes, openApiRoutes);
const extra = difference(openApiRoutes, serverRoutes);

console.log(`OpenAPI routes: ${openApiRoutes.size}`);
console.log(`Server routes: ${serverRoutes.size}`);

if (missing.length > 0) {
  console.error("\nMissing from OpenAPI:");
  for (const route of missing) console.error(`  ${route}`);
}
if (extra.length > 0) {
  console.error("\nNot found in server router:");
  for (const route of extra) console.error(`  ${route}`);
}
if (missing.length > 0 || extra.length > 0) process.exit(1);

console.log("OpenAPI route coverage matches the Go local runtime router.");
