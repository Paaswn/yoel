# Yoel Testcase Inspection Instructions

## Goal

Improve testcase inspection for non-correct submissions.

This work should solve two related problems:

1. For every testcase whose result is **not `CORRECT`**, Yoel should make the testcase input available for inspection.
2. Yoel must safely handle extremely large testcase input, expected output, and actual output without flooding or breaking the terminal.

Large testcase data may be large in either dimension:

- many rows, e.g. 20,000+ lines
- extremely wide rows, e.g. one line containing tens or hundreds of thousands of characters

Do not assume line count alone represents output size.

The terminal should provide a useful summary and preview by default, while allowing the user to inspect the complete raw data when needed.

---

# 1. Load Input for Every Non-Correct Testcase

A testcase should be treated as inspectable when its judge result is anything other than:

```text
CORRECT
```

Examples include:

```text
WRONG
RUNTIME ERROR
TIME LIMIT
MEMORY LIMIT
PARTIAL
```

and any other status returned by Cafe Grader that does not represent a fully correct testcase.

For every such testcase, Yoel should attempt to make its testcase input available.

Conceptually:

```text
judge result
    ↓
for each testcase
    ↓
status == CORRECT?
    ├─ yes → no testcase body required by default
    └─ no  → load/cache testcase input
```

Do not restrict input retrieval only to `WRONG`.

Input is useful for debugging runtime errors, timeout cases, memory errors, and other failures as well.

---

# 2. Do Not Automatically Dump Raw Testcase Data

Never blindly render full testcase bodies directly into the submission result screen.

This is unacceptable:

```text
Testcase 12 · WRONG

<20,000 lines of input>

<20,000 lines of expected output>

<20,000 lines of actual output>
```

Large raw output can:

- destroy terminal usability
- push useful submission metadata out of scrollback
- make interactive Bubble Tea layouts enormous
- cause rendering or cropping problems
- consume significant memory
- make comparison harder rather than easier

The default view should summarize testcase data instead.

Example:

```text
Testcase 12 · WRONG

Input       20,431 lines · 612 KB
Expected    20,000 lines · 584 KB
Actual      20,000 lines · 584 KB

First mismatch at line 13,482
```

Exact formatting may follow the existing Yoel UI style.

---

# 3. Measure Both Rows and Columns / Bytes

Do not classify testcase size using only line count.

Examples:

```text
20,000 lines × 20 chars
```

and:

```text
1 line × 2,000,000 chars
```

are both large but in very different ways.

At minimum track:

- total byte length
- line count
- maximum line width

Useful conceptual metadata:

```go
type TextStats struct {
    Bytes        int
    Lines        int
    MaxLineWidth int
}
```

The exact type and location are implementation details.

This metadata should allow Yoel to decide whether content is safe to render inline.

---

# 4. Default Rendering Policy

Use a size-aware rendering policy.

The exact thresholds are not strict requirements, but a reasonable initial policy is:

```text
Small
  <= 100 lines
  <= 32 KiB
  <= reasonable terminal width

Medium
  <= 1,000 lines
  <= 256 KiB

Large
  anything beyond these limits
```

Also classify content as effectively large if a single line is excessively wide.

For example:

```text
1 line
500 KiB
300,000 columns
```

must not be considered small merely because it contains one row.

Thresholds should preferably be constants so they can be tuned later.

---

# 5. Small Testcase Data

Small testcase content may be rendered directly.

Example:

```text
Input
─────
5
1 2 3 4 5

Expected
────────
15

Actual
──────
14
```

Even for small content, terminal width should still be respected.

Do not allow one extremely wide line to expand the layout uncontrollably.

---

# 6. Long Rows

Very wide lines require separate handling from large line counts.

Do not render a 50,000-character line directly into a normal Lip Gloss layout.

Long lines should be horizontally truncated for preview.

Example:

```text
Input
─────
1010101010011010101011010101010011010101…  [50,238 chars]
```

The preview should clearly communicate that truncation occurred.

Useful information may include:

```text
line 17 · 50,238 characters
```

Do not silently truncate content.

The complete line must remain accessible through raw inspection.

---

# 7. Long Columns / Many Lines

