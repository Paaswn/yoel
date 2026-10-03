package core

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	gapi "github.com/Paaswn/yoel/graderapi"
	"github.com/zalando/go-keyring"
)

func TestSaveAndLoadSession(t *testing.T) {
	keyring.MockInit()
	expires := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	want := &gapi.Session{Token: "fake-token", ExpiresAt: expires}

	if err := SaveSession(want); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	got, err := LoadSession()
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if got.Token != want.Token || !got.Expires.Equal(want.ExpiresAt) {
		t.Fatalf("LoadSession() = %#v, want token %q and expiry %v", got, want.Token, want.ExpiresAt)
	}
}

func TestLoadSessionExpired(t *testing.T) {
	keyring.MockInit()
	raw, err := json.Marshal(SavedSession{
		Token:   "expired-token",
		Expires: time.Now().Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := saveData(string(raw)); err != nil {
		t.Fatal(err)
	}

	got, err := LoadSession()
	if err != SessionExpiredError {
		t.Fatalf("LoadSession() error = %v, want %v", err, SessionExpiredError)
	}
	if got != (SavedSession{}) {
		t.Fatalf("LoadSession() session = %#v, want zero value on error", got)
	}
}

func TestLoadSessionRejectsMalformedData(t *testing.T) {
	keyring.MockInit()
	if err := saveData("{"); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadSession(); err == nil {
		t.Fatal("LoadSession() error = nil, want JSON decoding error")
	}
}

func TestLoginAndSaveSession(t *testing.T) {
	keyring.MockInit()
	expires := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/auth/login" {
			t.Errorf("request = %s %s, want POST /api/v1/auth/login", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token":      "login-token",
			"expires_at": expires.Format(time.RFC3339),
			"user":       map[string]any{"id": 1, "login": "test-user", "full_name": "Test User"},
		})
	}))
	defer server.Close()

	if err := LoginAndSaveSession(server.URL, "test-user", "fake-password", context.Background()); err != nil {
		t.Fatalf("LoginAndSaveSession() error = %v", err)
	}
	got, err := LoadSession()
	if err != nil {
		t.Fatalf("LoadSession() after login error = %v", err)
	}
	if got.Token != "login-token" || !got.Expires.Equal(expires) {
		t.Fatalf("saved session = %#v, want login token and expiry %v", got, expires)
	}
}

func TestLoginAndSaveSessionReturnsLoginError(t *testing.T) {
	keyring.MockInit()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "login rejected", http.StatusUnauthorized)
	}))
	defer server.Close()

	if err := LoginAndSaveSession(server.URL, "test-user", "fake-password", context.Background()); err == nil {
		t.Fatal("LoginAndSaveSession() error = nil, want login failure")
	}
}
