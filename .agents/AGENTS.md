# AGENTS.md

## Current workspace status

The executable is `cmd/yoel` and builds as `yoel` (`yoel.exe` on Windows). Its
module path is `yoel`; Cobra commands live in `internal/cli`, while
`internal/graderapi` remains the isolated HTTP/JSON boundary for Cafe Grader.

The CLI currently owns login/session persistence, question cache and PDF
downloads, source submission and result polling, and the `user` command. Do
not move those product decisions into `internal/graderapi`.

`internal/registry` stores Yoel's generated question index separately at
`<user-config-dir>/yoel/questions.json`. It is keyed by stable grader problem
ID and preserves remote name metadata plus real local question/source paths.
Question list refreshes remote records; successful question creation binds its
local paths. Submit resolves explicit files first, then exact registry keys
(source filename, directory, ID, name, full name), and finally reports when it
uses legacy recursive discovery. Do not move registry data into session storage,
Viper config, or the `.yoel/` local replay cache.

Interactive `yoel submit` results can locally replay eligible failed C++
testcases. `internal/graderapi` only downloads the authenticated raw testcase
input and expected output; `internal/runner` owns `g++` compilation, bounded
concurrent execution, timeouts, output capture, and approximate comparison.
The CLI coordinates those pieces through Bubble Tea messages so the Huh result
selector remains responsive. Remote Cafe Grader verdicts always remain
authoritative. Non-interactive submissions never execute local programs.

Local replay supports `.cpp` with `g++ -std=c++17 -O2 -pipe`. It compiles once,
runs at most `min(runtime.NumCPU(), 4)` cases concurrently, limits each case to
five seconds and 256 KiB per output stream, and caches source-specific binaries
and testcase artifacts under the question directory's ignored `.yoel/`
directory. Do not broaden language support, checker claims, limits, cache
layout, or automatic replay eligibility without owner review.

Release builds inject `main.version` from a `vMAJOR.MINOR.PATCH` tag via
`.github/workflows/release.yml`. Local builds use `dev`. The official release
source is `Paaswn/yoel`; archives are named `yoel_<version>_<os>_<arch>` and
are installed/updated only through `scripts/install.sh` or `scripts/install.ps1`.
`yoel update` reuses those installers after explicit confirmation. It must not
silently update ordinary commands or overwrite package-manager-managed installs.

After an interactive completed submission, a CLI-only update notice may check
GitHub Releases at most once per 24 hours. Its timestamp is a non-secret user
cache entry and `YOEL_NO_UPDATE_CHECK=1` disables it. Failed checks are ignored
and must not affect submission results.

## Project

This repository is a learning-focused Go client for the Cafe Grader JSON API.
The project owner writes and owns the executable: command names, argument
parsing, prompts, terminal output, token storage, polling policy, and product
flow.

The agent's job is narrower: build an understandable Go package that talks to
the grader over HTTP and exposes typed functions the owner's CLI can call.

The previous cookie/CSRF/multipart/Turbo-HTML implementation is abandoned. Do
not preserve, migrate, wrap, or copy it. Files remaining from that version are
stale workspace residue, not a compatibility requirement.

## Human ownership and AI boundary

The owner decides:

- command and flag syntax;
- what gets printed and how errors are worded for users;
- where and whether an API token is stored;
- when results are polled and when polling stops;
- which features are exposed by the CLI;
- any change to the public Go API described below.

The agent may implement:

- HTTP request creation and sending;
- JSON request and response types;
- API authentication headers;
- bounded response decoding;
- protocol-level error types;
- fake-server tests for the API package.

The API package must never parse `os.Args`, call `flag`, prompt on stdin, print,
call `os.Exit`, choose a token path, save credentials, or run an endless polling
loop.

For every meaningful checkpoint, first explain:

1. the functions and types to be added;
2. the exact HTTP request and response involved;
3. files that will change;
4. important error behavior and tradeoffs.

Wait for owner approval before implementation. After implementation, explain
the request path in plain language and show the relevant test.

## Source of truth

The deployed API contract is:

