# Yoel Local Testcase Replay — Implementation Instructions

## Goal

Extend `yoel submit` so that after Cafe Grader finishes judging a submission, Yoel can locally rerun failed testcases using the user's already-compiled program and display the locally reproduced testcase result inside the existing interactive submission-result UI.

This feature should build on the **current development version** of `yoel submit`. Do not redesign the whole submission flow or replace the existing `huh` + `lipgloss` UI.

The main goals are:

1. Preserve the current submission-result UI.
2. Only locally rerun testcases when doing so is useful.
3. Compile the user's source **once**.
4. Run eligible testcases concurrently.
5. Do not freeze the interactive `huh` UI while local programs are executing.
6. Cache the compiled binary and fetched testcase data inside the question directory.
7. Keep Cafe Grader API responsibilities separate from local program execution.

---

# Current `yoel submit` UI

The latest development build renders approximately:

```text
╭─────────────────────────────╮
│ Judging complete            │
│  Attempt  4                 │
│  Score    [PP-P] / 75.00 %  │
╰─────────────────────────────╯


┃ Test Cases Result
┃   Correct
┃   Correct
┃ > WRONG
┃   Correct


Testcase 11464 · wrong · score 0 · time 1 · memory 1536
```

The testcase list is interactive.

The final line:

```text
Testcase 11464 · wrong · score 0 · time 1 · memory 1536
```

is the detail area for the currently selected testcase.

The new local testcase information should integrate into this area instead of introducing an unrelated second UI.

Continue using:

- `huh` for interactive selection/state
- `lipgloss` for rendering/styling

Do not replace these libraries unless required by an unavoidable technical limitation.

---

# Expected `yoel submit` Flow

Conceptually:

```text
source
  │
  ├── submit to Cafe Grader
  │
  ▼
wait for remote judging
  │
  ▼
receive submission result
  │
  ├── all correct / no usable testcases
  │       └── render existing result only
  │
  └── one or more locally reproducible failed testcases
          │
          ├── obtain testcase input / expected output
          │
          ├── compile source ONCE
          │
          ├── run eligible cases concurrently
          │
          ├── cache local results
          │
          └── expose result in existing interactive UI
```

Remote judging remains the source of truth.

Local testcase execution is a debugging aid, not a replacement for Cafe Grader judging.

---

# When Local Execution Should NOT Run

Do **not** locally execute testcase programs in the following cases:

## 1. All remote testcases are correct

If Cafe Grader reports that all testcases passed, there is nothing useful to reproduce locally.

Simply render the normal submission result.

## 2. No usable testcase data exists

If Cafe Grader does not expose testcase input and/or enough information to locally reproduce a testcase, do not attempt execution.

Simply render the remote result.

The UI may indicate that local testcase data is unavailable if this is useful, but this should not be treated as an error.

## 3. Local compilation fails

Compile the user's source once before running any testcase.

If compilation fails:

- do not attempt to execute any testcase;
- preserve and display the normal Cafe Grader result;
- expose the local compiler error in an appropriate detail view;
- do not crash `yoel submit`.

A local compilation failure must not invalidate or overwrite Cafe Grader's remote result.

---

# Eligible Testcases

Local replay should primarily target failed testcases for which Cafe Grader exposes enough data.

For example:

```text
Correct
Correct
WRONG
Correct
```

Only the failed testcase needs local replay.

Do not waste resources rerunning already-correct cases unless a later explicit feature requires it.

Possible remote result types may include:

- correct
- wrong answer
- runtime error
- time limit exceeded
- memory limit exceeded
- compilation error
- other grader-specific statuses

Do not assume every non-correct result can necessarily be reproduced locally.

Create a clear predicate/function for determining whether a testcase is locally runnable rather than scattering this logic throughout the UI code.

---

# Compilation

The user's program must be compiled **once per local replay session**, before testcase goroutines are started.

Do not compile once per testcase.

Conceptually:

```go
binary, err := runner.Compile(...)
if err != nil {
    // expose compile error
    // no testcase execution
    return
}

runTests(binary, testcases)
```

Compilation and local execution should not belong to `graderapi`.

Recommended responsibility split:

```text
graderapi
    fetch remote submission/testcase information

runner
    compile source
    execute local binary
    capture stdout/stderr
    apply timeout
    return execution result

cli
    coordinate submission + local replay
    manage huh/lipgloss UI
```

