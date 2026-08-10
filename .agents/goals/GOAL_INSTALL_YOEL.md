# Goal: beginner-friendly `yoel` installation on Windows, macOS, and Linux

Read `AGENTS.md` completely before doing anything. This is an installation and
release-distribution checkpoint only. Do not change Yoel's commands, argument
parsing, API behavior, output wording, token storage, or product flow.

## User story

Friends using Windows and macOS want to install `yoel`, but they do not know how
to clone or compile a Go project. Installation must not require Git, Go, a
package manager, administrator/root access, or knowledge of shell profiles.

The README should ultimately offer one short command for Unix-like systems and
one short command for Windows PowerShell. The installer must download a
prebuilt binary, install it in a user-owned directory, add that directory to the
user's `PATH`, and explain when a new terminal must be opened.

This is not yet a Homebrew, Scoop, Winget, Chocolatey, AUR, Debian, or other
package-manager integration.

## Required deliverables

Subject to the repository's existing layout and release conventions, add:

```text
scripts/install.sh       # macOS and Linux
scripts/install.ps1      # Windows PowerShell
.github/workflows/release.yml
README.md                # short installation section only
```

If an equivalent release workflow or installer already exists, extend it
instead of creating a competing path. Do not silently replace a working release
process.

The executable exposed to users must be named:

```text
yoel       # macOS/Linux
yoel.exe   # Windows
```

Before editing, inspect the actual Go command package and identify the correct
`go build` target. Do not assume the stale `grader-cli` name or blindly rename
packages. If the repository does not currently build a `yoel` executable, stop
and show the owner the mismatch before proceeding.

## Release artifacts

The installers must download prebuilt files from GitHub Releases. If the
repository does not already publish suitable artifacts, add a release workflow
triggered by version tags such as:

```text
v0.1.0
```

Build with the Go version declared by `go.mod`. At minimum publish:

| Platform | `GOOS` | `GOARCH` | Archive |
|---|---|---|---|
| Windows Intel/AMD 64-bit | `windows` | `amd64` | `.zip` |
| Windows ARM 64-bit | `windows` | `arm64` | `.zip` |
| macOS Intel | `darwin` | `amd64` | `.tar.gz` |
| macOS Apple Silicon | `darwin` | `arm64` | `.tar.gz` |
| Linux Intel/AMD 64-bit | `linux` | `amd64` | `.tar.gz` |
| Linux ARM 64-bit | `linux` | `arm64` | `.tar.gz` |

Use one explicit, stable artifact naming scheme. Prefer:

```text
yoel_<version>_<os>_<arch>.tar.gz
yoel_<version>_<os>_<arch>.zip
checksums.txt
```

For example:

```text
yoel_v0.1.0_darwin_arm64.tar.gz
yoel_v0.1.0_windows_amd64.zip
```

Every archive must contain only the executable and any essential license/readme
files. Generate SHA-256 hashes for all archives in `checksums.txt`, and attach
the checksum file to the same GitHub Release.

The workflow must:

- run `go test ./...` before publishing;
- build from the repository source at the tag being released;
- use reproducible, explicit `GOOS` and `GOARCH` values;
- set an appropriate version in the binary only if the project already has a
  supported version variable; do not invent a version subsystem in this
  checkpoint;
- grant only the GitHub Actions permissions needed to create release assets;
- avoid production credentials and never contact the real grader during tests;
- fail the release when tests, compilation, packaging, or checksum generation
  fails.

Prefer an ordinary GitHub Actions matrix and standard shell/PowerShell tools.
Do not add a release framework or production dependency unless the owner
approves it first.

## Unix installer: `scripts/install.sh`

The POSIX-compatible shell installer supports macOS and Linux. It must:

1. Fail clearly on unsupported operating systems or CPU architectures.
2. Detect the operating system from `uname -s`:
   - `Darwin` -> `darwin`
   - `Linux` -> `linux`
3. Detect the CPU architecture from `uname -m`:
   - `x86_64` or `amd64` -> `amd64`
   - `arm64` or `aarch64` -> `arm64`
4. Resolve the repository owner/name from one audited constant near the top of
   the script. Do not ask beginners to edit the script.
5. Install the latest stable release by default.
6. Optionally accept a version through a documented mechanism such as
   `YOEL_VERSION=v0.1.0`; validate it before incorporating it into a URL.
