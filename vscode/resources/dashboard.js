(function () {
  "use strict";

  function filterLocalProblems(problems, query = "") {
    const text = query.trim().toLocaleLowerCase();
    return problems.filter(p => p.is_on_local === true &&
      (!text || `${p.code_name} ${p.pretty_name} ${p.id}`.toLocaleLowerCase().includes(text)));
  }

  function formatSubmittedAt(value) {
    if (!value || !Number.isFinite(Date.parse(value))) { return undefined; }
    return new Intl.DateTimeFormat(undefined, {
      year: "numeric", month: "short", day: "numeric",
      hour: "2-digit", minute: "2-digit", second: "2-digit", timeZoneName: "short",
    }).format(new Date(value));
  }

  if (typeof module !== "undefined") { module.exports = { filterLocalProblems, formatSubmittedAt }; }
  if (typeof document === "undefined") { return; }

  const vscode = acquireVsCodeApi();
  const saved = vscode.getState() || {};
  const search = document.getElementById("search");
  const list = document.getElementById("problems");
  const status = document.getElementById("status");
  const header = document.getElementById("dashboard-header");
  const resultPane = document.getElementById("submission");
  const expanded = new Set(Array.isArray(saved.expanded) ? saved.expanded : []);
  let snapshot = [];
  let message = "";
  let busy = false;
  let outcome = null;
  let displayedOutcome = "";
  search.value = typeof saved.query === "string" ? saved.query : "";

  function remember() {
    vscode.setState({ query: search.value, expanded: Array.from(expanded) });
  }

  function field(container, label, text) {
    const term = document.createElement("dt");
    term.textContent = label;
    const value = document.createElement("dd");
    value.textContent = text;
    container.append(term, value);
  }

  function renderOutcome() {
    const signature = JSON.stringify(outcome);
    if (displayedOutcome === signature) { return; }
    displayedOutcome = signature;
    const heading = document.createElement("h2");
    heading.textContent = outcome.type === "error" ? "Submission Failed" : "Submission Result";
    heading.tabIndex = -1;
    const content = document.createElement("dl");
    if (outcome.type === "error") {
      field(content, "Error", outcome.message);
    } else {
      const s = outcome.result;
      field(content, "Problem", s.problem_name);
      field(content, "Status", ["Grading complete", "Compilation error", "Grader error"][s.status]);
      if (s.points !== null) { field(content, "Score", String(s.points)); }
      else if (s.status === 0) { field(content, "Score", "Unavailable"); }
      if (s.id !== undefined) { field(content, "Submission ID", String(s.id)); }
      if (s.number !== undefined) { field(content, "Submission Number", String(s.number)); }
      if (s.language) { field(content, "Language", s.language); }
      const submitted = formatSubmittedAt(s.submitted_at);
      if (submitted) { field(content, "Submitted", submitted); }
      if (s.grader_comment?.trim()) {
        field(content, s.status === 0 ? "Evaluation" : "Grader Message", s.grader_comment);
      }
      if (s.compiler_message?.trim()) { field(content, "Compiler Message", s.compiler_message); }
      if (s.max_runtime != null) { field(content, "Max Runtime (as reported)", String(s.max_runtime)); }
      if (s.peak_memory != null) { field(content, "Peak Memory (as reported)", String(s.peak_memory)); }
    }
    const back = document.createElement("button");
    back.type = "button";
    back.id = "back";
    back.textContent = "Back to Problems";
    back.addEventListener("click", () => vscode.postMessage({ type: "back" }));
    resultPane.replaceChildren(heading, content, back);
    heading.focus();
  }

  function render() {
    header.hidden = !!outcome;
    list.hidden = !!outcome;
    status.hidden = !!outcome;
    resultPane.hidden = !outcome;
    document.getElementById("new").disabled = busy;
    document.getElementById("refresh").disabled = busy;
    if (outcome) {
      renderOutcome();
      return;
    }
    if (displayedOutcome) {
      displayedOutcome = "";
      resultPane.replaceChildren();
      search.focus();
    }
    const problems = filterLocalProblems(snapshot, search.value);
    status.textContent = message || (problems.length ? "" :
      snapshot.length ? "No local problems match your search." : "No local problems. Use New Problem to create one.");
    const rows = problems.map(p => {
      const row = document.createElement("details");
      row.dataset.problemId = String(p.id);
      row.open = expanded.has(p.id);
      const title = document.createElement("summary");
      title.textContent = p.code_name;
      const info = document.createElement("dl");
      field(info, "Pretty Name", p.pretty_name);
      field(info, "Best Score", String(p.best_score));
      const submittedAt = formatSubmittedAt(p.best_submitted_at);
      if (submittedAt) {
        const term = document.createElement("dt");
        term.textContent = "Best Score Submitted";
        const value = document.createElement("dd");
        const time = document.createElement("time");
        time.dateTime = p.best_submitted_at;
        time.textContent = submittedAt;
        value.append(time);
        info.append(term, value);
      }
      if (typeof p.best_evaluation === "string" && p.best_evaluation.trim()) {
        field(info, "Evaluation", p.best_evaluation);
      }
      const open = document.createElement("button");
      open.type = "button";
      open.className = "secondary";
      open.disabled = busy;
      open.textContent = "Open Source";
      open.setAttribute("aria-label", `Open source for ${p.code_name}`);
      open.addEventListener("click", () => vscode.postMessage({ type: "open", id: p.id }));
      const submit = document.createElement("button");
      submit.type = "button";
      submit.textContent = "Submit";
      submit.disabled = busy;
      submit.setAttribute("aria-label", `Submit ${p.code_name}`);
      submit.addEventListener("click", () => vscode.postMessage({ type: "submit", id: p.id }));
      const actions = document.createElement("div");
      actions.className = "actions row-actions";
      actions.append(open, submit);
      row.append(title, info, actions);
      row.addEventListener("toggle", () => {
        if (row.open) { expanded.add(p.id); } else { expanded.delete(p.id); }
        remember();
      });
      return row;
    });
    list.replaceChildren(...rows);
  }

  search.addEventListener("input", () => { remember(); render(); });
  document.getElementById("new").addEventListener("click", () => vscode.postMessage({ type: "new" }));
  document.getElementById("refresh").addEventListener("click", () => vscode.postMessage({ type: "refresh" }));
  window.addEventListener("message", event => {
    const value = event.data;
    if (value?.type !== "snapshot" || !Array.isArray(value.problems)) { return; }
    snapshot = filterLocalProblems(value.problems);
    message = typeof value.message === "string" ? value.message : "";
    busy = value.busy === true;
    outcome = value.outcome?.type === "result" || value.outcome?.type === "error" ? value.outcome : null;
    render();
  });
  vscode.postMessage({ type: "ready" });
})();
