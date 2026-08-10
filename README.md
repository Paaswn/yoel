# Yoel

Yoel is a cli client for [Chula's Computer Engineering Grader](https://grader.nattee.net/) written in Go. It is one of my learning project along with [Falling sand simulation](https://github.com/Paaswn/sandfall-sim). The only difference is that, I token-maxxing Yoel.

## Install

The installers download a prebuilt Yoel release. You do not need Go, Git, a
package manager, or administrator access. They support 64-bit Intel/AMD and
ARM computers on Windows, macOS, and Linux.

### Windows

```pwsh
$script = Join-Path $env:TEMP 'yoel-install.ps1'
Invoke-WebRequest https://raw.githubusercontent.com/Paaswn/yoel/main/scripts/install.ps1 -OutFile $script
& $script

```

### macOS & Linux

```sh
curl -fsSL https://raw.githubusercontent.com/Paaswn/yoel/main/scripts/install.sh -o /tmp/yoel-install.sh
sh /tmp/yoel-install.sh

```

The installers add their user-owned install directory to `PATH` when needed.
Open a new terminal afterwards, then verify with `yoel --help`. Run the same
installer again to replace Yoel with the latest stable release. To install a
specific version, set `YOEL_VERSION`, for example `YOEL_VERSION=v0.1.0 sh
/tmp/yoel-install.sh` on macOS/Linux or `$env:YOEL_VERSION='v0.1.0'; & $script`
in PowerShell.

Downloads are checked against the release SHA-256 checksums. If installation
does not work, download the matching archive manually from the
[Yoel Releases page](https://github.com/Paaswn/yoel/releases). Releases are not
currently code-signed or macOS-notarized, so Windows SmartScreen or macOS
Gatekeeper may show a warning; do not disable platform security protections.

## Build from source

### Windows

```pwsh
# clone this repo to anywhere using this command
git clone https://github.com/Paaswn/yoel.git --depth=1
cd Yoel && go build -o yoel .\cmd\yoel\main.go
# then add the binary to your system's PATH
```

### Mac & Linux

```sh
# clone this repo to anywhere using this command
git clone https://github.com/Paaswn/Yoel.git --depth=1
cd Yoel && go build -o yoel ./cmd/yoel/main.go
# then add the binary to your system's PATH
```

## Commands

```sh || pwsh
yoel help # display all command available for yoel

yoel login # prompt this in your first time using yoel

yoel question list # this will show an interactive list of grader's questions
# or
yoel question new [id] # this, without --language [lang] flag, will create an id.cpp file automatically

yoel question submit [file] # will submit this file to grader
```

Thanks to [cobra](https://github.com/spf13/cobra), [huh](https://github.com/charmbracelet/huh) and [lipgloss](https://github.com/charmbracelet/lipgloss), Yoel will be one of the **easiest** and **prettiest** cli tool you will ever use.

## LLM Usage

~90% of this repo was written by Codex, [see agent's stuffs](.agents). Afaik, I wrote `showQuestionList()`, some of its helper-functions inside [question.go](internal/cli/question.go), and this README.
