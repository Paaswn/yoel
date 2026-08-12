# Yoel Question Registry + Submit Resolution — Agent Instructions

## Context

Yoel already fetches and caches question metadata in commands such as:

```text
yoel question list
yoel question new
```

The implementation should take advantage of that existing flow.

The goal is **not** to build a manually maintained alias database.

The goal is to turn Yoel's existing question metadata/cache into a persistent question registry that knows both:

1. remote question identity/metadata;
2. local filesystem location/source file, when known.

Other commands, especially:

```text
yoel submit [key]
```

should resolve the user's intended source through this registry instead of recursively scanning the filesystem whenever possible.

The human developer has just finished fixing submission/question-result behavior. Avoid unrelated refactors in that area.

---

# Core Design

The registry is a persistent index of known questions.

Each question should have one canonical registry entry keyed by stable question ID.

Conceptually:

```go
type QuestionEntry struct {
    ID       string
    Name     string
    FullName string

    DirectoryPath string
    SourcePath    string
}
```

Exact field names/types should follow the existing grader API models and repository conventions.

Do not create duplicate canonical entries for different aliases.

For example:

```text
567
567.cpp
cpp_basics_1
"C++ Basics 1"
/home/user/cafe/cpp_basics_1/567.cpp
```

may all resolve to the same underlying question/source.

There should still be only one canonical question record.

---

# Registry Lifecycle

## `yoel question list`

This command already fetches question metadata such as:

- ID
- Name
- FullName
- other remote metadata already used by Yoel

Take this opportunity to refresh the registry.

For every fetched remote question:

- create the registry entry if it does not exist;
- refresh remote metadata if it does exist;
- preserve local fields such as `DirectoryPath` and `SourcePath`.

Conceptually:

```go
entry := registry[id]

entry.ID = remote.ID
entry.Name = remote.Name
entry.FullName = remote.FullName

registry[id] = entry
```

Do not erase known local filesystem information when refreshing remote metadata.

## `yoel question new`

This command knows more than `question list`.

When Yoel creates/downloads a new local question, it knows:

- the remote question ID;
- question name/full name;
- the actual local directory it created;
- the source file it created;
- possibly attachment/PDF locations.

After successfully creating the question, update the corresponding registry entry with the real local paths.

Conceptually:

```go
entry.DirectoryPath = createdQuestionDirectory
entry.SourcePath = createdSourceFile
```

This is the preferred moment to bind remote question identity to local filesystem state.

---

# Why This Exists

The old approach may recursively search directories for a matching source file.

That has several problems:

- unnecessary filesystem traversal;
- ambiguity when multiple files match;
- slower commands;
- harder-to-predict behavior;
- Yoel is rediscovering information it already knew earlier.

The registry should make resolution deterministic and cheap.

Preferred model:

```text
question list
    ↓
discover/refresh remote metadata

question new
    ↓
bind question to local directory + source

submit/test/etc.
    ↓
resolve from registry
```

---

# Persistence

Persist the registry independently from generated testcase/binary cache data.

Recommended conceptual layout:

```text
<user-config-dir>/yoel/
├── config.toml
└── questions.json
```

Use `os.UserConfigDir()` or equivalent platform-safe behavior.

Example Linux path:

```text
~/.config/yoel/questions.json
```

Do not hardcode Linux paths.

The registry is generated persistent state, not really user configuration.

Therefore:

- Viper may still be used for `config.toml`;
- do **not** force the question registry into Viper unless there is a compelling implementation reason;
- a simple typed JSON registry is preferred for this automatically managed data.

Recommended:

```go
type Registry map[string]QuestionEntry
```

where the map key is the canonical question ID.

---

# Registry Package

Create a clear package responsible for registry persistence and resolution.

Possible structure:

```text
internal/registry/
├── registry.go
├── resolve.go
└── registry_test.go
```

or similar.

Do not over-split if two files are enough.

The registry package should own:

- loading;
- saving;
- metadata merging;
- local path updates;
- lookup/resolution.

It should not own:

- CLI rendering;
- Cafe API requests;
- compilation;
- testcase replay;
- submission rendering.

---

# Submit Interface

The intended user-facing command is:

```text
yoel submit [key]
```

The key may represent several forms.

Supported lookup forms:

1. file name
2. directory path
3. question ID
4. question name
5. question full name
6. explicit file path

Examples:

```sh
yoel submit 567.cpp
yoel submit cpp_basics_1
yoel submit idk_name
yoel submit 567
yoel submit "C++ Basics 1"
yoel submit ./cpp_basics_1/567.cpp
```

The resolver should infer which registry entry/source the user means.

---

# Lookup Semantics

Resolution must be deterministic and explicit about ambiguity.

Recommended resolution strategy:

## 1. Explicit file path

If `[key]` directly identifies an existing file path, use it immediately.

Examples:

```text
./567.cpp
/home/user/cafe/cpp_basics_1/567.cpp
C:\Users\foo\questions\567.cpp
```

An explicit valid file path should bypass registry searching.

This is the strongest signal of user intent.

Do not force the user to register a source before submitting an explicit path.

