package graderapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSubmitSendsExpectedJSONAndDecodesAcknowledgement(t *testing.T) {
	const source = "package main\n\nfunc main() { println(\"สวัสดี\") }\n"
	server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/problems/673/submissions" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer fake-token" {
			t.Errorf("Authorization = %q", got)
		}

		var request map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		var gotSource, filename string
		if err := json.Unmarshal(request["source"], &gotSource); err != nil {
			t.Errorf("decode source: %v", err)
		}
		if err := json.Unmarshal(request["filename"], &filename); err != nil {
			t.Errorf("decode filename: %v", err)
		}
		if gotSource != source || filename != "673.cpp" {
			t.Errorf("source = %q, filename = %q", gotSource, filename)
		}
		if _, exists := request["language_id"]; exists {
			t.Errorf("request includes language_id: %s", request["language_id"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":924618,"number":3,"status":"submitted"}`)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	submission, err := client.WithToken("fake-token").Submit(context.Background(), 673, SubmissionRequest{
		Source:   source,
		Filename: "673.cpp",
	})
	if err != nil {
		t.Fatal(err)
	}
	if submission.ID != 924618 || submission.Number != 3 || submission.Status != "submitted" {
		t.Fatalf("submission = %#v", submission)
	}
}

func TestSubmitIncludesExplicitLanguageID(t *testing.T) {
	server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		var languageID int
		if err := json.Unmarshal(request["language_id"], &languageID); err != nil {
			t.Errorf("decode language_id: %v", err)
		}
		if languageID != 5 {
			t.Errorf("language_id = %d, want 5", languageID)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":1,"number":1,"status":"submitted"}`)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.WithToken("fake-token").Submit(context.Background(), 673, SubmissionRequest{
		Source:     "int main() {}",
		Filename:   "673.cpp",
		LanguageID: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSubmitRejectsInvalidInputWithoutRequest(t *testing.T) {
	var requests atomic.Int32
	server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, test := range map[string]struct {
		problemID int
		request   SubmissionRequest
	}{
		"zero problem ID":     {0, SubmissionRequest{Source: "source", Filename: "1.cpp"}},
		"negative problem ID": {-1, SubmissionRequest{Source: "source", Filename: "1.cpp"}},
		"empty source":        {1, SubmissionRequest{Filename: "1.cpp"}},
		"empty filename":      {1, SubmissionRequest{Source: "source"}},
		"negative language":   {1, SubmissionRequest{Source: "source", Filename: "1.cpp", LanguageID: -1}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := client.WithToken("fake-token").Submit(context.Background(), test.problemID, test.request)
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("error = %v, want ErrInvalidInput", err)
			}
		})
	}
	if requests.Load() != 0 {
		t.Fatalf("server received %d requests, want 0", requests.Load())
	}
}

func TestSubmitClassifiesHTTPFailuresWithoutSecretsOrRetries(t *testing.T) {
	const (
		token  = "fake-submit-token"
		source = "fake private source code"
	)
	for name, status := range map[string]int{
		"unauthorized": http.StatusUnauthorized,
		"forbidden":    http.StatusForbidden,
		"server error": http.StatusInternalServerError,
	} {
		t.Run(name, func(t *testing.T) {
			var requests atomic.Int32
			server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.WriteHeader(status)
				_, _ = io.WriteString(w, token+source)
			}))
			defer server.Close()

			client, err := NewClient(server.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.WithToken(token).Submit(context.Background(), 673, SubmissionRequest{Source: source, Filename: "673.cpp"})
			if status == http.StatusUnauthorized || status == http.StatusForbidden {
				if !errors.Is(err, ErrAuthentication) {
					t.Fatalf("error = %v, want ErrAuthentication", err)
				}
			} else {
				var httpErr *HTTPError
				if !errors.As(err, &httpErr) || httpErr.StatusCode != status {
					t.Fatalf("error = %v, want HTTPError %d", err, status)
				}
			}
			if strings.Contains(err.Error(), token) || strings.Contains(err.Error(), source) {
				t.Fatalf("error exposes token or source: %v", err)
			}
			if requests.Load() != 1 {
				t.Fatalf("requests = %d, want exactly 1", requests.Load())
			}
		})
	}
}

func TestSubmitRejectsMalformedIncompleteAndOversizedResponses(t *testing.T) {
	responses := map[string]string{
		"malformed":      "not-json",
		"missing ID":     `{"number":3,"status":"submitted"}`,
		"missing number": `{"id":924618,"status":"submitted"}`,
		"missing status": `{"id":924618,"number":3}`,
		"oversized":      strings.Repeat("x", maxResponseBodySize+1),
	}
	for name, response := range responses {
		t.Run(name, func(t *testing.T) {
			server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusCreated)
				_, _ = io.WriteString(w, response)
			}))
			defer server.Close()

			client, err := NewClient(server.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.WithToken("fake-token").Submit(context.Background(), 673, SubmissionRequest{Source: "fake source", Filename: "673.cpp"})
			if !errors.Is(err, ErrInvalidResponse) {
				t.Fatalf("error = %v, want ErrInvalidResponse", err)
			}
		})
	}
}

