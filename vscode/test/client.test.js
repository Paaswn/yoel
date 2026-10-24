const { test } = require("node:test");
const assert = require("node:assert/strict");
const { runJSON, problems, submission, resultMessage, commandError, YoelCommandError } = require("../out/client");

test("subprocess preserves argument boundaries and separates diagnostics", async () => {
  const args = ["path with spaces", "$(not-a-shell)", "--flag"];
  const result = await runJSON(process.execPath, [
    "-e", "console.error('diagnostic');console.log(JSON.stringify(process.argv.slice(1)))", "--", ...args,
  ]);
  assert.deepEqual(result, args);
});

test("missing binary, nonzero exit and invalid JSON are actionable failures", async () => {
  await assert.rejects(runJSON("yoel-nonexistent-binary-for-test", []), /executablePath/);
  await assert.rejects(runJSON(process.execPath, ["-e", "console.error('expired session');process.exit(2)"]), /expired session/);
  await assert.rejects(runJSON(process.execPath, ["-e", "console.log('not JSON')"]), /invalid JSON/);
});

test("structured failures retain codes, messages, and forward-compatible unknown codes", async () => {
  for (const code of ["problem_exists", "session_expired", "future_error"]) {
    const message = "arbitrary message, not parsed by the frontend";
    const payload = JSON.stringify({ error: { code, message } });
    await assert.rejects(
      runJSON(process.execPath, ["-e", `console.error(${JSON.stringify(payload)});process.exit(1)`]),
      error => error instanceof YoelCommandError && error.code === code && error.message === message,
    );
  }
});

test("malformed error payloads fall back to plain text", () => {
  for (const value of ["legacy error", "null", "{}", '{"error":{"code":4,"message":"bad"}}']) {
    assert.equal(commandError(value), undefined);
  }
});

test("problem_exists exposes the backend destination without interpreting the message", () => {
  const error = commandError(JSON.stringify({
    error: { code: "problem_exists", message: "conflict", details: { directory_path: "/workspace/sample" } },
  }));
  assert.equal(error.directoryPath, "/workspace/sample");
  for (const directory_path of [null, 123, {}]) {
    const error = commandError(JSON.stringify({
      error: { code: "problem_exists", message: "conflict", details: { directory_path } },
    }));
    assert.equal(error.directoryPath, undefined);
  }
});

test("empty registry and malformed metadata", () => {
  assert.deepEqual(problems([]), []);
  assert.throws(() => problems(null));
  assert.throws(() => problems([{ id: 1 }]));
});

test("evaluation metadata is optional but must be a string when present", () => {
  const p = {
    id: 42, code_name: "sample", pretty_name: "Sample", source_path: "",
    directory_path: "", is_on_local: false, best_score: 0,
  };
  for (const best_evaluation of [undefined, null, "", "[PP--]"]) {
    assert.equal(problems([{ ...p, best_evaluation }])[0].best_evaluation, best_evaluation);
  }
  assert.throws(() => problems([{ ...p, best_evaluation: 100 }]));
});

test("completed grading is not necessarily full marks", () => {
  const base = { problem_name: "example", points: 0, grader_comment: "[-]", compiler_message: null };
  assert.match(resultMessage(submission({ ...base, status: 0 })), /score 0/);
  assert.match(resultMessage(submission({ ...base, status: 1 })), /compilation error/);
  assert.match(resultMessage(submission({ ...base, status: 2 })), /grader error/);
  assert.throws(() => submission({ ...base, status: "queued" }));
});

test("submission detail fields are optional but validated when supplied", () => {
  const base = { status: 0, problem_name: "sample", points: 50, grader_comment: "[PP--]", compiler_message: null };
  const details = { id: 123, number: 2, submitted_at: "2026-04-01T10:20:30Z", language: "cpp", max_runtime: 0, peak_memory: 0 };
  assert.equal(submission({ ...base, ...details }).id, 123);
  for (const invalid of [{ id: "123" }, { number: 0.5 }, { submitted_at: "invalid" },
    { language: 123 }, { max_runtime: "fast" }, { peak_memory: "small" }]) {
    assert.throws(() => submission({ ...base, ...invalid }));
  }
});

test("best-submission timestamps are optional RFC3339 date strings, not invented dates", () => {
  const p = {
    id: 42, code_name: "sample", pretty_name: "Sample", source_path: "",
    directory_path: "", is_on_local: true, best_score: 100,
  };
  for (const best_submitted_at of [undefined, null, "2026-04-01T10:20:30Z"]) {
    assert.equal(problems([{ ...p, best_submitted_at }])[0].best_submitted_at, best_submitted_at);
  }
  for (const best_submitted_at of ["", "invalid", 123]) {
    assert.throws(() => problems([{ ...p, best_submitted_at }]));
  }
});