## 2. Exact registered source filename

Try matching the basename of known `SourcePath` values.

Example:

```text
567.cpp
```

matching:

```text
/home/user/cafe/cpp_basics_1/567.cpp
```

If exactly one entry matches, resolve it.

If multiple entries have the same filename, return an ambiguity error rather than guessing.

## 3. Directory path / directory identity

If the key identifies an existing directory, or matches a known registered `DirectoryPath`, resolve the corresponding question.

Also consider directory basename matching if it fits current Yoel behavior.

Example:

```text
cpp_basics_1
```

may resolve:

```text
/home/user/cafe/cpp_basics_1
```

Do not silently select among multiple matching directory basenames.

## 4. Exact question ID

Try exact canonical question ID lookup.

Example:

```text
567
```

This should be fast and direct:

```go
entry, ok := registry["567"]
```

Question ID is the canonical registry identity.

## 5. Exact question Name

Try exact match against the cached remote question `Name`.

Example:

```text
cpp_basics_1
```

If exactly one question matches, resolve it.

## 6. Exact question FullName

Try exact match against the cached remote `FullName`.

Example:

```text
C++ Basics 1
```

If exactly one question matches, resolve it.

---

# Ambiguity

Never silently choose one entry when multiple registry records match the same key.

Example:

```text
main.cpp
```

could exist for several questions.

Return a useful ambiguity error.

Conceptual output:

```text
"main.cpp" matches multiple questions:

  567  cpp_basics_1    /home/user/cafe/cpp_basics_1/main.cpp
  891  vector_intro    /home/user/cafe/vector_intro/main.cpp

Use a question ID or explicit file path.
```

Exact formatting should follow the existing CLI style.

The important requirement is:

> ambiguity is an error, not a guess.

---

# Missing Local Source

A registry entry may exist from `yoel question list` but may not yet have a local source.

Example:

```json
{
  "567": {
    "id": "567",
    "name": "cpp_basics_1",
    "full_name": "C++ Basics 1",
    "directory_path": "",
    "source_path": ""
  }
}
```

If `yoel submit 567` resolves the remote question but no local source is known:

- return a useful error;
- do not recursively search the whole filesystem by default;
- suggest the relevant workflow, e.g. creating/downloading the question first or passing an explicit file path.

Example conceptually:

```text
Question 567 is known, but no local source is registered.

Run:
  yoel question new 567

or submit an explicit file:
  yoel submit ./path/to/567.cpp
```

Avoid silent magical fallback unless the existing CLI contract intentionally requires it.

---

# Stale Local Paths

A registry entry may point to a file or directory that no longer exists.

On resolution:

- validate the path;
- return a clear stale-entry error;
- identify the question/key;
- show the stale path;
- do not silently recurse elsewhere.

---

# Matching Rules

Prefer exact matches first.

Do not introduce fuzzy matching in the first implementation.

Avoid:

- substring guesses;
- edit-distance guesses;
- "closest" question name.

Exact matching keeps `yoel submit [key]` predictable.

---

# Paths

Store normalized paths.

Use standard library helpers:

```go
filepath.Abs
filepath.Clean
filepath.Base
```

Do not concatenate path separators manually.

Support Windows correctly.

A path may point through a symlink.

Do not require resolving symlinks unless necessary.

---

# Registry Update Safety

Registry writes should be safe against partial corruption.

Preferred behavior:

```text
serialize
→ write temp file
→ close
→ rename over registry file
```

If the repository already has a safe persistence helper, reuse it.

Do not introduce a database.

---

# Metadata Merge Behavior

Remote refreshes and local-path updates are different operations.

## Remote refresh

May update:

```text
ID
Name
FullName
other remote metadata
```

Must preserve:

```text
DirectoryPath
SourcePath
other local-only state
```

## Local question creation

May update:

```text
DirectoryPath
SourcePath
```

and should also ensure remote metadata is present/current.

Create helper functions that make this behavior obvious rather than mutating maps ad hoc throughout CLI code.

For example:

```go
registry.UpsertRemote(problem)
registry.BindLocal(id, directory, source)
```

Exact names are flexible.

---

# Source Path Should Represent the Real File

Do not infer a source path forever from a naming rule such as:

```text
<id>.cpp
```

If `question new` creates the file, store the exact path it created.

If later Yoel supports configurable filenames, the registry should continue working without resolver changes.

The registry exists specifically to remember real local state.

---

# Integration With Existing Source Discovery

Inspect the current `submit_source.go` or equivalent source-resolution logic.

Do not blindly delete working behavior before understanding it.

Refactor toward:

```text
explicit path / registry resolver
```

and remove or minimize recursive searching where the registry makes it redundant.

If legacy discovery must remain temporarily for compatibility:

- keep it clearly isolated;
- place it after deterministic resolution;
- do not let it override registry results;
- add tests documenting when it is used.

The long-term preferred model is no broad recursive scan for ordinary registered questions.

---

# Suggested Resolver API

Conceptual only:

```go
type ResolveResult struct {
    Entry      QuestionEntry
    SourcePath string
}
```