func TestGetSubmissionSendsAuthenticatedGETAndDecodesResult(t *testing.T) {
	server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/submissions/924618" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer fake-token" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "" {
			t.Errorf("Content-Type = %q, want empty", got)
		}
		_, _ = io.WriteString(w, `{
			"id":924618,
			"problem_id":673,
			"problem_name":"hello",
			"user_id":7,
			"language":"cpp",
			"source":"private source that must not be retained",
			"source_filename":"673.cpp",
			"submitted_at":"2030-01-02T03:04:05Z",
			"points":75.5,
			"status":"done",
			"grader_comment":"partial score",
			"compiler_message":null,
			"max_runtime":0.125,
			"peak_memory":2048,
			"number":3,
			"evaluations":[{"testcase_id":11,"result":"correct","score":25.5,"time":12,"memory":256}]
		}`)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	submission, err := client.WithToken("fake-token").GetSubmission(context.Background(), 924618)
	if err != nil {
		t.Fatal(err)
	}
	if submission.ID != 924618 || submission.ProblemID != 673 || submission.ProblemName != "hello" || submission.Language != "cpp" || submission.Number != 3 || submission.Status != "done" {
		t.Fatalf("submission = %#v", submission)
	}
	if submission.Points == nil || *submission.Points != 75.5 || submission.GraderComment == nil || *submission.GraderComment != "partial score" {
		t.Fatalf("submission result fields = %#v", submission)
	}
	if submission.MaxRuntime == nil || *submission.MaxRuntime != 0.125 || submission.PeakMemory == nil || *submission.PeakMemory != 2048 {
		t.Fatalf("submission resource fields = %#v", submission)
	}
	if len(submission.Evaluations) != 1 || submission.Evaluations[0].Result == nil || *submission.Evaluations[0].Result != "correct" {
		t.Fatalf("evaluations = %#v", submission.Evaluations)
	}
}

func TestGetSubmissionRejectsInvalidIDWithoutRequest(t *testing.T) {
	var requests atomic.Int32
	server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, submissionID := range []int{0, -1} {
		if _, err := client.GetSubmission(context.Background(), submissionID); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("GetSubmission(%d) error = %v, want ErrInvalidInput", submissionID, err)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("server received %d requests, want 0", requests.Load())
	}
}

func TestGetSubmissionClassifiesAuthenticationFailure(t *testing.T) {
	server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, "fake-private-response")
	}))
	defer server.Close()

	client, err := NewClient(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.WithToken("fake-token").GetSubmission(context.Background(), 924618)
	if !errors.Is(err, ErrAuthentication) {
		t.Fatalf("error = %v, want ErrAuthentication", err)
	}
	if strings.Contains(err.Error(), "fake-token") || strings.Contains(err.Error(), "fake-private-response") {
		t.Fatalf("error exposes secret: %v", err)
	}
}

func TestGetSubmissionRejectsMalformedIncompleteAndOversizedResponses(t *testing.T) {
	responses := map[string]string{
		"malformed":          "not-json",
		"missing ID":         `{"problem_id":673,"language":"cpp","submitted_at":"2030-01-02T03:04:05Z"}`,
		"missing problem ID": `{"id":924618,"language":"cpp","submitted_at":"2030-01-02T03:04:05Z"}`,
		"missing language":   `{"id":924618,"problem_id":673,"submitted_at":"2030-01-02T03:04:05Z"}`,
		"missing timestamp":  `{"id":924618,"problem_id":673,"language":"cpp"}`,
		"oversized":          strings.Repeat("x", maxResponseBodySize+1),
	}
	for name, response := range responses {
		t.Run(name, func(t *testing.T) {
			server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, response)
			}))
			defer server.Close()

			client, err := NewClient(server.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.WithToken("fake-token").GetSubmission(context.Background(), 924618)
			if !errors.Is(err, ErrInvalidResponse) {
				t.Fatalf("error = %v, want ErrInvalidResponse", err)
			}
		})
	}
}

func TestSubmitHonorsContextCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)

	client, err := NewClient(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, requestErr := client.WithToken("fake-token").Submit(ctx, 673, SubmissionRequest{Source: "fake source", Filename: "673.cpp"})
		result <- requestErr
	}()
	<-started
	cancel()

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Submit did not observe cancellation")
	}
}
