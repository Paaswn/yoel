# Goal: Build the grader web-functions package

## What the owner actually wants

Build a small Go package containing useful functions that talk to the Cafe
Grader JSON API. The owner will write the executable around it.

The package is analogous to a tiny typed SDK:

```text
owner's CLI code
    -> graderapi function
    -> HTTP + JSON
    -> Cafe Grader
    -> typed Go result or error
    -> owner's CLI code decides what happens next
```

This goal does not include command parsing, flags, prompts, terminal output,
configuration files, token files, a TUI, browser automation, HTML parsing,
cookies, CSRF, multipart forms, or legacy compatibility.

## Completion result

The owner should be able to write code resembling:

```go
httpClient := &http.Client{Timeout: 15 * time.Second}

publicClient, err := graderapi.NewClient(
    "https://grader.nattee.net",
    httpClient,
)
if err != nil {
    // The owner decides how the CLI reports this.
}

session, err := publicClient.Login(ctx, username, password)
if err != nil {
    // The owner decides whether to ask again or exit.
}

client := publicClient.WithToken(session.Token)

problems, err := client.ListProblems(ctx)
submission, err := client.Submit(ctx, 676, graderapi.SubmissionRequest{
    Source:     source,
    Filename:   "main.cpp",
    LanguageID: 5,
})
result, err := client.GetSubmission(ctx, submission.ID)
```

This example shows ownership boundaries; exact fields must match the verified
deployed OpenAPI schema.

## Proposed package

Start with:

```text
internal/graderapi/
    client.go       # Client construction and shared HTTP behavior
    errors.go       # Small protocol-level error vocabulary
    auth.go         # Login
    languages.go    # Language list
    problems.go     # Problem list, detail, and description
    submissions.go  # Submit, retrieve result, and history
    *_test.go       # httptest.Server coverage beside each feature
testdata/api/       # sanitized JSON only if fixtures are large enough to help
```

Do not create every file immediately. Add files only as their checkpoint is
implemented. Keeping two related endpoints in one readable file is acceptable.

## Client model

Prefer an immutable-feeling client value with unexported fields:

```go
type Client struct {
    baseURL   *url.URL
    httpClient *http.Client
    token     string
}

func NewClient(baseURL string, httpClient *http.Client) (*Client, error)
func (c *Client) WithToken(token string) *Client
```

Required behavior:

- `NewClient` validates and parses the base URL.
- It rejects missing hosts and unsafe production schemes.
- It accepts an injected local HTTP server for tests.
- A nil `httpClient` may receive a small safe default, but this choice must be
  explained to the owner before implementation.
- `WithToken` returns a copy rather than unexpectedly mutating the original.
- The token remains unexported inside `Client` and never appears in an error.
- `Client` owns no command state and performs no disk I/O.

Do not add functional options merely to configure two values.

## Endpoint inventory

Verify each entry against the deployed Swagger document immediately before its
checkpoint. These are the currently expected operations:

| Function | HTTP request | Authentication |
| --- | --- | --- |
| `Login` | `POST /api/v1/auth/login` | No bearer token |
| `ListLanguages` | `GET /api/v1/languages` | Bearer token |
| `ListProblems` | `GET /api/v1/problems` | Bearer token |
| `GetProblem` | `GET /api/v1/problems/{id}` | Bearer token |
| `GetDescription` | `GET /api/v1/problems/{id}/description` | Bearer token |
| `Submit` | `POST /api/v1/problems/{id}/submissions` | Bearer token |
| `GetSubmission` | `GET /api/v1/submissions/{id}` | Bearer token |
| `ListSubmissions` | `GET /api/v1/problems/{id}/submissions` | Bearer token |

If any path, field, or authentication rule differs in the live contract, stop
and report the difference before coding.

## Checkpoint 1: Client foundation

Implement only `Client`, `NewClient`, `WithToken`, and the minimal shared code
needed to send a request safely.

Shared code may handle:

- resolving a relative API path;
- creating a request with the supplied context;
- applying common JSON and bearer headers;
- sending with the injected HTTP client;
- bounding and reading a response body;
- classifying non-success status codes.

Do not build a generic public `Request[T]` system. Endpoint files should still
show their method, path, request type, and response type clearly. A small
unexported helper is acceptable when it removes mechanical duplication without
hiding protocol behavior.

Tests must cover invalid base URLs, token copying, request cancellation, status
handling, and cross-origin redirect credential safety.

Stop and explain the finished request lifecycle to the owner.

## Checkpoint 2: Login

Expected request:

```http
POST /api/v1/auth/login
Accept: application/json
Content-Type: application/json

{
  "login": "student-login",
  "password": "student-password"
}
```

Expected conceptual types:

```go
type Session struct {
    Token     string
    ExpiresAt time.Time
    User      User
}

type User struct {
    ID       int
    Login    string
    FullName string
}

func (c *Client) Login(
    ctx context.Context,
    login string,
    password string,
) (Session, error)
```

Add the exact JSON tags and types from the deployed schema. Validate obviously
invalid empty inputs before sending. Validate that a success response contains
the fields required for future authenticated calls, especially a non-empty
token.

