import { execFile } from "node:child_process";

export interface Problem {
  id: number;
  code_name: string;
  pretty_name: string;
  source_path: string;
  directory_path: string;
  is_on_local: boolean;
  best_score: number;
  best_evaluation?: string | null;
  best_submitted_at?: string | null;
}

export interface Submission {
  status: 0 | 1 | 2;
  problem_name: string;
  points: number | null;
  grader_comment: string | null;
  compiler_message: string | null;
  id?: number;
  number?: number;
  submitted_at?: string;
  language?: string;
  max_runtime?: number | null;
  peak_memory?: number | null;
}

export class YoelCommandError extends Error {
  constructor(public readonly code: string, message: string, public readonly directoryPath?: string) {
    super(message);
    this.name = "YoelCommandError";
  }
}

// Unknown codes are intentionally preserved for forward compatibility.
export function commandError(stderr: string): YoelCommandError | undefined {
  try {
    const payload = JSON.parse(stderr);
    const error = payload?.error;
    if (typeof error?.code === "string" && typeof error?.message === "string") {
      const directoryPath = error.details?.directory_path;
      return new YoelCommandError(error.code, error.message,
        typeof directoryPath === "string" ? directoryPath : undefined);
    }
  } catch {
    // Older binaries and OS-level failures may still use plain text.
  }
  return undefined;
}

export function runJSON(executable: string, args: string[], cwd?: string): Promise<unknown> {
  return new Promise((resolve, reject) => {
    const child = execFile(executable, args, {
      cwd, shell: false, timeout: 120_000, maxBuffer: 8 * 1024 * 1024,
      encoding: "utf8", windowsHide: true,
    }, (error, stdout, stderr) => {
      if (error) {
        if (error.code === "ENOENT") {
          reject(new Error("Yoel executable was not found. Check yoel.executablePath in Settings."));
        } else if (error.killed) {
          reject(new Error("Yoel was stopped or exceeded its execution/output limit. A submission may already have been accepted; check the grader before retrying."));
        } else {
          reject(commandError(stderr) ?? new Error(`Yoel failed: ${stderr.trim() || error.message}`));
        }
        return;
      }
      try {
        resolve(JSON.parse(stdout));
      } catch {
        reject(new Error("Yoel returned invalid JSON. Install a compatible Yoel build (see the extension README)."));
      }
    });
    // Noninteractive commands must never wait for terminal input.
    child.stdin?.end();
  });
}

function record(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new Error("Incompatible Yoel JSON response.");
  }
  return value as Record<string, unknown>;
}

export function problem(value: unknown): Problem {
  const p = record(value);
  if (!Number.isInteger(p.id) ||
      !["code_name", "pretty_name", "source_path", "directory_path"].every(k => typeof p[k] === "string") ||
      typeof p.is_on_local !== "boolean" || typeof p.best_score !== "number" ||
      !(p.best_evaluation == null || typeof p.best_evaluation === "string") ||
      !(p.best_submitted_at == null || (typeof p.best_submitted_at === "string" &&
        Number.isFinite(Date.parse(p.best_submitted_at))))) {
    throw new Error("Incompatible Yoel problem metadata.");
  }
  return p as unknown as Problem;
}

export function problems(value: unknown): Problem[] {
  if (!Array.isArray(value)) { throw new Error("Expected a Yoel problem list."); }
  return value.map(problem);
}

export function submission(value: unknown): Submission {
  const s = record(value);
  if (![0, 1, 2].includes(s.status as number) || typeof s.problem_name !== "string" ||
      !(s.points === null || typeof s.points === "number") ||
      !["grader_comment", "compiler_message"].every(k => s[k] === null || typeof s[k] === "string") ||
      !["id", "number"].every(k => s[k] === undefined || Number.isInteger(s[k])) ||
      !["max_runtime", "peak_memory"].every(k => s[k] == null || typeof s[k] === "number") ||
      !(s.language === undefined || typeof s.language === "string") ||
      !(s.submitted_at === undefined || (typeof s.submitted_at === "string" &&
        Number.isFinite(Date.parse(s.submitted_at))))) {
    throw new Error("Incompatible Yoel grading result.");
  }
  return s as unknown as Submission;
}

export function resultMessage(s: Submission): string {
  if (s.status === 1) { return `${s.problem_name}: compilation error`; }
  if (s.status === 2) { return `${s.problem_name}: grader error`; }
  return `${s.problem_name}: grading complete — score ${s.points ?? "unavailable"}`;
}
