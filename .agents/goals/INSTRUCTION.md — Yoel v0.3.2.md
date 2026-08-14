# Yoel v0.3.2 Implementation Instructions

## Goal

Implement the v0.3.2 patch release as a focused usability improvement around question discovery, attachments, and submission targeting.

This release contains three user-facing changes:

1. Improve `yoel question new` so questions can be searched using cached/grader metadata instead of only question ID or question order.
2. Support questions containing multiple attachment files.
3. Allow `yoel submit <path-to-file> <question>` to explicitly submit a source file to a selected question.

These changes should be implemented as separate GitHub issues and pull requests.

Avoid unrelated refactors. v0.3.2 is a patch release, not the architectural cleanup planned for later versions.

---

# 1. Shared Question Resolution

The most important implementation rule for this release is:

> Do not create separate question lookup implementations for `question new`, `submit`, and other commands.

There should be one reusable mechanism for resolving/searching questions from the local registry.

Existing question metadata should be reused wherever possible.

A question may contain data such as:

- ID
- order/index
- short name
- full name
- tags
- difficulty
- score/submission metadata

The resolver/search layer should operate on the existing cached problem registry rather than requiring each CLI command to understand how problems are stored.

Example conceptual API:

```go
SearchProblems(query string) []Problem
```

or:

```go
ResolveProblem(query string) (Problem, error)
```

Exact naming and structure are implementation details.

The important requirement is that commands share the same resolution behavior.

---

# 2. Improve `yoel question new`

## Current behavior

`yoel question new` currently accepts only identifiers such as:

```bash
yoel question new 1142
```

or a question's position/order.

This makes finding questions unnecessarily difficult when the user remembers the name, tag, or part of the title but not its ID.

## New behavior

The command should accept a general query:

```bash
yoel question new <query>
```

Examples:

```bash
yoel question new 1142
yoel question new 17
yoel question new segment
yoel question new "segment tree"
yoel question new graph
```

The query should be searched against locally available problem metadata.

At minimum support:

- exact problem ID
- question order/index where currently supported
- exact short name
- exact full name
- case-insensitive prefix matching
- case-insensitive substring matching
- tags, if tags are available in the registry

Do not introduce heavyweight fuzzy-search dependencies for this patch unless they are clearly necessary.

Simple normalized matching should be sufficient.

## Match priority

Exact matches should have higher priority than broad matches.

Suggested conceptual priority:

```text
exact ID
exact order/index
exact short name
exact full name
prefix match
substring match
tag match
```

Exact matching does not need to literally use this implementation order if the resulting behavior is equivalent.

## Single match

If exactly one appropriate question is found, use it directly.

Example:

```bash
yoel question new segment-tree
```

If only one matching problem exists, continue without showing a selector.

## Multiple matches

If the query is ambiguous, display an interactive selector using the existing terminal UI stack.

Example:

```text
? Select question

> 1120 — Tree Traversal
  1142 — Segment Tree
  1191 — Minimum Spanning Tree
```

Reuse existing `huh`/Lip Gloss conventions instead of introducing another interactive UI library.

## No matches

Question lookup should be cache-first.

Desired behavior:

```text
load cached registry
        ↓
search query
        ↓
match found?
   yes → continue
   no
        ↓
refresh problem data from Cafe Grader once
        ↓
update registry/cache
        ↓
search again
```

This prevents unnecessary network requests during normal use while ensuring newly-added grader problems can still be discovered.

If the registry does not exist yet, fetch it from the grader, populate the cache, and then perform the search.

Do not repeatedly refresh for the same command invocation.

If there are still no matches after refreshing, return a useful error.

Example:

```text
No question matching "segmentxyz" was found.
```

---

# 3. Multiple Question Attachments

## Current behavior

The current attachment handling assumes that a question has exactly one attachment.

Some Cafe Grader questions contain multiple files, for example:

```text
main.cpp
solution.h
```

or:

```text
main.cpp
helper.hpp
data.txt
```

The current implementation must no longer assume:

```go
len(files) == 1
```

## Required behavior

When a question provides multiple downloadable attachments, Yoel should handle all of them correctly.

Requirements:

- retrieve the full attachment list
- download every requested/required attachment
- preserve the original filenames
- avoid overwriting files accidentally
- handle questions with zero, one, or many attachments
- propagate download errors clearly

Do not rename files unless required to avoid destructive behavior.

Starter-code filenames can be semantically important and should normally remain unchanged.

## Destination behavior

Reuse the existing destination/directory behavior where possible.

If attachment downloading currently creates or uses a question workspace, multiple files should be written into the same appropriate workspace.

Example result:

```text
question-directory/
├── main.cpp
├── solution.h
└── helper.hpp
```

Do not scatter files across unrelated locations.

## Partial failures

If one file fails to download, return enough information to identify which attachment failed.

Do not silently report success when only part of the attachment set was downloaded.

---

# 4. Explicit Question Target for `yoel submit`

## New syntax

Support:

```bash
yoel submit <path-to-file> <question>
```

Examples:

```bash
yoel submit ./main.cpp 1142

yoel submit ./main.cpp segment-tree

yoel submit ./solutions/tree.cpp "segment tree"
```

`<question>` must use the same shared question-resolution mechanism used by `question new`.

