# Yoel

Yoel is a cli client for [Chula's Computer Engineering Grader](https://grader.nattee.net/) written in Go. It is one of my learning project along with [Falling sand simulation](https://github.com/Paaswn/sandfall-sim). The only difference is that, I token-maxxing Yoel.

## Install

### Windows

```pwsh

```

### Max & Linux

```sh

```

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