If the repository does not yet contain a `runner` package, introducing something like:

```text
internal/runner/
```

is reasonable because local process execution is a distinct responsibility from both CLI rendering and Cafe Grader API communication.

Do not introduce unnecessary additional layers such as `service`, `repository`, `usecase`, etc. unless the existing code genuinely requires them.

---

# Concurrent Testcase Execution

After compilation succeeds, eligible testcases should be executable concurrently.

Use goroutines and channels, or another idiomatic Go concurrency mechanism.

Conceptually:

```go
for _, tc := range testcases {
    go func(tc TestCase) {
        result := runner.Run(binary, tc.Input)
        results <- result
    }(tc)
}
```

However, implementation must account for:

- testcase identity
- deterministic association between testcase and result
- process errors
- stderr
- timeout
- cancellation
- user leaving the UI
- avoiding goroutine leaks

The completion order of goroutines must **not** determine testcase display order.

Results should always remain associated with the original Cafe testcase ID/index.

Example:

```go
type LocalTestResult struct {
    TestcaseID string
    Index      int
    Stdout     string
    Stderr     string
    ExitCode   int
    Duration   time.Duration
    Err        error
}
```

Exact types/names may differ based on the existing repository.

---

# Concurrency Limit

Do not blindly spawn an unlimited number of child processes if a submission contains many testcases.

Prefer a bounded worker/semaphore strategy.

A reasonable default is based on available CPU count, with an upper limit.

Example conceptual approach:

```go
limit := min(runtime.NumCPU(), maxConcurrentTests)
```

The exact cap can be decided during implementation.

The important requirement is:

> testcase execution may be concurrent, but concurrency must remain bounded.

---

# Interactive UI Must Remain Responsive

The existing `huh` UI must not block while local testcase processes are running.

The user should still be able to:

- move between testcase entries;
- inspect already-available remote results;
- see local results appear when ready.

Local execution should therefore happen outside the main UI/update path.

Possible architecture:

```text
submission result received
        │
        ▼
interactive model starts
        │
        ├── UI/event loop
        │
        └── local replay coordinator
                 │
                 ├── compile
                 ├── worker goroutines
                 └── result channel
                           │
                           ▼
                     UI state update
```

Do not call a long-running compiler or user binary synchronously from the rendering function.

Do not mutate shared UI state unsafely from arbitrary goroutines.

Results should be sent through a channel or equivalent synchronization boundary and applied by the UI-owning code.

Run the race detector during development where practical:

```bash
go test -race ./...
```

---

# Selected Testcase Detail View

The existing bottom detail area should remain the place where testcase-specific information is shown.

Current example:

```text
Testcase 11464 · wrong · score 0 · time 1 · memory 1536
```

When local replay information exists, extend this area rather than replacing the remote information.

A possible presentation is:

```text
Testcase 11464 · wrong · score 0 · time 1 · memory 1536

Local replay: wrong answer

Input
─────
5
1 2 3 4 5

Expected
────────
15

Got
───
14
```

While the testcase is still running:

```text
Testcase 11464 · wrong · score 0 · time 1 · memory 1536

Local replay: running…
```

If local replay is unavailable:

```text
Local replay: unavailable
```

If the process crashes:

```text
Local replay: runtime error
```

If it times out:

```text
Local replay: timed out
```

Exact styling should follow the existing `lipgloss` design language.

Do not make the local result visually look more authoritative than Cafe Grader's remote result.

---

# Output Comparison

Do not assume that byte-for-byte equality perfectly reproduces Cafe Grader behavior.

The remote grader remains authoritative.

For the first implementation, use a clearly defined local comparison policy.

Recommended initial policy:

1. capture stdout;
2. normalize line endings;
3. optionally ignore insignificant trailing whitespace according to what the Nattee/Cafe grader actually does, if verified;
4. compare against the expected testcase output.

Keep this comparison logic isolated, for example:

```text
internal/runner/compare.go
```

or equivalent.

Do not bury comparison semantics inside UI code.

If exact Cafe Grader checker behavior is unknown, document the local result as an approximation.

Avoid claiming:

```text
Local PASS => Cafe will accept
```

The intended meaning is:

```text
Yoel reproduced this available testcase locally.
```

---

# Process Execution Safety

User programs are arbitrary binaries and may:

- run forever;
- consume excessive output;
- crash;
- write to stderr;
- exit non-zero;
- spawn subprocesses.

