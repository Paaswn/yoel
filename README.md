# Yoel

Yoel is a cli client for [Chula's Computer Engineering Grader](https://grader.nattee.net/) written in Go. It is one of my learning project along with [Falling sand simulation](https://github.com/Paaswn/sandfall-sim). The only difference is that, I token-maxxing Yoel.

## Install

The installers download a prebuilt Yoel release. You do not need Go, Git, a
package manager, or administrator access. They support 64-bit Intel/AMD and
ARM computers on Windows, macOS, and Linux.

### Windows

```pwsh
$script = Join-Path $env:TEMP 'yoel-install.ps1'
Invoke-WebRequest https://raw.githubusercontent.com/Paaswn/yoel/master/scripts/install.ps1 -OutFile $script
& $script

```

### macOS & Linux

```sh
curl -fsSL https://raw.githubusercontent.com/Paaswn/yoel/master/scripts/install.sh -o /tmp/yoel-install.sh
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

## Usage
```sh || pwsh
# validate your installation
yoel --help
# on your first use of yoel use this command
yoel login
```
### Example
```
. (cwd)
```
```sh || pwsh
# you can create a new quetion with question number ( follow grader's order )
yoel question new 1
# or pass a specific question's id
yoel question new --id 674
# or 
yoel question list 
```
```sh
.
├── CPP-Basics-1/  # from `yoel question 1`
│   ├── 673.cpp
│   └── symlink_to_PDF
├── CPP-Basics-2/ # from `yoel question --id 674`
│   ├── 674.cpp
│   └── symlink_to_PDF
├── CPP-Basics-3/ # from choosing in `yoel question list` window
│   ├── 676.cpp
│   └── symlink_to_PDF
```

to submit a question back to grader
```sh || pwsh
# you can submit the folder of question
yoel submit ExampleQustion
# or a source file of question, yoel will automatically find your file under cwd
yoel submit question_id.cpp
# relative or absolute path also accepted
yoel submit ExampleQuestion/example_id.cpp
yoel submit /home/user/ExampleQuestion/example_id.cpp
```
Thanks to [cobra](https://github.com/spf13/cobra), [huh](https://github.com/charmbracelet/huh) and [lipgloss](https://github.com/charmbracelet/lipgloss), Yoel will be one of the **easiest** and **prettiest** cli tool you will ever use.

## LLM Usage

~90% of this repo was written by Codex, [see agent's stuffs](.agents). Afaik, I wrote `showQuestionList()`, some of its helper-functions inside [question.go](internal/cli/question.go), and this README.
