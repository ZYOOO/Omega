import { mkdir, readFile, writeFile } from "fs/promises";
import { join } from "path";
import { spawn } from "child_process";
import { tmpdir } from "os";

function runCodex(workspace, prompt) {
  return new Promise((resolve) => {
    const child = spawn(
      "codex",
      [
        "--ask-for-approval",
        "never",
        "exec",
        "--skip-git-repo-check",
        "--sandbox",
        "workspace-write",
        "--model",
        "gpt-5.4-mini",
        "-C",
        workspace,
        prompt
      ],
      { shell: false }
    );

    let stdout = "";
    let stderr = "";

    child.stdout.on("data", (chunk) => {
      stdout += chunk.toString();
    });

    child.stderr.on("data", (chunk) => {
      stderr += chunk.toString();
    });

    child.on("close", (exitCode) => {
      resolve({ exitCode, stdout, stderr });
    });
  });
}

async function main() {
  const workspace = process.argv[2] ?? join(tmpdir(), `omega-codex-smoke-${Date.now()}`);
  const proofPath = join(workspace, ".omega", "proof");
  const proofFile = join(proofPath, "codex-smoke.txt");

  await mkdir(proofPath, { recursive: true });
  await writeFile(
    join(workspace, ".omega", "prompt.md"),
    "Create .omega/proof/codex-smoke.txt containing exactly OMEGA_OK."
  );

  const prompt = [
    "You are running in a temporary Omega Mission Control workspace.",
    "Only create the file .omega/proof/codex-smoke.txt containing exactly OMEGA_OK.",
    "Do not delete files.",
    "Do not install packages.",
    "Do not modify anything else.",
    "Reply DONE when finished."
  ].join(" ");

  const result = await runCodex(workspace, prompt);
  const proof = await readFile(proofFile, "utf8").catch(() => "");

  if (result.exitCode !== 0 || proof.trim() !== "OMEGA_OK") {
    console.error(result.stderr);
    console.error(`proof=${proof}`);
    process.exit(result.exitCode ?? 1);
  }

  console.log("Codex proof smoke passed");
  console.log(`workspace: ${workspace}`);
  console.log(`proof: ${proofFile}`);
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
