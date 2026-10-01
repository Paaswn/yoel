package graderapi

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestGetProblemStatementPDFDownloadsValidatedPDF(t *testing.T) {
	want := []byte("%PDF-1.7\nfake statement")
	server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/problems/673/download/statement" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Accept"); got != "application/pdf" {
			t.Errorf("Accept = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer fake-token" {
			t.Errorf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(want)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.WithToken("fake-token").GetProblemStatementPDF(context.Background(), 673)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("PDF = %q", got)
	}
}

func TestGetProblemStatementPDFRejectsInvalidID(t *testing.T) {
	client, err := NewClient("https://grader.example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.GetProblemStatementPDF(context.Background(), 0)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("error = %v, want ErrInvalidInput", err)
	}
}

func TestGetProblemStatementPDFRejectsNonPDFWithoutExposingBody(t *testing.T) {
	const secret = "fake-sensitive-html-body"
	server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(secret))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.WithToken("fake-token").GetProblemStatementPDF(context.Background(), 673)
	if !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("error = %v, want ErrInvalidResponse", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error contains response body: %v", err)
	}
}

func TestGetProblemStatementPDFClassifiesAuthenticationFailure(t *testing.T) {
	server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.WithToken("fake-token").GetProblemStatementPDF(context.Background(), 673)
	if !errors.Is(err, ErrAuthentication) {
		t.Fatalf("error = %v, want ErrAuthentication", err)
	}
}