For large multi-line bodies, do not show only the first and last portions as the primary debugging mechanism.

This:

```text
first 100 lines
...
last 100 lines
```

may completely miss the actual failure.

For expected-vs-actual output, prefer a mismatch-centered context window.

Example:

```text
First mismatch: line 13,482

Expected                          Actual
13479 | 42                        42
13480 | 91                        91
13481 | 17                        17
13482 | 108                       107
13483 | 56                        56
13484 | 73                        73
```

Show only a small number of surrounding lines.

A reasonable initial context window is approximately:

```text
3–5 lines before mismatch
3–5 lines after mismatch
```

The exact number may be adjusted for terminal size.

---

# 8. First Mismatch Detection

When both expected and actual output are available, Yoel should attempt to identify the first meaningful mismatch.

Initially, line-based comparison is acceptable.

Conceptually:

```text
expected lines
actual lines
    ↓
find first unequal line
```

Future versions may implement token-aware or whitespace-aware comparisons, but this is not required for the first implementation.

The result should communicate:

- mismatch line
- expected content preview
- actual content preview

Example:

```text
Mismatch at line 13,482

Expected:
108

Actual:
107
```

If the mismatch occurs because one output ends earlier:

```text
Expected output continues after actual output ended at line 382.
```

or:

```text
Actual output contains additional data after expected output ended.
```

Do not report a misleading normal line mismatch.

---

# 9. Interactive Testcase Inspector

The interactive testcase result screen should remain compact.

A selected testcase may show something conceptually like:

```text
┌ Testcase 12 · WRONG ───────────────────────────┐
│ Time       12 ms                               │
│ Memory     1536 KB                             │
│                                                │
│ Input      20,431 lines · 612 KB               │
│ Expected   20,000 lines · 584 KB               │
│ Actual     20,000 lines · 584 KB               │
│                                                │
│ First mismatch: line 13,482                    │
│                                                │
│ [Input] [Expected] [Actual] [Diff] [Open]      │
└────────────────────────────────────────────────┘
```

Exact UI controls are flexible.

Do not try to place the entire testcase body inside a `huh.Note`, description field, or similar component.

Large text inspection should be separated from the compact testcase summary.

---

# 10. Opening Large Content in `$EDITOR`

For sufficiently large testcase bodies, opening the data in the user's editor is preferred over attempting to build a full text editor inside Yoel.

Use the user's configured editor when available.

Suggested precedence:

```text
$VISUAL
$EDITOR
```

If neither exists, use a reasonable existing Yoel fallback mechanism or display a useful message.

Do not hardcode Vim, Nano, VS Code, or another editor as the universal default unless Yoel already has an established cross-platform editor-opening helper.

Example UX:

```text
Input is too large for inline rendering.

20,431 lines · 612 KB

Press e to open in $EDITOR
```

or a command-level equivalent.

The exact interaction may depend on the existing Bubble Tea design.

---

# 11. Temporary Files for Editor Inspection

When opening testcase content externally, write the content to a real file.

Suggested conceptual structure:

```text
<yoel-cache>/tests/<submission>/<testcase>/
├── input.txt
├── expected.txt
└── actual.txt
```

Alternatively, temporary files may be used if they fit the existing cache architecture better.

Prefer deterministic cached files when useful because they allow the user to reopen the testcase without downloading or regenerating it repeatedly.

Use descriptive filenames.

Do not pipe giant testcase bodies directly through command-line arguments.

---

# 12. External Editor Behavior

Opening `$EDITOR` should not corrupt the active terminal UI.

If Yoel is running inside Bubble Tea, use the framework-supported mechanism for temporarily executing an external process rather than spawning an editor while Bubble Tea continues rendering underneath it.

Desired lifecycle:

```text
Yoel interactive UI
        ↓
suspend UI
        ↓
launch $EDITOR testcase-file
        ↓
editor exits
        ↓
resume Yoel UI
```

The UI should redraw correctly after returning.

Avoid hacks involving manually clearing the screen unless necessary.

---

# 13. Raw Inspection

Users must always have a way to access the complete testcase data.

Possible command design:

