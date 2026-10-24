const { test } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const client = require("../out/client");

// Exercise the command handlers without starting VS Code or touching a registry.
function harness({ createError, choice, confirmation, forceError, missingSource = false, local = true,
  submissionResult, registryAfterSubmit, submitError, dirtySource = false, saveSuccess = true,
  submitTask, refreshErrorAfterSubmit } = {}) {
  const p = {
    id: 42, code_name: "sample", pretty_name: "Sample", best_score: 0,
    source_path: path.resolve("problems/sample/main.cpp"),
    directory_path: path.resolve("problems/sample"), is_on_local: local,
  };
  const calls = [], opened = [], warnings = [], errors = [], saved = [], focused = [];
  let registry = [p];
  let readError;
  let pending, view, dashboard, messageListener;
  const messages = [], picks = [];
  let resolved = false;
  let recreated = false;
  const commands = new Map();
  const folder = { uri: { scheme: "file", fsPath: path.resolve("problems") } };
  const vscode = {
    ProgressLocation: { Window: 10 },
    FileType: { File: 1 },
    Uri: {
      file: file => ({ scheme: "file", fsPath: file, toString: () => file }),
      joinPath: (uri, ...parts) => vscode.Uri.file(path.join(uri.fsPath, ...parts)),
    },
    commands: {
      registerCommand(name, action) {
        commands.set(name, action);
        return { dispose() {} };
      },
      executeCommand(name, ...args) {
        if (name === "yoel.problems.focus") { focused.push(name); return Promise.resolve(); }
        return pending = commands.get(name)(...args);
      },
    },
    workspace: {
      isTrusted: true,
      workspaceFolders: [folder],
      getConfiguration: () => ({ get: () => "yoel-test" }),
      fs: { stat: async () => {
        if (missingSource && !recreated) { throw new Error("ENOENT"); }
        return { type: 1 };
      } },
      openTextDocument: async uri => {
        if (missingSource && !recreated) { throw new Error("ENOENT"); }
        return { uri, isDirty: dirtySource, save: async () => { saved.push(uri.fsPath); return saveSuccess; } };
      },
    },
    window: {
      registerWebviewViewProvider(id, provider) {
        assert.equal(id, "yoel.problems");
        dashboard = provider;
        view = {
          webview: {
            cspSource: "https://webview.test",
            asWebviewUri: uri => uri,
            postMessage(value) { messages.push(value); return Promise.resolve(true); },
            onDidReceiveMessage(listener) { messageListener = listener; return { dispose() {} }; },
          },
        };
        return { dispose() {} };
      },
      createOutputChannel: () => ({ appendLine() {}, show() {}, dispose() {} }),
      showQuickPick: async items => { picks.push(items); return items[0]; },
      showTextDocument: async document => { opened.push(document.uri.fsPath); },
      showWarningMessage: async (...args) => {
        warnings.push(args);
        return args[1]?.modal ? confirmation : choice;
      },
      showInformationMessage: async () => undefined,
      showErrorMessage: async message => { errors.push(message); },
      withProgress: async (_, action) => action(),
    },
  };
  const fakeClient = {
    ...client,
    runJSON: async (_, args, cwd) => {
      calls.push({ args, cwd });
      if (args[0] === "list") {
        if (readError) { throw readError; }
        return registry;
      }
      if (args[0] === "submit") {
        if (submitTask) { await submitTask; }
        if (submitError) { throw submitError; }
        assert.ok(submissionResult);
        registry = registryAfterSubmit ?? registry;
        if (refreshErrorAfterSubmit) { readError = new Error("refresh failed"); }
        return submissionResult;
      }
      assert.equal(args[0], "new");
      if (args.includes("--force")) {
        if (forceError) { throw forceError; }
        recreated = true;
        const created = { ...p, is_on_local: true, source_path: path.join(createError.directoryPath, "main.cpp") };
        registry = [created];
        return created;
      }
      if (createError) { throw createError; }
      const created = { ...p, is_on_local: true };
      registry = [created];
      return created;
    },
  };
  function load(name) {
    const exports = {};
    vm.runInNewContext(fs.readFileSync(require.resolve(`../out/${name}`), "utf8"), {
      exports, Error,
      require: id => id === "vscode" ? vscode : id === "./client" ? fakeClient :
        id === "./dashboard" ? load("dashboard") : require(id),
    });
    return exports;
  }
  load("extension").activate({ subscriptions: [], extensionUri: vscode.Uri.file(path.resolve(__dirname, "..")) });
  return {
    calls, opened, warnings, errors, p, dashboard, view, vscode, picks, messages, saved, focused,
    run: (name = "yoel.new", ...args) => commands.get(name)(...args),
    setRegistry(value) { registry = value; },
    setRegistryAfterSubmit(value) { registryAfterSubmit = value; },
    setReadError(value) { readError = value; },
    get snapshot() { return messages.at(-1); },
    send: message => messageListener(message),
    async showDashboard() {
      if (!resolved) { dashboard.resolveWebviewView(view); resolved = true; }
      await messageListener({ type: "ready" });
      await pending;
    },
  };
}

