# Instruction: `yoel update` and update notifications

Read `AGENTS.md` and `GOAL_INSTALL_YOEL.md` completely. This is a CLI and
release-distribution checkpoint. Do not alter the Cafe Grader wire protocol,
the public `internal/graderapi` API, token storage, or submission polling
policy.

## Owner-approved product decisions

`v0.3.0` is the final rapid-feature release before a maintenance/ownership
pass. `yoel update` belongs in this release.

- Add an explicit `yoel update` command.
- Never silently download, replace Yoel, or run an installer during `yoel
  submit` or any other ordinary command.
- After an interactive submission-result flow, Yoel may advise that a newer
  version exists and tell the user to run `yoel update`.
- A notice must never block results, change the primary command exit status,
  or turn a successful grader operation into a failure.
- The first updater may reuse the official platform installer. Do not make a
  separate Go release/downloader/checksum/replacement implementation.
- The installer is the universal install/update path. Future package managers
  own updates for installations they manage.

## Preflight and owner approval

This workspace has stale legacy files. First inspect the actual executable
target, command framework, module path, version mechanism, and whether the
install scripts/release workflow already exist. Do not assume the stale README
binary name (`grader-cli`) is the current `yoel` command.

If binary name, build target, repository owner/name, artifact naming, or
version format differs from this brief, show the owner the exact mismatch and
wait. Do not rename binaries, invent a version system, or fabricate release
URLs as a side effect of this checkpoint.

Before editing code, propose one checkpoint containing:

1. Functions/types and command files to change.
2. Exact public GitHub request(s) and response fields used.
3. Every file to change.
4. Security/error tradeoffs, platforms, and package-manager behavior.

Wait for owner approval before implementation.

## `yoel update`

Add:

```text
yoel update
```

Show installed and latest stable versions, then ask only in an interactive
terminal:

```text
Current version: v0.2.0
Latest version:  v0.3.0

Update Yoel? [y/N]
```

Declining, cancelling, or already being current must leave the install
untouched and succeed. In non-interactive use, do not prompt. Print a clear
next step, or add an explicit non-interactive confirmation flag only after
owner approval.

After confirmation, reuse the official updater path:

| Platform | Updater |
| --- | --- |
| macOS/Linux | Download this repository’s `scripts/install.sh`; run it with `sh` |
| Windows | Download this repository’s `scripts/install.ps1`; run it with PowerShell |

Do not run hidden `curl ... | sh` through one shell string. Prefer:

1. Download the installer over HTTPS to a new temporary file.
2. Run that local file with the appropriate interpreter.
3. Forward only validated, documented, non-secret settings (version and, if
   needed, install directory).
4. Preserve its output and failure status.
5. Remove the temporary script afterwards.

The installer must retain `GOAL_INSTALL_YOEL.md` behavior: download the
matching prebuilt archive, verify SHA-256 before extraction/replacement, and
never use `sudo` or machine-wide PATH. On Windows, report a locked executable
cleanly rather than unsafe overwrite tricks.

Use the same release source, artifact names, checksum format, and version
selection rules as the installer. Do not create parallel update plumbing.

## Release check and version safety

The update checker may call this repository’s public GitHub latest-release API.
Use HTTPS, a bounded body, a CLI-owned context/timeout, and strict JSON
decoding of only required data (for example, `tag_name`).

Validate a received tag before showing it, building URLs, or forwarding it to
the installer. Compare versions using a real semantic-version parser or a
small, tested parser that explicitly supports the project format such as
`vMAJOR.MINOR.PATCH`; never compare version strings lexicographically.

For advisory checks, treat missing/invalid releases, timeout/network errors,
rate limiting, malformed or oversized metadata, and an equal/older version as
non-destructive: do not break ordinary Yoel functionality. `yoel update`
itself should report lookup/installer errors and return non-zero.

If a reliable installed version is absent, stop and ask the owner whether to
add a minimal build-time version variable. Never infer a version from the
executable filename or end-user Git state.

## Advisory notification after submission results

This is only a notification. Consider a check only when:

- the terminal is interactive;
- the primary submission/result output has completed;
- a successful check has not occurred recently;
- `YOEL_NO_UPDATE_CHECK` is not `1`.

Use a small user-owned timestamp/cache, at most once per 24 hours by default.
The owner decides its location. Keep it out of `internal/graderapi`; do not add
global mutable state. Failed checks must not suppress future checks forever.

If newer, append exactly this kind of compact message after results:

```text
A new Yoel version is available: v0.3.0. Run `yoel update` to install it.
```

Do not ask to update from the submission path in v0.3.0. Never check or print
notices when output is piped, redirected, or otherwise non-interactive.

## Installers and package managers

The official installers must be safely rerunnable: they install the latest
stable release (or a requested valid version), verify it, and replace the old
binary. This remains the manual fallback for updating.

Do not add Homebrew, Scoop, WinGet, AUR, apt, or other package-manager support
in this checkpoint. If supported later, package-managed installations should
use their manager instead:

```text
Homebrew: brew upgrade yoel
Scoop:    scoop update yoel
```

`yoel update` must not overwrite a package-manager-owned binary. If reliably
detecting the installation source is not possible, say that in command help and
README; do not guess from PATH order.

## Tests and verification

Tests must not contact the real Cafe Grader, submit real code, or depend on the
live GitHub repository for core assertions. Use a fake HTTP server/injectable
endpoint and fake installer files. Cover at least:

1. Newer/equal/older/invalid version comparison.
2. Sanitized latest-release JSON, malformed JSON, oversized body, non-2xx,
   timeout/cancellation, and rate-limit behavior.
3. Declining update confirmation leaves the executable untouched.
4. Notice appears only after interactive submission results, never for piped
   output, and obeys `YOEL_NO_UPDATE_CHECK=1`.
5. Successful checks are throttled; failures do not permanently suppress a
   retry.
6. The updater uses a temporary installer file and passes only validated,
   non-secret inputs.
7. Errors never reveal a token, password, authenticated grader payload, or
   source code.

Run normal formatting, tests, vet/lint, and any installer test command from
`GOAL_INSTALL_YOEL.md`. After implementation, explain this path and show the
relevant tests:

```text
interactive submission ends
→ optional throttled release check
→ “Run yoel update” notice

yoel update
→ release lookup + confirmation
→ temporary official installer
→ verified release archive
→ safe binary replacement
```

Stop for owner review before unrelated cleanup or new features.
