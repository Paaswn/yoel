const { test } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const { JSDOM } = require("jsdom");
const { filterLocalProblems, formatSubmittedAt } = require("../resources/dashboard");

function ui(t, saved) {
  const dom = new JSDOM('<header id="dashboard-header"><input id="search"><button id="new"></button><button id="refresh"></button></header><p id="status"></p><main id="problems"></main><section id="submission" hidden></section>',
    { runScripts: "outside-only" });
  t.after(() => dom.window.close());
  const messages = [];
  let state = saved;
  dom.window.acquireVsCodeApi = () => ({
    getState: () => state,
    setState: value => { state = value; },
    postMessage: value => { messages.push(JSON.parse(JSON.stringify(value))); },
  });
  dom.window.eval(fs.readFileSync(require.resolve("../resources/dashboard"), "utf8"));
  return {
    document: dom.window.document, messages,
    send(problems, extra = {}) {
      dom.window.dispatchEvent(new dom.window.MessageEvent("message", {
        data: { type: "snapshot", message: "", problems, ...extra },
      }));
    },
    search(value) {
      const input = dom.window.document.getElementById("search");
      input.value = value;
      input.dispatchEvent(new dom.window.Event("input"));
    },
    get state() { return state; },
    rows: () => Array.from(dom.window.document.querySelectorAll("details")),
  };
}

const problem = {
  id: 42, code_name: "sample", pretty_name: "Array Problem", best_score: 80, is_on_local: true,
};

test("local filter requires literal true and searches code, pretty name, and ID", () => {
  const records = [problem, ...[false, undefined, "true", 1].map((is_on_local, index) => ({
    ...problem, id: 100 + index, is_on_local,
  }))];
  for (const query of ["", "SAMPLE", "array", "42"]) {
    assert.deepEqual(filterLocalProblems(records, query), [problem]);
  }
  assert.deepEqual(filterLocalProblems(records, "missing"), []);
});

test("expanded details include pretty name, score, matching submission date/time, and optional evaluation", t => {
  const view = ui(t);
  view.send([{ ...problem, best_submitted_at: "2026-04-01T10:20:30Z", best_evaluation: "[PPP-]" }]);
  const [row] = view.rows();
  assert.equal(row.open, false);
  assert.equal(row.querySelector("summary").textContent, "sample");
  assert.deepEqual(Array.from(row.querySelectorAll("dt"), node => node.textContent),
    ["Pretty Name", "Best Score", "Best Score Submitted", "Evaluation"]);
  const date = row.querySelector("time");
  assert.equal(date.dateTime, "2026-04-01T10:20:30Z");
  assert.equal(date.textContent, formatSubmittedAt(date.dateTime));
  assert.match(date.textContent, /2026/);
  assert.match(date.textContent, /\d{1,2}:\d{2}/);
  row.querySelector("button").click();
  assert.deepEqual(view.messages.at(-1), { type: "open", id: 42 });
});

test("unknown dates and evaluations are omitted, including empty values", t => {
  const view = ui(t);
  for (const best_evaluation of [undefined, null, "", "   "]) {
    view.send([{ ...problem, best_evaluation }]);
    assert.deepEqual(Array.from(view.rows()[0].querySelectorAll("dt"), node => node.textContent),
      ["Pretty Name", "Best Score"]);
  }
  assert.equal(formatSubmittedAt("invalid"), undefined);
  assert.equal(formatSubmittedAt(null), undefined);
});

test("search filters visible local rows without messages or subprocess requests", t => {
  const view = ui(t);
  view.send([problem, { ...problem, id: 99, code_name: "remote", is_on_local: false }]);
  const count = view.messages.length;
  view.search("array");
  assert.equal(view.rows().length, 1);
  view.search("remote");
  assert.equal(view.rows().length, 0);
  assert.match(view.document.getElementById("status").textContent, /No local problems match/);
  view.search("");
  assert.equal(view.rows().length, 1);
  assert.equal(view.messages.length, count);
});

test("search and expansion state survive snapshot updates", async t => {
  const view = ui(t);
  view.send([problem]);
  view.rows()[0].open = true;
  await new Promise(resolve => setTimeout(resolve, 0));
  view.search("array");
  view.send([{ ...problem, best_score: 100 }]);
  assert.equal(view.rows()[0].open, true);
  assert.equal(view.document.getElementById("search").value, "array");
  assert.equal(view.state.query, "array");
});

