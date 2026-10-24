# Yoel for VS Code (MVP)

A thin frontend for the Yoel CLI. No server, grader HTTP client, or direct database
access is included.

## Getting started

1. Build/install Yoel **from this revision or a later compatible release**.
   Previously released binaries are not assumed compatible: the extension needs
   working `new --json`, `submit --source`, and structured JSON errors.
2. In a terminal, run `yoel login` and `yoel fetch`.
3. In this directory run `npm ci` and `npm run compile`, then open this directory
   in VS Code and press F5 to launch the Extension Development Host.
4. In the development host, open a trusted local workspace for your problems.
   Set `yoel.executablePath` in user settings if the binary is not on PATH.
   Supply only an executable path, not a shell command or flags.

## Sidebar dashboard

Click **Yoel** in the Activity Bar. The **Local Problems** view lists all problems
whose registry metadata has `is_on_local === true`, including ones outside the
current workspace. Non-local problems never appear in the dashboard or its search
results.

```text
⌄ problem_a
    Pretty Name: Array Problem
    Best Score: 100
    Best Score Submitted: Apr 1, 2026, 10:20:30 AM UTC
    Evaluation: [PPPPP]
› problem_b
› problem_c
```

Problem rows start collapsed. Expand a row to see its **Pretty Name** and
**Best Score**, plus the best submission's **date/time** and **Evaluation** when
known. Unknown dates and evaluations have no row. Dates use your local timezone,
with a timezone indicator; multiline evaluations are shown in full.

The persistent **Search local problems** bar filters by code name, pretty name,
or ID, entirely in memory. Searching and expanding rows never starts a subprocess.
**New Problem…** opens the separate catalog picker and existing creation/recreation
flow; this explicit creation picker can include non-local problems. **Refresh**
reloads the snapshot. Expand a problem and click **Open Source** to open its
registered file, or **Submit** to save and submit that problem's registered source
(regardless of which editor is active).

The sidebar uses a small, theme-aware webview because VS Code's native Tree View
cannot embed a persistent text input. It loads only packaged scripts/styles, uses
a restrictive content security policy, and renders grader text as text, not HTML.
Only local metadata is sent to it; file paths and CLI arguments stay in the
extension host.

The dashboard loads when first shown and refreshes after creation, recreation,
and completed submissions. Refresh manually after changes from an external
terminal. Empty registries and load failures show guidance in the view.

### Submission result screen

After **Submit**, Go submits and polls until it has a final grading result. The
Yoel tab then comes into focus and replaces the dashboard with **Submission Result**:

- Problem, final status, and score, including partial or zero scores.
- Evaluation, compiler messages, or grader messages when supplied.
- Submission ID/number, language, submission time, runtime, and memory when supplied.
  Runtime/memory values are shown as reported, without assuming undocumented units.

**Back to Problems** restores the normal dashboard with its search text and
expanded rows preserved. Best-score metadata refreshes in the background without
dismissing the result screen. The latest result survives hiding/reopening the
sidebar during the same extension session; this is not a persistent submission
history.

The Command Palette's **Yoel: Submit Problem** uses the active editor and shows
the same result screen. Submission buttons are disabled while an operation is
running to prevent duplicate submissions.

Missing sources, failed saves, subprocess errors, and invalid responses show
**Submission Failed**, not a fabricated grading result. Compilation and grader
errors are final grader outcomes, distinct from process failure. A timeout may
occur after the grader accepted the submission; check before retrying.

### Best-score submission details

Go persists the grader's submission timestamp and grader comment (the same
evaluation text used by the CLI) with the score. A result meeting or exceeding the
current registry best can provide its details, including a tied or zero score.
Lower scores and compilation/grader errors do not replace best-score details.
On a tie, a submission with an evaluation is preferred over one without; time and
evaluation are kept from the same chosen submission.

These details are exposed only when their stored score equals the registry's best
score. If a later `yoel fetch` discovers a higher score, previous details are
hidden until a matching result is available. Go automatically upgrades existing
databases, preserving scores, paths, and known evaluations. Legacy evaluations
without timestamps retain an unknown date; older submissions are not backfilled
from the grader.

## Commands

- **Yoel: New Problem** — search the registry, create under the chosen workspace
  folder, and open the registered source. Go decides whether creation is allowed.
  If it reports `problem_exists`, VS Code offers **Open Existing** when a source
  is registered, and **Recreate Problem…** when Go reports the destination.
  Recreation requires a separate modal **Delete and Recreate** confirmation.
  It replaces the entire destination directory, not just the source file; back up
  solutions and other files first. The destination is under the chosen workspace
  and may differ from the old registered location. The registry then points to
  the new source. Cancellation never forces creation.
  PDFs are saved without launching an external viewer.
- **Yoel: Open Problem** — search local problems and open the registered source. Non-local or
  stale paths produce an explicit error; there is no recursive file search.