Do not implement separate name matching specifically for `submit`.

## Existing behavior

Existing convenient submission behavior should continue working where possible.

For example, if Yoel can already infer a question from:

- filename
- directory
- cached mapping
- problem ID embedded in a filename

that behavior should not be unnecessarily removed.

The explicit second argument should act as an unambiguous override.

Conceptually:

```text
yoel submit <file>
        ↓
existing inference

yoel submit <file> <question>
        ↓
explicit question resolution
        ↓
submit file to that problem
```

If an explicit question was provided, do not silently submit to a different inferred problem.

The explicit target wins.

## Ambiguous question names

If the supplied question query matches multiple questions, use the same ambiguity handling as the shared resolver.

Prefer shared behavior over command-specific edge cases.

---

# 5. Architecture Constraints

v0.3.2 should improve behavior without creating more technical debt.

Avoid implementations resembling:

```go
resolveQuestionForSubmit(...)
findQuestionForNew(...)
lookupQuestionByName(...)
searchQuestionForAttachment(...)
```

when these functions duplicate matching logic.

Instead, centralize problem lookup/search.

The CLI layer should primarily:

1. parse command arguments
2. call the problem/search layer
3. handle interactive selection if necessary
4. invoke the requested operation

Do not move Cafe Grader HTTP implementation details into CLI commands.

Maintain the existing separation between:

```text
internal/cli
internal/graderapi
```

and use an appropriate existing package/location for registry/cache lookup logic.

Avoid introducing a new package unless it meaningfully improves ownership boundaries.

---

# 6. Tests

Every PR should include or update tests for its behavior.

## Question search tests

Cover cases such as:

```text
exact ID
exact question order
exact short name
exact full name
case-insensitive name
prefix
substring
tag
multiple matches
no cached match → refreshed match
no match after refresh
```

Network-dependent tests should continue using the existing test-server approach rather than Cafe Grader itself.

Do not require real credentials.

## Attachment tests

Cover:

```text
zero attachments
one attachment
multiple attachments
filenames preserved
file content preserved
one attachment download fails
```

A test with something resembling:

```text
main.cpp
helper.h
```

would represent the real-world case this patch is intended to fix.

## Submit tests

Cover:

```text
yoel submit file.cpp 1142
yoel submit file.cpp exact-name
yoel submit file.cpp partial-name
explicit question overrides inferred question
ambiguous question
unknown question
```

Also ensure existing one-argument submit behavior does not regress.

---

# 7. GitHub Development Workflow

Implement these changes using Issue → Branch → PR development.

Create separate issues for approximately:

```text
Improve question new search and problem resolution

Support multiple question attachments

Allow submit to explicitly target a question
```

Do not make one giant `v0.3.2` implementation PR.

Each issue should describe:

- current behavior
- desired user-visible behavior
- acceptance criteria
- relevant edge cases

Avoid prescribing implementation details unnecessarily.

## Branches

Use focused branch names such as:

```text
fix/question-new-search

fix/multiple-attachments

feat/explicit-submit-target
```

Exact names are not mandatory.

## Pull requests

Each issue should receive its own PR.

PR descriptions should reference the corresponding issue:

```text
Closes #<issue-number>
```

Keep each PR focused on the issue's scope.

If shared resolver work is required by several changes, implement it in the first relevant PR or isolate it cleanly without creating unnecessary dependency chains.

Avoid bundling unrelated formatting or refactoring changes.

---

# 8. CI Expectations

Before considering a PR complete, run:

```bash
go test ./...
go vet ./...
go build ./...
```

All must pass.

GitHub Actions should run the same basic checks for pull requests.

A failing CI result should block the change from being treated as complete.

Do not modify the release workflow unless required for v0.3.2.

---

# 9. Compatibility

This is a patch release.

Prefer backwards-compatible CLI behavior.

Existing valid commands should continue functioning unless their behavior is directly affected by a bug being fixed.

In particular:

```bash
yoel question new <id>
```

must continue to work.

Existing:

```bash
yoel submit <path>
```

must continue to work if it is currently supported.

The new syntax:

```bash
yoel submit <path> <question>
```

extends the command rather than replacing existing inference behavior.

---

# 10. Out of Scope

Do not use this patch as an excuse to perform the planned v0.4.0 rewrite/refactor.

Unless directly necessary for one of the three features, avoid:

- large package reorganizations
- replacing Cobra
- replacing Huh/Lip Gloss
- redesigning the whole cache format
- rewriting the HTTP client
- changing authentication
- changing the updater
- package-manager integration
- release-system redesign
- unrelated UI redesign
- broad naming cleanups

Small refactors required to share problem resolution are allowed.

---

# Definition of Done

v0.3.2 is ready when all three behaviors work:

```bash
yoel question new segment
```

can discover questions by meaningful cached/grader metadata,

questions containing:

```text
main.cpp
helper.h
```

can have all attachments handled correctly,

and:

```bash
yoel submit ./main.cpp segment-tree
```

can explicitly submit the given file to the selected question.

Additionally:

```text
go test ./...
go vet ./...
go build ./...
```

must pass.

Each feature should arrive through its own reviewed Issue/PR workflow.

The implementation should leave Yoel with **one shared concept of how a user refers to a question**, rather than several command-specific lookup systems.