test("untrusted grader strings render as text, never executable markup", t => {
  const view = ui(t);
  const malicious = '<img src=x onerror="alert(1)"><script>alert(1)</script>';
  view.send([{ ...problem, pretty_name: malicious, best_evaluation: malicious }]);
  assert.equal(view.document.querySelectorAll("img, script").length, 0);
  assert.ok(view.document.getElementById("problems").textContent.includes(malicious));
});

test("New Problem and Refresh send only allowlisted actions", t => {
  const view = ui(t);
  view.document.getElementById("new").click();
  view.document.getElementById("refresh").click();
  assert.deepEqual(view.messages, [{ type: "ready" }, { type: "new" }, { type: "refresh" }]);
});

const result = {
  problem_name: "sample", status: 0, points: 0, grader_comment: "[----]", compiler_message: null,
  id: 123, number: 2, language: "cpp", submitted_at: "2026-04-01T10:20:30Z",
};

test("expanded row Submit sends only the selected problem ID", t => {
  const view = ui(t);
  view.send([problem]);
  const submit = view.rows()[0].querySelector('button[aria-label="Submit sample"]');
  assert.equal(submit.textContent, "Submit");
  submit.click();
  assert.deepEqual(view.messages.at(-1), { type: "submit", id: 42 });
});

test("final submission replaces the dashboard with its result, including zero scores", t => {
  const view = ui(t);
  view.send([problem], { outcome: { type: "result", result } });
  assert.equal(view.document.getElementById("dashboard-header").hidden, true);
  assert.equal(view.document.getElementById("problems").hidden, true);
  const pane = view.document.getElementById("submission");
  assert.equal(pane.hidden, false);
  assert.match(pane.textContent, /Submission Result/);
  const fields = Object.fromEntries(Array.from(pane.querySelectorAll("dt"),
    term => [term.textContent, term.nextElementSibling.textContent]));
  assert.equal(fields.Score, "0");
  assert.equal(fields.Status, "Grading complete");
  assert.equal(fields.Evaluation, "[----]");
  assert.equal(fields["Submission ID"], "123");
  assert.match(fields.Submitted, /2026/);
});

test("compilation, grader, and process failures are displayed without claiming full marks", t => {
  const view = ui(t);
  for (const status of [1, 2]) {
    view.send([problem], { outcome: { type: "result", result: {
      ...result, status, points: null, compiler_message: status === 1 ? "syntax error" : null,
      grader_comment: status === 2 ? "grader unavailable" : null,
    } } });
    const text = view.document.getElementById("submission").textContent;
    assert.match(text, status === 1 ? /Compilation error.*syntax error/ : /Grader error.*grader unavailable/);
    assert.equal(text.includes("Grading complete"), false);
  }
  view.send([problem], { outcome: { type: "error", message: "Yoel executable was not found" } });
  assert.match(view.document.getElementById("submission").textContent, /Submission Failed.*executable was not found/);
});

test("Back restores local search, expanded rows, and refreshed best scores", async t => {
  const view = ui(t);
  view.send([problem]);
  view.rows()[0].open = true;
  await new Promise(resolve => setTimeout(resolve, 0));
  view.search("array");
  view.send([problem], { outcome: { type: "result", result } });
  view.document.getElementById("back").click();
  assert.deepEqual(view.messages.at(-1), { type: "back" });
  view.send([{ ...problem, best_score: 100 }], { outcome: null });
  assert.equal(view.document.getElementById("submission").hidden, true);
  assert.equal(view.document.getElementById("dashboard-header").hidden, false);
  assert.equal(view.document.getElementById("search").value, "array");
  assert.equal(view.rows()[0].open, true);
  assert.match(view.rows()[0].textContent, /Best Score100/);
});

test("refresh snapshots do not steal focus or replace the current result's Back button", t => {
  const view = ui(t);
  const outcome = { type: "result", result };
  view.send([problem], { outcome });
  const back = view.document.getElementById("back");
  back.focus();
  view.send([problem], { outcome, message: "Loading problems…" });
  assert.equal(view.document.getElementById("back"), back);
  assert.equal(view.document.activeElement, back);
});

test("busy operations disable submission actions and compiler messages cannot inject HTML", t => {
  const view = ui(t);
  view.send([problem], { busy: true, message: "Waiting for grading…" });
  for (const button of view.document.querySelectorAll("button")) { assert.equal(button.disabled, true); }
  const malicious = '<img src=x onerror="alert(1)"><script>alert(1)</script>';
  view.send([problem], { outcome: { type: "result", result: {
    ...result, status: 1, points: null, compiler_message: malicious,
  } } });
  assert.equal(view.document.querySelectorAll("img, script").length, 0);
  assert.ok(view.document.getElementById("submission").textContent.includes(malicious));
});
