# yoel

yoel is a cli client for [Chula's Computer Engineering Grader](https://grader.nattee.net/) written in Go. It is one of my learning project alongside with [Falling sand simulation](https://github.com/Paaswn/sandfall-sim).

> [!IMPORTANT]
>
> ### LLM USAGE (v0.4.x and beyond)
>
> There is no usage of LLM outside of ~~writing test cases~~, and graderapi package. Additionally, every part of yoel's source code that is written by Codex has its name as a co-author

## Installation

### Windows

```pwsh
$script = Join-Path $env:TEMP 'yoel-install.ps1'
Invoke-WebRequest https://raw.githubusercontent.com/Paaswn/yoel/master/scripts/install.ps1 -OutFile $script
& $script
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

## Usage

```sh || pwsh
    yoel new <query>
    yoel submit <query>
    # a query can be either name or id of a problem
```

> [!Note]
> There will be a prompt to select a problem if multiple are matched

## Updating

Release builds can check and install the latest Yoel release with:

```sh
yoel update
```