test("New Problem delegates to Go even when cached metadata says local", async () => {
  const h = harness();
  await h.run();
  const creation = h.calls.find(call => call.args[0] === "new");
  assert.deepEqual(Array.from(creation.args), ["new", "42", "--json", "--no-pdf"]);
  assert.equal(creation.cwd, path.resolve("problems"));
  assert.deepEqual(h.opened, [h.p.source_path]);
  assert.equal(h.calls.filter(call => call.args[0] === "list").length, 2);
});

test("problem_exists offers Open Existing only after the Go failure", async () => {
  const h = harness({
    createError: new client.YoelCommandError("problem_exists", "backend conflict"),
    choice: "Open Existing",
  });
  await h.run();
  assert.equal(h.warnings.length, 1);
  assert.equal(h.warnings[0][1], "Open Existing");
  assert.deepEqual(h.opened, [h.p.source_path]);
  assert.equal(h.calls.filter(call => call.args[0] === "new").length, 1);
  assert.equal(h.calls.filter(call => call.args[0] === "list").length, 2);
});

test("dismissing an existing-problem warning neither opens nor retries", async () => {
  const h = harness({ createError: new client.YoelCommandError("problem_exists", "conflict") });
  await h.run();
  assert.deepEqual(h.opened, []);
  assert.equal(h.calls.filter(call => call.args[0] === "new").length, 1);
});

test("stale registered source produces an error without recreating", async () => {
  const h = harness({
    createError: new client.YoelCommandError("problem_exists", "conflict"),
    choice: "Open Existing", missingSource: true,
  });
  await h.run();
  assert.match(h.errors[0], /missing or unreadable/);
  assert.deepEqual(h.opened, []);
  assert.equal(h.calls.filter(call => call.args[0] === "new").length, 1);
});

test("unregistered directory conflicts do not guess a source path", async () => {
  const h = harness({
    createError: new client.YoelCommandError("problem_exists", "backend conflict"),
    local: false,
  });
  await h.run();
  assert.match(h.errors[0], /No registered source/);
  assert.deepEqual(h.opened, []);
  assert.equal(h.warnings.length, 0);
});

test("recreation requires confirmation and uses the backend destination, not the old registered path", async () => {
  const destination = path.resolve("different workspace/sample");
  const h = harness({
    createError: new client.YoelCommandError("problem_exists", "conflict", destination),
    choice: "Recreate Problem…", confirmation: "Delete and Recreate",
  });
  await h.run();
  assert.equal(h.warnings.length, 2);
  const dialog = h.warnings[1];
  assert.equal(dialog[1].modal, true);
  assert.ok(dialog[1].detail.includes(destination));
  assert.match(dialog[1].detail, /entire destination directory/);
  const creates = h.calls.filter(call => call.args[0] === "new");
  assert.equal(creates.length, 2);
  assert.deepEqual(Array.from(creates[1].args), ["new", "42", "--json", "--no-pdf", "--force"]);
  assert.equal(creates[1].cwd, creates[0].cwd);
  assert.deepEqual(h.opened, [path.join(destination, "main.cpp")]);
  assert.equal(h.calls.filter(call => call.args[0] === "list").length, 3);
  assert.deepEqual(h.errors, []);
});

