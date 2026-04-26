declare const process: {
  env: Record<string, string | undefined>;
  execPath: string;
  cwd(): string;
};

type Buffer = {
  toString(): string;
};

declare module "child_process" {
  export interface SpawnOptions {
    cwd?: string;
    env?: Record<string, string | undefined>;
    shell?: boolean;
  }

  export interface ChildProcess {
    stdout: { on(event: "data", listener: (chunk: Buffer) => void): void };
    stderr: { on(event: "data", listener: (chunk: Buffer) => void): void };
    stdin: { write(chunk: string): void; end(): void };
    on(event: "close", listener: (exitCode: number | null) => void): void;
    on(event: "error", listener: (error: Error) => void): void;
    kill(signal?: string): void;
  }

  export function spawn(command: string, args: string[], options?: SpawnOptions): ChildProcess;
}

declare module "fs/promises" {
  export interface Dirent {
    name: string;
    isDirectory(): boolean;
  }

  export interface Stat {
    isDirectory(): boolean;
  }

  export function mkdir(path: string, options?: { recursive?: boolean }): Promise<void>;
  export function readdir(path: string, options: { withFileTypes: true }): Promise<Dirent[]>;
  export function stat(path: string): Promise<Stat>;
  export function writeFile(path: string, data: string): Promise<void>;
  export function readFile(path: string, encoding: "utf8"): Promise<string>;
  export function unlink(path: string): Promise<void>;
  export function rm(path: string, options?: { recursive?: boolean; force?: boolean }): Promise<void>;
  export function mkdtemp(prefix: string): Promise<string>;
}

declare module "path" {
  export function join(...paths: string[]): string;
  export function dirname(path: string): string;
  export function resolve(...paths: string[]): string;
}

declare module "os" {
  export function tmpdir(): string;
}

declare module "http" {
  export interface IncomingMessage {
    method?: string;
    url?: string;
    on(event: "data", listener: (chunk: unknown) => void): void;
    on(event: "end", listener: () => void): void;
  }

  export interface ServerResponse {
    statusCode: number;
    setHeader(name: string, value: string): void;
    end(body: string): void;
  }

  export interface AddressInfo {
    port: number;
  }

  export interface Server {
    listen(port: number, host: string, listener: () => void): void;
    address(): AddressInfo | string | null;
    close(listener: () => void): void;
  }

  export function createServer(
    listener: (request: IncomingMessage, response: ServerResponse) => void
  ): Server;
}
