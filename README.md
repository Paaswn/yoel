# yoel

yoel is a Go CLI/TUI client for [Chula's Computer Engineering Grader](https://grader.nattee.net/), with an experimental VS Code extension powered by the same CLI. It is one of my learning projects alongside [Falling sand simulation](https://github.com/Paaswn/sandfall-sim).

> [!IMPORTANT]
>
> ### LLM USAGE (v0.4.x and beyond)
>
> Parts of Yoel, including tests, the grader API package, and the VS Code frontend,
> were developed with LLM assistance. Commits with material Codex contributions
> include a `Co-authored-by: Codex` trailer.

## Installation

### Windows

```pwsh
irm https://raw.githubusercontent.com/Paaswn/yoel/master/scripts/install.ps1 | iex
```

### macOS & Linux

```sh
curl -fsSL https://raw.githubusercontent.com/Paaswn/yoel/master/scripts/install.sh | sh
```

Downloads are checked against the release SHA-256 checksums. If installation
does not work, download the matching archive manually from the
[Yoel Releases page](https://github.com/Paaswn/yoel/releases). Releases are not
currently code-signed or macOS-notarized, so Windows SmartScreen or macOS
Gatekeeper may show a warning; do not disable platform security protections.

## VS Code extension

The experimental extension provides a searchable local-problem dashboard, problem
creation/recreation, and submission results with **Back to Problems**.

1. Install the matching **Yoel CLI v0.6.0-alpha.1** from its
   [release page](https://github.com/Paaswn/yoel/releases/tag/v0.6.0-alpha.1).
   Choose the archive for your OS and CPU, verify it against `checksums.txt`,
   and extract the executable. Older stable binaries may lack the required JSON
   interface; the stable installation scripts above do not select prereleases.
2. Download the [Yoel extension package](vscode/yoel-0.1.0.vsix). In VS Code,
   open **Extensions → … → Install from VSIX…** and select the file, or run:

   ```sh
   code --install-extension yoel-0.1.0.vsix
   ```

3. Put the matching CLI on PATH, or set **Yoel: Executable Path** in VS Code's user
   settings to its absolute path. Run `yoel login` and `yoel fetch` in a terminal
   using that executable.
4. Open a trusted local workspace and click **Yoel** in the Activity Bar.
   Use **New Problem…** to create a problem, then expand it to **Open Source** or
   **Submit**.

Requires desktop VS Code 1.85 or newer. The extension package works on Windows,
macOS, and Linux; the separate CLI executable must match your platform.
Users installing the VSIX do not need Node.js, Go, or an Extension Development Host.
The extension is not published to Marketplace; install updated VSIX files manually.

See the [extension guide](vscode/README.md) for setup, development, and packaging.

## Usage

### `yoel fetch`
 yoel fetch updates local problem registry to match the grader's. fetching notice will be shown if yoel couldn't find any problem metadata from local registry

![fetch](usages/fetch.gif)

### `yoel list`
yoel list shows every available grader's problems which can be seleceted interactively

![list](usages/list.gif)

### `yoel new <query>`
create any problem locally, every attachments are downloaded and seamlessly extracted to your working directory 
![new](usages/new.gif)

### `yoel submit <query>`
submit a problem to the grader. yoel automatically solves which source file to be sent, ( student.h > main.h > main.cpp > *.cpp  )
![submit](usages/submit.gif)


> [!NOTE]
>
> a query could be problem's id, or problem's name

## Updating

Release builds can check and install the latest yoel release with:

```sh
yoel update
```