test("cancelling the destructive confirmation never invokes force", async () => {
  const h = harness({
    createError: new client.YoelCommandError("problem_exists", "conflict", path.resolve("sample")),
    choice: "Recreate Problem…",
  });
  await h.run();
  assert.equal(h.warnings.length, 2);
  assert.equal(h.calls.filter(call => call.args.includes("--force")).length, 0);
  assert.deepEqual(h.opened, []);
});

test("recreation recovers stale sources and unregistered destination conflicts", async () => {
  for (const options of [{ missingSource: true }, { local: false }]) {
    const destination = path.resolve("sample");
    const h = harness({
      ...options,
      createError: new client.YoelCommandError("problem_exists", "conflict", destination),
      choice: "Recreate Problem…", confirmation: "Delete and Recreate",
    });
    await h.run();
    assert.deepEqual(h.opened, [path.join(destination, "main.cpp")]);
    assert.deepEqual(h.errors, []);
  }
});

test("missing or relative backend destinations never enable recreation", async () => {
  for (const destination of [undefined, "relative/path"]) {
    const h = harness({
      createError: new client.YoelCommandError("problem_exists", "conflict", destination),
      choice: "Recreate Problem…", confirmation: "Delete and Recreate",
    });
    await h.run();
    assert.equal(h.warnings.length, 1);
    assert.equal(h.warnings[0].includes("Recreate Problem…"), false);
    assert.equal(h.calls.filter(call => call.args.includes("--force")).length, 0);
  }
});

test("failed recreation reports the backend error without opening or retrying again", async () => {
  const h = harness({
    createError: new client.YoelCommandError("problem_exists", "conflict", path.resolve("sample")),
    choice: "Recreate Problem…", confirmation: "Delete and Recreate",
    forceError: new client.YoelCommandError("command_failed", "download failed"),
  });
  await h.run();
  assert.match(h.errors[0], /download failed/);
  assert.deepEqual(h.opened, []);
  assert.equal(h.calls.filter(call => call.args.includes("--force")).length, 1);
});

test("expired sessions and legacy text are not mistaken for problem_exists", async () => {
  for (const createError of [
    new client.YoelCommandError("session_expired", "expired"),
    new Error("problem path already exists"),
    new client.YoelCommandError("future_code", "unknown failure"),
  ]) {
    const h = harness({ createError });
    await h.run();
    assert.equal(h.errors.length, 1);
    assert.equal(h.warnings.length, 0);
    assert.deepEqual(h.opened, []);
    if (createError.code === "session_expired") { assert.match(h.errors[0], /yoel login/); }
  }
});

test("sidebar loads on first display and sends strictly local records only", async () => {
  const h = harness();
  assert.equal(h.calls.length, 0);
  h.setRegistry([h.p, { ...h.p, id: 99, code_name: "remote", is_on_local: false }]);
  await h.showDashboard();
  assert.equal(h.snapshot.problems.length, 1);
  assert.equal(h.snapshot.problems[0].code_name, "sample");
  assert.equal(h.snapshot.problems[0].is_on_local, true);
  assert.equal(h.snapshot.problems[0].source_path, undefined);
  assert.equal(h.calls.length, 1);
  await h.showDashboard();
  assert.equal(h.calls.length, 1);
});

test("webview Open resolves the registered local ID without a picker or CLI", async () => {
  const h = harness();
  await h.showDashboard();
  const calls = h.calls.length;
  await h.send({ type: "open", id: h.p.id });
  assert.deepEqual(h.opened, [h.p.source_path]);
  assert.equal(h.calls.length, calls);
});

test("refresh publishes updated details including best-submission time", async () => {
  const h = harness();
  await h.showDashboard();
  const best_submitted_at = "2026-04-01T10:20:30Z";
  h.setRegistry([{ ...h.p, best_score: 75, best_evaluation: "[PPP-]", best_submitted_at }]);
  await h.run("yoel.refresh");
  assert.equal(h.snapshot.problems[0].pretty_name, "Sample");
  assert.equal(h.snapshot.problems[0].best_score, 75);
  assert.equal(h.snapshot.problems[0].best_submitted_at, best_submitted_at);
});