```text
https://grader.nattee.net/api-docs/v1/swagger.yaml
```

The server source is useful for understanding implementation details:

```text
https://github.com/nattee/cafe-grader-web
```

The deployed OpenAPI document wins when it differs from repository `master`.
Before implementing an endpoint, record a sanitized request/response fixture
or the relevant schema from the deployed contract. If the live contract cannot
be verified or differs from this repository's goal document, stop and show the
owner the mismatch. Do not guess JSON fields.

Never use real credentials in tests. Automated tests must not contact the real
grader or create real submissions.

## Intended package boundary

Use one ordinary package, tentatively `internal/graderapi`. It owns the wire
protocol but not application behavior.

The intended public surface is:

```go
func NewClient(baseURL string, httpClient *http.Client) (*Client, error)
func (c *Client) WithToken(token string) *Client

func (c *Client) Login(
    ctx context.Context,
    login string,
    password string,
) (Session, error)

func (c *Client) ListLanguages(ctx context.Context) ([]Language, error)
func (c *Client) ListProblems(ctx context.Context) ([]Problem, error)
func (c *Client) GetProblem(ctx context.Context, problemID int) (Problem, error)
func (c *Client) GetDescription(ctx context.Context, problemID int) (Description, error)
func (c *Client) Submit(ctx context.Context, problemID int, req SubmissionRequest) (Submission, error)
func (c *Client) GetSubmission(ctx context.Context, submissionID int) (Submission, error)
func (c *Client) ListSubmissions(ctx context.Context, problemID int) ([]Submission, error)
```

Names and exact result shapes may change only to match the verified deployed
contract and after explaining the change to the owner.

`Login` returns a token as part of `Session`. The package does not persist it.
The owner can pass the token to `WithToken` for authenticated calls. Keep token
fields out of formatted errors and debug output.

Do not introduce a generic REST framework, repository/service layers, code
generation, global mutable clients, or interfaces without a demonstrated need.
One concrete `Client` and explicit endpoint methods are preferred.

## HTTP rules

- Every operation accepts the caller's `context.Context`.
- Use an injected `*http.Client`; do not mutate `http.DefaultClient`.
- The real base URL must use HTTPS. Tests may use an HTTP `httptest.Server`.
- Resolve endpoint paths against only the configured base URL.
- Never send a password or bearer token to a different origin through a
  redirect. Prefer rejecting cross-origin redirects.
- Set `Accept: application/json` on API requests.
- Set `Content-Type: application/json` when sending JSON.
- Set `Authorization: Bearer <token>` only for authenticated endpoints.
- Encode request bodies with `encoding/json`, never string concatenation.
- Check the HTTP status before decoding a success model.
- Bound response bodies and reject oversized responses.
- Close every response body.
- Do not include passwords, bearer tokens, full authenticated bodies, or source
  code in errors.
- Wrap errors with the operation, such as `login: ...` or `list problems: ...`.

Use typed/sentinel errors only where the owner's CLI can make a useful choice,
at minimum authentication failure, invalid input, non-success HTTP status, and
invalid API response. Avoid an elaborate error hierarchy.

## Testing rules

Use `httptest.Server` for every endpoint test. Tests should assert:

- method and path;
- required headers;
- JSON request fields;
- bearer token presence where required and absence from login;
- successful typed decoding;
- `401`/`403` authentication behavior;
- representative non-2xx behavior;
- malformed JSON;
- truncated or oversized bodies;
- cancellation or timeout behavior where practical;
- secrets are absent from returned error strings.

Use fake usernames, passwords, tokens, source code, IDs, and response data.
Submitting to `https://grader.nattee.net` must never occur during `go test`.

## Implementation sequence

Follow `GOAL_GRADER_API_PACKAGE.md`. Implement one checkpoint at a time and
stop for owner review after each checkpoint. Do not build the CLI or TUI while
performing that goal.

The normal verification loop is:

```sh
gofmt -w ./internal
go test ./...
go vet ./...
```

Do not add production dependencies unless the standard library is demonstrably
insufficient and the owner approves the dependency first.