```go
func (r *Registry) Resolve(key string) (ResolveResult, error)
```

The CLI should not need to know all matching internals.

Possible resolver flow:

```text
existing file path?
    yes → use file

registered filename?
    yes → unique result

directory?
    yes → unique result

question ID?
    yes → result

question Name?
    yes → unique result

question FullName?
    yes → unique result

otherwise:
    not found
```

Keep lookup rules testable independently from Cobra.

---

# Explicit File Path and Registry Association

For the first implementation, an explicit file path should simply bypass registry resolution.

Example:

```sh
yoel submit /tmp/foo.cpp
```

should submit `/tmp/foo.cpp` even if it is not registered.

Do not automatically create a registry entry from every arbitrary explicit file path unless product behavior explicitly calls for it.

The registry should primarily be populated by Yoel's question workflows.

---

# `question list` Performance

`yoel question list` may fetch many questions.

Do not perform a full registry disk write once per question.

Preferred:

```text
fetch remote question list
↓
load registry once
↓
merge all metadata in memory
↓
save registry once
```

Likewise, avoid unnecessary filesystem stat calls for every registry entry unless required.

---

# `question new` Failure Behavior

Only bind local paths after the local question creation succeeds enough that those paths are valid.

Do not persist a source path before the file actually exists.

The registry should reflect real local state.

---

# Tests

Add focused tests for registry behavior.

## Registry persistence

- missing registry file returns empty registry;
- valid registry loads;
- malformed registry errors clearly;
- saving then loading preserves entries;
- save creates parent config directory.

## Remote metadata refresh

- new remote question creates entry;
- existing remote metadata updates;
- local `DirectoryPath` survives refresh;
- local `SourcePath` survives refresh.

## Local binding

- binding directory/source updates correct ID;
- binding preserves remote metadata;
- stored paths are normalized.

## Resolver: explicit path

- existing explicit source file bypasses registry;
- nonexistent path does not falsely resolve as explicit file.

## Resolver: filename

- unique filename resolves;
- duplicate filename returns ambiguity.

## Resolver: directory

- unique directory resolves;
- duplicate directory basename returns ambiguity if basename matching is supported.

## Resolver: ID

- exact ID resolves directly.

## Resolver: Name

- unique Name resolves;
- duplicate Name returns ambiguity.

## Resolver: FullName

- unique FullName resolves;
- duplicate FullName returns ambiguity.

## Stale entries

- known entry with missing source returns stale-path error.

## Missing local binding

- remote-only entry resolves identity but submission returns a clear "no local source" error.

## CLI integration

- `yoel submit 567.cpp`
- `yoel submit cpp_basics_1`
- `yoel submit 567`
- `yoel submit "C++ Basics 1"`
- `yoel submit ./path/to/source.cpp`

All should route to the correct source under their valid conditions.

Use temp directories in resolver tests; do not depend on the real user's filesystem.

---

# Non-Goals

Do not expand this task into:

- fuzzy search;
- interactive ambiguity picker;
- filesystem database/indexing;
- SQLite;
- automatic project discovery;
- generic manual alias management;
- Viper-backed question registry unless truly justified;
- testcase cache redesign;
- local replay output redesign;
- submission-result UI rewrite;
- question-result rendering refactor;
- authentication/session redesign.

Stay focused.

---

# Guardrails Around Existing Submission/Replay Work

The human developer has just fixed question/submission result behavior and is actively evolving local replay features.

Avoid touching:

- submission result visual layout;
- testcase result rendering;
- local replay output formatting;
- `huh` UI behavior;
- `lipgloss` styling;
- replay stdout/stderr presentation;
- runner execution semantics unrelated to source resolution.

If source-resolution integration requires editing `submit.go`:

- make the smallest change possible;
- feed it the resolved source path;
- leave result handling/rendering untouched.

Do not opportunistically clean up adjacent code.

---

# Expected User Experience

The user should be able to write:

```sh
yoel submit 567.cpp
```

and Yoel resolves the known registered source filename.

They should also be able to write:

```sh
yoel submit cpp_basics_1
```

and resolve using directory/question name.

Or:

```sh
yoel submit 567
```

and resolve using the canonical question ID.

Or:

```sh
yoel submit "C++ Basics 1"
```

and resolve using the full question name.

Or:

```sh
yoel submit ./somewhere/567.cpp
```

and Yoel should simply use that explicit file path without requiring registry lookup.

The important property is:

> users can identify the question/source using the information naturally available to them, while Yoel resolves it deterministically from knowledge it already cached.

---

# Expected Architecture

Conceptually:

```text
Cafe Grader API
       │
       ▼
yoel question list
       │
       └── refresh remote registry metadata

yoel question new
       │
       └── bind registry entry to real local paths

<user-config-dir>/yoel/questions.json
       │
       ▼
Question Registry
       │
       ▼
Resolver
       │
       ▼
yoel submit [key]
       │
       ▼
resolved source path
       │
       ▼
existing submit pipeline
```

This should simplify Yoel.

The registry is not extra ceremony: it eliminates repeated discovery work by preserving information Yoel already had.
