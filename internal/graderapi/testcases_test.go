package graderapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDownloadTestcaseFiles(t *testing.T) {
	for _, test := range []struct {
		name string
		path string
		call func(*Client) ([]byte, error)
		want string
	}{
		{
			name: "input",
			path: "/api/v1/testcases/11464/input",
			call: func(client *Client) ([]byte, error) {
				return client.DownloadTestcaseInput(context.Background(), 11464)
			},
			want: "5\n1 2 3 4 5\n",
		},
		{
			name: "solution",
			path: "/api/v1/testcases/11464/sol",
			call: func(client *Client) ([]byte, error) {
				return client.DownloadTestcaseSolution(context.Background(), 11464)
			},
			want: "15\n",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != test.path {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if got := r.Header.Get("Authorization"); got != "Bearer fake-testcase-token" {
					t.Errorf("Authorization = %q", got)
				}
				if got := r.Header.Get("Accept"); got != "text/plain" {
					t.Errorf("Accept = %q", got)
				}
				if got := r.Header.Get("Content-Type"); got != "" {
					t.Errorf("Content-Type = %q, want empty", got)
				}
				_, _ = io.WriteString(w, test.want)
			}))
			defer server.Close()

			client, err := NewClient(server.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			got, err := test.call(client.WithToken("fake-testcase-token"))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != test.want {
				t.Fatalf("data = %q, want %q", got, test.want)
			}
		})
	}
}

func TestDownloadTestcaseFilesRejectInvalidIDWithoutRequest(t *testing.T) {
	var requests atomic.Int32
	server := newTestServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{0, -1} {
		if _, err := client.DownloadTestcaseInput(context.Background(), id); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("ID %d error = %v, want ErrInvalidInput", id, err)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("requests = %d, want 0", requests.Load())
	}
}

func TestDownloadTestcaseFilesClassifyHTTPFailuresWithoutSecrets(t *testing.T) {
	const token = "fake-private-testcase-token"
	for _, status := range []int{http.StatusForbidden, http.StatusNotFound, http.StatusTooManyRequests} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, _ = io.WriteString(w, token)
			}))
			defer server.Close()

			client, err := NewClient(server.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.WithToken(token).DownloadTestcaseInput(context.Background(), 11464)
			if status == http.StatusForbidden {
				if !errors.Is(err, ErrAuthentication) {
					t.Fatalf("error = %v, want ErrAuthentication", err)
				}
			} else {
				var httpErr *HTTPError
				if !errors.As(err, &httpErr) || httpErr.StatusCode != status {
					t.Fatalf("error = %v, want HTTPError %d", err, status)
				}
			}
			if strings.Contains(err.Error(), token) {
				t.Fatalf("error exposes token: %v", err)
			}
		})
	}
}

func TestDownloadTestcaseFileWithoutTokenClassifiesAuthenticationFailure(t *testing.T) {
	server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization = %q, want empty", got)
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.DownloadTestcaseInput(context.Background(), 1); !errors.Is(err, ErrAuthentication) {
		t.Fatalf("error = %v, want ErrAuthentication", err)
	}
}

func TestDownloadTestcaseFileRejectsOversizedBody(t *testing.T) {
	server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat("x", int(maxTestcaseFileSize+1)))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.DownloadTestcaseInput(context.Background(), 1)
	if !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("error = %v, want ErrInvalidResponse", err)
	}
}

func TestDownloadTestcaseFileRejectsTruncatedBody(t *testing.T) {
	httpClient := &http.Client{Transport: testcaseRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(io.MultiReader(strings.NewReader("partial"), testcaseErrorReader{})),
		}, nil
	})}
	client, err := NewClient("https://grader.example", httpClient)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.DownloadTestcaseSolution(context.Background(), 1)
	if !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("error = %v, want ErrInvalidResponse", err)
	}
}

func TestDownloadTestcaseFileHonorsCancellation(t *testing.T) {
	started := make(chan struct{})
	server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()

	client, err := NewClient(server.URL, &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, downloadErr := client.WithToken("fake-token").DownloadTestcaseSolution(ctx, 1)
		result <- downloadErr
	}()
	<-started
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

type testcaseRoundTripFunc func(*http.Request) (*http.Response, error)

func (f testcaseRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type testcaseErrorReader struct{}

func (testcaseErrorReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}
