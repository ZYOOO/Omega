import { spawn } from "child_process";
import { mkdir, readdir, readFile, stat, writeFile } from "fs/promises";
import { join } from "path";

export interface LocalRunnerCommand {
  executable: string;
  args: string[];
  env?: Record<string, string>;
  stdinFile?: string;
}

export interface LocalMissionJobInput {
  runId: string;
  issueKey: string;
  stageId: string;
  agentId: string;
  workspaceRoot: string;
  prompt: string;
  command: LocalRunnerCommand;
  timeoutMs?: number;
}

export interface LocalMissionJobResult {
  status: "passed" | "failed";
  workspacePath: string;
  exitCode: number | null;
  stdout: string;
  stderr: string;
  proofFiles: string[];
  jobSpecPath: string;
}

interface JobSpec {
  runId: string;
  issueKey: string;
  stageId: string;
  agentId: string;
  prompt: string;
  createdAt: string;
}

function safeSegment(input: string): string {
  return input.replace(/[^a-zA-Z0-9._-]+/g, "-");
}

async function collectFiles(directory: string): Promise<string[]> {
  const entries = await readdir(directory, { withFileTypes: true });
  const files = await Promise.all(
    entries.map(async (entry) => {
      const fullPath = join(directory, entry.name);
      if (entry.isDirectory()) {
        return collectFiles(fullPath);
      }
      return [fullPath];
    })
  );

  return files.flat();
}

async function directoryExists(path: string): Promise<boolean> {
  try {
    return (await stat(path)).isDirectory();
  } catch {
    return false;
  }
}

function runCommand(
  command: LocalRunnerCommand,
  cwd: string,
  timeoutMs: number
): Promise<{ exitCode: number | null; stdout: string; stderr: string }> {
  return new Promise((resolve) => {
    const child = spawn(command.executable, command.args, {
      cwd,
      env: { ...process.env, ...command.env },
      shell: false
    });

    let stdout = "";
    let stderr = "";
    let settled = false;

    const timer = setTimeout(() => {
      if (settled) {
        return;
      }
      child.kill("SIGTERM");
    }, timeoutMs);

    child.stdout.on("data", (chunk: Buffer) => {
      stdout += chunk.toString();
    });

    child.stderr.on("data", (chunk: Buffer) => {
      stderr += chunk.toString();
    });

    if (command.stdinFile) {
      readFile(join(cwd, command.stdinFile), "utf8")
        .then((content) => {
          child.stdin.write(content);
          child.stdin.end();
        })
        .catch((error) => {
          stderr += error.message;
          child.stdin.end();
        });
    } else {
      child.stdin.end();
    }

    child.on("close", (exitCode) => {
      settled = true;
      clearTimeout(timer);
      resolve({ exitCode, stdout, stderr });
    });

    child.on("error", (error) => {
      settled = true;
      clearTimeout(timer);
      resolve({ exitCode: null, stdout, stderr: `${stderr}${error.message}` });
    });
  });
}

export async function runLocalMissionJob(
  input: LocalMissionJobInput
): Promise<LocalMissionJobResult> {
  const workspacePath = join(
    input.workspaceRoot,
    `${safeSegment(input.issueKey)}-${safeSegment(input.stageId)}`
  );
  const omegaPath = join(workspacePath, ".omega");
  const proofPath = join(omegaPath, "proof");
  const jobSpecPath = join(omegaPath, "job.json");

  await mkdir(proofPath, { recursive: true });

  const jobSpec: JobSpec = {
    runId: input.runId,
    issueKey: input.issueKey,
    stageId: input.stageId,
    agentId: input.agentId,
    prompt: input.prompt,
    createdAt: new Date().toISOString()
  };

  await writeFile(jobSpecPath, JSON.stringify(jobSpec, null, 2));
  await writeFile(join(omegaPath, "prompt.md"), input.prompt);

  const commandResult = await runCommand(
    input.command,
    workspacePath,
    input.timeoutMs ?? 30_000
  );

  const proofFiles = await directoryExists(proofPath) ? await collectFiles(proofPath) : [];

  return {
    status: commandResult.exitCode === 0 ? "passed" : "failed",
    workspacePath,
    exitCode: commandResult.exitCode,
    stdout: commandResult.stdout,
    stderr: commandResult.stderr,
    proofFiles,
    jobSpecPath
  };
}