```bash
yoel test show 12 --input
yoel test show 12 --expected
yoel test show 12 --actual
yoel test show 12 --diff
```

or:

```bash
yoel test open 12 input
yoel test open 12 expected
yoel test open 12 actual
```

Exact CLI syntax is not mandated by this instruction.

The important requirement is:

> truncation in the normal UI must never make the original testcase inaccessible.

---

# 14. Optional Save Command

A future-friendly design may allow:

```bash
yoel test save 12
```

producing something like:

```text
testcase-12/
├── input.txt
├── expected.txt
└── actual.txt
```

This is useful for:

- opening files manually
- using `diff`
- replaying programs
- sharing testcases
- debugging giant cases

This command is optional for the first implementation if cached/editor access already provides the necessary raw data.

Do not expand scope unnecessarily.

---

# 15. Actual Program Output

If Cafe Grader provides the user's actual output, use it.

If Cafe Grader does not provide actual output, Yoel may obtain it from the local testcase replay mechanism.

Do not automatically rerun every failure without considering status.

Suggested behavior:

```text
WRONG
    → local replay is useful

RUNTIME ERROR
    → local replay may be useful

TIME LIMIT
    → do not run without strict timeout

MEMORY LIMIT
    → do not run without strict resource handling
```

Local execution must have appropriate timeout/output protections.

A broken program must not be allowed to generate unbounded output and consume all available memory.

---

# 16. Output Capture Limits

Local testcase replay must protect Yoel from programs producing enormous or infinite output.

Do not blindly accumulate stdout into an unbounded:

```go
bytes.Buffer
```

for arbitrary user programs.

Introduce an output capture limit.

Example conceptual policy:

```text
maximum captured output: configurable constant
```

When exceeded:

```text
Actual output exceeded Yoel's capture limit.
Previewing the first 4 MiB.
```

The exact initial limit may be chosen based on existing architecture.

The important requirement is bounded memory usage.

---

# 17. Input Fetching and Caching

For non-correct testcases:

```text
fetch testcase input
        ↓
store/cache it
        ↓
make it available to inspector/editor
```

Avoid fetching the same testcase repeatedly during one inspection session.

If Cafe Grader exposes expected output separately, expected output may be fetched lazily when the user opens or compares the testcase unless obtaining it eagerly is already cheap.

Possible policy:

```text
Non-correct result:
    input            → fetch/cache
    expected metadata → obtain if available
    expected body     → fetch when needed
    actual body       → use grader output or generate on replay
```

Do not add unnecessary network traffic for `CORRECT` cases.

---

# 18. Error Handling

Failure to fetch testcase input should not crash the entire submission result view.

Example:

```text
Testcase 12 · WRONG

Input:
Unable to fetch testcase input: HTTP 404
```

Other testcase metadata should still be displayed.

Similarly, if expected output is unavailable:

```text
Expected output unavailable.
```

Do not fabricate empty expected output.

Distinguish clearly between:

```text
empty output
```

and:

```text
output unavailable
```

---

# 19. Terminal Width Awareness

All preview rendering must respect terminal width.

Do not assume an 80-, 120-, or 200-column terminal.

Long lines should be truncated according to the actual available width.

Reserve space for:

- line numbers
- diff markers
- borders
- labels

Example narrow view:

```text
13482 | exp: 108
      | got: 107
```

may be better than a side-by-side diff when the terminal is narrow.

A wide terminal may use:

```text
Expected                 Actual
108                      107
```

Rendering strategy may adapt to available width.

---

# 20. Side-by-Side vs Vertical Diff

Do not force side-by-side diff rendering at all terminal widths.

Suggested rule:

```text
wide terminal
    → side-by-side comparison

narrow terminal
    → stacked expected / actual comparison
```

For example:

```text
Line 13482

Expected:
108

Actual:
107
```

This avoids turning two long columns into unreadable six-character panes.

---

# 21. Avoid Rendering Work Before Needed

Large testcase handling should avoid unnecessary expensive formatting.

Do not:

1. construct a giant Lip Gloss string
2. calculate styling for 20,000 lines
3. then truncate it

Instead:

```text
inspect raw size
    ↓
select preview range
    ↓
format only preview
```