test("empty/error sidebar messages retain refresh recovery", async () => {
  const h = harness();
  h.setRegistry([]);
  await h.showDashboard();
  assert.match(h.snapshot.message, /No local problems/);
  h.setReadError(new Error("registry unavailable"));
  await h.run("yoel.refresh");
  assert.match(h.snapshot.message, /Could not refresh/);
  assert.match(h.errors[0], /registry unavailable/);
  h.setReadError(undefined);
  h.setRegistry([h.p]);
  await h.run("yoel.refresh");
  assert.equal(h.snapshot.message, "");
  assert.equal(h.snapshot.problems.length, 1);
});

test("creating a problem adds it to the local sidebar", async () => {
  const h = harness({ local: false });
  await h.run();
  await h.showDashboard();
  assert.equal(h.snapshot.problems.length, 1);
  assert.equal(h.snapshot.problems[0].code_name, "sample");
});

test("submission refreshes best score/evaluation in the sidebar", async () => {
  const submissionResult = {
    status: 0, problem_name: "sample", points: 100, grader_comment: "[PPPP]", compiler_message: null,
  };
  const h = harness({ submissionResult });
  await h.showDashboard();
  assert.equal(h.snapshot.problems[0].best_score, 0);
  // Keep fixture changes on the Go-response side rather than deriving best-score rules in TypeScript.
  h.setRegistryAfterSubmit([{ ...h.p, best_score: 100, best_evaluation: "[PPPP]" }]);
  h.vscode.window.activeTextEditor = { document: {
    uri: { scheme: "file", fsPath: h.p.source_path }, isDirty: false,
  } };
  await h.run("yoel.submit");
  assert.equal(h.snapshot.problems[0].best_score, 100);
  assert.equal(h.snapshot.problems[0].best_evaluation, "[PPPP]");
  assert.equal(h.snapshot.outcome.type, "result");
  assert.equal(h.snapshot.outcome.result.points, 100);
  assert.equal(h.focused.length, 1);
  const calls = h.calls.length;
  await h.send({ type: "back" });
  assert.equal(h.snapshot.outcome, null);
  assert.equal(h.calls.length, calls);
});

const finalSubmission = {
  status: 0, problem_name: "sample", points: 50, grader_comment: "[PP--]", compiler_message: null,
};

test("row Submit saves and submits the selected registered source, not the active editor", async () => {
  const h = harness({ submissionResult: finalSubmission, dirtySource: true });
  await h.showDashboard();
  h.vscode.window.activeTextEditor = { document: {
    uri: { scheme: "file", fsPath: "/unrelated.cpp" }, isDirty: true,
  } };
  await h.send({ type: "submit", id: 42, source_path: "/forged.cpp" });
  assert.deepEqual(h.saved, [h.p.source_path]);
  assert.deepEqual(Array.from(h.calls.find(call => call.args[0] === "submit").args),
    ["submit", "--json", "--source", h.p.source_path]);
  assert.equal(h.snapshot.outcome.result.points, 50);
  assert.equal(h.focused.length, 1);
});

test("compilation and grader errors are final grading results, not process failures", async () => {
  for (const status of [0, 1, 2]) {
    const result = { ...finalSubmission, status, points: status === 0 ? 0 : null,
      compiler_message: status === 1 ? "syntax error" : null,
      grader_comment: status === 2 ? "grader unavailable" : "[----]" };
    const h = harness({ submissionResult: result });
    await h.showDashboard();
    await h.send({ type: "submit", id: 42 });
    assert.equal(h.snapshot.outcome.type, "result");
    assert.equal(h.snapshot.outcome.result.status, status);
    assert.equal(h.errors.length, 0);
  }
});