7. Download over HTTPS from this project's GitHub Releases only.
8. Download into a newly created temporary directory and clean it with `trap`.
9. Download the archive and `checksums.txt`, then verify the archive's SHA-256
   before extracting it. Support the checksum utility normally available on
   each supported OS, such as `sha256sum` on Linux and `shasum -a 256` on
   macOS. Stop on any mismatch.
10. Validate that the archive contains the expected `yoel` executable. Do not
    allow archive paths to choose arbitrary installation destinations.
11. Install to `${YOEL_INSTALL_DIR:-$HOME/.local/bin}` and create that directory
    when needed.
12. Make the installed file executable and replace an older `yoel` atomically
    where practical.
13. Never use `sudo`, never write to `/usr/local/bin`, and never modify system
    profiles.
14. Add the install directory to the user's `PATH` only when it is absent.
15. Make profile updates idempotent: repeated installer runs must not append
    duplicate `PATH` lines.
16. Choose the relevant user profile conservatively based on the active shell:
    - zsh: prefer `~/.zprofile`;
    - bash: use the existing login/profile file when appropriate, otherwise a
      documented default;
    - unsupported/unknown shell: install successfully, then print the exact
      line the user should add manually instead of modifying a guessed file.
17. Never overwrite a profile. Append one clearly marked, safely quoted line.
18. Explain that an installer subprocess cannot change the parent shell's
    environment. If `PATH` was updated, tell the user to open a new terminal or
    run the appropriate `source` command.
19. Finish by printing the installed path and a simple verification command:

```text
yoel --help
```

Do not print tokens, environment secrets, or unrelated shell configuration.

The script must run with strict error handling appropriate to its chosen shell.
Quote paths correctly, including home directories containing spaces.

## Windows installer: `scripts/install.ps1`

The PowerShell installer supports ordinary non-admin Windows users. It must:

1. Work in the PowerShell version the project explicitly documents. Prefer
   compatibility with Windows PowerShell 5.1 unless a required feature makes
   that unreasonable.
2. Detect `amd64` versus `arm64` using reliable .NET/PowerShell APIs. Reject
   unsupported architectures clearly.
3. Install the latest stable release by default, with an optional documented
   version override equivalent to the Unix installer.
4. Download the matching `.zip` and `checksums.txt` over HTTPS from this
   project's GitHub Releases.
5. Use a newly created temporary directory and remove it in `finally`.
6. Verify SHA-256 using `Get-FileHash` before extraction. Stop on mismatch.
7. Validate that the archive contains `yoel.exe` and install only that expected
   file.
8. Install by default to:

```text
%LOCALAPPDATA%\Programs\yoel\bin\yoel.exe
```

9. Permit an explicit install-directory override, but never require admin
   privileges.
10. Add the install directory to the **current user's** persistent `PATH` using
    `[Environment]::SetEnvironmentVariable(..., 'User')` only when it is absent.
11. Compare Windows `PATH` entries case-insensitively and normalize trailing
    separators so reruns do not create duplicates.
12. Update `$env:Path` for the current installer process where useful, while
    clearly explaining that already-open terminals will not inherit a newly
    persisted user `PATH`.
13. Never modify the machine-wide `PATH`, registry locations unrelated to the
    user environment, PowerShell profiles, or execution policy.
14. Replace an existing `yoel.exe` safely and report a clear error if the file
    is locked by a running process.
15. Finish by printing the installed path, telling the user to open a new
    terminal if necessary, and showing:

```text
yoel --help
```

Use terminating errors and preserve useful HTTP/status context without dumping
response bodies or secrets.

## README installation section

Add a concise section for non-programmers. It should say that the installer
downloads a prebuilt release; users do not need Go or Git.

Provide separate macOS/Linux and Windows PowerShell commands using the final
raw GitHub URLs from this repository. Do not leave `OWNER/REPO` placeholders in
the committed README.

Prefer a transparent download-then-run example that lets users inspect the
script, for example:

```sh
curl -fsSL <raw-install.sh-url> -o /tmp/yoel-install.sh
sh /tmp/yoel-install.sh
```

```powershell
$script = Join-Path $env:TEMP 'yoel-install.ps1'
Invoke-WebRequest <raw-install.ps1-url> -OutFile $script
& $script
```

If the owner specifically wants shorter `curl | sh` or `irm | iex` one-liners,
they may be shown as an additional convenience, but do not make opaque remote
code execution the only documented installation route.

Also document:

- supported OS/CPU combinations;
- reopening the terminal after a first-time `PATH` change;
- `yoel --help` as the verification step;
- rerunning the installer to replace the current binary with the latest stable
  release;
