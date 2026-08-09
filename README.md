# Yoel

Yoel is a cli client for [Chula's Computer Engineering Grader](https://grader.nattee.net/) written in Go. It is one of my learning project along with my [Falling sand simulation](https://github.com/Paaswn/sandfall-sim). The only difference is that, I token-maxxing Yoel.

## Backstory you must have before using this cli
If you are a person who hate copying your source code, paste it to the browser, then come back to the IDE over and over like me, Yoel is for you. Yoel make querying, writing, and submitting grader's question a piece of cake. The only requirement is access to the grader site.

## Build

## Quick demo
```sh
# on your first session prompteasiet
yoel login 

# then you can
yoel question list # this will show an interactive list of grader's questions
# or
yoel question new [id] # this, without --language [lang] flag, will create an id.cpp file automatically
```

Thanks to [cobra](https://github.com/spf13/cobra), [huh](https://github.com/charmbracelet/huh) and [lipgloss](https://github.com/charmbracelet/lipgloss), Yoel will be one of the **easiest** and **prettiest** cli tool you will ever use. I lied.

## LLM Usage
~90% of this repo was written by Codex, [see agent's stuffs](.agents). Afaik, I wrote `showQuestionList()`, some of its helper-functions inside [question.go](internal/cli/question.go), and this README.