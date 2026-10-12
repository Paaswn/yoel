package graderapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
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

func TestListTestcasesDecodesMetadataAndPreservesOrder(t *testing.T) {
	server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/problems/42/testcases" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer fake-token" {
			t.Error("missing fake bearer authentication")
		}
		if r.Header.Get("Accept") != "application/json" || r.Header.Get("Content-Type") != "" {
			t.Error("incorrect JSON GET headers")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[
            {"id":30,"num":3,"group":2,"group_name":"main","weight":35},
            {"id":10,"num":1,"group":null,"group_name":null,"weight":null},
            {"id":20,"num":2,"group":0,"group_name":"","weight":0}
        ]`)
	}))
	defer server.Close()
	client, err := NewClient(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.WithToken("fake-token").ListTestcases(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	group, weight, zero := 2, 35, 0
	name, empty := "main", ""
	want := []Testcase{
		{ID: 30, Num: 3, Group: &group, GroupName: &name, Weight: &weight},
		{ID: 10, Num: 1},
		{ID: 20, Num: 2, Group: &zero, GroupName: &empty, Weight: &zero},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("testcases = %#v, want %#v", got, want)
	}
}

func TestListTestcasesAcceptsEmptyArray(t *testing.T) {
	server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "[]")
	}))
	defer server.Close()
	client, err := NewClient(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.WithToken("fake-token").ListTestcases(context.Background(), 42)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("result = %#v, error = %v; want empty array without error", got, err)
	}
}

func TestListTestcasesRejectsInvalidIDWithoutRequest(t *testing.T) {
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
		if got, err := client.ListTestcases(context.Background(), id); got != nil || !errors.Is(err, ErrInvalidInput) {
			t.Errorf("ID %d result = %#v, error = %v; want nil, ErrInvalidInput", id, got, err)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("requests = %d, want 0", requests.Load())
	}
}

func TestListTestcasesRejectsMalformedResponses(t *testing.T) {
	for _, body := range []string{"not JSON", "[", `{}`, `[{"id":"bad","num":1}]`, `[{"id":1,"num":1,"weight":"bad"}]`} {
		t.Run(body, func(t *testing.T) {
			server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, body)
			}))
			defer server.Close()
			client, err := NewClient(server.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got, err := client.WithToken("fake-token").ListTestcases(context.Background(), 42); got != nil || !errors.Is(err, ErrInvalidResponse) {
				t.Fatalf("result = %#v, error = %v; want nil, ErrInvalidResponse", got, err)
			}
		})
	}
}

func TestListTestcasesPreservesHTTPFailures(t *testing.T) {
	const token, privateBody = "fake-testcase-token", "private-permission-detail"
	for _, status := range []int{401, 403, 404, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, _ = io.WriteString(w, privateBody)
			}))
			defer server.Close()
			client, err := NewClient(server.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			got, err := client.WithToken(token).ListTestcases(context.Background(), 42)
			var httpErr *HTTPError
			if got != nil || !errors.As(err, &httpErr) || httpErr.StatusCode != status {
				t.Fatalf("result = %#v, error = %v; want nil, HTTPError %d", got, err, status)
			}
			if (status == 401 || status == 403) && !errors.Is(err, ErrAuthentication) {
				t.Errorf("error = %v, want existing authentication classification", err)
			}
			if strings.Contains(err.Error(), token) || strings.Contains(err.Error(), privateBody) {
				t.Error("error exposes private request or response data")
			}
		})
	}
}

func TestListTestcasesHonorsCancellation(t *testing.T) {
	started := make(chan struct{})
	server := newTestServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	client, err := NewClient(server.URL, &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := client.WithToken("fake-token").ListTestcases(ctx, 42)
		result <- err
	}()
	select {
	case <-started:
		cancel()
	case err := <-result:
		t.Fatalf("request ended before cancellation: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("request did not reach local server")
	}
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("request did not honor cancellation")
	}
}
