import { spawn } from "child_process";
import { mkdir, readdir, writeFile } from "fs/promises";
import { join, resolve } from "path";
import { tmpdir } from "os";

function safeSegment(input) {
  return input.replace(/[^a-zA-Z0-9._-]+/g, "-");
}

async function collectFiles(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  const nested = await Promise.all(
    entries.map(async (entry) => {
      const fullPath = join(directory, entry.name);
      if (entry.isDirectory()) {
        return collectFiles(fullPath);
      }
      return [fullPath];
    })
  );

  return nested.flat();
}

function runCommand(command, cwd) {
  return new Promise((resolveRun) => {
    const child = spawn(command.executable, command.args, {
      cwd,
      env: process.env,
      shell: false
    });

    let stdout = "";
    let stderr = "";

    child.stdout.on("data", (chunk) => {
      stdout += chunk.toString();
    });

    child.stderr.on("data", (chunk) => {
      stderr += chunk.toString();
    });

    child.on("close", (exitCode) => {
      resolveRun({ exitCode, stdout, stderr });
    });
  });
}

async function main() {
  const workspaceRoot = resolve(process.argv[2] ?? join(tmpdir(), "omega-local-runner"));
  const issueKey = "OMG-LOCAL";
  const stageId = "testing";
  const workspacePath = join(workspaceRoot, `${safeSegment(issueKey)}-${safeSegment(stageId)}`);
  const omegaPath = join(workspacePath, ".omega");
  const proofPath = join(omegaPath, "proof");

  await mkdir(proofPath, { recursive: true });

  await writeFile(
    join(omegaPath, "job.json"),
    JSON.stringify(
      {
        runId: "run_local_demo",
        issueKey,
        stageId,
        agentId: "testing",
        prompt: "Run a local validation job and attach proof.",
        createdAt: new Date().toISOString()
      },
      null,
      2
    )
  );
  await writeFile(join(omegaPath, "prompt.md"), "Run a local validation job and attach proof.");

  const command = {
    executable: process.execPath,
    args: [
      "-e",
      "const fs=require('fs'); fs.writeFileSync('.omega/proof/local-check.txt','local runner proof\\nstatus: passed'); console.log('local proof written')"
    ]
  };

  const result = await runCommand(command, workspacePath);
  const proofFiles = await collectFiles(proofPath);

  if (result.exitCode !== 0) {
    console.error(result.stderr);
    process.exit(result.exitCode ?? 1);
  }

  console.log("Local runner demo passed");
  console.log(`workspace: ${workspacePath}`);
  console.log(`stdout: ${result.stdout.trim()}`);
  console.log(`proof: ${proofFiles.join(", ")}`);
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
