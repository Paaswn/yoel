package core

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	gapi "github.com/Paaswn/yoel/graderapi"
)

func TestSubmitProblem(t *testing.T) {
	const source = "int main() { return 0; }\n"
	for _, tc := range []struct {
		name, terminal, failure string
		wantStatus              Status
		posts, polls, metadata  int32
		wantErr                 error
	}{
		{"done after pending", "done", "", Done, 1, 2, 1, nil},
		{"compilation error", "compilation_error", "", CompilationError, 1, 1, 0, nil},
		{"grader error", "grader_error", "", GraderError, 1, 1, 0, nil},
		{"missing source", "", "source", Done, 0, 0, 0, os.ErrNotExist},
		{"submission rejected", "", "post", Done, 1, 0, 0, gapi.ErrAuthentication},
		{"polling rejected", "", "poll", Done, 1, 1, 0, gapi.ErrAuthentication},
		{"testcase fallback", "done", "metadata", Done, 1, 2, 1, nil},
		{"cancel while pending", "", "cancel", Done, 1, 1, 0, context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "main.cpp")
			if tc.failure != "source" {
				if err := os.WriteFile(path, []byte(source), 0600); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			points, compiler, grader, evaluation := 83.5, "syntax error", "grader unavailable", "P"
			final := gapi.Submission{
				ID: 99, ProblemID: 42, ProblemName: "sample", Language: "cpp",
				SubmittedAt: time.Unix(1700000000, 0).UTC(), Status: tc.terminal,
			}
			switch tc.terminal {
			case "done":
				final.Points = &points
				final.Evaluations = []gapi.Evaluation{{TestcaseID: 1, Result: &evaluation}, {TestcaseID: 2}}
			case "compilation_error":
				final.CompilerMessage = &compiler
			case "grader_error":
				final.GraderComment = &grader
			}
			var posts, polls, metadata atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer fake-token" {
					t.Error("missing fake bearer authentication")
				}
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/v1/problems/42/testcases":
					metadata.Add(1)
					if tc.failure == "metadata" {
						w.WriteHeader(http.StatusForbidden)
						return
					}
					// Preserve metadata order independently of evaluation order.
					_, _ = w.Write([]byte(`[{"id":2,"num":2,"group":8},{"id":1,"num":1,"group":3}]`))
				case r.Method == http.MethodPost && r.URL.Path == "/api/v1/problems/42/submissions":
					posts.Add(1)
					var request gapi.SubmissionRequest
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
					}
					if request.Source != source || request.Filename != "main.cpp" {
						t.Errorf("request = %#v, want source contents and basename main.cpp", request)
					}
					if tc.failure == "post" {
						w.WriteHeader(http.StatusUnauthorized)
						return
					}
					w.WriteHeader(http.StatusCreated)
					_, _ = w.Write([]byte(`{"id":99,"number":1,"status":"submitted"}`))
				case r.Method == http.MethodGet && r.URL.Path == "/api/v1/submissions/99":
					count := polls.Add(1)
					if tc.failure == "poll" {
						w.WriteHeader(http.StatusForbidden)
						return
					}
					response := final
					if (tc.terminal == "done" && count == 1) || tc.failure == "cancel" {
						response.Status = "grading"
					}
					if err := json.NewEncoder(w).Encode(response); err != nil {
						t.Error(err)
					}
					if tc.failure == "cancel" {
						// Let the pending response return, then cancel before the next poll.
						time.AfterFunc(20*time.Millisecond, cancel)
					}
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			client, err := gapi.NewClient(server.URL, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			r := newRegistryForTest(t)
			got, err := submitProblem(ctx, client.WithToken("fake-token"), ProblemLite{ID: 42, SourcePath: path}, r)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) || got.ID != 0 {
					t.Errorf("result ID = %d, error = %v; want no result and %v", got.ID, err, tc.wantErr)
				}
			} else if err != nil {
				t.Error(err)
			} else {
				if got.ID != 99 || got.ProblemID != 42 || got.ProblemName != "sample" || got.Status != tc.wantStatus {
					t.Errorf("unexpected final submission: %#v", got)
				}
				if tc.wantStatus != Done && len(got.TestCaseResults) != 0 {
					t.Error("failed submission should have no testcase results")
				}
				switch tc.wantStatus {
				case Done:
					if got.Points == nil || *got.Points != points {
						t.Error("lost final score")
					}
					var want []gapi.Testcase
					if tc.failure != "metadata" {
						group8, group3 := 8, 3
						want = []gapi.Testcase{
							{ID: 2, Num: 2, Group: &group8},
							{ID: 1, Num: 1, Group: &group3},
						}
					}
					if !reflect.DeepEqual(got.TestCaseResults, want) {
						t.Errorf("testcase metadata = %#v, want %#v", got.TestCaseResults, want)
					}
				case CompilationError:
					if got.CompilerMessage == nil || *got.CompilerMessage != compiler {
						t.Error("lost compiler message")
					}
				case GraderError:
					if got.GraderComment == nil || *got.GraderComment != grader {
						t.Error("lost grader comment")
					}
				}
			}
			if posts.Load() != tc.posts || polls.Load() != tc.polls || metadata.Load() != tc.metadata {
				t.Errorf("submit/poll/metadata requests = %d/%d/%d, want %d/%d/%d", posts.Load(), polls.Load(), metadata.Load(), tc.posts, tc.polls, tc.metadata)
			}
		})
	}
}
