import * as vscode from "vscode";
import { randomBytes } from "node:crypto";
import { Problem, Submission } from "./client";

export type DashboardAction = { type: "new" | "refresh" } | { type: "open" | "submit"; id: number };
type SubmissionOutcome = { type: "result"; result: Submission } | { type: "error"; message: string };

// The webview receives only local metadata, never source paths or CLI arguments.
export class ProblemDashboard implements vscode.WebviewViewProvider {
  private snapshot: Problem[] = [];
  private message = "";
  private loaded = false;
  private busy = false;
  private outcome?: SubmissionOutcome;
  private view?: vscode.WebviewView;
  private subscriptions: vscode.Disposable[] = [];

  constructor(
    private readonly extensionUri: vscode.Uri,
    private readonly onAction: (action: DashboardAction) => Promise<void>,
  ) {}

  replace(problems: Problem[]): void {
    this.snapshot = problems.filter(p => p.is_on_local === true);
    this.loaded = true;
    this.publish();
  }

  setMessage(message = ""): void {
    this.message = message;
    this.publish();
  }

  setBusy(busy: boolean): void {
    this.busy = busy;
    this.publish();
  }

  beginSubmission(): void {
    this.outcome = undefined;
    this.setMessage("Submitting source and waiting for the final grading result…");
  }

  showResult(result: Submission): void {
    this.outcome = { type: "result", result };
    this.setMessage();
  }

  showSubmissionError(message: string): void {
    this.outcome = { type: "error", message };
    this.setMessage();
  }

  get count(): number { return this.snapshot.length; }

  getProblem(id: number): Problem | undefined {
    return this.snapshot.find(p => p.id === id && p.is_on_local === true);
  }

  resolveWebviewView(view: vscode.WebviewView): void {
    this.dispose();
    this.view = view;
    const resources = vscode.Uri.joinPath(this.extensionUri, "resources");
    view.webview.options = { enableScripts: true, localResourceRoots: [resources] };
    const nonce = randomBytes(16).toString("hex");
    const script = view.webview.asWebviewUri(vscode.Uri.joinPath(resources, "dashboard.js"));
    const style = view.webview.asWebviewUri(vscode.Uri.joinPath(resources, "dashboard.css"));
    view.webview.html = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src ${view.webview.cspSource}; script-src 'nonce-${nonce}';">
  <link rel="stylesheet" href="${style}">
  <title>Yoel local problems</title>
</head>
<body>
  <header id="dashboard-header">
    <label for="search">Search local problems</label>
    <input id="search" type="search" placeholder="Code, pretty name, or ID" autocomplete="off">
    <div class="actions">
      <button id="new" type="button">New Problem…</button>
      <button id="refresh" class="secondary" type="button">Refresh</button>
    </div>
  </header>
  <p id="status" role="status" aria-live="polite"></p>
  <main id="problems" aria-label="Local problems"></main>
  <section id="submission" hidden aria-label="Submission result"></section>
  <script nonce="${nonce}" src="${script}"></script>
</body>
</html>`;
    this.subscriptions.push(view.webview.onDidReceiveMessage(async (value: unknown) => {
      if (!value || typeof value !== "object") { return; }
      const action = value as Record<string, unknown>;
      try {
        if (action.type === "ready") {
          this.publish();
          if (!this.loaded && !this.busy) { await this.onAction({ type: "refresh" }); }
        } else if (action.type === "new" || action.type === "refresh") {
          await this.onAction({ type: action.type });
        } else if (action.type === "back") {
          this.outcome = undefined;
          this.publish();
          if (!this.loaded && !this.busy) { await this.onAction({ type: "refresh" }); }
        } else if ((action.type === "open" || action.type === "submit") && typeof action.id === "number" &&
          Number.isInteger(action.id) && this.getProblem(action.id)) {
          await this.onAction({ type: action.type, id: action.id });
        }
      } catch {
        this.setMessage("Could not complete the action. Check the Yoel output and retry.");
      }
    }));
  }

  private publish(): void {
    void this.view?.webview.postMessage({
      type: "snapshot",
      message: this.message,
      busy: this.busy,
      outcome: this.outcome ?? null,
      problems: this.snapshot.map(p => ({
        id: p.id, code_name: p.code_name, pretty_name: p.pretty_name,
        is_on_local: true, best_score: p.best_score,
        best_evaluation: p.best_evaluation, best_submitted_at: p.best_submitted_at,
      })),
    });
  }

  dispose(): void {
    this.subscriptions.forEach(subscription => subscription.dispose());
    this.subscriptions = [];
    this.view = undefined;
  }
}