- the version-override syntax, if implemented;
- a short manual fallback link to the GitHub Releases page.

Do not add login, token, grader usage, or command tutorials to this checkpoint.

## Version and URL behavior

Do not guess GitHub's latest release asset URL if the artifact filename embeds
the version. GitHub's `/releases/latest/download/...` redirect works only when
the final asset name is known. Choose one coherent design and explain it before
implementation. Acceptable approaches include:

- query GitHub's public latest-release API, validate the returned tag, then
  build the versioned asset name; or
- publish additional stable, unversioned asset names intended for
  `/releases/latest/download/...`.

Avoid parsing GitHub HTML. Do not require authenticated GitHub API access for a
public repository. Handle rate limiting and missing releases with a useful
error that points users to the Releases page.

All externally derived tag, filename, OS, and architecture values must be
validated before use in paths, commands, or URLs.

## Tests and verification

Installer tests must not depend on the live GitHub repository for their core
logic. Structure the scripts so download endpoints and install directories can
be overridden in tests, while production defaults remain fixed and safe.

At minimum verify:

1. OS and architecture mapping.
2. Correct artifact selection.
3. Checksum success and checksum mismatch failure.
4. Successful installation into a temporary user-owned directory.
5. Upgrade/reinstall replaces the old binary.
6. Unsupported OS/architecture fails without partial installation.
7. Missing/corrupt archive fails without replacing a working binary.
8. Unix `PATH` profile update is idempotent.
9. Windows user `PATH` update is idempotent and case-insensitive.
10. Paths containing spaces are handled correctly.
11. Temporary files are cleaned up on both success and failure.

Use the safest practical local test method for each script. A fake local HTTP
server or checked-in tiny fixtures is preferred over live network calls. Do not
download or execute the real release as part of `go test ./...`.

Before handoff, run the repository's existing checks plus relevant script
syntax/lint checks available in the development environment. Also inspect the
generated archives and ensure the binary inside has the expected name.

If Windows or macOS cannot be executed in the current environment, do not claim
end-to-end testing there. Validate syntax and isolated logic locally, rely on a
GitHub Actions OS matrix where appropriate, and clearly state what remains
platform-tested by CI.

## Security and non-goals

- Never require or collect the user's GitHub credentials.
- Never disable TLS verification, PowerShell execution policy, antivirus, or
  platform security features.
- Never pipe unchecked downloaded data into extraction or execution.
- Never continue after checksum verification fails.
- Never use `eval` or construct executable shell/PowerShell commands from
  untrusted release metadata.
- Never delete broad directories; cleanup must target only the installer's
  exact temporary directory.
- Do not add auto-update behavior inside `yoel`.
- Do not add background services, scheduled tasks, telemetry, desktop
  shortcuts, shell completion, uninstallers, or package-manager manifests.
- Do not sign or notarize binaries in this checkpoint unless the owner already
  has the required signing identities and explicitly approves that separate
  work.

macOS Gatekeeper or Windows reputation warnings may still occur for unsigned
binaries. Document observed behavior honestly; do not instruct users to disable
system-wide security protections.

## Ownership boundary

The agent owns only release packaging, installer scripts, installer-focused
tests, and the small README installation section.

The project owner continues to own:

- Yoel's commands and arguments;
- user-facing behavior after the process starts;
- token storage and authentication flow;
- terminal output from the application;
- grader API behavior;
- future package-manager distribution;
- code-signing/notarization decisions.

## Required checkpoint proposal

Before editing any implementation file, explain and wait for owner approval:

1. the existing executable build target and final binary name;
2. the release artifact names and supported platforms;
3. how the installers determine the latest version;
4. the exact default install directory on Unix and Windows;
5. exactly which user profile/environment value will be changed for `PATH`;
6. checksum verification and safe replacement behavior;
7. files to be added or changed;
8. how each installer will be tested; and
9. any unsigned-binary warning expected on macOS or Windows.

After approval, implement only this checkpoint. After implementation, show the
owner:

- the final README commands;
- one release artifact name per supported OS;
- the install and `PATH` flow in plain language;
- test/check results and which platforms actually executed them; and
- any remaining release prerequisite, such as pushing a `v*` tag.

Do not create or push a release, tag, or commit unless the owner separately
authorizes that action or the repository's applicable commit workflow already
authorizes an ordinary local commit. Never push automatically.
