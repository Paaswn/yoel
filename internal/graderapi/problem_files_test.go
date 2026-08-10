package graderapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestDownloadProblemPDF(t *testing.T) {
	wantData := []byte("%PDF-1.7\nfake problem PDF")

	server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		if r.URL.Path != "/api/v1/problems/673/files/pdf" {
			t.Errorf("path = %q, want API PDF path", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer fake-token" {
			t.Errorf("Authorization = %q, want bearer token", got)
		}
		if got := r.Header.Get("Accept"); got != "application/pdf" {
			t.Errorf("Accept = %q, want application/pdf", got)
		}

		w.Header().Set("Content-Disposition", `attachment; filename="../problem 673.pdf"`)
		w.Header().Set("Content-Type", "application/pdf; version=1.7")
		_, _ = w.Write(wantData)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	file, err := client.WithToken("fake-token").DownloadProblemPDF(context.Background(), 673)
	if err != nil {
		t.Fatalf("DownloadProblemPDF: %v", err)
	}
	if !bytes.Equal(file.Data, wantData) {
		t.Errorf("Data = %q, want %q", file.Data, wantData)
	}
	if file.Filename != "problem 673.pdf" {
		t.Errorf("Filename = %q, want safe basename", file.Filename)
	}
	if file.ContentType != "application/pdf; version=1.7" {
		t.Errorf("ContentType = %q, want response content type", file.ContentType)
	}
}

func TestDownloadProblemAttachment(t *testing.T) {
	wantData := []byte("PK\x03\x04fake zip")
	server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/problems/673/files/attachment" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer fake-token" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/zip, application/octet-stream" {
			t.Errorf("Accept = %q", got)
		}
		w.Header().Set("Content-Disposition", `attachment; filename="../starter.zip"`)
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(wantData)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	file, err := client.WithToken("fake-token").DownloadProblemAttachment(context.Background(), 673)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(file.Data, wantData) || file.Filename != "starter.zip" || file.ContentType != "application/zip" {
		t.Fatalf("file = %#v", file)
	}
}

func TestDownloadProblemAttachmentRejectsInvalidIDAndEmptyResponse(t *testing.T) {
	client, err := NewClient("https://grader.example", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.DownloadProblemAttachment(context.Background(), 0); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid ID error = %v, want ErrInvalidInput", err)
	}

	server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()
	client, err = NewClient(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.WithToken("fake-token").DownloadProblemAttachment(context.Background(), 673); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("empty response error = %v, want ErrInvalidResponse", err)
	}
}

func TestDownloadProblemAttachmentNonSuccessDoesNotExposeSecrets(t *testing.T) {
	const token = "fake-attachment-token"
	const bodySecret = "fake-private-attachment-response"
	server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, bodySecret)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.WithToken(token).DownloadProblemAttachment(context.Background(), 673)
	if !errors.Is(err, ErrAuthentication) {
		t.Fatalf("error = %v, want ErrAuthentication", err)
	}
	if strings.Contains(err.Error(), token) || strings.Contains(err.Error(), bodySecret) {
		t.Fatalf("error exposes secret: %v", err)
	}
}

func TestDownloadProblemPDFRejectsInvalidProblemID(t *testing.T) {
	client, err := NewClient("https://grader.example", nil)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.DownloadProblemPDF(context.Background(), 0)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("error = %v, want ErrInvalidInput", err)
	}
}

func TestDownloadProblemPDFMissingAuthentication(t *testing.T) {
	server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization = %q, want empty", got)
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.DownloadProblemPDF(context.Background(), 673)
	if !errors.Is(err, ErrAuthentication) {
		t.Fatalf("error = %v, want ErrAuthentication", err)
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("error = %v, want HTTPError with status 401", err)
	}
}

func TestDownloadProblemPDFNonSuccessDoesNotExposeSecrets(t *testing.T) {
	const fakeToken = "fake-super-secret-token"
	const fakeBodySecret = "fake-private-response-body"

	server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, fakeBodySecret)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.WithToken(fakeToken).DownloadProblemPDF(context.Background(), 673)
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusInternalServerError {
		t.Fatalf("error = %v, want HTTPError with status 500", err)
	}
	if strings.Contains(err.Error(), fakeToken) || strings.Contains(err.Error(), fakeBodySecret) {
		t.Fatalf("error exposes a fake secret: %v", err)
	}
}

func TestDownloadProblemPDFRejectsMalformedResponse(t *testing.T) {
	const fakeBody = "not-a-pdf-private-body"

	server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = io.WriteString(w, fakeBody)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.DownloadProblemPDF(context.Background(), 673)
	if !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("error = %v, want ErrInvalidResponse", err)
	}
	if strings.Contains(err.Error(), fakeBody) {
		t.Fatalf("error exposes response body: %v", err)
	}
}

func TestDownloadProblemPDFRejectsTruncatedResponse(t *testing.T) {
	server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = io.WriteString(w, "%PDF-short")
	}))
	defer server.Close()

	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.DownloadProblemPDF(context.Background(), 673)
	if !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("error = %v, want ErrInvalidResponse", err)
	}
}

func TestDownloadProblemPDFRejectsOversizedResponse(t *testing.T) {
	server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = io.CopyN(w, repeatingByteReader{'x'}, maxProblemPDFSize+1)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.DownloadProblemPDF(context.Background(), 673)
	if !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("error = %v, want ErrInvalidResponse", err)
	}
}

func TestDownloadProblemPDFHonorsContextCancellation(t *testing.T) {
	requestStarted := make(chan struct{})
	server := newTestServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		<-r.Context().Done()
	}))
	defer server.Close()

	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := client.DownloadProblemPDF(ctx, 673)
		result <- err
	}()

	select {
	case <-requestStarted:
		cancel()
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("request did not reach test server")
	}

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("DownloadProblemPDF did not return after cancellation")
	}
}

func TestProblemPDFFilename(t *testing.T) {
	tests := []struct {
		name               string
		contentDisposition string
		want               string
	}{
		{name: "missing", want: "problem-673.pdf"},
		{name: "path removed", contentDisposition: `attachment; filename="../../secret.pdf"`, want: "secret.pdf"},
		{name: "encoded unicode", contentDisposition: `attachment; filename*=UTF-8''caf%C3%A9.pdf`, want: "café.pdf"},
		{name: "malformed", contentDisposition: `attachment; filename="unterminated`, want: "problem-673.pdf"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := problemPDFFilename(test.contentDisposition, 673); got != test.want {
				t.Errorf("problemPDFFilename() = %q, want %q", got, test.want)
			}
		})
	}
}

type repeatingByteReader struct {
	b byte
}

func (r repeatingByteReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = r.b
	}
	return len(p), nil
}