At minimum, local execution needs a timeout.

Use `context.Context` / `exec.CommandContext` or an equivalent mechanism so Yoel can terminate a hung local process.

Do not allow an unlimited stdout/stderr buffer.

Place reasonable limits on captured process output.

A malicious or buggy program should not be able to make the Yoel UI hang indefinitely or consume unbounded memory.

---

# Cache Layout

Question directories should contain a hidden Yoel-managed cache directory.

Prefer a tool-specific directory such as:

```text
QuestionName/
├── .yoel/
│   ├── bin/
│   │   └── <build-id>
│   └── testcases/
│       └── <submission-or-testcase-data>
├── id.cpp
└── <question PDF symlink>
```

Prefer `.yoel/` over `.tmp/`.

Reason:

- `.tmp` implies disposable generic temporary data;
- `.yoel` clearly indicates ownership;
- it gives room for future Yoel-managed metadata;
- a dot-prefixed directory is hidden by convention on Unix-like systems;
- Windows can additionally mark/manage the directory appropriately if desired.

Do not rely on Unix-only filesystem behavior for correctness.

---

# Binary Cache

The compiled program should live under something like:

```text
.yoel/bin/
```

Example:

```text
.yoel/bin/8ca11f...
```

or on Windows:

```text
.yoel/bin/8ca11f....exe
```

Do not use a testcase ID as the binary name unless the testcase ID genuinely identifies the source build.

The binary corresponds to the **source/compiler configuration**, not to one testcase.

A build/cache key should ideally consider enough information to avoid running a stale binary, such as:

- source content hash
- language
- compiler
- relevant compiler flags

For the first implementation, recompiling once per `yoel submit` invocation is acceptable if implementing correct build caching would significantly complicate the feature.

Correctness is more important than clever caching.

Never reuse a cached executable when Yoel cannot confidently determine that it matches the current source.

---

# Testcase Cache Format

Recommended first implementation: use JSON for metadata and separate files for potentially large input/output.

Example:

```text
.yoel/
└── testcases/
    └── 11464/
        ├── meta.json
        ├── input.txt
        ├── expected.txt
        ├── stdout.txt
        └── stderr.txt
```

Example `meta.json`:

```json
{
  "testcase_id": "11464",
  "remote_status": "wrong",
  "score": 0,
  "remote_time": 1,
  "remote_memory": 1536,
  "local_status": "wrong",
  "exit_code": 0,
  "duration_ms": 3
}
```

Advantages over placing everything in one JSON object:

- testcase input/output can be inspected directly;
- multiline text requires no annoying JSON escaping when debugging manually;
- large stdout does not make metadata unreadable;
- files can later be reused directly as stdin.

This structure is only a recommendation.

If the existing Cafe API response naturally maps more cleanly to a single JSON cache file, that is also acceptable.

Do not introduce a database for this feature.

---

# Cache Writes

Cache writes should be safe against partially written files.

When practical:

1. write to a temporary file;
2. close it;
3. rename it to the final path.

Failure to write cache data should generally not make `yoel submit` fail if the submission result itself is still available.

Treat cache errors as secondary unless the cache is required for the requested operation.

---

# Suggested Internal Types

These are conceptual, not mandatory APIs.

```go
type TestCase struct {
    ID       string
    Input    string
    Expected string
}
```

```go
type RunResult struct {
    TestcaseID string
    Stdout     string
    Stderr     string
    ExitCode   int
    Duration   time.Duration
    TimedOut   bool
    Err        error
}
```

```go
type ComparisonResult struct {
    Match bool
    // optional diff/normalized values later
}
```

Keep remote grader data and local execution data distinguishable.

Do not overload one status field so that it becomes unclear whether `"wrong"` came from Cafe Grader or Yoel's local comparison.

Prefer concepts such as:

```go
RemoteStatus
LocalStatus
```

where needed.

---

# Error Handling

The following conditions should be recoverable and should not crash the interactive result UI:

- testcase API unavailable
- testcase has no public input
- expected output unavailable
- local compiler missing
- local compilation failure
- binary execution failure
- timeout
- cache read/write failure
- malformed cached testcase
- individual testcase runner error

A failure to locally reproduce one testcase should not prevent other eligible testcases from completing.

---

# Testing Requirements

Add unit/integration tests around the new behavior.

At minimum test:

## Runner

