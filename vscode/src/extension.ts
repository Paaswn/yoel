import * as vscode from "vscode";
import * as path from "node:path";
import { Problem, YoelCommandError, problem, problems, resultMessage, runJSON, submission } from "./client";
import { ProblemDashboard } from "./dashboard";

export function activate(context: vscode.ExtensionContext): void {
  const output = vscode.window.createOutputChannel("Yoel");
  const dashboard = new ProblemDashboard(context.extensionUri, async action => {
    if (action.type === "open" || action.type === "submit") {
      await vscode.commands.executeCommand(action.type === "open" ? "yoel.open" : "yoel.submit", action.id);
    } else {
      await vscode.commands.executeCommand(action.type === "new" ? "yoel.new" : "yoel.refresh");
    }
  });
  context.subscriptions.push(output, dashboard,
    vscode.window.registerWebviewViewProvider("yoel.problems", dashboard));
  let snapshot: Problem[] = [];
  let busy = false;
  const run = (args: string[], cwd?: string) =>
    runJSON(vscode.workspace.getConfiguration("yoel").get<string>("executablePath", "yoel"), args, cwd);

  async function refresh(): Promise<void> {
    dashboard.setMessage("Loading problems…");
    try {
      snapshot = problems(await run(["list", "--json"]));
      dashboard.replace(snapshot);
      dashboard.setMessage(dashboard.count ? "" : "No local problems. Use New Problem to create one.");
    } catch (error) {
      dashboard.setMessage("Could not refresh problems. Check the Yoel output, then use Refresh to retry.");
      throw error;
    }
  }

  async function pick(localOnly = false): Promise<Problem | undefined> {
    // Refresh once per picker, never on a keystroke.
    await refresh();
    const choices = snapshot.filter(p => !localOnly || p.is_on_local === true);
    if (!choices.length) {
      void vscode.window.showInformationMessage(localOnly ? "No local problems. Use Yoel: New Problem." :
        "No Yoel problems. Run 'yoel fetch' in a terminal, then refresh.");
      return;
    }
    const selected = await vscode.window.showQuickPick(choices.map(p => ({
      label: p.code_name,
      description: `${p.id} · ${p.is_on_local ? "local" : "not local"} · ${p.best_score} points`,
      detail: p.pretty_name,
      problem: p,
    })), { matchOnDescription: true, matchOnDetail: true,
      placeHolder: localOnly ? "Search local problems to open" : "Search the catalog to create a problem" });
    return selected?.problem;
  }

  async function open(p: Problem): Promise<void> {
    if (!p.is_on_local || !p.source_path) {
      throw new Error(`${p.code_name} has no local source. Use Yoel: New Problem.`);
    }
    if (!path.isAbsolute(p.source_path)) { throw new Error("Yoel returned a non-absolute source path."); }
    const uri = vscode.Uri.file(p.source_path);
    try {
      const stat = await vscode.workspace.fs.stat(uri);
      if (!(stat.type & vscode.FileType.File)) { throw new Error("Not a file"); }
    } catch {
      throw new Error(`Registered source is missing or unreadable: ${p.source_path}. Restore it or recreate the problem with Yoel.`);
    }
    await vscode.window.showTextDocument(await vscode.workspace.openTextDocument(uri));
  }

  async function refreshAfterOperation(): Promise<void> {
    snapshot = [];
    dashboard.replace([]);
    try { await refresh(); } catch (error) {
      void vscode.window.showWarningMessage(`Operation completed, but registry refresh failed: ${String(error)}`);
    }
  }

  async function revealDashboard(): Promise<void> {
    try {
      await vscode.commands.executeCommand("yoel.problems.focus");
    } catch (error) {
      // A UI focus failure must not turn a completed submission into a failure.
      output.appendLine(`Could not focus the Yoel view: ${String(error)}`);
    }
  }

  const commands: Record<string, (id?: number) => Promise<void>> = {
    "yoel.refresh": async () => {
      await refresh();
      void vscode.window.showInformationMessage(`Loaded ${dashboard.count} local Yoel problems.`);
    },
    "yoel.open": async (id) => {
      const p = id === undefined ? await pick(true) : dashboard.getProblem(id);
      if (id !== undefined && !p) { throw new Error("This problem is no longer registered as local. Refresh the dashboard."); }
      if (p) { await open(p); }
    },
    "yoel.new": async () => {
      const p = await pick();
      if (!p) { return; }
      const folders = vscode.workspace.workspaceFolders;
      const folder = folders?.length === 1 ? folders[0] : await vscode.window.showWorkspaceFolderPick();
      if (!folder) {
        void vscode.window.showInformationMessage("Open a local workspace folder to create a problem.");
        return;
      }
      if (folder.uri.scheme !== "file") { throw new Error("Choose a local filesystem workspace."); }
      let created: Problem;
      let recreated = false;
      try {
        created = problem(await run(["new", String(p.id), "--json", "--no-pdf"], folder.uri.fsPath));
      } catch (error) {
        if (!(error instanceof YoelCommandError) || error.code !== "problem_exists") { throw error; }
        output.appendLine(`[${error.code}] ${error.message}`);
        await refresh();
        const existing = snapshot.find(candidate => candidate.id === p.id);
        const canOpen = existing?.is_on_local && existing.source_path;
        const canRecreate = error.directoryPath && path.isAbsolute(error.directoryPath);
        const actions = [
          ...(canOpen ? ["Open Existing"] : []),
          ...(canRecreate ? ["Recreate Problem…"] : []),
        ];
        if (!actions.length) {
          throw new Error(`${error.message}\nNo registered source is available to open. Rebuild Yoel to enable recreation with a confirmed destination, or resolve the conflict in a terminal.`);
        }
        const choice = await vscode.window.showWarningMessage(
          `${p.code_name} already exists. No files were overwritten.`,
          ...actions,
        );
        if (choice === "Open Existing" && canOpen) {
          await open(existing);
          return;
        }
        if (choice !== "Recreate Problem…" || !canRecreate) { return; }
        const confirmed = await vscode.window.showWarningMessage(
          `Recreate ${p.code_name}?`,
          {
            modal: true,
            detail: `Yoel will delete and replace the entire destination directory, including existing solutions and other files:\n\n${error.directoryPath}\n\n` +
              `The registry will point to the recreated source in this destination. This is not a source-only repair. Back up any work you want to keep before continuing.`,
          },
          "Delete and Recreate",
        );
        if (confirmed !== "Delete and Recreate") { return; }
        created = problem(await run(["new", String(p.id), "--json", "--no-pdf", "--force"], folder.uri.fsPath));
        recreated = true;
      }
      await refreshAfterOperation();
      await open(created);
      void vscode.window.showInformationMessage(`${recreated ? "Recreated" : "Created"} ${created.code_name}.`);
    },
    "yoel.submit": async (id) => {
      try {
        let document = vscode.window.activeTextEditor?.document;
        if (id !== undefined) {
          const p = dashboard.getProblem(id);
          if (!p?.source_path || !path.isAbsolute(p.source_path)) {
            throw new Error("This problem has no registered local source. Refresh the dashboard.");
          }
          // Load the selected source, not whichever editor happens to be active.
          document = await vscode.workspace.openTextDocument(vscode.Uri.file(p.source_path));
        }
        if (!document || document.uri.scheme !== "file") {
          throw new Error("Open the registered Yoel source file before submitting.");
        }
        if (document.isDirty && !await document.save()) { throw new Error("Save the source before submitting."); }
        dashboard.beginSubmission();
        const result = submission(await run(["submit", "--json", "--source", document.uri.fsPath]));
        output.appendLine(resultMessage(result));
        output.appendLine(JSON.stringify(result, null, 2));
        dashboard.showResult(result);
        await revealDashboard();
        await refreshAfterOperation();
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        dashboard.showSubmissionError(message);
        await revealDashboard();
        throw error;
      }
    },
  };
  for (const [name, action] of Object.entries(commands)) {
    context.subscriptions.push(vscode.commands.registerCommand(name, async (id?: number) => {
      if (!vscode.workspace.isTrusted) { return; }
      if (busy) { void vscode.window.showInformationMessage("Another Yoel operation is running."); return; }
      busy = true;
      dashboard.setBusy(true);
      try {
        await vscode.window.withProgress({ location: vscode.ProgressLocation.Window, title: "Yoel" }, () => action(id));
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        output.appendLine(error instanceof YoelCommandError ? `[${error.code}] ${message}` : message);
        if (error instanceof YoelCommandError && error.code === "session_expired") {
          void vscode.window.showErrorMessage(`${message}\nRun 'yoel login' in a terminal, then retry.`);
        } else {
          void vscode.window.showErrorMessage(message);
        }
      } finally {
        busy = false;
        dashboard.setBusy(false);
      }
    }));
  }
}
