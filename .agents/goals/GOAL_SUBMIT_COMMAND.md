# Goal: implement `yoel submit [file]`

Read `AGENTS.md` and `GOAL_GRADER_API_PACKAGE.md` completely before doing
anything. This is a narrow submission checkpoint. Do not build a TUI, add
interactive prompts, or redesign unrelated commands.

## User-facing behavior

The command should eventually be:

```text
yoel submit 673.cpp
```

The submitted source filename follows this convention:

```text
<problem-id>.<extension>
```

Examples:

```text
673.cpp   -> problem ID 673
1142.py   -> problem ID 1142
900.rs    -> problem ID 900
```

The CLI owns the command and file handling. It should:

1. Require exactly one source-file argument.
2. Use `filepath.Base` when passing the filename to the API; directories must
   not become part of the submitted filename.
3. Extract the problem ID from the basename by requiring decimal digits before
   the first extension separator. Reject names that do not match the expected
   form instead of guessing.
4. Read the source file with `os.ReadFile`.
5. Pass the numeric problem ID, source text, and original basename to the API
   package.
6. Leave result printing and any future polling policy to the CLI owner.

Examples that must be rejected:

```text
solution.cpp
abc.cpp
673
673.extra.cpp
0.cpp
-1.cpp
```

For this checkpoint, do not add a `--problem` flag. The problem ID comes from
the filename. Do not require a `--language` flag: the grader can detect the
language from the filename extension. A later feature may add an explicit
language override, but it is not part of this task.

## Verified Cafe Grader API contract

The deployed OpenAPI document and server source define this request:

```http
POST /api/v1/problems/{problem_id}/submissions
Authorization: Bearer <token>
Accept: application/json
Content-Type: application/json
```

For `yoel submit 673.cpp`, the JSON body is:

```json
{
  "source": "<contents of 673.cpp>",
  "filename": "673.cpp"
}
```

`language_id` is optional. When it is omitted, the server uses the filename
extension to detect the language. If the API package exposes an integer
`LanguageID` field, make zero mean “not supplied” and omit the JSON field with
`omitempty`; do not send `language_id: 0`.

The successful response is HTTP `201 Created`:

```json
{
  "id": 924618,
  "number": 3,
  "status": "submitted"
}
```

This is an acknowledgement that a submission was created and queued. It is not
the final score. The API package may expose `GetSubmission`, but it must make
one request only. The CLI owner decides whether to poll, how often, and when
to stop.

The server source's language-selection order is relevant: an explicit
`language_id` is preferred, then the filename extension, then a sole permitted
language, then the server's C++ fallback. Therefore preserving the basename
and extension is important.

## API-package boundary

If `Submit` is missing or still uses the abandoned cookie/CSRF/multipart/HTML
flow, implement the typed JSON API method only:

```go
type SubmissionRequest struct {
	Source     string
	Filename   string
	LanguageID int // zero means omit; the server detects from Filename
}

func (c *Client) Submit(
	ctx context.Context,
	problemID int,
	req SubmissionRequest,
) (Submission, error)
```

The API package must:

- send the exact JSON request above;
- use the existing `Client` base URL, injected HTTP client, and bearer token;
- reject a non-positive problem ID before making a request;
- reject empty source before making a request;
- preserve Unicode and blank lines in source code;
- check HTTP status before decoding the success response;
- decode the required `id`, `number`, and `status` fields;
- bound and close response bodies;
- never automatically retry this POST, because a retry may create a duplicate
  submission;
- wrap errors with an operation such as `submit: ...` without exposing the
  bearer token, password, or complete source code.

Do not make the API package read files, parse filenames, parse arguments, print,
prompt, save tokens, choose cache paths, or poll.

## CLI integration boundary

The CLI layer may contain small helpers such as:

```go
func problemIDFromFilename(name string) (int, error)
```

That helper belongs outside `internal/graderapi` because filename conventions
are product behavior, not HTTP protocol behavior.

The intended flow is:

```text
argument "673.cpp"
        -> basename "673.cpp"
        -> problem ID 673
        -> os.ReadFile("673.cpp")
        -> client.Submit(ctx, 673, SubmissionRequest{
             Source: source,
             Filename: "673.cpp",
           })
        -> CLI displays acknowledgement
        -> optional later GetSubmission calls owned by CLI
```

Do not open a PDF, list problems, resolve names, or add polling in this
checkpoint.

## Tests

Use `httptest.Server`; never contact the real grader or create a real
submission. Tests must cover:

1. `POST` and the exact `/api/v1/problems/<id>/submissions` path;
2. bearer authentication and JSON content headers;
3. JSON body containing source and basename;
4. omission of `language_id` when it is zero;
5. preservation of Unicode and blank lines;
6. decoding the `201` acknowledgement;
7. invalid problem ID and empty source without a server request;
8. `401`/`403` authentication behavior and representative non-2xx behavior;
9. malformed or oversized responses;
10. context cancellation or timeout where practical; and
11. fake tokens and fake source code absent from returned error strings.

Also add focused CLI tests for filename parsing if the command layer is part of
the approved checkpoint. These tests should cover valid `id.ext` names,
directories, non-numeric IDs, missing extensions, extra dots, and non-positive
IDs. They must not invoke the real grader.

## Workflow constraint

Before editing files, propose the checkpoint in plain language. Explain:

1. the exact function and types to add or change;
2. the exact HTTP request and response;
3. the filename-to-problem-ID rule;
4. files that will change; and
5. important validation and error tradeoffs.

Wait for approval before implementation. After implementation, explain the
request path and show the relevant fake-server test. Stop after this submit
checkpoint; do not continue into TUI work or unrelated API features.

## Sources verified for this instruction

- Deployed contract: `https://grader.nattee.net/api-docs/v1/swagger.yaml`
- Server implementation:
  `https://github.com/cafe-grader-team/cafe-grader-web/blob/master/app/controllers/api/v1/submissions_controller.rb`