- program receives testcase input through stdin;
- stdout is captured;
- stderr is captured;
- non-zero exit status is represented correctly;
- timeout kills/terminates the process;
- multiple testcase runs remain associated with their correct testcase IDs;
- concurrent completion order does not reorder logical testcase results.

## Compilation

- valid source compiles;
- invalid source produces a compile error;
- no testcase process starts after compilation failure.

## Comparison

- exact match;
- mismatch;
- line-ending handling;
- whatever whitespace normalization policy is selected.

## Cache

- writes and reads testcase data;
- stale/corrupt cache fails gracefully;
- binary path works on the target platform;
- cache directory creation is idempotent.

## CLI/UI orchestration

- all-correct submission does not trigger local replay;
- no-testcase-data submission does not trigger local replay;
- compile failure still renders remote submission results;
- failed eligible testcase eventually receives a local result;
- selection remains tied to the correct testcase while asynchronous results arrive.

Do not write tests that depend on timing sleeps when channels/synchronization can make them deterministic.

---

# Non-Goals for the First Version

Do not expand this feature into a complete local clone of Cafe Grader.

The first version does **not** need:

- hidden grader testcases;
- perfect emulation of Cafe's sandbox;
- exact memory-limit reproduction;
- exact CPU-time reproduction;
- Docker/container isolation;
- custom checker support unless already exposed and trivial;
- automatic prediction of final grader acceptance;
- a generalized build system;
- support for every possible language immediately;
- a database;
- remote submission replacement.

The purpose is local debugging of grader-exposed testcase data.

---

# Important Implementation Principles

## Preserve current behavior first

The existing `yoel submit` flow already works.

The local replay feature should be additive.

A user who cannot use local replay should still receive the same useful remote result they receive today.

## Remote result is authoritative

Never overwrite Cafe Grader's verdict with Yoel's local verdict.

Example:

```text
Remote: WRONG
Local:  Correct
```

is a valid state and should be displayable.

It may indicate environment differences, checker behavior, nondeterminism, stale testcase data, or another mismatch.

## Compile once, execute many

This is a hard requirement for one replay session.

```text
1 source
   ↓
1 compilation
   ↓
N testcase executions
```

not:

```text
N testcases
   ↓
N compilations
```

## UI work and process execution must be decoupled

Do not run user binaries directly inside rendering callbacks.

The UI owns presentation state.

Workers perform slow external work.

Channels/messages transfer results between them.

## Keep boundaries boring and obvious

Prefer:

```text
cli      -> orchestration/UI
graderapi -> Cafe communication
runner   -> local process execution
cache    -> cache helpers, only if enough logic exists to justify separation
```

Do not create packages merely to make the directory tree look more architectural.

---

# Open Questions / Deferred Decisions

These should be resolved while implementing based on the actual Nattee/Cafe API and existing Yoel code.

1. Exactly which Cafe/Nattee endpoint exposes testcase input?
2. Does it expose expected output directly?
3. Which remote testcase statuses expose testcase data?
4. What output comparison semantics does Cafe Grader use?
5. What local timeout should Yoel use?
6. What maximum concurrency should Yoel allow?
7. Should cached testcase data be keyed by testcase ID, submission ID, or both?
8. Which languages should local compilation support in the first release?
9. Should local replay begin automatically after remote judging, or lazily when the user selects a failed testcase?

Unless repository/API evidence strongly suggests otherwise, default to:

- automatic replay of eligible failed cases;
- one compile;
- bounded concurrent execution;
- `.yoel/` cache directory;
- testcase-ID-based data folders;
- source-hash-based binary identity;
- remote result always visible;
- local replay shown as supplemental information.

---

# Expected End State

Given:

```text
╭─────────────────────────────╮
│ Judging complete            │
│  Attempt  4                 │
│  Score    [PP-P] / 75.00 %  │
╰─────────────────────────────╯

┃ Test Cases Result
┃   Correct
┃   Correct
┃ > WRONG
┃   Correct
```

Yoel should be able to fetch the available failed testcase, compile the submitted source once, run the failed testcase locally without freezing the UI, and update the selected testcase detail area when the result arrives.

For example:

```text
Testcase 11464 · wrong · score 0 · time 1 · memory 1536

Local replay · wrong

Input
─────
5
1 2 3 4 5

Expected
────────
15

Got
───
14
```

The implementation should feel like an extension of the existing `yoel submit` experience rather than a separate subsystem bolted onto it.
