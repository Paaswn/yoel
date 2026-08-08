package graderapi

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestListProblemsSendsAuthenticatedGETAndDecodesProblems(t *testing.T) {
	server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/problems" {
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
		_, _ = w.Write([]byte(`[
			{"id":42,"name":"arrays","full_name":"Array Problem","difficulty":3,"tags":["array"],"submission_count":2,"best_score":100,"last_score":50,"last_result":"partial","last_submission_time":"2030-01-02T03:04:05Z","last_submission_id":9,"has_testcase":true,"has_attachment":false,"permitted_languages":[{"id":1,"name":"cpp","ext":"cpp"}]},
			{"id":43,"name":"graphs","full_name":"Graph Problem","difficulty":null,"tags":[],"submission_count":0,"best_score":null,"last_score":null,"last_result":null,"last_submission_time":null,"last_submission_id":null,"has_testcase":false,"has_attachment":true,"permitted_languages":null}
		]`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	problems, err := client.WithToken("fake-token").ListProblems(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 2 {
		t.Fatalf("len(problems) = %d", len(problems))
	}
	if problems[0].ID != 42 || problems[0].Name != "arrays" || problems[0].Difficulty == nil || *problems[0].Difficulty != 3 {
		t.Fatalf("first problem = %#v", problems[0])
	}
	if problems[1].Difficulty != nil {
		t.Fatalf("second difficulty = %v, want nil", *problems[1].Difficulty)
	}
}

func TestListProblemsClassifiesAuthenticationFailure(t *testing.T) {
	server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.WithToken("fake-token").ListProblems(context.Background())
	if !errors.Is(err, ErrAuthentication) {
		t.Fatalf("error = %v, want ErrAuthentication", err)
	}
}

func TestListProblemsRejectsMalformedResponse(t *testing.T) {
	server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.WithToken("fake-token").ListProblems(context.Background())
	if !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("error = %v, want ErrInvalidResponse", err)
	}
}
