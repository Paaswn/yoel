package graderapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestLoginSendsExpectedJSONAndDecodesSession(t *testing.T) {
	server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/auth/login" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization = %q, want empty", got)
		}

		var request struct {
			Login    string `json:"login"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if request.Login != "fake-login" || request.Password != "fake-password" {
			t.Errorf("request body = %#v", request)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"fake-token","expires_at":"2030-01-02T03:04:05Z","user":{"id":7,"login":"fake-login","full_name":"Fake Student"}}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	session, err := client.WithToken("must-not-be-sent").Login(context.Background(), "fake-login", "fake-password")
	if err != nil {
		t.Fatal(err)
	}
	if session.Token != "fake-token" {
		t.Fatalf("Token = %q", session.Token)
	}
	if !session.ExpiresAt.Equal(time.Date(2030, time.January, 2, 3, 4, 5, 0, time.UTC)) {
		t.Fatalf("ExpiresAt = %v", session.ExpiresAt)
	}
	if session.User.ID != 7 || session.User.Login != "fake-login" || session.User.FullName != "Fake Student" {
		t.Fatalf("User = %#v", session.User)
	}
}

func TestLoginRejectsEmptyInputWithoutRequest(t *testing.T) {
	server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server received unexpected request")
	}))
	defer server.Close()

	client, err := NewClient(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range [][2]string{{"", "fake-password"}, {"fake-login", ""}} {
		_, err := client.Login(context.Background(), input[0], input[1])
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("Login(%q, %q) error = %v, want ErrInvalidInput", input[0], input[1], err)
		}
	}
}

func TestLoginRejectsMalformedOrIncompleteResponseWithoutSecrets(t *testing.T) {
	const fakeToken = "fake-token-that-must-not-be-in-errors"
	for name, response := range map[string]string{
		"malformed":     "not-json",
		"missing token": `{"expires_at":"2030-01-02T03:04:05Z","user":{"id":7,"login":"fake-login","full_name":"Fake Student"}}`,
		"missing user":  `{"token":"` + fakeToken + `","expires_at":"2030-01-02T03:04:05Z"}`,
	} {
		t.Run(name, func(t *testing.T) {
			server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(response))
			}))
			defer server.Close()

			client, err := NewClient(server.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Login(context.Background(), "fake-login", "fake-password")
			if !errors.Is(err, ErrInvalidResponse) {
				t.Fatalf("error = %v, want ErrInvalidResponse", err)
			}
			if strings.Contains(err.Error(), fakeToken) || strings.Contains(err.Error(), "fake-password") {
				t.Fatalf("error contains secret: %v", err)
			}
		})
	}
}

func TestLoginAuthenticationFailureDoesNotExposeSecrets(t *testing.T) {
	const fakePassword = "fake-password"
	server := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid credentials: ` + fakePassword + `"}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Login(context.Background(), "fake-login", fakePassword)
	if !errors.Is(err, ErrAuthentication) {
		t.Fatalf("error = %v, want ErrAuthentication", err)
	}
	if strings.Contains(err.Error(), fakePassword) {
		t.Fatalf("error contains password: %v", err)
	}
}
