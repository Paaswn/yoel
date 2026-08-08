package cli

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewRootCommandShowsHelpWithoutSubcommands(t *testing.T) {
	command := NewRootCommand()
	output := new(bytes.Buffer)
	command.SetOut(output)
	command.SetErr(output)

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !bytes.Contains(output.Bytes(), []byte("Usage:")) {
		t.Fatalf("help output = %q, want Usage", output.String())
	}
}

func TestNewRootCommandIncludesLoginCommand(t *testing.T) {
	root := NewRootCommand()
	login, _, err := root.Find([]string{"login"})
	if err != nil {
		t.Fatalf("Find(login) error = %v", err)
	}
	if login == root {
		t.Fatal("Find(login) returned the root command")
	}
	if login.Use != "login" {
		t.Fatalf("login.Use = %q, want %q", login.Use, "login")
	}
}

func TestLoginCommandCallsGraderAPI(t *testing.T) {
	server := newCLITestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/auth/login" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		var request map[string]string
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if request["login"] != "fake-login" || request["password"] != "fake-password" {
			t.Errorf("request body = %#v", request)
		}
		_, _ = w.Write([]byte(`{"token":"fake-token","expires_at":"2030-01-02T03:04:05Z","user":{"id":7,"login":"fake-login","full_name":"Fake Student"}}`))
	}))
	defer server.Close()

	command := NewRootCommand()
	output := new(bytes.Buffer)
	command.SetOut(output)
	command.SetErr(output)
	command.SetArgs([]string{
		"login",
		"--base-url", server.URL,
		"--login", "fake-login",
		"--password", "fake-password",
	})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := output.String(); got != "login successful\n" {
		t.Fatalf("output = %q", got)
	}
	if bytes.Contains(output.Bytes(), []byte("fake-token")) {
		t.Fatal("output contains the bearer token")
	}
}

func newCLITestServer(handler http.Handler) *httptest.Server {
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