Login must not:

- send an existing bearer token;
- store the password or token;
- print anything;
- return the password inside a value or error;
- expose the raw response body in an error.

Fake-server tests must inspect the JSON body and prove that neither fake
password nor returned fake token appears in failure messages.

Stop after tests pass. Show the owner how JSON encoding, POST, status checking,
and JSON decoding connect.

## Checkpoint 3: Languages and problems

Implement:

```go
func (c *Client) ListLanguages(ctx context.Context) ([]Language, error)
func (c *Client) ListProblems(ctx context.Context) ([]Problem, error)
func (c *Client) GetProblem(ctx context.Context, problemID int) (Problem, error)
func (c *Client) GetDescription(ctx context.Context, problemID int) (Description, error)
```

Use the deployed schemas. Keep API wire structs and useful domain types simple;
do not invent fields from the old HTML page. Preserve fields that the owner is
likely to need for selection and display, such as verified identifiers, names,
language information, scores, attempts, tags, and description format.

For IDs, reject values less than or equal to zero without sending a request.
Use `strconv.Itoa` or equivalent path construction; do not accept raw path
fragments from the caller.

Tests must prove the exact paths, bearer header, decoding, bad-ID behavior,
unauthorized behavior, and malformed response behavior.

Stop and let the owner write a tiny experimental `main` that logs in or uses a
fake token and prints one problem. Do not write that command for them unless
they explicitly ask.

## Checkpoint 4: Submit source

Implement:

```go
type SubmissionRequest struct {
    Source     string
    Filename   string
    LanguageID int
}

func (c *Client) Submit(
    ctx context.Context,
    problemID int,
    req SubmissionRequest,
) (Submission, error)
```

Expected request, subject to deployed-schema verification:

```http
POST /api/v1/problems/676/submissions
Authorization: Bearer <token>
Accept: application/json
Content-Type: application/json

{
  "source": "#include <iostream>\n...",
  "filename": "main.cpp",
  "language_id": 5
}
```

The function receives source text; it does not read a file. The owner's CLI is
responsible for `os.ReadFile` and choosing the filename and language.

Reject invalid IDs and an empty source before sending. Do not log source code.
Do not retry POST automatically: a retry could create a duplicate submission.

Tests must verify Unicode and blank lines survive JSON encoding, the exact
problem path, the language ID, bearer authentication, success decoding, and
that transport/status errors do not expose source code or token values.

Stop after the owner can explain why the returned submission is usually only
an acknowledgement, not the final score.

## Checkpoint 5: Submission result and history

Implement:

```go
func (c *Client) GetSubmission(
    ctx context.Context,
    submissionID int,
) (Submission, error)

func (c *Client) ListSubmissions(
    ctx context.Context,
    problemID int,
) ([]Submission, error)
```

Model the verified status, score, grader comment, compiler message, timestamps,
runtime, memory, evaluations, and attempt number only where they exist in the
deployed response.

Some fields may be absent while judging is in progress. Use pointer fields or
other explicit optionality when the wire contract distinguishes missing from a
real zero value. Do not turn unknown statuses into `accepted` and do not claim
that a submission is complete merely because HTTP returned `200`.

These methods perform one GET each. They do not sleep or poll. The owner decides
whether to call `GetSubmission` repeatedly, how often, and which statuses are
terminal.

Tests must include at least one in-progress response and one completed response.

## Error contract

Keep errors useful to caller code without building a taxonomy maze. Suggested
sentinels or types:

```go
var ErrAuthentication = errors.New("grader authentication failed")
var ErrInvalidInput = errors.New("invalid grader API input")
var ErrInvalidResponse = errors.New("invalid grader API response")

type HTTPError struct {
    StatusCode int
    Status     string
}
```

The owner should be able to use `errors.Is`/`errors.As` to distinguish:

- invalid arguments they supplied;
- expired or rejected authentication (`401`/`403`);
- a non-authentication HTTP error;
- transport or context cancellation;
- malformed/oversized/unexpected JSON.

Do not return server bodies by default. If a small sanitized API error message
is later useful, propose an explicit redaction and size policy first.

## Definition of done

The package is complete for this goal when:

- all eight endpoint methods match the verified deployed contract;
- every endpoint has fake-server success and failure coverage;
- no test touches the real grader;
- no function reads arguments, prompts, prints, exits, stores tokens, reads
  source files, or polls;
- tokens, passwords, and source code cannot appear in ordinary errors;
- `gofmt`, `go test ./...`, and `go vet ./...` pass;
- the owner has reviewed each checkpoint and can describe the request flow;
- no TUI, browser, HTML parser, cookie jar, generic SDK generator, or additional
  production dependency has been introduced.

## Prompt for Luna or Terra

Use this exact starting prompt:

> Read `AGENTS.md` and `GOAL_GRADER_API_PACKAGE.md` completely. This is a fresh
> project; do not preserve or reuse the old cookie/CSRF/HTML implementation.
> Propose Checkpoint 1 only. Explain the public types, request lifecycle, files,
> errors, and tradeoffs in plain language. Do not implement anything until I
> approve the proposal.