This is particularly important for Bubble Tea rendering loops.

Rendering should operate on the currently visible or selected subset of data.

---

# 22. Tests

Add tests covering both vertical and horizontal size extremes.

At minimum:

## Status behavior

```text
CORRECT
    → no testcase input fetched automatically

WRONG
    → testcase input available

RUNTIME ERROR
    → testcase input available

TIME LIMIT
    → testcase input available
```

## Small content

Test normal inline rendering.

## Very many rows

Example:

```text
20,000 lines
```

Ensure Yoel does not render all lines into the normal testcase summary.

## Extremely wide row

Example:

```text
1 line × 100,000 characters
```

Ensure preview truncation works and raw data remains intact.

## Large rows and columns

Example:

```text
20,000 lines
each line 1,000 characters
```

The UI must remain bounded.

## Mismatch detection

Test:

```text
mismatch near beginning
mismatch near middle
mismatch near end
actual shorter than expected
actual longer than expected
```

## Empty output

Distinguish:

```text
expected = ""
actual = ""
```

from unavailable output.

## Editor opening

Editor-launch logic should be abstracted enough to test without actually opening Vim or another interactive editor during automated tests.

---

# 23. Performance Constraints

Large testcase handling must not make every Bubble Tea update proportional to the entire testcase size.

Avoid:

```text
O(total testcase size)
```

formatting on every render frame.

Expensive analysis such as:

- line splitting
- line width calculation
- mismatch detection

should be performed once when content is loaded where practical.

The render loop should consume already-prepared metadata and only format visible content.

---

# 24. Architecture Guidance

Keep these concerns separated:

```text
graderapi
    ↓
raw testcase retrieval

test/replay layer
    ↓
local execution
    ↓
expected / actual comparison

inspection model
    ↓
size metadata
    ↓
preview/mismatch selection

CLI / Bubble Tea
    ↓
render summary
    ↓
open editor
```

Do not put HTTP calls, process execution, diffing, and Lip Gloss rendering into one large command function.

However, avoid creating excessive packages solely for architectural purity.

Use existing package boundaries where they make sense.

---

# 25. Out of Scope

Do not turn this feature into a full terminal text editor.

Avoid implementing:

- arbitrary text editing
- syntax highlighting
- full Vim-like navigation
- custom file browser
- unlimited interactive diff viewer
- sophisticated Myers diff unless clearly required
- external pager/editor replacement

The operating system already provides excellent tools for inspecting huge text files.

Yoel's responsibility is to:

```text
summarize
preview
identify likely failure
provide complete raw data
open the user's preferred tool when appropriate
```

---

# Desired User Experience

For a normal failure:

```text
Testcase 4 · WRONG

Input       7 lines
Expected    1 line
Actual      1 line

Expected: 42
Actual:   41
```

For a massive failure:

```text
Testcase 12 · WRONG

Input       20,431 lines · 612 KB
Expected    20,000 lines · 584 KB
Actual      20,000 lines · 584 KB

First mismatch: line 13,482

13480 | 42                       42
13481 | 91                       91
13482 | 108                      107
13483 | 56                       56

Full testcase is too large for inline rendering.
Press e to open in $EDITOR.
```

For a testcase containing one absurdly wide line:

```text
Testcase 7 · WRONG

Input       1 line · 488 KB
Max width   499,821 chars

Preview:
01001010110101001010110101001010… [499,821 chars]

Press e to open the complete input in $EDITOR.
```

The user should get useful debugging information immediately without Yoel turning the terminal into a 20,000-line jumpscare.

---

# Definition of Done

This feature is complete when:

- every non-`CORRECT` testcase can expose its input
- correct testcases do not unnecessarily fetch large testcase bodies
- large row counts are handled safely
- extremely wide rows are handled safely
- normal UI never blindly renders arbitrarily large testcase data
- expected and actual output can show mismatch-centered context
- full testcase content remains accessible
- large content can be opened using `$VISUAL` / `$EDITOR`
- Bubble Tea resumes correctly after an external editor exits
- local program output capture is bounded
- tests cover both huge line counts and huge individual lines
- `go test ./...`
- `go vet ./...`
- `go build ./...`

all pass.