test("submission failures show a failure screen and can return without a fake grader result", async () => {
  const h = harness({ submitError: new client.YoelCommandError("session_expired", "session expired") });
  await h.showDashboard();
  await h.send({ type: "submit", id: 42 });
  assert.equal(h.snapshot.outcome.type, "error");
  assert.match(h.snapshot.outcome.message, /session expired/);
  assert.equal(h.snapshot.outcome.result, undefined);
  await h.send({ type: "back" });
  assert.equal(h.snapshot.outcome, null);
  assert.equal(h.snapshot.message.includes("Submitting"), false);
});

test("missing sources and refused saves never submit", async () => {
  for (const options of [{ missingSource: true }, { dirtySource: true, saveSuccess: false }]) {
    const h = harness({ ...options, submissionResult: finalSubmission });
    await h.showDashboard();
    await h.send({ type: "submit", id: 42 });
    assert.equal(h.calls.some(call => call.args[0] === "submit"), false);
    assert.equal(h.snapshot.outcome.type, "error");
  }
});

test("no result is published while grading is pending and duplicate submits are blocked", async () => {
  let finish;
  const submitTask = new Promise(resolve => { finish = resolve; });
  const h = harness({ submissionResult: finalSubmission, submitTask });
  await h.showDashboard();
  const ongoing = h.send({ type: "submit", id: 42 });
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(h.snapshot.busy, true);
  assert.equal(h.snapshot.outcome, null);
  assert.match(h.snapshot.message, /waiting for the final grading/);
  await h.send({ type: "submit", id: 42 });
  assert.equal(h.calls.filter(call => call.args[0] === "submit").length, 1);
  finish();
  await ongoing;
  assert.equal(h.snapshot.busy, false);
  assert.equal(h.snapshot.outcome.type, "result");
});

test("completed result survives refresh failure and webview reload until Back is selected", async () => {
  const h = harness({ submissionResult: finalSubmission, refreshErrorAfterSubmit: true });
  await h.showDashboard();
  await h.send({ type: "submit", id: 42 });
  assert.equal(h.snapshot.outcome.type, "result");
  assert.equal(h.warnings.length, 1);
  await h.showDashboard();
  assert.equal(h.snapshot.outcome.result.points, 50);
  await h.send({ type: "back" });
  await h.showDashboard();
  assert.equal(h.snapshot.outcome, null);
});

test("webview rejects forged commands and non-local problem IDs", async () => {
  const h = harness();
  h.setRegistry([h.p, { ...h.p, id: 99, is_on_local: false }]);
  await h.showDashboard();
  const count = h.calls.length;
  for (const value of [null, "open", { type: "submit" }, { type: "submit", id: 99 }, { type: "open", id: 99 },
    { type: "open", id: "42" }, { type: "open", id: 42, source_path: "/untrusted" }]) {
    await h.send(value);
  }
  assert.equal(h.calls.length, count);
  assert.deepEqual(h.opened, [h.p.source_path]);
});

test("Open Problem's picker contains only local records", async () => {
  const h = harness();
  h.setRegistry([{ ...h.p, id: 99, is_on_local: false }, h.p]);
  await h.run("yoel.open");
  assert.equal(h.picks[0].length, 1);
  assert.equal(h.picks[0][0].problem.id, h.p.id);
});

test("webview has a persistent search input, restricted CSP, and no catalog data in HTML", async () => {
  const h = harness();
  await h.showDashboard();
  assert.match(h.view.webview.html, /type="search"/);
  assert.match(h.view.webview.html, /default-src 'none'/);
  assert.match(h.view.webview.html, /script-src 'nonce-[a-f0-9]+'/);
  assert.equal(h.view.webview.html.includes("sample"), false);
});

test("manifest declares a webview sidebar and removes the search icon action", () => {
  const { contributes } = require("../package.json");
  assert.equal(contributes.views.yoel[0].id, "yoel.problems");
  assert.equal(contributes.views.yoel[0].type, "webview");
  assert.equal(contributes.viewsContainers.activitybar[0].id, "yoel");
  assert.ok(fs.existsSync(path.resolve(__dirname, "..", contributes.viewsContainers.activitybar[0].icon)));
  assert.deepEqual(contributes.menus["view/title"].map(item => item.command), ["yoel.refresh"]);
});
