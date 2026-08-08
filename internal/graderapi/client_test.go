package graderapi

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewClientRejectsUnsafeBaseURLs(t *testing.T) {
	t.Parallel()

	for _, baseURL := range []string{
		"",
		"://not-a-url",
		"file:///tmp/grader",
		"http://example.com",
		"https://",
		"https://user:password@example.com",
		"https://example.com?token=secret",
	} {
		t.Run(baseURL, func(t *testing.T) {
			if _, err := NewClient(baseURL, nil); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("NewClient(%q) error = %v, want ErrInvalidInput", baseURL, err)
			}
		})
	}
}

func TestNewClientAcceptsLoopbackHTTP(t *testing.T) {
	server := newTestServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()

	if _, err := NewClient(server.URL, nil); err != nil {
		t.Fatalf("NewClient(%q): %v", server.URL, err)
	}
}

func TestWithTokenCopiesClient(t *testing.T) {
	var authorization atomic.Value
	server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization.Store(r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	original, err := NewClient(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	derived := original.WithToken("fake-bearer-token")

	if _, err := derived.do(context.Background(), "test request", http.MethodGet, "/test", nil, true); err != nil {
		t.Fatal(err)
	}
	if got := authorization.Load().(string); got != "Bearer fake-bearer-token" {
		t.Fatalf("derived Authorization = %q", got)
	}

	if _, err := original.do(context.Background(), "test request", http.MethodGet, "/test", nil, true); err != nil {
		t.Fatal(err)
	}
	if got := authorization.Load().(string); got != "" {
		t.Fatalf("original Authorization = %q, want empty", got)
	}
}

func TestDoSetsJSONHeadersAndReadsResponse(t *testing.T) {
	server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/test" {
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
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		if string(body) != `{"example":"value"}` {
			t.Errorf("body = %q", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL+"/api", nil)
	if err != nil {
		t.Fatal(err)
	}
	body, err := client.WithToken("fake-token").do(
		context.Background(),
		"test request",
		http.MethodPost,
		"/api/test",
		strings.NewReader(`{"example":"value"}`),
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"ok":true}` {
		t.Fatalf("response body = %q", body)
	}
}

func TestDoClassifiesHTTPStatusWithoutLeakingBody(t *testing.T) {
	const secret = "fake-secret-response-token"
	server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(secret))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.do(context.Background(), "login", http.MethodGet, "/test", nil, false)
	if !errors.Is(err, ErrAuthentication) {
		t.Fatalf("error = %v, want ErrAuthentication", err)
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("error = %v, want HTTPError 401", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error contains response secret: %v", err)
	}
}

func TestDoRejectsOversizedResponse(t *testing.T) {
	server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat("x", maxResponseBodySize+1))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.do(context.Background(), "test request", http.MethodGet, "/test", nil, false)
	if !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("error = %v, want ErrInvalidResponse", err)
	}
}

func TestDoHonorsCancellation(t *testing.T) {
	started := make(chan struct{})
	server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()

	client, err := NewClient(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, requestErr := client.do(ctx, "cancelled request", http.MethodGet, "/test", nil, false)
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
		t.Fatal("request did not observe cancellation")
	}
}

func TestDoRejectsCrossOriginRedirectBeforeFollowingIt(t *testing.T) {
	var targetRequests atomic.Int32
	target := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetRequests.Add(1)
	}))
	defer target.Close()

	source := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/target", http.StatusTemporaryRedirect)
	}))
	defer source.Close()

	const token = "fake-redirect-token"
	client, err := NewClient(source.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.WithToken(token).do(context.Background(), "redirected request", http.MethodGet, "/start", nil, true)
	if err == nil {
		t.Fatal("redirected request succeeded, want error")
	}
	if targetRequests.Load() != 0 {
		t.Fatal("cross-origin redirect reached target server")
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("error contains bearer token: %v", err)
	}
}

func newTestServer(handler http.Handler) *httptest.Server {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: handler},
	}
	server.Start()
	return server
}