- **Yoel: Submit Problem** — save the active source and let Go resolve its exact
  registered file identity, submit, and poll until grading finishes. Other files
  in the problem directory are not implicitly submitted. The final result appears
  in the Yoel tab with **Back to Problems**. The Yoel Output channel also retains
  the full structured result.
- **Yoel: Refresh Problems** — reload the local registry snapshot. This does not
  fetch from the grader; run `yoel fetch` when the remote problem list changes.

Pickers refresh once when opened; all typing filters the in-memory snapshot.
Creation and completed submissions also refresh it. Operations are serialized.
Subprocesses have a two-minute limit and bounded output. A timeout/error during
submission may occur after the grader accepted the code; check the grader before
retrying. Authentication remains a terminal operation.

For SSH/WSL/containers, install Yoel and authenticate in the environment where the
extension host runs. Virtual filesystems and untrusted workspaces are unsupported.

## CLI contract

Argument arrays are passed without a shell. Successful commands emit one JSON
value to stdout; diagnostics go to stderr; nonzero exits mean operational failure.
JSON mode is noninteractive, even when used in a terminal.

For commands with `--json`, operational failures emit this envelope on **stderr**
and exit nonzero, leaving stdout empty:

```json
{"error":{"code":"problem_exists","message":"cannot create ...","details":{"directory_path":"/workspace/sample"}}}
```

Stable codes: `problem_exists`, `session_expired`, `problem_not_found`, `timeout`,
`cancelled`, and fallback `command_failed`. Messages are human-readable and must
not be parsed for control flow. Unknown codes remain ordinary failures. Errors
without `--json` retain human-readable text. Older binaries or OS-level failures
may produce plain text; the extension displays it without guessing an error code.
`problem_exists` includes `details.directory_path`: the exact directory forced
creation would replace. Without an absolute backend-supplied destination, the
extension does not offer recreation. Rebuild Yoel if that action is unavailable.

| Invocation | Response |
| --- | --- |
| `yoel list --json` | Array of Go `ProblemLite` objects, including `[]` for an empty registry; no session required |
| `yoel new ID --json --no-pdf` | Created `ProblemLite`, with `is_on_local`, `directory_path`, and `source_path` updated |
| `yoel submit --json --source ABSOLUTE_PATH` | Final Go `SubmissionResult` |

Problem fields consumed: `id`, `code_name`, `pretty_name`, `best_score`,
optional `best_evaluation` and `best_submitted_at`, `is_on_local`, `directory_path`,
`source_path`. Unknown optional fields are omitted. `best_submitted_at` is the
grader's RFC3339 timestamp, normalized to UTC. Paths come from Go, not filename
heuristics. `submit ID` and `submit NAME` remain supported by the CLI.

Submission `status` retains the existing numeric contract: `0` done, `1`
compilation error, `2` grader error. A zero process exit does **not** mean full
marks. `points`, `grader_comment`, and `compiler_message` can be null. Unknown
statuses or malformed responses are rejected rather than treated as success.

## Validation

Run `npm test` here and `go test ./...` at the repository root.

Manual Extension Development Host checklist (requires a grader account):

- Open the Yoel Activity Bar view; verify only local problems appear, collapsed.
- Type in the persistent search bar and verify only matching local problems appear.
- Expand rows and verify Pretty Name, Best Score, and known submission date/time;
  verify unknown dates/evaluations have no row.
- Use **New Problem…** to create a problem and **Open Source** to open it.
- With an unrelated editor active, use a row's **Submit** button and verify it
  saves/submits only the selected problem's source.
- Verify the Yoel tab shows final partial/zero scores and compiler/grader errors,
  and **Back to Problems** restores the previous search and expanded rows.
- Hide/reopen the sidebar on a result, and verify registry refresh does not dismiss
  it. Verify failed saves and submission timeouts show failure, not success.
- Submit a best-scoring solution, restart the host, and verify its evaluation
  and date persist. Submit a lower score and verify best-score details remain intact.
- Search, cancel, create, and open a problem; verify the selected workspace and
  source path, including paths containing spaces.
- Edit a source, submit, and verify saved contents and final points; try a
  compilation error and a partial/zero-score solution.
- Try an unrelated active file, missing binary, expired session, and missing
  registered source; verify clear errors without a terminal prompt.
- Choose an already-local problem with **New Problem**; verify Go rejects creation
  and VS Code offers **Open Existing**. Dismiss it and verify no file opens.
- Choose **Recreate Problem…** in a disposable workspace. Verify the modal shows
  the exact destination. Cancel and verify contents are unchanged, then confirm
  and verify the directory is replaced and the new source opens.
- Delete a registered source, then use **New Problem → Recreate Problem…** to
  restore a fresh source; verify the registry snapshot refreshes.
- Try a destination directory conflict without a registered source; verify an
  explicit recreation option and that contents are preserved unless confirmed.
- Change registry metadata in an external terminal and reopen the picker.
- Verify post-create/post-submit refresh, and that typing does not launch processes.

No Marketplace publication or bundled binary is part of this MVP.
