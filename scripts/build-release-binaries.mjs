import { mkdirSync, rmSync } from "node:fs";
import path from "node:path";
import { spawnSync } from "node:child_process";

const repoRoot = path.resolve(import.meta.dirname, "..");
const outputDir = path.join(repoRoot, "dist", "release", "bin");
const platform = process.env.OMEGA_RELEASE_PLATFORM || process.platform;
const arch = process.env.OMEGA_RELEASE_ARCH || process.arch;
const goosByNode = {
  darwin: "darwin",
  linux: "linux",
  win32: "windows"
};
const goarchByNode = {
  arm64: "arm64",
  x64: "amd64"
};

const goos = process.env.GOOS || goosByNode[platform];
const goarch = process.env.GOARCH || goarchByNode[arch];
if (!goos || !goarch) {
  throw new Error(`Unsupported release target platform=${platform} arch=${arch}. Set GOOS/GOARCH explicitly.`);
}

rmSync(outputDir, { recursive: true, force: true });
mkdirSync(outputDir, { recursive: true });

const extension = goos === "windows" ? ".exe" : "";
const binaries = [
  { name: `omega-local-runtime${extension}`, packagePath: "./services/local-runtime/cmd/omega-local-runtime" },
  { name: `omega${extension}`, packagePath: "./services/local-runtime/cmd/omega" }
];

for (const binary of binaries) {
  const outputPath = path.join(outputDir, binary.name);
  const result = spawnSync("go", ["build", "-trimpath", "-ldflags", "-s -w", "-o", outputPath, binary.packagePath], {
    cwd: repoRoot,
    env: {
      ...process.env,
      GOOS: goos,
      GOARCH: goarch
    },
    stdio: "inherit"
  });
  if (result.status !== 0) {
    process.exit(result.status ?? 1);
  }
  console.log(`Built ${path.relative(repoRoot, outputPath)} for ${goos}/${goarch}`);
}
